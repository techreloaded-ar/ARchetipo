import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { createGitAndWikiBaselines, literalGitPathspec } from "./baseline.mjs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..", "..");
const fixtureRoot = path.join(__dirname, "fixtures", "worktree-wiki-review");

function run(command, args, { cwd, env = process.env } = {}) {
  return new Promise((resolve) => {
    const child = spawn(command, args, { cwd, env, windowsHide: true });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => { stdout += chunk.toString(); });
    child.stderr.on("data", (chunk) => { stderr += chunk.toString(); });
    child.on("error", (error) => resolve({ ok: false, code: -1, stdout, stderr: `${stderr}${error.message}` }));
    child.on("close", (code) => resolve({ ok: code === 0, code, stdout, stderr }));
  });
}

async function listFiles(root, prefix = "") {
  const result = [];
  for (const entry of await fs.readdir(root, { withFileTypes: true })) {
    const relative = prefix ? path.posix.join(prefix, entry.name) : entry.name;
    if (entry.isDirectory()) result.push(...await listFiles(path.join(root, entry.name), relative));
    else result.push(relative);
  }
  return result;
}

function parseJSON(stdout, label) {
  try {
    return JSON.parse(stdout);
  } catch (error) {
    throw new Error(`${label} returned invalid JSON: ${error.message}\n${stdout}`);
  }
}

test("focused Wiki baseline commits tracked evidence before approval and starts fresh", { timeout: 180_000 }, async (t) => {
  const tempRoot = await fs.mkdtemp(path.join(os.tmpdir(), "archetipo-e2e-baseline-"));
  t.after(() => fs.rm(tempRoot, { recursive: true, force: true }));
  const sandboxDir = path.join(tempRoot, "sandbox");
  const binary = path.join(tempRoot, process.platform === "win32" ? "archetipo.exe" : "archetipo");
  await fs.cp(fixtureRoot, sandboxDir, { recursive: true });
  await fs.mkdir(path.join(sandboxDir, "bin"), { recursive: true });
  await fs.mkdir(path.join(sandboxDir, ".pi", "skills"), { recursive: true });
  await fs.writeFile(path.join(sandboxDir, "bin", "archetipo-sentinel"), "untracked\n");
  await fs.writeFile(path.join(sandboxDir, ".pi", "skills", "sentinel"), "untracked\n");
  await fs.writeFile(path.join(sandboxDir, "literal[1].txt"), "literal bracket path\n");
  await fs.writeFile(path.join(sandboxDir, "literal1.txt"), "pathspec sibling\n");

  const build = await run("go", ["build", "-o", binary, "./cmd/archetipo"], { cwd: path.join(repoRoot, "cli") });
  assert.equal(build.ok, true, `go build failed: ${build.stderr || build.stdout}`);
  const cliEnv = { ...process.env, ARCHETIPO_DATA_DIR: repoRoot };
  const cli = (...args) => run(binary, args, { cwd: sandboxDir, env: cliEnv });
  let verifiedBeforePrompts = false;

  const baselines = await createGitAndWikiBaselines({
    baselinePaths: ["docs", "hello.txt", "package.json", "literal[1].txt"],
    seedReviewedPages: ["behavior", "references/prd"],
    runGit: (_phase, args) => run("git", args, { cwd: sandboxDir }),
    approvePages: (pages) => cli("wiki", "approve", ...pages),
    verifySeededPages: async (pages) => {
      const statusRun = await cli("wiki", "status");
      assert.equal(statusRun.ok, true, statusRun.stderr || statusRun.stdout);
      const status = parseJSON(statusRun.stdout, "wiki status").data;
      const states = new Map(status.items.map((item) => [item.id, item.state]));
      for (const id of pages) {
        assert.equal(states.get(id), "reviewed", `${id} was stale immediately after seeded review commit`);
        const findings = (status.findings ?? []).filter((finding) => finding.page_id === id);
        assert.equal(findings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED"), false, `${id} changed before prompts`);
      }

      const validationRun = await cli("wiki", "validate");
      assert.equal(validationRun.ok, true, validationRun.stderr || validationRun.stdout);
      const validation = parseJSON(validationRun.stdout, "wiki validate").data;
      for (const id of pages) {
        assert.deepEqual((validation.findings ?? []).filter((finding) => finding.page_id === id), [], `${id} was structurally invalid before prompts`);
      }
      verifiedBeforePrompts = true;
    },
  });

  assert.equal(verifiedBeforePrompts, true, "seeded freshness verification did not run");
  assert.notEqual(baselines.generatedBaselineCommit, baselines.seededReviewBaselineCommit);

  const firstNames = await run("git", ["diff-tree", "--root", "--no-commit-id", "--name-only", "-r", baselines.generatedBaselineCommit], { cwd: sandboxDir });
  assert.equal(firstNames.ok, true, firstNames.stderr);
  const expectedFirst = [
    ...(await listFiles(path.join(fixtureRoot, "docs"))).map((file) => path.posix.join("docs", file)),
    "hello.txt",
    "literal[1].txt",
    "package.json",
  ].sort();
  const generatedNames = firstNames.stdout.trim().split(/\r?\n/).filter(Boolean).sort();
  assert.deepEqual(generatedNames, expectedFirst);
  assert.equal(generatedNames.includes("literal1.txt"), false, "Git pathspec magic staged the bracket filename's sibling");
  const literalBracket = await run("git", ["diff-tree", "--root", "--no-commit-id", "--name-only", "-r", baselines.generatedBaselineCommit, "--", literalGitPathspec("literal[1].txt")], { cwd: sandboxDir });
  assert.equal(literalBracket.ok, true, literalBracket.stderr);
  assert.equal(literalBracket.stdout.trim(), "literal[1].txt");
  const sibling = await run("git", ["cat-file", "-e", `${baselines.generatedBaselineCommit}:literal1.txt`], { cwd: sandboxDir });
  assert.equal(sibling.ok, false, "literal1.txt was staged by a non-literal bracket pathspec");

  const secondNames = await run("git", ["diff-tree", "--no-commit-id", "--name-only", "-r", baselines.seededReviewBaselineCommit], { cwd: sandboxDir });
  assert.equal(secondNames.ok, true, secondNames.stderr);
  assert.deepEqual(secondNames.stdout.trim().split(/\r?\n/).filter(Boolean).sort(), [
    "docs/wiki/behavior.md",
    "docs/wiki/index.md",
    "docs/wiki/log.md",
    "docs/wiki/references/prd.md",
  ]);

  for (const unwanted of ["bin/archetipo-sentinel", ".pi/skills/sentinel", ".archetipo/config.yaml"]) {
    const tracked = await run("git", ["cat-file", "-e", `${baselines.seededReviewBaselineCommit}:${unwanted}`], { cwd: sandboxDir });
    assert.equal(tracked.ok, false, `${unwanted} was unexpectedly committed`);
  }
});
