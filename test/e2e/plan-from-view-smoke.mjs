#!/usr/bin/env node

// End-to-end smoke for "plan a spec from the viewer" (US-029).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the arcipelago execution provider, the receipt it
// demands and the records it persists under `.archetipo/executions/`. Only the
// ARcipelago hub is replaced, by a local Node server bound to 127.0.0.1, so the
// run needs no credential and no network. The fake hub does not simulate the
// planning either: for the success spec it really runs
// `archetipo spec plan US-901 --file <payload>` inside the sandbox before it
// declares the remote task `completed`, so the spec reaches PLANNED through the
// same command a remote agent would run.

import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildCLI as buildCLIShared, createRunDir as createRunDirShared, makeRunCommand, parseCommonArgs, readBody, stopProcess as stopProcessShared, waitForHTTP } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "plan-from-view-smoke");

// The credential is a sentinel: the arcipelago provider must send it to the hub
// (which checks it) and the viewer must never echo it back to the browser.
const TOKEN_SENTINEL = "smoke-secret-token-value";
const WORKSPACE_ID = "ws-smoke";
const SUCCESS_SPEC = "US-901";
const FAILURE_SPEC = "US-902";
const PLAN_TASKS = 2;
const REMOTE_FAILURE_REASON = "smoke: the remote agent aborted before persisting any plan";

const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, ARCIPELAGO_TOKEN: TOKEN_SENTINEL };

// Every viewer response body is kept so the final check can prove the sentinel
// never travelled to the browser, on any route touched by this run.
const viewerBodies = [];

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  // Starting a viewer records its project root in the user-level registry of
  // known workspaces. This run directory is a throwaway, so the entry must go
  // with it instead of accumulating in the real registry of the machine.
  process.env.ARCHETIPO_STATE_DIR = path.join(runDir, "state");
  cliEnv.ARCHETIPO_STATE_DIR = process.env.ARCHETIPO_STATE_DIR;
  const sandboxDir = path.join(runDir, "sandbox");
  const specsFile = path.join(runDir, "specs.json");
  const executionsDir = path.join(sandboxDir, ".archetipo", "executions");

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  let hub;
  try {
    await buildCLI();
    await runCommand("init", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], { cwd: sandboxDir });
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir });
    const planFile = await writePlanPayload(runDir);

    hub = await startFakeHub({ sandboxDir, planFile });
    console.log(`-> fake ARcipelago hub: ${hub.url}`);

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);

    // The workspace default provider is the real arcipelago one, pointed at the
    // local hub. Saving it through the API is what a user does in the Execution
    // panel, so the run starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: "arcipelago",
        config: {
          base_url: hub.url,
          workspace_id: WORKSPACE_ID,
          poll_interval_seconds: 1,
          timeout_seconds: 30,
        },
      }),
    });

    // AC-1 — one press creates exactly one execution, bound to the spec and to
    // the default provider, and one record on disk.
    //
    // Il server non rifiuta più una seconda pressione mentre la prima gira. Era
    // un vincolo illusorio — bastava aprire una conversazione accanto e chiedere
    // a mano qualcosa in conflitto — ed è diventato dannoso quando un'azione è
    // diventata una conversazione: una run ferma su una domanda resta aperta
    // finché qualcuno non risponde, e la spec di cui parla sarebbe rimasta
    // chiusa a ogni altro passo, fino a un'ora intera, per colpa di una run che
    // aspettava e basta. «Una pressione, una run» resta, e sta dove un debounce
    // ha senso: il bottone si disabilita al click.
    const started = await apiJSON(`${view.url}/api/spec/${SUCCESS_SPEC}/execution`, postAction("plan"), 201);
    if (started.status !== "RUNNING" || started.provider_id !== "arcipelago" || started.spec_code !== SUCCESS_SPEC) {
      throw new Error(`Unexpected execution record on start: ${JSON.stringify(started)}`);
    }
    if (!started.id || !started.created_at) {
      throw new Error(`The started execution is not identifiable: ${JSON.stringify(started)}`);
    }
    const afterStart = await listExecutionRecords(executionsDir);
    if (afterStart.length !== 1) {
      throw new Error(`Expected a single persisted record after one press; got ${JSON.stringify(afterStart)}`);
    }
    const remote = await hub.waitForTask(SUCCESS_SPEC);
    if (remote.request.externalId !== started.id || remote.request.source !== "archetipo" || remote.request.workspaceId !== WORKSPACE_ID) {
      throw new Error(`The remote task does not carry the ARchetipo external identity: ${JSON.stringify(remote.request)}`);
    }
    console.log(`-> AC-1 ok: one execution ${started.id} on provider arcipelago, one record on disk`);

    // AC-2 — while the hub has not answered yet the record is non-terminal and
    // the rest of the workspace stays navigable. The hub is gated, so the window
    // is deterministic and no sleep is involved.
    const running = await apiJSON(`${view.url}/api/execution/${started.id}`);
    if (running.status !== "RUNNING" || running.completed_at) {
      throw new Error(`Expected a non-terminal record while the hub is working; got ${JSON.stringify(running)}`);
    }
    const board = await apiJSON(`${view.url}/api/board`);
    if (!board || typeof board !== "object") {
      throw new Error("The board must stay reachable while an execution runs");
    }
    const detailWhileRunning = await apiJSON(`${view.url}/api/spec/${SUCCESS_SPEC}`);
    const planAction = (detailWhileRunning.actions || []).find((a) => a.id === "plan");
    if (!planAction || planAction.runnable !== false || !String(planAction.unavailable_reason || "").includes(started.id)) {
      throw new Error(`While running, the plan action must be reported unavailable naming the execution; got ${JSON.stringify(planAction)}`);
    }
    console.log("-> AC-2 ok: record RUNNING, board served, plan action reported unavailable without blocking navigation");

    // AC-3 — the hub really plans the spec, then declares the task completed
    // with the receipt the provider demands.
    await hub.completeWithPlan(SUCCESS_SPEC);
    const succeeded = await waitForTerminal(view.url, started.id);
    if (succeeded.status !== "SUCCEEDED") {
      throw new Error(`Expected SUCCEEDED; got ${JSON.stringify(succeeded)}`);
    }
    if (!succeeded.result || succeeded.result.external_id !== remote.id) {
      throw new Error(`The successful record must carry the remote task id; got ${JSON.stringify(succeeded.result)}`);
    }
    const plannedDetail = await apiJSON(`${view.url}/api/spec/${SUCCESS_SPEC}`);
    if (plannedDetail.spec.status !== "PLANNED") {
      throw new Error(`Expected ${SUCCESS_SPEC} to be PLANNED; got ${plannedDetail.spec.status}`);
    }
    if (!Array.isArray(plannedDetail.tasks) || plannedDetail.tasks.length !== PLAN_TASKS) {
      throw new Error(`Expected ${PLAN_TASKS} persisted tasks; got ${JSON.stringify((plannedDetail.tasks || []).map((t) => t.id))}`);
    }
    if (!String(plannedDetail.plan_body || "").trim()) {
      throw new Error("The spec detail must expose the persisted plan body");
    }
    if (!plannedDetail.execution || plannedDetail.execution.id !== started.id || plannedDetail.execution.status !== "SUCCEEDED") {
      throw new Error(`The detail must report the same execution as SUCCEEDED; got ${JSON.stringify(plannedDetail.execution)}`);
    }
    const persistedSuccess = await readExecutionRecord(executionsDir, started.id);
    if (persistedSuccess.status !== "SUCCEEDED" || !persistedSuccess.completed_at) {
      throw new Error(`The persisted record does not match the served one: ${JSON.stringify(persistedSuccess)}`);
    }
    console.log(`-> AC-3 ok: ${SUCCESS_SPEC} is PLANNED with ${PLAN_TASKS} tasks and a plan body, execution ${started.id} SUCCEEDED on disk`);

    // AC-4 — the failure path: the hub declares the task failed, the record
    // carries the remote reason and the spec never becomes PLANNED.
    const failing = await apiJSON(`${view.url}/api/spec/${FAILURE_SPEC}/execution`, postAction("plan"), 201);
    if (failing.status !== "RUNNING") {
      throw new Error(`Expected the second execution to start RUNNING; got ${JSON.stringify(failing)}`);
    }
    const failingRemote = await hub.waitForTask(FAILURE_SPEC);
    await hub.declareFailed(FAILURE_SPEC);
    const failed = await waitForTerminal(view.url, failing.id);
    if (failed.status !== "FAILED" || !failed.error) {
      throw new Error(`Expected FAILED with a reason; got ${JSON.stringify(failed)}`);
    }
    if (!failed.error.message.includes(REMOTE_FAILURE_REASON) || !failed.error.message.includes(failingRemote.id)) {
      throw new Error(`The failure must report the remote cause and task; got ${JSON.stringify(failed.error)}`);
    }
    if (!failed.error.code) {
      throw new Error(`The failed record must carry a machine-readable code; got ${JSON.stringify(failed.error)}`);
    }
    const failedDetail = await apiJSON(`${view.url}/api/spec/${FAILURE_SPEC}`);
    if (failedDetail.spec.status !== "TODO") {
      throw new Error(`A failed execution must leave ${FAILURE_SPEC} out of PLANNED; got ${failedDetail.spec.status}`);
    }
    if (String(failedDetail.plan_body || "").trim() || (failedDetail.tasks || []).length !== 0) {
      throw new Error("A failed execution must not leave a plan behind");
    }
    if (!failedDetail.execution || failedDetail.execution.id !== failing.id || failedDetail.execution.status !== "FAILED") {
      throw new Error(`The detail must report the failed execution; got ${JSON.stringify(failedDetail.execution)}`);
    }
    console.log(`-> AC-4 ok: execution ${failing.id} FAILED with the remote reason, ${FAILURE_SPEC} still TODO with no plan`);

    // AC-5 — a reload is a plain re-read of the spec detail: same execution,
    // same terminal outcome, no new record and no second remote task.
    const recordsBeforeReload = await listExecutionRecords(executionsDir);
    const remoteTasksBeforeReload = hub.createdCount();
    const reloaded = await apiJSON(`${view.url}/api/spec/${SUCCESS_SPEC}`);
    if (!reloaded.execution || reloaded.execution.id !== started.id || reloaded.execution.status !== "SUCCEEDED") {
      throw new Error(`After a reload the detail must still report ${started.id} as SUCCEEDED; got ${JSON.stringify(reloaded.execution)}`);
    }
    const reloadedFailure = await apiJSON(`${view.url}/api/spec/${FAILURE_SPEC}`);
    if (!reloadedFailure.execution || reloadedFailure.execution.id !== failing.id || reloadedFailure.execution.status !== "FAILED") {
      throw new Error(`After a reload the detail must still report ${failing.id} as FAILED; got ${JSON.stringify(reloadedFailure.execution)}`);
    }
    const recordsAfterReload = await listExecutionRecords(executionsDir);
    if (recordsAfterReload.join(",") !== recordsBeforeReload.join(",") || recordsAfterReload.length !== 2) {
      throw new Error(`A reload must not create a record: before ${JSON.stringify(recordsBeforeReload)}, after ${JSON.stringify(recordsAfterReload)}`);
    }
    if (hub.createdCount() !== remoteTasksBeforeReload || hub.createdCount() !== 2) {
      throw new Error(`A reload must not dispatch a second remote task; the hub saw ${hub.createdCount()} creations`);
    }
    console.log(`-> AC-5 ok: both executions found again after a reload, still 2 records and 2 remote tasks`);

    // The credential travelled to the hub and never to the browser.
    if (!hub.authorizedCalls()) {
      throw new Error("The provider never authenticated against the hub, so the credential path was not exercised");
    }
    if (hub.unauthorizedCalls()) {
      throw new Error(`The hub rejected ${hub.unauthorizedCalls()} call(s): the credential did not reach the provider`);
    }
    const leaked = viewerBodies.filter((body) => body.includes(TOKEN_SENTINEL));
    if (leaked.length) {
      throw new Error(`The viewer echoed the credential in ${leaked.length} response(s)`);
    }
    console.log(`-> secrets ok: token used on ${hub.authorizedCalls()} hub calls, absent from all ${viewerBodies.length} viewer responses`);

    console.log("\nPASS: plan-from-view smoke test completed.");
    console.log(`Sandbox: ${sandboxDir}`);
    console.log(`View URL: ${view.url}`);
  } finally {
    if (view) {
      await stopProcess(view.child);
    }
    if (hub) {
      await hub.close();
    }
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned workspace: ${runDir}`);
    }
  }
}

function postAction(action) {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action }),
  };
}

// --- fake ARcipelago hub ------------------------------------------------

// startFakeHub serves the two routes the arcipelago client uses. Task progress
// is gated by the test rather than by a timer: a created task stays `running`
// until completeWithPlan/declareFailed is called, which makes the non-terminal
// window of AC-2 deterministic without a single sleep.
async function startFakeHub({ sandboxDir, planFile }) {
  const tasks = new Map(); // task id -> record
  const bySpec = new Map(); // spec code -> task id
  let created = 0;
  let authorized = 0;
  let unauthorized = 0;

  const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://127.0.0.1");
    if (req.headers.authorization !== `Bearer ${TOKEN_SENTINEL}`) {
      unauthorized += 1;
      return sendJSON(res, 401, { error: "unauthorized" });
    }
    authorized += 1;

    // The viewer probes the default provider before it dispatches anything, and
    // the arcipelago probe is the whoami of the external namespace followed by
    // the list of destinations the credential may use. A hub that did not serve
    // them would be indistinguishable from one that is not there, and every
    // action would be refused before a task was ever created.
    if (req.method === "GET" && url.pathname === "/api/external/me") {
      return sendJSON(res, 200, {
        kind: "application",
        identity: { id: "app-smoke", name: "smoke", workspaceIds: [WORKSPACE_ID] },
      });
    }

    if (req.method === "GET" && url.pathname === "/api/external/workspaces") {
      return sendJSON(res, 200, {
        workspaces: [
          {
            id: WORKSPACE_ID,
            name: "smoke workspace",
            cwdHint: sandboxDir,
            requirements: [],
            archived: false,
            eligibleRunners: { known: 1, online: 1, missing: [] },
          },
        ],
      });
    }

    if (req.method === "POST" && url.pathname === "/api/external/tasks") {
      return readBody(req).then((raw) => {
        let request;
        try {
          request = JSON.parse(raw);
        } catch {
          return sendJSON(res, 400, { error: "invalid_json" });
        }
        if (request.workspaceId !== WORKSPACE_ID) {
          return sendJSON(res, 404, { error: "workspace_not_found" });
        }
        const specCode = (request.metadata || {}).spec_code;
        if (!specCode || !request.prompt || !request.title) {
          return sendJSON(res, 400, { error: "invalid_task" });
        }
        created += 1;
        const id = `task-${created}`;
        const record = { id, status: "running", resultSummary: "", specCode, request };
        tasks.set(id, record);
        bySpec.set(specCode, id);
        return sendJSON(res, 201, { task: publicTask(record) });
      });
    }

    if (req.method === "GET" && url.pathname === "/api/external/tasks/by-reference") {
      const externalID = url.searchParams.get("externalId");
      const match = [...tasks.values()].find((t) => t.request.externalId === externalID);
      if (!match) return sendJSON(res, 404, { error: "task_not_found" });
      return sendJSON(res, 200, { task: publicTask(match) });
    }

    if (req.method === "GET" && url.pathname.startsWith("/api/external/tasks/")) {
      const id = decodeURIComponent(url.pathname.slice("/api/external/tasks/".length));
      const record = tasks.get(id);
      if (!record) return sendJSON(res, 404, { error: "task_not_found" });
      return sendJSON(res, 200, { task: publicTask(record) });
    }

    return sendJSON(res, 404, { error: "not_found" });
  });

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  const url = `http://127.0.0.1:${address.port}`;

  async function waitForTask(specCode, timeoutMs = 20000) {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      const id = bySpec.get(specCode);
      if (id) return tasks.get(id);
      await delay(50);
    }
    throw new Error(`The provider never created a remote task for ${specCode}`);
  }

  // completeWithPlan is what keeps this smoke honest: before the task turns
  // `completed` the hub really persists a plan through the CLI, exactly as a
  // remote agent would, and only then emits the receipt the provider parses.
  async function completeWithPlan(specCode) {
    const record = await waitForTask(specCode);
    const result = await runCommand(`hub-spec-plan-${specCode}`, cliPath, ["spec", "plan", specCode, "--file", planFile], {
      cwd: sandboxDir,
      allowFailure: true,
    });
    if (result.code !== 0) {
      record.status = "failed";
      record.resultSummary = `the fake hub could not plan ${specCode}: ${result.stderr || result.stdout}`;
      throw new Error(`The fake hub failed to plan ${specCode}:\n${result.stderr}\n${result.stdout}`);
    }
    record.resultSummary = [
      `Planned ${specCode} through the ARchetipo planning skill.`,
      `{"spec_code":"${specCode}","status":"PLANNED","tasks":${PLAN_TASKS}}`,
    ].join("\n");
    record.status = "completed";
    return record;
  }

  async function declareFailed(specCode) {
    const record = await waitForTask(specCode);
    record.resultSummary = REMOTE_FAILURE_REASON;
    record.status = "failed";
    return record;
  }

  return {
    url,
    waitForTask,
    completeWithPlan,
    declareFailed,
    createdCount: () => created,
    authorizedCalls: () => authorized,
    unauthorizedCalls: () => unauthorized,
    close: () =>
      new Promise((resolve) => {
        server.closeAllConnections?.();
        server.close(() => resolve());
      }),
  };
}

function publicTask(record) {
  return { id: record.id, status: record.status, resultSummary: record.resultSummary };
}

function sendJSON(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8" });
  res.end(body);
}


// --- workspace fixtures --------------------------------------------------

async function writeSpecsPayload(file) {
  const payload = {
    specs: [
      {
        code: SUCCESS_SPEC,
        title: "Smoke spec pianificata dalla UI",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per il percorso di successo di `Pianifica`.",
      },
      {
        code: FAILURE_SPEC,
        title: "Smoke spec con pianificazione fallita",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per il percorso di fallimento di `Pianifica`.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

// The payload the fake hub feeds to the real `spec plan`: it is what makes the
// spec reach PLANNED with a readable plan, which is the oracle of AC-3.
async function writePlanPayload(runDir) {
  const file = path.join(runDir, "plan.json");
  const payload = {
    plan_body: "## Piano prodotto dall'esecuzione\n\nPersistito dall'agente remoto simulato durante lo smoke.",
    tasks: Array.from({ length: PLAN_TASKS }, (_, index) => ({
      id: `TASK-0${index + 1}`,
      title: `Smoke task ${index + 1}`,
      body: "## Objective\nNessuna.\n\n## Blockers\nNone.",
      type: "Impl",
      status: "TODO",
      dependencies: [],
    })),
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
  return file;
}

// --- oracles -------------------------------------------------------------

async function listExecutionRecords(dir) {
  let entries;
  try {
    entries = await fs.readdir(dir);
  } catch (error) {
    if (error.code === "ENOENT") return [];
    throw error;
  }
  return entries.filter((name) => name.endsWith(".json")).sort();
}

async function readExecutionRecord(dir, id) {
  return JSON.parse(await fs.readFile(path.join(dir, `${id}.json`), "utf8"));
}

async function waitForTerminal(viewURL, id, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await apiJSON(`${viewURL}/api/execution/${id}`);
    if (last.status === "SUCCEEDED" || last.status === "FAILED") return last;
    await delay(250);
  }
  throw new Error(`Execution ${id} did not reach a terminal status in time; last: ${JSON.stringify(last)}`);
}

// --- harness -------------------------------------------------------------

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for planning a spec from archetipo view against a fake local ARcipelago hub

Usage:
  node ./test/e2e/plan-from-view-smoke.mjs
  npm run test:view-plan-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated sandbox
  --cleanup               Remove the run directory after the test passes/fails
`);
}

async function createRunDir(root) {
  return createRunDirShared(root, false);
}

async function buildCLI() {
  return buildCLIShared(cliPath, repoRoot, runCommand);
}

async function startViewServer(cwd) {
  return startViewServerShared(cliPath, cwd, cliEnv, "/api/board");
}


async function apiJSON(url, init = {}, expected = null) {
  const response = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init.headers || {}),
    },
  });
  const text = await response.text();
  viewerBodies.push(text);
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${url}: ${typeof data === "string" ? data : JSON.stringify(data)}`);
  }
  if (expected !== null && response.status !== expected) {
    throw new Error(`Expected HTTP ${expected} for ${url}, got ${response.status}: ${text}`);
  }
  return data;
}

async function expectStatus(url, status, init = {}) {
  const response = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init.headers || {}),
    },
  });
  const text = await response.text();
  viewerBodies.push(text);
  if (response.status !== status) {
    throw new Error(`Expected HTTP ${status} for ${url}, got ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

async function runCommand(label, command, args, options = {}) {
  return makeRunCommand(cliEnv)(label, command, args, options);
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
