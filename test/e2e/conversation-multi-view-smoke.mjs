#!/usr/bin/env node

// End-to-end smoke for "several live conversations on the same workspace"
// (US-059).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector, the local `claude` provider, its
// stream-json client, the bounded local sessions behind the conversations, the
// conversation journal under `.archetipo/conversations/` and the viewer routes
// addressed by id (`POST` and `GET` on `/api/workspace/conversations`, `GET`,
// `POST .../messages` and `DELETE` on `/api/workspace/conversations/{id}`).
// Only the agent binaries are replaced, by a Node script that speaks the same
// protocol on stdio, so nothing here needs a credential or a network.
//
// What this smoke proves and the conversation smoke deliberately does not: the
// *plural*. Two agent processes alive at once, each keeping its own history;
// which process a message really reached; a close that ends one process and
// leaves the other running; several conversations alive at once past the
// ceiling of three that used to refuse them, each with an agent process of its
// own; and the workspace switch that must leave no agent process of the
// workspace behind and no unsealed record on disk.
//
// The oracles are the ones no viewer field can stand in for: the pid each agent
// process reports about itself, the frames that pid was really given, the
// records on the filesystem, and "is it still running", asked of the operating
// system by pid rather than believed from the route that closed it.
//
// Every fake is driven frame by frame through a control server, and every
// process gets a **channel of its own** on it: with three agents alive a single
// shared queue would let whichever process polls first take a frame meant for
// another, and "which pid received this frame" is the whole oracle of AC-2. The
// channel is carried into each process by a one-line launcher script written per
// conversation and saved through the very route the Execution panel uses, so
// each process is handed its own `FAKE_CLAUDE_CONTROL`. There is no arbitrary
// sleep anywhere: every wait polls a viewer route, the control server or the
// operating system with an explicit timeout naming what it expected and what
// arrived instead.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "conversation-multi-view-smoke");

const CODE_A = "US-A01";
const CODE_B = "US-B01";

// The sentinel stands for whatever authentication material lives in the
// viewer's environment. The agent owns its own authentication, so nothing of it
// may ever reach a viewer response or a workspace configuration.
const AUTH_SENTINEL = "claude-multi-conversation-session-material-DO-NOT-EXPOSE";
// One sentinel per conversation: a phrase that belongs to exactly one thread is
// what makes "the messages of a conversation stay in it" observable instead of
// asserted.
const SENTINEL_A = "smoke-multi-conversation-sentinel-belonging-to-A";
const SENTINEL_B = "smoke-multi-conversation-sentinel-belonging-to-B";
const SENTINEL_AFTER_CLOSE = "smoke-multi-conversation-sentinel-to-A-after-B-was-closed";

// Every viewer response body is kept, so the final check can prove no session
// material travelled to the browser on any route this run touched.
const viewerBodies = [];

// One entry per proved statement — one per acceptance criterion, and no more.
const checks = [];
// The cross-cutting invariants are recorded apart from them on purpose: they
// are conditions the whole run has to keep, not one of the six claims.
const invariants = [];

function ok(criterion, statement) {
  checks.push({ criterion, statement });
  console.log(`-> ${criterion} ok: ${statement}`);
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake agent binary relies on a POSIX shebang, and the per-conversation launchers are /bin/sh scripts");
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

    const dirA = await createWorkspace(runDir, targetsDir, "alfa", CODE_A, env);
    const dirB = await createWorkspace(runDir, targetsDir, "beta", CODE_B, env);

    await scenarioSeveralLiveConversations(runDir, dirA, dirB, env);
    await assertNoSessionMaterialLeaked([dirA, dirB]);
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
  console.log(`\nPASS: conversation-multi-view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  for (const invariant of invariants) {
    console.log(`  · ${invariant}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// --- AC-1 .. AC-6 ------------------------------------------------------------
//
// One viewer, one PID, one workspace holding several conversations at once, and
// a second workspace only at the very end: everything US-059 claims is a claim
// about the same process holding more than one agent, so it is proved on one
// process and never on a restart.
async function scenarioSeveralLiveConversations(runDir, dirA, dirB, env) {
  const realB = await fs.realpath(dirB);
  const launcherDir = path.join(runDir, "launchers");
  await fs.mkdir(launcherDir, { recursive: true });

  let view;
  let control;
  try {
    control = await startControlServer();
    console.log(`-> control server for the fake agents: ${control.url}`);
    view = await startViewServer(dirA, { ...env, FAKE_CLAUDE_CONTROL: `${control.url}/c/unassigned` });
    const pid = view.child.pid;
    console.log(`-> view ready: ${view.url} (pid ${pid}, launched in ${dirA})`);

    const conversationsURL = `${view.url}/api/workspace/conversations`;
    const open = (name) => openConversation(view, control, launcherDir, name);

    // --- AC-1 ---------------------------------------------------------------
    const a = await open("a");
    await assertSamePath(a.opened.conversation.working_dir, dirA, "AC-1: the directory the conversation A reports");
    a.channel.push(emit(assistantText("Sono la conversazione A e sto lavorando.")));
    const aOpening = await waitForConversation(
      view.url,
      a.id,
      0,
      (data) => (data.events || []).length === 1,
      "the assistant frame of A to become one text event",
    );
    const aSignatureBeforeB = signatureOf(aOpening.events);

    const b = await open("b");
    if (b.id === a.id) {
      throw new Error(`AC-1: the second open must answer with a conversation of its own; both are ${b.id}`);
    }
    if (b.pid === a.pid) {
      throw new Error(`AC-1: the second conversation reused the agent process of the first (pid ${a.pid})`);
    }
    await assertSamePath(b.opened.conversation.working_dir, dirA, "AC-1: the directory the conversation B reports");
    // The first conversation is untouched by the second being opened: the
    // operating system still has its process, and the history it serves is the
    // same events with the same ids.
    assertProcessAlive(a.pid, `the agent process of A (pid ${a.pid}) after B was opened beside it`);
    const aAfterB = await readConversation(view.url, a.id, 0);
    assertSameSignature("AC-1", aSignatureBeforeB, signatureOf(aAfterB.events), `the history of ${a.id} across the opening of ${b.id}`);
    if (aAfterB.conversation?.state !== "ACTIVE") {
      throw new Error(`AC-1: A must still be active while B is opened; got ${JSON.stringify(aAfterB.conversation)}`);
    }
    ok(
      "AC-1",
      `with ${a.id} already live, POST /api/workspace/conversations answered 201 with the distinct conversation ${b.id} whose agent process (pid ${b.pid}) is a second one, while the operating system still reports the process of A (pid ${a.pid}) alive and GET /api/workspace/conversations/${a.id} serves the very same event(s) [${aSignatureBeforeB.map((event) => event.id).join(",")}] with state ACTIVE`,
    );

    // --- AC-2 ---------------------------------------------------------------
    // The message goes in by the route, and the oracle is which process was
    // really handed the frame — asked of the fakes, which report every line they
    // read together with their own pid.
    await apiJSON(`${conversationsURL}/${encodeURIComponent(b.id)}/messages`, postJSON({ message: SENTINEL_B }), 202);
    const deliveredToB = await control.waitFor(
      userFrameCarrying(SENTINEL_B),
      1,
      `the frame carrying ${JSON.stringify(SENTINEL_B)} to reach an agent process`,
    );
    if (deliveredToB.pid !== b.pid) {
      throw new Error(`AC-2: the message for ${b.id} was handed to pid ${deliveredToB.pid}, want the pid of B ${b.pid}`);
    }
    assertOnlyPidWasGiven(control, SENTINEL_B, b.pid, "AC-2");
    b.channel.push(emit(userReplay(SENTINEL_B)));
    const bHistory = await waitForConversation(
      view.url,
      b.id,
      0,
      (data) => (data.events || []).some((event) => event.kind === "user_message" && event.text === SENTINEL_B),
      `the message of B to enter the history of ${b.id}`,
    );
    const aWhileBSpoke = await readConversation(view.url, a.id, 0);
    assertAbsentFromHistory("AC-2", aWhileBSpoke, SENTINEL_B, a.id);
    assertSameSignature("AC-2", aSignatureBeforeB, signatureOf(aWhileBSpoke.events), `the history of ${a.id} across the whole exchange of ${b.id}`);

    // …and symmetrically, so the claim is about both conversations and not
    // about the one that happened to be second.
    await apiJSON(`${conversationsURL}/${encodeURIComponent(a.id)}/messages`, postJSON({ message: SENTINEL_A }), 202);
    const deliveredToA = await control.waitFor(
      userFrameCarrying(SENTINEL_A),
      1,
      `the frame carrying ${JSON.stringify(SENTINEL_A)} to reach an agent process`,
    );
    if (deliveredToA.pid !== a.pid) {
      throw new Error(`AC-2: the message for ${a.id} was handed to pid ${deliveredToA.pid}, want the pid of A ${a.pid}`);
    }
    assertOnlyPidWasGiven(control, SENTINEL_A, a.pid, "AC-2");
    a.channel.push(emit(userReplay(SENTINEL_A)));
    const aHistory = await waitForConversation(
      view.url,
      a.id,
      0,
      (data) => (data.events || []).some((event) => event.kind === "user_message" && event.text === SENTINEL_A),
      `the message of A to enter the history of ${a.id}`,
    );
    const bWhileASpoke = await readConversation(view.url, b.id, 0);
    assertAbsentFromHistory("AC-2", bWhileASpoke, SENTINEL_A, b.id);
    assertAbsentFromHistory("AC-2", aHistory, SENTINEL_B, a.id);
    ok(
      "AC-2",
      `the message of B was handed to pid ${b.pid} and to no other process, and appears in the ${bHistory.events.length}-event history of ${b.id} and never in ${a.id}; the message of A was handed to pid ${a.pid} and to no other process, and appears in the ${aHistory.events.length}-event history of ${a.id} and never in ${b.id}`,
    );

    // --- AC-3, the half that must be read while both are live ---------------
    const indexBothLive = await apiJSON(conversationsURL);
    assertListedLive("AC-3", indexBothLive, a.id, true);
    assertListedLive("AC-3", indexBothLive, b.id, true);

    // --- AC-4 ---------------------------------------------------------------
    const aBeforeClose = signatureOf((await readConversation(view.url, a.id, 0)).events);
    const closed = await apiJSON(`${conversationsURL}/${encodeURIComponent(b.id)}?after_id=0`, { method: "DELETE" }, 200);
    if (closed.conversation?.id !== b.id) {
      throw new Error(`AC-4: the close must answer about the conversation it closed; got ${JSON.stringify(closed.conversation)}`);
    }
    if (closed.conversation.state !== "CLOSED") {
      throw new Error(`AC-4: the closed conversation must be reported CLOSED; got ${JSON.stringify(closed.conversation.state)}`);
    }
    // The two halves of "closing one does not touch the others", each asked of
    // the operating system and never of a route.
    await waitForProcessGone(b.pid, `the agent process of B (pid ${b.pid}) to be released by DELETE /api/workspace/conversations/${b.id}`);
    assertProcessAlive(a.pid, `the agent process of A (pid ${a.pid}) after B was closed`);

    // --- AC-3, the half that must be read once one of them is closed --------
    const indexAfterClose = await apiJSON(conversationsURL);
    assertListedLive("AC-3", indexAfterClose, a.id, true);
    const closedEntry = assertListedLive("AC-3", indexAfterClose, b.id, false);
    if (!String(closedEntry.state || "").trim()) {
      throw new Error(`AC-3: a closed conversation must carry the state it ended in; got ${JSON.stringify(closedEntry)}`);
    }
    ok(
      "AC-3",
      `GET /api/workspace/conversations marks both ${a.id} and ${b.id} live:true while both are held, and after ${b.id} is closed the same route marks ${a.id} live:true and ${b.id} live:false with the non-empty state ${JSON.stringify(closedEntry.state)}`,
    );

    const aAfterClose = await readConversation(view.url, a.id, 0);
    if (aAfterClose.conversation?.state !== "ACTIVE") {
      throw new Error(`AC-4: A must still be ACTIVE after B was closed; got ${JSON.stringify(aAfterClose.conversation)}`);
    }
    assertSameSignature("AC-4", aBeforeClose, signatureOf(aAfterClose.events), `the history of ${a.id} across the close of ${b.id}`);
    // The surviving conversation is not merely readable: it is still reachable,
    // which only the process it belongs to can settle.
    await apiJSON(`${conversationsURL}/${encodeURIComponent(a.id)}/messages`, postJSON({ message: SENTINEL_AFTER_CLOSE }), 202);
    const deliveredAfterClose = await control.waitFor(
      userFrameCarrying(SENTINEL_AFTER_CLOSE),
      1,
      `the frame sent to ${a.id} after the close of ${b.id} to reach an agent process`,
    );
    if (deliveredAfterClose.pid !== a.pid) {
      throw new Error(`AC-4: the message sent to A after the close reached pid ${deliveredAfterClose.pid}, want ${a.pid}`);
    }
    ok(
      "AC-4",
      `DELETE /api/workspace/conversations/${b.id} answered 200 with state CLOSED, the operating system reports its process (pid ${b.pid}) gone and the process of ${a.id} (pid ${a.pid}) still alive, ${a.id} is still ACTIVE with its ${aBeforeClose.length} events unchanged, and a message posted to it afterwards was handed to pid ${a.pid}`,
    );

    // --- AC-5 ---------------------------------------------------------------
    // There used to be a ceiling of three live conversations per workspace, and
    // this criterion proved the refusal of the fourth. The ceiling is gone: a
    // number nobody chose was turning "open one more" into an error to read and
    // a thread to close first, and once a step that finds no thread opens its
    // own, that refusal would have started falling on runs nobody asked to have
    // refused. What is proved now is the opposite, and it is proved past the old
    // number on purpose: every open is honoured, every one gets an agent process
    // of its own, and the index lists them all live.
    const beyondTheOldLimit = 5;
    const processesBeforeMany = agentProcessCount(control);
    const extra = [];
    for (let i = 2; i <= beyondTheOldLimit; i += 1) {
      extra.push(await open(`many-${i}`));
    }
    const liveIDs = [a.id, ...extra.map((entry) => entry.id)];
    if (liveIDs.length !== beyondTheOldLimit) {
      throw new Error(`AC-5: the workspace must hold ${beyondTheOldLimit} live conversations; it holds ${liveIDs.length}`);
    }
    const indexWithMany = await apiJSON(conversationsURL);
    for (const id of liveIDs) assertListedLive("AC-5", indexWithMany, id, true);
    const processesAfterMany = agentProcessCount(control);
    if (processesAfterMany !== processesBeforeMany + extra.length) {
      throw new Error(
        `AC-5: opening ${extra.length} more conversations should have started ${extra.length} more agent processes; the control server saw ${processesBeforeMany} before and ${processesAfterMany} after`,
      );
    }
    for (const entry of extra) {
      assertProcessAlive(entry.pid, `the agent process (pid ${entry.pid}) of the conversation ${entry.id}`);
    }
    ok(
      "AC-5",
      `${beyondTheOldLimit} conversations were opened on one workspace with no refusal — past the ceiling of three that used to exist — POST /api/workspace/conversations answered 201 every time, the control server counts one agent process per conversation (${processesBeforeMany} before, ${processesAfterMany} after ${extra.length} opens), and GET /api/workspace/conversations marks every one of ${liveIDs.join(", ")} live:true`,
    );

    // --- AC-6 ---------------------------------------------------------------
    await expectStatus(`${view.url}/api/workspaces`, 201, postJSON({ path: dirB }));
    const known = await apiJSON(`${view.url}/api/workspaces`);
    const entryB = await findByRealPath(known.workspaces, realB);
    if (!entryB) {
      throw new Error(`AC-6: the second workspace must be known: ${JSON.stringify(known.workspaces)}`);
    }
    const livePIDs = [a.pid, ...extra.map((entry) => entry.pid)];
    for (const livePID of livePIDs) {
      assertProcessAlive(livePID, `the agent process (pid ${livePID}) of the workspace about to be left`);
    }
    await expectStatus(`${view.url}/api/workspaces/${encodeURIComponent(entryB.id)}/open`, 200, postJSON({}));
    assertEqual(view.child.pid, pid, "the viewer PID after opening the second workspace");
    assertAlive(view.child, "the viewer process after opening the second workspace");
    for (const livePID of livePIDs) {
      await waitForProcessGone(livePID, `the agent process (pid ${livePID}) of the workspace left behind to be released by the workspace switch`);
    }

    const openedIDs = [...liveIDs, b.id];
    const sealed = await waitForSealedRecords(dirA, openedIDs);
    const rawIndexOfB = await rawGet(conversationsURL);
    for (const id of openedIDs) {
      if (rawIndexOfB.includes(id)) {
        throw new Error(`AC-6: the conversation ${id} of the workspace left behind appears in the index served for the new workspace: ${truncate(rawIndexOfB, 600)}`);
      }
    }
    ok(
      "AC-6",
      `POST /api/workspaces/{B}/open on the same viewer pid ${pid} left no agent process of the previous workspace running — all ${livePIDs.length} pids (${livePIDs.join(", ")}) reported gone by the operating system — sealed all ${sealed.length} records under .archetipo/conversations/ with a non-empty final_state (${sealed.map((record) => `${record.id}:${record.final_state}`).join(", ")}), and none of its ${openedIDs.length} conversation ids appears anywhere in the ${rawIndexOfB.length}-byte index served for the workspace now open`,
    );

    // The whole story happened on one viewer process.
    assertEqual(view.child.pid, pid, "the viewer PID at the end of the scenario");
    assertAlive(view.child, "the viewer process at the end of the scenario");
  } finally {
    if (view) await stopProcess(view.child);
    if (control) await control.close();
  }
}

// --- opening a conversation on a channel of its own ---------------------------

// openConversation starts one conversation and hands back the process behind it.
//
// The launcher script is what makes the plural testable: every agent needs a
// control channel of its own, the channel travels in the environment, and the
// environment of the agent is the viewer's — fixed when the viewer started. So
// each conversation gets a one-line `/bin/sh` launcher carrying its own
// `FAKE_CLAUDE_CONTROL`, saved as the workspace default through the very route
// the Execution panel uses, right before the open that will use it. Because the
// channel is private to the process about to be started, the `system`/`init`
// frame `OpenConversation` waits for is pushed exactly once and cannot be taken
// by an agent that is already live.
async function openConversation(view, control, launcherDir, name) {
  const channel = control.channel(name);
  const launcher = await writeAgentLauncher(launcherDir, name, channel.url);
  await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
    id: "claude",
    config: { command: launcher, timeout_seconds: 600 },
  }));
  channel.push(emit(initFrame(`conversation-${name}`)));
  const opened = await apiJSON(`${view.url}/api/workspace/conversations`, postJSON({}), 201);
  const id = opened.conversation?.id;
  if (!id) {
    throw new Error(`the open of the conversation ${name} answered without a conversation: ${JSON.stringify(opened)}`);
  }
  const invocation = await control.waitFor(
    (entry) => entry.channel === channel.name && entry.kind === "argv",
    1,
    `the agent process of the conversation ${name} to report its invocation on its own channel`,
  );
  if (!Number.isInteger(invocation.pid)) {
    throw new Error(`the agent process of the conversation ${name} reported no pid: ${JSON.stringify(invocation)}`);
  }
  console.log(`-> conversation ${name}: ${id} (agent pid ${invocation.pid})`);
  return { name, id, channel, pid: invocation.pid, opened, invocation };
}

async function writeAgentLauncher(launcherDir, name, controlURL) {
  const file = path.join(launcherDir, `claude-${name}.sh`);
  await fs.writeFile(
    file,
    `#!/bin/sh\n# The fake claude of the conversation ${name}, bound to its own control channel.\nFAKE_CLAUDE_CONTROL=${shQuote(controlURL)} exec ${shQuote(process.execPath)} ${shQuote(fakeClaudePath)} "$@"\n`,
    "utf8",
  );
  await fs.chmod(file, 0o755);
  return file;
}

function shQuote(value) {
  return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

// --- oracles ------------------------------------------------------------------

// signatureOf reduces a history to what must not change: the ids, the kinds and
// the texts, in order. Comparing whole payloads would fail on timestamps that
// are allowed to differ.
function signatureOf(events) {
  return (events || []).map((event) => ({ id: event.id, kind: event.kind, text: event.text ?? "" }));
}

function assertSameSignature(criterion, before, after, what) {
  if (JSON.stringify(before) !== JSON.stringify(after)) {
    throw new Error(
      `${criterion}: ${what} changed\n  before: ${truncate(JSON.stringify(before), 600)}\n  after:  ${truncate(JSON.stringify(after), 600)}`,
    );
  }
}

function assertAbsentFromHistory(criterion, view, sentinel, conversationID) {
  const found = (view.events || []).filter((event) => String(event.text || "").includes(sentinel));
  if (found.length) {
    throw new Error(`${criterion}: ${JSON.stringify(sentinel)} appears in the history of ${conversationID}: ${truncate(JSON.stringify(found), 400)}`);
  }
}

function assertListedLive(criterion, index, conversationID, live) {
  const entry = (index.conversations || []).find((row) => row.id === conversationID);
  if (!entry) {
    throw new Error(`${criterion}: the index does not list ${conversationID}: ${truncate(JSON.stringify(index.conversations), 600)}`);
  }
  if (entry.live !== live) {
    throw new Error(`${criterion}: the index reports ${conversationID} live:${entry.live}, want live:${live}: ${JSON.stringify(entry)}`);
  }
  return entry;
}

// assertOnlyPidWasGiven is the isolation oracle: not "the right process got it"
// but "no other one did". A frame that reached two agents would put a sentence
// of one conversation into another, which is exactly what AC-2 forbids.
function assertOnlyPidWasGiven(control, sentinel, expectedPID, criterion) {
  const elsewhere = control.reports().filter(
    (entry) => entry.kind === "received" && JSON.stringify(entry.frame || {}).includes(sentinel) && entry.pid !== expectedPID,
  );
  if (elsewhere.length) {
    throw new Error(
      `${criterion}: ${JSON.stringify(sentinel)} was also handed to the process(es) ${elsewhere.map((entry) => entry.pid).join(", ")}, and only pid ${expectedPID} should have received it`,
    );
  }
}

function agentProcessCount(control) {
  return control.reports().filter((entry) => entry.kind === "argv").length;
}

// waitForProcessGone polls the operating system until the agent process is no
// longer there. `kill(pid, 0)` sends nothing: it asks whether the process
// exists, and it is the only oracle that answers "the provider really released
// it" instead of "the route said so".
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
  throw new Error(`Timed out after ${timeoutMs}ms waiting for ${what}; the operating system still reports pid ${pid} alive`);
}

// assertProcessAlive is the same question asked the other way round, and it is
// asked of the operating system for the same reason: "the other conversation was
// not touched" is a statement about a process.
function assertProcessAlive(pid, what) {
  if (!Number.isInteger(pid)) {
    throw new Error(`the fake did not report the pid of its own process; got ${JSON.stringify(pid)}`);
  }
  try {
    process.kill(pid, 0);
  } catch (error) {
    if (error.code === "ESRCH") {
      throw new Error(`${what}: the operating system reports the process is gone`);
    }
    throw new Error(`could not ask the operating system about pid ${pid}: ${error.message}`);
  }
}

// waitForSealedRecords polls the filesystem until every conversation the run
// opened has a record carrying a final state. It polls rather than reads once
// because the seal happens inside the workspace switch, and it names in its
// timeout which record was still open.
async function waitForSealedRecords(root, ids, timeoutMs = 20000) {
  const dir = path.join(root, ".archetipo", "conversations");
  const started = Date.now();
  let unsealed = ids.slice();
  let records = [];
  while (Date.now() - started < timeoutMs) {
    records = [];
    unsealed = [];
    for (const id of ids) {
      let record = null;
      try {
        record = JSON.parse(await fs.readFile(path.join(dir, `${id}.json`), "utf8"));
      } catch {
        unsealed.push(`${id} (no readable record yet)`);
        continue;
      }
      if (!String(record.final_state || "").trim()) {
        unsealed.push(`${id} (final_state ${JSON.stringify(record.final_state ?? null)})`);
        continue;
      }
      records.push(record);
    }
    if (unsealed.length === 0) return records;
    await delay(100);
  }
  throw new Error(
    `Timed out after ${timeoutMs}ms waiting for every conversation of the workspace left behind to be sealed under ${dir}; still without a final state: ${unsealed.join(", ")}`,
  );
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

// --- cross-cutting -------------------------------------------------------------

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
  invariants.push(
    `the session material is absent from all ${viewerBodies.length} viewer responses and from every .archetipo/config.yaml`,
  );
  console.log(`-> invariant ok: ${invariants[invariants.length - 1]}`);
}

// --- the protocol ---------------------------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

function initFrame(sessionID) {
  return { type: "system", subtype: "init", session_id: sessionID };
}

function assistantText(text) {
  return { type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text }] } };
}

// userReplay is the frame the real binary emits back for a message it was given
// under `--replay-user-messages`: it is what turns a delivered message into
// history, and never the route that delivered it.
function userReplay(text) {
  return { type: "user", message: { content: [{ type: "text", text }] }, isReplay: true };
}

// The blocks of a user frame are looked at one by one, and never joined. The
// instruction that opens a conversation is held until the person writes, and
// then travels in the *same* frame as their first message, as a block of its
// own (cli/internal/execution/claude/streamjson.go, `hold`). A test that joined
// the blocks would read that first frame as "the instruction glued to the
// message" and never recognise the message it delivered — while the process was
// given exactly the message, told apart from the instruction, which is what the
// separate blocks are for.
function userFrameBlocks(entry) {
  return (entry.frame?.message?.content || []).map((block) => block.text || "");
}

function userFrameCarrying(text) {
  return (entry) => entry.kind === "received" && entry.frame?.type === "user" && userFrameBlocks(entry).includes(text);
}

// --- the control server -----------------------------------------------------------
//
// One server, one queue per channel. The fakes never progress on their own: each
// asks *its own* channel what to do next and reports everything it read, with
// its pid and the channel it belongs to. The per-channel queue is what makes
// three live agents addressable: a frame pushed for one process cannot be taken
// by another, which single-queue smokes had to work around with a timer because
// they only ever needed it for the instant two processes overlapped.

async function startControlServer() {
  const queues = new Map();
  const received = [];
  const queueOf = (name) => {
    if (!queues.has(name)) queues.set(name, []);
    return queues.get(name);
  };

  const server = http.createServer(async (req, res) => {
    const match = /^\/c\/([^/?]+)\/(next|received)/.exec(req.url || "");
    if (match) {
      const [, name, verb] = match;
      if (req.method === "GET" && verb === "next") {
        sendJSON(res, 200, queueOf(name).shift() || { kind: "none" });
        return;
      }
      if (req.method === "POST" && verb === "received") {
        received.push({ channel: name, ...JSON.parse(await readBody(req)) });
        sendJSON(res, 200, { ok: true });
        return;
      }
    }
    sendJSON(res, 404, { error: "not found" });
  });

  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const url = `http://127.0.0.1:${server.address().port}`;

  return {
    url,
    // channel hands out the private queue of one agent process, and the URL its
    // launcher will carry.
    channel(name) {
      queueOf(name);
      return {
        name,
        url: `${url}/c/${name}`,
        push(command) {
          queueOf(name).push(command);
        },
        drain() {
          queueOf(name).length = 0;
        },
      };
    },
    reports() {
      return received;
    },
    // waitFor polls until the fakes have reported at least `count` matching
    // requests, and returns the count-th of them. The count is explicit rather
    // than relative to a snapshot taken here, because a request can perfectly
    // well have arrived before the test got round to waiting for it.
    async waitFor(matcher, count = 1, what = "the expected report", timeoutMs = 30000) {
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matching = received.filter(matcher);
        if (matching.length >= count) return matching[count - 1];
        await delay(50);
      }
      throw new Error(
        `Timed out after ${timeoutMs}ms waiting for ${what} (${count} occurrence(s)); the control server had received ${JSON.stringify(received.map((entry) => `${entry.channel}/${entry.kind}@${entry.pid}`))}`,
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

// --- fixtures -------------------------------------------------------------------

// createWorkspace initializes one real workspace with a backlog holding a single
// recognisable spec code, so which workspace is being served is never in doubt.
async function createWorkspace(runDir, targetsDir, name, code, env) {
  const dir = path.join(targetsDir, name);
  await fs.mkdir(dir, { recursive: true });
  await runCommand(`init-${name}`, cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: dir, env });

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

// --- reading the conversation ------------------------------------------------------

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

// --- harness --------------------------------------------------------------------

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
  console.log(`Smoke test for several live conversations on the same workspace, against fake agent binaries

Usage:
  node ./test/e2e/conversation-multi-view-smoke.mjs
  npm run test:view-conversation-multi-smoke

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

// --- report ---------------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "conversation-multi-from-view",
    spec: "US-059",
    passed: !failure,
    started_at: new Date(startedAt).toISOString(),
    duration_ms: durationMs,
    run_dir: runDir,
    checks,
    invariants,
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
  const invariantRows = summary.invariants
    .map((line) => `<li>${escapeHTML(line)}</li>`)
    .join("\n        ");
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ARchetipo Smoke — Several live conversations on one workspace (US-059)</title>
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
      <h1>ARchetipo Smoke — Several live conversations on one workspace (US-059)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Two real workspaces initialized with <code>archetipo init --tool claude</code>, served by a single
    <code>archetipo view</code> process launched inside the first. Only the agent binaries are fake:
    <code>test/e2e/support/fake-claude.mjs</code>, one process per conversation, each driven frame by frame through a
    control channel of its own on a local control server. The oracles are the pid each agent process reports about
    itself, the frames that pid was really given, the records under <code>.archetipo/conversations/</code>, and
    whether a process is still running, asked of the operating system by pid.</p>

    <h2>Proved statements</h2>
    <table>
      <thead><tr><th>Criterion</th><th>Statement</th></tr></thead>
      <tbody>
        ${rows || '<tr><td colspan="2">none</td></tr>'}
      </tbody>
    </table>

    <h2>Cross-cutting invariants</h2>
    <ul>
        ${invariantRows || "<li>none</li>"}
    </ul>
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
