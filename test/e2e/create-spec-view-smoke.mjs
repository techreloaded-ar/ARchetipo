#!/usr/bin/env node

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
  await fs.mkdir(root, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const runDir = path.join(root, stamp);
  await fs.mkdir(runDir, { recursive: true });
  return runDir;
}

async function buildCLI() {
  console.log(`-> building CLI: ${cliPath}`);
  await runCommand("go-build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], {
    cwd: path.join(repoRoot, "cli"),
  });
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
  const child = spawn(cliPath, ["view", "--host", "127.0.0.1", "--port", "0", "--no-open"], {
    cwd,
    env: cliEnv,
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
  console.log(`-> ${label}: ${command} ${args.join(" ")}`);
  const result = await new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env || cliEnv,
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
