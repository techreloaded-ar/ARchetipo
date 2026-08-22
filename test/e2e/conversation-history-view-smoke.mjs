#!/usr/bin/env node

// End-to-end smoke for "find the past conversations of the workspace again"
// (US-058).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector, the local `claude` provider, its
// stream-json client, the conversation journal that writes
// `.archetipo/conversations/<id>.json`, and the routes the thread rail reads
// (`GET /api/workspace/conversations`, `GET /api/workspace/conversations/{id}`
// and `POST /api/workspace/conversations/{id}/resume`). Only the agent binary
// is replaced, by a Node script that speaks the same protocol on stdio, so the
// conversations need no credential and no network.
//
// This smoke exists for the claims US-053 could not make, because US-053 proved
// everything on one viewer process on purpose. Here the viewer is *killed* and
// started again on the same workspace: what survives is what is on the disk,
// and the oracles are the record file itself, a second process identifier, the
// frames the resumed agent was really given — its prompt, where the transcript
// of the past conversation has to be readable — and the end of a process, asked
// of the operating system by pid rather than believed from the route.
//
// The fake never progresses on its own: every frame it emits is commanded
// through a local control server, and every frame it receives is reported back
// to that server with the pid of the process that received it. There is no
// arbitrary sleep anywhere — each wait polls a viewer route, the control server
// or the operating system with an explicit timeout that names what it was
// waiting for and what arrived instead.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "conversation-history-view-smoke");

const CODE_A = "US-A01";
const CODE_B = "US-B01";
const CODE_C = "US-C01";

// The sentinels are what makes "this is the conversation of yesterday" a fact
// of the payload and never an inference: one for what the agent said, one for
// what the person wrote, one for the message that asked for the resume, and one
// for the conversation held on the other workspace.
const AGENT_SENTINEL = "smoke-history-agent-said-yesterday-sentinel";
const HUMAN_SENTINEL = "smoke-history-operator-said-yesterday-sentinel";
const RESUME_SENTINEL = "smoke-history-resume-message-sentinel";
const BETA_SENTINEL = "smoke-history-beta-conversation-sentinel";

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
    };
    const targetsDir = path.join(runDir, "targets");
    await fs.mkdir(targetsDir, { recursive: true });

    const dirA = await createWorkspace(runDir, targetsDir, "alfa", CODE_A, env);
    const dirB = await createWorkspace(runDir, targetsDir, "beta", CODE_B, env);
    const dirC = await createWorkspace(runDir, targetsDir, "gamma", CODE_C, env);

    await scenarioConversationsThatSurvive(dirA, dirB, dirC, env);
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
  console.log(`\nPASS: conversation-history-view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// --- AC-1, AC-3, AC-4, AC-6, AC-7 --------------------------------------------
//
// One story told across two viewer processes. The first one holds a
// conversation and is then killed; everything asserted afterwards is asserted
// against a process that never saw it happen, which is the whole point of a
// history that survives.
async function scenarioConversationsThatSurvive(dirA, dirB, dirC, env) {
  const realB = await fs.realpath(dirB);
  const realC = await fs.realpath(dirC);

  let control;
  let viewOne;
  let viewTwo;
  try {
    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);
    const viewEnv = { ...env, FAKE_CLAUDE_CONTROL: control.url };

    // --- yesterday, on a viewer process that is about to be killed ----------
    viewOne = await startViewServer(dirA, viewEnv);
    const pidOne = viewOne.child.pid;
    console.log(`-> first view ready: ${viewOne.url} (pid ${pidOne}, launched in ${dirA})`);
    await useFakeProvider(viewOne.url);

    control.push(emit(initFrame("conversation-yesterday")));
    const opened = await apiJSON(`${viewOne.url}/api/workspace/conversation`, postJSON({ spec_code: CODE_A }), 201);
    const yesterdayID = opened.conversation?.id;
    if (!yesterdayID) {
      throw new Error(`AC-1: unexpected payload on open: ${JSON.stringify(opened)}`);
    }
    const agentYesterday = await control.waitFor("argv", 1);

    control.push(emit(assistantText(AGENT_SENTINEL)));
    await waitForConversation(
      viewOne.url,
      0,
      (data) => (data.events || []).length === 1,
      "the assistant frame of the conversation of yesterday to become a text event",
    );
    await apiJSON(`${viewOne.url}/api/workspace/conversation/messages`, postJSON({ message: HUMAN_SENTINEL }), 202);
    // The first user frame carried the opening instruction; the operator's
    // message is the second one the process is given.
    await control.waitFor(userFrame, 2);
    control.push(emit(userReplay(HUMAN_SENTINEL)));
    const beforeRestart = await waitForConversation(
      viewOne.url,
      0,
      (data) => (data.events || []).length === 2,
      "the message of the person to enter the history of the conversation of yesterday",
    );
    const eventsBefore = beforeRestart.events;
    assertEventIDs(eventsBefore, [1, 2], "AC-1 the history held before the restart");

    const closed = await apiJSON(`${viewOne.url}/api/workspace/conversation?after_id=0`, { method: "DELETE" }, 200);
    if (closed.conversation?.id !== yesterdayID) {
      throw new Error(`AC-1: the close must answer about the conversation it closed; got ${JSON.stringify(closed.conversation)}`);
    }
    await waitForProcessGone(agentYesterday.pid, `the agent process of the conversation of yesterday (pid ${agentYesterday.pid})`);

    // --- AC-1, the record on the filesystem ---------------------------------
    const recordPath = path.join(dirA, ".archetipo", "conversations", `${yesterdayID}.json`);
    const record = JSON.parse(await fs.readFile(recordPath, "utf8"));
    if (record.id !== yesterdayID) {
      throw new Error(`AC-1: ${recordPath} holds ${JSON.stringify(record.id)}, want ${yesterdayID}`);
    }
    if (record.spec_code !== CODE_A) {
      throw new Error(`AC-1: the record lost the spec it was opened about; got ${JSON.stringify(record.spec_code)}`);
    }
    if (record.final_state !== "CLOSED") {
      throw new Error(`AC-1: a conversation closed by the operator must be recorded closed; got ${JSON.stringify(record.final_state)}`);
    }
    if (record.title !== HUMAN_SENTINEL) {
      throw new Error(`AC-1: the record must be named by the first thing the person said; got ${JSON.stringify(record.title)}`);
    }
    assertSameEvents(eventsBefore, record.events, "AC-1 the events written to the record file");

    // --- ARchetipo is closed, and started again ------------------------------
    await stopProcess(viewOne.child);
    await waitForProcessGone(pidOne, `the first viewer process (pid ${pidOne}) to be gone`);
    viewOne = null;

    viewTwo = await startViewServer(dirA, viewEnv);
    const pidTwo = viewTwo.child.pid;
    if (pidTwo === pidOne) {
      throw new Error(`AC-1: the second viewer must be a different process; both report pid ${pidTwo}`);
    }
    console.log(`-> second view ready: ${viewTwo.url} (pid ${pidTwo}, launched in ${dirA})`);

    const listed = await apiJSON(`${viewTwo.url}/api/workspace/conversations`);
    if (!Array.isArray(listed.conversations)) {
      throw new Error(`AC-1: the index must always be an array; got ${JSON.stringify(listed.conversations)}`);
    }
    const entryYesterday = listed.conversations.find((entry) => entry.id === yesterdayID);
    if (!entryYesterday) {
      throw new Error(`AC-1: the reopened viewer does not list the conversation of yesterday; got ${JSON.stringify(listed.conversations)}`);
    }
    if (entryYesterday.title !== HUMAN_SENTINEL) {
      throw new Error(`AC-1: the listed title is ${JSON.stringify(entryYesterday.title)}, want ${JSON.stringify(HUMAN_SENTINEL)}`);
    }
    if (entryYesterday.spec_code !== CODE_A) {
      throw new Error(`AC-1: the listed entry lost its spec; got ${JSON.stringify(entryYesterday.spec_code)}`);
    }
    if (entryYesterday.live !== false) {
      throw new Error("AC-1: a conversation whose process is long gone must never be listed as live");
    }
    if (entryYesterday.message_count !== 2) {
      throw new Error(`AC-1: the listed entry counts ${entryYesterday.message_count} messages, want 2`);
    }
    if (!String(entryYesterday.last_message_at || "").trim()) {
      throw new Error("AC-1: the listed entry must carry the moment of its last message");
    }
    ok(
      "AC-1",
      `the conversation ${yesterdayID} was written to ${path.relative(dirA, recordPath)} with its two events and final_state CLOSED, the viewer process ${pidOne} was killed, and the new process ${pidTwo} — a different pid on the same workspace — lists it as ${JSON.stringify(entryYesterday.title)} with live:false, spec ${entryYesterday.spec_code} and ${entryYesterday.message_count} messages`,
    );

    // --- AC-3 ---------------------------------------------------------------
    const transcript = await apiJSON(`${viewTwo.url}/api/workspace/conversations/${encodeURIComponent(yesterdayID)}`);
    if (transcript.id !== yesterdayID) {
      throw new Error(`AC-3: the transcript is about ${JSON.stringify(transcript.id)}, want ${yesterdayID}`);
    }
    if (transcript.live !== false) {
      throw new Error("AC-3: a past transcript must not declare itself live");
    }
    assertSameEvents(eventsBefore, transcript.events, "AC-3 the transcript read after the restart");
    assertStrictlyIncreasing(transcript.events, "AC-3 the transcript read after the restart");
    const spokenTexts = transcript.events.map((event) => event.text);
    if (!spokenTexts.includes(AGENT_SENTINEL) || !spokenTexts.includes(HUMAN_SENTINEL)) {
      throw new Error(`AC-3: the transcript lost what was said; got ${JSON.stringify(spokenTexts)}`);
    }
    if (spokenTexts.indexOf(AGENT_SENTINEL) > spokenTexts.indexOf(HUMAN_SENTINEL)) {
      throw new Error(`AC-3: the transcript is out of order; got ${JSON.stringify(spokenTexts)}`);
    }
    ok(
      "AC-3",
      `GET /api/workspace/conversations/${yesterdayID} on the new process returned the same ${transcript.events.length} events as before the restart — the same ids [${transcript.events.map((event) => event.id).join(",")}], the same kinds and the same texts — in the order they were spoken`,
    );

    // --- AC-4 ---------------------------------------------------------------
    // A live conversation first, because taking up a past one has to close
    // whatever is open: the claim is about two processes and not only about a
    // payload.
    await useFakeProvider(viewTwo.url);
    control.push(emit(initFrame("conversation-live")));
    const live = await apiJSON(`${viewTwo.url}/api/workspace/conversation`, postJSON({}), 201);
    const liveID = live.conversation?.id;
    if (!liveID || liveID === yesterdayID) {
      throw new Error(`AC-4: the live conversation must be one of its own; got ${JSON.stringify(live.conversation)}`);
    }
    const agentLive = await control.waitFor("argv", 2);
    if (promptsCarryingTheTranscript(control).length !== 0) {
      throw new Error("AC-4: no agent process may have been given the transcript of yesterday before the resume was asked for");
    }

    // The resume is answered only once the new process has announced itself, so
    // the call is started and awaited around the two things that must happen in
    // between: the old process ending, and the new one being told what to emit.
    // Pushing the init frame any earlier would let the process being closed
    // consume it.
    const resumeCall = settled(apiJSON(
      `${viewTwo.url}/api/workspace/conversations/${encodeURIComponent(yesterdayID)}/resume`,
      postJSON({ message: RESUME_SENTINEL }),
      201,
    ));
    await waitForProcessGone(agentLive.pid, `the agent process of the live conversation (pid ${agentLive.pid}) to be released by the resume`);
    control.push(emit(initFrame("conversation-resumed")));
    const resumed = await resumeCall();

    const resumedID = resumed.conversation?.id;
    if (!resumedID || resumedID === yesterdayID || resumedID === liveID) {
      throw new Error(`AC-4: a resume opens a new conversation; got ${JSON.stringify(resumed.conversation)}`);
    }
    if (resumed.conversation.resumed_from !== yesterdayID) {
      throw new Error(`AC-4: the new conversation must declare what it takes up; got ${JSON.stringify(resumed.conversation.resumed_from)}`);
    }
    if (resumed.conversation.spec_code !== CODE_A) {
      throw new Error(`AC-4: a resumed thread stays about what it was about; got ${JSON.stringify(resumed.conversation.spec_code)}`);
    }
    const agentResumed = await control.waitFor("argv", 3);
    if (agentResumed.pid === agentLive.pid) {
      throw new Error("AC-4: the resume reused the process of the conversation it just closed");
    }
    const prompt = await control.waitFor(framesOf(agentResumed.pid, userFrame), 1);
    const promptText = userFrameText(prompt);
    for (const sentinel of [AGENT_SENTINEL, HUMAN_SENTINEL]) {
      if (!promptText.includes(sentinel)) {
        throw new Error(`AC-4: the prompt the new agent process really received does not carry ${JSON.stringify(sentinel)}: ${truncate(promptText, 600)}`);
      }
    }
    await control.waitFor(framesOf(agentResumed.pid, (entry) => userFrame(entry) && userFrameText(entry) === RESUME_SENTINEL), 1);
    control.push(emit(userReplay(RESUME_SENTINEL)));
    const resumedHistory = await waitForConversation(
      viewTwo.url,
      0,
      (data) => (data.events || []).some((event) => event.kind === "user_message" && event.text === RESUME_SENTINEL),
      "the message that asked for the resume to enter the history of the new conversation",
    );
    if (resumedHistory.conversation?.id !== resumedID) {
      throw new Error(`AC-4: the workspace holds ${JSON.stringify(resumedHistory.conversation?.id)} instead of the resumed conversation ${resumedID}`);
    }
    if ((resumedHistory.events || []).some((event) => event.text === AGENT_SENTINEL)) {
      throw new Error("AC-4: the past conversation was copied into the new one instead of being handed to it as context");
    }
    ok(
      "AC-4",
      `POST /api/workspace/conversations/${yesterdayID}/resume answered 201 with the new conversation ${resumedID} declaring resumed_from ${resumed.conversation.resumed_from}, the operating system reports the process of the live conversation (pid ${agentLive.pid}) gone, the new agent process (pid ${agentResumed.pid}) was really given both sentences of yesterday in its prompt, and the message that asked for the resume entered the history of the new conversation`,
    );

    // --- AC-6 ---------------------------------------------------------------
    await expectStatus(`${viewTwo.url}/api/workspaces`, 201, postJSON({ path: dirB }));
    const known = await apiJSON(`${viewTwo.url}/api/workspaces`);
    const entryB = await findByRealPath(known.workspaces, realB);
    if (!entryB) {
      throw new Error(`AC-6: the second workspace must be known: ${JSON.stringify(known.workspaces)}`);
    }
    await expectStatus(`${viewTwo.url}/api/workspaces/${encodeURIComponent(entryB.id)}/open`, 200, postJSON({}));
    assertEqual(viewTwo.child.pid, pidTwo, "the viewer PID after opening B");
    await waitForProcessGone(agentResumed.pid, `the agent process of A (pid ${agentResumed.pid}) to be released by the workspace switch`);

    await useFakeProvider(viewTwo.url);
    control.push(emit(initFrame("conversation-beta")));
    const openedInB = await apiJSON(`${viewTwo.url}/api/workspace/conversation`, postJSON({}), 201);
    const conversationB = openedInB.conversation?.id;
    if (!conversationB) {
      throw new Error(`AC-6: B must open a conversation of its own; got ${JSON.stringify(openedInB.conversation)}`);
    }
    const agentB = await control.waitFor("argv", 4);
    control.push(emit(assistantText(BETA_SENTINEL)));
    await waitForConversation(viewTwo.url, 0, (data) => (data.events || []).length === 1, "the history of the conversation of B");

    const rawIndexB = await rawGet(`${viewTwo.url}/api/workspace/conversations`);
    for (const [label, id] of [["of yesterday", yesterdayID], ["that was live", liveID], ["that resumed it", resumedID]]) {
      if (rawIndexB.includes(id)) {
        throw new Error(`AC-6: the conversation ${label} (${id}) of A appears in the index served for B: ${truncate(rawIndexB, 600)}`);
      }
    }
    const indexB = JSON.parse(rawIndexB);
    if (indexB.conversations.length !== 1 || indexB.conversations[0].id !== conversationB) {
      throw new Error(`AC-6: B must be served its own single conversation; got ${JSON.stringify(indexB.conversations)}`);
    }
    if (indexB.conversations[0].live !== true) {
      throw new Error(`AC-6: the conversation B is holding right now must be listed as live; got ${JSON.stringify(indexB.conversations[0])}`);
    }
    ok(
      "AC-6",
      `after POST /api/workspaces/{B}/open on the same pid ${pidTwo} the index answers with the single conversation ${conversationB} of B, and none of the three conversation ids of A appears anywhere in the ${rawIndexB.length}-byte response body`,
    );

    // --- AC-7 ---------------------------------------------------------------
    await expectStatus(`${viewTwo.url}/api/workspaces`, 201, postJSON({ path: dirC }));
    const knownWithC = await apiJSON(`${viewTwo.url}/api/workspaces`);
    const entryC = await findByRealPath(knownWithC.workspaces, realC);
    if (!entryC) {
      throw new Error(`AC-7: the third workspace must be known: ${JSON.stringify(knownWithC.workspaces)}`);
    }
    await expectStatus(`${viewTwo.url}/api/workspaces/${encodeURIComponent(entryC.id)}/open`, 200, postJSON({}));
    await waitForProcessGone(agentB.pid, `the agent process of B (pid ${agentB.pid}) to be released by the workspace switch`);

    const rawIndexC = await rawGet(`${viewTwo.url}/api/workspace/conversations`);
    if (!rawIndexC.includes('"conversations":[]')) {
      throw new Error(`AC-7: a workspace nobody has talked to must answer with an empty array and never with null; got ${truncate(rawIndexC, 400)}`);
    }
    const onDisk = await fs.readdir(path.join(dirC, ".archetipo", "conversations")).catch(() => []);
    if (onDisk.length !== 0) {
      throw new Error(`AC-7: the never-conversed workspace already holds records: ${JSON.stringify(onDisk)}`);
    }
    // "There are none" is only half of it: the empty state has to offer the
    // conversation it says does not exist yet.
    await useFakeProvider(viewTwo.url);
    const offer = await apiJSON(`${viewTwo.url}/api/workspace/conversation?after_id=0`, {}, 200);
    if (offer.available !== true || offer.conversation !== null) {
      throw new Error(`AC-7: the empty state must offer to start a conversation; got ${JSON.stringify({ available: offer.available, conversation: offer.conversation, reason: offer.unavailable_reason })}`);
    }
    ok(
      "AC-7",
      `on a workspace nobody has ever talked to GET /api/workspace/conversations answers 200 with the raw body ${JSON.stringify(rawIndexC.trim())}, no record exists under .archetipo/conversations, and GET /api/workspace/conversation answers available:true with conversation:null — there are none, and one can be started`,
    );

    assertAlive(viewTwo.child, "the viewer process at the end of the scenario");
  } finally {
    if (viewOne) await stopProcess(viewOne.child);
    if (viewTwo) await stopProcess(viewTwo.child);
    if (control) await control.close();
  }
}

// --- oracles ------------------------------------------------------------------

// assertSameEvents compares two histories element by element on what a reader
// would call the same conversation: the id, the kind and the text. It is
// deliberately not a JSON comparison of the whole event, because that would
// also assert the fields this claim is not about.
function assertSameEvents(expected, actual, label) {
  const got = actual || [];
  if (got.length !== expected.length) {
    throw new Error(`${label}: expected ${expected.length} events, got ${got.length}: ${truncate(JSON.stringify(got), 600)}`);
  }
  for (let i = 0; i < expected.length; i += 1) {
    for (const field of ["id", "kind", "text"]) {
      if (got[i][field] !== expected[i][field]) {
        throw new Error(
          `${label}: event ${i + 1} differs on ${field}\n  before: ${JSON.stringify(expected[i])}\n  after:  ${JSON.stringify(got[i])}`,
        );
      }
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

// waitForProcessGone polls the operating system until a process is no longer
// there. `kill(pid, 0)` sends nothing: it asks whether the process exists, and
// it is the only oracle that answers "it really ended" instead of "the route
// said so".
async function waitForProcessGone(pid, what, timeoutMs = 20000) {
  if (!Number.isInteger(pid)) {
    throw new Error(`no pid was reported for ${what}; got ${JSON.stringify(pid)}`);
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

// --- the protocol -------------------------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

function initFrame(sessionID) {
  return { type: "system", subtype: "init", session_id: sessionID };
}

function assistantText(text) {
  return { type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text }] } };
}

// userReplay is what `--replay-user-messages` produces on the real binary: the
// process re-emitting the message it was given, which is what puts it in the
// history.
function userReplay(text) {
  return { type: "user", message: { content: [{ type: "text", text }] }, isReplay: true };
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

// framesOf narrows a matcher to one agent process. Correlating by pid is what
// makes "the *new* process was given the transcript" a statement about that
// process and not about whichever process happened to be listening.
function framesOf(pid, matcher) {
  return function framesOfProcess(entry) {
    return entry.pid === pid && matcher(entry);
  };
}

// promptsCarryingTheTranscript is the negative half of the same claim: before
// anybody asked for a resume, no process may have been handed the conversation
// of yesterday.
function promptsCarryingTheTranscript(control) {
  return control.reports().filter((entry) => userFrame(entry) && userFrameText(entry).includes(AGENT_SENTINEL));
}

// --- the control server --------------------------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received, so every assertion above is a statement
// about a state the test produced. Several fake processes share it across the
// run — at most one is ever alive at a time here, because every switch and every
// resume waits for the previous process to be gone — and each report carries the
// pid of the process that made it.

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
        `Timed out after ${timeoutMs}ms waiting for the fake to report ${describe(matcher)} ${count} time(s); it reported ${JSON.stringify(received.map((entry) => `${entry.kind}@${entry.pid}`))}`,
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

// useFakeProvider points the open workspace at the fake agent through the very
// route the Execution panel uses, so every conversation starts from the state
// the UI produces. It is called again after every workspace switch and after
// every restart, because the selection belongs to the workspace and not to the
// viewer.
async function useFakeProvider(viewURL) {
  await apiJSON(`${viewURL}/api/execution/provider/default`, putJSON({
    id: "claude",
    config: { command: fakeClaudePath, timeout_seconds: 600 },
  }));
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

// --- reading the conversation ---------------------------------------------------

async function readConversation(viewURL, afterID) {
  return apiJSON(`${viewURL}/api/workspace/conversation?after_id=${afterID}`);
}

// waitForConversation polls one viewer route until it reports what the fake was
// just told. `what` is not decoration: a timeout has to say what it was waiting
// for and what arrived instead, or the failure names nothing.
async function waitForConversation(viewURL, afterID, predicate, what, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readConversation(viewURL, afterID);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(
    `Timed out after ${timeoutMs}ms waiting for ${what}; the last read at after_id=${afterID} was ${truncate(JSON.stringify(last), 600)}`,
  );
}

// settled starts a request now and lets it be awaited later, without a rejection
// going unhandled in between. The resume needs it: the route answers only once
// the new agent process has announced itself, and what makes it announce itself
// has to happen while the request is already in flight.
function settled(promise) {
  const outcome = promise.then(
    (value) => () => value,
    (error) => () => {
      throw error;
    },
  );
  return async () => (await outcome)();
}

// --- harness ---------------------------------------------------------------------

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
  console.log(`Smoke test for the past conversations of a workspace, against a fake agent binary

Usage:
  node ./test/e2e/conversation-history-view-smoke.mjs
  npm run test:view-conversation-history-smoke

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
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    delay(5000),
  ]);
  if (child.exitCode === null && child.signalCode === null) {
    child.kill("SIGKILL");
    await Promise.race([
      new Promise((resolve) => child.once("exit", resolve)),
      delay(3000),
    ]);
  }
}

function truncate(value, max = 200) {
  const text = String(value ?? "");
  return text.length <= max ? text : `${text.slice(0, max)}…`;
}

// --- report ---------------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "conversation-history-from-view",
    spec: "US-058",
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
  <title>ARchetipo Smoke — Past conversations of the workspace (US-058)</title>
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
      <h1>ARchetipo Smoke — Past conversations of the workspace (US-058)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Three real workspaces initialized with <code>archetipo init --tool claude --connector file</code>. A
    conversation is held on <code>A</code>, then the <code>archetipo view</code> process that held it is
    <em>killed</em> and a second one is started on the same directory: everything asserted afterwards is asserted
    on a different pid. That second process reads the index, reopens the past transcript, resumes it into a new
    conversation, switches to <code>B</code> and finally to a workspace nobody has ever talked to. Only the agent
    binary is fake: <code>test/e2e/support/fake-claude.mjs</code>, driven frame by frame through a local control
    server. The oracles are the record file on disk, the second process identifier, the prompt the resumed agent
    process was really given, and the end of a process asked of the operating system by pid.</p>

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
