#!/usr/bin/env node

import fs from "node:fs/promises";
import os from "node:os";
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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "delete-story-view-smoke");
const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  // Starting a viewer records its project root in the user-level registry of
  // known workspaces. This run directory is a throwaway, so the entry must go
  // with it instead of accumulating in the real registry of the machine.
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
    await seedPlanAndReviewArtifacts(sandboxDir, "US-901");

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);

    const initialBoard = await apiJSON(`${view.url}/api/board`);
    assertBoardHas(initialBoard, "US-901");
    assertBoardHas(initialBoard, "US-902");

    console.log("-> deleting US-901 via web API");
    await apiJSON(`${view.url}/api/spec/US-901`, { method: "DELETE" });

    const boardAfterDelete = await apiJSON(`${view.url}/api/board`);
    assertBoardMissing(boardAfterDelete, "US-901");
    assertBoardHas(boardAfterDelete, "US-902");

    await expectNotFound(`${view.url}/api/spec/US-901`);
    await assertDeletedArtifacts(sandboxDir, "US-901");

    console.log("\nPASS: delete-story view smoke test completed.");
    console.log(`Sandbox: ${sandboxDir}`);
    console.log(`View URL: ${view.url}`);
    console.log("Deleted spec: US-901");
    console.log("Remaining spec: US-902");
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
  console.log(`Smoke test for backlog-card deletion in archetipo view

Usage:
  node ./test/e2e/delete-story-view-smoke.mjs
  npm run test:view-delete-smoke

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
        title: "Smoke delete from card",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per la cancellazione via viewer.",
      },
      {
        code: "US-902",
        title: "Smoke survivor spec",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di controllo che deve restare nel backlog.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

async function seedPlanAndReviewArtifacts(sandboxDir, code) {
  const planPath = path.join(sandboxDir, ".archetipo", "plans", `${code}-plan.yaml`);
  const reviewPath = path.join(sandboxDir, ".archetipo", "reviews", `${code}.yaml`);
  await fs.mkdir(path.dirname(planPath), { recursive: true });
  await fs.mkdir(path.dirname(reviewPath), { recursive: true });
  await fs.writeFile(planPath, `schema: archetipo/plan/v2\nspec_code: ${code}\nbody: |\n  ## Smoke plan\n\n  Delete-path artifact cleanup check.\ntasks:\n  - id: TASK-01\n    title: Smoke task\n    description: Validate deletion cleanup\n    type: Impl\n    status: TODO\n`);
  await fs.writeFile(reviewPath, `schema: archetipo/review/v1\nspec_code: ${code}\ncomments:\n  - file: hello.txt\n    line: 1\n    side: new\n    body: Smoke review comment\n`);
}

async function startViewServer(cwd) {
  return startViewServerShared(cliPath, cwd, cliEnv, "/api/board");
}



function collectCodes(board) {
  return new Set((board.columns || []).flatMap((column) => (column.specs || []).map((spec) => spec.code)));
}

function assertBoardHas(board, code) {
  const codes = collectCodes(board);
  if (!codes.has(code)) {
    throw new Error(`Expected board to contain ${code}; got [${[...codes].join(", ")}]`);
  }
}

function assertBoardMissing(board, code) {
  const codes = collectCodes(board);
  if (codes.has(code)) {
    throw new Error(`Expected board to omit ${code}; got [${[...codes].join(", ")}]`);
  }
}

async function expectNotFound(url) {
  const response = await fetch(url, { headers: { Accept: "application/json" } });
  if (response.status !== 404) {
    const body = await response.text();
    throw new Error(`Expected 404 for ${url}, got ${response.status}: ${body}`);
  }
}

async function assertDeletedArtifacts(sandboxDir, code) {
  const paths = [
    path.join(sandboxDir, ".archetipo", "specs", `${code}.yaml`),
    path.join(sandboxDir, ".archetipo", "plans", `${code}-plan.yaml`),
    path.join(sandboxDir, ".archetipo", "reviews", `${code}.yaml`),
  ];
  for (const target of paths) {
    try {
      await fs.access(target);
      throw new Error(`Expected deleted artifact to be missing: ${target}`);
    } catch (error) {
      if (error?.message?.startsWith("Expected deleted artifact")) {
        throw error;
      }
      if (error?.code !== "ENOENT") {
        throw error;
      }
    }
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
