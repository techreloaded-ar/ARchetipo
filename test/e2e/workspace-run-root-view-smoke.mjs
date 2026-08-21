#!/usr/bin/env node

// End-to-end smoke for "run in the open workspace, not in the launch directory"
// (US-050).
//
// It walks the demonstration written in the story: the viewer is launched inside
// workspace A, a second workspace B is registered and opened from the UI, and an
// action is started on a spec of B. Everything on the ARchetipo side is real —
// the CLI built from source, `archetipo view`, the filefs connector, the local
// `claude` provider and its stream-json client — and only the agent binary is
// replaced by a Node script that speaks the same protocol on stdio, so the run
// needs no credential and no network.
//
// The oracle is deliberately the working directory the agent process was really
// started in, reported by the fake itself at startup, plus a file that same
// process writes *relative to its own cwd*. A viewer field echoing a project
// root would prove nothing about where a process was spawned; `cmd.Dir` does.
//
// Nothing here sleeps for an outcome: every wait polls a viewer route or the
// control server until it reports what the fake process was just told.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "workspace-run-root-view-smoke");

const CODE_A = "US-A01";
const CODE_B = "US-B01";
const ARTIFACT_BEFORE = "artefatto-run.txt";
const ARTIFACT_AFTER = "artefatto-dopo-switch.txt";

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
  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });
  await buildCLI();

  const startedAt = Date.now();
  let failure = null;
  try {
    await scenarioRunFollowsTheOpenWorkspace(runDir);
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
  console.log(`\nPASS: workspace-run-root view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// The whole story on one pair of workspaces, because it is one story: where the
// first run executes, where the next one executes after another workspace has
// been opened, where a run already in flight stays, and what happens when the
// directory it all points at is gone.
async function scenarioRunFollowsTheOpenWorkspace(runDir) {
  const targetsDir = path.join(runDir, "targets");
  const stateDir = path.join(runDir, "state");
  await fs.mkdir(targetsDir, { recursive: true });
  // Every process started from here writes the registry of known workspaces
  // inside the run directory, never in the real state of the machine.
  const env = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, ARCHETIPO_STATE_DIR: stateDir };

  let view;
  let control;
  try {
    const dirA = await createWorkspace(runDir, targetsDir, "alfa", CODE_A, env);
    const dirB = await createWorkspace(runDir, targetsDir, "beta", CODE_B, env);
    const realA = await fs.realpath(dirA);
    const realB = await fs.realpath(dirB);

    control = await startControlServer();
    console.log(`-> control server for the fake claude: ${control.url}`);

    // The viewer is launched *inside A*: that is the directory a run must not
    // execute in once another workspace is open.
    view = await startViewServer(dirA, { ...env, FAKE_CLAUDE_CONTROL: control.url });
    const pidBefore = view.child.pid;
    console.log(`-> view ready: ${view.url} (pid ${pidBefore}, launched in ${dirA})`);

    // B is registered and then opened from the UI, on the same process.
    await postExpectingStatus(`${view.url}/api/workspaces`, { path: dirB }, 201);
    const known = await apiJSON(`${view.url}/api/workspaces`);
    const entryA = await findByRealPath(known.workspaces, realA);
    const entryB = await findByRealPath(known.workspaces, realB);
    if (!entryA || !entryB) {
      throw new Error(`both workspaces must be known: ${JSON.stringify(known.workspaces)}`);
    }
    await postExpectingStatus(`${view.url}/api/workspaces/${encodeURIComponent(entryB.id)}/open`, {}, 200);
    assertSameList(await boardCodes(view.url), [CODE_B], "the board after opening B");

    // The provider is configured on B, through the very route the Execution
    // panel uses, so the run goes through the real local claude provider.
    await apiJSON(`${view.url}/api/execution/provider/default`, putJSON({
      id: "claude",
      config: { command: fakeClaudePath, timeout_seconds: 600 },
    }));

    // The state of A, immediately before the run: whatever the run does, none of
    // it may land here.
    const treeABefore = await snapshotTree(dirA);

    // --- AC-1 and AC-2 ------------------------------------------------------
    const started = await apiJSON(`${view.url}/api/spec/${CODE_B}/execution`, postJSON({ action: "plan" }), 201);
    if (started.status !== "RUNNING" || !started.id) {
      throw new Error(`AC-1: unexpected execution record on start: ${JSON.stringify(started)}`);
    }
    const runID = started.id;

    const invocation = await control.waitFor("argv", 1);
    const startedIn = await fs.realpath(invocation.cwd);
    if (startedIn !== realB) {
      throw new Error(`AC-1: the agent process was started in ${invocation.cwd}, want the project root of the open workspace ${dirB}`);
    }
    if (startedIn === realA) {
      throw new Error("AC-1: the agent process was started in the directory the viewer was launched from");
    }

    // The artifact the run produces is written by the agent relative to its own
    // working directory: it appears under B and nowhere else.
    control.push({ kind: "write", name: ARTIFACT_BEFORE, text: "prodotto dalla run del workspace aperto\n" });
    await control.waitFor("wrote", 1);
    await assertFileExists(path.join(dirB, ARTIFACT_BEFORE), "AC-1: the artifact of the run under B");
    await assertFileAbsent(path.join(dirA, ARTIFACT_BEFORE), "AC-1: the artifact of the run under A");

    const record = await apiJSON(`${view.url}/api/execution/${runID}`);
    await assertSamePath(record.working_dir, dirB, "AC-1: the working directory the record reports");

    assertEqual(view.child.pid, pidBefore, "the viewer PID after the run started");
    assertAlive(view.child, "the viewer process after the run started");
    await assertSameTree(treeABefore, await snapshotTree(dirA), "AC-1: the tree of the workspace the viewer was launched from");

    ok("AC-1", `the agent process was started in ${dirB}, the record reports it as its working directory, its artifact appeared under B, and the launch directory A is untouched`);
    ok("AC-2", `the first action started after opening B ran in B on the very same viewer process (pid ${pidBefore}), with no restart`);

    // --- AC-3 ---------------------------------------------------------------
    // The run is still open. Opening another workspace must not move it.
    const executionsBefore = await listExecutionRecords(dirB);
    await postExpectingStatus(`${view.url}/api/workspaces/${encodeURIComponent(entryA.id)}/open`, {}, 200);
    assertEqual(view.child.pid, pidBefore, "the viewer PID after reopening A");
    assertAlive(view.child, "the viewer process after reopening A");

    // Whatever became of the run, the record of it lives in B and says it ran in
    // B — and nothing of it appeared in A.
    const persisted = await readExecutionRecord(dirB, runID);
    await assertSamePath(persisted.working_dir, dirB, "AC-3: the working directory the persisted record reports");
    if (persisted.spec_code !== CODE_B) {
      throw new Error(`AC-3: the record of the run belongs to ${persisted.spec_code}, want ${CODE_B}`);
    }
    const recordsInA = await listExecutionRecords(dirA);
    if (recordsInA.length !== 0) {
      throw new Error(`AC-3: the run left ${recordsInA.length} record(s) in the workspace it never ran in: ${JSON.stringify(recordsInA)}`);
    }
    await assertFileAbsent(path.join(dirA, ARTIFACT_AFTER), "AC-3: an artifact under A after the switch");
    if ((await listExecutionRecords(dirB)).length !== executionsBefore.length) {
      throw new Error("AC-3: opening another workspace created or removed a record in B");
    }
    ok("AC-3", `the run started in B keeps its working directory ${dirB} on the record after A was opened, and left nothing at all in A`);

    // --- AC-4 ---------------------------------------------------------------
    // B is served again and then its directory is renamed under the viewer.
    await postExpectingStatus(`${view.url}/api/workspaces/${encodeURIComponent(entryB.id)}/open`, {}, 200);
    const movedB = path.join(targetsDir, "beta-rinominato");
    const recordsBeforeRefusal = await listExecutionRecords(dirB);
    await fs.rename(dirB, movedB);

    const refusedSpec = await postExpectingStatus(`${view.url}/api/spec/${CODE_B}/execution`, { action: "plan" }, 409);
    assertNamesDirectory(refusedSpec.body, dirB, "AC-4: the refusal on the spec route");
    const refusedWorkspace = await postExpectingStatus(`${view.url}/api/workspace/execution`, { action: "inception" }, 409);
    assertNamesDirectory(refusedWorkspace.body, dirB, "AC-4: the refusal on the workspace route");

    const recordsAfterRefusal = await listExecutionRecords(movedB);
    if (recordsAfterRefusal.length !== recordsBeforeRefusal.length) {
      throw new Error(
        `AC-4: a refused start created ${recordsAfterRefusal.length - recordsBeforeRefusal.length} record(s): ${JSON.stringify(recordsAfterRefusal)}`,
      );
    }
    assertEqual(view.child.pid, pidBefore, "the viewer PID after the refusals");
    assertAlive(view.child, "the viewer process after the refusals");
    ok("AC-4", `with ${dirB} renamed away, both start routes answered 409 with a message naming that directory, and no new execution record was created`);
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

function assertNamesDirectory(body, dir, label) {
  const message = typeof body?.error === "string" ? body.error : JSON.stringify(body);
  if (!message.includes(dir)) {
    throw new Error(`${label} does not name the directory ${dir}: ${message}`);
  }
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

async function assertFileExists(file, label) {
  try {
    await fs.stat(file);
  } catch {
    throw new Error(`${label}: ${file} does not exist`);
  }
}

async function assertFileAbsent(file, label) {
  try {
    await fs.stat(file);
  } catch {
    return;
  }
  throw new Error(`${label}: ${file} exists and must not`);
}

// snapshotTree lists every path under a directory, relative and sorted, so two
// snapshots can be compared as one string.
async function snapshotTree(root) {
  const out = [];
  async function walk(dir) {
    let entries = [];
    try {
      entries = await fs.readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      out.push(path.relative(root, full));
      if (entry.isDirectory()) await walk(full);
    }
  }
  await walk(root);
  out.sort();
  return out;
}

async function assertSameTree(before, after, label) {
  const added = after.filter((entry) => !before.includes(entry));
  const removed = before.filter((entry) => !after.includes(entry));
  if (added.length || removed.length) {
    throw new Error(`${label} changed: added ${JSON.stringify(added)}, removed ${JSON.stringify(removed)}`);
  }
}

async function listExecutionRecords(root) {
  try {
    const entries = await fs.readdir(path.join(root, ".archetipo", "executions"));
    return entries.filter((name) => name.endsWith(".json")).sort();
  } catch {
    return [];
  }
}

async function readExecutionRecord(root, id) {
  const file = path.join(root, ".archetipo", "executions", `${id}.json`);
  return JSON.parse(await fs.readFile(file, "utf8"));
}

// --- the control server ----------------------------------------------------
//
// The fake binary emits nothing on its own: it asks this server what to do next
// and reports everything it received, so every assertion above is a statement
// about a state the test produced.

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

  return {
    url,
    push(command) {
      commands.push(command);
    },
    // waitFor polls until the fake has reported at least `count` matching
    // requests, and returns the count-th of them. The count is explicit rather
    // than relative to a snapshot taken here, because a request can perfectly
    // well have arrived before the test got round to waiting for it.
    async waitFor(kind, count = 1, timeoutMs = 30000) {
      const started = Date.now();
      while (Date.now() - started < timeoutMs) {
        const matching = received.filter((entry) => entry.kind === kind);
        if (matching.length >= count) return matching[count - 1];
        await delay(50);
      }
      throw new Error(
        `The fake never reported ${kind} ${count} time(s); it reported ${JSON.stringify(received.map((entry) => entry.kind))}`,
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

// --- fixtures ---------------------------------------------------------------

// createWorkspace initializes one real workspace with a backlog holding a single
// recognisable spec code. The code is the oracle of which workspace is served.
async function createWorkspace(runDir, targetsDir, name, code, env) {
  const dir = path.join(targetsDir, name);
  await fs.mkdir(dir, { recursive: true });
  await runCommand(`init-${name}`, cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], { cwd: dir, env });

  const specsFile = path.join(runDir, `specs-${name}.json`);
  await fs.writeFile(specsFile, JSON.stringify({
    specs: [{
      code,
      title: `Smoke ${name}`,
      epic: { code: name === "alfa" ? "EP-001" : "EP-002", title: `Smoke ${name}` },
      priority: "LOW",
      points: 1,
      status: "TODO",
      body: `Story di test del workspace ${name}.`,
    }],
  }, null, 2));
  await runCommand(`spec-add-${name}`, cliPath, ["spec", "add", "--file", specsFile], { cwd: dir, env });
  return dir;
}

async function boardCodes(url) {
  const board = await apiJSON(`${url}/api/board`);
  const codes = [];
  for (const column of board.columns || []) {
    for (const spec of column.specs || []) codes.push(spec.code);
  }
  codes.sort();
  return codes;
}

// findByRealPath compares through fs.realpath on both sides, because on macOS
// /var is a symlink to /private/var and the two spellings are the same place.
async function findByRealPath(entries, target) {
  for (const entry of entries) {
    try {
      if ((await fs.realpath(entry.path)) === target) return entry;
    } catch {
      // An unreachable entry cannot be resolved; fall back to the literal path.
    }
    if (entry.path === target) return entry;
  }
  return null;
}

// --- harness ---------------------------------------------------------------

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
  console.log(`Smoke test for running in the open workspace instead of the launch directory

Usage:
  node ./test/e2e/workspace-run-root-view-smoke.mjs
  npm run test:view-workspace-root-smoke

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
  throw new Error(`Timed out waiting for ${url}`);
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
    throw new Error(`HTTP ${response.status} for ${url}: ${typeof data === "string" ? data : JSON.stringify(data)}`);
  }
  if (expected !== null && response.status !== expected) {
    throw new Error(`Expected HTTP ${expected} for ${url}, got ${response.status}: ${text}`);
  }
  return data;
}

async function postExpectingStatus(url, payload, expectedStatus) {
  const response = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (response.status !== expectedStatus) {
    throw new Error(`Expected HTTP ${expectedStatus} for POST ${url}, got ${response.status}: ${text}`);
  }
  return { status: response.status, body };
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`Unexpected ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

function assertSameList(actual, expected, label) {
  const a = JSON.stringify(actual ?? null);
  const b = JSON.stringify(expected ?? null);
  if (a !== b) {
    throw new Error(`Unexpected ${label}:\n  expected ${b}\n  got      ${a}`);
  }
}

function assertAlive(child, label) {
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(`${label}: the process is gone (exit ${child.exitCode}, signal ${child.signalCode})`);
  }
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

// --- report -----------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "workspace-run-root-from-view",
    spec: "US-050",
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
  <title>ARchetipo Smoke — Run in the open workspace (US-050)</title>
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
      <h1>ARchetipo Smoke — Run in the open workspace (US-050)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Two real workspaces, A and B. The real <code>archetipo view</code> is launched inside A, B is
    registered and opened from the UI on the same process, and a <code>plan</code> action is started on a
    spec of B through the real local <code>claude</code> provider pointed at a fake agent binary.
    The oracle is the working directory the agent process reports at startup — its real
    <code>cmd.Dir</code> — plus a file that same process writes relative to its own working
    directory, so "the run acted on this workspace" is a fact on the filesystem.</p>

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
