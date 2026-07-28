import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const repo = resolve(import.meta.dirname, "../..");
const sandbox = mkdtempSync(join(tmpdir(), "archetipo-wiki-e2e-"));
const binary = join(sandbox, process.platform === "win32" ? "archetipo.exe" : "archetipo");

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
  writeFileSync(join(sandbox, "src", "hub.ts"), "export const hub = true;\n");
  mkdirSync(join(sandbox, "docs"), { recursive: true });
  writeFileSync(join(sandbox, "docs", "PRD.md"), "# Product requirements\n\nAuthoritative intent.\n");
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
type: architecture
title: Runtime
description: Runtime boundaries
status: generated
---
# Runtime
`);
  const decisionDir = join(sandbox, "docs", "wiki", "decisions");
  mkdirSync(decisionDir, { recursive: true });
  writeFileSync(join(decisionDir, "shared-runtime.md"), `---
type: decision
decision_status: accepted
title: Shared runtime
description: Use one shared runtime implementation
status: generated
sources:
  - path: docs/wiki/architecture/runtime.md
    role: implementation
---
# Shared runtime

<!-- archetipo:wiki section=context -->
Multiple processes need consistent behavior.

<!-- archetipo:wiki section=decision -->
Use the shared runtime implementation.

<!-- archetipo:wiki section=alternatives -->
Per-process implementations were rejected because they can drift.

<!-- archetipo:wiki section=consequences -->
Consistency improves, while the shared dependency becomes operationally significant.

<!-- archetipo:wiki section=verification -->
The exported runtime and its tests verify adoption.
`);
  const referenceDir = join(sandbox, "docs", "wiki", "references");
  writeFileSync(join(referenceDir, "prd.md"), `---
type: reference
title: Product requirements
description: Authoritative product requirements
status: generated
sources:
  - path: docs/PRD.md
    role: original
    freshness: tracked
  - path: src/hub.ts
    role: implementation
    freshness: context
---
# Product requirements

The original requirements remain authoritative; the hub is contextual navigation only.
`);
  const rootSourcePath = join(sandbox, "docs", "wiki", "root-source.md");
  writeFileSync(rootSourcePath, `---
type: guide
title: Project-wide evidence
description: Tracks all project-relative changes
status: generated
sources:
  - path: ./
    freshness: tracked
---
# Project-wide evidence
`);
  const validation = run(["wiki", "validate"]);
  assert.equal(validation.kind, "validation_result");
  assert.equal(validation.data.ok, true);
  const unsafePath = join(sandbox, "docs", "wiki", "unsafe.md");
  writeFileSync(unsafePath, `---
type: guide
title: Unsafe evidence
description: Exercises traversal validation
status: generated
sources:
  - path: ../outside.ts
---
# Unsafe evidence
`);
  const unsafeValidation = run(["wiki", "validate"]);
  assert.equal(unsafeValidation.data.ok, false);
  assert.equal(unsafeValidation.data.findings.some((finding) => finding.code === "WIKI_UNSAFE_SOURCE_PATH" && finding.page_id === "unsafe"), true);
  assert.equal(unsafeValidation.data.findings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED" && finding.page_id === "unsafe"), false);
  rmSync(unsafePath);
  assert.equal(run(["wiki", "search", "runtime", "--type", "decision"]).data.count, 1);
  assert.equal(run(["wiki", "catalog"]).data.cataloged, 4);
  const rootAffected = run(["wiki", "affected", "--file", "src/index.ts"]);
  assert.equal(rootAffected.data.items.some((item) => item.id === "root-source"), true);
  assert.match(readFileSync(join(pageDir, "runtime.md"), "utf8"), /status: generated/);
  assert.match(readFileSync(join(sandbox, "docs", "wiki", "index.md"), "utf8"), /\[Runtime\]\(architecture\/runtime\.md\)/);

  execFileSync("git", ["add", "."], { cwd: sandbox });
  execFileSync("git", ["commit", "-qm", "generated Wiki baseline"], { cwd: sandbox });

  assert.equal(run(["wiki", "approve", "architecture/runtime", "decisions/shared-runtime", "references/prd"]).data.approved, 3);
  const approvedRuntime = readFileSync(join(pageDir, "runtime.md"), "utf8");
  const approvedDecision = readFileSync(join(decisionDir, "shared-runtime.md"), "utf8");
  const approvedReference = readFileSync(join(referenceDir, "prd.md"), "utf8");
  assert.match(approvedRuntime, /status: reviewed/);
  assert.match(approvedRuntime, /evidence_hash: sha256:[0-9a-f]{64}/);
  assert.match(approvedDecision, /evidence_hash: sha256:[0-9a-f]{64}/);
  assert.match(approvedReference, /freshness: tracked/);
  assert.match(approvedReference, /freshness: context/);
  assert.match(approvedReference, /evidence_hash: sha256:[0-9a-f]{64}/);
  assert.match(readFileSync(join(sandbox, "docs", "wiki", "index.md"), "utf8"), /architecture\/runtime\.md/);

  const beforeCommitStatus = run(["wiki", "status"]);
  assert.equal(beforeCommitStatus.data.items.find((item) => item.id === "decisions/shared-runtime")?.state, "reviewed");
  execFileSync("git", ["add", "docs/wiki"], { cwd: sandbox });
  execFileSync("git", ["commit", "-qm", "approve Wiki batch"], { cwd: sandbox });

  const approvedStatus = run(["wiki", "status"]);
  assert.equal(approvedStatus.data.items.find((item) => item.id === "decisions/shared-runtime")?.state, "reviewed");
  assert.equal(approvedStatus.data.findings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED" && finding.page_id === "decisions/shared-runtime"), false);
  const approvedValidation = run(["wiki", "validate"]);
  assert.equal(approvedValidation.data.ok, true);
  assert.equal(approvedValidation.data.findings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED" && finding.page_id === "decisions/shared-runtime"), false);

  writeFileSync(join(sandbox, "src", "hub.ts"), "export const hub = false;\n");
  const contextStatus = run(["wiki", "status"]);
  assert.equal(contextStatus.data.items.find((item) => item.id === "references/prd")?.state, "reviewed");
  const contextAffected = run(["wiki", "affected", "--file", "src/hub.ts"]);
  assert.equal(contextAffected.data.items.some((item) => item.id === "references/prd"), false);

  writeFileSync(join(sandbox, "docs", "PRD.md"), "# Product requirements\n\nChanged authoritative intent.\n");
  const prdStatus = run(["wiki", "status"]);
  assert.equal(prdStatus.data.items.find((item) => item.id === "references/prd")?.state, "evidence-changed");
  const prdAffected = run(["wiki", "affected", "--file", "docs/PRD.md"]);
  assert.equal(prdAffected.data.items.some((item) => item.id === "references/prd"), true);

  writeFileSync(join(pageDir, "runtime.md"), `${approvedRuntime}\nSemantic runtime guidance changed after review.\n`);
  const changedStatus = run(["wiki", "status"]);
  assert.equal(changedStatus.data.items.find((item) => item.id === "decisions/shared-runtime")?.state, "evidence-changed");
  const changedValidation = run(["wiki", "validate"]);
  assert.equal(changedValidation.data.findings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED" && finding.page_id === "decisions/shared-runtime"), true);

  const reconfirmed = run(["wiki", "reconfirm", "decisions/shared-runtime"]);
  assert.equal(reconfirmed.kind, "wiki_reconfirm_result");
  assert.equal(reconfirmed.data.reconfirmed, 1);
  assert.deepEqual(reconfirmed.data.pages, ["decisions/shared-runtime"]);
  const reconfirmedStatus = run(["wiki", "status"]);
  assert.equal(reconfirmedStatus.data.items.find((item) => item.id === "decisions/shared-runtime")?.state, "reviewed");
  assert.equal(reconfirmedStatus.data.items.find((item) => item.id === "references/prd")?.state, "evidence-changed");

  const evidenceDir = join(sandbox, "evidence");
  mkdirSync(evidenceDir);
  writeFileSync(join(evidenceDir, "item.txt"), "readable evidence\n");
  const operationalPath = join(sandbox, "docs", "wiki", "operational.md");
  writeFileSync(operationalPath, `---
type: guide
title: Operational evidence
description: Evidence inspection failure coverage
status: generated
sources:
  - path: evidence/item.txt
---
# Operational evidence
`);
  assert.equal(run(["wiki", "approve", "operational"]).data.approved, 1);
  rmSync(evidenceDir, { recursive: true });
  writeFileSync(evidenceDir, "not a directory\n");
  const unreadableStatus = run(["wiki", "status"]);
  assert.equal(unreadableStatus.data.ok, false);
  assert.equal(unreadableStatus.data.items.find((item) => item.id === "operational")?.state, "stale");
  const unreadableFindings = unreadableStatus.data.findings.filter((finding) => finding.page_id === "operational");
  assert.equal(unreadableFindings.some((finding) => finding.code === "WIKI_EVIDENCE_UNREADABLE" && finding.severity === "error"), true);
  assert.equal(unreadableFindings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED"), false);
  const unreadableValidation = run(["wiki", "validate"]);
  assert.equal(unreadableValidation.data.ok, false);
  assert.equal(unreadableValidation.data.findings.some((finding) => finding.code === "WIKI_EVIDENCE_UNREADABLE" && finding.page_id === "operational"), true);
  assert.equal(unreadableValidation.data.findings.some((finding) => finding.code === "WIKI_EVIDENCE_CHANGED" && finding.page_id === "operational"), false);
  console.log("wiki smoke: pass");
} finally {
  rmSync(sandbox, { recursive: true, force: true });
}
