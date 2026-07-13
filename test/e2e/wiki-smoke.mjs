import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const repo = resolve(import.meta.dirname, "../..");
const sandbox = mkdtempSync(join(tmpdir(), "archetipo-wiki-e2e-"));
const binary = join(sandbox, "archetipo");

function run(args) {
  return JSON.parse(execFileSync(binary, args, { cwd: sandbox, encoding: "utf8" }));
}

try {
  execFileSync("go", ["build", "-o", binary, "./cmd/archetipo"], { cwd: join(repo, "cli"), stdio: "inherit" });
  mkdirSync(join(sandbox, ".archetipo"), { recursive: true });
  writeFileSync(join(sandbox, ".archetipo", "config.yaml"), "connector: file\npaths:\n  wiki: docs/wiki/\n");
  mkdirSync(join(sandbox, "src"), { recursive: true });
  writeFileSync(join(sandbox, "package.json"), '{"name":"wiki-smoke"}\n');
  writeFileSync(join(sandbox, "src", "index.ts"), "export const runtime = true;\n");
  execFileSync("git", ["init", "-q", "-b", "main"], { cwd: sandbox });
  execFileSync("git", ["config", "user.email", "e2e@archetipo.local"], { cwd: sandbox });
  execFileSync("git", ["config", "user.name", "ARchetipo E2E"], { cwd: sandbox });

  const inspection = run(["wiki", "inspect"]);
  assert.equal(inspection.kind, "wiki_inspection_result");
  assert.ok(inspection.data.boundaries.some((boundary) => boundary.path === "src"));
  assert.equal(run(["wiki", "init"]).kind, "wiki_init_result");
  const pageDir = join(sandbox, "docs", "wiki", "architecture");
  mkdirSync(pageDir, { recursive: true });
  writeFileSync(join(pageDir, "runtime.md"), `---
id: architecture.runtime
type: architecture
summary: Runtime boundaries
status: draft
---
# Runtime
`);
  const validation = run(["wiki", "validate"]);
  assert.equal(validation.kind, "validation_result");
  assert.equal(validation.data.ok, true);
  assert.equal(run(["wiki", "search", "runtime"]).data.count, 1);
  assert.equal(run(["wiki", "catalog"]).data.cataloged, 1);
  assert.match(readFileSync(join(pageDir, "runtime.md"), "utf8"), /status: draft/);
  assert.match(readFileSync(join(sandbox, "docs", "wiki", "index.md"), "utf8"), /\| draft \|/);
  assert.equal(run(["wiki", "publish"]).data.published, 1);
  assert.match(readFileSync(join(pageDir, "runtime.md"), "utf8"), /status: verified/);
  assert.match(readFileSync(join(sandbox, "docs", "wiki", "index.md"), "utf8"), /architecture\.runtime/);
  console.log("wiki smoke: pass");
} finally {
  rmSync(sandbox, { recursive: true, force: true });
}
