#!/usr/bin/env node

// A Codex CLI that never thinks.
//
// It answers `--version` like the real binary, and under `app-server --listen
// stdio://` it speaks the same JSON-RPC protocol on stdin and stdout:
// `initialize`, `model/list`, `thread/start`, `turn/start`, `turn/steer`,
// `turn/interrupt`.
// The shapes are the ones observed on codex-cli 0.147.0, including the refusal
// `-32600 no active turn to steer` once the turn is over.
//
// It never progresses on its own. Everything it emits is commanded by the smoke
// through a control server on 127.0.0.1, so every assertion is made against a
// state the test produced rather than against a timer it waited out.

import process from "node:process";
import readline from "node:readline";

let turnCount = 0;

const control = process.env.FAKE_CODEX_CONTROL;

if (process.argv.includes("--version")) {
  process.stdout.write("codex-cli 0.147.0-fake\n");
  process.exit(0);
}

if (!control) {
  process.stderr.write("FAKE_CODEX_CONTROL is not set\n");
  process.exit(2);
}

let turnActive = false;

function write(payload) {
  process.stdout.write(JSON.stringify(payload) + "\n");
}

async function report(kind, body) {
  try {
    await fetch(`${control}/received`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind, ...body }),
    });
  } catch {
    // The control server going away is the end of the test, not an error the
    // fake has to survive.
  }
}

function noActiveTurn(id, what) {
  write({ id, error: { code: -32600, message: `no active turn to ${what}` } });
}

// Reported once at startup, exactly like fake-claude.mjs does and for the
// same reason: where the process was started and its pid are facts no viewer
// field can stand in for — "the provider released the agent process" is a
// statement about the process, settled only by asking the operating system
// whether it is still there.
await report("argv", { argv: process.argv.slice(2), cwd: process.cwd(), pid: process.pid });

const rl = readline.createInterface({ input: process.stdin });

rl.on("line", async (line) => {
  if (!line.trim()) return;
  let message;
  try {
    message = JSON.parse(line);
  } catch {
    return;
  }
  switch (message.method) {
    case "initialize":
      write({ id: message.id, result: { userAgent: "fake-codex" } });
      break;
    case "initialized":
      break;
    case "model/list":
      write({ id: message.id, result: { data: [{ id: "gpt-fake", model: "gpt-fake", displayName: "GPT Fake", isDefault: true, defaultReasoningEffort: "medium", supportedReasoningEfforts: [{ reasoningEffort: "low" }, { reasoningEffort: "medium" }, { reasoningEffort: "high" }] }] } });
      break;
    case "skills/list": {
      const cwd = message.params?.cwds?.[0] || process.cwd();
      write({ id: message.id, result: { data: [{ cwd, skills: [
        { name: "codex-only", description: "Solo Codex", path: `${cwd}/.agents/skills/codex-only/SKILL.md`, enabled: true, scope: "repo" },
        { name: "shared", description: "Condivisa", path: `${cwd}/.agents/skills/shared/SKILL.md`, enabled: true, scope: "repo" },
        { name: "disabled", description: "Disabilitata", path: `${cwd}/.agents/skills/disabled/SKILL.md`, enabled: false, scope: "repo" },
        { name: "plugin:fixture", description: "Plugin", path: `${cwd}/plugins/fixture/SKILL.md`, enabled: true, scope: "user" },
      ] }] } });
      break;
    }
    case "thread/start":
      await report("thread/start", { params: message.params });
      write({ id: message.id, result: { thread: { id: "thread-1" } } });
      break;
    case "thread/resume":
      await report("thread/resume", { params: message.params });
      write({ id: message.id, result: { thread: { id: message.params?.threadId || "thread-1", ephemeral: false }, model: "gpt-fake", reasoningEffort: "medium" } });
      break;
    case "turn/start":
      turnActive = true;
      turnCount += 1;
      await report("turn/start", { params: message.params });
      write({ id: message.id, result: { turn: { id: `turn-${turnCount}` } } });
      write({ method: "turn/started", params: { turn: { id: `turn-${turnCount}` } } });
      break;
    case "turn/steer": {
      const text = (message.params?.input || []).map((entry) => entry.text).join("");
      await report("turn/steer", { text, params: message.params });
      if (!turnActive) {
        noActiveTurn(message.id, "steer");
        break;
      }
      write({ id: message.id, result: {} });
      break;
    }
    case "turn/interrupt":
      await report("turn/interrupt", { params: message.params });
      if (!turnActive) {
        noActiveTurn(message.id, "interrupt");
        break;
      }
      // Delivered, and nothing else: the real runner decides when the turn
      // really ends, and this fake waits to be told.
      write({ id: message.id, result: {} });
      break;
    default:
      if (message.id !== undefined && message.method === undefined) {
        await report("response", { id: message.id, result: message.result, error: message.error });
        break;
      }
      if (message.id !== undefined) {
        write({ id: message.id, error: { code: -32601, message: "unknown method" } });
      }
  }
});

// The process ends when its input does, exactly like the real app server.
rl.on("close", () => process.exit(0));

// Commands come from the smoke, one at a time.
async function pump() {
  for (;;) {
    let command = null;
    try {
      const response = await fetch(`${control}/next`);
      command = await response.json();
    } catch {
      return;
    }
    if (command && command.kind === "emit") {
      if (command.method === "turn/completed") {
        turnActive = false;
      }
      write({ method: command.method, params: command.params });
    } else if (command && command.kind === "request") {
      write({ id: command.id, method: command.method, params: command.params });
    } else if (command && command.kind === "exit") {
      process.exit(command.code ?? 0);
    }
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
}

pump();
