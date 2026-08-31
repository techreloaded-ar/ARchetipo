#!/usr/bin/env node

// End-to-end smoke for "talk to a local run" (US-038 for Codex, US-039 for
// Claude).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the local execution provider, its protocol client, the
// local session, the server-side run follower and the four viewer routes
// (`GET /api/execution/{id}/run`, `POST .../run/messages`,
// `POST .../run/cancel`, and the provider list). Only the agent binary is
// replaced, by a Node script that speaks the same protocol on stdio, so the run
// needs no credential and no network.
//
// The script is written once and run once per local provider. That is the whole
// point: the two providers must behave the same way, so the proof is the *same*
// script producing equivalent results on both, not a second script written to
// match a second implementation. What each provider description carries is
// only the protocol: how its fake is driven, which frames it exchanges, and
// which configuration fields it declares — two agents speak two protocols and
// no amount of parameterization changes that. Every *assertion* is written
// once, in `runScript`, and every provider is held to it identically; a check
// that had to be weakened for one of them would be a failure of the story, not
// a difference to accommodate.
//
// The fake never progresses on its own: the test emits every frame, sends the
// message, cancels while the process resists, and ends the turn by hand, so
// each acceptance criterion is proved against a state the test commanded. There
// is no arbitrary sleep anywhere: the only waits poll a viewer route or the
// control server until it reports what the fake process was just told.

import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildCLI as buildCLIShared, createRunDir as createRunDirShared, makeRunCommand, readBody, stopProcess as stopProcessShared, waitForHTTP } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const fakeCodexPath = path.join(__dirname, "support", "fake-codex.mjs");
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "local-run-view-smoke");

const SPEC = "US-901";
const MESSAGE_SENTINEL = "smoke-operator-message-sentinel";
const UNKNOWN_EXECUTION = "EXEC-does-not-exist";

// Every viewer response body of the provider being exercised is kept, so the
// final check can prove no session material travelled to the browser on any
// route this run touched. It is emptied at the start of each provider's run:
// each sentinel is checked against the responses of the run that could have
// leaked it.
const viewerBodies = [];

// --- the two local providers ------------------------------------------------
//
// One entry per provider, and nothing about a provider anywhere else in this
// file. An entry says how its sandbox is initialized, how its process is
// configured, what its fake must be told to produce the six moments of the
// script, and how to recognize what that fake received. Everything the script
// asserts is written once, below, against these entries.

const PROVIDERS = [
  {
    id: "codex",
    // `archetipo init --tool <tool>` installs the planning skill where the
    // provider looks for it.
    tool: "codex",
    fake: fakeCodexPath,
    controlEnv: "FAKE_CODEX_CONTROL",
    // The sentinel stands for whatever authentication material lives in the
    // viewer's environment. The agent owns its own authentication, so nothing
    // of it may ever reach a viewer response or the workspace configuration.
    authEnv: "CODEX_FAKE_AUTH",
    authSentinel: "codex-session-material-DO-NOT-EXPOSE",
    configFields: "command,model,sandbox,timeout_seconds",
    providerConfig: { command: fakeCodexPath, timeout_seconds: 600 },
    // Nothing that names the agent's own session material may end up in the
    // workspace configuration.
    forbiddenInConfig: /codex_home|CODEX_HOME/,

    // The prompt the process was given, plus whatever else this protocol says
    // about how the session was opened.
    async awaitStart(control) {
      const turn = await control.waitFor("turn/start");
      const thread = control.first("thread/start");
      if (thread.params?.approvalPolicy !== "never" || thread.params?.sandbox !== "workspace-write") {
        throw new Error(`The session was not opened as configured: ${JSON.stringify(thread.params)}`);
      }
      return String(turn.params?.input?.[0]?.text || "");
    },

    // Codex answers `initialize` by itself; there is no further handshake for
    // the test to command.
    openSession() {},

    emitWork(control) {
      control.push(emitCodex("item/agentMessage/delta", { delta: "Analizzo la spec" }));
      control.push(emitCodex("item/started", { item: { type: "commandExecution", command: ["ls"], status: "inProgress" } }));
      control.push(emitCodex("item/completed", { item: { type: "commandExecution", command: ["ls"], status: "completed", exitCode: 0 } }));
      control.push(emitCodex("item/agentMessage/delta", { delta: " e leggo il backlog" }));
    },

    async awaitOperatorMessage(control) {
      const steered = await control.waitFor("turn/steer");
      if (steered.params?.expectedTurnId !== "turn-1") {
        throw new Error(`AC-3: the steer must name the turn in progress; got ${JSON.stringify(steered.params)}`);
      }
      return steered.text;
    },

    emitReplay(control, text) {
      control.push(emitCodex("item/started", { item: { type: "userMessage", content: [{ type: "text", text }] } }));
    },

    async awaitInterrupt(control) {
      await control.waitFor("turn/interrupt");
    },

    emitTurnEnd(control) {
      control.push(emitCodex("item/completed", { item: { type: "agentMessage", text: "interrotto prima di pianificare" } }));
      control.push(emitCodex("turn/completed", { turn: { id: "turn-1" } }));
    },
  },
  {
    id: "claude",
    // The claude provider refuses to spawn without the planning skill the
    // prompt invokes, and `--tool claude` is what installs it.
    tool: "claude",
    fake: fakeClaudePath,
    controlEnv: "FAKE_CLAUDE_CONTROL",
    authEnv: "CLAUDE_FAKE_AUTH",
    authSentinel: "claude-session-material-DO-NOT-EXPOSE",
    configFields: "command,model,permission_mode,timeout_seconds",
    providerConfig: { command: fakeClaudePath, timeout_seconds: 600 },
    forbiddenInConfig: /claude_home|CLAUDE_HOME|api_key|API_KEY/,

    async awaitStart(control) {
      // stream-json has no `initialize` call: how the session was opened is the
      // invocation itself, and the streaming flags are what make a live
      // dialogue possible at all.
      const invocation = await control.waitFor("argv");
      const argv = invocation.argv || [];
      for (const flag of ["--print", "--verbose", "--replay-user-messages", "--no-session-persistence"]) {
        if (!argv.includes(flag)) {
          throw new Error(`The session was not opened as configured, ${flag} is missing: ${JSON.stringify(argv)}`);
        }
      }
      for (const [flag, value] of [
        ["--input-format", "stream-json"],
        ["--output-format", "stream-json"],
        ["--permission-mode", "auto"],
      ]) {
        if (argv[argv.indexOf(flag) + 1] !== value) {
          throw new Error(`The session was not opened as configured, ${flag} is not ${value}: ${JSON.stringify(argv)}`);
        }
      }
      // The instruction travels inside the protocol, as the first user frame.
      const first = await control.waitFor(userFrame, 1);
      return userFrameText(first);
    },

    // The process announces itself, and only then is the dialogue attached to
    // the session. Emitting it is the test's decision, like every other frame.
    openSession(control) {
      control.push(emitClaude({ type: "system", subtype: "init", session_id: "session-1" }));
    },

    emitWork(control) {
      control.push(emitClaude({
        type: "assistant",
        message: { model: "claude-fake", content: [{ type: "text", text: "Analizzo la spec" }] },
      }));
      control.push(emitClaude({
        type: "assistant",
        message: { content: [{ type: "tool_use", id: "toolu_1", name: "Bash", input: { command: "ls" } }] },
      }));
      control.push(emitClaude({
        type: "user",
        message: { content: [{ tool_use_id: "toolu_1", type: "tool_result", content: "README.md", is_error: false }] },
      }));
      control.push(emitClaude({
        type: "assistant",
        message: { content: [{ type: "text", text: " e leggo il backlog" }] },
      }));
    },

    async awaitOperatorMessage(control) {
      // The first user frame carried the instruction; the operator's message is
      // the second one the process is given.
      const steered = await control.waitFor(userFrame, 2);
      if (steered.frame?.message?.role !== "user") {
        throw new Error(`AC-3: the message must reach the process as a user frame: ${JSON.stringify(steered.frame)}`);
      }
      return userFrameText(steered);
    },

    emitReplay(control, text) {
      control.push(emitClaude({
        type: "user",
        message: { content: [{ type: "text", text }] },
        isReplay: true,
      }));
    },

    async awaitInterrupt(control) {
      await control.waitFor((entry) => entry.frame?.type === "control_request" && entry.frame?.request?.subtype === "interrupt");
    },

    emitTurnEnd(control) {
      control.push(emitClaude({
        type: "result",
        subtype: "error_during_execution",
        is_error: true,
        result: "interrotto prima di pianificare",
      }));
    },
  },
];

function emitCodex(method, params) {
  return { kind: "emit", method, params };
}

function emitClaude(frame) {
  return { kind: "emit", frame };
}

// userFrame recognizes a frame the claude process was given on its standard
// input as an operator message, which is the shape the instruction and every
// later message share.
function userFrame(entry) {
  return entry.kind === "received" && entry.frame?.type === "user";
}

function userFrameText(entry) {
  return (entry.frame?.message?.content || []).map((block) => block.text || "").join("");
}

// --- the script, run once per provider --------------------------------------

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (process.platform === "win32") {
    console.log("SKIP: the fake binaries rely on a POSIX shebang");
    return;
  }
  const providers = selectProviders(options.provider);
  const runDir = await createRunDir(options.workspaceRoot);
  // Starting a viewer records its project root in the user-level registry of
  // known workspaces. This run directory is a throwaway, so the entry must go
  // with it instead of accumulating in the real registry of the machine.
  process.env.ARCHETIPO_STATE_DIR = path.join(runDir, "state");
  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const passed = [];
  try {
    for (const provider of providers) {
      console.log(`\n=== provider ${provider.id} ===`);
      await runScript(provider, runDir);
      passed.push(provider.id);
    }
  } finally {
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned workspace: ${runDir}`);
    }
  }

  console.log(`\nPASS: local-run-view smoke test completed for ${passed.join(" and ")}.`);
  for (const id of passed) {
    console.log(`  - ${id}: the same script, the same assertions, the same results`);
  }
  console.log(`Sandboxes: ${runDir}`);
}

async function runScript(provider, runDir) {
  const sandboxDir = path.join(runDir, `sandbox-${provider.id}`);
  const specsFile = path.join(runDir, `specs-${provider.id}.json`);
  const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, [provider.authEnv]: provider.authSentinel };
  viewerBodies.length = 0;

  await fs.mkdir(sandboxDir, { recursive: true });

  let view;
  let control;
  try {
    await runCommand("init", cliPath, ["init", "--tool", provider.tool, "--connector", "file", "--yes"], { cwd: sandboxDir, env });
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir, env });

    control = await startControlServer();
    console.log(`-> control server for the fake ${provider.id}: ${control.url}`);

    view = await startViewServer(sandboxDir, { ...env, [provider.controlEnv]: control.url });
    console.log(`-> view ready: ${view.url}`);

    // AC-1 — the provider list declares the dialogue next to the capabilities
    // the provider already exposes, and the configurable fields are exactly the
    // ones it accepts.
    const providers = await apiJSON(`${view.url}/api/execution/providers`);
    const listed = (providers.providers || []).find((entry) => entry.id === provider.id);
    if (!listed) {
      throw new Error(`AC-1: the ${provider.id} provider is not listed: ${JSON.stringify(providers.providers)}`);
    }
    const capabilities = listed.capabilities || [];
    if (!capabilities.includes("run.dialog") || !capabilities.includes("spec.plan")) {
      throw new Error(`AC-1: ${provider.id} must declare run.dialog beside spec.plan; got ${JSON.stringify(capabilities)}`);
    }
    const fields = (listed.config_fields || []).map((field) => field.name).sort();
    if (fields.join(",") !== provider.configFields) {
      throw new Error(`AC-1: unexpected ${provider.id} configuration fields: [${fields.join(", ")}]`);
    }
    console.log(`-> AC-1 ok: ${provider.id} declares [${capabilities.join(", ")}]`);

    // The workspace default is the real provider, pointed at the fake binary.
    // Saving it through the API is what a person does in the Execution panel,
    // so the run starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: provider.id, config: provider.providerConfig }),
    });

    const started = await apiJSON(`${view.url}/api/spec/${SPEC}/execution`, postJSON({ action: "plan" }), 201);
    if (started.status !== "RUNNING" || !started.id) {
      throw new Error(`Unexpected execution record on start: ${JSON.stringify(started)}`);
    }
    const executionID = started.id;
    const prompt = await provider.awaitStart(control);
    if (!prompt.includes(SPEC)) {
      throw new Error(`The prompt does not ask to plan ${SPEC}: ${JSON.stringify(prompt)}`);
    }
    provider.openSession(control);
    console.log(`-> execution ${executionID} opened a local session on the fake ${provider.id}`);

    // AC-2 — the history grows in order and the cursor returns only what is new.
    provider.emitWork(control);
    const four = await waitForRun(view.url, executionID, (data) => data.events.length === 4);
    assertEventIDs(four.events, [1, 2, 3, 4], "AC-2");
    if (four.run?.state !== "ACTIVE") {
      throw new Error(`AC-2: the run must be active while the agent works; got ${JSON.stringify(four.run)}`);
    }
    const kinds = four.events.map((event) => event.kind).join(",");
    if (kinds !== "text,tool_start,tool_end,text") {
      throw new Error(`AC-2: the frames were not translated; got [${kinds}]`);
    }
    // The seq of an event is served beside its kind, so it is part of what a
    // caller can compare between the two providers. Both number their first
    // turn 1: a provider that started at zero would announce itself just as
    // plainly as a provider-specific kind would.
    const seqs = four.events.map((event) => event.seq);
    if (seqs.some((seq) => seq !== 1)) {
      throw new Error(`AC-2: every event of the first turn must carry seq 1; got [${seqs.join(",")}]`);
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
    const delivered = await provider.awaitOperatorMessage(control);
    if (delivered !== MESSAGE_SENTINEL) {
      throw new Error(`AC-3: the process received ${JSON.stringify(delivered)} instead of the sentinel`);
    }
    const stillFour = await readRun(view.url, executionID, 0);
    assertEventIDs(stillFour.events, [1, 2, 3, 4], "AC-3 before the re-emission");
    provider.emitReplay(control, MESSAGE_SENTINEL);
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
    await provider.awaitInterrupt(control);
    if (!cancelling.run || cancelling.run.state !== "ACTIVE") {
      throw new Error(`AC-4: while the process is still alive the viewer must not close the run; got ${JSON.stringify(cancelling.run)}`);
    }
    const stillActive = await readRun(view.url, executionID, 0);
    if (stillActive.run.state !== "ACTIVE") {
      throw new Error(`AC-4: a cancelled run must stay as the process reports it; got ${JSON.stringify(stillActive.run)}`);
    }
    // Now the process really ends the turn, and only now may the state change.
    provider.emitTurnEnd(control);
    const closed = await waitForRun(view.url, executionID, (data) => data.run && data.run.state !== "ACTIVE");
    if (!closed.run.closed_at) {
      throw new Error(`AC-4: a run that ended must carry the instant it was observed; got ${JSON.stringify(closed.run)}`);
    }
    console.log(`-> AC-4 ok: the cancel reported ACTIVE, the state became ${closed.run.state} only when the process ended the turn`);

    // AC-5 — a command on a run this workspace does not hold, and a command on
    // a run that is over, are both refused with the reason, and the history
    // does not change by a single byte.
    const baseline = await readRun(view.url, executionID, 0);
    const commands = [
      ["messages", `run/messages`, postJSON({ message: "sei ancora lì?" })],
      ["cancel", `run/cancel`, { method: "POST" }],
    ];
    for (const [label, route, init] of commands) {
      const missing = await expectStatus(`${view.url}/api/execution/${UNKNOWN_EXECUTION}/${route}`, 404, init);
      if (!String(missing.error || "").trim()) {
        throw new Error(`AC-5: the ${label} refusal on an unknown execution must state a reason; got ${JSON.stringify(missing)}`);
      }
      const refusal = await expectStatus(`${view.url}/api/execution/${executionID}/${route}`, 409, init);
      if (!String(refusal.error || "").includes("run_not_active")) {
        throw new Error(`AC-5: the ${label} refusal must name run_not_active; got ${JSON.stringify(refusal)}`);
      }
    }
    const after = await readRun(view.url, executionID, 0);
    if (JSON.stringify(baseline) !== JSON.stringify(after)) {
      throw new Error(`AC-5: the projection changed across four refused commands\nbefore: ${JSON.stringify(baseline)}\nafter:  ${JSON.stringify(after)}`);
    }
    console.log("-> AC-5 ok: four refused commands answered 404 and 409 run_not_active and left the projection byte-for-byte identical");

    // AC-6 — nothing of the agent's own authentication material reaches the
    // browser or the workspace configuration.
    const leaked = viewerBodies.filter((body) => body.includes(provider.authSentinel));
    if (leaked.length) {
      throw new Error(`AC-6: the viewer echoed the session material in ${leaked.length} response(s)`);
    }
    const configBody = await fs.readFile(path.join(sandboxDir, ".archetipo", "config.yaml"), "utf8");
    if (configBody.includes(provider.authSentinel) || provider.forbiddenInConfig.test(configBody)) {
      throw new Error(`AC-6: the workspace configuration carries agent session material:\n${configBody}`);
    }
    console.log(`-> AC-6 ok: the sentinel is absent from all ${viewerBodies.length} viewer responses and from the configuration`);
    console.log(`-> ${provider.id}: PASS (sandbox ${sandboxDir})`);
  } finally {
    if (view) {
      await stopProcess(view.child);
    }
    if (control) {
      await control.close();
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
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received. That is what makes every assertion above
// a statement about a state the test produced.

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

  // A matcher is either the reported kind or a predicate over the whole
  // report, because what identifies a request differs from protocol to
  // protocol: Codex names its methods, while a stream-json frame is identified
  // by its own shape.
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
    first(matcher) {
      return received.find(matches(matcher)) || {};
    },
    // waitFor polls until the fake has reported at least `count` matching
    // requests, and returns the count-th of them. The count is explicit rather
    // than relative to a snapshot taken here, because a request can perfectly
    // well have arrived before the test got round to waiting for it.
    async waitFor(matcher, count = 1, timeoutMs = 30000) {
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matching = received.filter(matches(matcher));
        if (matching.length >= count) {
          return matching[count - 1];
        }
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
        body: "Story di test per il dialogo con una run locale.",
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
  const options = { workspaceRoot: defaultWorkspaceRoot, cleanup: false, provider: null };
  for (let i = 0; i < argv.length; i += 1) {
    switch (argv[i]) {
      case "--workspace-root":
        options.workspaceRoot = path.resolve(argv[++i]);
        break;
      case "--provider":
        options.provider = argv[++i];
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

function selectProviders(id) {
  if (!id) return PROVIDERS;
  const selected = PROVIDERS.filter((provider) => provider.id === id);
  if (!selected.length) {
    throw new Error(`Unknown provider ${JSON.stringify(id)}; known: ${PROVIDERS.map((provider) => provider.id).join(", ")}`);
  }
  return selected;
}

function printHelp() {
  console.log(`Smoke test for talking to a local run from archetipo view against a fake agent binary

The same script runs on every local provider: ${PROVIDERS.map((provider) => provider.id).join(", ")}.

Usage:
  node ./test/e2e/local-run-view-smoke.mjs
  npm run test:view-local-run-smoke
  npm run test:view-local-run-smoke -- --provider claude

Options:
  --provider <id>         Run the script for one provider only (${PROVIDERS.map((provider) => provider.id).join(" | ")})
  --workspace-root <dir>  Parent directory for the generated sandboxes
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
  return makeRunCommand()(label, command, args, options);
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
