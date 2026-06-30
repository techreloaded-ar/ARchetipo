#!/usr/bin/env node

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const binDir = path.join(repoRoot, "test", "e2e", ".bin");
const binName = process.platform === "win32" ? "archetipo.exe" : "archetipo";
const cliPath = path.join(binDir, binName);
const defaultWorkspaceRoot = path.join(repoRoot, "test", "workspaces", "validate-inception-smoke");
const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

// PRD with 8 of 9 required markers (vision is intentionally omitted)
// and two unresolved {{PLACEHOLDER}} tokens.
const invalidPRD = `<!-- archetipo:prd section=elevator_pitch required=true -->
A concise elevator pitch with {{UNRESOLVED}} still here.

<!-- archetipo:prd section=user_personas required=true -->
Detailed personas describing target users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights gathered during brainstorming sessions.

<!-- archetipo:prd section=product_scope required=true -->
MVP scope and out-of-scope items.

<!-- archetipo:prd section=technical_architecture required=true -->
Stack: {{TECH_STACK}}.

<!-- archetipo:prd section=functional_requirements required=true -->
List of functional requirements with IDs.

<!-- archetipo:prd section=non_functional_requirements required=true -->
Performance and reliability requirements.

<!-- archetipo:prd section=next_steps required=true -->
Concrete next steps.
`;

// Valid PRD with all 9 required markers and no placeholders.
const validPRD = `<!-- archetipo:prd section=elevator_pitch required=true -->
A concise elevator pitch summarizing the product.

<!-- archetipo:prd section=vision required=true -->
The long-term vision for the product.

<!-- archetipo:prd section=user_personas required=true -->
Detailed personas describing target users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights gathered during brainstorming sessions.

<!-- archetipo:prd section=product_scope required=true -->
MVP scope and out-of-scope items.

<!-- archetipo:prd section=technical_architecture required=true -->
The chosen tech stack and architecture decisions.

<!-- archetipo:prd section=functional_requirements required=true -->
List of functional requirements with IDs.

<!-- archetipo:prd section=non_functional_requirements required=true -->
Performance, security, and reliability requirements.

<!-- archetipo:prd section=next_steps required=true -->
Concrete next steps and owners.
`;

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const runDir = await createRunDir(options.workspaceRoot);
  const sandboxDir = path.join(runDir, "sandbox");
  const reportPath = path.join(runDir, "report.html");

  console.log(`-> workspace: ${runDir}`);
  await fs.mkdir(sandboxDir, { recursive: true });
  await fs.mkdir(binDir, { recursive: true });

  const startedAt = Date.now();
  const events = [];

  let passed = false;
  let errorMessage = "";
  let findings = [];

  try {
    // 1. Build the CLI binary.
    await buildCLI();

    // 2. Initialize the sandbox project.
    await runAndRecord("init", cliPath, ["init", "--tool", "pi", "--connector", "file", "--yes"], { cwd: sandboxDir }, events);

    // 3. Write invalid PRD input files.
    const invalidPath = path.join(runDir, "invalid-prd.md");
    const validPath = path.join(runDir, "valid-prd.md");
    await fs.writeFile(invalidPath, invalidPRD);
    await fs.writeFile(validPath, validPRD);

    // STEP 1 — persist invalid PRD.
    console.log("\n═══ STEP 1/4: persist invalid PRD ═══");
    const write1 = await runAndRecord("prd-write-invalid", cliPath, ["prd", "write", "--file", invalidPath], { cwd: sandboxDir }, events);
    assertExit(write1, 0, "ExitOK");
    assertKind(write1, "write_result");
    assertDataOk(write1);

    // STEP 2 — validate → E_VALIDATION.
    console.log("═══ STEP 2/4: validate prd (expects E_VALIDATION) ═══");
    const val1 = await runAndRecord("validate-invalid", cliPath, ["validate", "prd"], { cwd: sandboxDir }, events);
    assertExit(val1, 2, "ExitInvalidInput");
    assertErrorCode(val1, "E_VALIDATION");
    findings = assertErrorDetails(val1);

    console.log(`\n   Validation findings (${findings.length}):`);
    for (const f of findings) {
      console.log(`   ❌ [${f.code}] ${f.message}`);
      console.log(`      path: ${f.path}`);
      console.log(`      hint: ${f.hint}`);
    }

    // STEP 3 — persist valid PRD.
    console.log("\n═══ STEP 3/4: persist valid PRD ═══");
    const write2 = await runAndRecord("prd-write-valid", cliPath, ["prd", "write", "--file", validPath], { cwd: sandboxDir }, events);
    assertExit(write2, 0, "ExitOK");
    assertKind(write2, "write_result");
    assertDataOk(write2);

    // STEP 4 — validate → OK.
    console.log("═══ STEP 4/4: validate prd (expects success) ═══");
    const val2 = await runAndRecord("validate-valid", cliPath, ["validate", "prd"], { cwd: sandboxDir }, events);
    assertExit(val2, 0, "ExitOK");
    const envOk = JSON.parse(val2.stdout);
    if (envOk.kind !== "validation_result") {
      throw new Error(`Expected kind=validation_result, got ${envOk.kind}`);
    }
    if (envOk.data?.ok !== true) {
      throw new Error(`Expected data.ok=true, got ${JSON.stringify(envOk.data)}`);
    }

    passed = true;
  } catch (error) {
    errorMessage = error.message;
  } finally {
    // Always write the HTML report.
    const endedAt = Date.now();
    const durationMs = endedAt - startedAt;
    const html = renderReport({ sandboxDir, events, startedAt, endedAt, durationMs, findings, passed, errorMessage });
    await fs.writeFile(reportPath, html);

    if (passed) {
      console.log(`\n╔══════════════════════════════════════════╗`);
      console.log(`║  PASS: validate-inception smoke test     ║`);
      console.log(`╠══════════════════════════════════════════╣`);
      console.log(`║  Report:  ${reportPath}`);
      console.log(`╚══════════════════════════════════════════╝`);
    } else {
      console.error(`\nFAIL: ${errorMessage}`);
      console.error(`Report written anyway: ${reportPath}`);
    }

    if (options.cleanup) {
      await fs.rm(runDir, { recursive: true, force: true });
      console.log(`-> cleaned workspace: ${runDir}`);
    }
  }

  if (!passed) {
    throw new Error(errorMessage);
  }
}

// ---------------------------------------------------------------------------
// CLI runner
// ---------------------------------------------------------------------------

async function runAndRecord(step, command, args, options, events) {
  console.log(`   $ ${command} ${args.join(" ")}`);
  const startedAt = Date.now();
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
  const endedAt = Date.now();

  let stdoutEnv = null;
  let stderrEnv = null;
  try { stdoutEnv = JSON.parse(result.stdout); } catch { /* raw text */ }
  try { stderrEnv = JSON.parse(result.stderr); } catch { /* raw text */ }

  events.push({
    step,
    command,
    args,
    startedAt,
    endedAt,
    durationMs: endedAt - startedAt,
    exit: result.code,
    stdout: result.stdout,
    stderr: result.stderr,
    stdoutEnv,
    stderrEnv,
  });
  return result;
}

async function buildCLI() {
  console.log(`-> building CLI: ${cliPath}`);
  const result = await new Promise((resolve) => {
    const child = spawn("go", ["build", "-o", cliPath, "./cmd/archetipo"], {
      cwd: path.join(repoRoot, "cli"),
      env: cliEnv,
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
    throw new Error(`go build failed: ${result.stderr || result.stdout}`);
  }
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

function assertExit(result, expected, label) {
  if (result.code !== expected) {
    throw new Error(
      `Expected exit ${expected} (${label}), got ${result.code}\n` +
      `STDOUT: ${result.stdout}\nSTDERR: ${result.stderr}`,
    );
  }
}

function assertKind(result, expected) {
  const env = JSON.parse(result.stdout);
  if (env.kind !== expected) {
    throw new Error(`Expected kind=${expected}, got ${env.kind}\n${result.stdout}`);
  }
}

function assertDataOk(result) {
  const env = JSON.parse(result.stdout);
  if (env.data?.ok !== true) {
    throw new Error(`Expected data.ok=true, got ${JSON.stringify(env.data)}`);
  }
}

function assertErrorCode(result, expected) {
  let env;
  try { env = JSON.parse(result.stderr); } catch {
    throw new Error(`Failed to parse stderr as JSON\nSTDERR: ${result.stderr}`);
  }
  if (env.error?.code !== expected) {
    throw new Error(`Expected error.code=${expected}, got ${env.error?.code}\nSTDERR: ${result.stderr}`);
  }
}

function assertErrorDetails(result) {
  const env = JSON.parse(result.stderr);
  const details = env.error?.details;
  if (!details || typeof details !== "object") {
    throw new Error(`Expected error.details to be populated\nSTDERR: ${result.stderr}`);
  }
  const findings = details.findings;
  if (!Array.isArray(findings) || findings.length === 0) {
    throw new Error(`Expected error.details.findings to be a non-empty array\nSTDERR: ${result.stderr}`);
  }
  const codes = findings.map((f) => f.code).filter(Boolean);
  if (!codes.includes("PRD_PLACEHOLDER_LEFT")) {
    throw new Error(`Expected PRD_PLACEHOLDER_LEFT, got [${codes.join(", ")}]\nSTDERR: ${result.stderr}`);
  }
  if (!codes.includes("PRD_MISSING_SECTION")) {
    throw new Error(`Expected PRD_MISSING_SECTION, got [${codes.join(", ")}]\nSTDERR: ${result.stderr}`);
  }
  return findings;
}

// ---------------------------------------------------------------------------
// HTML report (matches run.mjs style)
// ---------------------------------------------------------------------------

function renderReport({ sandboxDir, events, startedAt, endedAt, durationMs, findings, passed, errorMessage }) {
  const startedISO = new Date(startedAt).toISOString();
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ARchetipo Smoke — Validate PRD Correction Loop</title>
  <style>
    :root { color-scheme: light; --bg: #f6f7f9; --panel: #ffffff; --ink: #172026; --muted: #61707d; --line: #d8dee6; --ok: #18794e; --fail: #c93a2f; --warn: #8a5a00; }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--bg); color: var(--ink); font: 14px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { max-width: 1100px; margin: 0 auto; padding: 28px; }
    header { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; margin-bottom: 24px; }
    h1 { margin: 0 0 12px; font-size: 22px; }
    h2 { margin: 24px 0 12px; font-size: 18px; border-bottom: 1px solid var(--line); padding-bottom: 8px; }
    .meta { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px; }
    .meta div { border: 1px solid var(--line); border-radius: 6px; padding: 8px 10px; background: #fbfcfd; }
    .label { display: block; color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
    .value { overflow-wrap: anywhere; }
    .badge { display: inline-flex; align-items: center; gap: 6px; border-radius: 999px; padding: 3px 9px; font-size: 12px; font-weight: 650; border: 1px solid var(--line); background: #fff; color: var(--muted); }
    .badge.pass { border-color: #b7dec9; background: #eefaf3; color: var(--ok); }
    .badge.fail { border-color: #f0bbb6; background: #fff0ef; color: var(--fail); }
    .badge.cli   { border-color: #c4deb9; background: #f0faeb; color: #386a20; }

    /* Timeline */
    .timeline { display: grid; gap: 14px; }
    .event { border: 1px solid var(--line); border-left-width: 5px; border-radius: 8px; background: var(--panel); padding: 16px; }
    .event.cli { border-left-color: #386a20; }
    .event-head { display: flex; gap: 10px; align-items: flex-start; justify-content: space-between; flex-wrap: wrap; }
    .event-title { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
    .step { font-size: 16px; font-weight: 750; }
    .time { color: var(--muted); font-variant-numeric: tabular-nums; }
    .command-line, pre, code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace; }
    .command-line { margin-top: 10px; padding: 9px 10px; border-radius: 6px; background: #f2f4f7; overflow-x: auto; white-space: pre-wrap; overflow-wrap: anywhere; }
    details { margin-top: 10px; border: 1px solid var(--line); border-radius: 6px; background: #fbfcfd; }
    summary { cursor: pointer; padding: 8px 10px; color: var(--muted); font-weight: 650; }
    pre { margin: 0; padding: 10px; overflow-x: auto; white-space: pre-wrap; overflow-wrap: anywhere; max-height: 520px; font-size: 13px; }

    /* Findings table */
    .findings-table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 13px; }
    .findings-table th { text-align: left; padding: 8px 10px; border-bottom: 2px solid var(--line); color: var(--muted); font-weight: 650; background: #fbfcfd; }
    .findings-table td { padding: 8px 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
    .findings-table .code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 650; color: var(--fail); }
    .findings-table .hint { color: var(--muted); font-size: 12px; }

    /* Checks table (success) */
    .checks-table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 13px; }
    .checks-table th { text-align: left; padding: 8px 10px; border-bottom: 2px solid var(--line); color: var(--muted); font-weight: 650; background: #fbfcfd; }
    .checks-table td { padding: 8px 10px; border-bottom: 1px solid var(--line); }
    .checks-table .check-code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 650; }
    .checks-table .status-pass { color: var(--ok); }
    .checks-table .status-fail { color: var(--fail); }

    .section-desc { color: var(--muted); margin-bottom: 16px; }
    @media (max-width: 720px) { main { padding: 16px; } }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>ARchetipo Smoke — Validate PRD Correction Loop</h1>
      <div class="meta">
        ${metaItem("Status", passed ? "PASS" : "FAIL")}
        ${metaItem("Started", startedISO)}
        ${metaItem("Duration", formatDuration(durationMs))}
        ${metaItem("Sandbox", sandboxDir)}
      </div>
    </header>

    <h2>Scenario</h2>
    <p class="section-desc">
      This smoke test exercises the <code>archetipo validate prd</code> correction loop
      without an AI agent. It writes a deliberately broken PRD (missing <code>vision</code> marker,
      <code>{{UNRESOLVED}}</code> + <code>{{TECH_STACK}}</code> placeholders), validates,
      inspects the <code>E_VALIDATION</code> findings, corrects the PRD, and re-validates to
      confirm the loop closes.
    </p>

    ${!passed ? `<h2>Error</h2>
    <p class="section-desc" style="color:var(--fail);font-weight:650;">${esc(errorMessage)}</p>` : ""}

    <h2>Timeline</h2>
    <section class="timeline">
      ${events.map((e, i) => renderEvent(e, i, startedAt)).join("")}
    </section>

    <h2>Validation Findings (invalid PRD)</h2>
    ${findings ? renderFindingsTable(findings) : `<p class="section-desc">No findings — test did not reach the validation step.</p>`}

    <h2>Raw Envelopes</h2>
    ${events.filter(e => e.stdoutEnv || e.stderrEnv).map((e) => renderEnvelopeDetails(e)).join("")}
  </main>
</body>
</html>`;
}

function metaItem(label, value) {
  return `<div><span class="label">${esc(label)}</span><span class="value">${esc(String(value ?? ""))}</span></div>`;
}

function renderEvent(event, index, runStartedAt) {
  const offset = formatDuration(event.startedAt - runStartedAt);
  const dur = formatDuration(event.durationMs);
  const status = event.exit === 0 ? "pass" : "fail";
  const stepLabel = event.step.replace(/-/g, " ");

  let details = "";
  if (event.step === "validate-invalid" && event.stderrEnv) {
    const f = event.stderrEnv?.error?.details?.findings ?? [];
    details = f.length > 0 ? renderFindingsTable(f) : "";
  }
  if (event.step === "validate-valid" && event.stdoutEnv) {
    const checks = event.stdoutEnv?.data?.checks ?? [];
    details = checks.length > 0 ? renderChecksTable(checks) : "";
  }

  return `<article class="event cli" id="event-${index + 1}">
    <div class="event-head">
      <div class="event-title">
        <span class="step">${esc(stepLabel)}</span>
        <span class="badge cli">cli</span>
        <span class="badge ${status}">${status}</span>
      </div>
      <div class="time">+${offset} · ${dur}</div>
    </div>
    <div class="command-line">$ archetipo ${esc(event.args.join(" "))}</div>
    ${details}
  </article>`;
}

function renderFindingsTable(findings) {
  return `<table class="findings-table">
    <thead><tr><th>Code</th><th>Severity</th><th>Message</th><th>Path</th><th>Hint</th></tr></thead>
    <tbody>${findings.map((f) => `
      <tr>
        <td class="code">${esc(f.code)}</td>
        <td>${esc(f.severity)}</td>
        <td>${esc(f.message)}</td>
        <td><code>${esc(f.path)}</code></td>
        <td class="hint">${esc(f.hint)}</td>
      </tr>`).join("")}
    </tbody>
  </table>`;
}

function renderChecksTable(checks) {
  return `<table class="checks-table">
    <thead><tr><th>Check</th><th>Status</th><th>Message</th></tr></thead>
    <tbody>${checks.map((c) => `
      <tr>
        <td class="check-code">${esc(c.code)}</td>
        <td class="status-${c.status === "passed" ? "pass" : "fail"}">${esc(c.status)}</td>
        <td>${esc(c.message ?? "-")}</td>
      </tr>`).join("")}
    </tbody>
  </table>`;
}

function renderEnvelopeDetails(event) {
  const sections = [];
  if (event.stdoutEnv) {
    sections.push(`<details><summary>📤 STDOUT — ${esc(event.step)}</summary><pre>${esc(JSON.stringify(event.stdoutEnv, null, 2))}</pre></details>`);
  }
  if (event.stderrEnv) {
    sections.push(`<details><summary>📥 STDERR — ${esc(event.step)}</summary><pre>${esc(JSON.stringify(event.stderrEnv, null, 2))}</pre></details>`);
  }
  return sections.join("\n");
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

function parseArgs(argv) {
  const options = { workspaceRoot: defaultWorkspaceRoot, cleanup: false };
  for (let i = 0; i < argv.length; i += 1) {
    switch (argv[i]) {
      case "--workspace-root": options.workspaceRoot = path.resolve(argv[++i]); break;
      case "--cleanup": options.cleanup = true; break;
      case "--help": case "-h": printHelp(); process.exit(0);
      default: throw new Error(`Unknown argument: ${argv[i]}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Smoke test for inception PRD validation correction loop

Usage:
  node ./test/e2e/validate-inception-smoke.mjs
  npm run test:validate-inception

Options:
  --workspace-root <dir>  Parent directory for the generated sandbox
  --cleanup               Remove the run directory after the test
`);
}

async function createRunDir(root) {
  await fs.mkdir(root, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const runDir = path.join(root, stamp);
  await fs.mkdir(runDir, { recursive: true });
  return runDir;
}

function formatDuration(ms) {
  const totalSeconds = Math.max(1, Math.round(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes === 0 ? `${seconds}s` : `${minutes}m ${seconds}s`;
}

function esc(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

main().catch((error) => {
  console.error(`\nFAIL: ${error.message}`);
  process.exit(1);
});
