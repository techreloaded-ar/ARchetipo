package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

func TestLifecycleSearchAffectedAndApprove(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src", "auth.go"), []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := `---
type: architecture
title: Authentication
description: Authentication boundaries and token flow
status: generated
sources:
  - path: src/auth.go
---
# Authentication
`
	if err := os.MkdirAll(filepath.Join(root, "architecture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "architecture", "auth.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if !report.OK {
		t.Fatalf("validation failed: %+v", report.Findings)
	}
	items, err := Search(project, root, "token", "", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("search: items=%d err=%v", len(items), err)
	}
	affected, err := Affected(project, root, []string{"src/auth.go"})
	if err != nil || len(affected) != 1 {
		t.Fatalf("affected: items=%d err=%v", len(affected), err)
	}
	approved, err := Approve(project, root, []string{"architecture/auth"})
	if err != nil || approved != 1 {
		t.Fatalf("approve: count=%d err=%v", approved, err)
	}
	loaded, err := Load(root)
	if err != nil || loaded[0].Meta.Status != "reviewed" || loaded[0].Meta.Review == nil {
		t.Fatalf("load after approve: %+v err=%v", loaded, err)
	}
	reset, err := Reset(project, root, []string{"architecture/auth"})
	if err != nil || reset != 1 {
		t.Fatalf("reset: count=%d err=%v", reset, err)
	}
	loaded, err = Load(root)
	if err != nil || loaded[0].Meta.Status != "generated" || loaded[0].Meta.Review != nil {
		t.Fatalf("load after reset: %+v err=%v", loaded, err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSearchIncludesReferenceConcepts(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	reference := `---
type: reference
title: Product vision
description: Original product vision
status: generated
sources:
  - path: docs/Vision.md
    role: original
---
# Product vision

Distinctive roadmap phrase.
`
	if err := os.WriteFile(filepath.Join(root, "references", "vision.md"), []byte(reference), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := Search(project, root, "distinctive roadmap", "reference", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("reference search: items=%+v err=%v", items, err)
	}
	if items[0].Path != "references/vision.md" || items[0].ID != "references/vision" || items[0].Body != "" {
		t.Fatalf("unexpected reference result: %+v", items[0])
	}
}

func TestInitCreatesOnlyReferenceSection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "references")); err != nil || !info.IsDir() {
		t.Fatalf("references section missing or not a directory: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(root, "components")); !os.IsNotExist(err) {
		t.Fatalf("semantic section should not be created before a page needs it: %v", err)
	}
}

func TestConceptIDIsDerivedFromPagePath(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := `---
type: architecture
title: Authentication
description: Authentication boundaries
status: generated
---
# Authentication
`
	if err := os.WriteFile(filepath.Join(root, "architecture-auth.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil || len(pages) != 1 || pages[0].ID != "architecture-auth" {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
}

func TestValidateBrokenLinksAndStaleSources(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := `---
type: domain
title: Billing
description: Billing rules
classification: candidate
status: generated
sources:
  - path: src/missing.go
    role: application
---
# Billing

See [missing concept](/domains/missing.md).
` + requiredSectionBody("domains/billing", "domain") + `
`
	if err := os.MkdirAll(filepath.Join(root, "domains"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "domains", "billing.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	codes := map[string]bool{}
	for _, finding := range report.Findings {
		codes[finding.Code] = true
	}
	if !codes["WIKI_BROKEN_LINK"] || !codes["WIKI_STALE_SOURCE"] {
		t.Fatalf("findings: %+v", report.Findings)
	}
}

func TestValidateWarnsAboutBrokenMarkdownLink(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := `---
type: overview
title: Project overview
description: Project overview and scope
status: generated
---
# Overview

See [missing page](/missing/page.md).
`
	if err := os.WriteFile(filepath.Join(root, "overview.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if !report.OK || !hasFinding(report, "WIKI_BROKEN_LINK") {
		t.Fatalf("expected non-blocking broken link finding: %+v", report.Findings)
	}
}

func TestValidateRejectsMalformedWikiLog(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "log.md"), []byte("# Wiki Log\n\n## Review\n\n- reviewed overview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_LOG_FORMAT") {
		t.Fatalf("expected malformed Wiki log finding: %+v", report.Findings)
	}
}

func TestValidateRejectsModelProtocolArtifacts(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	path := filepath.Join(root, "overview.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n</content>\n</invoke>\n")...)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_PROTOCOL_ARTIFACT") {
		t.Fatalf("expected protocol artifact finding: %+v", report.Findings)
	}
}

func TestValidateRejectsIssuesWrittenInBody(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	path := filepath.Join(root, "overview.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n<!-- archetipo:wiki section=issues -->\n- code: LOST_ISSUE\n  summary: This would not be parsed from frontmatter.\n")...)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_BODY_ISSUES") {
		t.Fatalf("expected body issues finding: %+v", report.Findings)
	}
}

func TestValidateBootstrapCoverage(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src", "index.ts"), []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "package.json", "")
	writeCorePage(t, root, "architecture/context-map", "context-map", "src/index.ts", "")
	writeCorePage(t, root, "operations/development", "operations", "package.json", "")
	coverage := `coverage:
  - kind: boundary
    path: .
    status: mapped
    pages: [overview]
  - kind: boundary
    path: src
    status: mapped
    pages: [architecture/context-map]
`
	writeCorePage(t, root, "engineering/code-map", "code-map", "src", coverage)

	report, err := ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("bootstrap should be valid: %+v", report.Findings)
	}

	path := filepath.Join(root, "engineering", "code-map.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "  - kind: boundary\n    path: src\n    status: mapped\n    pages: [architecture/context-map]\n", "", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasFinding(report, "WIKI_UNCOVERED_BOUNDARY") {
		t.Fatalf("expected uncovered boundary: %+v", report.Findings)
	}
}

func TestValidateBootstrapRequiresCorePages(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasFinding(report, "WIKI_BOOTSTRAP_PAGE_MISSING") {
		t.Fatalf("expected missing core page findings: %+v", report.Findings)
	}
}

func TestValidateBootstrapRejectsOrphanCorePage(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	writeCorePage(t, root, "architecture/context-map", "context-map", "README.md", "")
	writeCorePage(t, root, "engineering/code-map", "code-map", "README.md", "")
	writeCorePage(t, root, "operations/development", "operations", "README.md", "")

	path := filepath.Join(root, "operations", "development.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "\n## Related concepts\n\nSee [overview](/overview.md).\n", "", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasFinding(report, "WIKI_BOOTSTRAP_CORE_ORPHAN") {
		t.Fatalf("expected orphan core page finding: %+v", report.Findings)
	}
}

func TestValidateBootstrapRejectsUnreviewedBoundedContext(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "domains/trips", "domain", "README.md", "classification: bounded-context\n")
	report, err := ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasFinding(report, "WIKI_BOOTSTRAP_BOUNDARY_UNREVIEWED") {
		t.Fatalf("expected unreviewed boundary finding: %+v", report.Findings)
	}
	if _, err := Approve(project, root, []string{"domains/trips"}); err != nil {
		t.Fatal(err)
	}
	report, err = ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if hasFinding(report, "WIKI_BOOTSTRAP_BOUNDARY_UNREVIEWED") {
		t.Fatalf("reviewed bounded context should pass the semantic-review gate: %+v", report.Findings)
	}
}

func TestValidateBootstrapRequiresExistingCoreEvidence(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "missing.md", "")
	writeCorePage(t, root, "architecture/context-map", "context-map", "README.md", "")
	writeCorePage(t, root, "operations/development", "operations", "README.md", "")
	writeCorePage(t, root, "engineering/code-map", "code-map", "README.md", "")
	report, err := ValidateBootstrap(project, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasFinding(report, "WIKI_BOOTSTRAP_SOURCE_MISSING") {
		t.Fatalf("expected missing concrete evidence: %+v", report.Findings)
	}
}

func TestValidateCoverageExclusionRequiresNote(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	extra := "coverage:\n  - kind: boundary\n    path: legacy\n    status: excluded\n"
	writeCorePage(t, root, "engineering/code-map", "code-map", "README.md", extra)
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_INVALID_COVERAGE") {
		t.Fatalf("expected invalid coverage finding: %+v", report.Findings)
	}

	path := filepath.Join(root, "engineering", "code-map.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "    status: excluded\n", "    status: excluded\n    note: Legacy code is intentionally outside the maintained product.\n", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report = Validate(project, root)
	if !report.OK {
		t.Fatalf("motivated exclusion should be valid: %+v", report.Findings)
	}
}

func TestCatalogPreservesGeneratedStatus(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	if _, err := Catalog(project, root); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "* [overview](overview.md) - overview description. _State: generated._") {
		t.Fatalf("catalog should preserve generated state:\n%s", index)
	}
}

func TestDomainPagesRequireDDDClassificationAndSections(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "domains/trips", "domain", "README.md", "classification: candidate\n")
	report := Validate(project, root)
	if !report.OK {
		t.Fatalf("complete candidate domain should validate: %+v", report.Findings)
	}

	path := filepath.Join(root, "domains", "trips.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "<!-- archetipo:wiki section=ownership -->", "", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report = Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_DDD_SECTION_MISSING") {
		t.Fatalf("expected missing DDD section: %+v", report.Findings)
	}
}

func TestDomainPagesRequireRepositoryEvidence(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "domains/trips", "domain", "README.md", "classification: candidate\n")
	path := filepath.Join(root, "domains", "trips.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "sources:\n  - path: README.md\n    role: application\n", "", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_DOMAIN_SOURCE_MISSING") {
		t.Fatalf("expected missing domain evidence: %+v", report.Findings)
	}
}

func TestDecisionPagesRequireLifecycleEvidenceAndSections(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "decisions/shared-rate-limit-store", "decision", "README.md", "decision_status: accepted\n")
	report := Validate(project, root)
	if !report.OK {
		t.Fatalf("complete decision should validate: %+v", report.Findings)
	}

	path := filepath.Join(root, "decisions", "shared-rate-limit-store.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "decision_status: accepted\n", "decision_status: proposed\n", 1))
	raw = []byte(strings.Replace(string(raw), "sources:\n  - path: README.md\n    role: application\n", "", 1))
	raw = []byte(strings.Replace(string(raw), "<!-- archetipo:wiki section=alternatives -->", "", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report = Validate(project, root)
	for _, code := range []string{"WIKI_INVALID_DECISION_STATUS", "WIKI_DECISION_SOURCE_MISSING", "WIKI_DDD_SECTION_MISSING"} {
		if !hasFinding(report, code) {
			t.Fatalf("expected %s: %+v", code, report.Findings)
		}
	}
}

func TestValidateBootstrapRequiresConfiguredSourceReference(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "Vision.MD"), []byte("# Intent"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	writeCorePage(t, root, "architecture/context-map", "context-map", "README.md", "")
	writeCorePage(t, root, "operations/development", "operations", "README.md", "")
	writeCorePage(t, root, "engineering/code-map", "code-map", "README.md", "coverage:\n  - kind: boundary\n    path: .\n    status: mapped\n    pages: [overview]\n")

	report, err := ValidateBootstrap(project, root, "docs/Vision.MD")
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || !hasFinding(report, "WIKI_PROJECT_REFERENCE_MISSING") {
		t.Fatalf("expected missing source reference: %+v", report.Findings)
	}
	reference := `---
type: reference
title: Product vision
description: Original product vision
status: generated
sources:
  - path: docs/Vision.MD
    role: original
---
# Product vision

# Intent
`
	if err := os.WriteFile(filepath.Join(root, "references", "vision.md"), []byte(reference), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = ValidateBootstrap(project, root, "docs/Vision.MD")
	if err != nil || !report.OK {
		t.Fatalf("reference concept should validate: report=%+v err=%v", report, err)
	}
}

func TestPageStateDerivesAttentionAndStale(t *testing.T) {
	page := Page{Meta: domain.WikiPageMeta{Status: domain.WikiStatusGenerated, Issues: []domain.WikiIssue{{Code: "CONFLICT", Summary: "Code and intent differ"}}}, Body: "body"}
	if state := PageState(t.TempDir(), page); state != "attention" {
		t.Fatalf("state=%s", state)
	}
	page.Meta.Issues = nil
	page.Meta.Status = domain.WikiStatusReviewed
	page.Meta.Review = &domain.WikiReview{ContentHash: "sha256:old", EvidenceRevision: "unavailable", ReviewedAt: "2026-07-13T00:00:00Z"}
	if state := PageState(t.TempDir(), page); state != "stale" {
		t.Fatalf("state=%s", state)
	}
}

func TestPageStateBecomesStaleWhenSemanticMetadataChanges(t *testing.T) {
	page := Page{
		ID: "overview",
		Meta: domain.WikiPageMeta{
			Type:        "overview",
			Title:       "Overview",
			Description: "Original description",
			Status:      domain.WikiStatusReviewed,
		},
		Body: "# Overview\n",
	}
	page.Meta.Review = &domain.WikiReview{
		ContentHash:      pageContentHash(page),
		EvidenceRevision: "unavailable",
		ReviewedAt:       "2026-07-13T00:00:00Z",
	}
	if state := PageState(t.TempDir(), page); state != "reviewed" {
		t.Fatalf("state before metadata change=%s", state)
	}
	page.Meta.Description = "Changed description"
	if state := PageState(t.TempDir(), page); state != "stale" {
		t.Fatalf("state after metadata change=%s", state)
	}
	page.Meta.Description = "Original description"
	page.Meta.Review.ContentHash = pageContentHash(page)
	page.ID = "renamed-overview"
	if state := PageState(t.TempDir(), page); state != "stale" {
		t.Fatalf("state after identity change=%s", state)
	}
}

func TestApprovedPageBecomesStaleWhenEvidenceChanges(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "add", "README.md")
	git(t, project, "commit", "-qm", "baseline")
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil || PageState(project, pages[0]) != "reviewed" {
		t.Fatalf("expected reviewed page: %+v err=%v", pages, err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, pages[0]); state != "stale" {
		t.Fatalf("state=%s", state)
	}
}

func TestApproveRejectsUnresolvedIssues(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "issues:\n  - code: OPEN_BOUNDARY\n    summary: Ownership is unresolved\n")
	if _, err := Approve(project, root, []string{"overview"}); err == nil || !strings.Contains(err.Error(), "unresolved issues") {
		t.Fatalf("expected unresolved issue conflict, got %v", err)
	}
}

func TestFrontmatterSupportsCRLFAndDerivesConceptID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "architecture"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\r\ntype: architecture\r\ntitle: Runtime\r\ndescription: Runtime boundaries\r\nstatus: generated\r\n---\r\n# Runtime\r\n"
	if err := os.WriteFile(filepath.Join(root, "architecture", "runtime.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil || len(pages) != 1 || pages[0].ID != "architecture/runtime" {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
}

func TestApproveAndResetPreserveUnknownFrontmatter(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	raw := `---
type: overview
title: Project overview
description: Project scope
status: generated
owner:
  team: platform
---
# Overview
`
	if err := os.WriteFile(filepath.Join(root, "overview.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reset(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(filepath.Join(root, "overview.md"))
	if err != nil || !strings.Contains(string(persisted), "owner:\n    team: platform") {
		t.Fatalf("unknown metadata was not preserved:\n%s\nerr=%v", persisted, err)
	}
}

func TestLogGroupsNewestEntriesByDate(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	if _, err := Catalog(project, root); err != nil {
		t.Fatal(err)
	}
	if _, err := Catalog(project, root); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format(time.DateOnly)
	if strings.Count(string(raw), "## "+today) != 1 || strings.Count(string(raw), "* **Update**:") != 2 {
		t.Fatalf("unexpected grouped log:\n%s", raw)
	}
}

func writeCorePage(t *testing.T, root, id, pageType, source, extra string) {
	t.Helper()
	rel := id + ".md"
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	relationship := "\n## Related concepts\n\nSee [overview](/overview.md).\n"
	if id == "overview" {
		relationship = "\n## Related concepts\n\nSee [context map](/architecture/context-map.md).\n"
	}
	body := "---\ntype: " + pageType + "\ntitle: " + id + "\ndescription: " + id + " description\nstatus: generated\nsources:\n  - path: " + source + "\n    role: application\n" + extra + "---\n# " + id + "\n" + requiredSectionBody(id, pageType) + relationship
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requiredSectionBody(id, pageType string) string {
	page := Page{ID: id, Meta: domain.WikiPageMeta{Type: pageType}}
	var body strings.Builder
	for _, section := range requiredSectionsForPage(page) {
		body.WriteString("\n<!-- archetipo:wiki section=" + section + " -->\nContent for " + section + ".\n")
	}
	return body.String()
}

func hasFinding(report Report, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
