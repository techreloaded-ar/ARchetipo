#!/usr/bin/env node

// Smoke test for US-046: opening a workspace from the home without restarting
// the viewer.
//
// It walks the demonstration written in the story from end to end. Everything
// is real except the browser: the CLI is built from source, the viewer is the
// real one, both workspaces are real directories with different backlogs, and
// every assertion is made on the HTTP contract or on the filesystem. There is
// no fake server and no arbitrary sleep — every wait polls a viewer route.
//
// The single fact the whole story rests on is asserted throughout: the PID of
// the viewer never changes. A test that restarted the process would prove
// nothing about "without restarting the viewer".

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { apiJSON, buildCLI as buildCLIShared, createRunDir as createRunDirShared, makeRunCommand, parseCommonArgs, stopProcess as stopProcessShared, waitForHTTP } from "./support/view-smoke-harness.mjs";
import { startViewServer as startViewServerShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "open-workspace-view-smoke");
const baseEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

const CODE_A = "US-A01";
const CODE_B = "US-B01";

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const targetsDir = path.join(runDir, "targets");
  const stateDir = path.join(runDir, "state");
  const registryFile = path.join(stateDir, "workspaces.json");
  // Every process started from here writes the registry inside the run
  // directory, never in the real state of the machine.
  const cliEnv = { ...baseEnv, ARCHETIPO_STATE_DIR: stateDir };
  const verified = [];

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(targetsDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  try {
    await buildCLI(cliEnv);

    // Two real workspaces with different backlogs. The backlog is the oracle:
    // a spec code that exists in only one of them is what makes "which
    // workspace is being served" an observable fact rather than an opinion.
    const dirA = await createWorkspace(runDir, targetsDir, "alfa", CODE_A, cliEnv);
    const dirB = await createWorkspace(runDir, targetsDir, "beta", CODE_B, cliEnv);

    view = await startViewServer(dirA, cliEnv);
    console.log(`-> view ready: ${view.url} (pid ${view.child.pid})`);
    const pidBefore = view.child.pid;

    // B is known but not open.
    await postExpectingStatus(`${view.url}/api/workspaces`, { path: dirB }, 201);

    const listBefore = await apiJSON(`${view.url}/api/workspaces`);
    const entryA = await findByRealPath(listBefore.workspaces, await fs.realpath(dirA));
    const entryB = await findByRealPath(listBefore.workspaces, await fs.realpath(dirB));
    if (!entryA || !entryB) {
      throw new Error(`both workspaces must be known: ${JSON.stringify(listBefore.workspaces)}`);
    }
    const lastOpenedBefore = Date.parse(entryB.lastOpenedAt);

    // Before the switch the viewer serves A.
    assertSameList(await boardCodes(view.url), [CODE_A], "the board before the switch");
    const configBefore = await apiJSON(`${view.url}/api/config`);
    assertUnder(configBefore.path, dirA, "the config path before the switch");
    const actionsBefore = await apiJSON(`${view.url}/api/workspace/actions`);
    assertEqual(actionsBefore.has_prd, true, "the PRD flag of A before the switch");

    // AC-1, AC-2 — open B from the registry.
    console.log(`-> opening ${entryB.id} (${dirB})`);
    const opened = await postExpectingStatus(
      `${view.url}/api/workspaces/${encodeURIComponent(entryB.id)}/open`,
      {},
      200,
    );
    assertEqual(opened.body?.current, true, "the opened workspace marked as current");

    assertSameList(await boardCodes(view.url), [CODE_B], "the board after the switch");
    const configAfter = await apiJSON(`${view.url}/api/config`);
    assertUnder(configAfter.path, dirB, "the config path after the switch");
    const actionsAfter = await apiJSON(`${view.url}/api/workspace/actions`);
    assertEqual(actionsAfter.has_prd, false, "the PRD flag of B after the switch");

    assertEqual(view.child.pid, pidBefore, "the viewer PID after the switch");
    assertAlive(view.child, "the viewer process after the switch");
    verified.push("AC-1 — opening a workspace from the registry makes the viewer serve its board");
    verified.push("AC-2 — configuration, backlog and offered actions all follow the opened workspace, on the same process");

    // AC-3 — nothing of A survives the switch.
    const orphan = await statusOf(`${view.url}/api/spec/${CODE_A}`);
    assertEqual(orphan, 404, `the status of GET /api/spec/${CODE_A} after the switch`);
    verified.push("AC-3 — no spec and no code of the previous workspace is reachable after the switch");

    // AC-5 — the last access moved, in the API and on disk.
    const listAfter = await apiJSON(`${view.url}/api/workspaces`);
    const openedEntry = listAfter.workspaces.find((e) => e.id === entryB.id);
    if (!openedEntry) {
      throw new Error("AC-5: the opened workspace disappeared from the list");
    }
    const lastOpenedAfter = Date.parse(openedEntry.lastOpenedAt);
    if (!(lastOpenedAfter > lastOpenedBefore)) {
      throw new Error(`AC-5: lastOpenedAt did not move: ${openedEntry.lastOpenedAt} <= ${entryB.lastOpenedAt}`);
    }
    assertEqual(listAfter.workspaces[0].id, entryB.id, "the workspace at the head of the list");
    const onDisk = JSON.parse(await fs.readFile(registryFile, "utf8"));
    const diskEntry = onDisk.workspaces.find((e) => e.id === entryB.id);
    if (!diskEntry || !(Date.parse(diskEntry.lastOpenedAt) > lastOpenedBefore)) {
      throw new Error(`AC-5: the registry file did not record the access: ${JSON.stringify(diskEntry)}`);
    }
    verified.push("AC-5 — a successful open updates the last access, in the list and in the registry file");

    // AC-4 — the workspace now to be made unreachable is A, because B is the
    // one being served.
    const lastOpenedAbefore = listAfter.workspaces.find((e) => e.id === entryA.id).lastOpenedAt;
    await fs.rename(dirA, path.join(targetsDir, "alfa-spostato"));

    const refused = await postExpectingStatus(
      `${view.url}/api/workspaces/${encodeURIComponent(entryA.id)}/open`,
      {},
      404,
    );
    const reason = typeof refused.body?.error === "string" ? refused.body.error : JSON.stringify(refused.body);
    if (!/no longer exists|not readable|not a directory|not a workspace/.test(reason)) {
      throw new Error(`AC-4: the refusal does not name the probed reason: ${reason}`);
    }

    // The workspace that was open is not merely readable: it is usable.
    assertSameList(await boardCodes(view.url), [CODE_B], "the board after the refused open");
    const configStill = await apiJSON(`${view.url}/api/config`);
    assertUnder(configStill.path, dirB, "the config path after the refused open");
    await postExpectingStatus(`${view.url}/api/spec`, {
      epic_code: "EP-002",
      title: "Ancora scrivibile",
      priority: "LOW",
      points: 1,
      body: writableSpecBody(),
    }, 201);
    const afterWrite = await boardCodes(view.url);
    if (afterWrite.length !== 2 || !afterWrite.includes(CODE_B)) {
      throw new Error(`AC-4: the write did not land on the workspace still open: ${JSON.stringify(afterWrite)}`);
    }
    assertEqual(view.child.pid, pidBefore, "the viewer PID after the refused open");
    assertAlive(view.child, "the viewer process after the refused open");

    const listFinal = await apiJSON(`${view.url}/api/workspaces`);
    const entryAfinal = listFinal.workspaces.find((e) => e.id === entryA.id);
    assertEqual(entryAfinal.lastOpenedAt, lastOpenedAbefore, "the last access of the refused workspace");
    verified.push("AC-4 — a refused open declares its reason, leaves the previous workspace served and writable, and records no access");

    console.log("\nPASS: open-workspace view smoke test completed.");
    for (const line of verified) {
      console.log(`  ✓ ${line}`);
    }
    console.log(`Run directory: ${runDir}`);
    console.log(`State directory: ${stateDir}`);
  } finally {
    if (view) {
      await stopProcess(view.child);
    }
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned workspace: ${runDir}`);
    }
  }
}

// createWorkspace initializes one real workspace with a backlog holding a
// single recognisable spec code. Only the first one gets a PRD, so the offered
// workspace actions differ between the two and AC-2 has something to observe
// beyond the board.
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
      epic: { code: name === "alfa" ? "EP-001" : "EP-002", title: `Smoke ${name}` },
      priority: "LOW",
      points: 1,
      status: "TODO",
      body: `Story di test del workspace ${name}.`,
    }],
  }, null, 2));
  await runCommand(`spec-add-${name}`, cliPath, ["spec", "add", "--file", specsFile], { cwd: dir, env });

  if (name === "alfa") {
    await fs.mkdir(path.join(dir, "docs"), { recursive: true });
    await fs.writeFile(path.join(dir, "docs", "PRD.md"), `# PRD ${name}\n\nProdotto di prova.\n`, "utf8");
  }
  return dir;
}

// writableSpecBody is the smallest body the spec validator accepts, because the
// point of the write is that it lands, not what it says.
function writableSpecBody() {
  return [
    "**User Story**",
    "Come persona voglio verificare che il workspace aperto sia ancora scrivibile.",
    "",
    "**Dimostrazione**",
    "La spec appare nella colonna TODO del workspace aperto.",
    "",
    "**Criteri di accettazione**",
    "- [ ] AC-1 — la spec appare nella colonna TODO.",
    "",
  ].join("\n");
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

async function statusOf(url) {
  const response = await fetch(url, { headers: { Accept: "application/json" } });
  await response.text();
  return response.status;
}

function assertUnder(actual, root, label) {
  if (typeof actual !== "string" || !actual.startsWith(root)) {
    throw new Error(`Unexpected ${label}: expected a path under ${root}, got ${JSON.stringify(actual)}`);
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
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for opening a workspace from the home of archetipo view

Usage:
  node ./test/e2e/open-workspace-view-smoke.mjs
  npm run test:view-open-workspace-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated run directory
  --cleanup               Remove the run directory after the test passes/fails
`);
}

async function createRunDir(root) {
  return createRunDirShared(root, false);
}

async function buildCLI() {
  return buildCLIShared(cliPath, repoRoot, runCommand);
}

async function startViewServer(cwd, env) {
  return startViewServerShared(cliPath, cwd, env, "/api/workspaces");
}



async function postExpectingStatus(url, payload, expectedStatus) {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
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

async function runCommand(label, command, args, options = {}) {
  return makeRunCommand(baseEnv)(label, command, args, options);
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
