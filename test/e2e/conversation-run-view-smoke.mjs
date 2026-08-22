#!/usr/bin/env node

// End-to-end smoke for "follow a run and answer its requests inside the
// conversation that asked for it" (US-060).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector on disk, the `claude` provider with
// its stream-json client and its local session, the `arcipelago` provider with
// its SSE consumer and the server-side run follower, the conversation routes
// (`POST` on `/api/workspace/conversations` and `GET`, `POST .../messages`,
// `POST .../proposal`, `DELETE` on `/api/workspace/conversations/{id}`), the
// rail route `GET /api/workspace/runs`, the
// approval route `POST /api/execution/{id}/run/approvals/{id}` and the document
// the browser is really served on `GET /index.html`. Two things are replaced,
// and only two: the ARcipelago hub, by a local Node server bound to 127.0.0.1,
// and the agent binary, by `support/fake-claude.mjs`, which speaks the same
// protocol on stdio. Nothing here needs a credential or leaves the loopback
// interface.
//
// **Why two providers.** The story needs a run that stops and asks, and a
// conversation that keeps answering while it is stopped. Neither provider can
// do both: a local session runs with approvals disabled — `localrun.Collaborator`
// answers an empty list and refuses every response — so it never asks anything,
// and only `claude` can hold a conversation at all. A workspace has a single
// default, so the default is moved from one provider to the other through the
// very route the Execution panel uses, exactly as a person would.
//
// **Why two control servers.** Every frame the fake emits is commanded through
// a control server, and a command is taken by whichever process asks for it
// first. Two fake processes are alive at once here — the one holding the
// conversation and the one running the confirmed action — so a single server
// would hand a frame meant for the run to the conversation half the time. Each
// process is therefore started through its own one-line wrapper, written into
// the run directory, which exports its own `FAKE_CLAUDE_CONTROL` before exec'ing
// the very same fake: the wrapper is what the Execution panel is pointed at when
// each process is about to be started, so which process receives a frame is a
// fact of the fixture and never a race.
//
// **What proves what.** AC-2 is proved by absence and it is counted, not
// claimed: every viewer request this file makes goes through one recorder, and
// the assertion is that the growth of the run was observed with **zero** calls
// to `GET /api/execution/{id}/run`. AC-4 is proved on the hub — which option it
// was really told, and that the run then closed — and never on the HTTP status
// of the answer. AC-5 is proved by the agent process itself reporting the frame
// it was given, never by the 202 of the route.
//
// Nothing progresses on its own and nothing sleeps for an outcome: every agent
// frame is emitted by this test, the hub opens and closes each approval by hand,
// and each wait polls a route or the control server with an explicit timeout
// that names what it expected and what arrived instead.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "conversation-run-view-smoke");

// The spec the agent proposes to implement: planned, with a persisted plan, so
// the process really admits the action on it.
const SPEC = "US-A01";
const PROPOSED_ACTION = "implement";

// The remote side of the story.
const WORKSPACE_ID = "ws-conversation-run";
const TOKEN_ENV = "ARCIPELAGO_CONVERSATION_RUN_TOKEN";
const TOKEN_SENTINEL = "conversation-run-smoke-secret-token";
const RUN_ID = "run-1";

// The two decisions the run asks for. The command is asserted verbatim: a
// consent card that paraphrases what is about to be executed is not a consent
// card.
const APPROVAL_ALLOWED = "appr-1";
const APPROVAL_DENIED = "appr-2";
const APPROVAL_COMMAND = "git worktree prune --verbose";
const OPTION_ALLOW = "allow";
const OPTION_DENY = "deny";

// The message sent while the run is stopped, and the answer to it. Both are
// sentinels: the first has to reach the agent process, the second has to come
// back into the timeline of the same conversation.
const MESSAGE_SENTINEL = "smoke-message-while-the-run-waits";
const REPLY_SENTINEL = "smoke-reply-while-the-run-waits";

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

// One workspace, one conversation, one run — because that is the story: the run
// is born in the conversation, grows in it, stops in it, is answered in it and
// ends in it, and the person never opens a separate list to learn any of that.
async function scenario(runDir) {
  const sandboxDir = path.join(runDir, "sandbox");
  await fs.mkdir(sandboxDir, { recursive: true });

  let conversationControl;
  let runControl;
  let hub;
  let view;
  try {
    // Every process started from here writes the registry of known workspaces
    // inside the run directory, never in the real state of the machine.
    const env = {
      ...process.env,
      ARCHETIPO_DATA_DIR: repoRoot,
      ARCHETIPO_STATE_DIR: path.join(runDir, "state"),
      [TOKEN_ENV]: TOKEN_SENTINEL,
    };

    await createWorkspace(runDir, sandboxDir, env);

    conversationControl = await startControlServer();
    runControl = await startControlServer();
    console.log(`-> control server of the conversation process: ${conversationControl.url}`);
    console.log(`-> control server of the run process: ${runControl.url}`);
    hub = await startFakeHub();
    console.log(`-> fake ARcipelago hub: ${hub.url}`);

    const conversationCommand = await writeFakeWrapper(runDir, "conversation", conversationControl.url);
    const runCommandPath = await writeFakeWrapper(runDir, "run", runControl.url);

    view = await startViewServer(sandboxDir, env);
    console.log(`-> view ready: ${view.url} (launched in ${sandboxDir})`);

    // --- the conversation --------------------------------------------------
    // The provider is saved through the very route the Execution panel uses, so
    // the conversation starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: conversationCommand, timeout_seconds: 600 },
    }));

    conversationControl.push(emit({ type: "system", subtype: "init", session_id: "conversation-1" }));
    const opened = await apiJSON(`${view.url}/api/workspace/conversations`, postJSON({}), 201);
    const conversationID = opened.conversation?.id;
    if (opened.available !== true || !conversationID) {
      throw new Error(`the conversation did not open: ${JSON.stringify(opened)}`);
    }
    if (!Array.isArray(opened.runs) || opened.runs.length !== 0) {
      throw new Error(`a freshly opened conversation has started nothing; got ${JSON.stringify(opened.runs)}`);
    }
    await conversationControl.waitFor("argv", 1);

    conversationControl.push(emit(assistantProposal(
      `Posso implementare ${SPEC}, che è pianificata.`,
      PROPOSED_ACTION,
      SPEC,
    )));
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

    // The next agent process to be started is the run's, so the default is
    // pointed at its wrapper before the confirmation and not after: what the
    // conversation process is talking to was decided when it was spawned.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: runCommandPath, timeout_seconds: 600 },
    }));

    // --- AC-1 ---------------------------------------------------------------
    const confirmed = await apiJSON(
      `${view.url}/api/workspace/conversations/${conversationID}/proposal`,
      postJSON({ proposal_id: proposalEventID, decision: "accept" }),
      201,
    );
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
    const records = await listExecutionRecords(sandboxDir);
    if (records.length !== 1 || records[0] !== `${executionID}.json`) {
      throw new Error(`AC-1: the block names ${executionID} but the filesystem holds ${JSON.stringify(records)}`);
    }
    ok(
      "AC-1",
      `confirming the proposal carried at event ${proposalEventID} answered 201 with execution ${executionID}, the conversation then carries exactly one run block for that id — action ${block.action} on ${block.spec_code}, decision ${block.decision} — anchored to event ${block.anchor_event_id}, and that id is the single record under .archetipo/executions/`,
    );

    // --- AC-2 ---------------------------------------------------------------
    // The run's own process is driven frame by frame, and the growth is read
    // from the conversation alone: the count below is what says the run panel
    // was never opened to learn any of it.
    await runControl.waitFor("argv", 1);
    runControl.push(emit({ type: "system", subtype: "init", session_id: "run-1" }));
    runControl.push(emit(assistantText("Leggo il piano di " + SPEC)));
    runControl.push(emit(assistantText(" e apro i file")));
    const growing = await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.runs?.[0]?.events || []).filter((event) => event.kind === "text").length >= 2,
      `the two frames of the run process to appear inside the run block of ${executionID}`,
    );
    const growingBlock = growing.runs[0];
    assertEqual(growingBlock.run?.state, "ACTIVE", "AC-2: the state of the run while the agent works");
    assertEqual(growingBlock.status, "RUNNING", "AC-2: the status of the execution record while the agent works");
    const texts = growingBlock.events.map((event) => event.text || "").join("");
    if (!texts.includes("Leggo il piano di " + SPEC) || !texts.includes(" e apro i file")) {
      throw new Error(`AC-2: the frames the run emitted are not the ones the block shows; got ${JSON.stringify(growingBlock.events)}`);
    }
    assertNoRunPanelCalls(executionID, "AC-2: while the growth of the run was being observed");
    const conversationReads = viewerRequests.filter((entry) => entry.path.startsWith("/api/workspace/conversations/")).length;
    ok(
      "AC-2",
      `the two frames the run process emitted appear inside the run block and its state became ACTIVE, observed through ${conversationReads} read(s) of /api/workspace/conversations/${conversationID} and exactly 0 calls to GET /api/execution/${executionID}/run — counted over the ${viewerRequests.length} requests this smoke had made`,
    );

    // --- the remote provider ------------------------------------------------
    // Same panel, same route: the workspace default becomes the arcipelago
    // provider pointed at the local hub, which is the only provider that can
    // ask for a consent at all. The hub serves a task whose externalId is the
    // execution born in the conversation, so the very same run block is now
    // followed on the hub.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "arcipelago",
      config: {
        base_url: hub.url,
        workspace_id: WORKSPACE_ID,
        token_env: TOKEN_ENV,
        poll_interval_seconds: 1,
        timeout_seconds: 600,
      },
    }));
    const remote = hub.seedTask(executionID, SPEC);
    hub.assignRun(remote.id);
    const bound = await waitForConversation(
      view.url,
      conversationID,
      (data) => data.runs?.[0]?.run?.run_id === RUN_ID,
      `the run block of ${executionID} to be bound to the remote run ${RUN_ID}`,
    );
    assertEqual(bound.runs[0].execution_id, executionID, "the execution the bound run block is about");
    console.log(`-> the run block now follows the remote run ${RUN_ID} of task ${remote.id}`);

    // --- AC-3 ---------------------------------------------------------------
    hub.setApprovals([approval(APPROVAL_ALLOWED)]);
    const waiting = await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.runs?.[0]?.approvals || []).length === 1,
      `the consent ${APPROVAL_ALLOWED} to appear inside the run block of ${executionID}`,
    );
    const waitingBlock = waiting.runs[0];
    const asked = waitingBlock.approvals[0];
    assertEqual(asked.id, APPROVAL_ALLOWED, "AC-3: the consent the run block names");
    assertEqual(asked.tool_name, "Bash", "AC-3: the tool the consent is about");
    const args = JSON.stringify(asked.args);
    if (!args.includes(APPROVAL_COMMAND)) {
      throw new Error(`AC-3: the command must reach the flow verbatim; got ${args}`);
    }
    const optionIDs = (asked.options || []).map((option) => option.id).sort();
    if (JSON.stringify(optionIDs) !== JSON.stringify([OPTION_ALLOW, OPTION_DENY].sort())) {
      throw new Error(`AC-3: the consent must offer both answers; got ${JSON.stringify(asked.options)}`);
    }
    assertEqual(waitingBlock.awaiting_response, true, "AC-3: whether the run block reports the wait");
    assertEqual(waitingBlock.run?.state, "ACTIVE", "AC-3: the state of a run stopped on a consent");
    ok(
      "AC-3",
      `the consent ${APPROVAL_ALLOWED} appears inside the run block carrying the command ${JSON.stringify(APPROVAL_COMMAND)} verbatim and both answers [${optionIDs.join(", ")}], the block reports awaiting_response:true and the run stays ACTIVE while nobody has answered`,
    );

    // --- AC-5 ---------------------------------------------------------------
    // The oracle is the agent process saying it was given the message, never
    // the 202 of the route.
    const accepted = await apiJSON(
      `${view.url}/api/workspace/conversations/${conversationID}/messages`,
      postJSON({ message: MESSAGE_SENTINEL }),
      202,
    );
    assertEqual(accepted.available, true, "AC-5: whether the conversation is still available while a run of its own waits");
    // The first user frame carried the opening instruction; this is the second.
    const delivered = await conversationControl.waitFor(userFrame, 2);
    if (userFrameText(delivered) !== MESSAGE_SENTINEL) {
      throw new Error(`AC-5: the process received ${JSON.stringify(userFrameText(delivered))} instead of the sentinel`);
    }
    conversationControl.push(emit(assistantText(REPLY_SENTINEL)));
    const answered = await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.events || []).some((event) => (event.text || "").includes(REPLY_SENTINEL)),
      "the agent's answer to appear in the timeline of the conversation",
    );
    assertEqual(answered.conversation?.id, conversationID, "AC-5: the conversation that answered");
    assertEqual(answered.runs?.[0]?.awaiting_response, true, "AC-5: whether the run is still waiting after the exchange");
    ok(
      "AC-5",
      `while the run was stopped on ${APPROVAL_ALLOWED}, POST /api/workspace/conversations/${conversationID}/messages answered 202 with available:true, the agent process itself reported having been given ${JSON.stringify(MESSAGE_SENTINEL)}, its answer came back into the timeline of ${conversationID}, and the run block was still awaiting its consent`,
    );

    // --- AC-6 ---------------------------------------------------------------
    const listed = await apiJSON(`${view.url}/api/workspace/runs`);
    const entry = (listed.runs || []).find((row) => row.id === executionID);
    if (!entry) {
      throw new Error(`AC-6: the waiting run must be listed; got ${JSON.stringify(listed.runs)}`);
    }
    assertEqual(entry.awaiting_response, true, "AC-6: whether the listed entry reports the wait");
    assertEqual(entry.pending?.id, APPROVAL_ALLOWED, "AC-6: the decision the listed entry names");
    assertEqual(entry.conversation_id, conversationID, "AC-6: the conversation the listed entry points back to");
    assertEqual(entry.anchor_event_id, proposalEventID, "AC-6: the point of that conversation the listed entry points back to");
    const html = await rawGet(`${view.url}/index.html`);
    if (!html.includes('id="runs-attention"')) {
      throw new Error("AC-6: the served index.html carries no #runs-attention: the notice would have nowhere to appear");
    }
    ok(
      "AC-6",
      `GET /api/workspace/runs marks ${executionID} awaiting_response:true on ${entry.pending.id} and carries conversation_id ${entry.conversation_id} with anchor_event_id ${entry.anchor_event_id} — the conversation and the exact point the notice leads back to — while the served index.html carries #runs-attention`,
    );

    // --- AC-4, the consent granted ------------------------------------------
    // Proved on the hub: what it was really told, and what the run did next.
    await apiJSON(
      `${view.url}/api/execution/${executionID}/run/approvals/${APPROVAL_ALLOWED}`,
      postJSON({ option_id: OPTION_ALLOW }),
      202,
    );
    const allowedResponses = hub.approvalResponses();
    if (allowedResponses.length !== 1
      || allowedResponses[0].approvalId !== APPROVAL_ALLOWED
      || allowedResponses[0].optionId !== OPTION_ALLOW) {
      throw new Error(`AC-4: the hub must have recorded ("${APPROVAL_ALLOWED}","${OPTION_ALLOW}"); got ${JSON.stringify(allowedResponses)}`);
    }
    hub.emitRunEvent("Riprendo: eseguo il comando consentito");
    hub.emitRunEvent(" e ho finito di ripulire");
    const resumed = await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.runs?.[0]?.events || []).some((event) => (event.text || "").includes("comando consentito"))
        && (data.runs?.[0]?.approvals || []).length === 0,
      `the run of ${executionID} to carry on inside the conversation after the consent was granted`,
    );
    assertEqual(resumed.runs[0].awaiting_response, false, "AC-4: whether an answered run still reports a wait");
    assertEqual(resumed.runs[0].run?.state, "ACTIVE", "AC-4: the state of a run that was allowed to carry on");

    // --- AC-4, the consent denied -------------------------------------------
    hub.setApprovals([approval(APPROVAL_DENIED)]);
    await waitForConversation(
      view.url,
      conversationID,
      (data) => (data.runs?.[0]?.approvals || [])[0]?.id === APPROVAL_DENIED,
      `the second consent ${APPROVAL_DENIED} to appear inside the run block`,
    );
    await apiJSON(
      `${view.url}/api/execution/${executionID}/run/approvals/${APPROVAL_DENIED}`,
      postJSON({ option_id: OPTION_DENY }),
      202,
    );
    const denialResponses = hub.approvalResponses();
    const denial = denialResponses[denialResponses.length - 1];
    if (denialResponses.length !== 2 || denial.approvalId !== APPROVAL_DENIED || denial.optionId !== OPTION_DENY) {
      throw new Error(`AC-4: the hub must have recorded ("${APPROVAL_DENIED}","${OPTION_DENY}"); got ${JSON.stringify(denialResponses)}`);
    }
    // The refusal is the runner's to act on: the hub closes the run, exactly as
    // an agent that has been told not to would.
    hub.closeRun();
    const stopped = await waitForConversation(
      view.url,
      conversationID,
      (data) => data.runs?.[0]?.run?.state === "CLOSED",
      `the run of ${executionID} to be reported as closed inside the conversation`,
    );
    assertEqual(stopped.runs[0].awaiting_response, false, "AC-4: whether a stopped run still reports a wait");
    // The refusal stays readable where it was taken: the block is still the one
    // anchored to the proposal, on the execution the conversation decided upon.
    assertEqual(stopped.runs[0].execution_id, executionID, "AC-4: the execution the stopped block is still about");
    assertEqual(stopped.runs[0].anchor_event_id, proposalEventID, "AC-4: the point of the conversation the stopped block is still anchored to");
    ok(
      "AC-4",
      `answering ${OPTION_ALLOW} on ${APPROVAL_ALLOWED} was recorded by the hub and the run carried on inside the conversation with the events that followed; answering ${OPTION_DENY} on ${APPROVAL_DENIED} was recorded by the hub too, the run then stopped, and the conversation reports it CLOSED on the block it has kept since event ${proposalEventID}`,
    );

    // --- the credential -----------------------------------------------------
    // Not a criterion of the story, but the condition under which everything
    // above means anything: a hub that had been answering 401 would have made
    // the whole remote half a sequence of notices.
    if (!hub.authorizedCalls() || hub.unauthorizedCalls()) {
      throw new Error(
        `the remote half was not exercised under a valid credential: ${hub.authorizedCalls()} authorized and ${hub.unauthorizedCalls()} refused hub call(s)`,
      );
    }

    // --- the closing discipline of the sibling smokes ------------------------
    const closed = await apiJSON(`${view.url}/api/workspace/conversations/${conversationID}`, { method: "DELETE" }, 200);
    if (closed.conversation?.state !== "CLOSED") {
      throw new Error(`the close must report the state the session observed; got ${JSON.stringify(closed.conversation)}`);
    }
  } finally {
    if (view) await stopProcess(view.child);
    if (hub) await hub.close();
    if (runControl) await runControl.close();
    if (conversationControl) await conversationControl.close();
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
async function writeFakeWrapper(runDir, name, controlURL) {
  const file = path.join(runDir, `fake-claude-${name}.sh`);
  await fs.writeFile(
    file,
    `#!/bin/sh\nFAKE_CLAUDE_CONTROL='${controlURL}' exec '${process.execPath}' '${fakeClaudePath}' "$@"\n`,
  );
  await fs.chmod(file, 0o755);
  return file;
}

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

// --- the fake ARcipelago hub ------------------------------------------------------
//
// It serves the task routes the arcipelago client uses plus the run namespace
// the follower needs. Nothing progresses on its own: this test seeds the task,
// assigns the run, opens each approval, publishes each event and closes the run
// by hand.
//
// It is a copy of the hub of workspace-runs-view-smoke.mjs and not an import,
// by the harness's own contract: every smoke is a standalone script. What it
// adds is what this story needs and that one did not — a task seeded on an
// execution id, published run events, and a run that can be closed.

async function startFakeHub() {
  const tasks = new Map(); // task id -> record
  let created = 0;
  let authorized = 0;
  let unauthorized = 0;
  let eventID = 0;

  const run = { id: "", taskId: "", state: "active", createdAt: Date.now(), closedAt: 0, error: "" };
  const history = [];
  const approvalResponses = [];
  let approvals = [];
  let stream = null; // the currently attached SSE response, at most one

  const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://127.0.0.1");
    if (req.headers.authorization !== `Bearer ${TOKEN_SENTINEL}`) {
      unauthorized += 1;
      return sendJSON(res, 401, { error: "unauthorized" });
    }
    authorized += 1;

    if (req.method === "GET" && url.pathname === "/api/external/tasks/by-reference") {
      const externalID = url.searchParams.get("externalId");
      const match = [...tasks.values()].find((task) => task.request.externalId === externalID);
      if (!match) return sendJSON(res, 404, { error: "task_not_found" });
      return sendJSON(res, 200, { task: publicTask(match) });
    }

    if (req.method === "GET" && url.pathname.startsWith("/api/external/tasks/")) {
      const id = decodeURIComponent(url.pathname.slice("/api/external/tasks/".length));
      const record = tasks.get(id);
      if (!record) return sendJSON(res, 404, { error: "task_not_found" });
      return sendJSON(res, 200, { task: publicTask(record) });
    }

    const runRoute = /^\/api\/external\/runs\/([^/]+)(\/.*)?$/.exec(url.pathname);
    if (runRoute) {
      const runID = decodeURIComponent(runRoute[1]);
      const rest = runRoute[2] || "";
      if (!run.id || runID !== run.id) {
        return sendJSON(res, 404, { error: "run_not_found" });
      }
      if (req.method === "GET" && rest === "") {
        return sendJSON(res, 200, {
          run: {
            id: run.id,
            runnerId: "runner-1",
            taskId: run.taskId,
            state: run.state,
            createdAt: run.createdAt,
            closedAt: run.closedAt,
            error: run.error,
          },
        });
      }
      if (req.method === "GET" && rest === "/approvals") {
        return sendJSON(res, 200, { approvals });
      }
      if (req.method === "GET" && rest === "/events") {
        return openStream(req, res, url);
      }
      const respondRoute = /^\/approvals\/([^/]+)\/respond$/.exec(rest);
      if (req.method === "POST" && respondRoute) {
        const approvalID = decodeURIComponent(respondRoute[1]);
        return readBody(req).then((raw) => {
          let body = {};
          try {
            body = JSON.parse(raw || "{}");
          } catch {
            return sendJSON(res, 400, { error: "invalid_json" });
          }
          approvalResponses.push({ runId: runID, approvalId: approvalID, optionId: body.optionId });
          approvals = approvals.filter((approval) => approval.id !== approvalID);
          return sendJSON(res, 202, { ok: true });
        });
      }
      return sendJSON(res, 404, { error: "not_found" });
    }

    return sendJSON(res, 404, { error: "not_found" });
  });

  // openStream serves the SSE history from the cursor the subscriber asked for,
  // and holds the socket open while the run is active.
  function openStream(req, res, url) {
    const afterParam = url.searchParams.get("afterId");
    const headerCursor = req.headers["last-event-id"];
    const afterId = Number.parseInt(afterParam ?? headerCursor ?? "0", 10) || 0;

    res.writeHead(200, {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    });
    for (const frame of history) {
      if (frame.id > afterId) res.write(frameText(frame));
    }
    if (run.state !== "active") {
      res.write(endFrameText("closed"));
      res.end();
      return undefined;
    }
    stream = res;
    req.on("close", () => {
      if (stream === res) stream = null;
    });
    return undefined;
  }

  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const url = `http://127.0.0.1:${server.address().port}`;

  // seedTask is how a run born on another provider becomes followable here: the
  // hub is told about a task whose externalId is the execution id, which is the
  // identity the client resolves a still-running record by.
  function seedTask(externalID, specCode) {
    created += 1;
    const id = `task-${created}`;
    const record = {
      id,
      status: "running",
      resultSummary: "",
      runId: "",
      specCode,
      request: { externalId: externalID, workspaceId: WORKSPACE_ID },
    };
    tasks.set(id, record);
    return record;
  }

  // assignRun is what turns an observable task into a followable run: until the
  // task carries a runId, ResolveRun has nothing to resolve.
  function assignRun(taskID) {
    const record = tasks.get(taskID);
    if (!record) throw new Error(`no remote task ${taskID} to bind a run to`);
    record.runId = RUN_ID;
    run.id = RUN_ID;
    run.taskId = taskID;
    run.state = "active";
    run.createdAt = Date.now();
    return run;
  }

  return {
    url,
    seedTask,
    assignRun,
    setApprovals: (list) => {
      approvals = list;
    },
    // emitRunEvent publishes one frame of the run's history, to the attached
    // stream when there is one and to the replayable history either way.
    emitRunEvent: (text) => {
      eventID += 1;
      const frame = {
        id: eventID,
        runId: run.id,
        seq: 1,
        ts: Date.now(),
        event: { type: "message_update", assistantMessageEvent: { type: "text_delta", delta: text } },
      };
      history.push(frame);
      if (stream) stream.write(frameText(frame));
      return frame;
    },
    // closeRun is the runner acting on a refusal: the run ends, and the stream
    // says so instead of merely dropping.
    closeRun: (reason = "closed") => {
      run.state = "closed";
      run.closedAt = Date.now();
      if (stream) {
        const res = stream;
        stream = null;
        res.write(endFrameText(reason));
        res.end();
      }
    },
    approvalResponses: () => approvalResponses.map((entry) => ({ ...entry })),
    authorizedCalls: () => authorized,
    unauthorizedCalls: () => unauthorized,
    close: () =>
      new Promise((resolve) => {
        if (stream) {
          const res = stream;
          stream = null;
          res.destroy();
        }
        server.closeAllConnections?.();
        server.close(() => resolve());
      }),
  };
}

function publicTask(record) {
  return { id: record.id, status: record.status, resultSummary: record.resultSummary, runId: record.runId };
}

// approval is the consent the run asks for, in the hub's own vocabulary. The
// command travels inside args because that is what the person has to read
// before answering.
function approval(id) {
  return {
    id,
    runId: RUN_ID,
    runnerId: "runner-1",
    createdAt: Date.now(),
    request: {
      toolName: "Bash",
      title: "Ripulire i worktree",
      args: { command: APPROVAL_COMMAND },
      options: [
        { optionId: OPTION_ALLOW, name: "Consenti", kind: "allow" },
        { optionId: OPTION_DENY, name: "Rifiuta", kind: "reject" },
      ],
    },
  };
}

function frameText(frame) {
  return `id: ${frame.id}\nevent: run_event\ndata: ${JSON.stringify(frame)}\n\n`;
}

function endFrameText(reason) {
  return `event: end\ndata: ${JSON.stringify({ reason })}\n\n`;
}

function sendJSON(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Content-Length": Buffer.byteLength(body) });
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
  const runsRoot = path.join(root, "runs");
  await fs.mkdir(runsRoot, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const runDir = path.join(runsRoot, stamp);
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
  await waitForHTTP(`${url}/api/board`);
  return { child, url };
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
  if (!child || child.killed) return;
  child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    delay(3000),
  ]);
  if (!child.killed) child.kill("SIGKILL");
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
    proposal is confirmed, and the run it starts is followed from then on through
    <code>GET /api/workspace/conversations/{id}</code> alone — this smoke records every request it makes and asserts
    that it never called <code>GET /api/execution/{id}/run</code> to learn that the run was growing. The
    workspace default is then moved to the <code>arcipelago</code> provider pointed at a fake local hub, which
    opens two consents on that same run: the first is granted and the run carries on, the second is refused and
    the run stops. What the hub was really told is the oracle for both. While the run is stopped, a message is
    sent in the same conversation and the agent process itself reports having been given it.</p>

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
