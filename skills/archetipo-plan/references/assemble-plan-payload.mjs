#!/usr/bin/env node
// Assembles the `archetipo spec plan` payload from staged part files.
//
// The planning worker never generates the whole payload in a single model
// response: it writes one part file per unit of content, then this script
// merges them deterministically. Content that already exists (the persisted
// plan body and the previously persisted tasks) is carried over verbatim
// through `carry-over` and never regenerated.
//
// Usage:
//   node assemble-plan-payload.mjs carry-over <US-CODE> <staging-dir> [--project-root <path>]
//   node assemble-plan-payload.mjs build      <US-CODE> <staging-dir> <out.json>
//   node assemble-plan-payload.mjs clean      <staging-dir> [<out.json>]
//
// See ./payload-assembly.md for the staging layout and the part file format.

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const [, , mode, ...rest] = process.argv;

function fail(message) {
  console.error(`assemble-plan-payload: ${message}`);
  process.exit(1);
}

function readText(file) {
  return fs.readFileSync(file, "utf8");
}

function listStaged(dir, prefix) {
  if (!fs.existsSync(dir)) return [];
  return fs
    .readdirSync(dir)
    .filter((name) => name.startsWith(prefix) && name.endsWith(".md"))
    .map((name) => {
      const digits = name.match(/(\d+)/);
      return { name, order: digits ? Number(digits[1]) : Number.MAX_SAFE_INTEGER };
    })
    .sort((a, b) => a.order - b.order || a.name.localeCompare(b.name))
    .map((entry) => path.join(dir, entry.name));
}

// Parses a part file: a `---` delimited key: value header followed by the
// markdown body. Only the first closing `---` is treated as the delimiter, so
// horizontal rules inside the body are safe.
function parseTaskPart(file) {
  const lines = readText(file).split(/\r?\n/);
  if (lines[0].trim() !== "---") fail(`missing front matter delimiter in ${file}`);
  const close = lines.findIndex((line, index) => index > 0 && line.trim() === "---");
  if (close === -1) fail(`unterminated front matter in ${file}`);

  const meta = {};
  for (const line of lines.slice(1, close)) {
    if (!line.trim()) continue;
    const separator = line.indexOf(":");
    if (separator === -1) fail(`malformed front matter line in ${file}: ${line}`);
    meta[line.slice(0, separator).trim()] = line.slice(separator + 1).trim();
  }
  for (const required of ["id", "title", "type"]) {
    if (!meta[required]) fail(`front matter field "${required}" missing in ${file}`);
  }

  const body = lines.slice(close + 1).join("\n").replace(/^\n+/, "").replace(/\s+$/, "");
  if (!body) fail(`empty task body in ${file}`);

  const task = {
    id: meta.id,
    title: meta.title,
    type: meta.type,
    status: meta.status || "TODO",
    body,
  };
  const dependencies = (meta.dependencies || "")
    .replace(/^\[|\]$/g, "")
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);
  if (dependencies.length) task.dependencies = dependencies;
  return task;
}

function runCarryOver(specCode, stagingDir, projectRoot) {
  fs.mkdirSync(stagingDir, { recursive: true });

  let raw;
  try {
    raw = execFileSync("archetipo", ["spec", "show", specCode], {
      cwd: projectRoot,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch (error) {
    fail(`archetipo spec show ${specCode} failed: ${error.message}`);
  }

  let envelope;
  try {
    envelope = JSON.parse(raw);
  } catch {
    fail(`archetipo spec show ${specCode} did not return a JSON envelope`);
  }

  const data = envelope.data || {};
  const tasks = (data.tasks || []).map((task) => {
    const carried = {
      id: task.id,
      title: task.title,
      type: task.type,
      status: task.status,
      body: task.body || "",
    };
    if (task.dependencies && task.dependencies.length) carried.dependencies = task.dependencies;
    return carried;
  });

  const missingBody = tasks.filter((task) => !task.body).map((task) => task.id);
  if (missingBody.length) fail(`persisted tasks without body: ${missingBody.join(", ")}`);

  fs.writeFileSync(
    path.join(stagingDir, "existing-tasks.json"),
    `${JSON.stringify(tasks, null, 1)}\n`,
    "utf8",
  );

  const planBody = data.plan_body || "";
  if (planBody.trim()) {
    fs.writeFileSync(path.join(stagingDir, "plan-body-00-carried.md"), planBody, "utf8");
  }

  console.log(
    `carried over: ${tasks.length} task(s), plan_body ${planBody.length} chars → ${stagingDir}`,
  );
}

function runBuild(specCode, stagingDir, outFile) {
  if (!fs.existsSync(stagingDir)) fail(`staging directory not found: ${stagingDir}`);

  const bodyParts = listStaged(stagingDir, "plan-body");
  if (!bodyParts.length) fail(`no plan-body*.md part files in ${stagingDir}`);
  const planBody = bodyParts.map((file) => readText(file).replace(/\s+$/, "")).join("\n\n");

  const tasks = [];
  const carriedFile = path.join(stagingDir, "existing-tasks.json");
  if (fs.existsSync(carriedFile)) {
    const carried = JSON.parse(readText(carriedFile));
    if (!Array.isArray(carried)) fail(`existing-tasks.json is not an array`);
    tasks.push(...carried);
  }
  for (const file of listStaged(stagingDir, "task-")) tasks.push(parseTaskPart(file));

  if (!tasks.length) fail(`no tasks assembled from ${stagingDir}`);

  const seen = new Set();
  for (const task of tasks) {
    if (seen.has(task.id)) fail(`duplicate task id: ${task.id}`);
    seen.add(task.id);
  }
  for (const task of tasks) {
    for (const dependency of task.dependencies || []) {
      if (!seen.has(dependency)) fail(`task ${task.id} depends on unknown task ${dependency}`);
    }
  }

  fs.mkdirSync(path.dirname(path.resolve(outFile)), { recursive: true });
  fs.writeFileSync(
    outFile,
    `${JSON.stringify({ plan_body: planBody, tasks }, null, 1)}\n`,
    "utf8",
  );

  const todo = tasks.filter((task) => task.status !== "DONE").length;
  console.log(
    `${specCode}: ${tasks.length} task(s) (${todo} not DONE), plan_body ${planBody.length} chars, ` +
      `${bodyParts.length} body part(s) → ${outFile}`,
  );
}

function runClean(stagingDir, outFile) {
  // This is the only destructive operation in the script: guard it so a wrong
  // argument can never remove anything but a staging directory.
  const resolved = path.resolve(stagingDir);
  if (
    path.basename(resolved).startsWith("plan-") === false ||
    path.basename(path.dirname(resolved)) !== "tmp"
  ) {
    fail(`refusing to remove ${resolved}: a staging directory must be <...>/tmp/plan-<US-CODE>`);
  }
  if (resolved === path.resolve(process.cwd()) || process.cwd().startsWith(`${resolved}${path.sep}`)) {
    fail(`refusing to remove ${resolved}: it contains the current working directory`);
  }
  if (outFile && !path.basename(path.resolve(outFile)).endsWith(".json")) {
    fail(`refusing to remove ${outFile}: the payload file must be a .json file`);
  }

  fs.rmSync(resolved, { recursive: true, force: true });
  if (outFile) fs.rmSync(outFile, { force: true });
  console.log(`cleaned: ${resolved}${outFile ? ` and ${outFile}` : ""}`);
}

switch (mode) {
  case "carry-over": {
    const [specCode, stagingDir, ...flags] = rest;
    if (!specCode || !stagingDir) fail("usage: carry-over <US-CODE> <staging-dir> [--project-root <path>]");
    const rootIndex = flags.indexOf("--project-root");
    runCarryOver(specCode, stagingDir, rootIndex === -1 ? process.cwd() : flags[rootIndex + 1]);
    break;
  }
  case "build": {
    const [specCode, stagingDir, outFile] = rest;
    if (!specCode || !stagingDir || !outFile) fail("usage: build <US-CODE> <staging-dir> <out.json>");
    runBuild(specCode, stagingDir, outFile);
    break;
  }
  case "clean": {
    const [stagingDir, outFile] = rest;
    if (!stagingDir) fail("usage: clean <staging-dir> [<out.json>]");
    runClean(stagingDir, outFile);
    break;
  }
  default:
    fail("usage: assemble-plan-payload.mjs <carry-over|build|clean> ...");
}
