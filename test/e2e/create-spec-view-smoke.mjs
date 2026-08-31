#!/usr/bin/env node

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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "create-spec-view-smoke");
const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

const EPIC_CODE = "EP-900";
const CREATED_CODE = "US-903";
const NEW_SPEC_TITLE = "Creare una spec dalla UI";
const NEW_SPEC_BODY = [
  "**User Story**",
  "Come membro di un team di prodotto, voglio creare una nuova spec dalla UI del workspace.",
  "",
  "**Dimostrazione**",
  "Il revisore apre il viewer, conferma la modale di creazione e ritrova la nuova card in TODO.",
  "",
  "**Criteri di accettazione**",
  "- [ ] La spec compare nel backlog con codice progressivo e stato TODO",
  "- [ ] Il contenuto inviato è rileggibile dal connector",
  "",
].join("\n");

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

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);

    const initialBoard = await apiJSON(`${view.url}/api/board`);
    assertBoardHas(initialBoard, "US-901");
    assertBoardHas(initialBoard, "US-902");
    const initialCodes = collectCodes(initialBoard);
    assertEpicKnown(initialBoard, EPIC_CODE);

    // AC-2 - creation assigns the progressive code and persists a TODO spec.
    console.log("-> creating a spec via POST /api/spec");
    const created = await postExpectingStatus(`${view.url}/api/spec`, validPayload(), 201);
    assertEqual(created.body?.created, true, "created flag on first creation");
    assertEqual(created.body?.spec?.code, CREATED_CODE, "assigned spec code");
    assertEqual(created.body?.spec?.status, "TODO", "status of the created spec");

    const detail = await apiJSON(`${view.url}/api/spec/${CREATED_CODE}`);
    assertEqual(detail?.spec?.body, NEW_SPEC_BODY, "body read back from GET /api/spec/{code}");

    const shown = await runCommand("spec-show", cliPath, ["spec", "show", CREATED_CODE], { cwd: sandboxDir });
    if (!shown.stdout.includes(CREATED_CODE)) {
      throw new Error(`Expected 'archetipo spec show ${CREATED_CODE}' to expose the code; got:\n${shown.stdout}`);
    }

    // AC-2 - the board grows by exactly one card, in the todo column.
    const boardAfterCreate = await apiJSON(`${view.url}/api/board`);
    assertColumnHas(boardAfterCreate, "todo", CREATED_CODE);
    assertEqual(
      countCards(boardAfterCreate),
      countCards(initialBoard) + 1,
      "board card count after creation",
    );

    // AC-3 - an invalid payload answers 400 with per-field errors and touches nothing.
    console.log("-> rejecting an invalid payload");
    const invalid = await postExpectingStatus(`${view.url}/api/spec`, {
      epic_code: EPIC_CODE,
      title: "",
      priority: "MEDIUM",
      points: 3,
      body: "**Dimostrazione**\nNessun criterio di accettazione in checklist.\n",
    }, 400);
    const fields = invalid.body?.fields;
    if (!Array.isArray(fields) || fields.length === 0) {
      throw new Error(`Expected 400 body to carry a non-empty 'fields' array; got ${JSON.stringify(invalid.body)}`);
    }
    const fieldNames = new Set(fields.map((f) => f.field));
    for (const expected of ["title", "body"]) {
      if (!fieldNames.has(expected)) {
        throw new Error(`Expected field error on '${expected}'; got [${[...fieldNames].join(", ")}]`);
      }
    }

    const boardAfterInvalid = await apiJSON(`${view.url}/api/board`);
    assertSameCodes(collectCodes(boardAfterInvalid), new Set([...initialCodes, CREATED_CODE]), "board codes after the invalid attempt");
    const backlogAfterInvalid = await readBacklogFile(sandboxDir);
    if (backlogAfterInvalid.includes("US-904")) {
      throw new Error("Expected the backlog file to contain no US-904 after the invalid attempt");
    }

    // AC-4 - confirming the same spec again does not create a second one.
    console.log("-> repeating the same creation twice");
    const repeatOne = await postExpectingStatus(`${view.url}/api/spec`, validPayload(), 200);
    assertEqual(repeatOne.body?.created, false, "created flag on the first repeat");
    assertEqual(repeatOne.body?.spec?.code, CREATED_CODE, "spec code on the first repeat");

    const repeatTwo = await postExpectingStatus(`${view.url}/api/spec`, validPayload(), 200);
    assertEqual(repeatTwo.body?.created, false, "created flag on the second repeat");
    assertEqual(repeatTwo.body?.spec?.code, CREATED_CODE, "spec code on the second repeat");

    const boardAfterRepeat = await apiJSON(`${view.url}/api/board`);
    const repeated = countOccurrences(boardAfterRepeat, CREATED_CODE);
    if (repeated !== 1) {
      throw new Error(`Expected exactly one ${CREATED_CODE} card on the board; found ${repeated}`);
    }
    const backlogAfterRepeat = await readBacklogFile(sandboxDir);
    const backlogHits = countMatches(backlogAfterRepeat, CREATED_CODE);
    if (backlogHits !== 1) {
      throw new Error(`Expected exactly one ${CREATED_CODE} entry in the backlog file; found ${backlogHits}`);
    }

    console.log("\nPASS: create-spec view smoke test completed.");
    console.log(`Sandbox: ${sandboxDir}`);
    console.log(`View URL: ${view.url}`);
    console.log(`Created spec: ${CREATED_CODE}`);
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

function validPayload() {
  return {
    epic_code: EPIC_CODE,
    title: NEW_SPEC_TITLE,
    priority: "MEDIUM",
    points: 3,
    body: NEW_SPEC_BODY,
  };
}

function parseArgs(argv) {
  return parseCommonArgs(argv, defaultWorkspaceRoot, printHelp);
}

function printHelp() {
  console.log(`Smoke test for spec creation from archetipo view

Usage:
  node ./test/e2e/create-spec-view-smoke.mjs
  npm run test:view-create-smoke

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
  const seedBody = [
    "**User Story**",
    "Come utente voglio una storia di semina per lo smoke di creazione.",
    "",
    "**Dimostrazione**",
    "Il revisore vede la storia nel backlog del sandbox.",
    "",
    "**Criteri di accettazione**",
    "- [ ] La storia esiste nel backlog",
    "",
  ].join("\n");
  const payload = {
    specs: [
      {
        code: "US-901",
        title: "Smoke create seed spec",
        epic: { code: EPIC_CODE, title: "Smoke create" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: seedBody,
      },
      {
        code: "US-902",
        title: "Smoke create second seed spec",
        epic: { code: EPIC_CODE, title: "Smoke create" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: seedBody,
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

async function startViewServer(cwd) {
  return startViewServerShared(cliPath, cwd, cliEnv, "/api/board");
}



// postExpectingStatus posts a JSON payload and asserts the HTTP status without
// throwing on non-2xx: the invalid-input case is an expected outcome here, not
// a transport failure.
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

function collectCodes(board) {
  return new Set((board.columns || []).flatMap((column) => (column.specs || []).map((spec) => spec.code)));
}

function countCards(board) {
  return (board.columns || []).reduce((total, column) => total + (column.specs || []).length, 0);
}

function countOccurrences(board, code) {
  return (board.columns || []).reduce(
    (total, column) => total + (column.specs || []).filter((spec) => spec.code === code).length,
    0,
  );
}

function countMatches(text, needle) {
  return text.split(needle).length - 1;
}

function assertBoardHas(board, code) {
  const codes = collectCodes(board);
  if (!codes.has(code)) {
    throw new Error(`Expected board to contain ${code}; got [${[...codes].join(", ")}]`);
  }
}

function assertColumnHas(board, columnID, code) {
  const column = (board.columns || []).find((c) => c.id === columnID);
  if (!column) {
    throw new Error(`Expected board to expose a '${columnID}' column`);
  }
  const codes = (column.specs || []).map((spec) => spec.code);
  if (!codes.includes(code)) {
    throw new Error(`Expected column ${columnID} to contain ${code}; got [${codes.join(", ")}]`);
  }
}

function assertEpicKnown(board, code) {
  const codes = (board.epics || []).map((epic) => epic.code);
  if (!codes.includes(code)) {
    throw new Error(`Expected the board to expose epic ${code}; got [${codes.join(", ")}]`);
  }
}

function assertSameCodes(actual, expected, label) {
  const a = [...actual].sort();
  const b = [...expected].sort();
  if (a.length !== b.length || a.some((code, i) => code !== b[i])) {
    throw new Error(`Unexpected ${label}: expected [${b.join(", ")}], got [${a.join(", ")}]`);
  }
}

function assertEqual(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`Unexpected ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

async function readBacklogFile(sandboxDir) {
  return fs.readFile(path.join(sandboxDir, ".archetipo", "backlog.yaml"), "utf8");
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
