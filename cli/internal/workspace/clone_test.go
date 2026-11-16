package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitFixture(t *testing.T, withConfig bool) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "demo-repo")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = src
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(src, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if withConfig {
		if err := os.WriteFile(filepath.Join(src, ".archetipo", "config.yaml"), []byte("connector: file\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("init", "-q", "-b", "main")
	run("-c", "user.name=t", "-c", "user.email=t@example.test", "add", "-A")
	run("-c", "user.name=t", "-c", "user.email=t@example.test", "commit", "-q", "-m", "init")
	return src
}

func allowFileClones(t *testing.T) {
	t.Helper()
	allowedCloneSchemes["file"] = true
	t.Cleanup(func() { delete(allowedCloneSchemes, "file") })
}

func fieldCode(t *testing.T, err error) string {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) || len(invalid.Fields) != 1 || invalid.Fields[0].Field != "url" {
		t.Fatalf("expected one url field error, got %v", err)
	}
	return invalid.Fields[0].Code
}

func TestCloneReportsWhetherTheRepositoryIsAlreadyAWorkspace(t *testing.T) {
	allowFileClones(t)
	base := t.TempDir()
	plain := gitFixture(t, false)
	result, err := Clone(context.Background(), base, "file://"+plain)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dir != filepath.Join(base, "demo-repo") || result.Name != "demo-repo" || result.Initialized {
		t.Fatalf("plain repository: %+v", result)
	}
	if _, err = os.Stat(filepath.Join(result.Dir, "README.md")); err != nil {
		t.Fatal("clone content missing")
	}
	if code := fieldCode(t, mustFail(Clone(context.Background(), base, "file://"+plain))); code != CodeCloneDestination {
		t.Fatalf("second clone of the same name: %s", code)
	}

	other := t.TempDir()
	result, err = Clone(context.Background(), other, "file://"+gitFixture(t, true))
	if err != nil || !result.Initialized {
		t.Fatalf("workspace repository: %+v %v", result, err)
	}
}

func TestCloneRefusesUnsafeURLsAndLeavesNothingBehind(t *testing.T) {
	base := t.TempDir()
	for raw, want := range map[string]string{
		"":                              CodeCloneURLInvalid,
		"git@github.com:owner/repo.git": CodeCloneURLInvalid,
		"http://github.com/owner/repo":  CodeCloneURLInvalid,
		"ssh://github.com/owner/repo":   CodeCloneURLInvalid,
		"https://user:token@github.com/owner/repo": CodeCloneURLInvalid,
		"https://github.com/":                      CodeCloneURLInvalid,
		"https://github.com/owner/..":              CodeCloneURLInvalid,
		"https://github.com/owner/repo?x=1":        CodeCloneURLInvalid,
	} {
		if code := fieldCode(t, mustFail(Clone(context.Background(), base, raw))); code != want {
			t.Fatalf("%q: code %s", raw, code)
		}
	}
	if code := fieldCode(t, mustFail(Clone(context.Background(), "", "https://github.com/owner/repo"))); code != CodeCloneNotOffered {
		t.Fatalf("clone without a directory: %s", code)
	}
	allowFileClones(t)
	if code := fieldCode(t, mustFail(Clone(context.Background(), base, "file://"+filepath.Join(t.TempDir(), "missing")))); code != CodeCloneUnreachable {
		t.Fatalf("unreachable repository: %s", code)
	}
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Fatalf("refused clones left %d entries", len(entries))
	}
}

func mustFail(_ CloneResult, err error) error { return err }
