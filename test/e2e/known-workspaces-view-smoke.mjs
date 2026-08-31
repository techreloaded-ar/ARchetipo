#!/usr/bin/env node

// Smoke test for US-045: finding the workspaces you already know about.
//
// It walks the demonstration written in the story from end to end. Everything
// is real except the browser: the CLI is built from source, the viewer is the
// real one, the registry is the real one, and the assertions are made on the
// HTTP contract and on the filesystem. There is no fake server and no
// arbitrary sleep — every wait polls a viewer route.
//
// The registry lives in a user-level state directory, so every process this
// script starts is given ARCHETIPO_STATE_DIR pointed inside its own run
// directory. Without that, the smoke would write into the real registry of
// whoever runs it.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "known-workspaces-view-smoke");
const baseEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const targetsDir = path.join(runDir, "targets");
  const stateDir = path.join(runDir, "state");
  const specsFile = path.join(runDir, "specs.json");
  const registryFile = path.join(stateDir, "workspaces.json");
  // Every process started from here writes the registry inside the run
  // directory, never in the real state of the machine.
  const cliEnv = { ...baseEnv, ARCHETIPO_STATE_DIR: stateDir };
  const verified = [];

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(targetsDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  try {
    await buildCLI(cliEnv);
    // The host workspace the viewer serves. Opening it is what records it.
    await runCommand("init-sandbox", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], {
      cwd: sandboxDir,
      env: cliEnv,
    });

    // One spec, so that /api/board — the route this smoke uses to observe that
    // the viewer is operative — has a backlog to serve.
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], {
      cwd: sandboxDir,
      env: cliEnv,
    });

    // AC-5 — nothing has been written yet, and that is not a problem.
    await assertMissing(registryFile, "AC-5: the registry file before anything happened");

    view = await startViewServer(sandboxDir, cliEnv);
    console.log(`-> view ready: ${view.url}`);

    await apiJSON(`${view.url}/api/board`);
    const initial = await apiJSON(`${view.url}/api/workspaces`);
    if (!Array.isArray(initial.workspaces)) {
      throw new Error(`AC-5: the list is not an array: ${JSON.stringify(initial)}`);
    }

    // AC-1 (opening) — starting the viewer on a workspace records it.
    const sandboxReal = await fs.realpath(sandboxDir);
    const served = await findByRealPath(initial.workspaces, sandboxReal);
    if (!served) {
      throw new Error(`AC-1: the served workspace is not in the list: ${JSON.stringify(initial.workspaces)}`);
    }
    assertEqual(served.current, true, "the served workspace marked as current");
    // The registry lives outside every workspace.
    await assertExists(registryFile, "AC-1: the registry file in the state directory");
    const sandboxEntries = await listRecursive(sandboxDir);
    if (sandboxEntries.some((rel) => rel.endsWith("workspaces.json"))) {
      throw new Error("AC-1: the registry was written inside the workspace it should live outside of");
    }

    // AC-1 (creation) — a workspace created from the UI enters the registry.
    const created = path.join(targetsDir, "creato");
    console.log("-> creating a workspace via POST /api/workspace");
    await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: created,
      connector: "file",
      tools: ["pi"],
    }, 201);

    let list = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(list.workspaces.length, 2, "the number of known workspaces after the creation");
    const createdReal = await fs.realpath(created);
    const createdEntry = await findByRealPath(list.workspaces, createdReal);
    if (!createdEntry) {
      throw new Error(`AC-1: the created workspace is not in the list: ${JSON.stringify(list.workspaces)}`);
    }
    assertEqual(createdEntry.current, false, "the created workspace marked as current");
    verified.push("AC-1 — the served workspace and the one created from the UI both enter a registry that lives outside them");

    // AC-2 — name, path and last opened, on every entry.
    const startedAt = Date.now() - 60_000;
    for (const entry of list.workspaces) {
      if (!entry.name) {
        throw new Error(`AC-2: an entry has no name: ${JSON.stringify(entry)}`);
      }
      if (!path.isAbsolute(entry.path)) {
        throw new Error(`AC-2: the path is not absolute: ${JSON.stringify(entry)}`);
      }
      const at = Date.parse(entry.lastOpenedAt);
      if (Number.isNaN(at) || at <= 0) {
        throw new Error(`AC-2: lastOpenedAt does not parse as a date: ${JSON.stringify(entry)}`);
      }
      if (at < startedAt) {
        throw new Error(`AC-2: lastOpenedAt predates this run: ${JSON.stringify(entry)}`);
      }
    }
    assertEqual(createdEntry.name, path.basename(created), "the name of the created workspace");
    assertEqual(served.name, path.basename(sandboxDir), "the name of the served workspace");
    verified.push("AC-2 — the list reports name, path and last opened for every workspace");

    // Adding a workspace the viewer has never seen.
    const existing = path.join(targetsDir, "esistente");
    await fs.mkdir(existing, { recursive: true });
    await runCommand("init-existing", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], {
      cwd: existing,
      env: cliEnv,
    });
    const added = await postExpectingStatus(`${view.url}/api/workspaces`, { path: existing }, 201);
    assertEqual(added.body?.reachable, true, "the added workspace reachability");

    list = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(list.workspaces.length, 3, "the number of known workspaces after the addition");

    // A refusal names the offending field with a stable code.
    const refused = await postExpectingStatus(`${view.url}/api/workspaces`, { path: "relativo" }, 400);
    assertEqual(refused.body?.fields?.[0]?.field, "path", "the refused field");
    assertEqual(refused.body?.fields?.[0]?.code, "WORKSPACE_REGISTRY_PATH_NOT_ABSOLUTE", "the refusal code");
    list = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(list.workspaces.length, 3, "the number of known workspaces after a refused addition");

    // AC-3 — a renamed directory is reported, not dropped.
    await fs.rename(created, path.join(targetsDir, "creato-rinominato"));
    list = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(list.workspaces.length, 3, "the number of known workspaces after the rename");
    const renamed = list.workspaces.find((e) => e.id === createdEntry.id);
    if (!renamed) {
      throw new Error("AC-3: the renamed workspace silently disappeared from the list");
    }
    assertEqual(renamed.reachable, false, "the renamed workspace reachability");
    assertEqual(renamed.status, "missing", "the renamed workspace status");
    verified.push("AC-3 — a workspace that is no longer reachable is reported, not removed");

    // AC-4 — forgetting an entry does not touch the disk.
    const existingEntry = await findByRealPath(list.workspaces, await fs.realpath(existing));
    if (!existingEntry) {
      throw new Error(`AC-4: the added workspace is not in the list: ${JSON.stringify(list.workspaces)}`);
    }
    const before = await listRecursive(existing);
    await deleteExpectingStatus(`${view.url}/api/workspaces/${encodeURIComponent(existingEntry.id)}`, 204);

    list = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(list.workspaces.length, 2, "the number of known workspaces after the removal");
    if (list.workspaces.some((e) => e.id === existingEntry.id)) {
      throw new Error("AC-4: the removed entry is still listed");
    }
    assertSameList(await listRecursive(existing), before, "the removed workspace directory after the removal");
    await assertExists(path.join(existing, ".archetipo", "config.yaml"), "AC-4: the config of the forgotten workspace");
    verified.push("AC-4 — removing an entry from the registry does not touch the files on disk");

    // AC-5 — a corrupt registry blocks nothing and repairs itself.
    await fs.writeFile(registryFile, "not json", "utf8");
    await apiJSON(`${view.url}/api/board`);
    const corrupt = await apiJSON(`${view.url}/api/workspaces`);
    assertEqual(corrupt.workspaces.length, 0, "the number of known workspaces read from a corrupt registry");

    await postExpectingStatus(`${view.url}/api/workspaces`, { path: existing }, 201);
    const repaired = JSON.parse(await fs.readFile(registryFile, "utf8"));
    assertEqual(repaired.schema, 1, "the schema of the rewritten registry");
    assertEqual(repaired.workspaces?.length, 1, "the number of entries in the rewritten registry");
    verified.push("AC-5 — a missing or unreadable registry blocks nothing and is recreated on the first write");

    console.log("\nPASS: known-workspaces view smoke test completed.");
    for (const line of verified) {
      console.log(`  ✓ ${line}`);
    }
    console.log(`Sandbox: ${sandboxDir}`);
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

async function writeSpecsPayload(file) {
  const payload = {
    specs: [
      {
        code: "US-901",
        title: "Smoke known workspaces",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per il registro dei workspace conosciuti.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
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

async function listRecursive(dir) {
  const out = [];
  async function walk(current, prefix) {
    let entries;
    try {
      entries = await fs.readdir(current, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
      out.push(rel);
      if (entry.isDirectory()) {
        await walk(path.join(current, entry.name), rel);
      }
    }
  }
  await walk(dir, "");
  out.sort();
  return out;
}

async function assertExists(file, label) {
  try {
    await fs.stat(file);
  } catch (error) {
    throw new Error(`${label} is missing: ${error.message}`);
  }
}

async function assertMissing(target, label) {
  try {
    await fs.stat(target);
  } catch {
    return;
  }
  throw new Error(`${label} exists but should not`);
}

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for the known-workspaces registry of archetipo view

Usage:
  node ./test/e2e/known-workspaces-view-smoke.mjs
  npm run test:view-workspaces-smoke

Options:
  --workspace-root <dir>  Parent directory for the generated sandbox
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

async function deleteExpectingStatus(url, expectedStatus) {
  const response = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  const text = await response.text();
  if (response.status !== expectedStatus) {
    throw new Error(`Expected HTTP ${expectedStatus} for DELETE ${url}, got ${response.status}: ${text}`);
  }
  if (expectedStatus === 204 && text !== "") {
    throw new Error(`Expected an empty body for DELETE ${url}, got: ${text}`);
  }
  return text;
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
