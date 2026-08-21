#!/usr/bin/env node

// End-to-end smoke for "propose an action from the conversation" (US-054).
//
// Everything on the ARchetipo side is real: the CLI built from source,
// `archetipo view`, the filefs connector on disk, the local `claude` provider,
// its stream-json client, the conversation of US-053 and the five viewer routes
// it now exposes — `GET`, `POST`, `POST .../messages`,
// `POST .../proposal` and `DELETE` on `/api/workspace/conversation` — plus the
// board's own `POST /api/spec/{code}/execution`. Only the agent binary is
// replaced, by a Node script that speaks the same protocol on stdio, so nothing
// here needs a credential or a network.
//
// The oracles are deliberately the ones no viewer field can stand in for: the
// execution records that do or do not exist under `.archetipo/executions/`, the
// status of the spec as the connector wrote it in `.archetipo/specs/*.yaml`,
// the frames the agent process was really given, reported by the process
// itself, and — for the parity of AC-2 — a second workspace prepared
// identically where the very same action is started from the board instead of
// from a conversation.
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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "conversation-action-view-smoke");

// Two specs per workspace, and the difference between them is the whole point:
// the same proposed action is admitted on one and not on the other, because of
// the status each is in. RUNNABLE is planned and carries a persisted plan, so
// `implement` is a step the process really admits on it; BLOCKED stays in TODO,
// where the process admits `plan` and nothing else.
const CODE_RUNNABLE = "US-A01";
const CODE_BLOCKED = "US-B01";
const PROPOSED_ACTION = "implement";

// The message sent after a refusal: it has to reach the agent process itself,
// reported by the process, and not merely be accepted by the route.
const MESSAGE_SENTINEL = "smoke-message-after-the-declined-proposal";

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
    // Every process started from here writes the registry of known workspaces
    // inside the run directory, never in the real state of the machine.
    const env = {
      ...process.env,
      ARCHETIPO_DATA_DIR: repoRoot,
      ARCHETIPO_STATE_DIR: path.join(runDir, "state"),
    };
    const targetsDir = path.join(runDir, "targets");
    await fs.mkdir(targetsDir, { recursive: true });

    const dirConfirm = await createWorkspace(runDir, targetsDir, "conferma", env);
    const dirDecline = await createWorkspace(runDir, targetsDir, "rifiuto", env);

    const confirmed = await scenarioRefusedThenConfirmed(dirConfirm, env);
    await scenarioDeclinedAndBoardParity(dirDecline, env, confirmed);
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
  console.log(`\nPASS: conversation-action-view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// --- AC-3, then AC-1, AC-2, AC-5 ---------------------------------------------
//
// One workspace, one conversation, two proposals. The refused one comes first
// on purpose: at that point nothing has ever been started here, so "the refusal
// created nothing" is a statement about an empty filesystem and not about a
// difference between two states that both hold records.
async function scenarioRefusedThenConfirmed(dir, env) {
  let view;
  let control;
  try {
    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);
    view = await startViewServer(dir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
    console.log(`-> view ready: ${view.url} (launched in ${dir})`);

    // The provider is configured through the very route the Execution panel
    // uses, so both the conversation and the confirmed start begin from the
    // state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: fakeClaudePath, timeout_seconds: 600 },
    }));

    control.push(emit({ type: "system", subtype: "init", session_id: "conversation-conferma" }));
    const opened = await apiJSON(`${view.url}/api/workspace/conversation`, postJSON({}), 201);
    if (opened.available !== true || !opened.conversation?.id) {
      throw new Error(`the conversation did not open: ${JSON.stringify(opened)}`);
    }
    if (opened.proposal !== null) {
      throw new Error(`a freshly opened conversation has nothing to decide; got ${JSON.stringify(opened.proposal)}`);
    }
    await control.waitFor("argv", 1);

    // --- AC-3 ---------------------------------------------------------------
    // The agent proposes a step the process does not admit on this spec in this
    // status. The proposal must arrive resolved as unavailable, with the reason
    // of the process, and pressing it must be refused with that same reason.
    const listingBeforeRefusal = await listRecursively(dir);
    control.push(emit(assistantText(
      `Posso implementare ${CODE_BLOCKED} subito.`,
      PROPOSED_ACTION,
      CODE_BLOCKED,
    )));
    const blocked = await waitForProposal(view.url, (data) => data.proposal?.spec_code === CODE_BLOCKED, `the proposal on ${CODE_BLOCKED}`);
    const blockedProposal = blocked.proposal;
    if (blockedProposal.runnable !== false) {
      throw new Error(`AC-3: the proposal must not be runnable; got ${JSON.stringify(blockedProposal)}`);
    }
    if (blockedProposal.action !== PROPOSED_ACTION || blockedProposal.scope !== "spec") {
      throw new Error(`AC-3: the proposal must name the action and its scope; got ${JSON.stringify(blockedProposal)}`);
    }
    const blockedReason = String(blockedProposal.unavailable_reason || "");
    if (!blockedReason.includes(PROPOSED_ACTION) || !blockedReason.includes("TODO") || !blockedReason.includes(CODE_BLOCKED)) {
      throw new Error(`AC-3: the reason must name the action, the spec and the status it is in; got ${JSON.stringify(blockedReason)}`);
    }
    const refused = await expectStatus(
      `${view.url}/api/workspace/conversation/proposal`,
      409,
      postJSON({ proposal_id: blockedProposal.event_id, decision: "accept" }),
    );
    if (String(refused.error || "") !== blockedReason) {
      throw new Error(
        `AC-3: the refusal must be word for word the reason the card declared\n  card:    ${JSON.stringify(blockedReason)}\n  refusal: ${JSON.stringify(refused.error)}`,
      );
    }
    const recordsAfterRefusal = await listExecutionRecords(dir);
    if (recordsAfterRefusal.length !== 0) {
      throw new Error(`AC-3: the refused acceptance wrote ${recordsAfterRefusal.length} execution record(s): ${JSON.stringify(recordsAfterRefusal)}`);
    }
    if ((await readSpecStatus(dir, CODE_BLOCKED)) !== "TODO") {
      throw new Error(`AC-3: ${CODE_BLOCKED} moved on disk to ${await readSpecStatus(dir, CODE_BLOCKED)}`);
    }
    await assertSameListing("AC-3", listingBeforeRefusal, await listRecursively(dir), dir);
    ok(
      "AC-3",
      `the proposal of ${PROPOSED_ACTION} on ${CODE_BLOCKED} came back runnable:false stating ${JSON.stringify(truncate(blockedReason, 140))}, the acceptance answered 409 with the identical sentence, .archetipo/executions/ is still empty and the ${listingBeforeRefusal.length} paths of the workspace are unchanged`,
    );

    // --- AC-1 ---------------------------------------------------------------
    // The same action on the spec the process does admit. Between the proposal
    // arriving and the confirmation, the filesystem must be exactly as it was.
    control.push(emit(assistantText(
      `Allora implemento ${CODE_RUNNABLE}, che è pianificata.`,
      PROPOSED_ACTION,
      CODE_RUNNABLE,
    )));
    const pending = await waitForProposal(view.url, (data) => data.proposal?.spec_code === CODE_RUNNABLE, `the proposal on ${CODE_RUNNABLE}`);
    const proposal = pending.proposal;
    if (proposal.runnable !== true) {
      throw new Error(`AC-1: the proposal must be runnable; got ${JSON.stringify(proposal)}`);
    }
    if (proposal.unavailable_reason || proposal.unlocked_by) {
      throw new Error(`AC-1: a runnable proposal carries no reason; got ${JSON.stringify(proposal)}`);
    }
    if (proposal.spec_status !== "PLANNED" || !proposal.label) {
      throw new Error(`AC-1: the card must state what it is about; got ${JSON.stringify(proposal)}`);
    }
    const listingBeforeConfirmation = await listRecursively(dir);
    const recordsBefore = await listExecutionRecords(dir);
    if (recordsBefore.length !== 0) {
      throw new Error(`AC-1: a proposal must create nothing; got ${JSON.stringify(recordsBefore)}`);
    }
    const statusBefore = await readSpecStatus(dir, CODE_RUNNABLE);
    if (statusBefore !== "PLANNED") {
      throw new Error(`AC-1: ${CODE_RUNNABLE} must still be PLANNED before the confirmation; got ${statusBefore}`);
    }
    ok(
      "AC-1",
      `the agent proposed ${PROPOSED_ACTION} on ${CODE_RUNNABLE} and the viewer resolved it runnable:true, while .archetipo/executions/ holds no record and ${CODE_RUNNABLE} is still ${statusBefore} on disk — the proposal by itself started nothing`,
    );

    // --- AC-2, AC-5 ---------------------------------------------------------
    const confirmed = await apiJSON(
      `${view.url}/api/workspace/conversation/proposal`,
      postJSON({ proposal_id: proposal.event_id, decision: "accept" }),
      201,
    );
    if (confirmed.proposal !== null) {
      throw new Error(`AC-2: a decided proposal is no longer pending; got ${JSON.stringify(confirmed.proposal)}`);
    }
    const outcome = confirmed.outcome;
    if (!outcome || outcome.decision !== "confirmed" || outcome.proposal_id !== proposal.event_id) {
      throw new Error(`AC-5: the outcome must be about the proposal that was confirmed; got ${JSON.stringify(outcome)}`);
    }
    if (outcome.action !== PROPOSED_ACTION || outcome.spec_code !== CODE_RUNNABLE || outcome.scope !== "spec") {
      throw new Error(`AC-5: the outcome must name what was started; got ${JSON.stringify(outcome)}`);
    }
    if (!outcome.execution_id) {
      throw new Error(`AC-5: the outcome must name the execution it started; got ${JSON.stringify(outcome)}`);
    }
    const recordsAfter = await listExecutionRecords(dir);
    if (recordsAfter.length !== 1 || recordsAfter[0] !== `${outcome.execution_id}.json`) {
      throw new Error(
        `AC-5: the outcome names ${outcome.execution_id} but the filesystem holds ${JSON.stringify(recordsAfter)}`,
      );
    }
    const record = await readExecutionRecord(dir, outcome.execution_id);
    if (record.spec_code !== CODE_RUNNABLE || record.action !== PROPOSED_ACTION) {
      throw new Error(`AC-2: the record does not carry the proposed action on the proposed spec; got ${JSON.stringify(record)}`);
    }
    const statusAfter = await readSpecStatus(dir, CODE_RUNNABLE);
    if (statusAfter !== "IN PROGRESS") {
      throw new Error(`AC-2: the confirmation must move ${CODE_RUNNABLE} as a start does; got ${statusAfter}`);
    }
    if ((await readSpecStatus(dir, CODE_BLOCKED)) !== "TODO") {
      throw new Error(`AC-2: the confirmation touched the spec it was not about`);
    }
    // The record and the run it started are the only two things that may have
    // appeared: everything else in the workspace is exactly as it was.
    const listingAfter = await listRecursively(dir);
    const appeared = listingAfter.filter((entry) => !listingBeforeConfirmation.includes(entry));
    const executionsRel = path.join(".archetipo", "executions");
    const unexpected = appeared.filter((entry) => entry !== executionsRel && !entry.startsWith(`${executionsRel}${path.sep}`) && entry !== path.join(".archetipo", "specs", `${CODE_RUNNABLE}.yaml`));
    if (unexpected.length !== 0) {
      throw new Error(`AC-2: the confirmation created paths nobody asked for: ${JSON.stringify(unexpected)}`);
    }
    ok(
      "AC-2 / AC-5",
      `POST /api/workspace/conversation/proposal with "accept" answered 201 carrying outcome.execution_id ${outcome.execution_id}, that very id is the single record under .archetipo/executions/ — action ${record.action} on ${record.spec_code}, provider ${record.provider_id}, status ${record.status} — and ${CODE_RUNNABLE} moved from ${statusBefore} to ${statusAfter} on disk`,
    );

    // The conversation carries on after a confirmation: it did not become a run.
    const stillOpen = await readConversation(view.url);
    if (stillOpen.conversation?.state !== "ACTIVE") {
      throw new Error(`AC-2: the conversation must survive its own proposal; got ${JSON.stringify(stillOpen.conversation)}`);
    }

    return {
      record,
      statusBefore,
      statusAfter,
      executionID: outcome.execution_id,
    };
  } finally {
    if (view) await stopProcess(view.child);
    if (control) await control.close();
  }
}

// --- AC-4, and the board half of AC-2 ----------------------------------------
//
// A second workspace prepared exactly like the first. The proposal is declined,
// and then — on the same workspace, whose specs the refusal left untouched —
// the very same action is started from the board. What the board produces is
// what the confirmation of the first workspace had to produce: that comparison
// is the whole content of "the same start as the board".
async function scenarioDeclinedAndBoardParity(dir, env, confirmed) {
  let view;
  let control;
  try {
    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);
    view = await startViewServer(dir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
    console.log(`-> view ready: ${view.url} (launched in ${dir})`);

    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: fakeClaudePath, timeout_seconds: 600 },
    }));

    control.push(emit({ type: "system", subtype: "init", session_id: "conversation-rifiuto" }));
    const opened = await apiJSON(`${view.url}/api/workspace/conversation`, postJSON({}), 201);
    if (opened.available !== true || !opened.conversation?.id) {
      throw new Error(`AC-4: the conversation did not open: ${JSON.stringify(opened)}`);
    }
    await control.waitFor("argv", 1);

    const listingBefore = await listRecursively(dir);
    control.push(emit(assistantText(
      `Vuoi che implementi ${CODE_RUNNABLE}?`,
      PROPOSED_ACTION,
      CODE_RUNNABLE,
    )));
    const pending = await waitForProposal(view.url, (data) => data.proposal?.runnable === true, `the runnable proposal on ${CODE_RUNNABLE}`);
    const proposal = pending.proposal;

    const declined = await apiJSON(
      `${view.url}/api/workspace/conversation/proposal`,
      postJSON({ proposal_id: proposal.event_id, decision: "decline" }),
      200,
    );
    if (declined.proposal !== null) {
      throw new Error(`AC-4: a declined proposal must stop being pending; got ${JSON.stringify(declined.proposal)}`);
    }
    if (declined.outcome?.decision !== "declined" || declined.outcome.proposal_id !== proposal.event_id) {
      throw new Error(`AC-4: the outcome must record the refusal; got ${JSON.stringify(declined.outcome)}`);
    }
    if (declined.outcome.execution_id) {
      throw new Error(`AC-4: a refusal names no execution; got ${JSON.stringify(declined.outcome)}`);
    }
    if (declined.conversation?.state !== "ACTIVE") {
      throw new Error(`AC-4: the conversation must stay active after a refusal; got ${JSON.stringify(declined.conversation)}`);
    }
    const recordsAfterDecline = await listExecutionRecords(dir);
    if (recordsAfterDecline.length !== 0) {
      throw new Error(`AC-4: the refusal left ${recordsAfterDecline.length} execution record(s): ${JSON.stringify(recordsAfterDecline)}`);
    }
    if ((await readSpecStatus(dir, CODE_RUNNABLE)) !== "PLANNED") {
      throw new Error(`AC-4: ${CODE_RUNNABLE} moved on a refusal, to ${await readSpecStatus(dir, CODE_RUNNABLE)}`);
    }
    await assertSameListing("AC-4", listingBefore, await listRecursively(dir), dir);

    // The conversation really carries on: the oracle is the agent process
    // saying it was given the message, never the 202 of the route.
    await apiJSON(`${view.url}/api/workspace/conversation/messages`, postJSON({ message: MESSAGE_SENTINEL }), 202);
    // The first user frame carried the opening instruction; this is the second.
    const delivered = await control.waitFor(userFrame, 2);
    if (userFrameText(delivered) !== MESSAGE_SENTINEL) {
      throw new Error(`AC-4: the process received ${JSON.stringify(userFrameText(delivered))} instead of the sentinel`);
    }
    ok(
      "AC-4",
      `POST /api/workspace/conversation/proposal with "decline" answered 200 recording decision "declined" with no execution, .archetipo/executions/ stayed empty, ${CODE_RUNNABLE} stayed PLANNED, the ${listingBefore.length} paths of the workspace are unchanged, and the message sent afterwards was reported as received by the agent process itself`,
    );

    // --- AC-2, the parity ---------------------------------------------------
    // Same workspace shape, same spec, same status, same provider: the only
    // difference is who asked. What the board writes here is the yardstick for
    // what the confirmation wrote there.
    const fromBoard = await apiJSON(`${view.url}/api/spec/${CODE_RUNNABLE}/execution`, postJSON({ action: PROPOSED_ACTION }), 201);
    const boardRecord = await readExecutionRecord(dir, fromBoard.id);
    const boardStatus = await readSpecStatus(dir, CODE_RUNNABLE);
    const compared = ["spec_code", "action", "capability", "provider_id", "spec_status_before", "status"];
    for (const field of compared) {
      if (JSON.stringify(boardRecord[field]) !== JSON.stringify(confirmed.record[field])) {
        throw new Error(
          `AC-2: the confirmation and the board disagree on ${field}\n  conversation: ${JSON.stringify(confirmed.record[field])}\n  board:        ${JSON.stringify(boardRecord[field])}`,
        );
      }
    }
    if (JSON.stringify(boardRecord.model_choice) !== JSON.stringify(confirmed.record.model_choice)) {
      throw new Error(
        `AC-2: the confirmation and the board disagree on the model the run started with\n  conversation: ${JSON.stringify(confirmed.record.model_choice)}\n  board:        ${JSON.stringify(boardRecord.model_choice)}`,
      );
    }
    if (boardStatus !== confirmed.statusAfter) {
      throw new Error(
        `AC-2: the board left ${CODE_RUNNABLE} in ${boardStatus} while the confirmation left it in ${confirmed.statusAfter}`,
      );
    }
    if (boardRecord.id === confirmed.record.id) {
      throw new Error("AC-2: two distinct runs in two distinct workspaces cannot share an id");
    }
    ok(
      "AC-2",
      `starting ${PROPOSED_ACTION} on ${CODE_RUNNABLE} from the board of an identically prepared workspace produced a record agreeing field by field with the one the confirmation produced (${compared.join(", ")}, model_choice) and left the spec in the same ${boardStatus}`,
    );

    // --- the closing discipline of the existing smoke ------------------------
    const closed = await apiJSON(`${view.url}/api/workspace/conversation`, { method: "DELETE" }, 200);
    if (closed.conversation?.state !== "CLOSED") {
      throw new Error(`the close must report the state the session observed; got ${JSON.stringify(closed.conversation)}`);
    }
  } finally {
    if (view) await stopProcess(view.child);
    if (control) await control.close();
  }
}

// --- oracles ------------------------------------------------------------------

// readSpecStatus reads the status of a spec from the file the filefs connector
// wrote, and not from a viewer route: "the backlog on disk" is what a
// transition has to have changed, and a payload is a report about it.
async function readSpecStatus(dir, code) {
  const file = path.join(dir, ".archetipo", "specs", `${code}.yaml`);
  const raw = await fs.readFile(file, "utf8");
  for (const line of raw.split("\n")) {
    const match = /^status:\s*(.+?)\s*$/.exec(line);
    if (match) return match[1].replace(/^["']|["']$/g, "");
  }
  throw new Error(`no status in ${file}:\n${truncate(raw, 400)}`);
}

async function listExecutionRecords(root) {
  const entries = await fs.readdir(path.join(root, ".archetipo", "executions")).catch(() => []);
  return entries.filter((name) => name.endsWith(".json")).sort();
}

async function readExecutionRecord(root, id) {
  return JSON.parse(await fs.readFile(path.join(root, ".archetipo", "executions", `${id}.json`), "utf8"));
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

// --- the protocol -------------------------------------------------------------

function emit(frame) {
  return { kind: "emit", frame };
}

// assistantText is a message of the agent closed by the proposal line: a
// sentence a person reads, then the single JSON line
// execution.ParseActionProposal recognizes. The line is last because that is
// what the recognizer scans for, and the sentence is there because a proposal
// nobody can read is not a proposal.
function assistantText(sentence, action, specCode) {
  const line = JSON.stringify({ artifact: "action_proposal", action, ...(specCode ? { spec: specCode } : {}) });
  return { type: "assistant", message: { model: "claude-fake", content: [{ type: "text", text: `${sentence}\n${line}` }] } };
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

// --- the control server --------------------------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received, so every assertion above is a statement
// about a state the test produced. A confirmed proposal starts a second fake
// process — the run — which shares this server; nothing is ever queued for it,
// so it polls, receives no command and stays exactly where the start left it.

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

// --- fixtures -------------------------------------------------------------------

// createWorkspace initializes one real workspace holding the two specs the
// scenarios need: one planned, with a plan persisted through the CLI, and one
// left in TODO. Both workspaces the smoke uses are built by this one function,
// because the parity of AC-2 is only a comparison if the two sides start from
// the same state.
async function createWorkspace(runDir, targetsDir, name, env) {
  const dir = path.join(targetsDir, name);
  await fs.mkdir(dir, { recursive: true });
  await runCommand(`init-${name}`, cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: dir, env });

  const specsFile = path.join(runDir, `specs-${name}.json`);
  await fs.writeFile(specsFile, JSON.stringify({
    specs: [
      {
        code: CODE_RUNNABLE,
        title: `Spec pianificabile di ${name}`,
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "HIGH",
        points: 1,
        status: "TODO",
        body: `Story del workspace ${name} che sarà pianificata.`,
      },
      {
        code: CODE_BLOCKED,
        title: `Spec ancora da pianificare di ${name}`,
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: `Story del workspace ${name} che resta in TODO.`,
      },
    ],
  }, null, 2));
  await runCommand(`spec-add-${name}`, cliPath, ["spec", "add", "--file", specsFile], { cwd: dir, env });

  // `spec plan` persists the plan and moves the spec to PLANNED, which is the
  // status in which the process admits the action this smoke proposes. Doing it
  // through the CLI rather than by writing the files keeps the fixture a state
  // the product itself can produce.
  const planFile = path.join(runDir, `plan-${name}.json`);
  await fs.writeFile(planFile, JSON.stringify({
    plan_body: `Piano minimo per ${CODE_RUNNABLE}: nulla da costruire, serve solo che il piano esista.`,
    tasks: [
      {
        id: "TASK-01",
        title: "Task unico",
        type: "Impl",
        status: "TODO",
        body: "## Descrizione\n\nNessun lavoro reale: il piano esiste perché l'azione di implementazione lo richiede.\n",
      },
    ],
  }, null, 2));
  await runCommand(`spec-plan-${name}`, cliPath, ["spec", "plan", CODE_RUNNABLE, "--file", planFile], { cwd: dir, env });
  return dir;
}

// --- reading the conversation -----------------------------------------------------

async function readConversation(viewURL, afterID = 0) {
  return apiJSON(`${viewURL}/api/workspace/conversation?after_id=${afterID}`);
}

// waitForProposal polls the read route until the viewer reports the proposal the
// fake was just told to make. `what` is not decoration: a timeout has to say
// what it was waiting for and what arrived instead, or the failure names
// nothing.
async function waitForProposal(viewURL, predicate, what, timeoutMs = 60000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await readConversation(viewURL);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(
    `Timed out after ${timeoutMs}ms waiting for ${what}; the last read carried proposal ${truncate(JSON.stringify(last?.proposal), 400)}`,
  );
}

// --- harness ----------------------------------------------------------------------

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
  console.log(`Smoke test for the action proposed from a conversation, against a fake agent binary

Usage:
  node ./test/e2e/conversation-action-view-smoke.mjs
  npm run test:view-conversation-action-smoke

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

// --- report -----------------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "conversation-action-from-view",
    spec: "US-054",
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
  <title>ARchetipo Smoke — An action proposed from the conversation (US-054)</title>
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
      <h1>ARchetipo Smoke — An action proposed from the conversation (US-054)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Two real workspaces, prepared identically: one spec planned through <code>archetipo spec plan</code> and one
    left in <code>TODO</code>. Each is served by its own <code>archetipo view</code> process with the real local
    <code>claude</code> provider pointed at <code>test/e2e/support/fake-claude.mjs</code>, driven frame by frame
    through a local control server. In the first workspace the agent proposes an action the process does not admit —
    refused with its own sentence, creating nothing — and then one it does, which is confirmed. In the second the
    same proposal is declined, the conversation carries on, and the very same action is finally started from the
    board so the two records can be compared. The oracles are the execution records under
    <code>.archetipo/executions/</code>, the spec status as the connector wrote it in
    <code>.archetipo/specs/</code>, and the frames the agent process itself reports having been given.</p>

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
