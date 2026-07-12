package wiki

import (
	"os"
	"path/filepath"
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

func TestMigrateArchivesLegacyArtifacts(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "PRD.md"), []byte("# PRD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := Migrate(project, root, "docs/PRD.md", "docs/CODEMAP.md")
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "prd.md")); err != nil {
		t.Fatal(err)
	}
}
