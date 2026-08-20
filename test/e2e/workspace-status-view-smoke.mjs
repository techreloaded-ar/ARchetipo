#!/usr/bin/env node

// Smoke test for US-043: knowing where the workspace is in the process and
// which step comes next.
//
// It walks the Dimostrazione of the story from end to end on a real viewer.
// Everything is real except the browser: the CLI is built from source, the
// workspace is a real directory, the PRD and the backlog appear as real files
// on disk, and every assertion is made on the HTTP contract. No AI agent is
// involved, because the scenario runs no action at all: it writes artifacts and
// watches the answer of `GET /api/workspace/status` change.
//
// There is no arbitrary sleep as an oracle: every wait polls a viewer route
// until it reports what the filesystem was just told, with an explicit timeout
// and a message naming what was expected and what arrived.
//
// The fact the whole story rests on is asserted after every transition: the PID
// of the viewer never changes. The recommended step follows the workspace on the
// same process, or "without restarting the viewer" would mean nothing.

import fs from "node:fs/promises";
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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "workspace-status-view-smoke");
const baseEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

// Two specs in TODO, because the recommended step must point at *one* of them —
// the first card of the column — and a single spec would make that impossible to
// tell apart from "whatever spec exists".
const SEEDED_SPECS = [
  seedSpec("US-001", "Vedere lo stato del workspace"),
  seedSpec("US-002", "Vedere il passo successivo"),
];

const PRD_BODY = [
  "# PRD — Prodotto di smoke",
  "",
  "## Vision",
  "Un prodotto inventato da uno smoke test per far comparire il PRD sul disco.",
  "",
].join("\n");

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const stateDir = path.join(runDir, "state");
  // Every process started from here writes the registry of known workspaces
  // inside the run directory, never in the real state of the machine.
  const cliEnv = { ...baseEnv, ARCHETIPO_STATE_DIR: stateDir };
  const verified = [];

  console.log(`-> workspace: ${sandboxDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  let stream;
  try {
    await buildCLI(cliEnv);

    // One real workspace, initialized and nothing else: no PRD, no backlog.
    await runCommand("init", cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], {
      cwd: sandboxDir,
      env: cliEnv,
    });

    view = await startViewServer(sandboxDir, cliEnv);
    console.log(`-> view ready: ${view.url} (pid ${view.child.pid})`);
    const pidBefore = view.child.pid;

    // --- State 1: the empty workspace ---------------------------------------
    const empty = await apiJSON(`${view.url}/api/workspace/status`);
    assertEqual(empty.stage?.id, "senza-prd", "the stage of the empty workspace");
    assertEqual(empty.next_step?.scope, "workspace", "the scope of the next step of the empty workspace");
    assertEqual(empty.next_step?.action, "inception", "the action of the next step of the empty workspace");
    assertEqual(empty.has_prd, false, "the PRD flag of the empty workspace");
    assertEqual(empty.has_backlog, false, "the backlog flag of the empty workspace");
    console.log(`-> state 1: ${empty.stage.id} -> ${empty.next_step.scope}/${empty.next_step.action}`);
    verified.push("AC-1/AC-2 — the empty workspace declares its stage and the step that advances it");

    // AC-4 — no action is inert: everything that cannot be run says what unlocks it.
    const blocked = (empty.actions || []).filter((action) => action.runnable === false);
    if (blocked.length === 0) {
      throw new Error("AC-4: the empty workspace offers no blocked action, so nothing proves the unlock condition");
    }
    for (const action of blocked) {
      if (typeof action.unlocked_by !== "string" || action.unlocked_by.trim() === "") {
        throw new Error(
          `AC-4: the action ${JSON.stringify(action.id)} is not runnable but declares no unlock condition: ${JSON.stringify(action)}`,
        );
      }
    }
    if (!blocked.some((action) => action.id === "backlog")) {
      throw new Error(
        `AC-4: the backlog action is expected among the blocked ones of an empty workspace, got ${JSON.stringify(blocked.map((a) => a.id))}`,
      );
    }
    verified.push("AC-4 — every non-runnable action of the real payload carries its own unlock condition");

    // --- The update channel -------------------------------------------------
    // The board stream is the mechanism the viewer refreshes itself on. It is
    // read as raw text instead of through EventSource, which Node exposes only
    // in recent versions and which would hide what is being asserted.
    stream = await openBoardStream(view.url);
    console.log("-> board stream connected");

    // --- State 2: the PRD appears -------------------------------------------
    // Written straight onto the filesystem: nothing is said to the viewer, which
    // is the whole point of AC-3.
    await fs.mkdir(path.join(sandboxDir, "docs"), { recursive: true });
    await fs.writeFile(path.join(sandboxDir, "docs", "PRD.md"), PRD_BODY, "utf8");

    const withPRD = await waitForStatus(
      view.url,
      (status) => status.stage?.id === "senza-backlog" && status.next_step?.action === "backlog",
      'stage.id "senza-backlog" with next_step.action "backlog" after docs/PRD.md appeared on disk',
    );
    assertEqual(withPRD.has_prd, true, "the PRD flag after the PRD appeared");
    assertEqual(withPRD.has_backlog, false, "the backlog flag after the PRD appeared");
    assertEqual(withPRD.next_step?.scope, "workspace", "the scope of the next step after the PRD appeared");
    assertEqual(view.child.pid, pidBefore, "the viewer PID after the PRD appeared");
    assertAlive(view.child, "the viewer process after the PRD appeared");
    console.log(`-> state 2: ${withPRD.stage.id} -> ${withPRD.next_step.scope}/${withPRD.next_step.action}`);
    verified.push("AC-3 — a PRD written on disk moves the stage and the recommended step, with nothing told to the viewer");

    // --- State 3: the backlog appears ---------------------------------------
    // `archetipo spec add` is the command the spec skill prescribes, so the
    // backlog appears the way it really appears — under .archetipo/, which is
    // also the directory the viewer's watcher observes.
    await addSpecs(sandboxDir, SEEDED_SPECS, cliEnv);

    const withBacklog = await waitForStatus(
      view.url,
      (status) =>
        status.stage?.id === "da-pianificare" &&
        status.next_step?.scope === "spec" &&
        status.next_step?.action === "plan",
      'stage.id "da-pianificare" with next_step.scope "spec" and next_step.action "plan" after the backlog appeared on disk',
    );
    assertEqual(withBacklog.has_backlog, true, "the backlog flag after the backlog appeared");
    console.log(`-> state 3: ${withBacklog.stage.id} -> ${withBacklog.next_step.scope}/${withBacklog.next_step.action}`);

    // The oracle that ties the recommendation to what the person actually sees:
    // the target spec is the first card of the TODO column, not a code chosen
    // here.
    const firstTodo = await firstTodoCard(view.url);
    assertEqual(withBacklog.next_step?.spec?.code, firstTodo.code, "the spec the recommended step points at");
    assertEqual(withBacklog.next_step?.spec?.status, "TODO", "the status of the spec the recommended step points at");
    assertEqual(view.child.pid, pidBefore, "the viewer PID after the backlog appeared");
    assertAlive(view.child, "the viewer process after the backlog appeared");
    verified.push("AC-2 — the recommended step targets the first card of the TODO column read from GET /api/board");

    // AC-5 — the watcher notified the connected clients: this is the mechanism
    // the board refreshes the status strip on, without a reload and without a
    // restart.
    await waitFor(
      () => stream.buffer().includes("board_changed"),
      "a board_changed event on GET /api/board/stream after the backlog appeared under .archetipo/",
      () => `received so far: ${JSON.stringify(stream.buffer())}`,
    );
    verified.push("AC-5 — the board stream emitted board_changed when the workspace changed on disk");

    // --- Closing ------------------------------------------------------------
    assertEqual(view.child.pid, pidBefore, "the viewer PID at the end of the three states");
    assertAlive(view.child, "the viewer process at the end of the three states");
    verified.push("AC-5 — the three states were observed on one process, with the PID compared after every transition");

    console.log("\nPASS: workspace-status view smoke test completed.");
    console.log(`  states: ${empty.stage.id} -> ${withPRD.stage.id} -> ${withBacklog.stage.id} (pid ${pidBefore} throughout)`);
    for (const line of verified) {
      console.log(`  ✓ ${line}`);
    }
    console.log(`Run directory: ${runDir}`);
    console.log(`State directory: ${stateDir}`);
  } finally {
    if (stream) {
      await stream.close();
    }
    if (view) {
      await stopProcess(view.child);
    }
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned run directory: ${runDir}`);
    }
  }
}

// --- the workspace ----------------------------------------------------------

function seedSpec(code, title) {
  return {
    code,
    title,
    epic: { code: "EP-001", title: "Impianto del prodotto" },
    priority: "HIGH",
    points: 3,
    status: "TODO",
    body: [
      "**User Story**",
      `Come persona di smoke voglio ${title.toLowerCase()}, per provare il passo raccomandato.`,
      "",
      "**Dimostrazione**",
      "La spec appare nella colonna TODO del workspace.",
      "",
      "**Criteri di accettazione**",
      "- [ ] AC-1 — la spec appare nella colonna TODO.",
      "",
    ].join("\n"),
  };
}

// addSpecs persists epics and specs through `archetipo spec add`, the command
// the spec skill prescribes and therefore the one that really creates a backlog.
async function addSpecs(sandboxDir, specs, env) {
  const file = path.join(sandboxDir, ".archetipo", "specs-input.json");
  await fs.writeFile(file, `${JSON.stringify({ specs }, null, 2)}\n`);
  await runCommand("spec-add", cliPath, ["spec", "add", "--file", file], { cwd: sandboxDir, env });
  await fs.rm(file, { force: true });
}

// firstTodoCard returns the card the board shows on top of the TODO column,
// which is what the person looking at the viewer sees first.
async function firstTodoCard(url) {
  const board = await apiJSON(`${url}/api/board`);
  const column = (board.columns || []).find((c) => c.status === "TODO" || c.id === "todo");
  if (!column) {
    throw new Error(`GET /api/board has no TODO column: ${JSON.stringify((board.columns || []).map((c) => c.id))}`);
  }
  const first = (column.specs || [])[0];
  if (!first) {
    throw new Error("GET /api/board reports an empty TODO column, so the recommended step has nothing to be compared to");
  }
  return first;
}

// --- the update channel -----------------------------------------------------

// openBoardStream keeps one SSE connection open and accumulates whatever the
// server writes on it. The body is read incrementally as text: the assertion is
// then a plain statement about what the server sent.
async function openBoardStream(url) {
  const controller = new AbortController();
  const response = await fetch(`${url}/api/board/stream`, {
    headers: { Accept: "text/event-stream" },
    signal: controller.signal,
  });
  if (!response.ok || !response.body) {
    throw new Error(`GET ${url}/api/board/stream answered ${response.status}`);
  }
  let buffer = "";
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const pump = (async () => {
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) return;
        buffer += decoder.decode(value, { stream: true });
      }
    } catch {
      // The stream is closed by aborting it, which surfaces here as an error.
    }
  })();

  return {
    buffer: () => buffer,
    async close() {
      controller.abort();
      await pump;
    },
  };
}

// --- waiting, always on a route ---------------------------------------------

// waitFor polls a condition until it holds or the timeout expires. The failure
// names what was expected and what the last reading was: a mute timeout would
// say nothing about which state the workspace got stuck in.
async function waitFor(condition, expectation, describeLast, timeoutMs = 15000) {
  const started = Date.now();
  let lastError = null;
  while (Date.now() - started < timeoutMs) {
    try {
      if (await condition()) return;
      lastError = null;
    } catch (error) {
      lastError = error;
    }
    await delay(200);
  }
  const detail = lastError ? `last error: ${lastError.message}` : describeLast ? await describeLast() : "no reading";
  throw new Error(`Timed out after ${timeoutMs}ms waiting for ${expectation}\n  ${detail}`);
}

// waitForStatus polls GET /api/workspace/status until the predicate holds, and
// returns the payload that satisfied it.
async function waitForStatus(url, predicate, expectation, timeoutMs = 15000) {
  let last = null;
  await waitFor(
    async () => {
      last = await apiJSON(`${url}/api/workspace/status`);
      return predicate(last);
    },
    expectation,
    () =>
      `last status: stage.id ${JSON.stringify(last?.stage?.id)}, next_step ${JSON.stringify(last?.next_step?.scope)}/${JSON.stringify(last?.next_step?.action)}, has_prd ${JSON.stringify(last?.has_prd)}, has_backlog ${JSON.stringify(last?.has_backlog)}`,
    timeoutMs,
  );
  return last;
}

// --- plumbing ---------------------------------------------------------------

function parseArgs(argv) {
  const options = {
    workspaceRoot: defaultWorkspaceRoot,
    cleanup: false,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
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
        throw new Error(`Unknown argument: ${arg}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Smoke test for the workspace status and the recommended next step

Usage:
  node ./test/e2e/workspace-status-view-smoke.mjs
  npm run test:view-workspace-status-smoke

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

async function buildCLI(env) {
  console.log(`-> building CLI: ${cliPath}`);
  await runCommand("go-build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], {
    cwd: path.join(repoRoot, "cli"),
    env,
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
  await waitForHTTP(`${url}/api/workspace/status`);
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

async function apiJSON(url, init = {}) {
  const response = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init.headers || {}),
    },
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
  return data;
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

async function runCommand(label, command, args, options = {}) {
  console.log(`-> ${label}: ${command} ${args.join(" ")}`);
  const result = await new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env || baseEnv,
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
  if (process.platform === "win32") {
    await runCommand("taskkill", "taskkill", ["/PID", String(child.pid), "/T", "/F"]);
    return;
  }
  child.kill("SIGTERM");
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    delay(3000),
  ]);
  if (!child.killed) {
    child.kill("SIGKILL");
  }
}

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
