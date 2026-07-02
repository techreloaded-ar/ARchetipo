package gitwt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
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
	if mergeCount(t, root) != 0 {
		t.Fatalf("expected a fast-forward (no merge commit) when base had not moved")
	}
}

// mergeCount returns the number of merge commits reachable from the current
// HEAD, used to distinguish a fast-forward (0) from a real merge commit (>0).
func mergeCount(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command("git", "log", "--merges", "--oneline")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log --merges: %v\n%s", err, out)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// TestIntegrate_RealGit_MergeCommitWhenBaseDiverged verifies that when base has
// moved on since the branch forked (fast-forward not possible), Integrate
// falls back to an explicit merge commit so both sides' changes and an
// audit-trail commit are preserved.
func TestIntegrate_RealGit_MergeCommitWhenBaseDiverged(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	branch, worktreeRel, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	worktreeAbs := filepath.Join(root, worktreeRel)
	commitInWorktree(t, worktreeAbs, "b.txt", "hello\n", "add b")

	// Advance base independently (non-conflicting file) so fast-forward is
	// not possible.
	baseFile := filepath.Join(root, "base-only.txt")
	if err := os.WriteFile(baseFile, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "advance base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := Integrate(ctx, root, c, branch, worktreeRel); err != nil {
		t.Fatalf("Integrate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
		t.Fatalf("branch file missing on base after merge: %v", err)
	}
	if _, err := os.Stat(baseFile); err != nil {
		t.Fatalf("base file missing after merge: %v", err)
	}
	if mergeCount(t, root) == 0 {
		t.Fatalf("expected an explicit merge commit when base had diverged")
	}
	if refExists(ctx, root, branch) {
		t.Fatalf("branch not deleted after integrate")
	}
}

func TestEnsure_CopiesRootEnvFilesIntoNewWorktree(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	envFiles := map[string]string{
		".env":             "BASE_URL=http://localhost:3000\n",
		".env.local":       "TOKEN=local\n",
		".env.development": "MODE=development\n",
	}
	for name, body := range envFiles {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".env.d"), 0o755); err != nil {
		t.Fatalf("mkdir .env.d: %v", err)
	}

	_, worktreeRel, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	worktreeAbs := filepath.Join(root, worktreeRel)

	for name, want := range envFiles {
		got, err := os.ReadFile(filepath.Join(worktreeAbs, name))
		if err != nil {
			t.Fatalf("read copied %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("copied %s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(worktreeAbs, ".env.d")); !os.IsNotExist(err) {
		t.Fatalf("expected .env.d directory to be ignored, got err=%v", err)
	}
}

func TestEnsure_DoesNotOverwriteEnvFilesInExistingWorktree(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("TOKEN=root\n"), 0o600); err != nil {
		t.Fatalf("write root env: %v", err)
	}
	_, worktreeRel, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	worktreeAbs := filepath.Join(root, worktreeRel)
	worktreeEnv := filepath.Join(worktreeAbs, ".env.local")
	if err := os.WriteFile(worktreeEnv, []byte("TOKEN=worktree\n"), 0o600); err != nil {
		t.Fatalf("write worktree env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("TOKEN=changed-root\n"), 0o600); err != nil {
		t.Fatalf("update root env: %v", err)
	}

	if _, _, _, err := Ensure(ctx, root, c, "US-001", c.Base); err != nil {
		t.Fatalf("Ensure existing worktree: %v", err)
	}

	got, err := os.ReadFile(worktreeEnv)
	if err != nil {
		t.Fatalf("read worktree env: %v", err)
	}
	if string(got) != "TOKEN=worktree\n" {
		t.Fatalf("existing worktree env was overwritten: %q", got)
	}
}

// TestForkRef_StaleDoneBlockerBranch_RealGit reproduces the bug where a DONE
// blocker still carries stale branch metadata after its branch was deleted by
// integrate. ForkRef must ignore the stale branch and fork from base instead.
func TestForkRef_StaleDoneBlockerBranch_RealGit(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	if err := EnsureRepo(ctx, root, c.Base); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// Create and integrate US-001, which deletes archetipo/US-001.
	branch, worktreeRel, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure US-001: %v", err)
	}
	worktreeAbs := filepath.Join(root, worktreeRel)
	commitInWorktree(t, worktreeAbs, "b.txt", "hello\n", "add b")
	if err := Integrate(ctx, root, c, branch, worktreeRel); err != nil {
		t.Fatalf("Integrate US-001: %v", err)
	}
	if refExists(ctx, root, branch) {
		t.Fatal("expected branch archetipo/US-001 to be deleted after integrate")
	}

	// Simulate stale metadata: US-001 is DONE but still records the old branch.
	// This is what happens before the fix when Integrate doesn't clear metadata.
	staleDone := domain.Spec{
		Code:   "US-001",
		Status: domain.StatusDone,
		Branch: branch,
	}
	dependent := domain.Spec{
		Code:      "US-002",
		BlockedBy: []string{"US-001"},
	}
	allSpecs := []domain.Spec{staleDone, dependent}

	ref, err := ForkRef(ctx, root, c, dependent, allSpecs)
	if err != nil {
		t.Fatalf("ForkRef with stale DONE blocker should not error, got: %v", err)
	}
	if ref != c.Base {
		t.Fatalf("expected fork from base %q with stale DONE blocker, got %q", c.Base, ref)
	}
}

// TestForkRef_DeletedBranchOnNonDoneBlocker_RealGit tests that a non-DONE blocker
// whose branch was deleted is detected as a broken state and returns an error.
func TestForkRef_DeletedBranchOnNonDoneBlocker_RealGit(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	if err := EnsureRepo(ctx, root, c.Base); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// US-001 has a recorded branch that doesn't actually exist in git,
	// and US-001 is NOT DONE. This is a corrupted state.
	brokenBlocker := domain.Spec{
		Code:   "US-001",
		Status: domain.StatusInProgress,
		Branch: BranchName(c, "US-001"), // never created
	}
	dependent := domain.Spec{
		Code:      "US-002",
		BlockedBy: []string{"US-001"},
	}
	allSpecs := []domain.Spec{brokenBlocker, dependent}

	_, err := ForkRef(ctx, root, c, dependent, allSpecs)
	if err == nil {
		t.Fatal("expected error for non-DONE blocker with missing branch")
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
	// Advance base independently so the merge cannot fast-forward and a real
	// merge commit (requiring committer identity) is actually attempted.
	if err := os.WriteFile(filepath.Join(root, "base-only.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitOnBase := exec.Command("git", "add", ".")
	commitOnBase.Dir = root
	if out, err := commitOnBase.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	commitOnBaseCommit := exec.Command("git", "commit", "-q", "-m", "advance base")
	commitOnBaseCommit.Dir = root
	commitOnBaseCommit.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
	if out, err := commitOnBaseCommit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

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
