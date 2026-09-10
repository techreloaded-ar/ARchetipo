#!/usr/bin/env node

// End-to-end smoke for "follow an action and go on talking inside the
// conversation that asked for it" (US-060).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector on disk, the `claude` provider with
// its stream-json client and its native session, the conversation routes
// (`POST` on `/api/workspace/conversations` and `GET`, `POST .../messages`,
// `POST .../proposal`, `DELETE` on `/api/workspace/conversations/{id}`), the
// rail route `GET /api/workspace/runs` and the document the browser is really
// served on `GET /index.html`. One thing is replaced, and only one: the agent
// binary, by `support/fake-claude.mjs`, which speaks the same protocol on
// stdio. Nothing here needs a credential or leaves the loopback interface.
//
// **What changed, and why one provider is now enough.** An ARchetipo action is
// carried out in the conversation's own native session and never by a second
// agent started beside it. There is therefore no run process to drive apart
// from the conversation, no second control server, and no run block with a
// timeline of its own: the work of the action *is* the thread, and what used to
// be "the events of the run" is what the conversation says while its turn
// works.
//
// **What is not asserted here.** The consents of a run served by a remote
// ARcipelago hub, and the answers given to them, belonged to the model where
// the run was a separate agent reached through a provider of its own. On a
// native session an approval is asked by the session and answered in it; the
// remote half of that story is the business of tasks 10 and 11 of the native
// sessions plan, which are not delivered, so this smoke no longer claims it.
//
// **What proves what.** AC-2 is proved by absence and it is counted, not
// claimed: every viewer request this file makes goes through one recorder, and
// the assertion is that what the action was doing was learnt with **zero** calls
// to `GET /api/execution/{id}/run`. AC-5 is proved by the agent process itself
// reporting the frame it was given, never by the 202 of the route.
//
// Nothing progresses on its own and nothing sleeps for an outcome: every agent
// frame is emitted by this test, and each wait polls a route or the control
// server with an explicit timeout that names what it expected and what arrived
// instead.

import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildCLI as buildCLIShared, createRunDir as createRunDirShared, escapeHTML, parseCommonArgs, readBody, stopProcess as stopProcessShared } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "conversation-run-view-smoke");

// The spec the agent proposes to implement: planned, with a persisted plan, so
// the process really admits the action on it.
const SPEC = "US-A01";
const PROPOSED_ACTION = "implement";

// The message sent while the action is being carried out, and the answer to it.
// Both are sentinels: the first has to reach the agent process, the second has
// to come back into the timeline of the same conversation.
const MESSAGE_SENTINEL = "smoke-message-while-the-action-runs";
const REPLY_SENTINEL = "smoke-reply-while-the-action-runs";

// Every viewer request this file makes, in order. It is what turns "the panel
// of that run was never opened" from a comment into an assertion.
const viewerRequests = [];

// One entry per proved statement, for the report.
const checks = [];

function ok(criterion, statement) {
  checks.push({ criterion, statement });
  console.log(`-> ${criterion} ok: ${statement}`);
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake binary relies on a POSIX shebang");
    return;
  }
  const runDir = await createRunDir(options.workspaceRoot);
  console.log(`-> run directory: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const startedAt = Date.now();
  let failure = null;
  try {
    await scenario(runDir);
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
    console.log(`-> cleaned run directory: ${runDir}`);
  }
  console.log(`\nPASS: conversation-run view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// One workspace, one conversation, one action — because that is the story: the
// action is born in the conversation, carried out *in* it, answered in it and
// ended in it, and the person never opens a separate list to learn any of that.
async function scenario(runDir) {
  const sandboxDir = path.join(runDir, "sandbox");
  await fs.mkdir(sandboxDir, { recursive: true });

  let control;
  let view;
  try {
    // Every process started from here writes the registry of known workspaces
    // inside the run directory, never in the real state of the machine.
    const env = {
      ...process.env,
      ARCHETIPO_DATA_DIR: repoRoot,
      ARCHETIPO_STATE_DIR: path.join(runDir, "state"),
    };

    await createWorkspace(runDir, sandboxDir, env);

    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);

    view = await startViewServer(sandboxDir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
    console.log(`-> view ready: ${view.url} (launched in ${sandboxDir})`);

    // --- the conversation --------------------------------------------------
    // The provider is saved through the very route the Execution panel uses, so
    // the conversation starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: fakeClaudePath, timeout_seconds: 600 },
    }));

    const opened = await apiJSON(`${view.url}/api/workspace/conversations`, postJSON({}), 201);
    const conversationID = opened.conversation?.id;
    if (opened.available !== true || !conversationID) {
      throw new Error(`the conversation did not open: ${JSON.stringify(opened)}`);
    }
    if (!Array.isArray(opened.runs) || opened.runs.length !== 0) {
      throw new Error(`a freshly opened conversation has started nothing; got ${JSON.stringify(opened.runs)}`);
    }
    const agent = await control.waitFor("argv", 1);
    control.push(emit(initFrame(nativeSessionIDOf(agent))), agent.pid);

    control.push(emit(assistantProposal(
      `Posso implementare ${SPEC}, che è pianificata.`,
      PROPOSED_ACTION,
      SPEC,
    )), agent.pid);
    const pending = await waitForConversation(
      view.url,
      conversationID,
      (data) => data.proposal?.runnable === true,
      `the runnable proposal of ${PROPOSED_ACTION} on ${SPEC}`,
    );
    const proposalEventID = pending.proposal.event_id;
    if (!Number.isInteger(proposalEventID) || proposalEventID <= 0) {
      throw new Error(`the proposal must be anchored to an event of the history; got ${JSON.stringify(pending.proposal)}`);
    }

    // --- AC-1 ---------------------------------------------------------------
    // The confirmation starts the action in this very conversation: the turn
    // that carries it out is run by a process of its own, on the session that
    // is already open, and it has to be served while the route is answering.
    const confirmed = await serveStartingProcesses(control, apiJSON(
      `${view.url}/api/workspace/conversations/${conversationID}/proposal`,
      postJSON({ proposal_id: proposalEventID, decision: "accept" }),
      201,
    ));
    const executionID = confirmed.outcome?.execution_id;
    if (!executionID) {
      throw new Error(`the confirmation must name the execution it started; got ${JSON.stringify(confirmed.outcome)}`);
    }

    const born = await readConversation(view.url, conversationID);
    if (!Array.isArray(born.runs) || born.runs.length !== 1) {
      throw new Error(`AC-1: the conversation must carry exactly the run it asked for; got ${JSON.stringify(born.runs)}`);
    }
    const block = born.runs[0];
    assertEqual(block.execution_id, executionID, "AC-1: the execution the run block is about");
    assertEqual(block.anchor_event_id, proposalEventID, "AC-1: the event of the history the run block is anchored to");
    assertEqual(block.action, PROPOSED_ACTION, "AC-1: the action the run block names");
    assertEqual(block.spec_code, SPEC, "AC-1: the spec the run block names");
    assertEqual(block.decision, "confirmed", "AC-1: the decision that started the run");
    // The action runs in this thread and not beside it. It is the difference
    // the whole story now rests on: no second agent was started, so the block
    // says the work is happening here.
    assertEqual(block.in_this_thread, true, "AC-1: whether the action is carried out in this conversation");
    const records = await listExecutionRecords(sandboxDir);
    if (records.length !== 1 || records[0] !== `${executionID}.json`) {
      throw new Error(`AC-1: the block names ${executionID} but the filesystem holds ${JSON.stringify(records)}`);
    }
    ok(
      "AC-1",
      `confirming the proposal carried at event ${proposalEventID} answered 201 with execution ${executionID}, the conversation then carries exactly one run block for that id — action ${block.action} on ${block.spec_code}, decision ${block.decision}, carried out in this thread — anchored to event ${block.anchor_event_id}, and that id is the single record under .archetipo/executions/`,
    );

    // --- AC-2 ---------------------------------------------------------------
    // What the action does is read from the conversation alone. In a native
    // session the work of an action *is* the thread: its turn speaks into the
    // same timeline the person is reading, so what used to be the events of a
    // separate run is now what the conversation says. The count below is what
    // says the run panel was never opened to learn any of it.
    const turn = await control.waitFor("argv", 2);
    control.push(emit(assistantText("Leggo il piano di " + SPEC)), turn.pid);
    control.push(emit(assistantText(" e apro i file")), turn.pid);
    const growing = await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.events || []).filter((event) => (event.text || "").includes("Leggo il piano di ") || (event.text || "").includes(" e apro i file")).length >= 2,
      `the two frames of the turn carrying ${executionID} to appear in the timeline of the conversation`,
    );
    assertEqual(growing.runs?.[0]?.status, "RUNNING", "AC-2: the status of the execution record while the agent works");
    assertEqual(growing.runs?.[0]?.execution_id, executionID, "AC-2: the execution the block is still about");
    assertNoRunPanelCalls(executionID, "AC-2: while the work of the action was being observed");
    const conversationReads = viewerRequests.filter((entry) => entry.path.startsWith("/api/workspace/conversations/")).length;
    ok(
      "AC-2",
      `the two frames the turn of ${executionID} emitted appear in the timeline of ${conversationID} itself and its record is RUNNING, observed through ${conversationReads} read(s) of /api/workspace/conversations/${conversationID} and exactly 0 calls to GET /api/execution/${executionID}/run — counted over the ${viewerRequests.length} requests this smoke had made`,
    );

    // --- AC-5 ---------------------------------------------------------------
    // The conversation is not taken hostage by the action running in it: a
    // person can speak while the turn works, and what they say is steered into
    // that turn. The oracle is the agent process saying it was given the
    // message, never the 202 of the route.
    const accepted = await serveStartingProcesses(control, apiJSON(
      `${view.url}/api/workspace/conversations/${conversationID}/messages`,
      postJSON({ message: MESSAGE_SENTINEL }),
      202,
    ));
    assertEqual(accepted.available, true, "AC-5: whether the conversation is still available while an action of its own runs");
    const delivered = await control.waitFor(userFrameCarrying(MESSAGE_SENTINEL), 1);
    control.push(emit(assistantText(REPLY_SENTINEL)), delivered.pid);
    const answered = await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.events || []).some((event) => (event.text || "").includes(REPLY_SENTINEL)),
      "the agent's answer to appear in the timeline of the conversation",
    );
    assertEqual(answered.conversation?.id, conversationID, "AC-5: the conversation that answered");
    assertEqual(answered.runs?.[0]?.status, "RUNNING", "AC-5: whether the action is still running after the exchange");
    ok(
      "AC-5",
      `while ${executionID} was being carried out, POST /api/workspace/conversations/${conversationID}/messages answered 202 with available:true, the agent process itself reported having been given ${JSON.stringify(MESSAGE_SENTINEL)}, its answer came back into the timeline of ${conversationID}, and the action was still running`,
    );

    // --- AC-6 ---------------------------------------------------------------
    const listed = await apiJSON(`${view.url}/api/workspace/runs`);
    const entry = (listed.runs || []).find((row) => row.id === executionID);
    if (!entry) {
      throw new Error(`AC-6: the running action must be listed; got ${JSON.stringify(listed.runs)}`);
    }
    assertEqual(entry.conversation_id, conversationID, "AC-6: the conversation the listed entry points back to");
    assertEqual(entry.anchor_event_id, proposalEventID, "AC-6: the point of that conversation the listed entry points back to");
    const html = await rawGet(`${view.url}/index.html`);
    if (!html.includes('id="runs-attention"')) {
      throw new Error("AC-6: the served index.html carries no #runs-attention: the notice would have nowhere to appear");
    }
    ok(
      "AC-6",
      `GET /api/workspace/runs lists ${executionID} carrying conversation_id ${entry.conversation_id} with anchor_event_id ${entry.anchor_event_id} — the conversation and the exact point the rail leads back to — while the served index.html carries #runs-attention`,
    );

    // --- the closing discipline of the sibling smokes ------------------------
    const closed = await apiJSON(`${view.url}/api/workspace/conversations/${conversationID}`, { method: "DELETE" }, 200);
    if (closed.conversation?.state !== "CLOSED") {
      throw new Error(`the close must report the state the session observed; got ${JSON.stringify(closed.conversation)}`);
    }
  } finally {
    if (view) await stopProcess(view.child);
    if (control) await control.close();
  }
}


// --- oracles ------------------------------------------------------------------

// assertNoRunPanelCalls expresses AC-2 as a count: the run panel is what calls
// GET /api/execution/{id}/run, and following a run inside the conversation must
// not require having opened it.
function assertNoRunPanelCalls(executionID, label) {
  const prefix = `/api/execution/${executionID}/run`;
  const calls = viewerRequests.filter((entry) => entry.path === prefix || entry.path.startsWith(`${prefix}?`));
  if (calls.length !== 0) {
    throw new Error(`${label}: the run panel of ${executionID} was opened ${calls.length} time(s): ${JSON.stringify(calls)}`);
  }
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`Unexpected ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

async function listExecutionRecords(root) {
  const entries = await fs.readdir(path.join(root, ".archetipo", "executions")).catch(() => []);
  return entries.filter((name) => name.endsWith(".json")).sort();
}

// --- reading the conversation ---------------------------------------------------

async function readConversation(viewURL, conversationID, afterID = 0) {
  return apiJSON(`${viewURL}/api/workspace/conversations/${encodeURIComponent(conversationID)}?after_id=${afterID}`);
}

// waitForConversation polls the read route until the conversation reports what
// the test has just caused. `what` is not decoration: a timeout has to say what
// it was waiting for and what the last reading held, or the failure names
// nothing.
async function waitForConversation(viewURL, conversationID, predicate, what, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  let lastError = null;
  while (Date.now() - started < timeoutMs) {
    try {
      last = await readConversation(viewURL, conversationID);
      lastError = null;
      if (predicate(last)) return last;
    } catch (error) {
      lastError = error;
    }
    await delay(150);
  }
  const detail = lastError
    ? `last error: ${lastError.message}`
    : `last read: proposal ${truncate(JSON.stringify(last?.proposal), 200)}, ${(last?.events || []).length} event(s), runs ${truncate(JSON.stringify(last?.runs), 700)}`;
  throw new Error(`Timed out after ${timeoutMs}ms waiting for ${what}\n  ${detail}`);
}

// --- the protocol of the fake agent ---------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

function assistantText(text) {
  return { type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text }] } };
}

// assistantProposal is a message of the agent closed by the proposal line: a
// sentence a person reads, then the single JSON line
// execution.ParseActionProposal recognizes. The line is last because that is
// what the recognizer scans for.
function assistantProposal(sentence, action, specCode) {
  const line = JSON.stringify({ artifact: "action_proposal", action, spec: specCode });
  return assistantText(`${sentence}\n${line}`);
}

// userFrame recognizes a frame the process was given on its standard input as
// an operator message, which is the shape the opening instruction and every
// later message share.
// serveStartingProcesses answers, while a request is in flight, every agent
// process that request starts: each is given the announcement of the very
// session id ARchetipo assigned to it, which is the only one the provider
// accepts from it. A route that starts a process answers only once that process
// has announced itself, so the two have to happen at the same time.
async function serveStartingProcesses(control, request) {
  // The request is taken hold of first: anything that threw before this line
  // would leave it floating, and a rejected promise nobody is waiting on takes
  // the whole run down with an error that names none of this.
  let pending = true;
  const answer = request.finally(() => {
    pending = false;
  });
  answer.catch(() => {});
  let served = control.reports().filter((entry) => entry.kind === "argv").length;
  while (pending) {
    const invocations = control.reports().filter((entry) => entry.kind === "argv");
    while (served < invocations.length) {
      control.push(emit(initFrame(nativeSessionIDOf(invocations[served]))), invocations[served].pid);
      served += 1;
    }
    await delay(25);
  }
  return answer;
}

// nativeSessionIDOf reads, from the command line of a process, which native
// session ARchetipo told it to be — or to take up.
function nativeSessionIDOf(invocation) {
  const argv = invocation.argv || [];
  for (const flag of ["--session-id", "--resume"]) {
    const at = argv.indexOf(flag);
    if (at >= 0 && argv[at + 1]) return argv[at + 1];
  }
  throw new Error(`the invocation names no native session: ${JSON.stringify(argv)}`);
}

function initFrame(sessionID) {
  return { type: "system", subtype: "init", session_id: sessionID };
}

// userFrameBlocks keeps the blocks of a user frame apart, which is what
// identifying one message requires: the instruction that opens a conversation
// is held until the first message and travels in the same frame, as a block of
// its own.
function userFrameBlocks(entry) {
  const content = entry.frame?.message?.content;
  if (!Array.isArray(content)) return [];
  return content.filter((block) => block?.type === "text").map((block) => block.text ?? "");
}

function userFrameCarrying(text) {
  const matcher = (entry) => userFrame(entry) && userFrameBlocks(entry).includes(text);
  Object.defineProperty(matcher, "name", { value: `a user frame carrying ${JSON.stringify(text)}` });
  return matcher;
}

function userFrame(entry) {
  return entry.kind === "received" && entry.frame?.type === "user";
}

function userFrameText(entry) {
  return (entry.frame?.message?.content || []).map((block) => block.text || "").join("");
}

// writeFakeWrapper produces the executable the Execution panel is pointed at
// for one of the two agent processes. It is one line: the same fake, started
// with its own control server. See the header for why the two processes cannot
// share one.
// --- the control server of a fake agent -------------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received, so every assertion is a statement about a
// state the test produced. One server per agent process, so a command is never
// taken by the process it was not meant for.

async function startControlServer() {
  const commands = [];
  const received = [];

  const server = http.createServer(async (req, res) => {
    if (req.method === "GET" && req.url.startsWith("/next")) {
      // A command may be addressed to one process. A native session is opened
      // by one process and every turn in it is run by another, so several of
      // them poll this server at once, and a frame meant for one — an
      // announcement above all, valid only for the process ARchetipo told to be
      // that session — must not be taken by whichever asks first.
      const pid = Number(new URL(req.url, url).searchParams.get("pid")) || 0;
      const index = commands.findIndex((entry) => entry.pid === 0 || entry.pid === pid);
      sendJSON(res, 200, index < 0 ? { kind: "none" } : commands.splice(index, 1)[0].command);
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
    push(command, pid = 0) {
      commands.push({ command, pid });
    },
    reports() {
      return received;
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
        `Timed out after ${timeoutMs}ms waiting for the fake to report ${describe(matcher)} ${count} time(s); it reported ${JSON.stringify(received.map((entry) => entry.kind))}`,
      );
    },
    close() {
      return new Promise((resolve) => server.close(resolve));
    },
  };
}

function sendJSON(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Content-Length": Buffer.byteLength(body) });
  res.end(body);
}


// --- fixtures ---------------------------------------------------------------------

// createWorkspace initializes one real workspace holding the spec the agent
// proposes to implement: planned, with a plan persisted through the CLI, which
// is the status in which the process admits that action.
async function createWorkspace(runDir, sandboxDir, env) {
  await runCommand("init", cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: sandboxDir, env });

  const specsFile = path.join(runDir, "specs.json");
  await fs.writeFile(specsFile, `${JSON.stringify({
    specs: [
      {
        code: SPEC,
        title: "Seguire una run dentro la conversazione",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "HIGH",
        points: 1,
        status: "TODO",
        body: [
          "**User Story**",
          "Come persona di smoke voglio seguire una run dentro la conversazione, per non sorvegliare un elenco a parte.",
          "",
          "**Criteri di accettazione**",
          "- [ ] AC-1 — la spec appare nella colonna TODO.",
          "",
        ].join("\n"),
      },
    ],
  }, null, 2)}\n`);
  await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir, env });

  // `spec plan` persists the plan and moves the spec to PLANNED. Doing it
  // through the CLI rather than by writing the files keeps the fixture a state
  // the product itself can produce.
  const planFile = path.join(runDir, "plan.json");
  await fs.writeFile(planFile, `${JSON.stringify({
    plan_body: `Piano minimo per ${SPEC}: nulla da costruire, serve solo che il piano esista.`,
    tasks: [
      {
        id: "TASK-01",
        title: "Task unico",
        type: "Impl",
        status: "TODO",
        body: "## Descrizione\n\nNessun lavoro reale: il piano esiste perché l'azione di implementazione lo richiede.\n",
      },
    ],
  }, null, 2)}\n`);
  await runCommand("spec-plan", cliPath, ["spec", "plan", SPEC, "--file", planFile], { cwd: sandboxDir, env });
}

// --- harness ------------------------------------------------------------------------

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for the run followed and answered inside its conversation (US-060)

Usage:
  node ./test/e2e/conversation-run-view-smoke.mjs
  npm run test:view-conversation-run-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated run directory
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
  return startViewServerShared(cliPath, cwd, env, "/api/board");
}

async function waitForHTTP(url) {
  const started = Date.now();
  while (Date.now() - started < 15000) {
    try {
      const response = await fetch(url, { headers: { Accept: "application/json" } });
      if (response.ok) return;
    } catch {
      // keep polling
    }
    await delay(200);
  }
  throw new Error(`Timed out after 15000ms waiting for ${url} to answer`);
}

function postJSON(payload) {
  return { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

function putJSON(payload) {
  return { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

// record is the single door every viewer request goes through, which is what
// makes the AC-2 count possible at all.
function record(url, init) {
  const parsed = new URL(url);
  viewerRequests.push({ method: (init.method || "GET").toUpperCase(), path: `${parsed.pathname}${parsed.search}` });
}

async function rawGet(url) {
  record(url, {});
  const response = await fetch(url);
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${url}: ${truncate(text, 400)}`);
  }
  return text;
}

async function apiJSON(url, init = {}, expected = null) {
  record(url, init);
  const response = await fetch(url, {
    ...init,
    headers: { Accept: "application/json", ...(init.headers || {}) },
  });
  const text = await response.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${url}: ${typeof data === "string" ? truncate(data, 400) : truncate(JSON.stringify(data), 400)}`);
  }
  if (expected !== null && response.status !== expected) {
    throw new Error(`Expected HTTP ${expected} for ${url}, got ${response.status}: ${truncate(text, 400)}`);
  }
  return data;
}

async function runCommand(label, command, args, options = {}) {
  console.log(`-> ${label}: ${command} ${args.join(" ")}`);
  const result = await new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env || process.env,
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

  if (result.code !== 0) {
    throw new Error(`${label} failed with exit ${result.code}\nSTDOUT:\n${result.stdout}\nSTDERR:\n${result.stderr}`);
  }
  return result;
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

function truncate(value, max = 200) {
  const text = String(value ?? "");
  return text.length <= max ? text : `${text.slice(0, max)}…`;
}

// --- report -------------------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "conversation-run-from-view",
    spec: "US-060",
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
  <title>ARchetipo Smoke — A run followed and answered inside its conversation (US-060)</title>
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
      <h1>ARchetipo Smoke — A run followed and answered inside its conversation (US-060)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>One real workspace served by the real <code>archetipo view</code>. The agent proposes an action, the
    proposal is confirmed, and the action it starts is carried out <em>inside that conversation</em> — no second
    agent is spawned — and followed from then on through <code>GET /api/workspace/conversations/{id}</code> alone:
    this smoke records every request it makes and asserts that it never called
    <code>GET /api/execution/{id}/run</code> to learn what the action was doing. While the action works, a message
    is sent in the same conversation and the agent process itself reports having been given it, and the rail
    <code>GET /api/workspace/runs</code> leads back to the exact point of the thread the action was asked at.</p>

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
