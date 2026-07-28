package wiki

import (
	"errors"
	"os"
	"os/exec"
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
	if state := PageState(t.TempDir(), t.TempDir(), page); state != "attention" {
		t.Fatalf("state=%s", state)
	}
	page.Meta.Issues = nil
	page.Meta.Status = domain.WikiStatusReviewed
	page.Meta.Review = &domain.WikiReview{ContentHash: "sha256:old", EvidenceRevision: "unavailable", ReviewedAt: "2026-07-13T00:00:00Z"}
	if state := PageState(t.TempDir(), t.TempDir(), page); state != "stale" {
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
	if state := PageState(t.TempDir(), t.TempDir(), page); state != "reviewed" {
		t.Fatalf("state before metadata change=%s", state)
	}
	page.Meta.Description = "Changed description"
	if state := PageState(t.TempDir(), t.TempDir(), page); state != "stale" {
		t.Fatalf("state after metadata change=%s", state)
	}
	page.Meta.Description = "Original description"
	page.Meta.Review.ContentHash = pageContentHash(page)
	page.ID = "renamed-overview"
	if state := PageState(t.TempDir(), t.TempDir(), page); state != "stale" {
		t.Fatalf("state after identity change=%s", state)
	}
}

func TestSourceFreshnessControlsEvidenceAndAffected(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"docs/PRD.md":    "authoritative product intent\n",
		"src/hub.ts":     "export const hub = true;\n",
		"src/omitted.ts": "export const omitted = true;\n",
		"src/tracked.ts": "export const tracked = true;\n",
	} {
		if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSimplePage(t, root, "omitted", "src/omitted.ts")
	tracked := `---
type: guide
title: tracked
description: tracked description
status: generated
sources:
  - path: src/tracked.ts
    freshness: tracked
---
# tracked
`
	if err := os.WriteFile(filepath.Join(root, "tracked.md"), []byte(tracked), 0o644); err != nil {
		t.Fatal(err)
	}
	reference := `---
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
`
	if err := os.WriteFile(filepath.Join(root, "references", "prd.md"), []byte(reference), 0o644); err != nil {
		t.Fatal(err)
	}
	if approved, err := Approve(project, root, []string{"omitted", "tracked", "references/prd"}); err != nil || approved != 3 {
		t.Fatalf("approve source relevance pages: approved=%d err=%v", approved, err)
	}

	referencePath := filepath.Join(root, "references", "prd.md")
	approvedReference, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(approvedReference), "freshness: context") || !strings.Contains(string(approvedReference), "freshness: tracked") {
		t.Fatalf("source freshness was not serialized:\n%s", approvedReference)
	}
	changedFreshness := strings.Replace(string(approvedReference), "freshness: context", "freshness: tracked", 1)
	if err := os.WriteFile(referencePath, []byte(changedFreshness), 0o644); err != nil {
		t.Fatal(err)
	}
	changedPages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pagesByID(changedPages)["references/prd"]); state != "stale" {
		t.Fatalf("changing source freshness did not invalidate content review: %s", state)
	}
	if report := Validate(project, root); !hasFinding(report, "WIKI_REVIEW_OUTDATED") {
		t.Fatalf("changing source freshness did not produce an outdated review: %+v", report.Findings)
	}
	if err := os.WriteFile(referencePath, approvedReference, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "src", "hub.ts"), []byte("export const hub = false;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pagesByID(pages)["references/prd"]); state != "reviewed" {
		t.Fatalf("context source changed reference state: %s", state)
	}
	affected, err := Affected(project, root, []string{"src/hub.ts"})
	if err != nil || len(affected) != 0 {
		t.Fatalf("context source appeared in affected: pages=%+v err=%v", affected, err)
	}

	if err := os.WriteFile(filepath.Join(project, "docs", "PRD.md"), []byte("changed product intent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pagesByID(pages)["references/prd"]); state != "stale" {
		t.Fatalf("authoritative PRD change did not stale reference: %s", state)
	}
	affected, err = Affected(project, root, []string{"docs/PRD.md"})
	if err != nil || len(affected) != 1 || affected[0].ID != "references/prd" {
		t.Fatalf("authoritative PRD was not affected: pages=%+v err=%v", affected, err)
	}

	for _, id := range []string{"omitted", "tracked"} {
		path := "src/" + id + ".ts"
		affected, err = Affected(project, root, []string{path})
		if err != nil || len(affected) != 1 || affected[0].ID != id {
			t.Fatalf("%s freshness affected mismatch: pages=%+v err=%v", id, affected, err)
		}
		if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(path)), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"omitted", "tracked"} {
		if state := PageState(project, root, pagesByID(pages)[id]); state != "stale" {
			t.Fatalf("%s source did not retain tracked compatibility: %s", id, state)
		}
	}
}

func TestAffectedNormalizesProjectRelativePaths(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writePage := func(id, source, freshness string) {
		t.Helper()
		page := Page{
			ID:   id,
			Path: id + ".md",
			Meta: domain.WikiPageMeta{
				Type:        "guide",
				Title:       id,
				Description: id + " description",
				Status:      domain.WikiStatusGenerated,
				Sources:     []domain.WikiSource{{Path: source, Freshness: freshness}},
			},
			Body: "# " + id + "\n",
		}
		raw, err := renderPage(page)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, page.Path), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writePage("root", ".", "")
	writePage("root-slash", "./", "tracked")
	writePage("src", "src", "tracked")
	writePage("context-root", ".", "context")
	writePage("external", "https://example.test/evidence", "tracked")

	tests := []struct {
		name      string
		files     []string
		want      []string
		wantError bool
	}{
		{name: "root and directory", files: []string{"src/a.go"}, want: []string{"root", "root-slash", "src"}},
		{name: "normalized changed path", files: []string{"./src/a.go"}, want: []string{"root", "root-slash", "src"}},
		{name: "component boundary", files: []string{"src2/a.go"}, want: []string{"root", "root-slash"}},
		{name: "empty changed path", files: []string{""}, wantError: true},
		{name: "outside changed path", files: []string{"../outside/a.go"}, wantError: true},
		{name: "absolute changed path", files: []string{filepath.Join(project, "src", "a.go")}, wantError: true},
		{name: "external changed path", files: []string{"https://example.test/src/a.go"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pages, err := Affected(project, root, test.files)
			if test.wantError {
				if !errors.Is(err, ErrInvalidChangedFile) {
					t.Fatalf("Affected(%v) error=%v", test.files, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(pages))
			for _, page := range pages {
				got = append(got, page.ID)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("Affected(%v)=%v want=%v", test.files, got, test.want)
			}
		})
	}
}

func TestAffectedFailsClosedOnInvalidPersistedSource(t *testing.T) {
	for _, source := range []string{"", "../outside"} {
		t.Run(source, func(t *testing.T) {
			project := t.TempDir()
			root := filepath.Join(project, "docs", "wiki")
			if _, err := Init(root); err != nil {
				t.Fatal(err)
			}
			writeSimplePage(t, root, "invalid", source)
			if _, err := Affected(project, root, []string{"src/item.go"}); err == nil || (!errors.Is(err, ErrInvalidSourcePath) && !errors.Is(err, ErrUnsafeSourcePath)) {
				t.Fatalf("invalid persisted source error=%v", err)
			}
		})
	}
}

func TestAffectedValidatesLaterPersistedSourceAfterEarlierMatch(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := Page{
		ID:   "mixed",
		Path: "mixed.md",
		Meta: domain.WikiPageMeta{
			Type:        "guide",
			Title:       "mixed",
			Description: "mixed description",
			Status:      domain.WikiStatusGenerated,
			Sources: []domain.WikiSource{
				{Path: "src/item.go"},
				{Path: "../outside"},
			},
		},
		Body: "# mixed\n",
	}
	raw, err := renderPage(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, page.Path), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := Affected(project, root, []string{"src/item.go"})
	if !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("later invalid source error=%v items=%v", err, items)
	}
	if items != nil {
		t.Fatalf("persisted source failure returned partial items: %v", items)
	}
}

func TestGitChangedFilesPreservesNamesAndBothRenameSides(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "config", "commit.gpgSign", "false")
	if err := os.MkdirAll(filepath.Join(project, "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"evidence/name with spaces.txt": "before\n",
		"evidence/old-name.txt":         "renamed\n",
	} {
		if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, project, "add", ".")
	git(t, project, "commit", "-qm", "baseline")
	base := gitRevision(project)

	if err := os.WriteFile(filepath.Join(project, "evidence", "name with spaces.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(project, "evidence", "old-name.txt"), filepath.Join(project, "evidence", "new-name.txt")); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", "-A")
	git(t, project, "commit", "-qm", "change and rename")

	changed, err := GitChangedFiles(project, base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"evidence/name with spaces.txt", "evidence/new-name.txt", "evidence/old-name.txt"}
	if strings.Join(changed, "\n") != strings.Join(want, "\n") {
		t.Fatalf("GitChangedFiles=%q want=%q", changed, want)
	}

	writeSimplePage(t, root, "spaces", "evidence/name with spaces.txt")
	writeSimplePage(t, root, "old", "evidence/old-name.txt")
	writeSimplePage(t, root, "new", "evidence/new-name.txt")
	affected, err := Affected(project, root, changed)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]string, 0, len(affected))
	for _, page := range affected {
		gotIDs = append(gotIDs, page.ID)
	}
	if got, expected := strings.Join(gotIDs, ","), "new,old,spaces"; got != expected {
		t.Fatalf("Affected IDs=%s want=%s", got, expected)
	}
}

func TestGitChangedFilesNULParserPreservesControlCharacters(t *testing.T) {
	raw := []byte("name with spaces\x00name\twith-tab\x00name\nwith-newline\x00Unicode-è.txt\x00")
	got := parseNULPathList(raw)
	want := []string{"name with spaces", "name\twith-tab", "name\nwith-newline", "Unicode-è.txt"}
	if len(got) != len(want) {
		t.Fatalf("parseNULPathList=%q want=%q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("parseNULPathList[%d]=%q want=%q", index, got[index], want[index])
		}
	}
}

func TestValidateRejectsInvalidSourceFreshnessAndChecksContextPaths(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "existing.md"), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `---
type: guide
title: Invalid freshness
description: Invalid freshness value
status: generated
sources:
  - path: existing.md
    freshness: sometimes
  - path: missing-context.md
    freshness: context
---
# Invalid freshness
`
	if err := os.WriteFile(filepath.Join(root, "invalid.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_INVALID_SOURCE_FRESHNESS") {
		t.Fatalf("invalid source freshness validated: %+v", report.Findings)
	}
	if !hasFinding(report, "WIKI_STALE_SOURCE") {
		t.Fatalf("invalid/context source skipped existing path validation: %+v", report.Findings)
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
	if err != nil || PageState(project, root, pages[0]) != "reviewed" {
		t.Fatalf("expected reviewed page: %+v err=%v", pages, err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pages[0]); state != "stale" {
		t.Fatalf("state=%s", state)
	}
}

func TestApproveKeepsSameBatchWikiEvidenceFreshAcrossCommit(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "guides"), 0o755); err != nil {
		t.Fatal(err)
	}
	guidePath := filepath.Join(root, "guides", "runtime.md")
	guide := `---
type: guide
title: Runtime guide
description: Runtime operating guide
status: generated
sources:
  - path: README.md
---
# Runtime guide

Original guidance.

See [decision](/decisions/runtime.md).
`
	if err := os.WriteFile(guidePath, []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "decisions"), 0o755); err != nil {
		t.Fatal(err)
	}
	decision := `---
type: decision
title: Runtime decision
description: Runtime architecture decision
decision_status: accepted
status: generated
sources:
  - path: docs/wiki/guides/runtime.md
---
# Runtime decision
` + requiredSectionBody("decisions/runtime", "decision") + `
See [runtime guide](/guides/runtime.md).
`
	if err := os.WriteFile(filepath.Join(root, "decisions", "runtime.md"), []byte(decision), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "add", ".")
	git(t, project, "commit", "-qm", "generated baseline")

	guide = strings.Replace(guide, "Original guidance.", "Semantically updated guidance.", 1)
	if err := os.WriteFile(guidePath, []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	if approved, err := Approve(project, root, []string{"guides/runtime", "decisions/runtime"}); err != nil || approved != 2 {
		t.Fatalf("same-batch approve: approved=%d err=%v", approved, err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if state := PageState(project, root, page); state != "reviewed" {
			t.Fatalf("%s state before commit=%s", page.ID, state)
		}
		if page.Meta.Review == nil || !wikiContentHashPattern.MatchString(page.Meta.Review.EvidenceHash) {
			t.Fatalf("%s missing evidence hash: %+v", page.ID, page.Meta.Review)
		}
	}
	approvedGuide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", "docs/wiki")
	git(t, project, "commit", "-qm", "approve Wiki batch")

	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if state := PageState(project, root, page); state != "reviewed" {
			t.Fatalf("%s state after commit=%s", page.ID, state)
		}
	}
	if report := Validate(project, root); hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("same-batch approval became stale after commit: %+v", report.Findings)
	}

	if err := os.WriteFile(guidePath, append(approvedGuide, []byte("\nSemantic change after review.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	byID := pagesByID(pages)
	if state := PageState(project, root, byID["decisions/runtime"]); state != "stale" {
		t.Fatalf("decision did not detect semantic Wiki evidence change: %s", state)
	}

	if err := os.WriteFile(guidePath, approvedGuide, 0o644); err != nil {
		t.Fatal(err)
	}
	if reset, err := Reset(project, root, []string{"guides/runtime"}); err != nil || reset != 1 {
		t.Fatalf("reset guide: reset=%d err=%v", reset, err)
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pagesByID(pages)["decisions/runtime"]); state != "reviewed" {
		t.Fatalf("decision stale after lifecycle-only reset: %s", state)
	}
	if approved, err := Approve(project, root, []string{"guides/runtime"}); err != nil || approved != 1 {
		t.Fatalf("reapprove guide: approved=%d err=%v", approved, err)
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pagesByID(pages)["decisions/runtime"]); state != "reviewed" {
		t.Fatalf("decision stale after lifecycle-only reapproval: %s", state)
	}
}

func TestEvidenceHashCapturesDirtyUntrackedAndDeletedFiles(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "dirty.txt"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "delete.txt"), []byte("delete later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "add", "evidence")
	git(t, project, "commit", "-qm", "evidence baseline")
	if err := os.WriteFile(filepath.Join(project, "evidence", "dirty.txt"), []byte("dirty at review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "untracked.txt"), []byte("untracked at review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "dirty", "evidence/dirty.txt")
	writeSimplePage(t, root, "untracked", "evidence/untracked.txt")
	writeSimplePage(t, root, "deleted", "evidence/delete.txt")
	if approved, err := Approve(project, root, []string{"dirty", "untracked", "deleted"}); err != nil || approved != 3 {
		t.Fatalf("approve working-tree evidence: approved=%d err=%v", approved, err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if state := PageState(project, root, page); state != "reviewed" {
			t.Fatalf("%s was stale immediately after approval: %s", page.ID, state)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "dirty.txt"), []byte("dirty changed again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "untracked.txt"), []byte("untracked changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(project, "evidence", "delete.txt")); err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if state := PageState(project, root, page); state != "stale" {
			t.Fatalf("%s did not detect changed working-tree evidence: %s", page.ID, state)
		}
	}
}

func TestGitChangedFilesNestedProjectRootIsLocalAndRelative(t *testing.T) {
	repository := t.TempDir()
	project := filepath.Join(repository, "apps", "service")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repository, "init", "-q")
	git(t, repository, "config", "user.email", "wiki-test@example.test")
	git(t, repository, "config", "user.name", "Wiki Test")
	for file, content := range map[string]string{"apps/service/item.txt": "before\n", "outside.txt": "before\n"} {
		path := filepath.Join(repository, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repository, "add", ".")
	git(t, repository, "commit", "-qm", "baseline")
	base := gitRevision(repository)
	if err := os.WriteFile(filepath.Join(project, "item.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "outside.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repository, "add", ".")
	git(t, repository, "commit", "-qm", "changes")
	changed, err := GitChangedFiles(project, base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(changed, ",") != "item.txt" {
		t.Fatalf("nested changed files=%q", changed)
	}
}

func TestExternalSourceClassification(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		external bool
	}{
		{name: "HTTP", source: "http://example.test/evidence", external: true},
		{name: "HTTPS", source: "https://example.test/evidence", external: true},
		{name: "uppercase scheme", source: "HTTPS://example.test/evidence", external: true},
		{name: "hostless HTTP", source: "https:///evidence"},
		{name: "port-only authority", source: "https://:443/evidence"},
		{name: "IPv4 with port", source: "https://127.0.0.1:8443/evidence", external: true},
		{name: "IPv6 with port", source: "https://[::1]:8443/evidence", external: true},
		{name: "malformed HTTP", source: "https://[invalid"},
		{name: "unknown scheme", source: "ftp://example.test/evidence"},
		{name: "Windows-looking path", source: "C://evidence/file.txt"},
		{name: "traversal containing delimiter", source: "../outside://secret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isExternal(test.source); got != test.external {
				t.Fatalf("isExternal(%q)=%v want=%v", test.source, got, test.external)
			}
		})
	}
}

func TestExternalSourcesRemainOptionalWhileURILookingPathsAreValidated(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "external", "HTTPS://example.test/evidence")
	if report := Validate(project, root); !report.OK {
		t.Fatalf("valid external source failed validation: %+v", report.Findings)
	}

	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"../outside://secret", "C://evidence/file.txt", "ftp://example.test/evidence", "https:///missing-host"} {
		t.Run(source, func(t *testing.T) {
			page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: source}}}}
			if isExternal(source) {
				t.Fatalf("URI-looking local source classified as external: %q", source)
			}
			missing, missingErr := pageHasMissingEvidence(resolver, page)
			if missingErr == nil && !missing {
				t.Fatalf("URI-looking local source bypassed path checks: source=%q", source)
			}
		})
	}
	writeSimplePage(t, root, "malformed-http", "https:///missing-host")
	report := Validate(project, root)
	if !hasFinding(report, "WIKI_INVALID_SOURCE") {
		t.Fatalf("malformed HTTP source bypassed portable local-source validation: %+v", report.Findings)
	}
	if _, err := Approve(project, root, []string{"malformed-http"}); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("malformed HTTP source bypassed approval: %v", err)
	}
}

func TestSourcePathResolverRejectsLexicalEscapesAndPreservesMissingEvidence(t *testing.T) {
	project := t.TempDir()
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"../outside", "nested/../../outside", `..\outside`} {
		t.Run(source, func(t *testing.T) {
			if _, err := resolver.resolve(source); !errors.Is(err, ErrUnsafeSourcePath) {
				t.Fatalf("resolve(%q) error=%v", source, err)
			}
		})
	}
	for _, source := range []string{"/absolute", `C:\outside`, `\\server\share\outside`} {
		t.Run(source, func(t *testing.T) {
			if _, err := resolver.resolve(source); !errors.Is(err, ErrInvalidSourcePath) {
				t.Fatalf("resolve(%q) error=%v", source, err)
			}
		})
	}
	missing, err := resolver.resolve("missing/terminal.txt")
	if err != nil || missing.Exists || missing.Relative != "missing/terminal.txt" {
		t.Fatalf("missing terminal resolution=%+v err=%v", missing, err)
	}
	root, err := resolver.resolve("./")
	if err != nil || !root.Exists || !root.Info.IsDir() || root.Relative != "." {
		t.Fatalf("root resolution=%+v err=%v", root, err)
	}
}

func TestSourcePathResolverSymlink(t *testing.T) {
	t.Run("real symlink integration", testSourcePathResolverRealSymlinkIntegration)
}

func testSourcePathResolverRealSymlinkIntegration(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, "redirect")); err != nil {
		if symlinkCapabilityUnavailable(err) {
			t.Skipf("real symlink capability explicitly unavailable: %v", err)
		}
		t.Fatalf("creating intermediate symlink failed unexpectedly: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(project, "terminal")); err != nil {
		t.Fatal(err)
	}

	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("redirect/outside.txt"); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("intermediate symlink error=%v", err)
	}
	terminal, err := resolver.resolve("terminal")
	if err != nil || !terminal.Exists || terminal.Info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("terminal symlink resolution=%+v err=%v", terminal, err)
	}
	missing, err := pageHasMissingEvidence(resolver, Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "terminal"}}}})
	if err != nil || missing {
		t.Fatalf("terminal symlink was treated as missing: missing=%v err=%v", missing, err)
	}

	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "unsafe", "redirect/outside.txt")
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_UNSAFE_SOURCE_PATH") {
		t.Fatalf("unsafe source validated: %+v", report.Findings)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evidenceFingerprint(project, root, pages[0]); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("fingerprinting unsafe source error=%v", err)
	}
	if _, err := pageHasMissingEvidence(resolver, pages[0]); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("missing-evidence check unsafe source error=%v", err)
	}

	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "terminal"}}}}
	before, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("changed outside content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := evidenceFingerprint(project, root, page)
	if err != nil || after != before {
		t.Fatalf("terminal symlink followed its target: before=%s after=%s err=%v", before, after, err)
	}

	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "project-alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	aliasResolver, err := newEvidencePathResolver(alias)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := aliasResolver.resolve(".")
	physicalProject, evalErr := filepath.EvalSymlinks(project)
	if err != nil || evalErr != nil || resolvedRoot.Path != physicalProject {
		t.Fatalf("root alias did not resolve to trusted anchor: resolved=%+v physical=%s err=%v evalErr=%v", resolvedRoot, physicalProject, err, evalErr)
	}
}

func TestMissingEvidenceUsesTerminalLstat(t *testing.T) {
	project := t.TempDir()
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "missing.txt"}}}}
	missing, err := pageHasMissingEvidence(resolver, page)
	if err != nil || !missing {
		t.Fatalf("missing evidence result=%v err=%v", missing, err)
	}
}

func TestEvidenceFingerprintDirectoryModeAndMissingDeterminism(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(project, "evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.sh"), []byte("echo a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence"}}}}
	first, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	second, err := evidenceFingerprint(project, root, page)
	if err != nil || second != first {
		t.Fatalf("directory fingerprint is not deterministic: first=%s second=%s err=%v", first, second, err)
	}
	missingPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "missing.txt"}}}}
	missingBefore, err := evidenceFingerprint(project, root, missingPage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "missing.txt"), []byte("present\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	present, err := evidenceFingerprint(project, root, missingPage)
	if err != nil || present == missingBefore {
		t.Fatalf("missing and regular entries collided: missing=%s present=%s err=%v", missingBefore, present, err)
	}
	if err := os.Remove(filepath.Join(project, "missing.txt")); err != nil {
		t.Fatal(err)
	}
	missingAfter, err := evidenceFingerprint(project, root, missingPage)
	if err != nil || missingAfter != missingBefore {
		t.Fatalf("missing entry fingerprint is not deterministic: before=%s after=%s err=%v", missingBefore, missingAfter, err)
	}

	wikiDirectoryPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "docs/wiki"}}}}
	reservedBefore, err := evidenceFingerprint(project, root, wikiDirectoryPage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.md"), []byte("generated catalog changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "log.md"), []byte("generated log changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reservedAfter, err := evidenceFingerprint(project, root, wikiDirectoryPage)
	if err != nil || reservedAfter != reservedBefore {
		t.Fatalf("indirect reserved Wiki artifacts affected fingerprint: before=%s after=%s err=%v", reservedBefore, reservedAfter, err)
	}
}

func TestGitIndexParserIsNULSafeAndRejectsUnmergedStages(t *testing.T) {
	oid := strings.Repeat("a", 64)
	path := "dir/name\twith\ncontrols.txt"
	entries, err := parseGitIndex([]byte("100755 " + oid + " 0\t" + path + "\x00"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("parseGitIndex entries=%+v err=%v", entries, err)
	}
	entry := entries[0]
	if entry.Mode != "100755" || entry.OID != oid || entry.Stage != 0 || entry.Path != path {
		t.Fatalf("parsed index entry=%+v", entry)
	}
	index := &gitIndex{byPath: map[string][]gitIndexEntry{
		"conflicted.txt": {
			{Mode: "100644", OID: strings.Repeat("b", 40), Stage: 1, Path: "conflicted.txt"},
			{Mode: "100644", OID: strings.Repeat("c", 40), Stage: 2, Path: "conflicted.txt"},
		},
		"modules/component": {
			{Mode: "160000", OID: strings.Repeat("d", 40), Stage: 0, Path: "modules/component"},
		},
		"broken/ancestor": {
			{Mode: "160000", OID: strings.Repeat("e", 40), Stage: 2, Path: "broken/ancestor"},
		},
	}}
	if _, err := index.entry("conflicted.txt"); !errors.Is(err, ErrGitIndexConflict) {
		t.Fatalf("unmerged index error=%v", err)
	}
	if _, err := index.pathsWithin("."); !errors.Is(err, ErrGitIndexConflict) {
		t.Fatalf("parent directory hid unmerged index error=%v", err)
	}
	ancestor, err := index.strictGitlinkAncestor("modules/component/file.txt")
	if err != nil || ancestor == nil || ancestor.Path != "modules/component" {
		t.Fatalf("strict gitlink ancestor=%+v err=%v", ancestor, err)
	}
	if ancestor, err := index.strictGitlinkAncestor("modules/component-extra/file.txt"); err != nil || ancestor != nil {
		t.Fatalf("component-prefix lookalike matched gitlink: ancestor=%+v err=%v", ancestor, err)
	}
	if _, err := index.strictGitlinkAncestor("broken/ancestor/file.txt"); !errors.Is(err, ErrGitIndexConflict) {
		t.Fatalf("unmerged ancestor was not fail-closed: %v", err)
	}
}

func TestTrackedRegularEvidenceUsesGitCleanIdentity(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "config", "commit.gpgSign", "false")
	git(t, project, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(project, ".gitattributes"), []byte("evidence.txt text eol=lf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(project, "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("working\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", ".gitattributes", "evidence.txt")
	git(t, project, "commit", "-qm", "tracked evidence")
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence.txt"}}}}

	baseline, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("working\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	crlf, err := evidenceFingerprint(project, root, page)
	if err != nil || crlf != baseline {
		t.Fatalf("Git-clean-equivalent EOL changed fingerprint: LF=%s CRLF=%s err=%v", baseline, crlf, err)
	}
	if err := os.WriteFile(evidencePath, []byte("changed\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	semanticChange, err := evidenceFingerprint(project, root, page)
	if err != nil || semanticChange == baseline {
		t.Fatalf("semantic content change was not detected: baseline=%s changed=%s err=%v", baseline, semanticChange, err)
	}

	if err := os.WriteFile(evidencePath, []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("working\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree, err := evidenceFingerprint(project, root, page)
	if err != nil || worktree != baseline {
		t.Fatalf("tracked fingerprint used staged blob instead of worktree bytes: baseline=%s got=%s err=%v", baseline, worktree, err)
	}

	untrackedPath := filepath.Join(project, "untracked.txt")
	untrackedPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "untracked.txt"}}}}
	if err := os.WriteFile(untrackedPath, []byte("raw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untrackedLF, err := evidenceFingerprint(project, root, untrackedPage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(untrackedPath, []byte("raw\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untrackedCRLF, err := evidenceFingerprint(project, root, untrackedPage)
	if err != nil || untrackedCRLF == untrackedLF {
		t.Fatalf("untracked raw EOL representations were normalized: LF=%s CRLF=%s err=%v", untrackedLF, untrackedCRLF, err)
	}

	nonGit := t.TempDir()
	nonGitRoot := filepath.Join(nonGit, "docs", "wiki")
	if _, err := Init(nonGitRoot); err != nil {
		t.Fatal(err)
	}
	nonGitPath := filepath.Join(nonGit, "evidence.txt")
	if err := os.WriteFile(nonGitPath, []byte("raw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nonGitPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence.txt"}}}}
	nonGitLF, err := evidenceFingerprint(nonGit, nonGitRoot, nonGitPage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nonGitPath, []byte("raw\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nonGitCRLF, err := evidenceFingerprint(nonGit, nonGitRoot, nonGitPage)
	if err != nil || nonGitCRLF == nonGitLF {
		t.Fatalf("non-Git raw EOL representations were normalized: LF=%s CRLF=%s err=%v", nonGitLF, nonGitCRLF, err)
	}
}

func TestTrackedRegularEvidenceRequiredCleanFilterFailureIsUnreadable(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "config", "commit.gpgSign", "false")
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", "evidence.txt")
	git(t, project, "commit", "-qm", "tracked evidence")
	if err := os.WriteFile(filepath.Join(project, ".gitattributes"), []byte("evidence.txt filter=required-evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "config", "filter.required-evidence.required", "true")
	git(t, project, "config", "filter.required-evidence.clean", "archetipo-wiki-missing-clean-filter")
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence.txt"}}}}
	if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("required clean-filter failure error=%v", err)
	}
}

func TestGitIndexExecutableModeIsAuthoritative(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(project, "script.sh")
	if err := os.WriteFile(script, []byte("echo portable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "add", "script.sh")
	git(t, project, "commit", "-qm", "tracked script")
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "script.sh"}}}}

	nonExecutable, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	git(t, project, "update-index", "--chmod=+x", "script.sh")
	executable, err := evidenceFingerprint(project, root, page)
	if err != nil || executable == nonExecutable {
		t.Fatalf("index executable mode did not change fingerprint: before=%s after=%s err=%v", nonExecutable, executable, err)
	}
	git(t, project, "config", "core.fileMode", "false")
	withFileModeDisabled, err := evidenceFingerprint(project, root, page)
	if err != nil || withFileModeDisabled != executable {
		t.Fatalf("core.fileMode changed index-authoritative hash: executable=%s disabled=%s err=%v", executable, withFileModeDisabled, err)
	}
	git(t, project, "update-index", "--chmod=-x", "script.sh")
	restored, err := evidenceFingerprint(project, root, page)
	if err != nil || restored != nonExecutable {
		t.Fatalf("restoring index mode did not restore fingerprint: initial=%s restored=%s err=%v", nonExecutable, restored, err)
	}
}

func TestExecutableMarkerIgnoresUntrackedHostMode(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "untracked.sh")
	if err := os.WriteFile(path, []byte("echo portable\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "untracked.sh"}}}}
	first, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("echo portable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := evidenceFingerprint(project, root, page)
	if err != nil || second != first {
		t.Fatalf("host mode changed untracked fingerprint: first=%s second=%s err=%v", first, second, err)
	}
}

func TestTrackedSymlinkUsesPortableGitIndexMode(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "config", "core.symlinks", "false")
	linkPath := filepath.Join(project, "portable-link")
	if err := os.WriteFile(linkPath, []byte("target.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = project
	cmd.Stdin = strings.NewReader("target.txt")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hashing symlink blob: %v", err)
	}
	oid := strings.TrimSpace(string(out))
	git(t, project, "update-index", "--add", "--cacheinfo", "120000", oid, "portable-link")
	git(t, project, "commit", "-qm", "tracked portable symlink")

	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "portable-link"}}}}
	materialized, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	git(t, project, "config", "core.symlinks", "true")
	withConfigChanged, err := evidenceFingerprint(project, root, page)
	if err != nil || withConfigChanged != materialized {
		t.Fatalf("core.symlinks changed materialized hash: before=%s after=%s err=%v", materialized, withConfigChanged, err)
	}

	t.Run("real OS symlink integration", func(t *testing.T) {
		if err := os.Remove(linkPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target.txt", linkPath); err != nil {
			if writeErr := os.WriteFile(linkPath, []byte("target.txt"), 0o644); writeErr != nil {
				t.Fatalf("restoring materialized link after symlink failure: %v", writeErr)
			}
			if symlinkCapabilityUnavailable(err) {
				t.Skipf("real symlink capability explicitly unavailable: %v", err)
			}
			t.Fatalf("creating real symlink failed unexpectedly: %v", err)
		}
		actualLink, err := evidenceFingerprint(project, root, page)
		if err != nil || actualLink != materialized {
			t.Fatalf("OS and materialized symlink hashes differ: materialized=%s symlink=%s err=%v", materialized, actualLink, err)
		}
	})
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkPath, []byte("other-target.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := evidenceFingerprint(project, root, page)
	if err != nil || changed == materialized {
		t.Fatalf("tracked symlink payload change was not fingerprinted: baseline=%s changed=%s err=%v", materialized, changed, err)
	}
}

type wikiSubmoduleFixture struct {
	project string
	root    string
	origin  string
	module  string
}

func newWikiSubmoduleFixture(t *testing.T) wikiSubmoduleFixture {
	t.Helper()
	origin := t.TempDir()
	git(t, origin, "init", "-q")
	git(t, origin, "config", "user.email", "wiki-test@example.test")
	git(t, origin, "config", "user.name", "Wiki Test")
	if err := os.WriteFile(filepath.Join(origin, "module.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, origin, "add", "module.txt")
	git(t, origin, "commit", "-qm", "first module revision")

	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "-c", "protocol.file.allow=always", "submodule", "add", "-q", origin, "modules/component")
	git(t, project, "add", ".gitmodules", "modules/component")
	git(t, project, "commit", "-qm", "add module")
	return wikiSubmoduleFixture{project: project, root: root, origin: origin, module: filepath.Join(project, "modules", "component")}
}

func (fixture wikiSubmoduleFixture) addOriginRevision(t *testing.T) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fixture.origin, "module.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, fixture.origin, "add", "module.txt")
	git(t, fixture.origin, "commit", "-qm", "second module revision")
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixture.origin
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (fixture wikiSubmoduleFixture) checkoutModule(t *testing.T, oid string) {
	t.Helper()
	git(t, fixture.module, "fetch", "-q", "origin")
	git(t, fixture.module, "checkout", "-q", oid)
}

func TestSubmoduleCleanApprovalAndParentDirectoryFingerprint(t *testing.T) {
	fixture := newWikiSubmoduleFixture(t)
	writeSimplePage(t, fixture.root, "direct", "modules/component")
	writeSimplePage(t, fixture.root, "parent", "modules")
	if approved, err := Approve(fixture.project, fixture.root, []string{"direct", "parent"}); err != nil || approved != 2 {
		t.Fatalf("clean submodule approval: approved=%d err=%v", approved, err)
	}
	pages, err := Load(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if state := PageState(fixture.project, fixture.root, page); state != "reviewed" {
			t.Fatalf("clean submodule page %s state=%s", page.ID, state)
		}
	}
}

func TestSubmoduleIgnoredFilesRemainGitClean(t *testing.T) {
	fixture := newWikiSubmoduleFixture(t)
	// Submodule .git is commonly a gitfile, so resolve the actual Git directory.
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = fixture.module
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(fixture.module, gitDir)
	}
	infoExclude := filepath.Join(gitDir, "info", "exclude")
	file, err := os.OpenFile(infoExclude, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("ignored.tmp\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.module, "ignored.tmp"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "modules/component"}}}}
	if _, err := evidenceFingerprint(fixture.project, fixture.root, page); err != nil {
		t.Fatalf("ignored submodule file was not Git-clean: %v", err)
	}
}

func TestSubmoduleGitlinkDescendantsAreRejectedBeforeWorktreeAccess(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, wikiSubmoduleFixture)
	}{
		{name: "clean", mutate: func(*testing.T, wikiSubmoduleFixture) {}},
		{name: "dirty", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			if err := os.WriteFile(filepath.Join(fixture.module, "module.txt"), []byte("dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			if err := os.RemoveAll(fixture.module); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "uninitialized", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			git(t, fixture.project, "submodule", "deinit", "-q", "-f", "modules/component")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWikiSubmoduleFixture(t)
			test.mutate(t, fixture)
			page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "modules/component/module.txt"}}}}
			if _, err := evidenceFingerprint(fixture.project, fixture.root, page); !errors.Is(err, ErrSubmoduleEvidence) {
				t.Fatalf("gitlink descendant fingerprint error=%v", err)
			}
		})
	}

	fixture := newWikiSubmoduleFixture(t)
	lookalike := filepath.Join(fixture.project, "modules", "component-extra", "file.txt")
	if err := os.MkdirAll(filepath.Dir(lookalike), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lookalike, []byte("ordinary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "modules/component-extra/file.txt"}}}}
	if _, err := evidenceFingerprint(fixture.project, fixture.root, page); err != nil {
		t.Fatalf("component-prefix lookalike was rejected: %v", err)
	}
}

func TestGitlinkStagedUpdateChangesFingerprint(t *testing.T) {
	fixture := newWikiSubmoduleFixture(t)
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "modules/component"}}}}
	before, err := evidenceFingerprint(fixture.project, fixture.root, page)
	if err != nil {
		t.Fatal(err)
	}
	second := fixture.addOriginRevision(t)
	fixture.checkoutModule(t, second)
	git(t, fixture.project, "add", "modules/component")
	after, err := evidenceFingerprint(fixture.project, fixture.root, page)
	if err != nil || after == before {
		t.Fatalf("staged matching gitlink did not change fingerprint: before=%s after=%s err=%v", before, after, err)
	}
}

func TestSubmoduleBlocksHeadMismatchDirtyAndUninitializedStates(t *testing.T) {
	tests := []struct {
		name   string
		source string
		mutate func(*testing.T, wikiSubmoduleFixture)
	}{
		{name: "HEAD mismatch", source: "modules/component", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			fixture.checkoutModule(t, fixture.addOriginRevision(t))
		}},
		{name: "dirty tracked", source: "modules/component", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			if err := os.WriteFile(filepath.Join(fixture.module, "module.txt"), []byte("dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "dirty untracked through parent", source: "modules", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			if err := os.WriteFile(filepath.Join(fixture.module, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "conflicted", source: "modules/component", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			git(t, fixture.module, "config", "user.email", "wiki-test@example.test")
			git(t, fixture.module, "config", "user.name", "Wiki Test")
			if err := os.WriteFile(filepath.Join(fixture.module, "module.txt"), []byte("local\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			git(t, fixture.module, "add", "module.txt")
			git(t, fixture.module, "commit", "-qm", "local conflicting revision")
			git(t, fixture.project, "add", "modules/component")
			remote := fixture.addOriginRevision(t)
			git(t, fixture.module, "fetch", "-q", "origin")
			cmd := exec.Command("git", "merge", "--no-edit", remote)
			cmd.Dir = fixture.module
			if err := cmd.Run(); err == nil {
				t.Fatal("expected submodule merge conflict")
			}
		}},
		{name: "uninitialized", source: "modules/component", mutate: func(t *testing.T, fixture wikiSubmoduleFixture) {
			git(t, fixture.project, "submodule", "deinit", "-q", "-f", "modules/component")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWikiSubmoduleFixture(t)
			test.mutate(t, fixture)
			page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: test.source}}}}
			if _, err := evidenceFingerprint(fixture.project, fixture.root, page); !errors.Is(err, ErrSubmoduleEvidence) {
				t.Fatalf("unsafe submodule fingerprint error=%v", err)
			}
		})
	}
}

func TestSubmoduleNestedUninitializedWorktreeFollowsGitCleanStatus(t *testing.T) {
	fixture := newWikiSubmoduleFixture(t)
	nestedOrigin := t.TempDir()
	git(t, nestedOrigin, "init", "-q")
	git(t, nestedOrigin, "config", "user.email", "wiki-test@example.test")
	git(t, nestedOrigin, "config", "user.name", "Wiki Test")
	if err := os.WriteFile(filepath.Join(nestedOrigin, "nested.txt"), []byte("nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, nestedOrigin, "add", "nested.txt")
	git(t, nestedOrigin, "commit", "-qm", "nested baseline")
	git(t, fixture.module, "config", "user.email", "wiki-test@example.test")
	git(t, fixture.module, "config", "user.name", "Wiki Test")
	git(t, fixture.module, "-c", "protocol.file.allow=always", "submodule", "add", "-q", nestedOrigin, "nested/child")
	git(t, fixture.module, "add", ".gitmodules", "nested/child")
	git(t, fixture.module, "commit", "-qm", "add nested module")
	git(t, fixture.project, "add", "modules/component")
	git(t, fixture.module, "submodule", "deinit", "-q", "-f", "nested/child")

	status := gitSubmoduleStatusForTest(t, fixture.module)
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "modules/component"}}}}
	_, err := evidenceFingerprint(fixture.project, fixture.root, page)
	if len(status) == 0 && err != nil {
		t.Fatalf("Git-clean nested uninitialized worktree was rejected: %v", err)
	}
	if len(status) != 0 && !errors.Is(err, ErrSubmoduleEvidence) {
		t.Fatalf("Git-dirty nested uninitialized worktree was accepted: status=%q err=%v", status, err)
	}
}

func gitSubmoduleStatusForTest(t *testing.T, dir string) []byte {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=none")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSubmoduleNestedDirtyStateIsBlocked(t *testing.T) {
	nestedOrigin := t.TempDir()
	git(t, nestedOrigin, "init", "-q")
	git(t, nestedOrigin, "config", "user.email", "wiki-test@example.test")
	git(t, nestedOrigin, "config", "user.name", "Wiki Test")
	if err := os.WriteFile(filepath.Join(nestedOrigin, "nested.txt"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, nestedOrigin, "add", "nested.txt")
	git(t, nestedOrigin, "commit", "-qm", "nested baseline")

	moduleOrigin := t.TempDir()
	git(t, moduleOrigin, "init", "-q")
	git(t, moduleOrigin, "config", "user.email", "wiki-test@example.test")
	git(t, moduleOrigin, "config", "user.name", "Wiki Test")
	if err := os.WriteFile(filepath.Join(moduleOrigin, "module.txt"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, moduleOrigin, "add", "module.txt")
	git(t, moduleOrigin, "commit", "-qm", "module baseline")
	git(t, moduleOrigin, "-c", "protocol.file.allow=always", "submodule", "add", "-q", nestedOrigin, "nested/child")
	git(t, moduleOrigin, "add", ".gitmodules", "nested/child")
	git(t, moduleOrigin, "commit", "-qm", "add nested module")

	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "-c", "protocol.file.allow=always", "submodule", "add", "-q", moduleOrigin, "modules/component")
	git(t, project, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init", "--recursive")
	git(t, project, "add", ".gitmodules", "modules/component")
	git(t, project, "commit", "-qm", "add nested module tree")
	if err := os.WriteFile(filepath.Join(project, "modules", "component", "nested", "child", "nested.txt"), []byte("dirty nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "modules/component"}}}}
	if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrSubmoduleEvidence) {
		t.Fatalf("nested dirty submodule error=%v", err)
	}
}

func TestGitDirectoryFingerprintIncludesUntrackedAndDeletedEntries(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	trackedPath := filepath.Join(project, "evidence", "tracked.txt")
	untrackedPath := filepath.Join(project, "evidence", "untracked.txt")
	if err := os.WriteFile(trackedPath, []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "add", "evidence/tracked.txt")
	git(t, project, "commit", "-qm", "tracked evidence")
	if err := os.WriteFile(untrackedPath, []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence"}}}}
	baseline, err := evidenceFingerprint(project, root, page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(untrackedPath, []byte("changed untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untrackedChanged, err := evidenceFingerprint(project, root, page)
	if err != nil || untrackedChanged == baseline {
		t.Fatalf("untracked directory entry was not fingerprinted: hash=%s err=%v", untrackedChanged, err)
	}
	if err := os.WriteFile(untrackedPath, []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(trackedPath); err != nil {
		t.Fatal(err)
	}
	trackedDeleted, err := evidenceFingerprint(project, root, page)
	if err != nil || trackedDeleted == baseline {
		t.Fatalf("deleted tracked directory entry was not fingerprinted: hash=%s err=%v", trackedDeleted, err)
	}
}

func TestEmbeddedGitRepositoriesAreRejectedAsEvidenceBoundaries(t *testing.T) {
	t.Run("Git parent direct descendant and directory expansion", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		git(t, project, "init", "-q")
		git(t, project, "config", "user.email", "wiki-test@example.test")
		git(t, project, "config", "user.name", "Wiki Test")
		git(t, project, "config", "commit.gpgSign", "false")
		if err := os.MkdirAll(filepath.Join(project, "evidence", "ordinary"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "evidence", "ordinary", "item.txt"), []byte("ordinary\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		repository := filepath.Join(project, "evidence", "repository")
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, repository, "init", "-q")
		git(t, repository, "config", "user.email", "wiki-test@example.test")
		git(t, repository, "config", "user.name", "Wiki Test")
		git(t, repository, "config", "commit.gpgSign", "false")
		if err := os.WriteFile(filepath.Join(repository, "nested.txt"), []byte("nested\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, repository, "add", "nested.txt")
		git(t, repository, "commit", "-qm", "nested baseline")

		for _, source := range []string{"evidence/repository", "evidence/repository/nested.txt", "evidence"} {
			t.Run(source, func(t *testing.T) {
				page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: source}}}}
				if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrUnsupportedEvidenceEntry) {
					t.Fatalf("embedded repository source %q error=%v", source, err)
				}
			})
		}
		ordinary := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence/ordinary"}}}}
		before, err := evidenceFingerprint(project, root, ordinary)
		if err != nil {
			t.Fatalf("ordinary directory rejected: %v", err)
		}
		if err := os.WriteFile(filepath.Join(project, "evidence", "ordinary", "item.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		after, err := evidenceFingerprint(project, root, ordinary)
		if err != nil || after == before {
			t.Fatalf("ordinary directory was not content-sensitive: before=%s after=%s err=%v", before, after, err)
		}
	})

	t.Run("non-Git parent", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		repository := filepath.Join(project, "evidence", "repository")
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, repository, "init", "-q")
		page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence"}}}}
		if _, err := evidenceFingerprint(project, root, page); err == nil || (!errors.Is(err, ErrUnsupportedEvidenceEntry) && !errors.Is(err, ErrEvidenceUnreadable)) {
			t.Fatalf("non-Git parent hid embedded repository: %v", err)
		}
	})

	t.Run("Git confirmation failure is fail-closed", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		repository := filepath.Join(project, "evidence", "repository")
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, repository, "init", "-q")
		t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
		page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence/repository"}}}}
		if _, err := evidenceFingerprint(project, root, page); err == nil || (!errors.Is(err, ErrEvidenceUnreadable) && !errors.Is(err, ErrUnsupportedEvidenceEntry)) {
			t.Fatalf("failed repository confirmation was not fail-closed: %v", err)
		}
	})

	t.Run("linked worktree git file", func(t *testing.T) {
		origin := t.TempDir()
		git(t, origin, "init", "-q")
		git(t, origin, "config", "user.email", "wiki-test@example.test")
		git(t, origin, "config", "user.name", "Wiki Test")
		git(t, origin, "config", "commit.gpgSign", "false")
		if err := os.WriteFile(filepath.Join(origin, "item.txt"), []byte("item\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, origin, "add", "item.txt")
		git(t, origin, "commit", "-qm", "baseline")

		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		worktree := filepath.Join(project, "evidence", "worktree")
		if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, origin, "worktree", "add", "-q", "--detach", worktree)
		page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence/worktree"}}}}
		if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrUnsupportedEvidenceEntry) {
			t.Fatalf("linked worktree was not rejected: %v", err)
		}
	})

	t.Run("bare repository", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		bare := filepath.Join(project, "evidence", "bare.git")
		if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, project, "init", "--bare", "-q", bare)
		for _, source := range []string{"evidence/bare.git", "evidence"} {
			page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: source}}}}
			if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrUnsupportedEvidenceEntry) {
				t.Fatalf("bare repository source %q error=%v", source, err)
			}
		}
	})

	t.Run("bare repository with symlink HEAD", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		bare := filepath.Join(project, "evidence", "symlink-head.git")
		if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, project, "init", "--bare", "-q", bare)
		if err := os.Rename(filepath.Join(bare, "HEAD"), filepath.Join(bare, "HEAD.real")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("HEAD.real", filepath.Join(bare, "HEAD")); err != nil {
			if symlinkCapabilityUnavailable(err) {
				t.Skipf("real symlink capability explicitly unavailable: %v", err)
			}
			t.Fatalf("creating symlink HEAD failed unexpectedly: %v", err)
		}
		page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence/symlink-head.git"}}}}
		if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrUnsupportedEvidenceEntry) && !errors.Is(err, ErrEvidenceUnreadable) {
			t.Fatalf("symlink-HEAD bare repository was not rejected fail-closed: %v", err)
		}
	})

	t.Run("reftable bare repository", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		bare := filepath.Join(project, "evidence", "reftable.git")
		if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "init", "--bare", "--ref-format=reftable", "-q", bare)
		cmd.Dir = project
		if out, err := cmd.CombinedOutput(); err != nil {
			if isUnsupportedReftableDiagnostic(string(out)) {
				t.Skipf("Git explicitly reports reftable unsupported: %v (%s)", err, out)
			}
			t.Fatalf("reftable initialization failed unexpectedly: %v (%s)", err, out)
		}
		page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence/reftable.git"}}}}
		if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, ErrUnsupportedEvidenceEntry) {
			t.Fatalf("reftable bare repository error=%v", err)
		}
	})
}

func isUnsupportedReftableDiagnostic(output string) bool {
	message := strings.ToLower(output)
	return strings.Contains(message, "unknown option `ref-format") ||
		strings.Contains(message, "unknown option 'ref-format") ||
		strings.Contains(message, "unknown option: --ref-format") ||
		strings.Contains(message, "unknown option --ref-format") ||
		strings.Contains(message, "unknown ref storage format 'reftable'") ||
		strings.Contains(message, "unknown ref storage format `reftable`") ||
		strings.Contains(message, "unsupported ref storage format 'reftable'") ||
		strings.Contains(message, "unsupported ref storage format `reftable`") ||
		strings.Contains(message, "reftable is not supported") ||
		strings.Contains(message, "does not support reftable")
}

func TestUnsupportedReftableDiagnosticIsExplicit(t *testing.T) {
	for _, diagnostic := range []string{
		"error: unknown option `ref-format=reftable'",
		"fatal: unsupported ref storage format 'reftable'",
		"reftable is not supported by this Git build",
	} {
		if !isUnsupportedReftableDiagnostic(diagnostic) {
			t.Fatalf("explicit unsupported diagnostic was not recognized: %q", diagnostic)
		}
	}
	for _, diagnostic := range []string{
		"permission denied",
		"disk full",
		"fatal: repository initialization failed",
		"fatal: cannot create /tmp/reftable.git: resource not available",
		"fatal: invalid value for ref-format config: permission denied",
		"fatal: ref-format setup failed: device not available",
		"fatal: ref-format config contains unknown option 'foo'",
		"fatal: /tmp/reftable.git config contains unknown option 'foo'",
	} {
		if isUnsupportedReftableDiagnostic(diagnostic) {
			t.Fatalf("unexpected infrastructure failure would skip: %q", diagnostic)
		}
	}
}

func TestEmbeddedGitRepositoryIgnoredParentAndProjectRootRules(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "config", "commit.gpgSign", "false")
	if err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte("evidence/ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "ordinary.txt"), []byte("ordinary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", ".gitignore", "evidence/ordinary.txt")
	git(t, project, "commit", "-qm", "baseline")

	rootPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "."}}}}
	if _, err := evidenceFingerprint(project, root, rootPage); err != nil {
		t.Fatalf("project repository root was rejected: %v", err)
	}
	ignored := filepath.Join(project, "evidence", "ignored")
	if err := os.MkdirAll(ignored, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, ignored, "init", "-q")
	parentPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence"}}}}
	if _, err := evidenceFingerprint(project, root, parentPage); err != nil {
		t.Fatalf("ignored embedded repository affected parent fingerprint: %v", err)
	}
	directPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "evidence/ignored"}}}}
	if _, err := evidenceFingerprint(project, root, directPage); !errors.Is(err, ErrUnsupportedEvidenceEntry) {
		t.Fatalf("directly cited ignored repository error=%v", err)
	}

	outer := t.TempDir()
	git(t, outer, "init", "-q")
	projectSubdir := filepath.Join(outer, "configured-project")
	if err := os.MkdirAll(projectSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	subdirRoot := filepath.Join(projectSubdir, "docs", "wiki")
	if _, err := Init(subdirRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectSubdir, "item.txt"), []byte("item\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subdirPage := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "item.txt"}}}}
	if _, err := evidenceFingerprint(projectSubdir, subdirRoot, subdirPage); err != nil {
		t.Fatalf("outer repository was misclassified as embedded: %v", err)
	}
}

func TestValidateRejectsMalformedEvidenceHashAndAcceptsLegacyReview(t *testing.T) {
	t.Run("malformed hash", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeSimplePage(t, root, "overview", "README.md")
		if _, err := Approve(project, root, []string{"overview"}); err != nil {
			t.Fatal(err)
		}
		pages, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		pages[0].Meta.Review.EvidenceHash = "sha256:ABC"
		raw, err := renderPage(pages[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pages[0].Path), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		report := Validate(project, root)
		if report.OK || !hasFinding(report, "WIKI_REVIEW_METADATA_INVALID") {
			t.Fatalf("malformed evidence hash validated: %+v", report.Findings)
		}
	})

	t.Run("legacy revision fallback", func(t *testing.T) {
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Project\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "context.md"), []byte("context\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, project, "init", "-q")
		git(t, project, "config", "user.email", "wiki-test@example.test")
		git(t, project, "config", "user.name", "Wiki Test")
		git(t, project, "add", "README.md", "context.md")
		git(t, project, "commit", "-qm", "baseline")
		writeSimplePage(t, root, "overview", "README.md")
		pages, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		pages[0].Meta.Sources = append(pages[0].Meta.Sources, domain.WikiSource{Path: "context.md", Freshness: "context"})
		raw, err := renderPage(pages[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pages[0].Path), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Approve(project, root, []string{"overview"}); err != nil {
			t.Fatal(err)
		}
		pages, err = Load(root)
		if err != nil {
			t.Fatal(err)
		}
		pages[0].Meta.Review.EvidenceHash = ""
		raw, err = renderPage(pages[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, pages[0].Path), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		pages, err = Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if report := Validate(project, root); !report.OK || hasFinding(report, "WIKI_REVIEW_METADATA_INVALID") {
			t.Fatalf("legacy review became invalid: %+v", report.Findings)
		}
		if state := PageState(project, root, pages[0]); state != "reviewed" {
			t.Fatalf("legacy review state=%s", state)
		}
		if err := os.WriteFile(filepath.Join(project, "context.md"), []byte("changed context\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if state := PageState(project, root, pages[0]); state != "reviewed" {
			t.Fatalf("legacy revision fallback included context source: %s", state)
		}
		if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if state := PageState(project, root, pages[0]); state != "stale" {
			t.Fatalf("legacy revision fallback did not detect change: %s", state)
		}
	})
}

func TestReconfirmRefreshesStaleEvidenceAndPreservesSemanticContent(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `---
type: guide
title: Behavioral guide
description: Reviewed behavioral guidance
status: generated
sources:
  - path: evidence.txt
    freshness: tracked
owner:
  team: platform
---
# Behavioral guide

Unchanged guidance.
`
	pagePath := filepath.Join(root, "behavior.md")
	if err := os.WriteFile(pagePath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if approved, err := Approve(project, root, []string{"behavior"}); err != nil || approved != 1 {
		t.Fatalf("approve: approved=%d err=%v", approved, err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	before := pages[0]
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("changed evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, before); state != "stale" {
		t.Fatalf("expected stale evidence before reconfirmation, got %s", state)
	}
	if reconfirmed, err := Reconfirm(project, root, []string{"behavior"}); err != nil || reconfirmed != 1 {
		t.Fatalf("reconfirm: count=%d err=%v", reconfirmed, err)
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	after := pages[0]
	if state := PageState(project, root, after); state != "reviewed" {
		t.Fatalf("state after reconfirmation=%s", state)
	}
	if after.Meta.Review.ContentHash != before.Meta.Review.ContentHash || after.Body != before.Body || after.Meta.Sources[0].Freshness != "tracked" {
		t.Fatalf("reconfirmation changed semantic page content: before=%+v after=%+v", before, after)
	}
	if after.Meta.Review.EvidenceHash == before.Meta.Review.EvidenceHash {
		t.Fatal("reconfirmation did not refresh the evidence hash")
	}
	persisted, err := os.ReadFile(pagePath)
	if err != nil || !strings.Contains(string(persisted), "owner:\n    team: platform") {
		t.Fatalf("unknown frontmatter was not preserved:\n%s\nerr=%v", persisted, err)
	}
	log, err := os.ReadFile(filepath.Join(root, "log.md"))
	if err != nil || strings.Count(string(log), "Reconfirmed 1 page(s)") != 1 {
		t.Fatalf("reconfirm audit entry mismatch:\n%s\nerr=%v", log, err)
	}
}

func TestReconfirmContextChangeAndFreshHashAreNoOpsAndLegacyUpgrades(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "context.txt"), []byte("context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `---
type: guide
title: Stable guide
description: Stable reviewed guidance
status: generated
sources:
  - path: tracked.txt
  - path: context.txt
    freshness: context
---
# Stable guide
`
	pagePath := filepath.Join(root, "stable.md")
	if err := os.WriteFile(pagePath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(project, root, []string{"stable"}); err != nil {
		t.Fatal(err)
	}
	beforePage, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeLog, err := os.ReadFile(filepath.Join(root, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "context.txt"), []byte("changed context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if reconfirmed, err := Reconfirm(project, root, []string{"stable"}); err != nil || reconfirmed != 0 {
		t.Fatalf("context-only reconfirm: count=%d err=%v", reconfirmed, err)
	}
	afterPage, _ := os.ReadFile(pagePath)
	afterLog, _ := os.ReadFile(filepath.Join(root, "log.md"))
	if string(afterPage) != string(beforePage) || string(afterLog) != string(beforeLog) {
		t.Fatalf("fresh no-op churned page or log:\npage before=%s\npage after=%s\nlog before=%s\nlog after=%s", beforePage, afterPage, beforeLog, afterLog)
	}

	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pages[0].Meta.Review.EvidenceHash = ""
	legacyRaw, err := renderPage(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pagePath, legacyRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if reconfirmed, err := Reconfirm(project, root, []string{"stable"}); err != nil || reconfirmed != 1 {
		t.Fatalf("legacy upgrade: count=%d err=%v", reconfirmed, err)
	}
	pages, err = Load(root)
	if err != nil || pages[0].Meta.Review.EvidenceHash == "" {
		t.Fatalf("legacy page did not gain evidence hash: pages=%+v err=%v", pages, err)
	}
	logAfterUpgrade, err := os.ReadFile(filepath.Join(root, "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if reconfirmed, err := Reconfirm(project, root, []string{"stable"}); err != nil || reconfirmed != 0 {
		t.Fatalf("fresh reconfirm after upgrade: count=%d err=%v", reconfirmed, err)
	}
	finalLog, _ := os.ReadFile(filepath.Join(root, "log.md"))
	if string(finalLog) != string(logAfterUpgrade) {
		t.Fatalf("fresh reconfirm appended audit log:\nbefore=%s\nafter=%s", logAfterUpgrade, finalLog)
	}
}

func TestReconfirmRejectsIneligiblePages(t *testing.T) {
	newProjectPage := func(t *testing.T) (string, string) {
		t.Helper()
		project := t.TempDir()
		root := filepath.Join(project, "docs", "wiki")
		if _, err := Init(root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("evidence\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeSimplePage(t, root, "page", "evidence.txt")
		return project, root
	}

	t.Run("generated", func(t *testing.T) {
		project, root := newProjectPage(t)
		if _, err := Reconfirm(project, root, []string{"page"}); !errors.Is(err, ErrReconfirmIneligible) {
			t.Fatalf("generated page error=%v", err)
		}
	})
	t.Run("content changed", func(t *testing.T) {
		project, root := newProjectPage(t)
		if _, err := Approve(project, root, []string{"page"}); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "page.md")
		raw, _ := os.ReadFile(path)
		if err := os.WriteFile(path, append(raw, []byte("changed content\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Reconfirm(project, root, []string{"page"}); !errors.Is(err, ErrReconfirmIneligible) {
			t.Fatalf("content-changed page error=%v", err)
		}
	})
	t.Run("issues", func(t *testing.T) {
		project, root := newProjectPage(t)
		if _, err := Approve(project, root, []string{"page"}); err != nil {
			t.Fatal(err)
		}
		pages, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		pages[0].Meta.Issues = []domain.WikiIssue{{Code: "OPEN", Summary: "Still unresolved"}}
		pages[0].Meta.Review.ContentHash = pageContentHash(pages[0])
		raw, err := renderPage(pages[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "page.md"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Reconfirm(project, root, []string{"page"}); !errors.Is(err, ErrUnresolvedIssues) {
			t.Fatalf("issue page error=%v", err)
		}
	})
	t.Run("missing evidence", func(t *testing.T) {
		project, root := newProjectPage(t)
		if _, err := Approve(project, root, []string{"page"}); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(project, "evidence.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := Reconfirm(project, root, []string{"page"}); !errors.Is(err, ErrMissingEvidence) {
			t.Fatalf("missing-evidence page error=%v", err)
		}
	})
	t.Run("global validation", func(t *testing.T) {
		project, root := newProjectPage(t)
		if _, err := Approve(project, root, []string{"page"}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "invalid.md"), []byte("---\ntype: guide\nstatus: generated\n---\n# Invalid\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Reconfirm(project, root, []string{"page"}); !errors.Is(err, ErrValidationFailed) {
			t.Fatalf("global validation error=%v", err)
		}
	})
}

func TestReconfirmPreflightsEveryFingerprintBeforeWriting(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "good.txt"), []byte("good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "unsupported"), []byte("regular before review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "a-good", "good.txt")
	writeSimplePage(t, root, "b-unsupported", "unsupported")
	if approved, err := Approve(project, root, []string{"a-good", "b-unsupported"}); err != nil || approved != 2 {
		t.Fatalf("approve: count=%d err=%v", approved, err)
	}
	if err := os.WriteFile(filepath.Join(project, "good.txt"), []byte("changed good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setUnmergedGitIndex(t, project, "unsupported")
	goodPath := filepath.Join(root, "a-good.md")
	before, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if reconfirmed, err := Reconfirm(project, root, []string{"a-good", "b-unsupported"}); err == nil || reconfirmed != 0 {
		t.Fatalf("expected fingerprint preflight failure: count=%d err=%v", reconfirmed, err)
	}
	after, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("first page mutated before batch preflight completed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestApprovePreflightsEveryFingerprintBeforeWriting(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "good.txt"), []byte("good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "unsupported.txt"), []byte("conflicted evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setUnmergedGitIndex(t, project, "unsupported.txt")
	writeSimplePage(t, root, "a-good", "good.txt")
	writeSimplePage(t, root, "b-unsupported", "unsupported.txt")
	goodPath := filepath.Join(root, "a-good.md")
	before, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if approved, err := Approve(project, root, []string{"a-good", "b-unsupported"}); err == nil || approved != 0 {
		t.Fatalf("expected fingerprint preflight failure with zero approvals: approved=%d err=%v", approved, err)
	}
	after, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("first selected page mutated before every fingerprint succeeded:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if page.Meta.Status != domain.WikiStatusGenerated || page.Meta.Review != nil {
			t.Fatalf("page partially approved after preflight failure: %+v", page)
		}
	}
}

func TestEvidenceRecomputationErrorsFailClosed(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(project, "evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "item.txt"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "overview", "evidence/item.txt")
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(evidenceDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidenceDir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pages[0]); state != "stale" {
		t.Fatalf("fingerprint recomputation error did not fail closed: %s", state)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_EVIDENCE_UNREADABLE") {
		t.Fatalf("validation did not report unreadable evidence: %+v", report.Findings)
	}
	if hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("recomputation error was also reported as changed evidence: %+v", report.Findings)
	}
}

func TestEvidenceChangedRequiresSuccessfulRecomputation(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(project, "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "overview", "evidence.txt")
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if !report.OK || !hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("successful mismatch was not reported as a warning: %+v", report.Findings)
	}
	for _, code := range []string{"WIKI_EVIDENCE_UNREADABLE", "WIKI_EVIDENCE_RECOMPUTE_FAILED", "WIKI_UNSAFE_SOURCE_PATH"} {
		if hasFinding(report, code) {
			t.Fatalf("successful mismatch also reported %s: %+v", code, report.Findings)
		}
	}
}

func TestUnsafeSourcePathIsAnErrorNotEvidenceChanged(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "overview", "evidence.txt")
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pages[0].Meta.Sources[0].Path = "../outside.txt"
	pages[0].Meta.Review.ContentHash = pageContentHash(pages[0])
	raw, err := renderPage(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, pages[0].Path), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_UNSAFE_SOURCE_PATH") {
		t.Fatalf("unsafe traversal did not block validation: %+v", report.Findings)
	}
	if hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("unsafe traversal was mislabeled as changed evidence: %+v", report.Findings)
	}
}

func TestLegacyEvidenceRecomputationFailure(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence.txt"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki-test@example.test")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "add", "evidence.txt")
	git(t, project, "commit", "-qm", "baseline")
	writeSimplePage(t, root, "overview", "evidence.txt")
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pages[0].Meta.Review.EvidenceHash = ""
	pages[0].Meta.Review.EvidenceRevision = "deadbeef"
	raw, err := renderPage(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, pages[0].Path), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state := PageState(project, root, pages[0]); state != "stale" {
		t.Fatalf("broken legacy revision state=%s", state)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_EVIDENCE_RECOMPUTE_FAILED") {
		t.Fatalf("broken legacy revision did not produce an error: %+v", report.Findings)
	}
	if hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("broken legacy revision was mislabeled as changed evidence: %+v", report.Findings)
	}
}

func TestWikiApproveBlocksUnreadableEvidence(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence"), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "overview", "evidence/item.txt")
	if _, err := Approve(project, root, []string{"overview"}); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("approval error=%v", err)
	}
}

func TestWikiReconfirmBlocksUnreadableEvidence(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(project, "evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "item.txt"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "overview", "evidence/item.txt")
	if _, err := Approve(project, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(evidenceDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidenceDir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconfirm(project, root, []string{"overview"}); !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("reconfirmation error=%v", err)
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
	if err := os.WriteFile(filepath.Join(project, "context.md"), []byte("context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `---
type: overview
title: Project overview
description: Project scope
status: generated
sources:
  - path: context.md
    freshness: context
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
	if err != nil || !strings.Contains(string(persisted), "owner:\n    team: platform") || !strings.Contains(string(persisted), "freshness: context") {
		t.Fatalf("unknown metadata or source freshness was not preserved:\n%s\nerr=%v", persisted, err)
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

func setUnmergedGitIndex(t *testing.T, project, path string) {
	t.Helper()
	if !isGitRepository(project) {
		git(t, project, "init", "-q")
	}
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = project
	cmd.Stdin = strings.NewReader("conflicted evidence\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hashing conflicted evidence: %v", err)
	}
	oid := strings.TrimSpace(string(out))
	zero := strings.Repeat("0", len(oid))
	cmd = exec.Command("git", "update-index", "--index-info")
	cmd.Dir = project
	cmd.Stdin = strings.NewReader("0 " + zero + "\t" + path + "\n100644 " + oid + " 1\t" + path + "\n100644 " + oid + " 2\t" + path + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating unmerged index entry: %v\n%s", err, out)
	}
}

func pagesByID(pages []Page) map[string]Page {
	result := make(map[string]Page, len(pages))
	for _, page := range pages {
		result[page.ID] = page
	}
	return result
}

func writeSimplePage(t *testing.T, root, id, source string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(id+".md"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\ntype: guide\ntitle: " + id + "\ndescription: " + id + " description\nstatus: generated\nsources:\n  - path: " + source + "\n---\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
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
