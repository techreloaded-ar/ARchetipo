#!/usr/bin/env node

// Smoke test for US-056: seeing the recommended next step while working.
//
// It walks the Dimostrazione of the story from end to end on a real viewer.
// Everything is real except the browser: the CLI is built from source, the
// workspaces are real directories, the backlog appears as real files on disk,
// and every assertion is made on the HTTP contract. No AI agent is involved and
// no execution is ever started: the story is about what the strip *says* and
// whether the board offers the same thing, not about running it. The local
// provider is configured only to make the step runnable at all.
//
// There is no arbitrary sleep as an oracle: every wait polls a viewer route
// until it reports what the filesystem was just told, with an explicit timeout
// and a message naming what was expected and what arrived.
//
// The fact the whole story rests on is asserted after every transition: the PID
// of the viewer never changes. The recommended step follows the workspace on
// the same process, or "without reloading the page" would mean nothing.
//
// Usage:
//   node ./test/e2e/next-step-view-smoke.mjs
//   npm run test:view-next-step-smoke

import fs from "node:fs/promises";
import os from "node:os";
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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "next-step-view-smoke");
const baseEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

// The fake local provider. It is never spawned by this smoke — nothing here
// starts an execution — but a provider has to be configured for a step to be
// reported runnable at all, and a fake keeps the smoke free of credentials.
const fakeClaudePath = path.join(__dirname, "support", "fake-claude.mjs");
const PROVIDER = { id: "claude", config: { command: fakeClaudePath, timeout_seconds: 600 } };

// Two specs in TODO, because the step that "updates itself" is proved by the
// target moving from the first to the second: with a single spec, planning it
// would only change the stage, and the weaker assertion would pass for the
// wrong reason.
const SEEDED_SPECS = [
  seedSpec("US-001", "Vedere il passo suggerito"),
  seedSpec("US-002", "Avviare il passo suggerito"),
];

const PRD_BODY = [
  "# PRD — Prodotto di smoke",
  "",
  "## Vision",
  "Un prodotto inventato da uno smoke test per far comparire il PRD sul disco.",
  "",
].join("\n");

// One entry per proved statement, for the closing report.
const verified = [];

function ok(scenario, statement) {
  verified.push({ scenario, statement });
  console.log(`-> ${scenario} ok: ${statement}`);
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const stateDir = path.join(runDir, "state");
  // Every process started from here writes the registry of known workspaces
  // inside the run directory, never in the real state of the machine.
  const cliEnv = { ...baseEnv, ARCHETIPO_STATE_DIR: stateDir };

  console.log(`-> workspace: ${sandboxDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let home;
  let view;
  let stream;
  // The neutral directory lives outside the repository, for the reason the
  // no-workspace story is about: the CLI walks *up* from the launch directory,
  // and this repository is itself an ARchetipo workspace. A directory under
  // test/workspaces/ would resolve to it and the scenario would silently test
  // its opposite.
  const neutralDir = await fs.mkdtemp(path.join(os.tmpdir(), "archetipo-next-step-neutral-"));

  try {
    await buildCLI(cliEnv);

    // --- Scenario 1: no workspace is open -----------------------------------
    // AC-5. Nothing is suggested because there is nothing to suggest it for:
    // the route that carries the recommended step refuses, and its refusal
    // carries no step at all rather than an empty one.
    home = await startViewServer(neutralDir, cliEnv);
    const homePid = home.child.pid;
    console.log(`-> home view ready: ${home.url} (pid ${homePid})`);

    const refusal = await call(`${home.url}/api/workspace/status`);
    if (refusal.status !== 409) {
      throw new Error(
        `AC-5: GET /api/workspace/status answered ${refusal.status} with no workspace open, want 409: ${refusal.text}`,
      );
    }
    if (refusal.body === null || typeof refusal.body !== "object") {
      throw new Error(`AC-5: the refusal carries no JSON body: ${refusal.text}`);
    }
    assertEqual(refusal.body.workspaceOpen, false, "`workspaceOpen` in the refusal of GET /api/workspace/status");
    for (const key of ["stage", "next_step", "actions"]) {
      if (key in refusal.body) {
        throw new Error(
          `AC-5: the refusal carries "${key}" — it answered emptily instead of declaring that no workspace is open: ${refusal.text}`,
        );
      }
    }
    assertEqual(home.child.pid, homePid, "the PID of the home viewer after the refusal");
    assertAlive(home.child, "the home viewer after the refusal");
    ok("no workspace", "GET /api/workspace/status answers 409 with workspaceOpen:false and no stage, next_step or actions");

    await stopProcess(home.child);
    home = null;

    // --- One real workspace, one viewer, for the rest of the story ----------
    // Initialized and nothing else: no PRD, no backlog, and above all no
    // provider configured, which is what makes the first step blocked.
    await runCommand("init", cliPath, ["init", "--tool", "claude", "--connector", "file", "--yes"], {
      cwd: sandboxDir,
      env: cliEnv,
    });

    view = await startViewServer(sandboxDir, cliEnv);
    const pid = view.child.pid;
    console.log(`-> view ready: ${view.url} (pid ${pid})`);

    // --- Scenario 2: the step is blocked, and says what unlocks it ----------
    // AC-4. The unlock condition is asserted as "non-empty and stable", never
    // against a sentence written here: a phrase hard-coded in the smoke would
    // be the test declaring the words of the process instead of reading them.
    const blocked = await waitForStatus(
      view.url,
      (status) => status.next_step && status.next_step.runnable === false,
      "a next_step with runnable:false on a workspace with no usable provider",
    );
    assertNonEmpty(blocked.next_step.unlocked_by, "the unlock condition of the blocked step");
    assertNonEmpty(blocked.next_step.unavailable_reason, "the reason the blocked step cannot be started");
    assertNonEmpty(blocked.next_step.action, "the action of the blocked step");
    const blockedAgain = await apiJSON(`${view.url}/api/workspace/status`);
    assertEqual(
      blockedAgain.next_step?.unlocked_by,
      blocked.next_step.unlocked_by,
      "the unlock condition read a second time from the payload",
    );
    assertEqual(view.child.pid, pid, "the viewer PID after the blocked step was read");
    assertAlive(view.child, "the viewer after the blocked step was read");
    console.log(
      `-> blocked step: ${blocked.next_step.scope}/${blocked.next_step.action} unlocked_by ${JSON.stringify(blocked.next_step.unlocked_by)}`,
    );
    ok(
      "blocked step",
      "a non-runnable step carries a non-empty unlock condition that comes from the payload, identical on a second reading",
    );

    // --- The update channel -------------------------------------------------
    // The board stream is the mechanism the viewer refreshes itself on. It is
    // read as raw text instead of through EventSource, which Node exposes only
    // in recent versions and which would hide what is being asserted.
    stream = await openBoardStream(view.url);
    console.log("-> board stream connected");

    // --- Scenario 3: the step and the board agree ---------------------------
    // AC-2 at the level of the contract. A usable provider, a PRD and a backlog
    // make the recommended step a spec-scoped, runnable one; the oracle is that
    // the very same action is offered as runnable by the spec the step points
    // at — which is the action the board would start.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(PROVIDER),
    });
    await fs.mkdir(path.join(sandboxDir, "docs"), { recursive: true });
    await fs.writeFile(path.join(sandboxDir, "docs", "PRD.md"), PRD_BODY, "utf8");
    await addSpecs(sandboxDir, SEEDED_SPECS, cliEnv);

    const runnable = await waitForStatus(
      view.url,
      (status) =>
        status.next_step?.scope === "spec" &&
        status.next_step?.runnable === true &&
        typeof status.next_step?.spec?.code === "string",
      'a runnable next_step with scope "spec" after the provider, the PRD and the backlog appeared',
    );
    const target = runnable.next_step.spec.code;
    console.log(`-> runnable step: ${runnable.next_step.scope}/${runnable.next_step.action} on ${target}`);

    const detail = await apiJSON(`${view.url}/api/spec/${target}`);
    const offered = (detail.actions || []).find((action) => action.id === runnable.next_step.action);
    if (!offered) {
      throw new Error(
        `AC-2: the step recommends ${JSON.stringify(runnable.next_step.action)} on ${target}, but the spec offers ${JSON.stringify((detail.actions || []).map((a) => a.id))}`,
      );
    }
    if (offered.runnable !== true) {
      throw new Error(
        `AC-2: the step is runnable but the same action on ${target} is not: ${JSON.stringify(offered)}`,
      );
    }
    assertEqual(view.child.pid, pid, "the viewer PID after the step and the board were compared");
    assertAlive(view.child, "the viewer after the step and the board were compared");
    ok(
      "step and board agree",
      `the recommended step names ${JSON.stringify(runnable.next_step.action)} on ${target}, the very action GET /api/spec/${target} offers as runnable`,
    );

    // --- Scenario 4: the step updates itself --------------------------------
    // AC-3. The target spec is planned through the CLI — nothing is told to the
    // viewer — and the recommended step moves on its own. The stream is what
    // the browser refreshes on, so the board_changed event is awaited first and
    // only then the new answer of the status route.
    const planFile = path.join(runDir, "plan.json");
    await fs.writeFile(planFile, `${JSON.stringify(planPayload(), null, 2)}\n`);
    await runCommand("spec-plan", cliPath, ["spec", "plan", target, "--file", planFile], {
      cwd: sandboxDir,
      env: cliEnv,
    });

    await waitFor(
      () => stream.buffer().includes("board_changed"),
      `a board_changed event on GET /api/board/stream after ${target} was planned`,
      () => `received so far: ${JSON.stringify(stream.buffer())}`,
    );

    // The predicate demands a step that is still *there*: a next_step that
    // simply vanished would satisfy "different spec code" while proving
    // nothing, and AC-3 is about the step moving on, not about it disappearing.
    const moved = await waitForStatus(
      view.url,
      (status) =>
        Boolean(status.next_step?.action) &&
        (status.stage?.id !== runnable.stage?.id ||
          status.next_step?.spec?.code !== target),
      `a still-recommended step with a different stage.id or a different next_step.spec.code after ${target} was planned`,
    );
    console.log(
      `-> updated step: ${moved.stage?.id} -> ${moved.next_step?.scope}/${moved.next_step?.action} on ${moved.next_step?.spec?.code ?? "the workspace"}`,
    );
    assertEqual(view.child.pid, pid, "the viewer PID after the recommended step moved");
    assertAlive(view.child, "the viewer after the recommended step moved");
    ok(
      "step updates itself",
      `planning ${target} on disk moved the recommended step (${runnable.stage?.id}/${target} -> ${moved.stage?.id}/${moved.next_step?.spec?.code ?? "workspace"}) with nothing told to the viewer`,
    );

    // --- The fact the whole story rests on ----------------------------------
    assertEqual(view.child.pid, pid, "the viewer PID at the end of the story");
    assertAlive(view.child, "the viewer at the end of the story");
    ok("one process throughout", `every transition was observed on pid ${pid}, compared after each one: no reload, no restart`);

    console.log("\nPASS: next-step view smoke test completed.");
    for (const check of verified) {
      console.log(`  ✓ ${check.scenario} — ${check.statement}`);
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
    if (home) {
      await stopProcess(home.child);
    }
    await fs.rm(neutralDir, { recursive: true, force: true });
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

// planPayload is the smallest plan that moves a spec to PLANNED. Its content is
// irrelevant here: what matters is that the state of the workspace changes
// under the viewer, through the command the process prescribes.
function planPayload() {
  return {
    plan_body: "## Piano di smoke\n\nSolo per portare la spec in PLANNED.",
    tasks: [
      {
        id: "TASK-01",
        title: "Smoke task",
        body: "## Objective\nNessuna.\n\n## Blockers\nNone.",
        type: "Impl",
        status: "TODO",
        dependencies: [],
      },
    ],
  };
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
      `last status: stage.id ${JSON.stringify(last?.stage?.id)}, next_step ${JSON.stringify(last?.next_step?.scope)}/${JSON.stringify(last?.next_step?.action)} on ${JSON.stringify(last?.next_step?.spec?.code)}, runnable ${JSON.stringify(last?.next_step?.runnable)}, unlocked_by ${JSON.stringify(last?.next_step?.unlocked_by)}`,
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
  console.log(`Smoke test for the recommended next step shown while working

Usage:
  node ./test/e2e/next-step-view-smoke.mjs
  npm run test:view-next-step-smoke

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

// startViewServer starts the real viewer. Readiness is probed on the route that
// answers with or without an open workspace, because this smoke starts the
// viewer both ways and a probe on a workspace-scoped route would never come
// back in the neutral directory.
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
  await waitForHTTP(`${url}/api/workspaces`);
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

// call returns the raw outcome of a request, refusals included: the
// no-workspace scenario asserts on a 409, which apiJSON would turn into an
// exception.
async function call(url, init = {}) {
  const response = await fetch(url, {
    ...init,
    headers: { Accept: "application/json", ...(init.headers || {}) },
  });
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = null;
  }
  return { status: response.status, body, text };
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

function assertNonEmpty(value, label) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`Expected ${label} to be a non-empty string, got ${JSON.stringify(value)}`);
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
