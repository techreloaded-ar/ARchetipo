package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleSearchAffectedAndPublish(t *testing.T) {
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
id: architecture.auth
type: architecture
summary: Authentication boundaries and token flow
status: draft
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
	items, err := Search(root, "token", "", "", false)
	if err != nil || len(items) != 1 {
		t.Fatalf("search: items=%d err=%v", len(items), err)
	}
	affected, err := Affected(project, root, []string{"src/auth.go"})
	if err != nil || len(affected) != 1 {
		t.Fatalf("affected: items=%d err=%v", len(affected), err)
	}
	published, err := Publish(project, root)
	if err != nil || published != 1 {
		t.Fatalf("publish: count=%d err=%v", published, err)
	}
	loaded, err := Load(root, false)
	if err != nil || loaded[0].Meta.Status != "verified" {
		t.Fatalf("load after publish: %+v err=%v", loaded, err)
	}
	if _, err := os.Stat(filepath.Join(root, "index.md")); err != nil {
		t.Fatal(err)
	}
}

func TestInitCreatesOnlySourceSection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "sources")); err != nil || !info.IsDir() {
		t.Fatalf("sources section missing or not a directory: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(root, "components")); !os.IsNotExist(err) {
		t.Fatalf("semantic section should not be created before a page needs it: %v", err)
	}
}

func TestValidateRejectsNoncanonicalPagePath(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := `---
id: architecture.auth
type: architecture
summary: Authentication boundaries
status: draft
---
# Authentication
`
	if err := os.WriteFile(filepath.Join(root, "architecture-auth.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK {
		t.Fatal("expected noncanonical path to fail validation")
	}
	if len(report.Findings) != 1 || report.Findings[0].Code != "WIKI_NONCANONICAL_PATH" {
		t.Fatalf("findings: %+v", report.Findings)
	}
	if report.Findings[0].Message != "page id architecture.auth must live at architecture/auth.md" {
		t.Fatalf("message: %s", report.Findings[0].Message)
	}
}

func TestValidateBrokenLinksAndStaleSources(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := `---
id: domains.billing
type: domain
summary: Billing rules
status: verified
links:
  - id: missing.page
sources:
  - path: src/missing.go
---
# Billing
`
	if err := os.MkdirAll(filepath.Join(root, "domains"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "domains", "billing.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK {
		t.Fatal("expected invalid report")
	}
	codes := map[string]bool{}
	for _, finding := range report.Findings {
		codes[finding.Code] = true
	}
	if !codes["WIKI_BROKEN_LINK"] || !codes["WIKI_STALE_SOURCE"] {
		t.Fatalf("findings: %+v", report.Findings)
	}
}

func TestValidateRejectsBrokenBodyWikiLink(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	page := `---
id: overview
type: overview
summary: Project overview
status: draft
---
# Overview

See [[missing.page]].
`
	if err := os.WriteFile(filepath.Join(root, "overview.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_BROKEN_BODY_LINK") {
		t.Fatalf("expected broken body link finding: %+v", report.Findings)
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
	writeCorePage(t, root, "architecture", "architecture", "src/index.ts", "")
	writeCorePage(t, root, "operations.development", "operations", "package.json", "")
	coverage := `coverage:
  - path: .
    status: documented
    pages: [overview]
  - path: src
    status: documented
    pages: [architecture]
`
	writeCorePage(t, root, "engineering.code-map", "engineering", "src", coverage)

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
	raw = []byte(strings.Replace(string(raw), "  - path: src\n    status: documented\n    pages: [architecture]\n", "", 1))
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
	writeCorePage(t, root, "architecture", "architecture", "README.md", "")
	writeCorePage(t, root, "operations.development", "operations", "README.md", "")
	writeCorePage(t, root, "engineering.code-map", "engineering", "README.md", "")
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
	extra := "coverage:\n  - path: legacy\n    status: excluded\n"
	writeCorePage(t, root, "engineering.code-map", "engineering", "README.md", extra)
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

func TestCatalogPreservesDraftStatus(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	writeCorePage(t, root, "overview", "overview", "README.md", "")
	if _, err := Catalog(root); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "| draft |") {
		t.Fatalf("catalog should preserve draft status:\n%s", index)
	}
}

func writeCorePage(t *testing.T, root, id, pageType, source, extra string) {
	t.Helper()
	rel := canonicalPagePath(id)
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: " + id + "\ntype: " + pageType + "\nsummary: " + id + " summary\nstatus: draft\nsources:\n  - path: " + source + "\n" + extra + "---\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasFinding(report Report, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
