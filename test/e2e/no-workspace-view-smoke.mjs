#!/usr/bin/env node

// Smoke test for US-051: opening ARchetipo without being inside a workspace.
//
// It walks the demonstration written in the story from end to end. Everything
// is real except the browser: the CLI is built from source, the viewer is the
// real one, the workspaces are real directories with different backlogs, the
// registry of known workspaces is the real one, and every assertion is made on
// the HTTP contract or on the filesystem. There is no fake server, no
// credential, and no arbitrary sleep — every wait polls a viewer route with an
// explicit timeout that names what was expected and what arrived.
//
// The registry lives in a user-level state directory, so every process this
// script starts is given ARCHETIPO_STATE_DIR pointed inside its own run
// directory. Without that, the smoke would write into the real registry of
// whoever runs it — and AC-4, which is a byte-for-byte comparison of that very
// file, would be comparing someone else's state.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "no-workspace-view-smoke");
const baseEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

const CODE_A = "US-A01";
const CODE_B = "US-B01";
const CODE_O = "US-O01";

// The marker of an initialized workspace, the same relative path the CLI looks
// for when it walks up from the launch directory.
const WORKSPACE_MARKER = path.join(".archetipo", "config.yaml");

// The routes that presuppose an open workspace. This is the list the story
// names: one per family, so a regression that unguarded a whole area cannot
// hide behind a sibling that is still guarded.
const WORKSPACE_SCOPED_ROUTES = [
  { method: "GET", path: "/api/board" },
  { method: "GET", path: "/api/metrics" },
  { method: "GET", path: "/api/workspace/status" },
  { method: "GET", path: "/api/workspace/actions" },
  { method: "GET", path: "/api/prd" },
  { method: "GET", path: "/api/config" },
  { method: "GET", path: "/api/execution/providers" },
  { method: "GET", path: "/api/mockups" },
  { method: "POST", path: "/api/workspace/execution", body: {} },
];

// The keys a refusal must never carry: they are the payload keys of the routes
// above, and their absence is what tells "no workspace is open" apart from
// "here is your empty board".
const CONTENT_KEYS = ["columns", "specs", "providers", "stage", "content"];

const CHOSEN_PATHS = {
  prd: "docs/prodotto.md",
  wiki: "docs/kb/",
  mockups: "docs/mock/",
  test_results: "docs/esiti/",
};
const CHOSEN_WORKTREE = {
  enabled: false,
  base: "main",
  dir: ".archetipo/wt",
  branch_prefix: "us/",
};

// One entry per proved statement, for the report.
const checks = [];

function ok(criterion, statement) {
  checks.push({ criterion, statement });
  console.log(`-> ${criterion} ok: ${statement}`);
}

// openCalls counts every POST .../open this script has issued. AC-3 rests on
// the fact that the workspace was already open, so the count must not move
// while that part of the story runs.
let openCalls = 0;

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(binDir, { recursive: true });

  const startedAt = Date.now();
  let failure = null;
  try {
    await scenarioStartOutsideAnyWorkspace(runDir);
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
  console.log(`\nPASS: no-workspace view smoke test completed (${checks.length} statements proved).`);
  for (const check of checks) {
    console.log(`  ✓ ${check.criterion} — ${check.statement}`);
  }
  console.log(`Run directory: ${runDir}`);
}

// The whole story on one set of directories, because it is one story: where
// ARchetipo was started from decides what it opens, and nothing else does.
async function scenarioStartOutsideAnyWorkspace(runDir) {
  const targetsDir = path.join(runDir, "targets");
  const stateDir = path.join(runDir, "state");
  const registryFile = path.join(stateDir, "workspaces.json");
  const cliEnv = { ...baseEnv, ARCHETIPO_STATE_DIR: stateDir };

  await fs.mkdir(targetsDir, { recursive: true });
  // The neutral directory lives outside the repository, not inside the run
  // directory, for the reason the whole story is about: the CLI walks *up*
  // from the launch directory, and this repository is itself an ARchetipo
  // workspace. A `neutral/` under test/workspaces/ would resolve to it, and
  // the smoke would silently test the opposite scenario. The run directory
  // still holds every artifact; only the launch point is elsewhere.
  const neutralDir = await fs.mkdtemp(path.join(os.tmpdir(), "archetipo-neutral-"));

  await buildCLI(cliEnv);

  // Two real workspaces with different backlogs, plus a third that will be
  // moved away. The backlog is the oracle: a spec code that exists in only one
  // of them turns "which workspace is being served" into an observable fact.
  const dirA = await createWorkspace(runDir, targetsDir, "alfa", CODE_A, cliEnv);
  const dirB = await createWorkspace(runDir, targetsDir, "beta", CODE_B, cliEnv);
  const dirO = await createWorkspace(runDir, targetsDir, "orfano", CODE_O, cliEnv);
  // A directory *inside* beta, to prove later that the walk up to the
  // workspace root happens.
  const dirBdeep = path.join(dirB, "docs", "dettagli");
  await fs.mkdir(dirBdeep, { recursive: true });

  // The neutral directory is only neutral if no ancestor of it carries the
  // marker: the CLI walks up, so a workspace three levels above would make the
  // whole scenario a different one, silently.
  await assertNoWorkspaceAbove(neutralDir);
  ok("setup", `no ancestor of the launch directory ${neutralDir} carries ${WORKSPACE_MARKER}`);

  let seed;
  let view;
  try {
    // Seeding the registry: starting inside alfa records alfa, and the two
    // others are added over HTTP. This is the state a person would already
    // have before ever launching ARchetipo from somewhere else.
    seed = await startViewServer(dirA, cliEnv);
    console.log(`-> seed view ready: ${seed.url} (pid ${seed.child.pid})`);
    await postExpectingStatus(`${seed.url}/api/workspaces`, { path: dirB }, 201);
    await postExpectingStatus(`${seed.url}/api/workspaces`, { path: dirO }, 201);
    await stopProcess(seed.child);
    seed = null;

    // The third entry is made unreachable, so the home has to render a
    // workspace it cannot open as well as two it can.
    const dirOmoved = path.join(targetsDir, "orfano-spostato");
    await fs.rename(dirO, dirOmoved);

    // The baseline of AC-4: the exact bytes of the registry before ARchetipo
    // is started outside any workspace.
    const registryBefore = await fs.readFile(registryFile);

    // --- the launch this story is about -------------------------------------
    view = await startViewServer(neutralDir, cliEnv);
    const homePid = view.child.pid;
    console.log(`-> home view ready: ${view.url} (pid ${homePid})`);

    // AC-1 — the answer to the very first question the home asks.
    const home = view.firstList;
    assertEqual(home.open, false, "`open` on the first list served outside any workspace");
    assertEqual(home.currentPath, "", "`currentPath` on the first list served outside any workspace");
    assertEqual(home.workspaces.length, 3, "the number of known workspaces offered by the home");

    const realA = await fs.realpath(dirA);
    const realB = await fs.realpath(dirB);
    const realNeutral = await fs.realpath(neutralDir);
    const entryA = await findByRealPath(home.workspaces, realA);
    const entryB = await findByRealPath(home.workspaces, realB);
    const entryO = home.workspaces.find((e) => e !== entryA && e !== entryB);
    if (!entryA || !entryB || !entryO) {
      throw new Error(`AC-1: the home does not list the three known workspaces: ${JSON.stringify(home.workspaces)}`);
    }
    for (const entry of home.workspaces) {
      assertNonEmpty(entry.name, `the name of ${entry.id}`);
      assertNonEmpty(entry.path, `the path of ${entry.id}`);
      assertNonEmpty(entry.lastOpenedAt, `the last access of ${entry.id}`);
      if (Number.isNaN(Date.parse(entry.lastOpenedAt))) {
        throw new Error(`AC-1: the last access of ${entry.id} is not a date: ${JSON.stringify(entry.lastOpenedAt)}`);
      }
      assertEqual(entry.current, false, `the \`current\` flag of ${entry.id} while no workspace is open`);
    }
    assertEqual(entryA.status, "reachable", "the probed status of alfa");
    assertEqual(entryA.reachable, true, "the reachability of alfa");
    assertEqual(entryB.status, "reachable", "the probed status of beta");
    assertEqual(entryB.reachable, true, "the reachability of beta");
    assertEqual(entryO.status, "missing", "the probed status of the workspace that was moved away");
    assertEqual(entryO.reachable, false, "the reachability of the workspace that was moved away");
    assertEqual(entryO.name, "orfano", "the remembered name of the workspace that was moved away");
    ok("AC-1", "started in a neutral directory the viewer opens the home: open:false, currentPath empty, three entries with name, path, last access and probed reachability, none marked current");

    // AC-5 — every route that presupposes a workspace says so.
    for (const route of WORKSPACE_SCOPED_ROUTES) {
      const answer = await call(view.url, route);
      if (answer.status !== 409) {
        throw new Error(`AC-5: ${route.method} ${route.path} answered ${answer.status}, want 409: ${answer.text}`);
      }
      if (answer.body === null || typeof answer.body !== "object") {
        throw new Error(`AC-5: ${route.method} ${route.path} refused without a JSON body: ${answer.text}`);
      }
      assertEqual(answer.body.workspaceOpen, false, `\`workspaceOpen\` in the refusal of ${route.method} ${route.path}`);
      for (const key of CONTENT_KEYS) {
        if (key in answer.body) {
          throw new Error(`AC-5: the refusal of ${route.method} ${route.path} carries "${key}" — it answered emptily instead of declaring that no workspace is open`);
        }
      }
    }
    ok("AC-5", `all ${WORKSPACE_SCOPED_ROUTES.length} workspace-scoped routes answer 409 with workspaceOpen:false and no content key`);

    // AC-4 — the launch directory left no trace at all.
    const registryAfterHome = await fs.readFile(registryFile);
    if (!registryBefore.equals(registryAfterHome)) {
      throw new Error(`AC-4: the registry changed on a launch outside any workspace:\n  before ${registryBefore.toString("utf8")}\n  after  ${registryAfterHome.toString("utf8")}`);
    }
    const neutralEntry = await findByRealPath(home.workspaces, realNeutral);
    if (neutralEntry) {
      throw new Error(`AC-4: the launch directory was added to the known workspaces: ${JSON.stringify(neutralEntry)}`);
    }
    ok("AC-4", "workspaces.json is byte-for-byte identical after the neutral launch, and the launch directory never appears among the entries");

    // AC-6 — add, forget and create, all from the home, with nothing open.
    const dirG = path.join(targetsDir, "gamma");
    await fs.mkdir(dirG, { recursive: true });
    await runCommand("init-gamma", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], {
      cwd: dirG,
      env: cliEnv,
    });
    const added = await postExpectingStatus(`${view.url}/api/workspaces`, { path: dirG }, 201);
    const gammaId = added.body?.id;
    assertNonEmpty(gammaId, "the id of the workspace added from the home");
    const withGamma = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(withGamma.open, false, "`open` after adding a workspace from the home");
    if (!withGamma.workspaces.some((e) => e.id === gammaId)) {
      throw new Error(`AC-6: the added workspace is not in the list: ${JSON.stringify(withGamma.workspaces)}`);
    }

    const gammaBefore = await listTree(dirG);
    const removal = await requestExpectingStatus(`${view.url}/api/workspaces/${encodeURIComponent(gammaId)}`, "DELETE", null, 204);
    if (removal.text !== "") {
      throw new Error(`AC-6: forgetting answered with a body: ${removal.text}`);
    }
    const withoutGamma = await apiJSON(`${view.url}/api/workspaces`);
    if (withoutGamma.workspaces.some((e) => e.id === gammaId)) {
      throw new Error(`AC-6: the forgotten workspace is still listed: ${JSON.stringify(withoutGamma.workspaces)}`);
    }
    const gammaAfter = await listTree(dirG);
    assertSameList(gammaAfter, gammaBefore, "the contents of the forgotten workspace directory");

    const dirD = path.join(targetsDir, "delta");
    const created = await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: dirD,
      connector: "file",
      tools: ["pi"],
      paths: CHOSEN_PATHS,
      worktree: CHOSEN_WORKTREE,
    }, 201);
    assertEqual(created.body?.dir, dirD, "the directory of the workspace created from the home");
    await assertExists(path.join(dirD, WORKSPACE_MARKER), "AC-6: the workspace created from the home");
    ok("AC-6", "with no workspace open the home can still add a known workspace, forget one — leaving its directory untouched — and create a new one on disk");

    // AC-2 — opening from the home, on the very same process.
    const betaId = entryB.id;
    const lastOpenedBefore = Date.parse(entryB.lastOpenedAt);
    openCalls += 1;
    const opened = await postExpectingStatus(
      `${view.url}/api/workspaces/${encodeURIComponent(betaId)}/open`,
      {},
      200,
    );
    assertEqual(opened.body?.current, true, "the opened workspace marked as current");
    assertEqual(view.child.pid, homePid, "the viewer PID after opening from the home");
    assertAlive(view.child, "the viewer process after opening from the home");

    assertSameList(await boardCodes(view.url), [CODE_B], "the board served after opening beta from the home");
    const listOpen = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(listOpen.open, true, "`open` after a workspace was opened from the home");
    assertEqual(await fs.realpath(listOpen.currentPath), realB, "the realpath of `currentPath` after opening beta");
    const openedEntry = listOpen.workspaces.find((e) => e.id === betaId);
    if (!openedEntry || !(Date.parse(openedEntry.lastOpenedAt) > lastOpenedBefore)) {
      throw new Error(`AC-2: the last access of beta did not move: ${JSON.stringify(openedEntry?.lastOpenedAt)} <= ${entryB.lastOpenedAt}`);
    }
    ok("AC-2", "a reachable workspace chosen from the home becomes the open one on the same PID: its board is served, the list reports open:true with its path, and its last access moves");

    await stopProcess(view.child);
    view = null;

    // AC-3 — started inside a workspace, from its root and from a directory
    // below it, ARchetipo opens that workspace without passing through the
    // home. The oracle is the first answer after readiness, plus the fact that
    // no open request was ever sent.
    for (const [label, cwd] of [["root", dirB], ["subdirectory", dirBdeep]]) {
      const openCallsBefore = openCalls;
      const inside = await startViewServer(cwd, cliEnv);
      try {
        console.log(`-> in-workspace view ready from the ${label}: ${inside.url} (pid ${inside.child.pid})`);
        assertEqual(inside.firstList.open, true, `\`open\` on the first list served from the ${label} of beta`);
        assertEqual(
          await fs.realpath(inside.firstList.currentPath),
          realB,
          `the realpath of \`currentPath\` on the first list served from the ${label} of beta`,
        );
        assertSameList(await boardCodes(inside.url), [CODE_B], `the board served from the ${label} of beta`);
        assertEqual(openCalls, openCallsBefore, `the number of open requests sent while starting from the ${label} of beta`);
      } finally {
        await stopProcess(inside.child);
      }
    }
    ok("AC-3", "started inside beta, from its root and from a directory below it, the viewer already serves beta — currentPath and board say so on the first answer, and no open request was ever sent");
  } finally {
    if (seed) await stopProcess(seed.child);
    if (view) await stopProcess(view.child);
    await fs.rm(neutralDir, { recursive: true, force: true });
  }
}

// createWorkspace initializes one real workspace with a backlog holding a
// single recognisable spec code.
async function createWorkspace(runDir, targetsDir, name, code, env) {
  const dir = path.join(targetsDir, name);
  await fs.mkdir(dir, { recursive: true });
  await runCommand(`init-${name}`, cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], {
    cwd: dir,
    env,
  });

  const specsFile = path.join(runDir, `specs-${name}.json`);
  await fs.writeFile(specsFile, JSON.stringify({
    specs: [{
      code,
      title: `Smoke ${name}`,
      epic: { code: "EP-001", title: `Smoke ${name}` },
      priority: "LOW",
      points: 1,
      status: "TODO",
      body: `Story di test del workspace ${name}.`,
    }],
  }, null, 2));
  await runCommand(`spec-add-${name}`, cliPath, ["spec", "add", "--file", specsFile], { cwd: dir, env });
  return dir;
}

// assertNoWorkspaceAbove walks up from dir to the filesystem root looking for
// the marker the CLI itself looks for. It goes all the way up rather than
// stopping at the run directory because the CLI does: an ancestor outside the
// repository would make the "neutral" directory anything but.
async function assertNoWorkspaceAbove(dir) {
  let current = path.resolve(dir);
  for (;;) {
    const marker = path.join(current, WORKSPACE_MARKER);
    try {
      await fs.stat(marker);
      throw new Error(`the directory meant to be outside every workspace has a workspace above it: ${marker}`);
    } catch (error) {
      if (error.code !== "ENOENT" && error.code !== "ENOTDIR") throw error;
    }
    const parent = path.dirname(current);
    if (parent === current) return;
    current = parent;
  }
}

async function boardCodes(url) {
  const board = await apiJSON(`${url}/api/board`);
  const codes = [];
  for (const column of board.columns || []) {
    for (const spec of column.specs || []) {
      codes.push(spec.code);
    }
  }
  codes.sort();
  return codes;
}

// listTree is the oracle of "forgetting does not delete": every file under the
// directory with its size, so a removal or a truncation both show up.
async function listTree(root) {
  const out = [];
  async function walk(dir) {
    const entries = await fs.readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      const rel = path.relative(root, full);
      if (entry.isDirectory()) {
        out.push(`d ${rel}`);
        await walk(full);
      } else {
        const stat = await fs.lstat(full);
        out.push(`f ${rel} ${stat.size}`);
      }
    }
  }
  await walk(root);
  out.sort();
  return out;
}

function assertNonEmpty(value, label) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`Unexpected ${label}: expected a non-empty string, got ${JSON.stringify(value)}`);
  }
}

async function assertExists(target, label) {
  try {
    await fs.stat(target);
  } catch (error) {
    throw new Error(`${label}: ${target} does not exist (${error.code})`);
  }
}

function assertAlive(child, label) {
  if (child.exitCode !== null || child.signalCode !== null) {
    throw new Error(`${label}: the process is gone (exit ${child.exitCode}, signal ${child.signalCode})`);
  }
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
  console.log(`Smoke test for starting archetipo view outside any workspace

Usage:
  node ./test/e2e/no-workspace-view-smoke.mjs
  npm run test:view-no-workspace-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated run directory
  --cleanup               Remove the run directory after the test passes/fails
`);
}

async function createRunDir(root) {
  await fs.mkdir(root, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const runDir = path.join(root, stamp);
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

// startViewServer starts the real viewer and returns, besides the process and
// its URL, the very first list it served. AC-3 is an assertion on that first
// answer, so it must be the one the readiness probe obtained, not a later one:
// polling until a workspace appears open would prove nothing.
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
      reject(new Error(`the viewer never announced its address (expected "ARchetipo view ready at <url>" on stderr within 15s)\nSTDERR:\n${stderr}\nSTDOUT:\n${stdout}`));
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
  const firstList = await waitForWorkspaceList(`${url}/api/workspaces`);
  return { child, url, firstList };
}

// waitForWorkspaceList polls the route that is always available — with or
// without an open workspace — and returns the first answer it serves. The
// timeout names what was expected and what arrived, because "timed out" alone
// makes a failing smoke unreadable.
async function waitForWorkspaceList(url, timeoutMs = 10000) {
  const started = Date.now();
  let lastOutcome = "no response at all";
  while (Date.now() - started < timeoutMs) {
    try {
      const response = await fetch(url, { headers: { Accept: "application/json" } });
      const text = await response.text();
      if (response.ok) {
        const data = JSON.parse(text);
        if (Array.isArray(data.workspaces) && typeof data.open === "boolean" && typeof data.currentPath === "string") {
          return data;
        }
        lastOutcome = `HTTP 200 with an unusable body: ${text}`;
      } else {
        lastOutcome = `HTTP ${response.status}: ${text}`;
      }
    } catch (error) {
      lastOutcome = `the request failed: ${error.message}`;
    }
    await delay(200);
  }
  throw new Error(`Timed out after ${timeoutMs}ms waiting for ${url} to serve a workspace list with \`workspaces\`, \`open\` and \`currentPath\`; last outcome: ${lastOutcome}`);
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

// call issues one request against a route description and never throws on a
// refusal: the refusal is what the AC-5 assertions are made of.
async function call(baseURL, route) {
  const init = {
    method: route.method,
    headers: { Accept: "application/json" },
  };
  if (route.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(route.body);
  }
  const response = await fetch(`${baseURL}${route.path}`, init);
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = null;
  }
  return { status: response.status, text, body };
}

async function requestExpectingStatus(url, method, payload, expectedStatus) {
  const init = { method, headers: { Accept: "application/json" } };
  if (payload !== null && payload !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(payload);
  }
  const response = await fetch(url, init);
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (response.status !== expectedStatus) {
    throw new Error(`Expected HTTP ${expectedStatus} for ${method} ${url}, got ${response.status}: ${text}`);
  }
  return { status: response.status, body, text };
}

function postExpectingStatus(url, payload, expectedStatus) {
  return requestExpectingStatus(url, "POST", payload ?? {}, expectedStatus);
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

// --- report -----------------------------------------------------------------

async function writeReport(runDir, { startedAt, durationMs, failure }) {
  const summary = {
    smoke: "no-workspace-from-view",
    spec: "US-051",
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
  <title>ARchetipo Smoke — Starting outside a workspace (US-051)</title>
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
      <h1>ARchetipo Smoke — Starting outside a workspace (US-051)</h1>
      <div class="meta">
        <div><span class="label">Status</span><span class="value ${summary.passed ? "pass" : "fail"}">${summary.passed ? "PASS" : "FAIL"}</span></div>
        <div><span class="label">Started</span><span class="value">${escapeHTML(summary.started_at)}</span></div>
        <div><span class="label">Duration</span><span class="value">${(summary.duration_ms / 1000).toFixed(1)}s</span></div>
        <div><span class="label">Run directory</span><span class="value">${escapeHTML(summary.run_dir)}</span></div>
      </div>
    </header>

    <h2>Scenario</h2>
    <p>Three real workspaces — <code>alfa</code>, <code>beta</code> and one moved away after being
    registered — plus a <code>neutral/</code> directory with no workspace above it. The real
    <code>archetipo view</code> is launched from <code>neutral/</code>, so it opens the home; the home is
    then used to add, forget and create workspaces, and finally to open <code>beta</code> on the same
    process. The viewer is restarted inside <code>beta</code> and inside a directory below it to prove
    the home is skipped. The oracles are the HTTP contract — <code>open</code>,
    <code>currentPath</code>, the <code>409</code> with <code>workspaceOpen:false</code>, the board
    codes — the viewer PID, and the bytes of <code>workspaces.json</code> on disk.</p>

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
