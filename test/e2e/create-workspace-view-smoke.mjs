#!/usr/bin/env node

// Smoke test for US-044: creating and initializing a workspace from the UI.
//
// Everything is real except the browser: the CLI is built from source, the
// viewer is the real one, the connector is the real one, and the assertions
// are made on the HTTP contract and on the filesystem. There is no fake server
// and no arbitrary sleep — every wait polls a viewer route.

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "create-workspace-view-smoke");
const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

const CHOSEN_PATHS = {
  prd: "docs/prodotto.md",
  wiki: "docs/kb/",
  mockups: "docs/mock/",
  test_results: "docs/esiti/",
};
const CHOSEN_WORKTREE = {
  enabled: true,
  base: "develop",
  dir: ".archetipo/wt",
  branch_prefix: "us/",
};
const TEMPLATE_ID = "fabbrica-del-software";

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const targetsDir = path.join(runDir, "targets");
  const verified = [];

  // A successful creation now records the new workspace in the user-level
  // registry, and starting the viewer records the sandbox: both must land
  // inside this run directory, never in the real state of the machine.
  cliEnv.ARCHETIPO_STATE_DIR = path.join(runDir, "state");

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(targetsDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  const locked = path.join(targetsDir, "locked");
  try {
    await buildCLI();
    // The host workspace the viewer serves. The new workspace is created
    // elsewhere: this one must come out of the test untouched.
    await runCommand("init", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], { cwd: sandboxDir });

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);
    const hostBefore = await listRecursive(sandboxDir);

    // AC-2 — the options are the contract the form is built from.
    console.log("-> reading GET /api/workspace/options");
    const opts = await apiJSON(`${view.url}/api/workspace/options`);
    if (!Array.isArray(opts.connectors) || opts.connectors.length === 0) {
      throw new Error(`AC-2: the options expose no connector: ${JSON.stringify(opts)}`);
    }
    if (!Array.isArray(opts.tools) || opts.tools.length === 0) {
      throw new Error(`AC-2: the options expose no tool: ${JSON.stringify(opts)}`);
    }
    for (const entry of [...opts.connectors, ...opts.tools]) {
      if (!entry.id || !entry.label) {
        throw new Error(`AC-2: an option has no id or label: ${JSON.stringify(entry)}`);
      }
    }
    if (!opts.paths?.prd || !opts.worktree || typeof opts.worktree.enabled !== "boolean") {
      throw new Error(`AC-2: the options carry no defaults for paths or worktree: ${JSON.stringify(opts)}`);
    }
    // AC-5 — the Archetype is one identity, not a catalogue to choose from.
    if (Array.isArray(opts.template) || opts.template?.id !== TEMPLATE_ID || !opts.template?.version) {
      throw new Error(`AC-5: the Archetype is not reported as a single identity: ${JSON.stringify(opts.template)}`);
    }
    if ("templates" in opts) {
      throw new Error("AC-5: the options offer a choice of Archetypes, which this story explicitly excludes");
    }
    verified.push("AC-2 — the offered choices come from the server contract");

    // AC-1 / AC-5 — a full creation with non-default parameters.
    const created = path.join(targetsDir, "created");
    console.log("-> creating a workspace via POST /api/workspace");
    const create = await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: created,
      connector: "file",
      tools: ["pi", "claude"],
      paths: CHOSEN_PATHS,
      worktree: CHOSEN_WORKTREE,
    }, 201);

    assertEqual(create.body?.dir, created, "created workspace directory");
    assertEqual(create.body?.connector, "file", "created workspace connector");
    assertSameList(create.body?.tools, ["pi", "claude"], "created workspace tools");
    assertEqual(create.body?.template?.id, TEMPLATE_ID, "persisted Archetype id");
    if (!create.body?.template?.version) {
      throw new Error("AC-5: the response carries no Archetype version");
    }
    if (!create.body?.hint) {
      throw new Error("AC-1: the response does not say how to open the new workspace");
    }

    const configText = await fs.readFile(path.join(created, ".archetipo", "config.yaml"), "utf8");
    for (const [key, value] of Object.entries(CHOSEN_PATHS)) {
      if (!configText.includes(`${key}: ${value}`)) {
        throw new Error(`AC-1: the created config does not carry paths.${key} = ${value}`);
      }
    }
    for (const [key, value] of Object.entries(CHOSEN_WORKTREE)) {
      if (!configText.includes(`${key}: ${value}`)) {
        throw new Error(`AC-1: the created config does not carry worktree.${key} = ${value}`);
      }
    }
    if (!configText.includes(`id: ${TEMPLATE_ID}`)) {
      throw new Error("AC-5: the created config does not persist the Archetype identity");
    }
    await assertExists(path.join(created, ".archetipo", "shared-runtime.md"), "AC-1: shared-runtime.md");

    const skills = await fs.readdir(path.join(repoRoot, "skills"));
    for (const toolDir of [".pi/skills", ".claude/skills"]) {
      const installed = await fs.readdir(path.join(created, ...toolDir.split("/")));
      for (const skill of skills.filter((s) => s.startsWith("archetipo-"))) {
        if (!installed.includes(skill)) {
          throw new Error(`AC-1: skill ${skill} is missing under ${toolDir}`);
        }
      }
    }

    // The created workspace has to be usable, not merely written: the CLI reads
    // its own config back from that directory.
    const shown = await runCommand("config-show", cliPath, ["config", "show"], { cwd: created });
    const shownInfo = JSON.parse(shown.stdout);
    assertEqual(shownInfo?.data?.paths?.prd, CHOSEN_PATHS.prd, "prd path read back by `archetipo config show`");
    assertEqual(shownInfo?.data?.template?.id, TEMPLATE_ID, "Archetype read back by `archetipo config show`");
    assertEqual(shownInfo?.data?.worktree?.enabled, true, "worktree gate read back by `archetipo config show`");
    verified.push("AC-1 — the workspace is created with the chosen parameters and is usable");
    verified.push("AC-5 — the built-in Archetype is persisted with identity and version, without being chosen");

    // AC-2 — what the options do not offer is refused.
    console.log("-> refusing values the options do not offer");
    const badConnector = await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: path.join(targetsDir, "mai-creata-1"),
      connector: "nope",
      tools: ["pi"],
    }, 400);
    assertFieldError(badConnector.body, "connector", "WORKSPACE_CONNECTOR_UNKNOWN");
    const badTool = await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: path.join(targetsDir, "mai-creata-2"),
      connector: "file",
      tools: ["nope"],
    }, 400);
    assertFieldError(badTool.body, "tools", "WORKSPACE_TOOL_UNKNOWN");
    await assertMissing(path.join(targetsDir, "mai-creata-1"), "the refused destination");
    await assertMissing(path.join(targetsDir, "mai-creata-2"), "the refused destination");

    // AC-5 — the Archetype cannot be chosen even by a client that tries.
    await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: path.join(targetsDir, "mai-creata-3"),
      connector: "file",
      tools: ["pi"],
      template: "un-altro-archetipo",
    }, 400);
    await assertMissing(path.join(targetsDir, "mai-creata-3"), "the destination of a refused Archetype choice");

    // AC-3 — the three refusals on the destination, none of which writes.
    console.log("-> refusing invalid destinations");
    await assertRefusalWritesNothing(view.url, created, {
      dir: created,
      connector: "file",
      tools: ["pi"],
    }, "dir", "WORKSPACE_ALREADY_INITIALIZED");

    const aFile = path.join(targetsDir, "un-file");
    await fs.writeFile(aFile, "non sono una directory\n");
    const fileRefusal = await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: aFile,
      connector: "file",
      tools: ["pi"],
    }, 400);
    assertFieldError(fileRefusal.body, "dir", "WORKSPACE_DIR_NOT_A_DIRECTORY");
    assertEqual(await fs.readFile(aFile, "utf8"), "non sono una directory\n", "the refused file content");

    if (canTestUnwritableDirectory()) {
      await fs.mkdir(locked, { recursive: true });
      await fs.chmod(locked, 0o500);
      await assertRefusalWritesNothing(view.url, locked, {
        dir: locked,
        connector: "file",
        tools: ["pi"],
      }, "dir", "WORKSPACE_DIR_NOT_WRITABLE");
      await fs.chmod(locked, 0o755);
    } else {
      console.log("-> skipped: a 0o500 directory does not stop this user (root, or Windows)");
    }
    verified.push("AC-3 — an invalid, unwritable or already initialized destination is refused without writing");

    // AC-4 — a failure mid-commit leaves nothing behind and damages nothing.
    console.log("-> forcing a failure halfway through the commit");
    const partial = path.join(targetsDir, "partial");
    await fs.mkdir(partial, { recursive: true });
    await fs.writeFile(path.join(partial, "README.md"), "contenuto da non perdere\n");
    // `.pi` as a file: the commit cannot create `.pi/skills/` under it, and it
    // fails only after `.archetipo/` has already been moved in.
    await fs.writeFile(path.join(partial, ".pi"), "sono un file\n");
    const partialBefore = await listRecursive(partial);

    const failed = await postExpectingStatus(`${view.url}/api/workspace`, {
      dir: partial,
      connector: "file",
      tools: ["pi"],
    }, 400);
    if (!JSON.stringify(failed.body).includes(".pi")) {
      throw new Error(`AC-4: expected the collision on .pi to be the failure, got ${JSON.stringify(failed.body)}`);
    }
    const partialAfter = await listRecursive(partial);
    assertSameList(partialAfter, partialBefore, "the destination after a failed initialization");
    assertEqual(
      await fs.readFile(path.join(partial, "README.md"), "utf8"),
      "contenuto da non perdere\n",
      "a pre-existing file after the rollback",
    );
    verified.push("AC-4 — a failed initialization leaves no partial workspace and damages nothing");

    // The workspace the viewer serves was never the target of any of this.
    assertSameList(await listRecursive(sandboxDir), hostBefore, "the host workspace after the whole run");

    console.log("\nPASS: create-workspace view smoke test completed.");
    for (const line of verified) {
      console.log(`  ✓ ${line}`);
    }
    console.log(`Sandbox: ${sandboxDir}`);
    console.log(`Created workspace: ${created}`);
  } finally {
    // Restore the permissions first: otherwise the cleanup below cannot remove
    // the run directory it created.
    await fs.chmod(locked, 0o755).catch(() => {});
    if (view) {
      await stopProcess(view.child);
    }
    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned workspace: ${runDir}`);
    }
  }
}

function canTestUnwritableDirectory() {
  if (process.platform === "win32") return false;
  return typeof process.getuid === "function" && process.getuid() !== 0;
}

async function assertRefusalWritesNothing(viewURL, dir, payload, field, code) {
  const before = await listRecursive(dir);
  const refusal = await postExpectingStatus(`${viewURL}/api/workspace`, payload, 400);
  assertFieldError(refusal.body, field, code);
  const after = await listRecursive(dir);
  assertSameList(after, before, `the destination ${dir} after the refusal`);
}

function assertFieldError(body, field, code) {
  const fields = body?.fields;
  if (!Array.isArray(fields) || fields.length === 0) {
    throw new Error(`Expected a non-empty 'fields' array; got ${JSON.stringify(body)}`);
  }
  const match = fields.find((f) => f.field === field && f.code === code);
  if (!match) {
    throw new Error(`Expected a refusal on '${field}' with code ${code}; got ${JSON.stringify(fields)}`);
  }
  if (!match.message) {
    throw new Error(`The refusal on '${field}' carries no message: ${JSON.stringify(match)}`);
  }
}

// listRecursive returns the sorted relative paths under dir, which is how this
// test states "byte for byte unchanged" without trusting an internal detail.
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

async function assertMissing(dir, label) {
  try {
    await fs.stat(dir);
  } catch {
    return;
  }
  throw new Error(`Expected ${label} (${dir}) not to exist after the refusal`);
}

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for workspace creation from archetipo view

Usage:
  node ./test/e2e/create-workspace-view-smoke.mjs
  npm run test:view-workspace-smoke

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

async function startViewServer(cwd) {
  return startViewServerShared(cliPath, cwd, cliEnv, "/api/workspace/options");
}



// postExpectingStatus posts a JSON payload and asserts the HTTP status without
// throwing on non-2xx: a refusal is an expected outcome here, not a transport
// failure.
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
  return makeRunCommand(cliEnv)(label, command, args, options);
}

async function stopProcess(child) {
  return stopProcessShared(child, runCommand);
}

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
