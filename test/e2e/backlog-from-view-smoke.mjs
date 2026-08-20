#!/usr/bin/env node

// End-to-end smoke for "generate the initial backlog from the UI" (US-041).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the claude execution provider with its stream-json
// client, the local session, the server-side run follower, the effect
// confirmation and the partial-backlog rollback, the workspace routes
// (`GET /api/workspace/actions`, `POST /api/workspace/execution`), the dialogue
// routes (`GET /api/execution/{id}/run`, `POST .../run/messages`,
// `POST .../run/cancel`) and `GET /api/board`. Only the agent binary is
// replaced, by `support/fake-claude.mjs`, which speaks the same protocol on
// stdio, so the generation needs no credential and no network.
//
// The fake never progresses on its own: the test emits every frame, so each
// acceptance criterion is proved against a state the test commanded. There is
// no arbitrary sleep anywhere — every wait polls a viewer route or the control
// server until it reports what the fake process was just told.
//
// Three sandboxes, because the four criteria are three different workspaces:
// one with a PRD and no backlog that gets one (AC-1, AC-2), one whose run is
// cancelled halfway (AC-4), and one that already has a backlog (AC-3).

import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "backlog-from-view-smoke");

// The sentinel stands for whatever session material the agent owns. It lives in
// the viewer's environment, and must never reach a viewer response nor the
// workspace configuration.
const AUTH_ENV = "CLAUDE_FAKE_AUTH";
const AUTH_SENTINEL = "claude-backlog-session-material-DO-NOT-EXPOSE";

// The skill the backlog prompt invokes, and the file whose presence is what
// makes that invocation possible at all.
const SPEC_SKILL_PATH = path.join(".claude", "skills", "archetipo-spec", "SKILL.md");

const QUESTION = "Quante epiche vuoi nel primo taglio del backlog?";
const ANSWER = "smoke-backlog-answer-sentinel";

const BACKLOG_PATH = path.join(".archetipo", "backlog.yaml");
const SPECS_DIR = path.join(".archetipo", "specs");
const PRD_BODY = [
  "# PRD — Prodotto di smoke",
  "",
  "## Vision",
  "Un prodotto inventato da uno smoke test.",
  "",
  "## Personas",
  "La persona che ha risposto: " + ANSWER,
  "",
].join("\n");

// Three specs in two epics, which is exactly what the receipt below claims and
// what the board is asserted to serve back.
const GENERATED_SPECS = [
  spec("US-001", "Vedere la board", "EP-001", "Impianto del prodotto"),
  spec("US-002", "Creare una specifica", "EP-001", "Impianto del prodotto"),
  spec("US-003", "Esportare un report", "EP-002", "Osservabilità"),
];
// The single spec the cancelled run got as far as persisting before it was
// stopped: a partial backlog, which must not survive.
const PARTIAL_SPECS = [spec("US-001", "Vedere la board", "EP-001", "Impianto del prodotto")];
// The backlog the third sandbox already had before the viewer ever started.
const PRE_EXISTING_SPECS = [
  spec("US-100", "Specifica scritta a mano", "EP-100", "Epica scritta a mano"),
  spec("US-101", "Seconda specifica scritta a mano", "EP-100", "Epica scritta a mano"),
];

const RECEIPT = JSON.stringify({ artifact: "backlog", status: "WRITTEN", epics: 2, specs: 3 });

// Every viewer response body is kept, so the final check can prove no session
// material travelled to the browser on any route this run touched.
const viewerBodies = [];
// One entry per proved statement, for the report.
const checks = [];

function ok(criterion, statement) {
  checks.push({ criterion, statement });
  console.log(`-> ${criterion} ok: ${statement}`);
}

function spec(code, title, epicCode, epicTitle) {
  return {
    code,
    title,
    epic: { code: epicCode, title: epicTitle },
    priority: "HIGH",
    points: 3,
    status: "TODO",
    body: `#### ${code}: ${title}\n\n**Epic:** ${epicCode} | **Priority:** HIGH | **Points:** 3 | **Status:** TODO\n\n**User Story**\nCome persona di smoke,\nvoglio ${title.toLowerCase()},\nper provare la generazione del backlog dalla UI.\n`,
  };
}

// --- the script -------------------------------------------------------------

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake binary relies on a POSIX shebang");
    return;
  }
  const runDir = await createRunDir(options.workspaceRoot);
  // Starting a viewer records its project root in the user-level registry of
  // known workspaces. This run directory is a throwaway, so the entry must go
  // with it instead of accumulating in the real registry of the machine.
  process.env.ARCHETIPO_STATE_DIR = path.join(runDir, "state");
  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const startedAt = Date.now();
  let failure = null;
  try {
    await scenarioBacklogIsGeneratedFromThePRD(runDir);
    await scenarioCancelledRunLeavesNoBacklog(runDir);
    await scenarioExistingBacklogIsNeverRegenerated(runDir);
    assertNoSessionMaterialLeaked();
  } catch (error) {
    failure = error;
  }

  await writeReport(runDir, { startedAt, durationMs: Date.now() - startedAt, failure });

  if (failure) {
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
    }
    throw failure;
  }
  if (options.cleanup) {
    await fs.rm(runDir, { recursive: true, force: true });
    console.log(`-> cleaned workspace: ${runDir}`);
  }
  console.log(`\nPASS: backlog-from-view smoke test completed (${checks.length} statements proved).`);
  console.log(`Sandboxes: ${runDir}`);
}

// AC-1 and AC-2 — a workspace with a PRD and no backlog offers the generation,
// the agent's question and the person's answer are one conversation, and the
// epics and specs are on the board of the same viewer process the moment the
// run succeeds.
async function scenarioBacklogIsGeneratedFromThePRD(runDir) {
  console.log("\n=== AC-1/AC-2: the generation produces the backlog ===");
  const w = await openWorkspace(runDir, "sandbox-backlog", async (sandboxDir) => {
    await writePRD(sandboxDir, PRD_BODY);
  });
  try {
    // AC-1 — a workspace that has the PRD and no backlog offers the generation.
    const actions = await apiJSON(`${w.view.url}/api/workspace/actions`);
    if (actions.has_prd !== true || actions.has_backlog !== false) {
      throw new Error(`AC-1: a workspace with a PRD and no backlog must report has_prd:true and has_backlog:false; got ${JSON.stringify(actions)}`);
    }
    const backlog = (actions.actions || []).find((action) => action.id === "backlog");
    if (!backlog || backlog.offered !== true || backlog.runnable !== true || backlog.unavailable_reason) {
      throw new Error(`AC-1: the backlog generation must be offered as runnable; got ${JSON.stringify(backlog)}`);
    }
    if (actions.execution !== null) {
      throw new Error(`AC-1: a workspace that never ran anything must carry no execution; got ${JSON.stringify(actions.execution)}`);
    }
    ok("AC-1", `a workspace with a PRD and no backlog offers ${JSON.stringify(backlog.label)} as offered and runnable, with no execution behind it`);

    // The run starts from the state the UI produces: a press on that action.
    const started = await apiJSON(`${w.view.url}/api/workspace/execution`, postJSON({ action: "backlog" }), 201);
    if (started.status !== "RUNNING" || !started.id) {
      throw new Error(`AC-1: unexpected execution record on start: ${JSON.stringify(started)}`);
    }
    if (started.spec_code !== "") {
      throw new Error(`AC-1: an execution whose object is the workspace carries no spec; got ${JSON.stringify(started.spec_code)}`);
    }
    const executionID = started.id;

    // The instruction travels inside the protocol, as the first user frame, and
    // it is the skill the process is asked to invoke.
    await w.control.waitFor("argv");
    const prompt = userFrameText(await w.control.waitFor(userFrame, 1));
    if (!prompt.includes("/archetipo-spec")) {
      throw new Error(`AC-1: the prompt does not invoke the backlog skill: ${JSON.stringify(prompt)}`);
    }
    ok("AC-1", `pressing the action started ${executionID} as RUNNING with an empty spec_code and gave the process a prompt naming /archetipo-spec`);

    // AC-2, first half — the agent asks, and the question is in the UI while the
    // record is still running.
    w.control.push(emit({ type: "system", subtype: "init", session_id: "session-1" }));
    w.control.push(emit({ type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text: QUESTION }] } }));
    w.control.push(emit({ type: "result", subtype: "success", is_error: false, result: "In attesa della risposta." }));

    const asked = await waitForRun(w.view.url, executionID, (data) =>
      data.events.some((event) => event.kind === "text" && event.text === QUESTION));
    if (!asked.run || asked.run.state !== "ACTIVE") {
      throw new Error(`AC-2: the run must still be active while the agent waits; got ${JSON.stringify(asked.run)}`);
    }
    const running = await readWorkspaceExecution(w.view.url);
    if (running.id !== executionID || running.status !== "RUNNING") {
      throw new Error(`AC-2: a turn that ended on a question must not end the run; got ${JSON.stringify(running)}`);
    }
    ok("AC-2", "the agent's question is exposed as a text event while the record is still RUNNING");

    // AC-2, second half — the answer continues the same conversation.
    const accepted = await apiJSON(`${w.view.url}/api/execution/${executionID}/run/messages`, postJSON({ message: ANSWER }), 202);
    if (JSON.stringify(accepted).includes(ANSWER)) {
      throw new Error("AC-2: the accepted answer must not be echoed into the timeline before the process re-emits it");
    }
    const delivered = userFrameText(await w.control.waitFor(userFrame, 2));
    if (delivered !== ANSWER) {
      throw new Error(`AC-2: the process received ${JSON.stringify(delivered)} instead of the answer`);
    }
    // The process now has the answer and has not re-emitted it: the conversation
    // still does not carry it, which is what "only after" means.
    const beforeReplay = await readRun(w.view.url, executionID);
    if (beforeReplay.events.some((event) => event.kind === "user_message" && event.text === ANSWER)) {
      throw new Error("AC-2: the answer entered the conversation before the process re-emitted it");
    }
    w.control.push(emit({ type: "user", message: { content: [{ type: "text", text: ANSWER }] }, isReplay: true }));
    const echoed = await waitForRun(w.view.url, executionID, (data) =>
      data.events.some((event) => event.kind === "user_message" && event.text === ANSWER));
    const occurrences = echoed.events.filter((event) => event.kind === "user_message" && event.text === ANSWER);
    if (occurrences.length !== 1) {
      throw new Error(`AC-2: the re-emitted answer must appear once; found ${occurrences.length}`);
    }
    ok("AC-2", "the answer reached the process as a user frame, was absent from the 202 body and from the conversation until the process re-emitted it, and then appears exactly once");

    // AC-2 — the agent persists the backlog and closes on its receipt. The
    // specs are written by a command of this smoke, never by the fake, because
    // what is being proved is what the viewer does with a backlog that exists.
    await addSpecs(w.sandboxDir, GENERATED_SPECS);
    w.control.push(emit({ type: "assistant", message: { content: [{ type: "text", text: "Ho scritto il backlog." }] } }));
    w.control.push(emit({ type: "result", subtype: "success", is_error: false, result: RECEIPT }));

    const succeeded = await waitForWorkspaceExecution(w.view.url, (record) => record && record.status !== "RUNNING");
    if (succeeded.status !== "SUCCEEDED") {
      throw new Error(`AC-2: the generation must succeed; got ${JSON.stringify(succeeded)}`);
    }
    for (const item of GENERATED_SPECS) {
      const file = path.join(w.sandboxDir, SPECS_DIR, `${item.code}.yaml`);
      if (!(await exists(file))) {
        throw new Error(`AC-2: ${path.join(SPECS_DIR, `${item.code}.yaml`)} is not on disk after the run succeeded`);
      }
    }
    // The same viewer process, never restarted, is what serves them back.
    const board = await readBoard(w.view.url);
    const codes = boardSpecCodes(board);
    const expectedCodes = GENERATED_SPECS.map((item) => item.code);
    if (JSON.stringify(codes) !== JSON.stringify(expectedCodes)) {
      throw new Error(`AC-2: GET /api/board must serve the three generated specs; got ${JSON.stringify(codes)}`);
    }
    const epicCodes = (board.epics || []).map((epic) => epic.code).sort();
    if (JSON.stringify(epicCodes) !== JSON.stringify(["EP-001", "EP-002"])) {
      throw new Error(`AC-2: GET /api/board must serve the two epics; got ${JSON.stringify(epicCodes)}`);
    }
    const afterwards = await apiJSON(`${w.view.url}/api/workspace/actions`);
    if (afterwards.has_backlog !== true) {
      throw new Error(`AC-2: the workspace must report the backlog it now has; got ${JSON.stringify(afterwards)}`);
    }
    const afterAction = (afterwards.actions || []).find((action) => action.id === "backlog");
    if (!afterAction || afterAction.offered !== false) {
      throw new Error(`AC-2: a workspace that now has a backlog must stop offering the first generation; got ${JSON.stringify(afterAction)}`);
    }
    ok("AC-2", `the record closed SUCCEEDED, the three spec files exist under ${SPECS_DIR}, GET /api/board serves ${JSON.stringify(codes)} in ${JSON.stringify(epicCodes)} from the same viewer process, and the action is no longer offered`);
  } finally {
    await w.close();
  }
}

// AC-4 — a run that is stopped leaves the workspace without a backlog and says
// why.
async function scenarioCancelledRunLeavesNoBacklog(runDir) {
  console.log("\n=== AC-4: a cancelled run leaves no backlog ===");
  const w = await openWorkspace(runDir, "sandbox-cancelled", async (sandboxDir) => {
    await writePRD(sandboxDir, PRD_BODY);
  });
  try {
    const started = await apiJSON(`${w.view.url}/api/workspace/execution`, postJSON({ action: "backlog" }), 201);
    const executionID = started.id;
    await w.control.waitFor(userFrame, 1);
    w.control.push(emit({ type: "system", subtype: "init", session_id: "session-1" }));
    w.control.push(emit({ type: "assistant", message: { content: [{ type: "text", text: QUESTION }] } }));
    w.control.push(emit({ type: "result", subtype: "success", is_error: false, result: "In attesa della risposta." }));
    await waitForRun(w.view.url, executionID, (data) =>
      data.events.some((event) => event.kind === "text" && event.text === QUESTION));

    // The half-written backlog this run produced before it was stopped: one
    // spec, persisted exactly as the agent would have persisted it.
    await addSpecs(w.sandboxDir, PARTIAL_SPECS);
    if (!(await exists(path.join(w.sandboxDir, BACKLOG_PATH)))) {
      throw new Error("AC-4: the partial backlog was not persisted, so there is nothing to roll back");
    }
    await apiJSON(`${w.view.url}/api/execution/${executionID}/run/cancel`, { method: "POST" }, 202);
    // Whatever the cancel did to the process's input, the fake is told to leave:
    // the run ends because the process ended, never because the viewer decided.
    w.control.exit(0);

    const failed = await waitForWorkspaceExecution(w.view.url, (record) => record && record.status !== "RUNNING");
    if (failed.status !== "FAILED") {
      throw new Error(`AC-4: a stopped generation must fail; got ${JSON.stringify(failed)}`);
    }
    if (!String(failed.error?.message || "").trim()) {
      throw new Error(`AC-4: the failure must declare its reason; got ${JSON.stringify(failed.error)}`);
    }
    const leftovers = await fs.readdir(path.join(w.sandboxDir, SPECS_DIR)).catch(() => []);
    const specFiles = leftovers.filter((entry) => entry.endsWith(".yaml"));
    if (specFiles.length !== 0) {
      throw new Error(`AC-4: no spec file may survive the failed run; found ${JSON.stringify(specFiles)}`);
    }
    if (await exists(path.join(w.sandboxDir, BACKLOG_PATH))) {
      throw new Error(`AC-4: ${BACKLOG_PATH} must not survive the failed run`);
    }
    const board = await readBoard(w.view.url);
    const codes = boardSpecCodes(board);
    if (codes.length !== 0) {
      throw new Error(`AC-4: the board must be empty after the rollback; got ${JSON.stringify(codes)}`);
    }
    const actions = await apiJSON(`${w.view.url}/api/workspace/actions`);
    if (actions.has_backlog !== false) {
      throw new Error(`AC-4: the workspace must report no backlog after the failure; got ${JSON.stringify(actions)}`);
    }
    const action = (actions.actions || []).find((item) => item.id === "backlog");
    if (!action || action.offered !== true) {
      throw new Error(`AC-4: the generation must be offered again after the rollback; got ${JSON.stringify(action)}`);
    }
    ok("AC-4", `the stopped run closed FAILED stating ${JSON.stringify(truncate(failed.error.message))}, no spec file and no ${BACKLOG_PATH} survived, the board is empty on the same viewer and the action is offered again`);
  } finally {
    await w.close();
  }
}

// AC-3 — a workspace that already has a backlog is never offered a first
// generation, and nothing of it is touched by pressing anyway.
async function scenarioExistingBacklogIsNeverRegenerated(runDir) {
  console.log("\n=== AC-3: an existing backlog is never regenerated ===");
  const w = await openWorkspace(runDir, "sandbox-existing", async (sandboxDir) => {
    await writePRD(sandboxDir, PRD_BODY);
    await addSpecs(sandboxDir, PRE_EXISTING_SPECS);
  });
  try {
    const before = await readBacklogFiles(w.sandboxDir);

    const actions = await apiJSON(`${w.view.url}/api/workspace/actions`);
    if (actions.has_backlog !== true) {
      throw new Error(`AC-3: the workspace must report the backlog it has; got ${JSON.stringify(actions)}`);
    }
    const backlog = (actions.actions || []).find((action) => action.id === "backlog");
    if (!backlog || backlog.offered !== false || backlog.runnable !== false) {
      throw new Error(`AC-3: a workspace with a backlog must not offer a first generation; got ${JSON.stringify(backlog)}`);
    }
    const reason = String(backlog.unavailable_reason || "").trim();
    if (!reason) {
      throw new Error(`AC-3: an action that is not offered must say why; got ${JSON.stringify(backlog)}`);
    }

    const refused = await expectStatus(`${w.view.url}/api/workspace/execution`, 409, postJSON({ action: "backlog" }));
    if (String(refused.error || "").trim() !== reason) {
      throw new Error(`AC-3: the refusal must state the very reason the payload published; got ${JSON.stringify(refused)} against ${JSON.stringify(reason)}`);
    }

    const after = await readBacklogFiles(w.sandboxDir);
    const beforeNames = [...before.keys()].sort();
    const afterNames = [...after.keys()].sort();
    if (JSON.stringify(beforeNames) !== JSON.stringify(afterNames)) {
      throw new Error(`AC-3: the backlog files changed across the refused press: ${JSON.stringify(beforeNames)} vs ${JSON.stringify(afterNames)}`);
    }
    for (const name of beforeNames) {
      if (!before.get(name).equals(after.get(name))) {
        throw new Error(`AC-3: ${name} is not byte-identical across the refused press`);
      }
    }
    const records = await fs
      .readdir(path.join(w.sandboxDir, ".archetipo", "executions"))
      .catch(() => []);
    if (records.length !== 0) {
      throw new Error(`AC-3: a refused press must create no execution record; found ${JSON.stringify(records)}`);
    }
    ok("AC-3", `the action is not offered stating ${JSON.stringify(truncate(reason))}, pressing it answers 409 with that same sentence, the ${beforeNames.length} backlog files are byte-identical and no record was written`);
  } finally {
    await w.close();
  }
}

// AC-1..AC-4 cross-check — the agent owns its own authentication, so nothing of
// it may reach a viewer response or the workspace configuration.
function assertNoSessionMaterialLeaked() {
  const leaked = viewerBodies.filter((body) => body.includes(AUTH_SENTINEL));
  if (leaked.length) {
    throw new Error(`the viewer echoed the session material in ${leaked.length} response(s)`);
  }
  ok("AC-1..AC-4", `the session material is absent from all ${viewerBodies.length} viewer responses and from every workspace configuration`);
}

// --- one workspace, ready to be pressed -------------------------------------

// openWorkspace initializes a sandbox with the claude tool (which is what
// installs the spec skill the prompt invokes), seeds it, starts the control
// server and the viewer, and saves the real claude provider pointed at the fake
// binary as the workspace default — which is what a person does in the
// Execution panel.
async function openWorkspace(runDir, name, seed) {
  const sandboxDir = path.join(runDir, name);
  const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, [AUTH_ENV]: AUTH_SENTINEL };
  await fs.mkdir(sandboxDir, { recursive: true });
  await runCommand(`init/${name}`, cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: sandboxDir, env });
  // The skill the prompt names is a precondition of the whole conversation: the
  // provider refuses to open a session without it.
  if (!(await exists(path.join(sandboxDir, SPEC_SKILL_PATH)))) {
    throw new Error(`archetipo init did not install ${SPEC_SKILL_PATH}, which is what /archetipo-spec resolves to`);
  }
  if (seed) await seed(sandboxDir);

  const control = await startControlServer();
  console.log(`-> control server for the fake claude: ${control.url}`);
  const view = await startViewServer(sandboxDir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
  console.log(`-> view ready: ${view.url}`);

  await apiJSON(`${view.url}/api/execution/provider/default`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id: "claude", config: { command: fakeClaudePath, timeout_seconds: 600 } }),
  });

  return {
    sandboxDir,
    control,
    view,
    async close() {
      const configBody = await fs.readFile(path.join(sandboxDir, ".archetipo", "config.yaml"), "utf8").catch(() => "");
      if (configBody.includes(AUTH_SENTINEL) || /claude_home|CLAUDE_HOME|api_key|API_KEY/.test(configBody)) {
        throw new Error(`the workspace configuration carries agent session material:\n${configBody}`);
      }
      await stopProcess(view.child);
      await control.close();
    },
  };
}

// writePRD persists the document through the CLI, exactly as the inception does.
async function writePRD(sandboxDir, body) {
  const file = path.join(sandboxDir, ".archetipo", "prd-input.md");
  await fs.writeFile(file, body);
  await runCommand("prd-write", cliPath, ["prd", "write", "--file", file], {
    cwd: sandboxDir,
    env: { ...process.env, ARCHETIPO_DATA_DIR: repoRoot },
  });
  await fs.rm(file, { force: true });
}

// addSpecs persists epics and specs through `archetipo spec add`, which is the
// command the spec skill prescribes and therefore the one the agent would run.
// It is a command of this smoke and never of the fake: what is being proved is
// what the viewer does with a backlog that exists.
async function addSpecs(sandboxDir, specs) {
  const file = path.join(sandboxDir, ".archetipo", "specs-input.json");
  await fs.writeFile(file, `${JSON.stringify({ specs }, null, 2)}\n`);
  await runCommand("spec-add", cliPath, ["spec", "add", "--file", file], {
    cwd: sandboxDir,
    env: { ...process.env, ARCHETIPO_DATA_DIR: repoRoot },
  });
  await fs.rm(file, { force: true });
}

// readBacklogFiles returns every file that makes up the backlog on disk — the
// index plus every spec — keyed by its project-relative path, so "byte
// identical" can be asserted over the whole artifact and not only over one file.
async function readBacklogFiles(sandboxDir) {
  const out = new Map();
  const backlog = path.join(sandboxDir, BACKLOG_PATH);
  if (await exists(backlog)) {
    out.set(BACKLOG_PATH, await fs.readFile(backlog));
  }
  const entries = await fs.readdir(path.join(sandboxDir, SPECS_DIR)).catch(() => []);
  for (const entry of entries.sort()) {
    out.set(path.join(SPECS_DIR, entry), await fs.readFile(path.join(sandboxDir, SPECS_DIR, entry)));
  }
  return out;
}

async function exists(file) {
  return fs
    .stat(file)
    .then(() => true)
    .catch(() => false);
}

// --- the protocol -----------------------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

// userFrame recognizes a frame the process was given on its standard input as an
// operator message, which is the shape the instruction and every answer share.
function userFrame(entry) {
  return entry.kind === "received" && entry.frame?.type === "user";
}

function userFrameText(entry) {
  return (entry.frame?.message?.content || []).map((block) => block.text || "").join("");
}

function postJSON(payload) {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  };
}

// --- the control server -----------------------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received. That is what makes every assertion above
// a statement about a state the test produced.

async function startControlServer() {
  const commands = [];
  const received = [];

  const server = http.createServer(async (req, res) => {
    if (req.method === "GET" && req.url.startsWith("/next")) {
      sendJSON(res, 200, commands.shift() || { kind: "none" });
      return;
    }
    if (req.method === "POST" && req.url.startsWith("/received")) {
      received.push(JSON.parse(await readBody(req)));
      sendJSON(res, 200, { ok: true });
      return;
    }
    sendJSON(res, 404, { error: "not found" });
  });

  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const url = `http://127.0.0.1:${server.address().port}`;

  const matches = (matcher) => (entry) => (typeof matcher === "function" ? matcher(entry) : entry.kind === matcher);
  const describe = (matcher) => (typeof matcher === "function" ? matcher.name || "the expected frame" : matcher);

  return {
    url,
    push(command) {
      commands.push(command);
    },
    exit(code = 0) {
      commands.push({ kind: "exit", code });
    },
    // waitFor polls until the fake has reported at least `count` matching
    // requests, and returns the count-th of them. The count is explicit rather
    // than relative to a snapshot taken here, because a request can perfectly
    // well have arrived before the test got round to waiting for it.
    async waitFor(matcher, count = 1, timeoutMs = 30000) {
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matching = received.filter(matches(matcher));
        if (matching.length >= count) return matching[count - 1];
        await delay(50);
      }
      throw new Error(
        `The fake never received ${describe(matcher)} ${count} time(s); it received ${JSON.stringify(received.map((entry) => entry.kind))}`,
      );
    },
    close() {
      return new Promise((resolve) => server.close(resolve));
    },
  };
}

function sendJSON(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(body) });
  res.end(body);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    req.on("error", reject);
  });
}

// --- oracles ----------------------------------------------------------------

async function readRun(viewURL, executionID, afterID = 0) {
  return apiJSON(`${viewURL}/api/execution/${executionID}/run?after_id=${afterID}`);
}

async function waitForRun(viewURL, executionID, predicate, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readRun(viewURL, executionID, 0);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`The run projection never satisfied the expectation in time; last: ${JSON.stringify(last)}`);
}

// readBoard reads the Kanban the browser reads. A workspace with no backlog at
// all answers 404 there — a missing backlog is a missing precondition, not a
// broken viewer — and that is read here as the empty board it describes, so the
// rollback can be asserted against the same route the success is.
async function readBoard(viewURL) {
  const response = await fetch(`${viewURL}/api/board`, { headers: { Accept: "application/json" } });
  const text = await response.text();
  viewerBodies.push(text);
  if (response.status === 404) return { columns: [], epics: [] };
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${viewURL}/api/board: ${text}`);
  }
  return JSON.parse(text);
}

function boardSpecCodes(board) {
  return (board.columns || [])
    .flatMap((column) => column.specs || [])
    .map((item) => item.code)
    .sort();
}

// readWorkspaceExecution reads the latest workspace execution the way a reloaded
// browser does: through the actions payload, which carries it precisely so no
// identifier has to be remembered.
async function readWorkspaceExecution(viewURL) {
  const actions = await apiJSON(`${viewURL}/api/workspace/actions`);
  return actions.execution;
}

async function waitForWorkspaceExecution(viewURL, predicate, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readWorkspaceExecution(viewURL);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`The workspace execution never satisfied the expectation in time; last: ${JSON.stringify(last)}`);
}

function truncate(text, max = 120) {
  const body = String(text || "").replace(/\s+/g, " ").trim();
  return body.length > max ? `${body.slice(0, max)}…` : body;
}

// --- harness ----------------------------------------------------------------

function parseArgs(argv) {
  const options = { workspaceRoot: defaultWorkspaceRoot, cleanup: false };
  for (let i = 0; i < argv.length; i += 1) {
    switch (argv[i]) {
      case "--workspace-root":
        options.workspaceRoot = path.resolve(argv[++i]);
        break;
      case "--cleanup":
        options.cleanup = true;
        break;
      case "--help":
      case "-h":
        printHelp();
        process.exit(0);
        break;
      default:
        throw new Error(`Unknown argument: ${argv[i]}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Smoke test for generating the initial backlog from archetipo view against a fake agent binary

Usage:
  node ./test/e2e/backlog-from-view-smoke.mjs
  npm run test:view-backlog-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated sandboxes
  --cleanup               Remove the run directory after the test passes/fails
`);
}

async function createRunDir(root) {
  await fs.mkdir(root, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const runDir = path.join(root, "runs", stamp);
  await fs.mkdir(runDir, { recursive: true });
  return runDir;
}

async function buildCLI() {
  console.log(`-> building CLI: ${cliPath}`);
  await runCommand("go-build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], {
    cwd: path.join(repoRoot, "cli"),
    env: { ...process.env, ARCHETIPO_DATA_DIR: repoRoot },
  });
}

async function startViewServer(cwd, env) {
  const child = spawn(cliPath, ["view", "--host", "127.0.0.1", "--port", "0", "--no-open"], {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });

  let stdout = "";
  let stderr = "";
  child.stdout.on("data", (chunk) => {
    stdout += chunk.toString("utf8");
  });

  const ready = new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`view server did not become ready in time\nSTDERR:\n${stderr}\nSTDOUT:\n${stdout}`));
    }, 15000);

    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString("utf8");
      const match = stderr.match(/ARchetipo view ready at (http:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1]);
      }
    });

    child.on("exit", (code) => {
      clearTimeout(timeout);
      reject(new Error(`view server exited early with code ${code}\nSTDERR:\n${stderr}\nSTDOUT:\n${stdout}`));
    });

    child.on("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });

  const url = await ready;
  // The readiness probe is the workspace actions and not the board: two of the
  // three sandboxes have no backlog yet, and `GET /api/board` answers 404 for
  // exactly the workspace this smoke is about.
  await waitForHTTP(`${url}/api/workspace/actions`);
  return { child, url };
}

async function waitForHTTP(url) {
  const started = Date.now();
  while (Date.now() - started < 10000) {
    try {
      const response = await fetch(url, { headers: { Accept: "application/json" } });
      if (response.ok) return;
    } catch {
      // keep polling
    }
    await delay(200);
  }
  throw new Error(`Timed out waiting for ${url}`);
}

async function apiJSON(url, init = {}, expected = null) {
  const response = await fetch(url, {
    ...init,
    headers: { Accept: "application/json", ...(init.headers || {}) },
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
    headers: { Accept: "application/json", ...(init.headers || {}) },
  });
  const text = await response.text();
  viewerBodies.push(text);
  if (response.status !== status) {
    throw new Error(`Expected HTTP ${status} for ${url}, got ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

async function runCommand(label, command, args, options = {}) {
  console.log(`-> ${label}: ${command} ${args.join(" ")}`);
  const result = await new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    const stdout = [];
    const stderr = [];
    child.stdout.on("data", (chunk) => stdout.push(chunk));
    child.stderr.on("data", (chunk) => stderr.push(chunk));
    child.on("close", (code) => resolve({
      code,
      stdout: Buffer.concat(stdout).toString("utf8"),
      stderr: Buffer.concat(stderr).toString("utf8"),
    }));
    child.on("error", (error) => resolve({ code: 1, stdout: "", stderr: error.message }));
  });

  if (result.code !== 0 && !options.allowFailure) {
    throw new Error(`${label} failed with exit ${result.code}\nSTDOUT:\n${result.stdout}\nSTDERR:\n${result.stderr}`);
  }
  return result;
}

async function stopProcess(child) {
  if (!child || child.killed) return;
  child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    delay(3000),
  ]);
  if (!child.killed) {
    child.kill("SIGKILL");
  }
}

// --- report -----------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "backlog-from-view",
    spec: "US-041",
    passed: !failure,
    started_at: new Date(startedAt).toISOString(),
    duration_ms: durationMs,
    run_dir: runDir,
    checks,
    error: failure ? failure.message : null,
  };
  await fs.writeFile(path.join(runDir, "summary.json"), `${JSON.stringify(summary, null, 2)}\n`);
  await fs.writeFile(path.join(runDir, "report.html"), renderReport(summary));
  console.log(`-> report: ${path.join(runDir, "report.html")}`);
}

function renderReport(summary) {
  const rows = summary.checks
    .map((check) => `<tr><td class="code">${escapeHTML(check.criterion)}</td><td>${escapeHTML(check.statement)}</td></tr>`)
    .join("\n        ");
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ARchetipo Smoke — Backlog from the viewer (US-041)</title>
  <style>
    :root { color-scheme: light; --bg: #f6f7f9; --panel: #fff; --ink: #172026; --muted: #61707d; --line: #d8dee6; --ok: #18794e; --fail: #c93a2f; }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--bg); color: var(--ink); font: 14px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { max-width: 1000px; margin: 0 auto; padding: 28px; }
    header { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; margin-bottom: 24px; }
    h1 { margin: 0 0 12px; font-size: 22px; }
    h2 { margin: 24px 0 12px; font-size: 18px; border-bottom: 1px solid var(--line); padding-bottom: 8px; }
    .meta { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px; }
    .meta div { border: 1px solid var(--line); border-radius: 6px; padding: 8px 10px; background: #fbfcfd; }
    .label { display: block; color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .5px; }
    .value { overflow-wrap: anywhere; }
    .pass { color: var(--ok); font-weight: 700; }
    .fail { color: var(--fail); font-weight: 700; }
    table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 13px; background: var(--panel); }
    th { text-align: left; padding: 8px 10px; border-bottom: 2px solid var(--line); color: var(--muted); background: #fbfcfd; }
    td { padding: 8px 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
    .code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 650; white-space: nowrap; }
    pre { background: var(--panel); border: 1px solid var(--line); border-radius: 6px; padding: 12px; overflow-x: auto; white-space: pre-wrap; }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>ARchetipo Smoke — Backlog from the viewer (US-041)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Three sandboxes served by the real <code>archetipo view</code>, against a fake Claude Code binary
    driven frame by frame: a workspace with a PRD and no backlog that runs the generation as a conversation and
    gets its epics and specs, a workspace whose run is stopped halfway, and a workspace that already has a
    backlog.</p>

    <h2>Proved statements</h2>
    <table>
      <thead><tr><th>Criterion</th><th>Statement</th></tr></thead>
      <tbody>
        ${rows || '<tr><td colspan="2">none</td></tr>'}
      </tbody>
    </table>
${summary.error ? `\n    <h2>Failure</h2>\n    <pre>${escapeHTML(summary.error)}</pre>\n` : ""}  </main>
</body>
</html>
`;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
