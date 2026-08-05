#!/usr/bin/env node
// Assembles the `archetipo spec add` payload from staged part files.
//
// The bootstrap and extension workers never generate the whole payload in a
// single model response: they write one part file per spec, then this script
// merges them deterministically. `archetipo spec add` is append-only and
// idempotent (specs whose code already exists are skipped and reported in
// `data.skipped`), so unlike planning there is no carried content to copy
// verbatim — only a build step is needed.
//
// Usage:
//   node assemble-specs-payload.mjs build <staging-dir> <out.json>
//   node assemble-specs-payload.mjs clean <staging-dir> [<out.json>]
//
// See ./specs-payload-assembly.md for the staging layout and the part file format.

import fs from "node:fs";
import path from "node:path";

const [, , mode, ...rest] = process.argv;

function fail(message) {
  console.error(`assemble-specs-payload: ${message}`);
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
function parseSpecPart(file) {
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
  for (const required of ["code", "title", "epic_code", "priority", "points", "scope"]) {
    if (!meta[required]) fail(`front matter field "${required}" missing in ${file}`);
  }

  const body = lines.slice(close + 1).join("\n").replace(/^\n+/, "").replace(/\s+$/, "");
  if (!body) fail(`empty spec body in ${file}`);

  const points = Number(meta.points);
  if (!Number.isFinite(points)) fail(`front matter field "points" is not a number in ${file}`);

  // An omitted epic_title is left empty on purpose: the CLI resolves the real
  // title from the existing epics. Defaulting it to the code would overwrite a
  // known epic title with the literal "EP-00N".
  const spec = {
    code: meta.code,
    title: meta.title,
    epic: { code: meta.epic_code, title: meta.epic_title || "" },
    priority: meta.priority,
    points,
    status: meta.status || "TODO",
    scope: meta.scope,
    body,
  };
  const blockedBy = (meta.blocked_by || "")
    .replace(/^\[|\]$/g, "")
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);
  spec.blocked_by = blockedBy;
  return spec;
}

function runBuild(stagingDir, outFile) {
  if (!fs.existsSync(stagingDir)) fail(`staging directory not found: ${stagingDir}`);

  const specs = listStaged(stagingDir, "spec-").map(parseSpecPart);
  if (!specs.length) fail(`no spec-*.md part files in ${stagingDir}`);

  const seen = new Set();
  for (const spec of specs) {
    if (seen.has(spec.code)) fail(`duplicate spec code: ${spec.code}`);
    seen.add(spec.code);
  }
  for (const spec of specs) {
    for (const dependency of spec.blocked_by) {
      if (!seen.has(dependency)) fail(`spec ${spec.code} is blocked by unknown spec ${dependency}`);
    }
  }

  fs.mkdirSync(path.dirname(path.resolve(outFile)), { recursive: true });
  fs.writeFileSync(outFile, `${JSON.stringify({ specs }, null, 1)}\n`, "utf8");

  console.log(`${specs.length} spec(s), ${specs.length} part file(s) → ${outFile}`);
}

function runClean(stagingDir, outFile) {
  // This is the only destructive operation in the script: guard it so a wrong
  // argument can never remove anything but a staging directory.
  const resolved = path.resolve(stagingDir);
  if (
    path.basename(resolved).startsWith("specs-") === false ||
    path.basename(path.dirname(resolved)) !== "tmp"
  ) {
    fail(`refusing to remove ${resolved}: a staging directory must be <...>/tmp/specs-<range>`);
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
  case "build": {
    const [stagingDir, outFile] = rest;
    if (!stagingDir || !outFile) fail("usage: build <staging-dir> <out.json>");
    runBuild(stagingDir, outFile);
    break;
  }
  case "clean": {
    const [stagingDir, outFile] = rest;
    if (!stagingDir) fail("usage: clean <staging-dir> [<out.json>]");
    runClean(stagingDir, outFile);
    break;
  }
  default:
    fail("usage: assemble-specs-payload.mjs <build|clean> ...");
}
