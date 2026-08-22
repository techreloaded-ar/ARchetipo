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

import fs from "node:fs/promises";
import path from "node:path";
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

// Every report carries the pid of the process that made it, and not only the
// one about the invocation: a smoke that starts several agent processes in turn
// has to be able to say *which* of them was given a frame, and a report without
// its author would only let it say that somebody was.
async function report(kind, body) {
  try {
    await fetch(`${control}/received`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind, pid: process.pid, ...body }),
    });
  } catch {
    // The control server going away is the end of the test, not an error the
    // fake has to survive.
  }
}

// The invocation itself is reported, because how the session is opened is part
// of the protocol: the streaming flags are what make a live dialogue possible
// at all, and the smoke asserts them the way it asserts a frame. The working
// directory belongs to the same report and for the same reason: where the agent
// was started is part of how the session was opened, and it is the one fact no
// viewer field can stand in for.
//
// The process identifier travels with them, and for a third reason of the same
// kind: "the provider released the agent process" is a statement about the
// process, and the only thing that can settle it is the operating system being
// asked whether that process is still there. A route answering 200 says what
// the viewer decided, not what became of the process.
report("argv", { argv: process.argv.slice(2), cwd: process.cwd(), pid: process.pid });

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
      // `frames` is the same command for a burst: a history long enough to
      // overflow a retention window has to be produced in one go, because one
      // command per frame would turn the poll interval into a wait of minutes.
      if (Array.isArray(command.frames)) {
        for (const frame of command.frames) write(frame);
      } else {
        write(command.frame);
      }
    } else if (command && command.kind === "write") {
      // An artifact the agent produces, written *relative to its own working
      // directory*: it is what makes "the run acted on this workspace" a fact on
      // the filesystem instead of a claim about a configuration value.
      const target = path.resolve(process.cwd(), command.name);
      try {
        await fs.mkdir(path.dirname(target), { recursive: true });
        await fs.writeFile(target, command.text ?? "", "utf8");
        await report("wrote", { name: command.name, path: target });
      } catch (error) {
        await report("wrote", { name: command.name, error: String(error) });
      }
    } else if (command && command.kind === "exit") {
      process.exit(command.code ?? 0);
    }
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
}

pump();
