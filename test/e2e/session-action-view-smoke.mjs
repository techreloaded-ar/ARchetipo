#!/usr/bin/env node

// End-to-end smoke for "the ARchetipo actions of a workspace happen inside one
// native session" (task 08 of the native-sessions plan).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the claude execution provider with its native session,
// the durable conversation journal, the execution records, the confirmation of
// the persisted effects, and the routes a person's gestures go through. Only
// the agent binary is replaced, by `support/fake-claude.mjs`, which speaks the
// same stream-json protocol on stdio, so nothing here needs a credential or a
// network.
//
// What it proves is the thing that used to be impossible: pressing "Pianifica"
// and then "Implementa" starts *two* executions in *one* conversation, held by
// *one* native session, and the conversation is still the person's to write in
// when both are over. The fake never progresses on its own, so every state
// asserted here is a state the test commanded.

import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildCLI as buildCLIShared, createRunDir as createRunDirShared, escapeHTML, makeRunCommand, parseCommonArgs, readBody, stopProcess as stopProcessShared } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "session-action-view-smoke");

const AUTH_ENV = "CLAUDE_FAKE_AUTH";
const AUTH_SENTINEL = "claude-session-action-material-DO-NOT-EXPOSE";

const SPEC = "US-901";
const QUESTION = "Con quale strategia di test vuoi il piano?";
const ANSWER = "smoke-session-action-answer-sentinel";
const FREE_MESSAGE = "smoke-session-action-free-message";
const PLAN_RECEIPT = JSON.stringify({ spec_code: SPEC, status: "PLANNED", tasks: 2 });
const IMPLEMENT_RECEIPT = JSON.stringify({ spec_code: SPEC, status: "REVIEW", tasks_done: 2, tests: "verdi" });
const WORKSPACE_MODEL = "sonnet";
const NEXT_TURN_MODEL = "opus";

// Every viewer response body is kept, so the final check can prove no session
// material travelled to the browser on any route this run touched.
const viewerBodies = [];
const checks = [];

function ok(criterion, statement) {
  checks.push({ criterion, statement });
  console.log(`-> ${criterion} ok: ${statement}`);
}

// --- the script -------------------------------------------------------------

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake binary relies on a POSIX shebang");
    return;
  }
  const runDir = await createRunDir(options.workspaceRoot);
  process.env.ARCHETIPO_STATE_DIR = path.join(runDir, "state");
  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const startedAt = Date.now();
  let failure = null;
  try {
    await scenarioTwoActionsInOneSession(runDir);
    assertNoSessionMaterialLeaked();
  } catch (error) {
    failure = error;
  }

  await writeReport(runDir, { startedAt, durationMs: Date.now() - startedAt, failure });

  if (failure) {
    if (options.cleanup) await fs.rm(runDir, { recursive: true, force: true });
    throw failure;
  }
  if (options.cleanup) {
    await fs.rm(runDir, { recursive: true, force: true });
    console.log(`-> cleaned workspace: ${runDir}`);
  }
  console.log(`\nPASS: session-action-view smoke test completed (${checks.length} statements proved).`);
  console.log(`Sandbox: ${runDir}`);
}

async function scenarioTwoActionsInOneSession(runDir) {
  console.log("\n=== one conversation, two actions, one native session ===");
  const w = await openWorkspace(runDir, "sandbox");
  try {
    // --- the plan, which asks before it answers ---------------------------
    const plan = await pressSpecAction(w, SPEC, { action: "plan" });
    if (plan.status !== "RUNNING" || !plan.id) {
      throw new Error(`the plan did not start: ${JSON.stringify(plan)}`);
    }
    const prompt = userFrameText(await w.control.waitFor(userFrame, 1));
    if (!prompt.includes("/archetipo-plan") || !prompt.includes(SPEC)) {
      throw new Error(`the prompt does not ask to plan ${SPEC}: ${JSON.stringify(prompt)}`);
    }
    const conversationID = await conversationOf(w.view.url, plan.id);
    const nativeBefore = await nativeSessionOf(w.view.url, conversationID);
    if (!nativeBefore) {
      throw new Error("the conversation carrying the action holds no native session");
    }
    ok("one session", `pressing the action opened the conversation ${conversationID} on the native session ${nativeBefore}, and gave it a prompt naming /archetipo-plan ${SPEC}`);

    // The agent asks. A turn that ends on a question does not end the action.
    w.control.push(emit({ type: "assistant", message: { content: [{ type: "text", text: QUESTION }] } }));
    w.control.push(emit({ type: "result", subtype: "success", is_error: false, result: QUESTION }));
    await waitForConversation(w.view.url, conversationID, (view) =>
      (view.events || []).some((event) => event.kind === "text" && event.text === QUESTION));
    const stillRunning = await apiJSON(`${w.view.url}/api/execution/${plan.id}`);
    if (stillRunning.status !== "RUNNING") {
      throw new Error(`a question closed the plan as ${stillRunning.status}: ${JSON.stringify(stillRunning.error)}`);
    }
    ok("many turns", "the agent's question ended its turn and left the plan RUNNING, waiting for a person");

    // The person answers in the thread they are reading, and the answer belongs
    // to the action it answers.
    announceSession(w);
    await apiJSON(`${w.view.url}/api/workspace/conversations/${conversationID}/messages`, postJSON({ message: ANSWER }), 202);
    const answered = userFrameText(await w.control.waitFor(userFrame, 2));
    if (answered !== ANSWER) {
      throw new Error(`the process received ${JSON.stringify(answered)} instead of the answer`);
    }

    // The plan is persisted — by this smoke, through the CLI, exactly as the
    // skill would — and the agent closes on its receipt.
    await persistPlan(w.sandboxDir);
    w.control.push(emit({ type: "assistant", message: { content: [{ type: "text", text: PLAN_RECEIPT }] } }));
    w.control.push(emit({ type: "result", subtype: "success", is_error: false, result: PLAN_RECEIPT }));
    const planned = await waitForExecution(w.view.url, plan.id, (record) => record.status !== "RUNNING");
    if (planned.status !== "SUCCEEDED") {
      throw new Error(`the plan must succeed once the backlog backs its receipt; got ${JSON.stringify(planned)}`);
    }
    ok("record closed", `the plan closed SUCCEEDED on a receipt emitted two turns after it started, verified against the backlog`);

    // --- the model of the next turn ---------------------------------------
    await apiJSON(
      `${w.view.url}/api/workspace/conversations/${conversationID}/next-turn`,
      putJSON({ model: NEXT_TURN_MODEL, model_options: {} }),
      200,
    );

    // --- the implementation, in the very same session ---------------------
    const implement = await pressSpecAction(w, SPEC, { action: "implement" }, conversationID);
    if (implement.id === plan.id) {
      throw new Error("the implementation reused the record of the plan");
    }
    const implementPrompt = userFrameText(await w.control.waitFor(userFrame, 3));
    if (!implementPrompt.includes("/archetipo-implement")) {
      throw new Error(`the prompt does not ask to implement: ${JSON.stringify(implementPrompt)}`);
    }
    const nativeAfter = await nativeSessionOf(w.view.url, conversationID);
    if (nativeAfter !== nativeBefore) {
      throw new Error(`the second action moved the conversation from ${nativeBefore} to ${nativeAfter}`);
    }
    const invocations = w.control.all("argv");
    const resumed = invocations[invocations.length - 1].argv || [];
    if (resumed[resumed.indexOf("--resume") + 1] !== nativeBefore) {
      throw new Error(`the implementation did not resume the same native session: ${JSON.stringify(resumed)}`);
    }
    if (resumed[resumed.indexOf("--model") + 1] !== NEXT_TURN_MODEL) {
      throw new Error(`the chosen model did not reach the turn: ${JSON.stringify(resumed)}`);
    }
    ok("same session", `the implementation ${implement.id} opened its turn with --resume ${nativeBefore} and --model ${NEXT_TURN_MODEL}, under a record of its own`);

    await markImplemented(w.sandboxDir);
    w.control.push(emit({ type: "assistant", message: { content: [{ type: "text", text: IMPLEMENT_RECEIPT }] } }));
    w.control.push(emit({ type: "result", subtype: "success", is_error: false, result: IMPLEMENT_RECEIPT }));
    const implemented = await waitForExecution(w.view.url, implement.id, (record) => record.status !== "RUNNING");
    if (implemented.status !== "SUCCEEDED") {
      throw new Error(`the implementation must succeed once the backlog backs its receipt; got ${JSON.stringify(implemented)}`);
    }

    // --- the conversation outlives both actions ---------------------------
    announceSession(w);
    await apiJSON(`${w.view.url}/api/workspace/conversations/${conversationID}/messages`, postJSON({ message: FREE_MESSAGE }), 202);
    const free = userFrameText(await w.control.waitFor(userFrame, 4));
    if (free !== FREE_MESSAGE) {
      throw new Error(`the free message did not reach the process: ${JSON.stringify(free)}`);
    }
    const view = await readConversation(w.view.url, conversationID);
    if (view.conversation?.state !== "ACTIVE") {
      throw new Error(`the conversation did not stay open: ${JSON.stringify(view.conversation)}`);
    }
    const runs = view.runs || [];
    if (runs.length !== 2 || !runs.every((run) => run.in_this_thread)) {
      throw new Error(`the thread must show its two actions and quote no other one: ${JSON.stringify(runs)}`);
    }
    if (runs.some((run) => (run.events || []).length !== 0)) {
      throw new Error("an action of this thread carries its own log, drawing the same turns twice");
    }
    ok("session outlives", `both records closed SUCCEEDED, the conversation is still ACTIVE and accepted a free message afterwards, and its two action blocks carry no second copy of the timeline`);

    const conversations = await apiJSON(`${w.view.url}/api/workspace/conversations`);
    if ((conversations.conversations || []).length !== 1) {
      throw new Error(`the workspace holds ${(conversations.conversations || []).length} conversations, want exactly one`);
    }
    ok("one agent", `the two actions and the free message were carried out by one conversation and one native session, never by a second agent`);
  } finally {
    await w.close();
  }
}

function assertNoSessionMaterialLeaked() {
  const leaked = viewerBodies.filter((body) => body.includes(AUTH_SENTINEL));
  if (leaked.length) {
    throw new Error(`the viewer echoed the session material in ${leaked.length} response(s)`);
  }
  ok("no leak", `the session material is absent from all ${viewerBodies.length} viewer responses and from the workspace configuration`);
}

// --- one workspace, ready to be pressed -------------------------------------

async function openWorkspace(runDir, name) {
  const sandboxDir = path.join(runDir, name);
  const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, [AUTH_ENV]: AUTH_SENTINEL };
  await fs.mkdir(sandboxDir, { recursive: true });
  await runCommand(`init/${name}`, cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: sandboxDir, env });
  await seedBacklog(sandboxDir);

  const control = await startControlServer();
  console.log(`-> control server for the fake claude: ${control.url}`);
  const view = await startViewServer(sandboxDir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
  console.log(`-> view ready: ${view.url}`);

  await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
    id: "claude",
    config: { command: fakeClaudePath, model: WORKSPACE_MODEL, timeout_seconds: 600 },
  }));

  return {
    sandboxDir,
    control,
    view,
    async close() {
      const configBody = await fs.readFile(path.join(sandboxDir, ".archetipo", "config.yaml"), "utf8").catch(() => "");
      if (configBody.includes(AUTH_SENTINEL)) {
        throw new Error(`the workspace configuration carries agent session material:\n${configBody}`);
      }
      await stopProcess(view.child);
      await control.close();
    },
  };
}

async function seedBacklog(sandboxDir) {
  const file = path.join(sandboxDir, ".archetipo", "specs-input.json");
  const specs = [{
    code: SPEC,
    title: "Portare avanti un'azione nella sessione",
    epic: { code: "EP-009", title: "Sessioni native" },
    priority: "HIGH",
    points: 3,
    status: "TODO",
    body: [
      "**User Story**",
      "Come persona, voglio che l'azione lavori nel thread che sto leggendo.",
      "",
      "**Criteri di accettazione**",
      "- [ ] AC-1 — L'azione entra nella sessione aperta.",
      "",
    ].join("\n"),
  }];
  await fs.writeFile(file, `${JSON.stringify({ specs }, null, 2)}\n`);
  await runCommand("spec-add", cliPath, ["spec", "add", "--file", file], { cwd: sandboxDir, env: { ...process.env, ARCHETIPO_DATA_DIR: repoRoot } });
  await fs.rm(file, { force: true });
}

// persistPlan writes the plan through the CLI, exactly as the planning skill
// does. The fake never plans anything: what is being proved is that the viewer
// confirms a receipt against the backlog, not that a fake can write one.
async function persistPlan(sandboxDir) {
  const file = path.join(sandboxDir, ".archetipo", "plan-input.json");
  await fs.writeFile(file, `${JSON.stringify({
    plan_body: `# Piano di ${SPEC}\n\nDue passi.\n`,
    tasks: [
      { id: "TASK-01", title: "Scrivere il codice", type: "impl", body: "## Objective\nCodice.\n" },
      { id: "TASK-02", title: "Scrivere i test", type: "test", body: "## Objective\nTest.\n" },
    ],
  }, null, 2)}\n`);
  await runCommand("spec-plan", cliPath, ["spec", "plan", SPEC, "--file", file], { cwd: sandboxDir, env: { ...process.env, ARCHETIPO_DATA_DIR: repoRoot } });
  await fs.rm(file, { force: true });
}

// markImplemented is what the implementation skill leaves behind: every task
// done and the spec waiting under review.
async function markImplemented(sandboxDir) {
  const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };
  for (const task of ["TASK-01", "TASK-02"]) {
    await runCommand(`task-complete/${task}`, cliPath, ["task", "complete", SPEC, task], { cwd: sandboxDir, env });
  }
  await runCommand("spec-review", cliPath, ["spec", "review", SPEC], { cwd: sandboxDir, env });
}

// --- the protocol -----------------------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

function userFrame(entry) {
  return entry.kind === "received" && entry.frame?.type === "user";
}

function userFrameText(entry) {
  return (entry.frame?.message?.content || []).map((block) => block.text || "").join("");
}

// pressSpecAction is a press on an action of a spec, in the shape the press now
// really has: the action works in a native session, so the agent process starts
// *inside* the request and announces its session while the response is still in
// flight.
async function pressSpecAction(w, code, payload, conversationID) {
  const body = conversationID ? { ...payload, conversation_id: conversationID } : payload;
  const pressed = apiJSON(`${w.view.url}/api/spec/${code}/execution`, postJSON(body), 201);
  const invocation = await w.control.waitFor("argv", w.control.count("argv") + 1);
  const argv = invocation.argv || [];
  const flag = argv.includes("--session-id") ? "--session-id" : "--resume";
  w.sessionID = argv[argv.indexOf(flag) + 1];
  announceSession(w);
  return pressed;
}

// announceSession queues the frame that opens the next turn's process: a
// streaming Claude exits when a turn ends, so every turn after the first is a
// new process resumed on the same session id, announcing itself with its own
// init.
function announceSession(w) {
  w.control.push(emit({ type: "system", subtype: "init", session_id: w.sessionID }));
}

function postJSON(payload) {
  return { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

function putJSON(payload) {
  return { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

// --- the control server -----------------------------------------------------

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

  return {
    url,
    push(command) {
      commands.push(command);
    },
    count(matcher) {
      return received.filter(matches(matcher)).length;
    },
    all(matcher) {
      return received.filter(matches(matcher));
    },
    async waitFor(matcher, count = 1, timeoutMs = 30000) {
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matching = received.filter(matches(matcher));
        if (matching.length >= count) return matching[count - 1];
        await delay(50);
      }
      throw new Error(
        `The fake never received ${typeof matcher === "function" ? matcher.name : matcher} ${count} time(s); it received ${JSON.stringify(received.map((entry) => entry.kind))}`,
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

// --- oracles ----------------------------------------------------------------

async function readConversation(viewURL, id) {
  return apiJSON(`${viewURL}/api/workspace/conversations/${id}`);
}

async function waitForConversation(viewURL, id, predicate, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readConversation(viewURL, id);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`The conversation never satisfied the expectation in time; last: ${JSON.stringify(last)}`);
}

async function waitForExecution(viewURL, executionID, predicate, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await apiJSON(`${viewURL}/api/execution/${executionID}`);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`The execution never satisfied the expectation in time; last: ${JSON.stringify(last)}`);
}

// conversationOf names the thread an execution is being carried out in. It is
// read from the run projection, which is where a board reader learns it.
async function conversationOf(viewURL, executionID) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < 30000) {
    last = await apiJSON(`${viewURL}/api/execution/${executionID}/run`);
    if (last.thread_id) return last.thread_id;
    await delay(100);
  }
  throw new Error(`The run of ${executionID} never named the thread it happens in; last: ${JSON.stringify(last)}`);
}

async function nativeSessionOf(viewURL, conversationID) {
  const view = await readConversation(viewURL, conversationID);
  return view.session?.session?.native?.id || "";
}

// --- harness ----------------------------------------------------------------

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for the ARchetipo actions of a workspace happening inside one native session

Usage:
  node ./test/e2e/session-action-view-smoke.mjs
  npm run test:view-session-action-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated sandbox
  --cleanup               Remove the run directory after the test passes/fails
`);
}

async function createRunDir(root) {
  return createRunDirShared(root, true);
}

async function buildCLI() {
  return buildCLIShared(cliPath, repoRoot, runCommand);
}

async function startViewServer(cwd, env) {
  return startViewServerShared(cliPath, cwd, env, "/api/workspace/actions");
}

async function apiJSON(url, init = {}, expected = null) {
  let response;
  try {
    response = await fetch(url, { ...init, headers: { Accept: "application/json", ...(init.headers || {}) } });
  } catch (error) {
    throw new Error(`${init.method || "GET"} ${url} did not answer: ${error.message}`, { cause: error });
  }
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

async function runCommand(label, command, args, options = {}) {
  return makeRunCommand()(label, command, args, options);
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

// --- report -----------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "session-action-view",
    spec: "sessioni-native/08",
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
  <title>ARchetipo Smoke — ARchetipo actions inside one native session</title>
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
      <h1>ARchetipo Smoke — ARchetipo actions inside one native session</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>One workspace served by the real <code>archetipo view</code>, against a fake Claude Code binary driven
    frame by frame: a plan started from the board opens one native session, asks a question, is answered in
    that same thread and closes on its receipt; the model is chosen for the next turn; an implementation runs
    in the very same session under a record of its own; and a free message follows, belonging to no action.</p>

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


main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
