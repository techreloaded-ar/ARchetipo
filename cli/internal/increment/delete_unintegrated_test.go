package increment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

// A PR closed without a merge leaves the increment published and says so: the
// person may reopen it, and the next refresh waits for the merge again.
func TestRefreshRecordsAPullRequestClosedWithoutMerge(t *testing.T) {
	s, r, hosting := publishedWork(t, nil)
	ctx := context.Background()
	if r.PullRequestClosedUnmerged() {
		t.Fatalf("an open PR was read as closed: %+v", r.PR)
	}
	hosting.pr.State = "CLOSED"
	r, err := s.Refresh(ctx, r.ID)
	if err != nil || r.Status != Proposed || !r.PullRequestClosedUnmerged() {
		t.Fatalf("the closed PR was not recorded: %+v %v", r, err)
	}
	if spec, readErr := s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode); readErr != nil || spec.Status != domain.StatusReview {
		t.Fatalf("closing the PR changed the backlog spec: %+v %v", spec, readErr)
	}
	hosting.pr.State = "OPEN"
	if r, err = s.Refresh(ctx, r.ID); err != nil || r.PullRequestClosedUnmerged() {
		t.Fatalf("a reopened PR was still read as closed: %+v %v", r, err)
	}
}

// Removing an increment that was never integrated takes its backlog spec with
// it: no merge is left to close that spec, and the original stories stay.
func TestDeletingAnUnintegratedIncrementRemovesItsBacklogSpec(t *testing.T) {
	s, r, hosting := publishedWork(t, []string{"US-010"})
	ctx := context.Background()
	hosting.pr.State = "CLOSED"
	r, err := s.Refresh(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RemovesBacklogSpec != r.BacklogSpecCode || plan.KeepsBacklogSpec != "" {
		t.Fatalf("the plan does not announce the spec it removes: %+v", plan)
	}
	options := DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}
	result, err := s.Delete(ctx, r.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted || !result.BacklogSpecSettled || result.BacklogSpecRemoved != r.BacklogSpecCode || result.EnvironmentAbandoned != nil {
		t.Fatalf("incomplete removal: %+v", result)
	}
	if _, err = os.Stat(r.Workdir); !os.IsNotExist(err) {
		t.Fatal("worktree retained", err)
	}
	items, err := s.Connector.FetchBacklogItems(ctx, "")
	if err != nil || len(items) != 1 || items[0].Code != "US-010" || items[0].Status != domain.StatusTodo {
		t.Fatalf("the backlog does not hold the original story alone: %+v %v", items, err)
	}
	receipt, err := s.DeletionReceipt(r.ID)
	if err != nil || receipt.BacklogSpecRemoved != r.BacklogSpecCode || receipt.Record.BacklogSpecCode != r.BacklogSpecCode {
		t.Fatalf("the receipt lost the trace of the removed spec: %+v %v", receipt, err)
	}
	if again, retryErr := s.Delete(ctx, r.ID, options); retryErr != nil || !again.Deleted {
		t.Fatalf("retry not idempotent: %+v %v", again, retryErr)
	}
}

// What was merged stays documented: the spec of a delivered increment is the
// trace of the integration and outlives the local environment.
func TestDeletingADeliveredIncrementKeepsItsBacklogSpec(t *testing.T) {
	s, r, hosting := publishedWork(t, nil)
	ctx := context.Background()
	hosting.pr.State = "MERGED"
	r, err := s.Refresh(ctx, r.ID)
	if err != nil || r.Status != Delivered {
		t.Fatalf("merge did not deliver the increment: %+v %v", r, err)
	}
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil || plan.RemovesBacklogSpec != "" {
		t.Fatalf("the plan removes the spec of a delivered increment: %+v %v", plan, err)
	}
	result, err := s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA})
	if err != nil || !result.Deleted || result.BacklogSpecRemoved != "" {
		t.Fatalf("removal of a delivered increment: %+v %v", result, err)
	}
	if spec, readErr := s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode); readErr != nil || spec.Status != domain.StatusDone {
		t.Fatalf("the delivered spec did not survive: %+v %v", spec, readErr)
	}
}

// The code of a removed spec is given again to the next story written: a
// deletion never removes a story that only happens to carry that code now.
func TestDeletionNeverRemovesAnotherStoryThatWasGivenTheSameCode(t *testing.T) {
	s, r, _ := publishedWork(t, []string{"US-010"})
	ctx := context.Background()
	published := r.BacklogSpecCode
	r.BacklogSpecCode = "US-010"
	if err := s.save(r); err != nil {
		t.Fatal(err)
	}
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil || plan.RemovesBacklogSpec != "" {
		t.Fatalf("the plan removes somebody else's story: %+v %v", plan, err)
	}
	result, err := s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA})
	if err != nil || !result.Deleted || result.BacklogSpecRemoved != "" {
		t.Fatalf("removal: %+v %v", result, err)
	}
	for _, code := range []string{"US-010", published} {
		if _, readErr := s.Connector.ReadSpecDetail(ctx, code); readErr != nil {
			t.Fatalf("a spec the increment does not own was removed: %s %v", code, readErr)
		}
	}
}

// Installed dependencies easily nest deeper than the path length Windows
// accepts by default: the checkout must still be removed whole.
func TestDeletionRemovesPathsBeyondThePlatformLimit(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	exclude := filepath.Join(s.Config.ProjectRoot, ".git", "info")
	if err := os.MkdirAll(exclude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exclude, "exclude"), []byte("installed/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(r.Workdir, "installed")
	for len(deep) < 400 {
		deep = filepath.Join(deep, strings.Repeat("d", 40))
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "module.js"), []byte("installed dependency"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA})
	if err != nil || !result.Deleted {
		t.Fatalf("a deep path stopped the removal: %+v %v", result, err)
	}
	if _, err = os.Stat(r.Workdir); !os.IsNotExist(err) {
		t.Fatal("worktree retained", err)
	}
}

// gitStopsHalfway reproduces what Git does when it cannot delete every file of
// a checkout (a path beyond the platform limit, a file still open): it drops
// its registration, leaves the directory and fails.
func gitStopsHalfway(t *testing.T) {
	t.Helper()
	original, patience := gitWorktreeRemove, leftoverRemovalPatience
	// The tests that keep the files busy do not wait for them to be released.
	leftoverRemovalPatience = 0
	t.Cleanup(func() { gitWorktreeRemove, leftoverRemovalPatience = original, patience })
	gitWorktreeRemove = func(ctx context.Context, projectRoot, workdir string) error {
		admin, err := git(ctx, workdir, "rev-parse", "--path-format=absolute", "--git-dir")
		if err != nil {
			return err
		}
		if err = os.RemoveAll(admin); err != nil {
			return err
		}
		return errors.New("error: failed to delete '" + workdir + "': Filename too long")
	}
}

// One confirmation is enough when Git stops halfway: the files it left are
// removed by the same deletion.
func TestDeletionFinishesWhatGitLeftBehind(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	gitStopsHalfway(t)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil || plan.Resuming {
		t.Fatalf("plan of a live checkout: %+v %v", plan, err)
	}
	result, err := s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA})
	if err != nil || !result.Deleted || !result.WorktreeRemoved || !result.BranchRemoved {
		t.Fatalf("removal stopped with Git: %+v %v", result, err)
	}
	if _, err = os.Stat(r.Workdir); !os.IsNotExist(err) {
		t.Fatal("leftover files retained", err)
	}
	if strings.Contains(runGit(t, s.Config.ProjectRoot, "worktree", "list", "--porcelain"), r.ID) {
		t.Fatal("Git registration retained")
	}
	if runGit(t, s.Config.ProjectRoot, "rev-parse", result.BackupRef) != plan.HeadSHA {
		t.Fatal("commit recovery ref missing")
	}
}

// A removal that stopped after saving the commits leaves a directory that is no
// longer a worktree. It is not a dead end: the plan says a removal is being
// resumed and the same confirmation finishes it, from either door.
func TestInterruptedDeletionIsResumedFromItsLeftovers(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "archive"}[preserve], func(t *testing.T) {
			ctx := context.Background()
			s := fixture(t)
			r := create(t, s)
			gitStopsHalfway(t)
			busy := errors.New("file in use")
			removeTree = func(string) error { return busy }
			t.Cleanup(func() { removeTree = os.RemoveAll })
			plan, err := s.DeletionPlan(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			options := DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}
			result, err := s.Delete(ctx, r.ID, options)
			if !errors.Is(err, busy) || result.Deleted || result.BackupRef == "" {
				t.Fatalf("a removal that could not finish was not reported as incomplete: %+v %v", result, err)
			}
			if _, statErr := os.Stat(filepath.Join(r.Workdir, "hello.txt")); statErr != nil {
				t.Fatal("the leftovers this test resumes from are missing", statErr)
			}
			if _, readErr := s.Read(r.ID); readErr != nil {
				t.Fatal("the interrupted removal lost the increment record", readErr)
			}

			resumed, err := s.DeletionPlan(ctx, r.ID)
			if err != nil || !resumed.Resuming || resumed.HeadSHA != plan.HeadSHA {
				t.Fatalf("the interrupted removal cannot be planned again: %+v %v", resumed, err)
			}
			if _, err = s.Delete(ctx, r.ID, options); !errors.Is(err, busy) {
				t.Fatalf("a retry with the files still in use: %v", err)
			}
			removeTree = os.RemoveAll
			options.PreserveConversations = preserve
			result, err = s.Delete(ctx, r.ID, options)
			if err != nil || !result.Deleted || !result.WorktreeRemoved || !result.BranchRemoved || result.ConversationsPreserved != preserve {
				t.Fatalf("could not resume: %+v %v", result, err)
			}
			if _, err = os.Stat(r.Workdir); !os.IsNotExist(err) {
				t.Fatal("leftover files retained", err)
			}
			if got := runGit(t, s.Config.ProjectRoot, "for-each-ref", "--format=%(refname)", "refs/heads/"+r.Branch); got != "" {
				t.Fatal("branch retained", got)
			}
			if runGit(t, s.Config.ProjectRoot, "rev-parse", result.BackupRef) != plan.HeadSHA {
				t.Fatal("commit recovery ref missing")
			}
			if records, listErr := s.List(); listErr != nil || len(records) != 0 {
				t.Fatalf("increment still indexed: %+v %v", records, listErr)
			}
		})
	}
}

// A recovery ref that no longer holds the confirmed commit never lets leftovers
// be removed: nothing proves that the work was saved.
func TestLeftoversAreKeptWithoutTheirRecoveryCommit(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	gitStopsHalfway(t)
	removeTree = func(string) error { return errors.New("file in use") }
	t.Cleanup(func() { removeTree = os.RemoveAll })
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	options := DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}
	result, err := s.Delete(ctx, r.ID, options)
	if err == nil {
		t.Fatal("the removal was expected to stop")
	}
	removeTree = os.RemoveAll
	runGit(t, s.Config.ProjectRoot, "update-ref", "-d", result.BackupRef)
	if resumed, planErr := s.DeletionPlan(ctx, r.ID); planErr == nil && resumed.Resuming {
		t.Fatalf("a removal without its recovery commit is offered again: %+v", resumed)
	}
	if _, err = s.Delete(ctx, r.ID, options); err == nil {
		t.Fatal("continued without a recovery ref")
	}
	if _, statErr := os.Stat(filepath.Join(r.Workdir, "hello.txt")); statErr != nil {
		t.Fatal("leftovers removed without their recovery commit", statErr)
	}
}

// failingCleanup leaves in the worktree an environment whose registered
// cleanup runs and fails, as a service that cannot be reached would.
func failingCleanup(t *testing.T, workdir string) {
	t.Helper()
	recipe := `{"version":1,"probe":[{"name":"git","argv":["git","--version"],"timeout_seconds":5}],"cleanup":[{"name":"drop-test-data","argv":["git","rev-parse","--verify","refs/heads/cleanup-target-that-does-not-exist"],"timeout_seconds":20}],"checks":{}}`
	if err := os.WriteFile(filepath.Join(workdir, verification.RecipePath), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(workdir, ".archetipo", "environment")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"checks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A cleanup that fails preserves the worktree and names its reason, and it is
// never a dead end: the explicit choice to leave the environment behind removes
// the increment and records what was left.
func TestDeletionWithFailedCleanupNeedsAnExplicitChoiceToProceed(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	failingCleanup(t, r.Workdir)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	options := DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}
	_, err = s.Delete(ctx, r.ID, options)
	var cleanup *EnvironmentCleanupError
	if !errors.As(err, &cleanup) {
		t.Fatalf("a failed cleanup was not reported as such: %v", err)
	}
	if !strings.HasPrefix(cleanup.Reason, "drop-test-data: ") || strings.HasPrefix(strings.TrimPrefix(cleanup.Reason, "drop-test-data: "), "{") || cleanup.ReportPath == "" {
		t.Fatalf("the failure does not name its step and reason: %+v", cleanup)
	}
	var coded *iox.CodedError
	if !errors.As(err, &coded) || coded.Code != iox.CodePreconditionMissing || !strings.Contains(coded.Message, cleanup.Reason) || !strings.Contains(coded.Hint, "--abandon-environment") {
		t.Fatalf("the CLI envelope does not carry the reason and the way forward: %+v", coded)
	}
	if _, statErr := os.Stat(filepath.Join(r.Workdir, "hello.txt")); statErr != nil {
		t.Fatal("a failed cleanup removed working files", statErr)
	}
	if _, readErr := s.Read(r.ID); readErr != nil {
		t.Fatal("a failed cleanup removed the increment record", readErr)
	}

	options.AbandonEnvironment = true
	result, err := s.Delete(ctx, r.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deleted || !result.WorktreeRemoved || !result.BranchRemoved {
		t.Fatalf("incomplete removal: %+v", result)
	}
	if result.EnvironmentAbandoned == nil || !strings.HasPrefix(result.EnvironmentAbandoned.Reason, "drop-test-data: ") {
		t.Fatalf("the removal does not say what it left behind: %+v", result.EnvironmentAbandoned)
	}
	if _, statErr := os.Stat(r.Workdir); !os.IsNotExist(statErr) {
		t.Fatal("worktree retained", statErr)
	}
	if runGit(t, s.Config.ProjectRoot, "rev-parse", result.BackupRef) != plan.HeadSHA {
		t.Fatal("commit recovery ref missing")
	}
	receipt, err := s.DeletionReceipt(r.ID)
	if err != nil || receipt.EnvironmentAbandoned == nil || receipt.EnvironmentAbandoned.Reason != result.EnvironmentAbandoned.Reason {
		t.Fatalf("the receipt does not retain what was left behind: %+v %v", receipt, err)
	}
}
