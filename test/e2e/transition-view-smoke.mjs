#!/usr/bin/env node

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { apiJSON, buildCLI as buildCLIShared, createRunDir as createRunDirShared, makeRunCommand, parseCommonArgs, startViewServer as startViewServerShared, stopProcess as stopProcessShared } from "./support/view-smoke-harness.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "transition-view-smoke");
const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  process.env.ARCHETIPO_STATE_DIR = path.join(runDir, "state");
  cliEnv.ARCHETIPO_STATE_DIR = process.env.ARCHETIPO_STATE_DIR;
  const sandboxDir = path.join(runDir, "sandbox");
  const specsFile = path.join(runDir, "specs.json");

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  try {
    await buildCLI();
    await runCommand("init", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], { cwd: sandboxDir });

    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir });
    const staging = await seedTemporaryArtifacts(sandboxDir, "US-901");

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);

    console.log("-> asking the preview for TODO -> DONE");
    const preview = await apiJSON(`${view.url}/api/spec/US-901/transition-preview?to=done`);
    assertPreview(preview);

    console.log("-> moving US-901 to done via web API");
    await apiJSON(`${view.url}/api/board/move`, {
      method: "POST",
      body: JSON.stringify({ code: "US-901", to: "done" }),
      headers: { "Content-Type": "application/json" },
    });

    const spec = await apiJSON(`${view.url}/api/spec/US-901`);
    const status = spec.spec ? spec.spec.status : spec.status;
    if (status !== "DONE") {
      throw new Error(`Expected US-901 to be DONE, got ${status}`);
    }
    await assertSwept(staging);

    console.log("\nPASS: transition view smoke test completed.");
    console.log(`Sandbox: ${sandboxDir}`);
    console.log(`View URL: ${view.url}`);
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

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for spec status changes in archetipo view

Usage:
  node ./test/e2e/transition-view-smoke.mjs

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

async function writeSpecsPayload(file) {
  const payload = {
    specs: [
      {
        code: "US-901",
        title: "Smoke transition from the viewer",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per il cambio di stato via viewer.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

// The sweep that closing a spec owes the workspace has something to remove only
// if something is there: this is the leftover a planning run would have staged.
async function seedTemporaryArtifacts(sandboxDir, code) {
  const staging = path.join(sandboxDir, ".archetipo", "tmp", `plan-${code}`);
  await fs.mkdir(staging, { recursive: true });
  await fs.writeFile(path.join(staging, "notes.md"), "# staging\n");
  return staging;
}

function assertPreview(preview) {
  if (preview.from !== "todo" || preview.to !== "done") {
    throw new Error(`Expected a todo->done preview, got ${preview.from}->${preview.to}`);
  }
  if (preview.allowed !== true) {
    throw new Error(`Expected the transition to be allowed, got ${JSON.stringify(preview)}`);
  }
  if (!(preview.impacts || []).includes("skips_review")) {
    throw new Error(`Expected skips_review among the impacts, got ${JSON.stringify(preview.impacts)}`);
  }
}

async function assertSwept(target) {
  try {
    await fs.access(target);
  } catch (error) {
    if (error?.code === "ENOENT") return;
    throw error;
  }
  throw new Error(`Expected temporary artifacts to be swept: ${target}`);
}

async function startViewServer(cwd) {
  return startViewServerShared(cliPath, cwd, cliEnv, "/api/board");
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
