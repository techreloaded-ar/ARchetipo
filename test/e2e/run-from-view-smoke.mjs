#!/usr/bin/env node

// End-to-end smoke for "interact with a remote run from the UI" (US-030).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the arcipelago execution provider, its SSE consumer,
// the server-side run follower and the four viewer routes
// (`GET /api/execution/{id}/run`, `POST .../run/messages`,
// `POST .../run/approvals/{approvalId}`, `POST .../run/cancel`). Only the
// ARcipelago hub is replaced, by a local Node server bound to 127.0.0.1, so the
// run needs no credential and no network.
//
// The hub never progresses on its own: the test emits every frame, drops the
// stream, opens the approval and closes the run by hand, so each of the six
// acceptance criteria is proved against a state the test commanded rather than
// against a timer it waited out. There is no arbitrary sleep anywhere: the only
// waits poll a viewer route until the projection reports what the hub was just
// told.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "run-from-view-smoke");

// The credential is a sentinel: the arcipelago provider must send it to the hub
// (which checks it, on the event stream too) and the viewer must never echo it
// back to the browser.
const TOKEN_SENTINEL = "smoke-secret-token-value";
const WORKSPACE_ID = "ws-smoke";
const SPEC = "US-901";
const RUN_ID = "run-1";
const APPROVAL_ID = "appr-1";
const APPROVAL_OPTION = "allow-once";
// A second, never-pending approval: AC-6 answers it while the hub is refusing,
// so nothing can be attributed to the approval really existing.
const MISSING_APPROVAL_ID = "appr-2";
// The operator message is a sentinel too: it must reach the hub, and it must
// not appear in the timeline until the hub republishes it as an event.
const MESSAGE_SENTINEL = "smoke-operator-message-sentinel";

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

    hub = await startFakeHub();
    console.log(`-> fake ARcipelago hub: ${hub.url}`);

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);

    // The workspace default provider is the real arcipelago one, pointed at the
    // local hub. Saving it through the API is what a user does in the Execution
    // panel, so the run starts from the state the UI produces. The timeout is
    // long on purpose: the remote task is never completed by this smoke, and a
    // dispatch that gave up mid-test would move the record under our feet.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: "arcipelago",
        config: {
          base_url: hub.url,
          workspace_id: WORKSPACE_ID,
          poll_interval_seconds: 1,
          timeout_seconds: 600,
        },
      }),
    });

    // The run is started from the UI exactly as US-029 does, then the hub hands
    // the remote task a run: that assignment is the only bridge from an
    // execution record to an interactive run.
    const started = await apiJSON(`${view.url}/api/spec/${SPEC}/execution`, postJSON({ action: "plan" }), 201);
    if (started.status !== "RUNNING" || !started.id) {
      throw new Error(`Unexpected execution record on start: ${JSON.stringify(started)}`);
    }
    const executionID = started.id;
    const remote = await hub.waitForTask(SPEC);
    hub.assignRun(remote.id);
    console.log(`-> execution ${executionID} bound to remote task ${remote.id}, run ${RUN_ID} active`);

    // The first read is what makes the viewer follow the run; nothing is emitted
    // before the stream is attached, so the duplicate of AC-1 is really carried
    // over a live connection and not silently skipped by a replay.
    const first = await waitForRun(view.url, executionID, (data) => data.run && data.run.state === "ACTIVE");
    if (first.run.run_id !== RUN_ID) {
      throw new Error(`Expected the viewer to follow ${RUN_ID}; got ${JSON.stringify(first.run)}`);
    }
    await hub.waitForStream();

    // AC-1 — ordered history, applied without visible duplication. Frame 3 is
    // sent twice on the live stream: the projection must keep one copy.
    hub.emit(textEvent("Analizzo la spec"));
    hub.emit(toolStartEvent("Read"));
    hub.emit(toolEndEvent("Read"));
    hub.resend(3);
    hub.emit(textEvent("Ho letto il file"));
    const four = await waitForRun(view.url, executionID, (data) => data.events.length === 4);
    assertEventIDs(four.events, [1, 2, 3, 4], "AC-1");
    if (four.last_id !== 4) {
      throw new Error(`AC-1: expected last_id 4; got ${four.last_id}`);
    }
    const tail = await readRun(view.url, executionID, 4);
    if (tail.events.length !== 0 || tail.last_id !== 4) {
      throw new Error(`AC-1: after_id=4 must be empty at last_id 4; got ${JSON.stringify(tail)}`);
    }
    console.log("-> AC-1 ok: events 1..4 in order, the re-sent frame 3 appears once, after_id=4 is empty");

    // AC-2 — after the stream falls the viewer resumes from what it already
    // holds. The oracle is the cursor the real provider sent to the hub on the
    // second subscription, not a shape the test invented.
    const beforeDrop = hub.subscriptions();
    if (beforeDrop.length !== 1 || beforeDrop[0].afterId !== 0) {
      throw new Error(`AC-2: expected exactly one subscription from 0 before the drop; got ${JSON.stringify(beforeDrop)}`);
    }
    hub.dropStream();
    hub.emit(thinkingEvent("Valuto le alternative"));
    hub.emit(turnEndEvent());
    const six = await waitForRun(view.url, executionID, (data) => data.events.length === 6);
    assertEventIDs(six.events, [1, 2, 3, 4, 5, 6], "AC-2");
    const afterDrop = hub.subscriptions();
    if (afterDrop.length < 2 || afterDrop[1].afterId !== 4) {
      throw new Error(`AC-2: the second subscription must resume from afterId=4; got ${JSON.stringify(afterDrop)}`);
    }
    console.log("-> AC-2 ok: the stream was resumed with afterId=4 and events 1..6 have no gap and no repetition");

    // AC-3 — a message is history only once the provider republishes it.
    const beforeMessages = hub.messages().length;
    const accepted = await apiJSON(
      `${view.url}/api/execution/${executionID}/run/messages`,
      postJSON({ message: MESSAGE_SENTINEL }),
      202,
    );
    if (JSON.stringify(accepted).includes(MESSAGE_SENTINEL)) {
      throw new Error("AC-3: the accepted message must not be echoed into the timeline before the hub republishes it");
    }
    const delivered = hub.messages();
    if (delivered.length !== beforeMessages + 1) {
      throw new Error(`AC-3: expected exactly one delivered message; got ${JSON.stringify(delivered)}`);
    }
    const last = delivered[delivered.length - 1];
    if (last.runId !== RUN_ID || last.message !== MESSAGE_SENTINEL) {
      throw new Error(`AC-3: the hub received ${JSON.stringify(last)} instead of the sentinel on ${RUN_ID}`);
    }
    hub.emit(userMessageEvent(MESSAGE_SENTINEL));
    const seven = await waitForRun(view.url, executionID, (data) => data.events.length === 7);
    assertEventIDs(seven.events, [1, 2, 3, 4, 5, 6, 7], "AC-3");
    const echoed = seven.events.filter((event) => event.kind === "user_message" && event.text === MESSAGE_SENTINEL);
    if (echoed.length !== 1) {
      throw new Error(`AC-3: the confirmed message must appear once; found ${echoed.length}`);
    }
    console.log("-> AC-3 ok: the message reached the hub, was absent from the 202 body and appears once after confirmation");

    // AC-4 — a pending approval is answered and the outcome is the one the
    // provider reports on re-read.
    //
    // The approval is opened on a healthy stream and nothing is disturbed: no
    // drop, no reconnection. That is the real shape of an approval — the agent
    // asks for a decision in the middle of a working run — and the projection
    // has to surface it on its own, because the event stream carries no
    // approval-bearing frame. A run that needed its connection to fail before
    // the card could appear would be a run nobody could answer.
    const subscriptionsBeforeApproval = hub.subscriptions().length;
    hub.setApprovals([pendingApproval()]);
    const pending = await waitForRun(view.url, executionID, (data) =>
      (data.approvals || []).some((approval) => approval.id === APPROVAL_ID),
    );
    if (hub.subscriptions().length !== subscriptionsBeforeApproval) {
      throw new Error(
        `AC-4: the approval must become visible without a reconnection; subscriptions went from ${subscriptionsBeforeApproval} to ${hub.subscriptions().length}`,
      );
    }
    const exposed = pending.approvals.find((approval) => approval.id === APPROVAL_ID);
    if (!Array.isArray(exposed.options) || exposed.options.length !== 2) {
      throw new Error(`AC-4: the approval must expose its two options; got ${JSON.stringify(exposed)}`);
    }
    if (exposed.options.map((option) => option.id).join(",") !== `${APPROVAL_OPTION},reject`) {
      throw new Error(`AC-4: the options must be carried verbatim; got ${JSON.stringify(exposed.options)}`);
    }
    const answered = await apiJSON(
      `${view.url}/api/execution/${executionID}/run/approvals/${APPROVAL_ID}`,
      postJSON({ option_id: APPROVAL_OPTION }),
      202,
    );
    const responses = hub.approvalResponses();
    if (responses.length !== 1 || responses[0].runId !== RUN_ID || responses[0].approvalId !== APPROVAL_ID || responses[0].optionId !== APPROVAL_OPTION) {
      throw new Error(`AC-4: the hub must have recorded ("${APPROVAL_ID}","${APPROVAL_OPTION}") on ${RUN_ID}; got ${JSON.stringify(responses)}`);
    }
    if ((answered.approvals || []).some((approval) => approval.id === APPROVAL_ID)) {
      throw new Error(`AC-4: the answer must reflect the re-read; the approval is still pending: ${JSON.stringify(answered.approvals)}`);
    }
    if (!answered.run || answered.run.state !== "ACTIVE") {
      throw new Error(`AC-4: the answer must report the run as the hub sees it; got ${JSON.stringify(answered.run)}`);
    }
    console.log(`-> AC-4 ok: ${APPROVAL_ID} was exposed with two options, answered "${APPROVAL_OPTION}" at the hub and is no longer pending`);

    // AC-5 — cancelling reports the state the hub confirms, and a run that has
    // ended is never reopened.
    const cancelling = await apiJSON(`${view.url}/api/execution/${executionID}/run/cancel`, { method: "POST" }, 202);
    if (hub.cancels().length !== 1 || hub.cancels()[0].runId !== RUN_ID) {
      throw new Error(`AC-5: the hub must have recorded exactly one cancel on ${RUN_ID}; got ${JSON.stringify(hub.cancels())}`);
    }
    if (!cancelling.run || cancelling.run.state !== "ACTIVE") {
      throw new Error(`AC-5: while the hub still reports the run active the viewer must not close it; got ${JSON.stringify(cancelling.run)}`);
    }
    hub.closeRun();
    const closed = await waitForRun(view.url, executionID, (data) => data.run && data.run.state === "CLOSED");
    if (!closed.run.closed_at) {
      throw new Error(`AC-5: a closed run must carry the instant the hub reported; got ${JSON.stringify(closed.run)}`);
    }
    const refusedCancel = await expectStatus(`${view.url}/api/execution/${executionID}/run/cancel`, 409, { method: "POST" });
    if (!String(refusedCancel.error || "").includes("run_not_active")) {
      throw new Error(`AC-5: the refusal must name run_not_active; got ${JSON.stringify(refusedCancel)}`);
    }
    const stillClosed = await readRun(view.url, executionID, 0);
    if (!stillClosed.run || stillClosed.run.state !== "CLOSED" || hub.cancels().length !== 1) {
      throw new Error(`AC-5: a refused cancel must not reopen the run; got ${JSON.stringify(stillClosed.run)}`);
    }
    console.log("-> AC-5 ok: the cancel reported the hub state, the closure was confirmed by the hub and the refused retry left it CLOSED");

    // AC-6 — a refused command changes nothing locally. The oracle is the whole
    // snapshot, captured once and compared after each of the six refusals.
    const baseline = await readRun(view.url, executionID, 0);
    for (const [mode, status] of [["not_found", 404], ["run_not_active", 409]]) {
      hub.setRefusal(mode);
      for (const [label, url, init] of [
        ["messages", `${view.url}/api/execution/${executionID}/run/messages`, postJSON({ message: "rifiutato" })],
        ["approvals", `${view.url}/api/execution/${executionID}/run/approvals/${MISSING_APPROVAL_ID}`, postJSON({ option_id: APPROVAL_OPTION })],
        ["cancel", `${view.url}/api/execution/${executionID}/run/cancel`, { method: "POST" }],
      ]) {
        const refusal = await expectStatus(url, status, init);
        if (!String(refusal.error || "").includes(mode)) {
          throw new Error(`AC-6: the ${label} refusal must name ${mode}; got ${JSON.stringify(refusal)}`);
        }
        assertSameProjection(baseline, await readRun(view.url, executionID, 0), `AC-6 after a ${mode} refusal of ${label}`);
      }
    }
    hub.setRefusal(null);
    console.log("-> AC-6 ok: six refused commands (404 and run_not_active on the three commands) left the projection identical");

    // The credential travelled to the hub — including on the event stream — and
    // never to the browser.
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

    console.log("\nPASS: run-from-view smoke test completed.");
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

function postJSON(payload) {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  };
}

// --- agent events --------------------------------------------------------
//
// The payloads are the hub's own AgentEvent shapes: the provider translates
// them, and asserting on the translated kind is what proves the translation is
// exercised rather than bypassed.

function textEvent(delta) {
  return { type: "message_update", assistantMessageEvent: { type: "text_delta", delta } };
}

function thinkingEvent(delta) {
  return { type: "message_update", assistantMessageEvent: { type: "thinking_delta", delta } };
}

function toolStartEvent(toolName) {
  return { type: "tool_execution_start", toolName };
}

function toolEndEvent(toolName) {
  return { type: "tool_execution_end", toolName, isError: false };
}

function turnEndEvent() {
  return { type: "message_end", message: { stopReason: "end_turn" } };
}

function userMessageEvent(text) {
  return { type: "user_message", text };
}

function pendingApproval() {
  return {
    id: APPROVAL_ID,
    runId: RUN_ID,
    runnerId: "runner-1",
    createdAt: Date.now(),
    request: {
      toolName: "Bash",
      title: "Eseguire la suite di test",
      args: { command: "npm test" },
      options: [
        { optionId: APPROVAL_OPTION, name: "Consenti una volta", kind: "allow" },
        { optionId: "reject", name: "Rifiuta", kind: "reject" },
      ],
    },
  };
}

// --- fake ARcipelago hub ------------------------------------------------

// startFakeHub serves the task routes the arcipelago client already used plus
// the run namespace US-030 needs. Nothing progresses on its own: the test emits
// every frame, drops the stream, opens the approval and closes the run, so the
// smoke never waits for a timer.
async function startFakeHub() {
  const tasks = new Map(); // task id -> record
  const bySpec = new Map(); // spec code -> task id
  let created = 0;
  let authorized = 0;
  let unauthorized = 0;

  const run = { id: "", taskId: "", state: "active", createdAt: Date.now(), closedAt: 0, error: "" };
  const history = []; // every frame ever emitted, so a reconnection can replay
  const subscribed = []; // { afterId, lastEventId } per subscription: the AC-2 oracle
  const messages = [];
  const approvalResponses = [];
  const cancels = [];
  let approvals = [];
  let stream = null; // the currently attached SSE response, at most one
  let refusal = null; // null | "not_found" | "run_not_active"

  const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://127.0.0.1");
    if (req.headers.authorization !== `Bearer ${TOKEN_SENTINEL}`) {
      unauthorized += 1;
      return sendJSON(res, 401, { error: "unauthorized" });
    }
    authorized += 1;

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
        const record = { id, status: "running", resultSummary: "", runId: "", specCode, request };
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
      if (req.method === "POST" && rest === "/messages") {
        return readBody(req).then((raw) => {
          if (refuseCommand(res)) return undefined;
          let body = {};
          try {
            body = JSON.parse(raw || "{}");
          } catch {
            return sendJSON(res, 400, { error: "invalid_json" });
          }
          messages.push({ runId: runID, message: body.message });
          return sendJSON(res, 202, { ok: true });
        });
      }
      const respondRoute = /^\/approvals\/([^/]+)\/respond$/.exec(rest);
      if (req.method === "POST" && respondRoute) {
        const approvalID = decodeURIComponent(respondRoute[1]);
        return readBody(req).then((raw) => {
          if (refuseCommand(res)) return undefined;
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
      if (req.method === "POST" && rest === "/cancel") {
        if (refuseCommand(res)) return undefined;
        cancels.push({ runId: runID });
        return sendJSON(res, 202, { ok: true });
      }
      return sendJSON(res, 404, { error: "not_found" });
    }

    return sendJSON(res, 404, { error: "not_found" });
  });

  // refuseCommand answers a command the run cannot accept. The explicit mode is
  // what AC-6 drives; a run that is no longer active refuses on its own, which
  // is what AC-5 relies on for the retried cancel.
  function refuseCommand(res) {
    if (refusal === "not_found") {
      sendJSON(res, 404, { error: "run_not_found" });
      return true;
    }
    if (refusal === "run_not_active" || run.state !== "active") {
      sendJSON(res, 409, { error: "run_not_active" });
      return true;
    }
    return false;
  }

  // openStream serves the SSE history. It records the cursor the subscriber
  // asked for — `afterId`, or `Last-Event-ID` when a client uses the transport
  // header instead — and replays only what comes after it.
  function openStream(req, res, url) {
    const afterParam = url.searchParams.get("afterId");
    const headerCursor = req.headers["last-event-id"];
    const afterId = Number.parseInt(afterParam ?? headerCursor ?? "0", 10) || 0;
    subscribed.push({ afterId, lastEventId: headerCursor ?? null });

    res.writeHead(200, {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    });
    for (const frame of history) {
      if (frame.id > afterId) res.write(frameText(frame));
    }
    if (run.state !== "active") {
      // A run that is over ends its stream instead of holding the socket open,
      // which is how the follower learns the run is terminal.
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

  // assignRun is what turns an observable task into a followable run: until the
  // task carries a runId, ResolveRun has nothing to resolve.
  function assignRun(taskID) {
    const record = tasks.get(taskID);
    if (!record) throw new Error(`No remote task ${taskID} to bind a run to`);
    record.runId = RUN_ID;
    run.id = RUN_ID;
    run.taskId = taskID;
    run.state = "active";
    run.createdAt = Date.now();
    return run;
  }

  async function waitForStream(timeoutMs = 20000) {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      if (stream) return stream;
      await delay(50);
    }
    throw new Error("The provider never subscribed to the run event stream");
  }

  // emit appends one frame to the history and publishes it when a subscriber is
  // attached. A frame emitted while nobody listens is not lost: the next
  // subscription replays it from its cursor, exactly as the real hub does.
  function emit(event) {
    const frame = { id: history.length + 1, runId: run.id, seq: history.length + 1, ts: Date.now(), event };
    history.push(frame);
    if (stream) stream.write(frameText(frame));
    return frame;
  }

  // resend republishes a frame already in the history under its own id. It is
  // the duplicate AC-1 must not render twice.
  function resend(id) {
    const frame = history.find((candidate) => candidate.id === id);
    if (!frame) throw new Error(`No frame ${id} to resend`);
    if (!stream) throw new Error(`No attached stream to resend frame ${id} on`);
    stream.write(frameText(frame));
    return frame;
  }

  // dropStream destroys the socket without terminating the chunked body, so the
  // consumer sees a broken stream and not a clean end. A clean end would mean
  // "the run is over", which is a different situation entirely.
  function dropStream() {
    if (!stream) throw new Error("No attached stream to drop");
    const res = stream;
    stream = null;
    res.destroy();
  }

  // closeRun is the hub's statement that the run is terminal: the state changes
  // and the stream ends with the terminal frame.
  function closeRun() {
    run.state = "closed";
    run.closedAt = Date.now();
    if (stream) {
      const res = stream;
      stream = null;
      res.write(endFrameText("closed"));
      res.end();
    }
  }

  return {
    url,
    waitForTask,
    assignRun,
    waitForStream,
    emit,
    resend,
    dropStream,
    closeRun,
    setApprovals: (list) => {
      approvals = list;
    },
    setRefusal: (mode) => {
      refusal = mode;
    },
    subscriptions: () => subscribed.map((entry) => ({ ...entry })),
    messages: () => messages.map((entry) => ({ ...entry })),
    approvalResponses: () => approvalResponses.map((entry) => ({ ...entry })),
    cancels: () => cancels.map((entry) => ({ ...entry })),
    createdCount: () => created,
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

function frameText(frame) {
  return `id: ${frame.id}\nevent: run_event\ndata: ${JSON.stringify(frame)}\n\n`;
}

function endFrameText(reason) {
  return `event: end\ndata: ${JSON.stringify({ reason })}\n\n`;
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
        code: SPEC,
        title: "Smoke spec seguita dalla UI",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per l'interazione con una run remota dalla UI.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

// --- oracles -------------------------------------------------------------

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

// assertSameProjection compares the two snapshots on everything a refused
// command could have touched: the history, the cursor, the run state and the
// pending approvals.
function assertSameProjection(before, after, label) {
  const shape = (data) => ({
    ids: (data.events || []).map((event) => event.id),
    lastID: data.last_id,
    run: data.run ? { run_id: data.run.run_id, state: data.run.state, closed_at: data.run.closed_at } : null,
    approvals: (data.approvals || []).map((approval) => approval.id),
  });
  const left = JSON.stringify(shape(before));
  const right = JSON.stringify(shape(after));
  if (left !== right) {
    throw new Error(`${label}: the projection changed\nbefore: ${left}\nafter:  ${right}`);
  }
}

async function readRun(viewURL, executionID, afterID) {
  return apiJSON(`${viewURL}/api/execution/${executionID}/run?after_id=${afterID}`);
}

// waitForRun polls the viewer until the projection reports what the hub was
// just told. It replaces every sleep this test could otherwise have needed.
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

// --- harness -------------------------------------------------------------

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for interacting with a remote run from archetipo view against a fake local ARcipelago hub

Usage:
  node ./test/e2e/run-from-view-smoke.mjs
  npm run test:view-run-smoke

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
