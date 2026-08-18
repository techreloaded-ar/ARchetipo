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
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "execution-config-view-smoke");
const TOKEN_SENTINEL = "smoke-secret-token-value";
const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot, ARCIPELAGO_TOKEN: TOKEN_SENTINEL };

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const specsFile = path.join(runDir, "specs.json");
  const configPath = path.join(sandboxDir, ".archetipo", "config.yaml");

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  let view;
  try {
    await buildCLI();
    await runCommand("init", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], { cwd: sandboxDir });
    await writeSpecsPayload(specsFile);
    await runCommand("spec-add", cliPath, ["spec", "add", "--file", specsFile], { cwd: sandboxDir });
    await runCommand("spec-plan", cliPath, ["spec", "plan", "US-902", "--file", await writePlanPayload(runDir)], { cwd: sandboxDir });

    view = await startViewServer(sandboxDir);
    console.log(`-> view ready: ${view.url}`);

    // AC-1 — registered providers are listed, and no secret is exposed.
    const providers = await apiJSON(`${view.url}/api/execution/providers`);
    const arcipelago = (providers.providers || []).find((p) => p.id === "arcipelago");
    if (!arcipelago) {
      throw new Error(`Expected the arcipelago provider to be listed; got ${JSON.stringify(providers.providers)}`);
    }
    if (!arcipelago.label || !(arcipelago.capabilities || []).length) {
      throw new Error(`Provider is not presentable: ${JSON.stringify(arcipelago)}`);
    }
    const fieldNames = (arcipelago.config_fields || []).map((f) => f.name).sort();
    const wantFields = ["base_url", "poll_interval_seconds", "timeout_seconds", "token_env", "workspace_id"];
    if (fieldNames.join(",") !== wantFields.join(",")) {
      throw new Error(`Unexpected configurable fields: [${fieldNames.join(", ")}]`);
    }
    const codex = (providers.providers || []).find((p) => p.id === "codex");
    if (!codex) {
      throw new Error(`Expected the codex provider to be listed; got ${JSON.stringify(providers.providers)}`);
    }
    if (!codex.label || !(codex.capabilities || []).includes("spec.plan")) {
      throw new Error(`The codex provider does not declare the spec.plan capability: ${JSON.stringify(codex)}`);
    }
    const codexFields = (codex.config_fields || []).map((f) => f.name).sort();
    const wantCodexFields = ["command", "exec_args", "model", "timeout_seconds"];
    if (codexFields.join(",") !== wantCodexFields.join(",")) {
      throw new Error(`Unexpected configurable fields for codex: [${codexFields.join(", ")}]`);
    }
    assertNoCredentialFields(codex);
    // Availability is observed, not required: the smoke must pass on a machine
    // with Codex installed and on one without it. What must always hold is that
    // the field exists and that an unavailable provider says why.
    if (typeof codex.available !== "boolean") {
      throw new Error(`The codex provider must report a boolean 'available'; got ${JSON.stringify(codex.available)}`);
    }
    if (codex.available === false && !(codex.unavailable_reason || "").trim()) {
      throw new Error(`An unavailable codex provider must state a reason; got ${JSON.stringify(codex)}`);
    }
    if (typeof arcipelago.available !== "boolean") {
      throw new Error(`The arcipelago provider must report a boolean 'available'; got ${JSON.stringify(arcipelago.available)}`);
    }
    if (JSON.stringify(providers).includes(TOKEN_SENTINEL)) {
      throw new Error("The provider list leaked the credential held in the environment");
    }
    if (providers.default !== null && providers.default !== undefined) {
      throw new Error(`A fresh workspace must report no default provider; got ${JSON.stringify(providers.default)}`);
    }
    console.log("-> AC-1 ok: providers listed with availability and without secrets");

    // AC-2 — a valid configuration is saved and survives a reload.
    await apiJSON(`${view.url}/api/execution/provider/default`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: "arcipelago", config: { base_url: "https://hub.test", workspace_id: "ws-smoke" } }),
    });
    const savedConfig = await fs.readFile(configPath, "utf8");
    if (!savedConfig.includes("default_provider") || !savedConfig.includes("arcipelago")) {
      throw new Error(`The selection did not reach the config file:\n${savedConfig}`);
    }
    const reloaded = await apiJSON(`${view.url}/api/execution/providers`);
    if (!reloaded.default || reloaded.default.id !== "arcipelago" || reloaded.default.config.workspace_id !== "ws-smoke") {
      throw new Error(`The default was not reported after reload: ${JSON.stringify(reloaded.default)}`);
    }
    console.log("-> AC-2 ok: default provider persisted and reported after reload");

    // AC-3 — an invalid configuration names the offending field and changes nothing.
    const rejection = await expectStatus(`${view.url}/api/execution/provider/default`, 400, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id: "arcipelago", config: { base_url: "non-un-url", workspace_id: "ws-smoke" } }),
    });
    if (rejection.field !== "base_url") {
      throw new Error(`Expected the rejection to name base_url; got ${JSON.stringify(rejection)}`);
    }
    const afterRejection = await fs.readFile(configPath, "utf8");
    if (afterRejection !== savedConfig) {
      throw new Error("A rejected configuration rewrote the config file");
    }
    const stillDefault = await apiJSON(`${view.url}/api/execution/providers`);
    if (!stillDefault.default || stillDefault.default.config.base_url !== "https://hub.test") {
      throw new Error(`The previously valid default was lost: ${JSON.stringify(stillDefault.default)}`);
    }
    console.log("-> AC-3 ok: invalid configuration rejected by field, previous default intact");

    // AC-4 — the spec detail exposes only the actions its status admits.
    const todo = await apiJSON(`${view.url}/api/spec/US-901`);
    assertActions(todo, [["plan", "Pianifica"]], "US-901 (TODO)");
    const planned = await apiJSON(`${view.url}/api/spec/US-902`);
    assertActions(planned, [["implement", "Implementa"]], "US-902 (PLANNED)");
    console.log("-> AC-4 ok: admitted actions exposed with id and label");

    // AC-5 — after a status change the list is recomputed, empty when nothing is admitted.
    await apiJSON(`${view.url}/api/board/move`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code: "US-901", to: "done" }),
    });
    const done = await apiJSON(`${view.url}/api/spec/US-901`);
    if (done.spec.status !== "DONE") {
      throw new Error(`Expected US-901 to be DONE; got ${done.spec.status}`);
    }
    if (!Array.isArray(done.actions) || done.actions.length !== 0) {
      throw new Error(`A DONE spec admits no action; got ${JSON.stringify(done.actions)}`);
    }
    console.log("-> AC-5 ok: actions recomputed as an empty list after the status change");

    console.log("\nPASS: execution-config view smoke test completed.");
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

// A local provider never needs a credential, so a configurable field whose very
// name asks for one is a design regression the smoke must catch before it ships.
const CREDENTIAL_FIELD_HINTS = ["token", "secret", "password", "passwd", "credential", "api_key", "apikey", "auth"];

function assertNoCredentialFields(provider) {
  for (const field of provider.config_fields || []) {
    const name = String(field.name || "").toLowerCase();
    const hint = CREDENTIAL_FIELD_HINTS.find((h) => name.includes(h));
    if (hint) {
      throw new Error(`Provider ${provider.id} exposes a credential-shaped configurable field: ${field.name}`);
    }
  }
}

function assertActions(detail, expected, label) {
  const got = (detail.actions || []).map((a) => [a.id, a.label]);
  if (JSON.stringify(got) !== JSON.stringify(expected)) {
    throw new Error(`Unexpected actions for ${label}: ${JSON.stringify(got)}, want ${JSON.stringify(expected)}`);
  }
  if (!detail.template || !detail.template.id) {
    throw new Error(`The spec detail of ${label} does not name the process Template`);
  }
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
  console.log(`Smoke test for execution provider configuration and spec actions in archetipo view

Usage:
  node ./test/e2e/execution-config-view-smoke.mjs
  npm run test:view-execution-smoke

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
  const payload = {
    specs: [
      {
        code: "US-901",
        title: "Smoke spec da pianificare",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per le azioni ammesse in TODO.",
      },
      {
        code: "US-902",
        title: "Smoke spec pianificata",
        epic: { code: "EP-999", title: "Smoke tests" },
        priority: "LOW",
        points: 1,
        status: "TODO",
        body: "Story di test per le azioni ammesse in PLANNED.",
      },
    ],
  };
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
}

// A real `spec plan` moves US-902 to PLANNED, so the second status is produced
// by the process itself rather than by editing the backlog by hand.
async function writePlanPayload(runDir) {
  const file = path.join(runDir, "plan.json");
  const payload = {
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
  await fs.writeFile(file, JSON.stringify(payload, null, 2));
  return file;
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

async function expectStatus(url, status, init = {}) {
  const response = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init.headers || {}),
    },
  });
  const text = await response.text();
  if (response.status !== status) {
    throw new Error(`Expected HTTP ${status} for ${url}, got ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
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
