package gitwt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// initRepo creates a real git repository with one commit on `main` and returns
// its path. It skips the test when git is unavailable.
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "archetipo-test@example.com")
	run("config", "user.name", "ARchetipo Test")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return root
}

func initRepoWithoutLocalIdentity(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	// Without this, git silently falls back to username@hostname on machines
	// with a fully qualified hostname and the commit succeeds anyway.
	run("config", "user.useConfigOnly", "true")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return root
}

func commitInWorktree(t *testing.T, worktree, file, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(worktree, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", msg}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = worktree
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestEnsureDiffIntegrate_RealGit(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	if err := EnsureRepo(ctx, root, c.Base); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	branch, worktreeRel, forkBase, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if branch != "archetipo/US-001" {
		t.Fatalf("unexpected branch %q", branch)
	}
	worktreeAbs := filepath.Join(root, worktreeRel)
	if _, err := os.Stat(worktreeAbs); err != nil {
		t.Fatalf("worktree not created: %v", err)
	}

	commitInWorktree(t, worktreeAbs, "b.txt", "hello\n", "add b")

	files, err := Diff(ctx, root, forkBase, branch)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	found := false
	for _, f := range files {
		if f.NewPath == "b.txt" && f.Status == "added" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diff did not isolate the spec change, got %+v", files)
	}

	ahead, behind, err := AheadBehind(ctx, root, c.Base, branch)
	if err != nil || ahead != 1 || behind != 0 {
		t.Fatalf("AheadBehind = (%d,%d,%v), want (1,0,nil)", ahead, behind, err)
	}

	if err := Integrate(ctx, root, c, branch, worktreeRel); err != nil {
		t.Fatalf("Integrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
		t.Fatalf("integrated file missing on base: %v", err)
	}
	if _, err := os.Stat(worktreeAbs); !os.IsNotExist(err) {
		t.Fatalf("worktree not removed after integrate")
	}
	if refExists(ctx, root, branch) {
		t.Fatalf("branch not deleted after integrate")
	}
}

func TestIntegrate_RealGit_MissingCommitterIdentityIsNotReportedAsConflict(t *testing.T) {
	root := initRepoWithoutLocalIdentity(t)
	isolatedHomeDir := t.TempDir()
	t.Setenv("HOME", isolatedHomeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(isolatedHomeDir, "xdg"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	ctx := context.Background()
	c := cfg()
	branch, worktreeRel, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	worktreeAbs := filepath.Join(root, worktreeRel)
	commitInWorktree(t, worktreeAbs, "b.txt", "hello\n", "add b")

	err = Integrate(ctx, root, c, branch, worktreeRel)
	if err == nil {
		t.Fatal("expected integrate to fail without a configured git identity")
	}

	var codedErr *iox.CodedError
	if !errors.As(err, &codedErr) {
		t.Fatalf("expected coded error, got %T: %v", err, err)
	}
	if codedErr.Code != iox.CodePreconditionMissing {
		t.Fatalf("expected %s, got %s (%v)", iox.CodePreconditionMissing, codedErr.Code, err)
	}
	if codedErr.Message != "git committer identity is not configured" {
		t.Fatalf("unexpected message %q", codedErr.Message)
	}
}
