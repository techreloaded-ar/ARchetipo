#!/usr/bin/env node

import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import YAML from "yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..", "..");
const defaultConfig = path.join(__dirname, "benchmark.yaml");
const generatedRoot = path.join(repoRoot, "test", "workspaces", "wiki-benchmark");
const ansiPattern = /[\u001B\u009B][[\]()#;?]*(?:(?:(?:[a-zA-Z\d]*(?:;[-a-zA-Z\d\/#&.:=?%@~_]+)*)?\u0007)|(?:(?:\d{1,4}(?:[;:]\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]))/g;

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const configPath = path.resolve(options.config ?? defaultConfig);
  const config = YAML.parse(await fs.readFile(configPath, "utf8"));
  const casesPath = path.resolve(path.dirname(configPath), config.cases_file);
  const caseManifest = YAML.parse(await fs.readFile(casesPath, "utf8"));
  const repository = path.resolve(path.dirname(configPath), options.repository ?? config.repository.path);
  const revision = options.revision ?? config.repository.revision;
  const model = options.model ?? config.agent.model;
  const repetitions = options.repetitions ?? config.repetitions ?? 1;
  const conditions = selectValues(options.conditions, config.conditions, ["raw", "wiki"]);
  const cases = selectCases(caseManifest.cases, options.cases);
  if (options.reparse) {
    await reparseRun({
      runRoot: path.resolve(options.reparse),
      benchmarkCases: caseManifest.cases,
      configPath,
      repository,
    });
    return;
  }
  const runId = new Date().toISOString().replaceAll(":", "-").replaceAll(".", "-");
  const runRoot = path.join(generatedRoot, "runs", runId);

  await assertRepositoryRevision(repository, revision);
  await fs.mkdir(runRoot, { recursive: true });

  const results = [];
  for (const [caseIndex, benchmarkCase] of cases.entries()) {
    for (let repetition = 1; repetition <= repetitions; repetition += 1) {
      const orderedConditions = balancedConditionOrder(conditions, caseIndex, repetition);
      for (const [scheduleIndex, condition] of orderedConditions.entries()) {
        console.log(`==> ${benchmarkCase.id} [${condition}] repetition ${repetition}/${repetitions}`);
        if (options.dryRun) {
          results.push({ case_id: benchmarkCase.id, condition, repetition, schedule_position: scheduleIndex + 1, status: "dry-run" });
          continue;
        }
        const result = await runCase({
          benchmarkCase,
          condition,
          repetition,
          repository,
          revision,
          runRoot,
          schedulePosition: scheduleIndex + 1,
          agent: { ...config.agent, model },
        });
        results.push(result);
        console.log(`    ${result.status} score=${result.score.total.toFixed(1)} tokens=${result.usage.total_tokens} files=${result.telemetry.files_read.length}`);
      }
    }
  }

  const summary = buildSummary({ configPath, repository, revision, model, runId, results });
  await fs.writeFile(path.join(runRoot, "summary.json"), `${JSON.stringify(summary, null, 2)}\n`);
  await fs.writeFile(path.join(runRoot, "report.html"), renderHtml(summary));
  console.log(`\nReport: ${path.join(runRoot, "report.html")}`);
  console.log(`Summary: ${path.join(runRoot, "summary.json")}`);
}

function balancedConditionOrder(conditions, caseIndex, repetition) {
  if (conditions.length !== 2) return [...conditions];
  return (caseIndex + repetition) % 2 === 0 ? [...conditions].reverse() : [...conditions];
}

function parseArgs(argv) {
  const options = {};
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    switch (arg) {
      case "--config": options.config = argv[++index]; break;
      case "--repository": options.repository = argv[++index]; break;
      case "--revision": options.revision = argv[++index]; break;
      case "--model": options.model = argv[++index]; break;
      case "--cases": options.cases = argv[++index]; break;
      case "--conditions": options.conditions = argv[++index]; break;
      case "--repetitions": options.repetitions = Number(argv[++index]); break;
      case "--reparse": options.reparse = argv[++index]; break;
      case "--dry-run": options.dryRun = true; break;
      case "--help":
      case "-h": printHelp(); process.exit(0); break;
      default: throw new Error(`Unknown argument: ${arg}`);
    }
  }
  if (options.repetitions !== undefined && (!Number.isInteger(options.repetitions) || options.repetitions < 1)) {
    throw new Error("--repetitions must be a positive integer");
  }
  return options;
}

function printHelp() {
  console.log(`RideAtlas Wiki benchmark\n\nUsage:\n  npm run test:wiki-benchmark -- [options]\n\nOptions:\n  --cases id1,id2\n  --conditions raw,wiki\n  --repetitions N\n  --repository PATH\n  --revision SHA\n  --model PROVIDER/MODEL\n  --reparse RUN_DIRECTORY\n  --dry-run\n`);
}

function selectValues(raw, configured, allowed) {
  const values = raw ? raw.split(",").map((value) => value.trim()).filter(Boolean) : configured;
  if (!Array.isArray(values) || values.length === 0) throw new Error("At least one condition is required");
  for (const value of values) if (!allowed.includes(value)) throw new Error(`Unsupported condition: ${value}`);
  return [...new Set(values)];
}

function selectCases(cases, rawFilter) {
  if (!Array.isArray(cases) || cases.length === 0) throw new Error("No benchmark cases configured");
  if (!rawFilter) return cases;
  const wanted = new Set(rawFilter.split(",").map((value) => value.trim()).filter(Boolean));
  const selected = cases.filter((item) => wanted.has(item.id));
  const missing = [...wanted].filter((id) => !selected.some((item) => item.id === id));
  if (missing.length > 0) throw new Error(`Unknown benchmark cases: ${missing.join(", ")}`);
  return selected;
}

async function assertRepositoryRevision(repository, revision) {
  const result = await runProcess("git", ["cat-file", "-e", `${revision}^{commit}`], { cwd: repository, timeoutMs: 30_000 });
  if (result.code !== 0) throw new Error(`Revision ${revision} is not available in ${repository}: ${result.stderr}`);
}

async function runCase({ benchmarkCase, condition, repetition, repository, revision, runRoot, schedulePosition, agent }) {
  const caseRoot = path.join(runRoot, benchmarkCase.id, condition, String(repetition));
  const sandbox = path.join(caseRoot, "sandbox");
  await fs.mkdir(caseRoot, { recursive: true });
  const clone = await runProcess("git", ["clone", "--quiet", "--no-hardlinks", repository, sandbox], { cwd: caseRoot, timeoutMs: 120_000 });
  if (clone.code !== 0) throw new Error(`Could not clone benchmark repository: ${clone.stderr}`);
  const checkout = await runProcess("git", ["checkout", "--quiet", "--detach", revision], { cwd: sandbox, timeoutMs: 30_000 });
  if (checkout.code !== 0) throw new Error(`Could not checkout ${revision}: ${checkout.stderr}`);
  if (condition === "raw") await fs.rm(path.join(sandbox, "docs", "wiki"), { recursive: true, force: true });

  const prompt = buildPrompt(benchmarkCase, condition);
  const startedAt = Date.now();
  const invocation = await runProcess(agent.command, [
    "--mode", "json",
    "--print",
    "--no-session",
    "--no-skills",
    "--no-extensions",
    "--no-context-files",
    "--offline",
    "--tools", "read,grep,find,ls",
    "--model", agent.model,
    prompt,
  ], { cwd: sandbox, timeoutMs: agent.timeout_ms ?? 600_000 });
  const trace = parsePiTrace(invocation.stdout, sandbox);
  const score = scoreResult(trace.finalText, benchmarkCase.oracle);
  const result = {
    case_id: benchmarkCase.id,
    category: benchmarkCase.category,
    condition,
    repetition,
    schedule_position: schedulePosition,
    status: invocation.code === 0 ? "pass" : "agent-error",
    revision,
    model: agent.model,
    duration_ms: Date.now() - startedAt,
    usage: trace.usage,
    telemetry: trace.telemetry,
    score,
    final_text: trace.finalText,
    stderr: invocation.stderr,
  };
  await fs.writeFile(path.join(caseRoot, "prompt.txt"), `${prompt}\n`);
  await fs.writeFile(path.join(caseRoot, "trace.jsonl"), invocation.stdout);
  await fs.writeFile(path.join(caseRoot, "result.json"), `${JSON.stringify(result, null, 2)}\n`);
  return result;
}

async function reparseRun({ runRoot, benchmarkCases, configPath, repository }) {
  const oracleByCase = new Map(benchmarkCases.map((benchmarkCase) => [benchmarkCase.id, benchmarkCase.oracle]));
  const resultPaths = (await listFiles(runRoot)).filter((file) => path.basename(file) === "result.json");
  if (resultPaths.length === 0) throw new Error(`No result.json files found below ${runRoot}`);
  const results = [];
  for (const resultPath of resultPaths.sort()) {
    const caseRoot = path.dirname(resultPath);
    const result = JSON.parse(await fs.readFile(resultPath, "utf8"));
    const oracle = oracleByCase.get(result.case_id);
    if (!oracle) throw new Error(`No oracle configured for case ${result.case_id}`);
    const trace = parsePiTrace(await fs.readFile(path.join(caseRoot, "trace.jsonl"), "utf8"), path.join(caseRoot, "sandbox"));
    result.usage = trace.usage;
    result.telemetry = trace.telemetry;
    result.score = scoreResult(trace.finalText, oracle);
    result.final_text = trace.finalText;
    await fs.writeFile(resultPath, `${JSON.stringify(result, null, 2)}\n`);
    results.push(result);
  }
  const first = results[0];
  const summary = buildSummary({
    configPath,
    repository,
    revision: first.revision,
    model: first.model,
    runId: path.basename(runRoot),
    results,
  });
  await fs.writeFile(path.join(runRoot, "summary.json"), `${JSON.stringify(summary, null, 2)}\n`);
  await fs.writeFile(path.join(runRoot, "report.html"), renderHtml(summary));
  console.log(`Re-scored ${results.length} traces without invoking the agent.`);
  console.log(`Report: ${path.join(runRoot, "report.html")}`);
  console.log(`Summary: ${path.join(runRoot, "summary.json")}`);
}

async function listFiles(root) {
  const entries = await fs.readdir(root, { withFileTypes: true });
  const nested = await Promise.all(entries.map((entry) => {
    const target = path.join(root, entry.name);
    return entry.isDirectory() ? listFiles(target) : [target];
  }));
  return nested.flat();
}

function buildPrompt(benchmarkCase, condition) {
  const contextInstruction = condition === "wiki"
    ? "È disponibile una Wiki in docs/wiki. Inizia da docs/wiki/index.md, carica solo le pagine pertinenti e verifica nel codice le affermazioni specifiche sull'implementazione."
    : "La Wiki di progetto non è disponibile. Esplora la codebase in modo chirurgico partendo da manifest, entry point, ricerca testuale e test pertinenti.";
  return `Sei in un benchmark read-only di comprensione della codebase. Non modificare file, non usare Internet e non eseguire comandi di scrittura.\n\n${contextInstruction}\n\nTask:\n${benchmarkCase.prompt}\n\nRispondi in italiano con un singolo oggetto JSON valido, senza markdown, con questa forma:\n{"summary":"...","files":["path"],"findings":["..."],"risks":["..."],"verification":["..."]}`;
}

function parsePiTrace(raw, sandbox = "") {
  const events = [];
  for (const line of raw.split(/\r?\n/)) {
    const clean = line.replace(ansiPattern, "").trim();
    if (!clean.startsWith("{")) continue;
    try { events.push(JSON.parse(clean)); } catch { /* ignore terminal noise */ }
  }
  const responses = new Map();
  let finalText = "";
  const toolCalls = new Map();
  for (const event of events) {
    if (event.type === "message_end" && event.message?.role === "assistant") {
      const responseId = event.message.responseId ?? `message-${responses.size}`;
      responses.set(responseId, event.message.usage ?? {});
      const text = extractText(event.message.content);
      if (text) finalText = text;
    }
    collectToolCalls(event, toolCalls);
  }
  const usage = { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, reasoning_tokens: 0, total_tokens: 0, cost: 0 };
  for (const item of responses.values()) {
    usage.input_tokens += item.input ?? 0;
    usage.output_tokens += item.output ?? 0;
    usage.cache_read_tokens += item.cacheRead ?? 0;
    usage.cache_write_tokens += item.cacheWrite ?? 0;
    usage.reasoning_tokens += item.reasoning ?? 0;
    usage.total_tokens += item.totalTokens ?? 0;
    usage.cost += item.cost?.total ?? 0;
  }
  const filesRead = new Set();
  const inspectedPaths = new Set();
  for (const call of toolCalls.values()) {
    collectPaths(call.arguments, inspectedPaths, sandbox);
    if (call.name === "read") collectPaths(call.arguments, filesRead, sandbox);
  }
  return {
    finalText,
    usage,
    telemetry: {
      tool_calls: toolCalls.size,
      files_read: [...filesRead].sort(),
      inspected_paths: [...inspectedPaths].sort(),
      event_count: events.length,
    },
  };
}

function extractText(content) {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content.filter((item) => item?.type === "text").map((item) => item.text ?? "").join("");
}

function collectToolCalls(value, target) {
  if (!value || typeof value !== "object") return;
  if (value.type === "toolCall" && typeof value.id === "string") {
    const previous = target.get(value.id) ?? {};
    const hasArguments = value.arguments && Object.keys(value.arguments).length > 0;
    target.set(value.id, {
      name: value.name ?? previous.name,
      arguments: hasArguments ? value.arguments : previous.arguments,
    });
  }
  for (const child of Object.values(value)) collectToolCalls(child, target);
}

function collectPaths(value, target, sandbox) {
  if (!value || typeof value !== "object") return;
  for (const [key, child] of Object.entries(value)) {
    if (["path", "file", "file_path"].includes(key) && typeof child === "string") {
      const normalized = normalizeRepositoryPath(child, sandbox);
      if (normalized) target.add(normalized);
    } else if (typeof child === "object") {
      collectPaths(child, target, sandbox);
    }
  }
}

function normalizeRepositoryPath(value, sandbox) {
  const candidate = value.replace(/^\.\//, "");
  if (!path.isAbsolute(candidate)) return candidate;
  if (!sandbox) return null;
  const relative = path.relative(sandbox, candidate);
  return relative && !relative.startsWith("..") && !path.isAbsolute(relative) ? relative : null;
}

function scoreResult(finalText, oracle = {}) {
  const normalized = finalText ?? "";
  const parsed = parseJsonObject(normalized);
  const fileValues = Array.isArray(parsed?.files) ? parsed.files : extractStringArray(normalized, "files");
  const reportedFiles = new Set(fileValues.map((item) => String(item).replace(/^\.\//, "")));
  const expectedFiles = oracle.expected_files ?? [];
  const matchedFiles = expectedFiles.filter((file) => reportedFiles.has(file));
  const fileScore = expectedFiles.length === 0 ? 40 : 40 * matchedFiles.length / expectedFiles.length;
  const required = oracle.required_findings ?? [];
  const matchedFindings = required.filter((finding) => (finding.any ?? []).some((pattern) => new RegExp(pattern, "is").test(normalized)));
  const findingScore = required.length === 0 ? 50 : 50 * matchedFindings.length / required.length;
  const forbiddenMatches = (oracle.forbidden_claims ?? []).filter((pattern) => new RegExp(pattern, "is").test(normalized));
  const forbiddenPenalty = Math.min(10, forbiddenMatches.length * 5);
  const formatScore = parsed && Array.isArray(parsed.files) && Array.isArray(parsed.findings) ? 10 : 0;
  return {
    total: Math.max(0, fileScore + findingScore + formatScore - forbiddenPenalty),
    file_score: fileScore,
    finding_score: findingScore,
    format_score: formatScore,
    forbidden_penalty: forbiddenPenalty,
    matched_files: matchedFiles,
    missing_files: expectedFiles.filter((file) => !reportedFiles.has(file)),
    matched_findings: matchedFindings.map((item) => item.id),
    missing_findings: required.filter((item) => !matchedFindings.includes(item)).map((item) => item.id),
    forbidden_matches: forbiddenMatches,
  };
}

function parseJsonObject(value) {
  const trimmed = value.trim();
  const candidates = [trimmed];
  const fenced = trimmed.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/i);
  if (fenced) candidates.push(fenced[1]);
  const firstBrace = trimmed.indexOf("{");
  const lastBrace = trimmed.lastIndexOf("}");
  if (firstBrace >= 0 && lastBrace > firstBrace) candidates.push(trimmed.slice(firstBrace, lastBrace + 1));
  for (const candidate of candidates) {
    try {
      const parsed = JSON.parse(candidate);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) return parsed;
    } catch { /* try the next tolerant representation */ }
  }
  return null;
}

function extractStringArray(value, property) {
  const propertyIndex = value.indexOf(`"${property}"`);
  const start = propertyIndex >= 0 ? value.indexOf("[", propertyIndex) : -1;
  if (start < 0) return [];
  let inString = false;
  let escaped = false;
  let end = -1;
  for (let index = start + 1; index < value.length; index += 1) {
    const character = value[index];
    if (escaped) {
      escaped = false;
    } else if (character === "\\" && inString) {
      escaped = true;
    } else if (character === '"') {
      inString = !inString;
    } else if (character === "]" && !inString) {
      end = index;
      break;
    }
  }
  if (end < 0) return [];
  try {
    const parsed = JSON.parse(value.slice(start, end + 1));
    return Array.isArray(parsed) ? parsed.filter((item) => typeof item === "string") : [];
  } catch {
    return [];
  }
}

function buildSummary({ configPath, repository, revision, model, runId, results }) {
  const aggregates = {};
  for (const result of results) {
    if (result.status === "dry-run") continue;
    const key = result.condition;
    aggregates[key] ??= {
      runs: 0,
      score_total: 0,
      tokens_total: 0,
      input_tokens_total: 0,
      output_tokens_total: 0,
      cache_read_tokens_total: 0,
      cost_total: 0,
      duration_total_ms: 0,
      files_total: 0,
    };
    aggregates[key].runs += 1;
    aggregates[key].score_total += result.score.total;
    aggregates[key].tokens_total += result.usage.total_tokens;
    aggregates[key].input_tokens_total += result.usage.input_tokens;
    aggregates[key].output_tokens_total += result.usage.output_tokens;
    aggregates[key].cache_read_tokens_total += result.usage.cache_read_tokens;
    aggregates[key].cost_total += result.usage.cost;
    aggregates[key].duration_total_ms += result.duration_ms;
    aggregates[key].files_total += result.telemetry.files_read.length;
  }
  for (const aggregate of Object.values(aggregates)) {
    aggregate.score_mean = aggregate.score_total / aggregate.runs;
    aggregate.tokens_mean = aggregate.tokens_total / aggregate.runs;
    aggregate.input_tokens_mean = aggregate.input_tokens_total / aggregate.runs;
    aggregate.output_tokens_mean = aggregate.output_tokens_total / aggregate.runs;
    aggregate.cache_read_tokens_mean = aggregate.cache_read_tokens_total / aggregate.runs;
    aggregate.cost_mean = aggregate.cost_total / aggregate.runs;
    aggregate.duration_mean_ms = aggregate.duration_total_ms / aggregate.runs;
    aggregate.files_mean = aggregate.files_total / aggregate.runs;
  }
  const raw = aggregates.raw;
  const wiki = aggregates.wiki;
  const comparison = raw && wiki ? {
    token_change_percent: percentChange(raw.tokens_mean, wiki.tokens_mean),
    input_token_change_percent: percentChange(raw.input_tokens_mean, wiki.input_tokens_mean),
    output_token_change_percent: percentChange(raw.output_tokens_mean, wiki.output_tokens_mean),
    cache_read_token_change_percent: percentChange(raw.cache_read_tokens_mean, wiki.cache_read_tokens_mean),
    cost_change_percent: percentChange(raw.cost_mean, wiki.cost_mean),
    score_change: wiki.score_mean - raw.score_mean,
    file_change_percent: percentChange(raw.files_mean, wiki.files_mean),
    duration_change_percent: percentChange(raw.duration_mean_ms, wiki.duration_mean_ms),
  } : null;
  return { schema: "archetipo/wiki-benchmark-result/v1", run_id: runId, config_path: configPath, repository, revision, model, aggregates, comparison, results };
}

function percentChange(baseline, candidate) {
  if (!baseline) return null;
  return 100 * (candidate - baseline) / baseline;
}

function renderHtml(summary) {
  const rows = summary.results.map((result) => `<tr><td>${escapeHtml(result.case_id)}</td><td>${escapeHtml(result.condition)}</td><td>${result.repetition}</td><td>${result.score?.total?.toFixed?.(1) ?? "-"}</td><td>${result.usage?.input_tokens ?? "-"}</td><td>${result.usage?.cache_read_tokens ?? "-"}</td><td>${result.usage?.total_tokens ?? "-"}</td><td>${result.telemetry?.files_read?.length ?? "-"}</td><td>${result.duration_ms ?? "-"}</td></tr>`).join("\n");
  const comparison = summary.comparison ? `<pre>${escapeHtml(JSON.stringify(summary.comparison, null, 2))}</pre>` : "<p>No A/B comparison available.</p>";
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>ARchetipo Wiki benchmark</title><style>body{font:14px system-ui;margin:2rem;max-width:1100px}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ddd;padding:.5rem;text-align:left}th{background:#f5f5f5}pre{background:#f6f8fa;padding:1rem}</style></head><body><h1>ARchetipo Wiki benchmark</h1><p><strong>Repository:</strong> ${escapeHtml(summary.repository)} @ <code>${escapeHtml(summary.revision)}</code></p><p><strong>Model:</strong> ${escapeHtml(summary.model)}</p><h2>A/B comparison</h2>${comparison}<h2>Runs</h2><table><thead><tr><th>Case</th><th>Condition</th><th>Rep</th><th>Score</th><th>Input</th><th>Cache read</th><th>Total processed</th><th>Files read</th><th>Duration ms</th></tr></thead><tbody>${rows}</tbody></table></body></html>`;
}

function escapeHtml(value) {
  return String(value ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

function runProcess(command, args, { cwd, timeoutMs }) {
  return new Promise((resolve) => {
    const child = spawn(command, args, { cwd, env: { ...process.env, PI_OFFLINE: "1", NO_COLOR: "1" }, stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    const timeout = setTimeout(() => child.kill("SIGTERM"), timeoutMs);
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.on("error", (error) => {
      clearTimeout(timeout);
      resolve({ code: 1, stdout, stderr: `${stderr}${error.message}` });
    });
    child.on("close", (code) => {
      clearTimeout(timeout);
      resolve({ code: code ?? 1, stdout, stderr });
    });
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === __filename) {
  main().catch((error) => {
    console.error(error.stack ?? error.message);
    process.exit(1);
  });
}

export { parsePiTrace, scoreResult };
