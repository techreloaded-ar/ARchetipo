#!/usr/bin/env node

// End-to-end smoke for "talk to the agent about the open workspace" (US-053).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector, the local `claude` provider, its
// stream-json client, the bounded local session behind a conversation and the
// viewer routes addressed by id (`POST` and `GET` on
// `/api/workspace/conversations`, `GET`, `POST .../messages` and `DELETE` on
// `/api/workspace/conversations/{id}`). Only the agent binary is replaced, by a Node
// script that speaks the same protocol on stdio, so the conversation needs no
// credential and no network.
//
// The oracles are deliberately the ones no viewer field can stand in for: the
// working directory the agent process itself reports at startup — its real
// `cmd.Dir` — the frames it was really given, the execution records that do or
// do not exist on the filesystem, and the end of the process, asked of the
// operating system by pid rather than believed from the route that closed it.
//
// The fake never progresses on its own: every frame it emits is commanded
// through a local control server, and every frame it receives is reported back
// to that server. There is no arbitrary sleep anywhere — each wait polls a
// viewer route or the control server with an explicit timeout that names what
// it was waiting for and what arrived instead.

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
const fakeCodexPath = path.join(__dirname, "support", "fake-codex.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "conversation-view-smoke");

const CODE_A = "US-A01";
const CODE_B = "US-B01";
const CODE_C = "US-C01";

// The sentinel stands for whatever authentication material lives in the
// viewer's environment. The agent owns its own authentication, so nothing of it
// may ever reach a viewer response or a workspace configuration.
const AUTH_SENTINEL = "claude-conversation-session-material-DO-NOT-EXPOSE";
const MESSAGE_SENTINEL = "smoke-operator-conversation-message-sentinel";

// The retention window a conversation keeps in memory, mirrored from
// `conversationRetainedEvents` in cli/internal/execution/claude/conversation.go.
// The flood below has to be larger than it, or the history would never be
// partial and the claim would be untestable.
const RETAINED_EVENTS = 2000;
const FLOOD_FRAMES = RETAINED_EVENTS;

// Every viewer response body is kept, so the final check can prove no session
// material travelled to the browser on any route this run touched.
const viewerBodies = [];

// One entry per proved statement, for the report.
const checks = [];

function ok(criterion, statement) {
  checks.push({ criterion, statement });
  console.log(`-> ${criterion} ok: ${statement}`);
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake binaries rely on a POSIX shebang");
    return;
  }
  const runDir = await createRunDir(options.workspaceRoot);
  console.log(`-> run directory: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const startedAt = Date.now();
  let failure = null;
  try {
    // Every process started from here writes the registry of known workspaces
    // inside the run directory, never in the real state of the machine.
    const env = {
      ...process.env,
      ARCHETIPO_DATA_DIR: repoRoot,
      ARCHETIPO_STATE_DIR: path.join(runDir, "state"),
      CLAUDE_FAKE_AUTH: AUTH_SENTINEL,
    };
    const targetsDir = path.join(runDir, "targets");
    await fs.mkdir(targetsDir, { recursive: true });

    const dirA = await createWorkspace(runDir, targetsDir, "alfa", "claude", CODE_A, env);
    const dirB = await createWorkspace(runDir, targetsDir, "beta", "claude", CODE_B, env);
    const dirC = await createWorkspace(runDir, targetsDir, "gamma", "codex", CODE_C, env);

    await scenarioNotOfferedWithoutTheCapability(dirC, env);
    await scenarioConversationOfTheOpenWorkspace(dirA, dirB, env);
    await assertNoSessionMaterialLeaked([dirA, dirB, dirC]);
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
  console.log(`\nPASS: conversation-view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// --- AC-4 -------------------------------------------------------------------
//
// A provider that is perfectly available and simply does not hold conversations
// must not be asked to. The workspace default is the real `codex` provider
// pointed at its own fake, which answers `--version` like the real binary: the
// runtime is usable, and the capability is the only thing missing.
async function scenarioNotOfferedWithoutTheCapability(dirC, env) {
  let view;
  let control;
  try {
    control = await startControlServer();
    // Both fakes are pointed at this server on purpose: it is what makes "no
    // agent process was started" an observation and not an assumption.
    view = await startViewServer(dirC, {
      ...env,
      FAKE_CODEX_CONTROL: control.url,
      FAKE_CLAUDE_CONTROL: control.url,
    });
    console.log(`-> view ready on the codex workspace: ${view.url}`);

    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "codex",
      config: { command: fakeCodexPath, timeout_seconds: 600 },
    }));

    const providers = await apiJSON(`${view.url}/api/execution/providers`);
    const codex = (providers.providers || []).find((entry) => entry.id === "codex");
    if (!codex) {
      throw new Error(`AC-4: the codex provider is not listed: ${JSON.stringify(providers.providers)}`);
    }
    if (codex.available !== true) {
      throw new Error(`AC-4: the codex runtime must be usable for this case to mean anything; got ${JSON.stringify(codex)}`);
    }
    if ((codex.capabilities || []).includes("workspace.converse")) {
      throw new Error(`AC-4: codex must not declare workspace.converse; got ${JSON.stringify(codex.capabilities)}`);
    }

    const listingBefore = await listRecursively(dirC);

    const offered = await apiJSON(`${view.url}/api/workspace/conversations`, {}, 200);
    if (offered.available !== false) {
      throw new Error(`AC-4: the conversation must not be offered; got ${JSON.stringify(offered)}`);
    }
    if ((offered.conversations || []).length !== 0) {
      throw new Error(`AC-4: a workspace that is not offered a conversation holds none; got ${JSON.stringify(offered)}`);
    }
    if (offered.provider_id !== "codex") {
      throw new Error(`AC-4: the refusal must name the provider it is about; got ${JSON.stringify(offered.provider_id)}`);
    }
    if (!String(offered.unavailable_reason || "").includes("workspace.converse")) {
      throw new Error(`AC-4: the reason must name workspace.converse; got ${JSON.stringify(offered.unavailable_reason)}`);
    }

    const refused = await expectStatus(`${view.url}/api/workspace/conversations`, 409, postJSON({}));
    if (String(refused.error || "") !== String(offered.unavailable_reason || "")) {
      throw new Error(
        `AC-4: pressing the button must be refused with the very sentence the payload declared\n  read:    ${JSON.stringify(offered.unavailable_reason)}\n  refusal: ${JSON.stringify(refused.error)}`,
      );
    }

    if (control.reports().length !== 0) {
      throw new Error(
        `AC-4: no agent process may be started for a conversation that is not offered; the control server saw ${JSON.stringify(control.reports().map((entry) => entry.kind))}`,
      );
    }
    await assertSameListing("AC-4", listingBefore, await listRecursively(dirC), dirC);
    ok(
      "AC-4",
      `with the available codex provider as default, GET /api/workspace/conversations answers 200 available:false stating ${JSON.stringify(truncate(offered.unavailable_reason))}, POST answers 409 with the identical sentence, no agent process was ever started and the ${listingBefore.length} paths of the sandbox are unchanged`,
    );
  } finally {
    if (view) await stopProcess(view.child);
    if (control) await control.close();
  }
}

// --- AC-1, AC-2, AC-3, AC-5, AC-6 -------------------------------------------
//
// One viewer, one PID, two real workspaces: everything the story claims about a
// conversation is a claim about the same process serving A and then B, so it is
// proved on one process and never on a restart.
async function scenarioConversationOfTheOpenWorkspace(dirA, dirB, env) {
  const realA = await fs.realpath(dirA);
  const realB = await fs.realpath(dirB);

  let view;
  let control;
  try {
    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);
    view = await startViewServer(dirA, { ...env, FAKE_CLAUDE_CONTROL: control.url });
    const pid = view.child.pid;
    console.log(`-> view ready: ${view.url} (pid ${pid}, launched in ${dirA})`);

    // B is registered now and opened later, on this very process.
    await expectStatus(`${view.url}/api/workspaces`, 201, postJSON({ path: dirB }));
    const known = await apiJSON(`${view.url}/api/workspaces`);
    const entryB = await findByRealPath(known.workspaces, realB);
    if (!entryB) {
      throw new Error(`both workspaces must be known: ${JSON.stringify(known.workspaces)}`);
    }

    // The provider is configured through the very route the Execution panel
    // uses, so the conversation starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: fakeClaudePath, timeout_seconds: 600 },
    }));

    // --- AC-2, the half that has to be captured before anything happens ------
    const boardBefore = await rawGet(`${view.url}/api/board`);
    const actionsBefore = await apiJSON(`${view.url}/api/workspace/actions`);
    if (actionsBefore.execution !== null) {
      throw new Error(`AC-2: the workspace must hold no execution before the conversation; got ${JSON.stringify(actionsBefore.execution)}`);
    }

    // --- AC-1 ---------------------------------------------------------------
    // The process announces itself before the open call can return, so the
    // frame is queued before the request that starts the process.
    control.push(emit({ type: "system", subtype: "init", session_id: "conversation-a" }));
    const opened = await apiJSON(`${view.url}/api/workspace/conversations`, postJSON({}), 201);
    if (opened.available !== true || !opened.conversation?.id) {
      throw new Error(`AC-1: unexpected payload on open: ${JSON.stringify(opened)}`);
    }
    const conversationA = opened.conversation.id;
    if (!conversationA.startsWith("conv-")) {
      throw new Error(`AC-1: a conversation id must not be mistakable for an execution id; got ${JSON.stringify(conversationA)}`);
    }
    await assertSamePath(opened.conversation.working_dir, dirA, "AC-1: the directory the conversation reports");

    const invocation = await control.waitFor("argv", 1);
    const startedIn = await fs.realpath(invocation.cwd);
    if (startedIn !== realA) {
      throw new Error(`AC-1: the agent process was started in ${invocation.cwd}, want the project root of the open workspace ${dirA}`);
    }
    if (startedIn === realB) {
      throw new Error("AC-1: the agent process was started in the workspace that is not open");
    }
    assertStreamingInvocation(invocation.argv || []);

    control.push(emit(assistantText("Ciao: leggo il workspace e rispondo.")));
    const first = await waitForConversation(
      view.url,
      conversationA,
      0,
      (data) => (data.events || []).length === 1,
      "the assistant frame to become one text event",
    );
    if (first.events[0].kind !== "text" || !first.events[0].text.includes("leggo il workspace")) {
      throw new Error(`AC-1: the frame was not translated into a text event; got ${JSON.stringify(first.events[0])}`);
    }

    const accepted = await apiJSON(`${view.url}/api/workspace/conversations/${conversationA}/messages`, postJSON({ message: MESSAGE_SENTINEL }), 202);
    if (JSON.stringify(accepted).includes(MESSAGE_SENTINEL)) {
      throw new Error("AC-1: the accepted message must not be echoed into the history before the process re-emits it");
    }
    // The first user frame carried the opening instruction; the operator's
    // message is the second one the process is given.
    const steered = await control.waitFor(userFrame, 2);
    if (userFrameText(steered) !== MESSAGE_SENTINEL) {
      throw new Error(`AC-1: the process received ${JSON.stringify(userFrameText(steered))} instead of the sentinel`);
    }
    const stillOne = await readConversation(view.url, conversationA, 0);
    if ((stillOne.events || []).length !== 1) {
      throw new Error(`AC-1: the history grew while the process merely held the message; got ${JSON.stringify(stillOne.events)}`);
    }
    control.push(emit({ type: "user", message: { content: [{ type: "text", text: MESSAGE_SENTINEL }] }, isReplay: true }));
    const two = await waitForConversation(
      view.url,
      conversationA,
      0,
      (data) => (data.events || []).length === 2,
      "the re-emitted message to enter the history",
    );
    const echoed = two.events.filter((event) => event.kind === "user_message" && event.text === MESSAGE_SENTINEL);
    if (echoed.length !== 1) {
      throw new Error(`AC-1: the re-emitted message must appear once; found ${echoed.length} in ${JSON.stringify(two.events)}`);
    }
    ok(
      "AC-1",
      `the agent process was started in ${dirA} — the working directory it reports itself — under the streaming invocation, the assistant frame became a text event, and the message reached the process as a user frame, was absent from the 202 body and entered the history exactly once only after the process re-emitted it`,
    );

    // --- AC-2 ---------------------------------------------------------------
    const boardAfter = await rawGet(`${view.url}/api/board`);
    if (boardAfter !== boardBefore) {
      throw new Error(`AC-2: the board changed across the conversation\n  before: ${truncate(boardBefore, 400)}\n  after:  ${truncate(boardAfter, 400)}`);
    }
    const actionsAfter = await apiJSON(`${view.url}/api/workspace/actions`);
    if (actionsAfter.execution !== null) {
      throw new Error(`AC-2: a conversation became an action of the process; got ${JSON.stringify(actionsAfter.execution)}`);
    }
    const records = await listExecutionRecords(dirA);
    if (records.length !== 0) {
      throw new Error(`AC-2: the conversation left ${records.length} execution record(s): ${JSON.stringify(records)}`);
    }
    ok(
      "AC-2",
      `after opening, answering and exchanging a message the board payload is byte-identical, GET /api/workspace/actions still answers execution: null and .archetipo/executions/ holds no record at all`,
    );

    // --- AC-3, the reload ---------------------------------------------------
    assertEqual(view.child.pid, pid, "the viewer PID at the reload");
    assertAlive(view.child, "the viewer process at the reload");
    const reloaded = await readConversation(view.url, conversationA, 0);
    if (reloaded.conversation?.id !== conversationA) {
      throw new Error(`AC-3: the reload lost the conversation; got ${JSON.stringify(reloaded.conversation)}`);
    }
    if (reloaded.truncated !== false || reloaded.notice) {
      throw new Error(`AC-3: a whole history must not declare itself partial; got ${JSON.stringify({ truncated: reloaded.truncated, notice: reloaded.notice })}`);
    }
    assertEventIDs(reloaded.events, [1, 2], "AC-3 the reloaded history");
    if (reloaded.last_id !== 2) {
      throw new Error(`AC-3: the cursor of the reloaded history is ${reloaded.last_id}, want 2`);
    }

    // --- a second conversation, and the refusal that must change nothing -----
    const beforeRefusals = await rawGet(`${view.url}/api/workspace/conversations/${conversationA}?after_id=0`);
    // A workspace holds several conversations at once, so a second open is
    // granted beside the first instead of being refused because one is already
    // there. What happens beyond the limit is not this smoke's subject.
    // The open answers only once the new agent process has announced itself,
    // and two processes now share this control server, so the announcement is
    // pushed on a timer until the open has answered instead of once: whichever
    // of the two takes a given frame, the new one gets the next. An extra
    // `system`/`init` frame produces no event in any conversation, and the
    // leftovers are drained rather than left for whoever polls next.
    const announcing = setInterval(
      () => control.push(emit({ type: "system", subtype: "init", session_id: "conversation-a-second" })),
      25,
    );
    let secondOpen;
    try {
      secondOpen = await apiJSON(`${view.url}/api/workspace/conversations`, postJSON({}), 201);
    } finally {
      clearInterval(announcing);
      control.drain();
    }
    const conversationA2 = secondOpen.conversation?.id;
    if (!conversationA2 || conversationA2 === conversationA) {
      throw new Error(`AC-3: a second open must answer with a conversation of its own; got ${JSON.stringify(secondOpen.conversation)}`);
    }
    // It is closed again straight away, so the rest of the scenario keeps
    // talking to a single agent process: the control server is shared by every
    // fake, and two live ones would race for the frames pushed to it.
    const secondInvocation = await control.waitFor("argv", 2);
    const secondClosed = await apiJSON(
      `${view.url}/api/workspace/conversations/${conversationA2}?after_id=0`,
      { method: "DELETE" },
      200,
    );
    if (secondClosed.conversation?.id !== conversationA2) {
      throw new Error(`AC-3: the close must answer about the conversation it closed; got ${JSON.stringify(secondClosed.conversation)}`);
    }
    await waitForProcessGone(
      secondInvocation.pid,
      `the agent process of the second conversation (pid ${secondInvocation.pid}) to be released by its close`,
    );
    const unknownWorkspace = await expectStatus(
      `${view.url}/api/workspaces/${encodeURIComponent("does-not-exist")}/open`,
      404,
      postJSON({}),
    );
    if (!String(unknownWorkspace.error || "").trim()) {
      throw new Error(`AC-3: the refusal on an unknown workspace must state a reason; got ${JSON.stringify(unknownWorkspace)}`);
    }
    const afterRefusals = await rawGet(`${view.url}/api/workspace/conversations/${conversationA}?after_id=0`);
    if (afterRefusals !== beforeRefusals) {
      throw new Error(`AC-3: a second conversation and a refused command changed the projection of the first\n  before: ${truncate(beforeRefusals, 400)}\n  after:  ${truncate(afterRefusals, 400)}`);
    }

    // --- AC-3, the partial history ------------------------------------------
    // More frames than the window retains, in one command: the fake still
    // progresses only when it is told to, and the burst is one telling.
    const flood = [];
    for (let i = 1; i <= FLOOD_FRAMES; i += 1) flood.push(assistantText(`riga ${i}`));
    control.push({ kind: "emit", frames: flood });
    const total = 2 + FLOOD_FRAMES;
    await waitForConversation(
      view.url,
      conversationA,
      total - 1,
      (data) => (data.events || []).length === 1 && data.events[0].id === total,
      `the last of ${FLOOD_FRAMES} flooded frames (event ${total})`,
    );
    const partial = await readConversation(view.url, conversationA, 0);
    if (partial.truncated !== true) {
      throw new Error(`AC-3: a cursor now outside the window must be answered truncated; got ${JSON.stringify({ truncated: partial.truncated, first: partial.events?.[0]?.id, count: partial.events?.length })}`);
    }
    if (!String(partial.notice || "").trim()) {
      throw new Error("AC-3: a partial history must carry a non-empty notice");
    }
    if (partial.events.length !== RETAINED_EVENTS) {
      throw new Error(`AC-3: the window must keep exactly ${RETAINED_EVENTS} events; got ${partial.events.length}`);
    }
    const dropped = total - RETAINED_EVENTS;
    if (partial.events[0].id !== dropped + 1) {
      throw new Error(`AC-3: the surviving history must begin at event ${dropped + 1}; got ${partial.events[0].id}`);
    }
    assertStrictlyIncreasing(partial.events, "AC-3 the partial history");
    if (partial.events[partial.events.length - 1].id !== total || partial.last_id !== total) {
      throw new Error(`AC-3: the newest event must be ${total}; got ${JSON.stringify({ last: partial.events[partial.events.length - 1].id, last_id: partial.last_id })}`);
    }
    ok(
      "AC-3",
      `the same viewer process (pid ${pid}) re-read the whole history with truncated:false, a second conversation ${conversationA2} opened beside it with 201 and was closed again while a refused command answered 404, all three leaving the projection of ${conversationA} byte-identical, and once ${total} events had been produced the read from a cursor now outside the ${RETAINED_EVENTS}-event window answered truncated:true beginning at event ${partial.events[0].id} with the notice ${JSON.stringify(truncate(partial.notice, 120))}`,
    );

    // --- AC-5 ---------------------------------------------------------------
    await expectStatus(`${view.url}/api/workspaces/${encodeURIComponent(entryB.id)}/open`, 200, postJSON({}));
    assertEqual(view.child.pid, pid, "the viewer PID after opening B");
    assertAlive(view.child, "the viewer process after opening B");

    // The conversation of A is asked for by its own id, from B: the workspace
    // that never held it does not know it, so the read answers 404 rather than
    // serving a conversation — and a history — that belongs to somewhere else.
    const inB = await expectStatus(
      `${view.url}/api/workspace/conversations/${encodeURIComponent(conversationA)}?after_id=0`,
      404,
    );
    if (!String(inB.error || "").trim()) {
      throw new Error(`AC-5: the refusal on the conversation of A must state a reason; got ${JSON.stringify(inB)}`);
    }
    if (inB.events || inB.conversation) {
      throw new Error(`AC-5: B was served the conversation of A; got ${JSON.stringify(inB)}`);
    }
    // The switch is also what released the process of A: the oracle is the
    // operating system, asked whether that process is still there.
    await waitForProcessGone(invocation.pid, `the agent process of A (pid ${invocation.pid}) to be released by the workspace switch`);

    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: fakeClaudePath, timeout_seconds: 600 },
    }));
    control.push(emit({ type: "system", subtype: "init", session_id: "conversation-b" }));
    const openedInB = await apiJSON(`${view.url}/api/workspace/conversations`, postJSON({}), 201);
    const conversationB = openedInB.conversation?.id;
    if (!conversationB || conversationB === conversationA) {
      throw new Error(`AC-5: B must open a conversation of its own; got ${JSON.stringify(openedInB.conversation)}`);
    }
    await assertSamePath(openedInB.conversation.working_dir, dirB, "AC-5: the directory the conversation of B reports");
    const invocationB = await control.waitFor("argv", 3);
    const startedInB = await fs.realpath(invocationB.cwd);
    if (startedInB !== realB) {
      throw new Error(`AC-5: the second agent process was started in ${invocationB.cwd}, want ${dirB}`);
    }
    ok(
      "AC-5",
      `after POST /api/workspaces/{B}/open on the same pid ${pid} the read of ${conversationA} from B answers 404 with a reason and no conversation and no history, the agent process of A was released, and the conversation opened in B starts its own process in ${dirB}`,
    );

    // --- AC-6 ---------------------------------------------------------------
    control.push(emit(assistantText("Sono la conversazione di beta.")));
    await waitForConversation(view.url, conversationB, 0, (data) => (data.events || []).length === 1, "the history of the conversation of B");

    const closed = await apiJSON(`${view.url}/api/workspace/conversations/${conversationB}?after_id=0`, { method: "DELETE" }, 200);
    if (closed.conversation?.id !== conversationB) {
      throw new Error(`AC-6: the close must answer about the conversation it closed; got ${JSON.stringify(closed.conversation)}`);
    }
    if (closed.conversation.state !== "CLOSED") {
      throw new Error(`AC-6: the state must be the one the session observed; got ${JSON.stringify(closed.conversation.state)}`);
    }
    if ((closed.events || []).length !== 1) {
      throw new Error(`AC-6: the operator who closed a conversation may still read what was said; got ${JSON.stringify(closed.events)}`);
    }
    await waitForProcessGone(invocationB.pid, `the agent process of B (pid ${invocationB.pid}) to be released by the close`);

    const beforeClosedRefusals = await rawGet(`${view.url}/api/workspace/conversations/${conversationB}?after_id=0`);
    const afterClose = await expectStatus(
      `${view.url}/api/workspace/conversations/${conversationB}/messages`,
      409,
      postJSON({ message: "sei ancora lì?" }),
    );
    if (!String(afterClose.error || "").trim()) {
      throw new Error(`AC-6: a message on a closed conversation must be refused with a reason; got ${JSON.stringify(afterClose)}`);
    }
    const secondClose = await expectStatus(`${view.url}/api/workspace/conversations/${conversationB}`, 409, { method: "DELETE" });
    if (!String(secondClose.error || "").trim()) {
      throw new Error(`AC-6: a second close must be refused with a reason; got ${JSON.stringify(secondClose)}`);
    }
    const afterClosedRefusals = await rawGet(`${view.url}/api/workspace/conversations/${conversationB}?after_id=0`);
    if (afterClosedRefusals !== beforeClosedRefusals) {
      throw new Error(`AC-6: two refused commands changed the projection\n  before: ${truncate(beforeClosedRefusals, 400)}\n  after:  ${truncate(afterClosedRefusals, 400)}`);
    }
    if (view.child.pid !== pid) {
      throw new Error("AC-6: the viewer restarted");
    }
    ok(
      "AC-6",
      `DELETE answered 200 with the conversation still readable and its state CLOSED, the operating system reports the agent process gone, a later message and a second close both answered 409 with a reason, and the projection stayed byte-identical across both refusals`,
    );

    // The whole story happened on one viewer process.
    assertEqual(view.child.pid, pid, "the viewer PID at the end of the scenario");
    assertAlive(view.child, "the viewer process at the end of the scenario");
  } finally {
    if (view) await stopProcess(view.child);
    if (control) await control.close();
  }
}

// --- cross-cutting ----------------------------------------------------------

async function assertNoSessionMaterialLeaked(dirs) {
  const leaked = viewerBodies.filter((body) => body.includes(AUTH_SENTINEL));
  if (leaked.length) {
    throw new Error(`the viewer echoed the session material in ${leaked.length} response(s)`);
  }
  for (const dir of dirs) {
    const config = await fs.readFile(path.join(dir, ".archetipo", "config.yaml"), "utf8");
    if (config.includes(AUTH_SENTINEL) || /claude_home|CLAUDE_HOME|api_key|API_KEY/.test(config)) {
      throw new Error(`the configuration of ${dir} carries agent session material:\n${config}`);
    }
  }
  ok(
    "AC-1..AC-6",
    `the session material is absent from all ${viewerBodies.length} viewer responses and from every .archetipo/config.yaml`,
  );
}

// --- oracles ----------------------------------------------------------------

// assertStreamingInvocation checks how the session was opened, because for
// stream-json there is no `initialize` call: the streaming flags are what make a
// live dialogue possible at all.
function assertStreamingInvocation(argv) {
  for (const flag of ["--print", "--verbose", "--replay-user-messages", "--no-session-persistence"]) {
    if (!argv.includes(flag)) {
      throw new Error(`AC-1: the session was not opened as configured, ${flag} is missing: ${JSON.stringify(argv)}`);
    }
  }
  for (const [flag, value] of [["--input-format", "stream-json"], ["--output-format", "stream-json"]]) {
    if (argv[argv.indexOf(flag) + 1] !== value) {
      throw new Error(`AC-1: the session was not opened as configured, ${flag} is not ${value}: ${JSON.stringify(argv)}`);
    }
  }
}

function assertEventIDs(events, expected, label) {
  const ids = (events || []).map((event) => event.id);
  if (ids.join(",") !== expected.join(",")) {
    throw new Error(`${label}: expected the ordered ids [${expected.join(",")}]; got [${ids.join(",")}]`);
  }
}

function assertStrictlyIncreasing(events, label) {
  for (let i = 1; i < events.length; i += 1) {
    if (events[i].id <= events[i - 1].id) {
      throw new Error(`${label}: the ids must strictly increase; ${events[i - 1].id} is followed by ${events[i].id}`);
    }
  }
}

// waitForProcessGone polls the operating system until the agent process is no
// longer there. `kill(pid, 0)` sends nothing: it asks whether the process
// exists, and it is the only oracle that answers "the provider really released
// it" instead of "the route said so". The process is a child of the viewer and
// is reaped by the provider that waited for it, so a zombie never keeps
// answering here.
async function waitForProcessGone(pid, what, timeoutMs = 20000) {
  if (!Number.isInteger(pid)) {
    throw new Error(`the fake did not report the pid of its own process; got ${JSON.stringify(pid)}`);
  }
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    try {
      process.kill(pid, 0);
    } catch (error) {
      if (error.code === "ESRCH") return;
      throw new Error(`could not ask the operating system about pid ${pid}: ${error.message}`);
    }
    await delay(50);
  }
  throw new Error(`Timed out after ${timeoutMs}ms waiting for ${what}; the process is still alive`);
}

async function assertSamePath(actual, expected, label) {
  if (typeof actual !== "string" || !actual) {
    throw new Error(`${label} is missing: ${JSON.stringify(actual)}`);
  }
  const [a, b] = [await realpathOrSelf(actual), await realpathOrSelf(expected)];
  if (a !== b) {
    throw new Error(`${label} is ${actual}, want ${expected}`);
  }
}

// realpathOrSelf resolves a path when it exists, because on macOS /var is a
// symlink to /private/var and the two spellings name the same place.
async function realpathOrSelf(value) {
  try {
    return await fs.realpath(value);
  } catch {
    return value;
  }
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`Unexpected ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

function assertAlive(child, label) {
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(`${label}: the process is gone (exit ${child.exitCode}, signal ${child.signalCode})`);
  }
}

async function assertSameListing(criterion, before, after, dir) {
  if (JSON.stringify(before) !== JSON.stringify(after)) {
    const added = after.filter((entry) => !before.includes(entry));
    const removed = before.filter((entry) => !after.includes(entry));
    throw new Error(`${criterion}: ${dir} changed: added ${JSON.stringify(added)}, removed ${JSON.stringify(removed)}`);
  }
}

// listRecursively is the coarse oracle of "nothing was written": every path
// under a directory, relative and sorted.
async function listRecursively(root) {
  const out = [];
  const walk = async (current, prefix) => {
    const entries = await fs.readdir(current, { withFileTypes: true }).catch(() => []);
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      const rel = path.join(prefix, entry.name);
      out.push(rel);
      if (entry.isDirectory()) await walk(path.join(current, entry.name), rel);
    }
  };
  await walk(root, "");
  return out.sort();
}

async function listExecutionRecords(root) {
  const entries = await fs.readdir(path.join(root, ".archetipo", "executions")).catch(() => []);
  return entries.filter((name) => name.endsWith(".json")).sort();
}

// --- the protocol -----------------------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

function assistantText(text) {
  return { type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text }] } };
}

// userFrame recognizes a frame the process was given on its standard input as an
// operator message, which is the shape the opening instruction and every later
// message share.
function userFrame(entry) {
  return entry.kind === "received" && entry.frame?.type === "user";
}

function userFrameText(entry) {
  return (entry.frame?.message?.content || []).map((block) => block.text || "").join("");
}

// --- the control server ------------------------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received, so every assertion above is a statement
// about a state the test produced. Several fake processes may share it — at most
// one is ever alive at a time here, because closing a conversation waits for its
// process — and each report carries the working directory of the process that
// made it.

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
    reports() {
      return received;
    },
    // drain throws away the commands nobody consumed. It exists for the one
    // moment two agent processes are alive at once: the announcement the new one
    // has to make cannot be pushed a single time, because the process already
    // live would take it, so it is pushed until the open has answered and what
    // is left over is dropped here rather than delivered to whoever polls next.
    drain() {
      commands.length = 0;
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

// --- fixtures ----------------------------------------------------------------

// createWorkspace initializes one real workspace with a backlog holding a single
// recognisable spec code, so which workspace is being served is never in doubt.
async function createWorkspace(runDir, targetsDir, name, tool, code, env) {
  const dir = path.join(targetsDir, name);
  await fs.mkdir(dir, { recursive: true });
  await runCommand(`init-${name}`, cliPath, ["init", "--tool", tool, "--connector", "file", "--yes"], { cwd: dir, env });

  const specsFile = path.join(runDir, `specs-${name}.json`);
  await fs.writeFile(specsFile, JSON.stringify({
    specs: [{
      code,
      title: `Smoke ${name}`,
      epic: { code: "EP-999", title: "Smoke tests" },
      priority: "LOW",
      points: 1,
      status: "TODO",
      body: `Story di test del workspace ${name}.`,
    }],
  }, null, 2));
  await runCommand(`spec-add-${name}`, cliPath, ["spec", "add", "--file", specsFile], { cwd: dir, env });
  return dir;
}

// findByRealPath compares through fs.realpath on both sides, because on macOS
// /var is a symlink to /private/var and the two spellings are the same place.
async function findByRealPath(entries, target) {
  for (const entry of entries || []) {
    try {
      if ((await fs.realpath(entry.path)) === target) return entry;
    } catch {
      // An unreachable entry cannot be resolved; fall back to the literal path.
    }
    if (entry.path === target) return entry;
  }
  return null;
}

// --- reading the conversation -------------------------------------------------

async function readConversation(viewURL, conversationID, afterID) {
  return apiJSON(`${viewURL}/api/workspace/conversations/${encodeURIComponent(conversationID)}?after_id=${afterID}`);
}

// waitForConversation polls one viewer route until it reports what the fake was
// just told. `what` is not decoration: a timeout has to say what it was waiting
// for and what arrived instead, or the failure names nothing.
async function waitForConversation(viewURL, conversationID, afterID, predicate, what, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readConversation(viewURL, conversationID, afterID);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(
    `Timed out after ${timeoutMs}ms waiting for ${what}; the last read at after_id=${afterID} was ${truncate(JSON.stringify(last), 600)}`,
  );
}

// --- harness ------------------------------------------------------------------

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
  console.log(`Smoke test for the conversation of the open workspace, against a fake agent binary

Usage:
  node ./test/e2e/conversation-view-smoke.mjs
  npm run test:view-conversation-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated run directory
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
  throw new Error(`Timed out after 10000ms waiting for ${url} to answer`);
}

function postJSON(payload) {
  return { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

function putJSON(payload) {
  return { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) };
}

async function rawGet(url) {
  const response = await fetch(url, { headers: { Accept: "application/json" } });
  const text = await response.text();
  viewerBodies.push(text);
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${url}: ${truncate(text, 400)}`);
  }
  return text;
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
    throw new Error(`HTTP ${response.status} for ${url}: ${typeof data === "string" ? truncate(data, 400) : truncate(JSON.stringify(data), 400)}`);
  }
  if (expected !== null && response.status !== expected) {
    throw new Error(`Expected HTTP ${expected} for ${url}, got ${response.status}: ${truncate(text, 400)}`);
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
    throw new Error(`Expected HTTP ${status} for ${init.method || "GET"} ${url}, got ${response.status}: ${truncate(text, 400)}`);
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

// --- report -------------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "conversation-from-view",
    spec: "US-053",
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
  <title>ARchetipo Smoke — Conversation of the open workspace (US-053)</title>
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
      <h1>ARchetipo Smoke — Conversation of the open workspace (US-053)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Three real workspaces. One is initialized with <code>archetipo init --tool codex</code> and holds the
    available-but-not-conversational provider; the other two, <code>A</code> and <code>B</code>, are initialized
    with <code>--tool claude</code> and are served by a single <code>archetipo view</code> process. Only the agent
    binary is fake: <code>test/e2e/support/fake-claude.mjs</code>, driven frame by frame through a local control
    server. The oracles are the working directory the agent process reports at startup, the frames it was really
    given, the execution records that never appear on the filesystem, and the end of the process, asked of the
    operating system by pid.</p>

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
