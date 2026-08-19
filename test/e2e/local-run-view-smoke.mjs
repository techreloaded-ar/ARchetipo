#!/usr/bin/env node

// End-to-end smoke for "talk to a local Codex run" (US-038).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the codex execution provider, its app-server protocol
// client, the local session, the server-side run follower and the four viewer
// routes (`GET /api/execution/{id}/run`, `POST .../run/messages`,
// `POST .../run/cancel`, and the provider list). Only the Codex binary is
// replaced, by a Node script that speaks the same JSON-RPC protocol on stdio,
// so the run needs no credential and no network.
//
// The fake never progresses on its own: the test emits every notification,
// sends the message, cancels while the process resists, and ends the turn by
// hand, so each acceptance criterion is proved against a state the test
// commanded. There is no arbitrary sleep anywhere: the only waits poll a viewer
// route until the projection reports what the fake process was just told.

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
const fakeCodexPath = path.join(__dirname, "support", "fake-codex.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "local-run-view-smoke");

const SPEC = "US-901";
// The sentinel stands for whatever authentication material lives in the
// viewer's environment. Codex owns its own authentication, so nothing of it may
// ever reach a viewer response or the workspace configuration.
const AUTH_SENTINEL = "codex-session-material-DO-NOT-EXPOSE";
const MESSAGE_SENTINEL = "smoke-operator-message-sentinel";

const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, CODEX_FAKE_AUTH: AUTH_SENTINEL };

// Every viewer response body is kept so the final check can prove no session
// material travelled to the browser on any route this run touched.
const viewerBodies = [];

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake Codex binary relies on a POSIX shebang");
    return;
  }
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const specsFile = path.join(runDir, "specs.json");

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  let control;
  try {
    await buildCLI();
    await runCommand("init", cliPath, ["init", "--tool", "codex", "--connector", "file", "--yes"], { cwd: sandboxDir });
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir });

    control = await startControlServer();
    console.log(`-> control server for the fake Codex: ${control.url}`);

    view = await startViewServer(sandboxDir, control.url);
    console.log(`-> view ready: ${view.url}`);

    // AC-1 — the provider list declares the dialogue next to the capabilities
    // codex already exposes, and a provider that does not hold a conversation
    // shows it as absent.
    const providers = await apiJSON(`${view.url}/api/execution/providers`);
    const codex = (providers.providers || []).find((provider) => provider.id === "codex");
    if (!codex) {
      throw new Error(`AC-1: the codex provider is not listed: ${JSON.stringify(providers.providers)}`);
    }
    const capabilities = codex.capabilities || [];
    if (!capabilities.includes("run.dialog") || !capabilities.includes("spec.plan")) {
      throw new Error(`AC-1: codex must declare run.dialog beside spec.plan; got ${JSON.stringify(capabilities)}`);
    }
    const fields = (codex.config_fields || []).map((field) => field.name).sort();
    if (fields.join(",") !== "command,model,sandbox,timeout_seconds") {
      throw new Error(`AC-1: unexpected codex configuration fields: [${fields.join(", ")}]`);
    }
    console.log(`-> AC-1 ok: codex declares [${capabilities.join(", ")}]`);

    // The workspace default is the real codex provider, pointed at the fake
    // binary. Saving it through the API is what a person does in the Execution
    // panel, so the run starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: "codex", config: { command: fakeCodexPath, timeout_seconds: 600 } }),
    });

    const started = await apiJSON(`${view.url}/api/spec/${SPEC}/execution`, postJSON({ action: "plan" }), 201);
    if (started.status !== "RUNNING" || !started.id) {
      throw new Error(`Unexpected execution record on start: ${JSON.stringify(started)}`);
    }
    const executionID = started.id;
    const turn = await control.waitFor("turn/start");
    if (!String(turn.params?.input?.[0]?.text || "").includes(SPEC)) {
      throw new Error(`The prompt does not ask to plan ${SPEC}: ${JSON.stringify(turn.params)}`);
    }
    const thread = control.first("thread/start");
    if (thread.params?.approvalPolicy !== "never" || thread.params?.sandbox !== "workspace-write") {
      throw new Error(`The session was not opened as configured: ${JSON.stringify(thread.params)}`);
    }
    console.log(`-> execution ${executionID} opened a local session on the fake Codex`);

    // AC-2 — the history grows in order and the cursor returns only what is new.
    control.emit("item/agentMessage/delta", { delta: "Analizzo la spec" });
    control.emit("item/started", { item: { type: "commandExecution", command: ["ls"], status: "inProgress" } });
    control.emit("item/completed", { item: { type: "commandExecution", command: ["ls"], status: "completed", exitCode: 0 } });
    control.emit("item/agentMessage/delta", { delta: " e leggo il backlog" });
    const four = await waitForRun(view.url, executionID, (data) => data.events.length === 4);
    assertEventIDs(four.events, [1, 2, 3, 4], "AC-2");
    if (four.run?.state !== "ACTIVE") {
      throw new Error(`AC-2: the run must be active while the agent works; got ${JSON.stringify(four.run)}`);
    }
    const kinds = four.events.map((event) => event.kind).join(",");
    if (kinds !== "text,tool_start,tool_end,text") {
      throw new Error(`AC-2: the notifications were not translated; got [${kinds}]`);
    }
    const tail = await readRun(view.url, executionID, 4);
    if (tail.events.length !== 0 || tail.last_id !== 4) {
      throw new Error(`AC-2: after_id=4 must be empty at last_id 4; got ${JSON.stringify(tail)}`);
    }
    const resumed = await readRun(view.url, executionID, 2);
    assertEventIDs(resumed.events, [3, 4], "AC-2 resumed");
    console.log("-> AC-2 ok: events 1..4 in order, after_id returns exactly what is new");

    // AC-3 — the message reaches the process and becomes history only when the
    // process re-emits it.
    const accepted = await apiJSON(
      `${view.url}/api/execution/${executionID}/run/messages`,
      postJSON({ message: MESSAGE_SENTINEL }),
      202,
    );
    if (JSON.stringify(accepted).includes(MESSAGE_SENTINEL)) {
      throw new Error("AC-3: the accepted message must not be echoed into the timeline before the process re-emits it");
    }
    const steered = await control.waitFor("turn/steer");
    if (steered.text !== MESSAGE_SENTINEL) {
      throw new Error(`AC-3: the process received ${JSON.stringify(steered.text)} instead of the sentinel`);
    }
    if (steered.params?.expectedTurnId !== "turn-1") {
      throw new Error(`AC-3: the steer must name the turn in progress; got ${JSON.stringify(steered.params)}`);
    }
    const stillFour = await readRun(view.url, executionID, 0);
    assertEventIDs(stillFour.events, [1, 2, 3, 4], "AC-3 before the re-emission");
    control.emit("item/started", { item: { type: "userMessage", content: [{ type: "text", text: MESSAGE_SENTINEL }] } });
    const five = await waitForRun(view.url, executionID, (data) => data.events.length === 5);
    const echoed = five.events.filter((event) => event.kind === "user_message" && event.text === MESSAGE_SENTINEL);
    if (echoed.length !== 1) {
      throw new Error(`AC-3: the re-emitted message must appear once; found ${echoed.length}`);
    }
    console.log("-> AC-3 ok: the message reached the process, was absent from the 202 body and appears once after the re-emission");

    // AC-4 — cancelling reports the state the process confirms. The fake takes
    // the interruption and keeps going, exactly as a runner that has not acted
    // on it yet.
    const cancelling = await apiJSON(`${view.url}/api/execution/${executionID}/run/cancel`, { method: "POST" }, 202);
    await control.waitFor("turn/interrupt");
    if (!cancelling.run || cancelling.run.state !== "ACTIVE") {
      throw new Error(`AC-4: while the process is still alive the viewer must not close the run; got ${JSON.stringify(cancelling.run)}`);
    }
    const stillActive = await readRun(view.url, executionID, 0);
    if (stillActive.run.state !== "ACTIVE") {
      throw new Error(`AC-4: a cancelled run must stay as the process reports it; got ${JSON.stringify(stillActive.run)}`);
    }
    // Now the process really ends the turn, and only now may the state change.
    control.emit("item/completed", { item: { type: "agentMessage", text: "interrotto prima di pianificare" } });
    control.emit("turn/completed", { turn: { id: "turn-1" } });
    const closed = await waitForRun(view.url, executionID, (data) => data.run && data.run.state !== "ACTIVE");
    if (!closed.run.closed_at) {
      throw new Error(`AC-4: a run that ended must carry the instant it was observed; got ${JSON.stringify(closed.run)}`);
    }
    console.log(`-> AC-4 ok: the cancel reported ACTIVE, the state became ${closed.run.state} only when the process ended the turn`);

    // AC-5 — on a run that is over, message and cancellation are refused with
    // the reason, and the history does not change by a single byte.
    const baseline = await readRun(view.url, executionID, 0);
    for (const [label, url, init] of [
      ["messages", `${view.url}/api/execution/${executionID}/run/messages`, postJSON({ message: "sei ancora lì?" })],
      ["cancel", `${view.url}/api/execution/${executionID}/run/cancel`, { method: "POST" }],
    ]) {
      const refusal = await expectStatus(url, 409, init);
      if (!String(refusal.error || "").includes("run_not_active")) {
        throw new Error(`AC-5: the ${label} refusal must name run_not_active; got ${JSON.stringify(refusal)}`);
      }
    }
    const after = await readRun(view.url, executionID, 0);
    if (JSON.stringify(baseline) !== JSON.stringify(after)) {
      throw new Error(`AC-5: the projection changed across two refused commands\nbefore: ${JSON.stringify(baseline)}\nafter:  ${JSON.stringify(after)}`);
    }
    console.log("-> AC-5 ok: two refused commands named run_not_active and left the projection byte-for-byte identical");

    // AC-6 — nothing of the agent's own authentication material reaches the
    // browser or the workspace configuration.
    const leaked = viewerBodies.filter((body) => body.includes(AUTH_SENTINEL));
    if (leaked.length) {
      throw new Error(`AC-6: the viewer echoed the session material in ${leaked.length} response(s)`);
    }
    const configBody = await fs.readFile(path.join(sandboxDir, ".archetipo", "config.yaml"), "utf8");
    if (configBody.includes(AUTH_SENTINEL) || /codex_home|CODEX_HOME/.test(configBody)) {
      throw new Error(`AC-6: the workspace configuration carries agent session material:\n${configBody}`);
    }
    console.log(`-> AC-6 ok: the sentinel is absent from all ${viewerBodies.length} viewer responses and from the configuration`);

    console.log("\nPASS: local-run-view smoke test completed.");
    console.log(`Sandbox: ${sandboxDir}`);
  } finally {
    if (view) {
      await stopProcess(view.child);
    }
    if (control) {
      await control.close();
    }
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned workspace: ${runDir}`);
    }
  }
}

function postJSON(payload) {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  };
}

// --- the control server ----------------------------------------------------
//
// The fake Codex binary emits nothing on its own: it asks this server what to
// do next and reports every request it received. That is what makes every
// assertion below a statement about a state the test produced.

async function startControlServer() {
  const commands = [];
  const received = [];

  const server = http.createServer(async (req, res) => {
    if (req.method === "GET" && req.url.startsWith("/next")) {
      const command = commands.shift() || { kind: "none" };
      sendJSON(res, 200, command);
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

  return {
    url,
    emit(method, params) {
      commands.push({ kind: "emit", method, params });
    },
    exit(code = 0) {
      commands.push({ kind: "exit", code });
    },
    first(kind) {
      return received.find((entry) => entry.kind === kind) || {};
    },
    // waitFor polls until the fake has reported at least `count` requests of
    // that kind. The count is explicit rather than relative to a snapshot taken
    // here, because a request can perfectly well have arrived before the test
    // got round to waiting for it.
    async waitFor(kind, count = 1, timeoutMs = 30000) {
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matching = received.filter((entry) => entry.kind === kind);
        if (matching.length >= count) {
          return matching[matching.length - 1];
        }
        await delay(50);
      }
      throw new Error(`The fake Codex never received ${kind}; it received ${JSON.stringify(received.map((entry) => entry.kind))}`);
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

// --- fixtures and oracles --------------------------------------------------

async function writeSpecsPayload(file) {
  const payload = {
    specs: [
      {
        code: SPEC,
        title: "Smoke spec seguita in locale",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per il dialogo con una run locale di Codex.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

function assertEventIDs(events, expected, label) {
  const ids = events.map((event) => event.id);
  if (ids.join(",") !== expected.join(",")) {
    throw new Error(`${label}: expected the ordered ids [${expected.join(",")}]; got [${ids.join(",")}]`);
  }
  for (let i = 1; i < ids.length; i += 1) {
    if (ids[i] <= ids[i - 1]) {
      throw new Error(`${label}: the ids must strictly increase; got [${ids.join(",")}]`);
    }
  }
}

async function readRun(viewURL, executionID, afterID) {
  return apiJSON(`${viewURL}/api/execution/${executionID}/run?after_id=${afterID}`);
}

async function waitForRun(viewURL, executionID, predicate, timeoutMs = 30000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readRun(viewURL, executionID, 0);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`The run projection never satisfied the expectation in time; last: ${JSON.stringify(last)}`);
}

// --- harness ---------------------------------------------------------------

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
  console.log(`Smoke test for talking to a local Codex run from archetipo view against a fake Codex binary

Usage:
  node ./test/e2e/local-run-view-smoke.mjs
  npm run test:view-local-run-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated sandbox
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
  });
}

async function startViewServer(cwd, controlURL) {
  const child = spawn(cliPath, ["view", "--host", "127.0.0.1", "--port", "0", "--no-open"], {
    cwd,
    env: { ...cliEnv, FAKE_CODEX_CONTROL: controlURL },
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
  await waitForHTTP(`${url}/api/board`);
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
      env: options.env || cliEnv,
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

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
