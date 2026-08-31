#!/usr/bin/env node

// End-to-end smoke for "choose model and options for a single run" (US-049).
//
// Everything on the ARchetipo side is real: the CLI binary, `archetipo view`,
// the filefs connector, the local `claude` provider, its stream-json client,
// the local session, the run follower and the viewer routes this story added
// (`GET /api/execution/model-choice`, the two start routes now accepting
// `model` and `model_options`, and the `model_choice` the execution record
// carries). Only the agent binary is replaced, by a Node script that speaks the
// same protocol on stdio, so the run needs no credential and no network.
//
// The oracle of this smoke is deliberately the *argv the process really
// received*, not a viewer field echoing back what was posted. `buildArgs` in
// cli/internal/execution/claude/prompt.go appends `--model` and `--effort` only
// when the corresponding value is non-empty, so the argument list is an exact
// statement of which model and which option reached the agent: a per-run choice
// that had silently fallen back to the workspace configuration would show up as
// `sonnet`/`high` in that list, and no assertion on a JSON body could tell the
// two apart as plainly.
//
// The fake never progresses on its own: the test emits every frame and ends
// every turn by hand, so each acceptance criterion is proved against a state the
// test commanded. There is no arbitrary sleep anywhere: the only waits poll a
// viewer route or the control server until it reports what the fake process was
// just told.

import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildCLI as buildCLIShared, createRunDir as createRunDirShared, escapeHTML, makeRunCommand, parseCommonArgs, readBody, stopProcess as stopProcessShared, waitForHTTP } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "run-model-choice-view-smoke");

const SPEC = "US-901";
// The model and option the workspace configures, and the ones a single run
// picks instead. They are deliberately disjoint in both fields, so the argv can
// be read as an exclusive statement: what the run used is present, what the
// workspace configures is absent.
const WORKSPACE_MODEL = "sonnet";
const WORKSPACE_EFFORT = "high";
const RUN_MODEL = "opus";
const RUN_EFFORT = "low";
const MESSAGE_SENTINEL = "smoke-operator-message-sentinel";
// The sentinel stands for whatever authentication material lives in the
// viewer's environment. The agent owns its own authentication, so nothing of it
// may ever reach a viewer response or the workspace configuration.
const AUTH_ENV = "CLAUDE_FAKE_AUTH";
const AUTH_SENTINEL = "claude-session-material-DO-NOT-EXPOSE";

// Every viewer response body is kept, so the final check can prove no session
// material travelled to the browser on any route this run touched.
const viewerBodies = [];
// One entry per proved statement, for the report.
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
  // Starting a viewer records its project root in the user-level registry of
  // known workspaces. This run directory is a throwaway, so the entry must go
  // with it instead of accumulating in the real registry of the machine.
  process.env.ARCHETIPO_STATE_DIR = path.join(runDir, "state");
  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const startedAt = Date.now();
  let failure = null;
  try {
    await scenarioPerRunChoiceOverridesOnlyThatRun(runDir);
  } catch (error) {
    failure = error;
  }

  await writeReport(runDir, { startedAt, durationMs: Date.now() - startedAt, failure });

  if (failure) {
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
    }
    throw failure;
  }
  if (options.cleanup) {
    await fs.rm(runDir, { recursive: true, force: true });
    console.log(`-> cleaned workspace: ${runDir}`);
  }
  console.log(`\nPASS: run-model-choice-view smoke test completed (${checks.length} statements proved).`);
  console.log(`Sandbox: ${runDir}`);
}

// The whole story on one sandbox, because it is one story: the model shown
// before starting, the model a single run overrides, the record that remembers
// it, the configuration that did not move, and the next run that starts again
// from the workspace. Splitting it would lose exactly what it proves — that the
// second run is unaffected by the first.
async function scenarioPerRunChoiceOverridesOnlyThatRun(runDir) {
  const sandboxDir = path.join(runDir, "sandbox");
  const specsFile = path.join(runDir, "specs.json");
  const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, [AUTH_ENV]: AUTH_SENTINEL };

  await fs.mkdir(sandboxDir, { recursive: true });

  let view;
  let control;
  try {
    // 1 — a real sandbox with the planning skill the prompt invokes, and one
    // TODO spec to plan.
    await runCommand("init", cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: sandboxDir, env });
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir, env });

    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);

    view = await startViewServer(sandboxDir, { ...env, FAKE_CLAUDE_CONTROL: control.url });
    console.log(`-> view ready: ${view.url}`);

    // 2 — the workspace default is the real claude provider pointed at the
    // fake binary, configured with a model and an option. Saving it through the
    // API is what a person does in the Execution panel, so every later run
    // starts from the state the UI produces.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: "claude",
        config: {
          command: fakeClaudePath,
          timeout_seconds: 600,
          model: WORKSPACE_MODEL,
          effort: WORKSPACE_EFFORT,
        },
      }),
    });

    // 3 — AC-1: before anything is started, the viewer says which model the run
    // would use and where it comes from, next to the alternatives.
    const choice = await apiJSON(`${view.url}/api/execution/model-choice`);
    if (choice.available !== true) {
      throw new Error(`AC-1: the catalog must be available; got ${JSON.stringify(choice)}`);
    }
    if (choice.provider_id !== "claude" || choice.model_field !== "model") {
      throw new Error(`AC-1: the view must name the provider and the model field; got ${JSON.stringify(choice)}`);
    }
    if (choice.model !== WORKSPACE_MODEL || choice.model_source !== "workspace") {
      throw new Error(`AC-1: the inherited model must be ${WORKSPACE_MODEL} from the workspace; got ${JSON.stringify(choice)}`);
    }
    if (JSON.stringify(choice.options || {}) !== JSON.stringify({ effort: WORKSPACE_EFFORT })) {
      throw new Error(`AC-1: the inherited options must be the configured ones; got ${JSON.stringify(choice.options)}`);
    }
    const catalog = choice.models || [];
    for (const id of ["opus", "sonnet", "haiku"]) {
      if (!catalog.some((entry) => entry.id === id)) {
        throw new Error(`AC-1: the catalog must offer ${id}; got ${JSON.stringify(catalog.map((entry) => entry.id))}`);
      }
    }
    const opus = catalog.find((entry) => entry.id === RUN_MODEL);
    const effort = (opus.options || []).find((option) => option.name === "effort");
    if (!effort || !(effort.choices || []).some((entryChoice) => entryChoice.value === RUN_EFFORT)) {
      throw new Error(`AC-1: ${RUN_MODEL} must declare the effort option with ${RUN_EFFORT}; got ${JSON.stringify(opus.options)}`);
    }
    ok("AC-1", `the start reports model ${WORKSPACE_MODEL} inherited from the workspace with {effort: ${WORKSPACE_EFFORT}}, beside a catalog offering opus, sonnet and haiku`);

    // 4 — the configuration file as it stands before any run. AC-4 is proved
    // against these exact bytes, not against a re-reading of the same fields.
    const configPath = path.join(sandboxDir, ".archetipo", "config.yaml");
    const configBefore = await fs.readFile(configPath);

    // 5 — AC-2 and AC-3: a run started with an explicit choice.
    const overridden = await apiJSON(
      `${view.url}/api/spec/${SPEC}/execution`,
      postJSON({ action: "plan", model: RUN_MODEL, model_options: { effort: RUN_EFFORT } }),
      201,
    );
    if (overridden.status !== "RUNNING" || !overridden.id) {
      throw new Error(`AC-2: unexpected execution record on start: ${JSON.stringify(overridden)}`);
    }
    const overriddenID = overridden.id;
    const overriddenArgv = (await control.waitFor("argv", 1)).argv || [];
    assertFlagValue(overriddenArgv, "--model", RUN_MODEL, "AC-2");
    assertFlagValue(overriddenArgv, "--effort", RUN_EFFORT, "AC-2");
    if (overriddenArgv.includes(WORKSPACE_MODEL) || overriddenArgv.includes(WORKSPACE_EFFORT)) {
      throw new Error(`AC-2: nothing of the workspace configuration may reach a run that overrode it; got ${JSON.stringify(overriddenArgv)}`);
    }
    ok("AC-2", `the process was invoked with --model ${RUN_MODEL} --effort ${RUN_EFFORT}, and neither ${WORKSPACE_MODEL} nor ${WORKSPACE_EFFORT} appears anywhere in its argv`);

    control.push(emitClaude({ type: "system", subtype: "init", session_id: "session-1" }));
    const openRecord = await apiJSON(`${view.url}/api/execution/${overriddenID}`);
    if (openRecord.status !== "RUNNING") {
      throw new Error(`AC-3: the record must still be open when it is read; got ${JSON.stringify(openRecord.status)}`);
    }
    assertModelChoice(openRecord.model_choice, { model: RUN_MODEL, options: { effort: RUN_EFFORT }, source: "run" }, "AC-3 while running");

    // 6 — the turn ends. A `plan` that persisted no plan closes FAILED, and
    // that is expected here: the subject of this smoke is which model reached
    // the process, not whether a plan exists — the fake never plans anything.
    // What matters is that the terminal write does not lose the choice.
    control.push(emitClaude({
      type: "result",
      subtype: "error_during_execution",
      is_error: true,
      result: "il finto non pianifica nulla",
    }));
    const closed = await waitForExecution(view.url, overriddenID, (record) => record.status !== "RUNNING");
    assertModelChoice(closed.model_choice, { model: RUN_MODEL, options: { effort: RUN_EFFORT }, source: "run" }, "AC-3 after the terminal write");
    ok("AC-3", `the record carried {model: ${RUN_MODEL}, options: {effort: ${RUN_EFFORT}}, source: run} while the run was open and still carries it now that it is ${closed.status}`);

    // 7 — AC-4: the workspace did not move. Byte-for-byte on the file, and the
    // same two values served back by the panel's own route.
    const configAfter = await fs.readFile(configPath);
    if (!configBefore.equals(configAfter)) {
      throw new Error(
        `AC-4: a per-run choice must not touch the workspace configuration\nbefore:\n${configBefore.toString("utf8")}\nafter:\n${configAfter.toString("utf8")}`,
      );
    }
    const providers = await apiJSON(`${view.url}/api/execution/providers`);
    const persisted = providers.default || {};
    if (persisted.id !== "claude" || persisted.config?.model !== WORKSPACE_MODEL || persisted.config?.effort !== WORKSPACE_EFFORT) {
      throw new Error(`AC-4: the persisted default must be unchanged; got ${JSON.stringify(persisted)}`);
    }
    ok("AC-4", "`.archetipo/config.yaml` is byte-for-byte identical after the overridden run, and the default provider still reads model sonnet with effort high");

    // 8 — AC-5: a run started with neither field is the run the workspace
    // configures, right down to the argv.
    const inherited = await apiJSON(`${view.url}/api/spec/${SPEC}/execution`, postJSON({ action: "plan" }), 201);
    if (inherited.status !== "RUNNING" || !inherited.id) {
      throw new Error(`AC-5: unexpected execution record on start: ${JSON.stringify(inherited)}`);
    }
    const inheritedID = inherited.id;
    if (inheritedID === overriddenID) {
      throw new Error("AC-5: the second start must be a new execution, not the previous record");
    }
    const inheritedArgv = (await control.waitFor("argv", 2)).argv || [];
    assertFlagValue(inheritedArgv, "--model", WORKSPACE_MODEL, "AC-5");
    assertFlagValue(inheritedArgv, "--effort", WORKSPACE_EFFORT, "AC-5");
    if (inheritedArgv.includes(RUN_MODEL) || inheritedArgv.includes(RUN_EFFORT)) {
      throw new Error(`AC-5: the previous run's choice must not survive into the next one; got ${JSON.stringify(inheritedArgv)}`);
    }
    const inheritedRecord = await apiJSON(`${view.url}/api/execution/${inheritedID}`);
    assertModelChoice(
      inheritedRecord.model_choice,
      { model: WORKSPACE_MODEL, options: { effort: WORKSPACE_EFFORT }, source: "workspace" },
      "AC-5",
    );
    ok("AC-5", `the next run, started without any choice, was invoked with --model ${WORKSPACE_MODEL} --effort ${WORKSPACE_EFFORT} and its record names the workspace as the source`);

    // 9 — regression: a run started after an overridden one still holds an
    // ordinary dialogue. The message reaches the process as a user frame and
    // enters the history only when the process re-emits it.
    control.push(emitClaude({ type: "system", subtype: "init", session_id: "session-2" }));
    await waitForRun(view.url, inheritedID, (data) => data.run && data.run.state === "ACTIVE");
    const accepted = await apiJSON(
      `${view.url}/api/execution/${inheritedID}/run/messages`,
      postJSON({ message: MESSAGE_SENTINEL }),
      202,
    );
    if (JSON.stringify(accepted).includes(MESSAGE_SENTINEL)) {
      throw new Error("regression: the accepted message must not be echoed into the timeline before the process re-emits it");
    }
    // The instruction is the first user frame of each run: this one is the
    // third the fake has been given overall, and the second of this run.
    const steered = await control.waitFor(userFrame, 3);
    if (userFrameText(steered) !== MESSAGE_SENTINEL) {
      throw new Error(`regression: the process received ${JSON.stringify(userFrameText(steered))} instead of the sentinel`);
    }
    const beforeReplay = await readRun(view.url, inheritedID, 0);
    if ((beforeReplay.events || []).some((event) => event.text === MESSAGE_SENTINEL)) {
      throw new Error("regression: the message must not be history while the process merely holds it");
    }
    control.push(emitClaude({
      type: "user",
      message: { content: [{ type: "text", text: MESSAGE_SENTINEL }] },
      isReplay: true,
    }));
    const replayed = await waitForRun(view.url, inheritedID, (data) =>
      (data.events || []).some((event) => event.kind === "user_message" && event.text === MESSAGE_SENTINEL));
    const echoed = replayed.events.filter((event) => event.kind === "user_message" && event.text === MESSAGE_SENTINEL);
    if (echoed.length !== 1) {
      throw new Error(`regression: the re-emitted message must appear once; found ${echoed.length}`);
    }
    ok("regression", "a run started after an overridden one still delivers an operator message to the process and shows it once, only after the re-emission");

    // The second run is closed the same way and for the same reason as the
    // first: the fake persists no plan, so the action ends FAILED.
    control.push(emitClaude({
      type: "result",
      subtype: "error_during_execution",
      is_error: true,
      result: "il finto non pianifica nulla",
    }));
    const inheritedClosed = await waitForExecution(view.url, inheritedID, (record) => record.status !== "RUNNING");
    assertModelChoice(
      inheritedClosed.model_choice,
      { model: WORKSPACE_MODEL, options: { effort: WORKSPACE_EFFORT }, source: "workspace" },
      "AC-5 after the terminal write",
    );

    // 10 — nothing of the agent's own authentication material reached the
    // browser or the workspace configuration.
    const leaked = viewerBodies.filter((body) => body.includes(AUTH_SENTINEL));
    if (leaked.length) {
      throw new Error(`the viewer echoed the session material in ${leaked.length} response(s)`);
    }
    if (configAfter.toString("utf8").includes(AUTH_SENTINEL)) {
      throw new Error(`the workspace configuration carries agent session material:\n${configAfter.toString("utf8")}`);
    }
    ok("no-leak", `the session-material sentinel is absent from all ${viewerBodies.length} viewer responses and from .archetipo/config.yaml`);
    console.log(`-> sandbox: ${sandboxDir}`);
  } finally {
    if (view) {
      await stopProcess(view.child);
    }
    if (control) {
      await control.close();
    }
  }
}

// --- oracles ----------------------------------------------------------------

// assertFlagValue holds the argv to the shape `buildArgs` produces: a flag
// immediately followed by its value. Testing mere membership would accept an
// argument list in which the value belonged to another flag.
function assertFlagValue(argv, flag, value, label) {
  const at = argv.indexOf(flag);
  if (at < 0 || argv[at + 1] !== value) {
    throw new Error(`${label}: the process was not invoked with ${flag} ${value}; got ${JSON.stringify(argv)}`);
  }
}

function assertModelChoice(actual, expected, label) {
  const normalized = actual
    ? { model: actual.model, options: actual.options || {}, source: actual.source }
    : null;
  if (JSON.stringify(normalized) !== JSON.stringify(expected)) {
    throw new Error(`${label}: expected model_choice ${JSON.stringify(expected)}; got ${JSON.stringify(actual)}`);
  }
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
    // well have arrived before the test got round to waiting for it — and
    // because two runs share one control server, so the ordinal is what tells
    // the second invocation from the first.
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


// --- fixtures ---------------------------------------------------------------

async function writeSpecsPayload(file) {
  const payload = {
    specs: [
      {
        code: SPEC,
        title: "Smoke spec avviata con un modello scelto",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per la scelta del modello sulla singola run.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
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

async function waitForExecution(viewURL, executionID, predicate, timeoutMs = 30000) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMs) {
    last = await apiJSON(`${viewURL}/api/execution/${executionID}`);
    if (predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`The execution record never satisfied the expectation in time; last: ${JSON.stringify(last)}`);
}

// --- harness ---------------------------------------------------------------

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for choosing model and options for a single run from archetipo view

Usage:
  node ./test/e2e/run-model-choice-view-smoke.mjs
  npm run test:view-run-model-smoke

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

async function runCommand(label, command, args, options = {}) {
  return makeRunCommand()(label, command, args, options);
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

// --- report -----------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "run-model-choice-from-view",
    spec: "US-049",
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
  <title>ARchetipo Smoke — Per-run model choice (US-049)</title>
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
      <h1>ARchetipo Smoke — Per-run model choice (US-049)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>One sandbox served by the real <code>archetipo view</code>, with the real local <code>claude</code> provider
    pointed at a fake agent binary driven frame by frame. The workspace configures
    <code>model: sonnet</code> and <code>effort: high</code>; one run overrides them with
    <code>opus</code> and <code>low</code>, and the next run starts again from the workspace.
    The oracle is the argument list the fake process really received, because
    <code>buildArgs</code> emits <code>--model</code> and <code>--effort</code> only for non-empty values.</p>

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
