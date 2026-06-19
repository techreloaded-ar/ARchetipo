package gitwt

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// Unit tests for ForkRef, now using real git repos when git branch refs must
// exist. No-blocker and no-branch tests still use non-git dirs; stacking and
// conflict tests use real git via helpers in gitwt_git_test.go.

func cfg() domain.WorktreeConfig {
	return domain.WorktreeConfig{Enabled: true, Base: "main", Dir: ".wt", BranchPrefix: "archetipo/"}
}

func TestForkRef_NoBlockers_ForksFromBase(t *testing.T) {
	spec := domain.Spec{Code: "US-001"}
	ref, err := ForkRef(context.Background(), t.TempDir(), cfg(), spec, []domain.Spec{spec})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "main" {
		t.Fatalf("want base 'main', got %q", ref)
	}
}

func TestForkRef_BlockerWithoutBranch_IsIntegrated(t *testing.T) {
	all := []domain.Spec{
		{Code: "US-001"}, // no branch -> considered integrated
		{Code: "US-002", BlockedBy: []string{"US-001"}},
	}
	ref, err := ForkRef(context.Background(), t.TempDir(), cfg(), all[1], all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "main" {
		t.Fatalf("want base 'main', got %q", ref)
	}
}

func TestForkRef_SingleUnmergedBlocker_StacksOnBranch(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	if err := EnsureRepo(ctx, root, c.Base); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// Create US-001 branch, commit to advance it beyond base.
	branch, worktreeRel, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure US-001: %v", err)
	}
	wt := filepath.Join(root, worktreeRel)
	commitInWorktree(t, wt, "us001.txt", "feature\n", "US-001 work")

	all := []domain.Spec{
		{Code: "US-001", Branch: branch},
		{Code: "US-002", BlockedBy: []string{"US-001"}},
	}
	ref, err := ForkRef(ctx, root, c, all[1], all)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != branch {
		t.Fatalf("want stack on blocker branch %q, got %q", branch, ref)
	}
}

func TestForkRef_MultipleUnmergedBlockers_Conflict(t *testing.T) {
	root := initRepo(t)
	ctx := context.Background()
	c := cfg()

	if err := EnsureRepo(ctx, root, c.Base); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	// Create both blocker branches and advance them beyond base.
	b1, wt1, _, err := Ensure(ctx, root, c, "US-001", c.Base)
	if err != nil {
		t.Fatalf("Ensure US-001: %v", err)
	}
	commitInWorktree(t, filepath.Join(root, wt1), "u1.txt", "a\n", "US-001 work")
	b2, wt2, _, err := Ensure(ctx, root, c, "US-002", c.Base)
	if err != nil {
		t.Fatalf("Ensure US-002: %v", err)
	}
	commitInWorktree(t, filepath.Join(root, wt2), "u2.txt", "b\n", "US-002 work")

	all := []domain.Spec{
		{Code: "US-001", Branch: b1},
		{Code: "US-002", Branch: b2},
		{Code: "US-003", BlockedBy: []string{"US-001", "US-002"}},
	}
	_, err = ForkRef(ctx, root, c, all[2], all)
	if err == nil {
		t.Fatal("expected a conflict error for multiple unmerged blockers")
	}
}
