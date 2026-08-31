import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";

export function parseCommonArgs(argv, defaultWorkspaceRoot, printHelp) {
  const options = { workspaceRoot: defaultWorkspaceRoot, cleanup: false };
  for (let index = 0; index < argv.length; index += 1) {
    switch (argv[index]) {
      case "--workspace-root":
        options.workspaceRoot = path.resolve(argv[++index]);
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
        throw new Error(`Unknown argument: ${argv[index]}`);
    }
  }
  return options;
}

export async function createRunDir(root, insideRuns = false) {
  const parent = insideRuns ? path.join(root, "runs") : root;
  await fs.mkdir(parent, { recursive: true });
  const runDir = path.join(parent, new Date().toISOString().replace(/[:.]/g, "-"));
  await fs.mkdir(runDir, { recursive: true });
  return runDir;
}

export function makeRunCommand(defaultEnv) {
  return async function runCommand(label, command, args, options = {}) {
    console.log(`-> ${label}: ${command} ${args.join(" ")}`);
    const result = await new Promise((resolve) => {
      const child = spawn(command, args, {
        cwd: options.cwd,
        env: options.env || defaultEnv,
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
    if (result.code !== 0 && !options.allowFailure) {
      throw new Error(`${label} failed with exit ${result.code}\nSTDOUT:\n${result.stdout}\nSTDERR:\n${result.stderr}`);
    }
    return result;
  };
}

export async function buildCLI(cliPath, repoRoot, runCommand) {
  console.log(`-> building CLI: ${cliPath}`);
  await runCommand("go-build", "go", ["build", "-o", cliPath, "./cmd/archetipo"], {
    cwd: path.join(repoRoot, "cli"),
  });
}

export async function startViewServer(cliPath, cwd, env, readyPath) {
  const child = spawn(cliPath, ["view", "--host", "127.0.0.1", "--port", "0", "--no-open"], {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stdout = "";
  let stderr = "";
  child.stdout.on("data", (chunk) => { stdout += chunk.toString("utf8"); });
  const url = await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error(
      `view server did not become ready in time\nSTDERR:\n${stderr}\nSTDOUT:\n${stdout}`,
    )), 15000);
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
  await waitForHTTP(`${url}${readyPath}`);
  return { child, url };
}

export async function stopProcess(child, runCommand) {
  if (!child || child.killed) return;
  if (process.platform === "win32") {
    await runCommand("taskkill", "taskkill", ["/PID", String(child.pid), "/T", "/F"]);
    return;
  }
  child.kill("SIGTERM");
  await Promise.race([new Promise((resolve) => child.once("exit", resolve)), delay(3000)]);
  if (!child.killed) child.kill("SIGKILL");
}

export async function waitForHTTP(url) {
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

export async function apiJSON(url, init = {}) {
  const response = await fetch(url, {
    ...init,
    headers: { Accept: "application/json", ...(init.headers || {}) },
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

export function readBody(request) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    request.on("data", (chunk) => chunks.push(chunk));
    request.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    request.on("error", reject);
  });
}

export function escapeHTML(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
