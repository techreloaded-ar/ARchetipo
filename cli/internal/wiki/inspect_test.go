package wiki

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInspectSingleProjectWithoutPRD(t *testing.T) {
	root := t.TempDir()
	writeInspectionFile(t, root, "package.json", `{"scripts":{"test":"jest"}}`)
	writeInspectionFile(t, root, "src/index.ts", "export const app = true")
	writeInspectionFile(t, root, "src/app/api/health/route.ts", "export function GET() {}")
	writeInspectionFile(t, root, "prisma/schema.prisma", "model User { id String @id }")
	writeInspectionFile(t, root, "src/index.test.ts", "test('app', () => {})")
	writeInspectionFile(t, root, "README.md", "# Project")

	got, err := Inspect(root, filepath.Join(root, "docs/wiki"), "docs/PRD.md")
	if err != nil {
		t.Fatal(err)
	}
	if got.PRD.Present {
		t.Fatal("PRD should be optional and absent")
	}
	if !hasBoundary(got, "src") || len(got.EntryPoints) == 0 || len(got.Routes) == 0 || len(got.Schemas) == 0 || len(got.Tests) == 0 {
		t.Fatalf("incomplete inspection: %+v", got)
	}
}

func TestInspectMonorepoBoundaries(t *testing.T) {
	root := t.TempDir()
	writeInspectionFile(t, root, "package.json", `{"workspaces":["apps/*","packages/*"]}`)
	writeInspectionFile(t, root, "apps/web/package.json", `{}`)
	writeInspectionFile(t, root, "apps/web/src/index.ts", "export {}")
	writeInspectionFile(t, root, "packages/core/package.json", `{}`)
	writeInspectionFile(t, root, "packages/core/src/index.ts", "export {}")

	got, err := Inspect(root, filepath.Join(root, "docs/wiki"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !hasBoundary(got, "apps/web") || !hasBoundary(got, "packages/core") {
		t.Fatalf("monorepo boundaries missing: %+v", got.Boundaries)
	}
}

func TestInspectUnknownStackAndNoEvidence(t *testing.T) {
	root := t.TempDir()
	writeInspectionFile(t, root, "README.md", "# Unknown stack")
	writeInspectionFile(t, root, "engine/source.xyz", "opaque")
	if _, err := Inspect(root, filepath.Join(root, "wiki"), ""); err != nil {
		t.Fatalf("documentation should be sufficient evidence: %v", err)
	}

	empty := t.TempDir()
	if _, err := Inspect(empty, filepath.Join(empty, "wiki"), ""); !errors.Is(err, ErrNoProjectEvidence) {
		t.Fatalf("expected ErrNoProjectEvidence, got %v", err)
	}
}

func TestInspectOmitsIgnoredAndSensitiveFilesAndContents(t *testing.T) {
	root := t.TempDir()
	writeInspectionFile(t, root, ".gitignore", "ignored/\n.env\n")
	writeInspectionFile(t, root, "go.mod", "module example.test/project")
	writeInspectionFile(t, root, "main.go", "package main")
	writeInspectionFile(t, root, ".env", "SUPER_SECRET=do-not-leak")
	writeInspectionFile(t, root, "private.key", "do-not-leak-key")
	writeInspectionFile(t, root, "ignored/hidden.ts", "do-not-leak-ignored")
	git(t, root, "init", "-q")

	got, err := Inspect(root, filepath.Join(root, "docs/wiki"), "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SUPER_SECRET", "do-not-leak", "ignored/hidden.ts", ".env", "private.key"} {
		if contains(string(raw), forbidden) {
			t.Fatalf("inspection leaked %q: %s", forbidden, raw)
		}
	}
}

func hasBoundary(inspection Inspection, path string) bool {
	for _, boundary := range inspection.Boundaries {
		if boundary.Path == path {
			return true
		}
	}
	return false
}

func writeInspectionFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
