import assert from "node:assert/strict";
import test from "node:test";
import { parsePiTrace, scoreResult } from "./run.mjs";

test("parsePiTrace deduplicates streamed tool calls and normalizes repository paths", () => {
  const sandbox = "/tmp/rideatlas";
  const events = [
    { type: "message_update", assistantMessageEvent: { type: "toolcall_start" }, message: { content: [{ type: "toolCall", id: "call-1", name: "read", arguments: {} }] } },
    { type: "message_update", assistantMessageEvent: { type: "toolcall_end" }, message: { content: [{ type: "toolCall", id: "call-1", name: "read", arguments: { path: `${sandbox}/src/a.ts` } }] } },
    { type: "message_update", assistantMessageEvent: { type: "toolcall_end" }, message: { content: [{ type: "toolCall", id: "call-2", name: "grep", arguments: { path: "src", pattern: "needle" } }] } },
    { type: "message_end", message: { role: "assistant", responseId: "r1", usage: { input: 12, output: 3, cacheRead: 20, totalTokens: 35, cost: { total: 0.01 } }, content: [{ type: "text", text: "result" }] } },
  ];
  const trace = parsePiTrace(events.map((event) => JSON.stringify(event)).join("\n"), sandbox);

  assert.equal(trace.telemetry.tool_calls, 2);
  assert.deepEqual(trace.telemetry.files_read, ["src/a.ts"]);
  assert.deepEqual(trace.telemetry.inspected_paths, ["src", "src/a.ts"]);
  assert.equal(trace.usage.total_tokens, 35);
  assert.equal(trace.finalText, "result");
});

test("scoreResult accepts a JSON object wrapped in a code fence", () => {
  const output = '```json\n{"summary":"ok","files":["src/a.ts"],"findings":["Bozza diventa Pubblicato"],"risks":[],"verification":[]}\n```';
  const score = scoreResult(output, {
    expected_files: ["src/a.ts"],
    required_findings: [{ id: "transition", any: ["Bozza.{0,30}Pubblicato"] }],
  });

  assert.equal(score.total, 100);
  assert.deepEqual(score.matched_files, ["src/a.ts"]);
  assert.deepEqual(score.matched_findings, ["transition"]);
});

test("scoreResult preserves content scoring when the outer JSON is truncated", () => {
  const output = '{"summary":"ok","files":["src/a.ts"],"findings":["Bozza diventa Pubblicato"]';
  const score = scoreResult(output, {
    expected_files: ["src/a.ts"],
    required_findings: [{ id: "transition", any: ["Bozza.{0,30}Pubblicato"] }],
  });

  assert.equal(score.total, 90);
  assert.equal(score.format_score, 0);
  assert.deepEqual(score.matched_files, ["src/a.ts"]);
});
