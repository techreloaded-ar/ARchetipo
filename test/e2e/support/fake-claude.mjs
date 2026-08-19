#!/usr/bin/env node

// A Claude Code CLI that never thinks.
//
// It answers `--version` like the real binary, and under the streaming flags
// the provider passes (`--print --input-format stream-json --output-format
// stream-json --verbose --replay-user-messages --no-session-persistence
// --permission-mode <mode>` plus an optional `--model`) it speaks the same
// NDJSON protocol on stdin and stdout, one frame per line. The shapes are the
// ones observed on Claude Code 2.1.235: `system`/`init`, `assistant`, `user`,
// `result` on the way out, an operator `user` frame and a `control_request` of
// subtype `interrupt` on the way in, and a `control_response` correlated by
// `request_id` as the answer to that request.
//
// It never progresses on its own. Everything it emits is commanded by the smoke
// through a control server on 127.0.0.1, so every assertion is made against a
// state the test produced rather than against a timer it waited out. Every line
// it reads is reported to that server too, so the smoke can assert what the
// process was really told and not only what it answered.

import process from "node:process";
import readline from "node:readline";

const control = process.env.FAKE_CLAUDE_CONTROL;

if (process.argv.includes("--version")) {
  process.stdout.write("2.1.235-fake (Claude Code)\n");
  process.exit(0);
}

if (!control) {
  process.stderr.write("FAKE_CLAUDE_CONTROL is not set\n");
  process.exit(2);
}

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

// The invocation itself is reported, because how the session is opened is part
// of the protocol: the streaming flags are what make a live dialogue possible
// at all, and the smoke asserts them the way it asserts a frame.
report("argv", { argv: process.argv.slice(2) });

const rl = readline.createInterface({ input: process.stdin });

rl.on("line", async (line) => {
  if (!line.trim()) return;
  let frame;
  try {
    frame = JSON.parse(line);
  } catch {
    return;
  }
  // Every frame is reported as it arrived. Naming it here would mean deciding,
  // inside the fake, what the test is allowed to look at.
  await report("received", { frame });
  if (frame.type === "control_request" && frame.request?.subtype === "interrupt") {
    // The command is acknowledged, and nothing else: what then happens to the
    // turn is the smoke's decision, expressed as an `emit`, exactly as with the
    // real binary, which keeps working until it really stops.
    write({
      type: "control_response",
      response: { subtype: "success", request_id: frame.request_id },
    });
  }
});

// The process ends when its input does, exactly like the real CLI.
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
      write(command.frame);
    } else if (command && command.kind === "exit") {
      process.exit(command.code ?? 0);
    }
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
}

pump();
