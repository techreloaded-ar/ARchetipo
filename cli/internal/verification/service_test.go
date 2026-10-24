package verification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
)

type testRunner struct {
	calls []string
	run   func(context.Context, string, Command) error
}

func (r *testRunner) Run(ctx context.Context, root string, _ []string, command Command, out io.Writer) error {
	r.calls = append(r.calls, command.Name)
	_, _ = io.WriteString(out, strings.Repeat(command.Name+" complete output\n", 1000))
	if r.run != nil {
		return r.run(ctx, root, command)
	}
	return nil
}

func fixture(t *testing.T) (*Service, *testRunner, Recipe) {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "lock.json"), "baseline")
	mustWrite(t, filepath.Join(root, "src", "app.txt"), "first candidate")
	recipe := Recipe{Version: 1, Inputs: []string{"lock.json"}, Probe: []Command{{Name: "ready", Argv: []string{"go", "version"}, TimeoutSeconds: 1}}, Prepare: []Command{{Name: "install", Argv: []string{"go", "version"}, TimeoutSeconds: 1}}, Checks: map[string]Check{"focused": {Command: Command{Name: "test", Argv: []string{"go", "version"}, TimeoutSeconds: 1}, Inputs: []string{"src"}, Cache: true}}}
	writeRecipe(t, root, recipe)
	runner := &testRunner{}
	s := New(root, "evidence")
	s.Runner = runner
	return s, runner, recipe
}

func writeRecipe(t *testing.T, root string, recipe Recipe) {
	t.Helper()
	raw, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, RecipePath), string(raw))
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func execute(t *testing.T, s *Service, operation string) Result {
	t.Helper()
	r, err := s.Execute(context.Background(), operation, "focused", "US-001", false)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWarmRunReusesSetupAndEvidenceButRechecksReadiness(t *testing.T) {
	s, runner, _ := fixture(t)
	first := execute(t, s, "check")
	second := execute(t, s, "check")
	if !first.Passed || !first.Stable || first.Reused || !second.Passed || !second.Reused || second.ReusedReport != first.ReportPath {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if strings.Join(runner.calls, ",") != "install,ready,test,ready" {
		t.Fatalf("repeated work or skipped readiness: %v", runner.calls)
	}
	if !strings.Contains(first.ReportPath, filepath.Join("evidence", "US-001")) {
		t.Fatal(first.ReportPath)
	}
	log, err := os.ReadFile(first.Steps[0].LogPath)
	if err != nil || strings.Count(string(log), "complete output") != 1000 {
		t.Fatal("full output was not retained", err)
	}
}

func TestInputChangeInvalidatesCheckWithoutReinstalling(t *testing.T) {
	s, runner, _ := fixture(t)
	first := execute(t, s, "check")
	mustWrite(t, filepath.Join(s.Root, "src", "new-untracked-file.txt"), "new behavior")
	second := execute(t, s, "check")
	if second.Reused || first.InputKey == second.InputKey || strings.Join(runner.calls, ",") != "install,ready,test,ready,test" {
		t.Fatalf("stale result or redundant setup: %+v %v", second, runner.calls)
	}
}

func TestDependencyChangeOrMissingOutputReprepares(t *testing.T) {
	s, runner, recipe := fixture(t)
	recipe.Outputs = []string{"local-deps/sentinel"}
	writeRecipe(t, s.Root, recipe)
	runner.run = func(_ context.Context, root string, command Command) error {
		if command.Name == "install" {
			mustWrite(t, filepath.Join(root, "local-deps", "sentinel"), "owned")
		}
		return nil
	}
	first := execute(t, s, "check")
	mustWrite(t, filepath.Join(s.Root, "lock.json"), "new dependency")
	second := execute(t, s, "check")
	if second.Reused || first.PreparationID == second.PreparationID {
		t.Fatal("dependency update reused setup")
	}
	if err := os.Remove(filepath.Join(s.Root, "local-deps", "sentinel")); err != nil {
		t.Fatal(err)
	}
	third := execute(t, s, "check")
	if third.Reused || third.PreparationID == second.PreparationID {
		t.Fatal("missing output reused setup")
	}
}

func TestDeadServiceIsRepairedOnceAndInvalidatesEvidence(t *testing.T) {
	s, runner, _ := fixture(t)
	first := execute(t, s, "check")
	failNext := true
	runner.run = func(_ context.Context, _ string, command Command) error {
		if command.Name == "ready" && failNext {
			failNext = false
			return errors.New("service unavailable")
		}
		return nil
	}
	second := execute(t, s, "check")
	if second.Reused || !second.Passed || first.PreparationID == second.PreparationID || strings.Join(runner.calls, ",") != "install,ready,test,ready,install,ready,test" {
		t.Fatalf("recovery: %+v %v", second, runner.calls)
	}
}

func TestFailureNeverBecomesCachedSuccess(t *testing.T) {
	s, runner, _ := fixture(t)
	runner.run = func(_ context.Context, _ string, command Command) error {
		if command.Name == "test" {
			return errors.New("assertion failed")
		}
		return nil
	}
	first := execute(t, s, "check")
	second := execute(t, s, "check")
	if first.Passed || second.Passed || second.Reused || first.ReportPath == second.ReportPath || first.Steps[0].ExitCode != -1 {
		t.Fatalf("failure lost or reused: %+v %+v", first, second)
	}
	if _, err := os.Stat(first.Steps[0].LogPath); err != nil {
		t.Fatal("failed evidence overwritten", err)
	}
}

func TestFreshFailureRevokesEarlierPass(t *testing.T) {
	s, runner, _ := fixture(t)
	if !execute(t, s, "check").Passed {
		t.Fatal("baseline failed")
	}
	runner.run = func(_ context.Context, _ string, command Command) error {
		if command.Name == "test" {
			return errors.New("a fresh run found a defect")
		}
		return nil
	}
	fresh, err := s.Execute(context.Background(), "check", "focused", "US-001", true)
	if err != nil || fresh.Passed {
		t.Fatal("fresh failure lost", err)
	}
	next := execute(t, s, "check")
	if next.Reused || next.Passed {
		t.Fatal("old success survived a newer failed check")
	}
}

func TestCorruptCheckpointCanBeCleaned(t *testing.T) {
	s, _, _ := fixture(t)
	mustWrite(t, filepath.Join(s.dir(), "state.json"), "corrupt")
	if _, err := s.Execute(context.Background(), "prepare", "", "", false); err == nil {
		t.Fatal("corruption ignored")
	}
	if !execute(t, s, "cleanup").Passed || !execute(t, s, "prepare").Passed {
		t.Fatal("cleanup did not recover the checkpoint")
	}
}

func TestSymlinkOutputsAndInputsAreRejected(t *testing.T) {
	s, runner, recipe := fixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.Root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	recipe.Outputs = []string{"linked/client.js"}
	writeRecipe(t, s.Root, recipe)
	if _, err := s.Execute(context.Background(), "prepare", "", "", false); err == nil || len(runner.calls) != 0 {
		t.Fatal("ancestor-owned dependency path accepted")
	}
}

func TestChangedRecipeDuringCheckCannotBeCertified(t *testing.T) {
	s, runner, recipe := fixture(t)
	runner.run = func(_ context.Context, root string, command Command) error {
		if command.Name == "test" {
			recipe.Checks["different-contract"] = recipe.Checks["focused"]
			writeRecipe(t, root, recipe)
		}
		return nil
	}
	if result := execute(t, s, "check"); result.Passed || result.Stable {
		t.Fatal("changed verification contract was certified")
	}
}

func TestMutationDuringCheckDoesNotCertifyCurrentSource(t *testing.T) {
	s, runner, _ := fixture(t)
	runner.run = func(_ context.Context, root string, command Command) error {
		if command.Name == "test" {
			mustWrite(t, filepath.Join(root, "src", "app.txt"), "changed while testing")
		}
		return nil
	}
	first := execute(t, s, "check")
	if first.Passed || first.Stable || first.Invalidation == "" || first.Steps[0].ExitCode != 0 {
		t.Fatalf("moving candidate was certified: %+v", first)
	}
}

func TestCleanupInvalidatesEvenWhenItFails(t *testing.T) {
	s, runner, recipe := fixture(t)
	recipe.Cleanup = []Command{{Name: "stop", Argv: []string{"go", "version"}, TimeoutSeconds: 1}}
	writeRecipe(t, s.Root, recipe)
	first := execute(t, s, "check")
	runner.run = func(_ context.Context, _ string, command Command) error {
		if command.Name == "stop" {
			return errors.New("cleanup interrupted")
		}
		return nil
	}
	clean := execute(t, s, "cleanup")
	second := execute(t, s, "check")
	if clean.Passed || second.Reused || first.PreparationID == second.PreparationID {
		t.Fatal("cleanup left a reusable checkpoint")
	}
}

func TestMissingOrModifiedEvidenceRunsAgain(t *testing.T) {
	s, _, _ := fixture(t)
	first := execute(t, s, "check")
	mustWrite(t, first.Steps[0].LogPath, "altered evidence")
	second := execute(t, s, "check")
	if second.Reused {
		t.Fatal("altered evidence reused")
	}
	if err := os.Remove(second.ReportPath); err != nil {
		t.Fatal(err)
	}
	if execute(t, s, "check").Reused {
		t.Fatal("absent evidence reused")
	}
}

func TestEnvironmentInputsAndRecipeChangesInvalidate(t *testing.T) {
	s, runner, recipe := fixture(t)
	recipe.EnvKeys = []string{"ARCHETIPO_TEST_RECIPE_INPUT"}
	writeRecipe(t, s.Root, recipe)
	t.Setenv("ARCHETIPO_TEST_RECIPE_INPUT", "first")
	first := execute(t, s, "check")
	t.Setenv("ARCHETIPO_TEST_RECIPE_INPUT", "second")
	second := execute(t, s, "check")
	if second.Reused || first.PreparationID == second.PreparationID {
		t.Fatal("environment input change was ignored")
	}
	before := strings.Join(runner.calls, ",")
	recipe.Checks["another"] = recipe.Checks["focused"]
	writeRecipe(t, s.Root, recipe)
	third := execute(t, s, "check")
	if third.Reused || strings.Join(runner.calls, ",") != before+",ready,test" {
		t.Fatal("check recipe change reused evidence or reinstalled dependencies", runner.calls)
	}
}

func TestInvalidRecipeFailsBeforeCommands(t *testing.T) {
	for _, path := range []string{"../outside", "/tmp/outside", ".", "src/../outside", `C:\outside`} {
		t.Run(path, func(t *testing.T) {
			s, runner, recipe := fixture(t)
			recipe.Inputs = []string{path}
			writeRecipe(t, s.Root, recipe)
			if _, err := s.Execute(context.Background(), "prepare", "", "", false); err == nil || len(runner.calls) != 0 {
				t.Fatal("invalid path reached runner")
			}
		})
	}
}

func TestConcurrentCallFailsPromptly(t *testing.T) {
	s, runner, _ := fixture(t)
	store, err := conversationlog.NewFileStore(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := store.TryLock(context.Background(), "verification-environment")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = s.Execute(ctx, "prepare", "", "", false); err == nil || len(runner.calls) != 0 || ctx.Err() != nil {
		t.Fatal("concurrent operation waited or ran", err)
	}
}

func TestCancellationLeavesNoReusablePreparation(t *testing.T) {
	s, runner, _ := fixture(t)
	runner.run = func(ctx context.Context, _ string, _ Command) error { return ctx.Err() }
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel during the first command, after acquisition of the ownership lock.
	runner.run = func(ctx context.Context, _ string, _ Command) error { cancel(); return ctx.Err() }
	result, err := s.Execute(ctx, "prepare", "", "", false)
	if err != nil || result.Passed || len(result.Steps) != 1 || result.Steps[0].Error != "context canceled" {
		t.Fatalf("cancellation not persisted: %+v %v", result, err)
	}
	runner.run = nil
	if next := execute(t, s, "prepare"); next.Reused || !next.Passed {
		t.Fatalf("cancelled setup was reused: %+v", next)
	}
}

func TestMissingPreparationEvidenceRequiresFreshSetup(t *testing.T) {
	s, runner, _ := fixture(t)
	first := execute(t, s, "prepare")
	if err := os.Remove(first.ReportPath); err != nil {
		t.Fatal(err)
	}
	second := execute(t, s, "prepare")
	if second.Reused || !second.Passed || first.PreparationID == second.PreparationID {
		t.Fatal("reused a preparation whose original evidence was removed")
	}
	if strings.Join(runner.calls, ",") != "install,ready,install,ready" {
		t.Fatalf("setup was not repeated: %v", runner.calls)
	}
}

func TestPreparationLogsAreBoundedAndConfinedToCheckout(t *testing.T) {
	s, _, _ := fixture(t)
	result := execute(t, s, "prepare")
	logs, err := s.Logs(result, 32)
	if err != nil || len(logs) > 32 {
		t.Fatalf("invalid bounded logs: %q %v", logs, err)
	}
	result.Steps[0].LogPath = filepath.Join(t.TempDir(), "unrelated.log")
	if _, err = s.Logs(result, 32); err == nil {
		t.Fatal("accepted a log outside the checkout's environment reports")
	}
}

func TestEvidenceRequiresCurrentOriginalReports(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	if _, err := s.Evidence(ctx); err == nil {
		t.Fatal("accepted narrative without reports")
	}
	first := execute(t, s, "check")
	evidence, err := s.Evidence(ctx)
	if err != nil || len(evidence) != 1 || evidence[0].ID != first.ID {
		t.Fatalf("original evidence: %+v %v", evidence, err)
	}
	execute(t, s, "check") // Reuse must still reference the original.
	evidence, err = s.Evidence(ctx)
	if err != nil || evidence[0].ID != first.ID {
		t.Fatal("reuse replaced original evidence", err)
	}
	mustWrite(t, filepath.Join(s.Root, "src", "app.txt"), "changed")
	if _, err = s.Evidence(ctx); err == nil {
		t.Fatal("accepted stale sources")
	}
	current := execute(t, s, "check")
	mustWrite(t, current.Steps[0].LogPath, "tampered")
	if _, err = s.Evidence(ctx); err == nil {
		t.Fatal("accepted missing/tampered original logs")
	}
}

func TestLiveCheckDoesNotRepairOrStopPreviewEnvironment(t *testing.T) {
	s, runner, recipe := fixture(t)
	check := recipe.Checks["focused"]
	check.DuringPreview = true
	recipe.Checks["focused"] = check
	writeRecipe(t, s.Root, recipe)
	execute(t, s, "prepare")
	store, _ := conversationlog.NewFileStore(s.Root)
	owner, err := store.TryLock(context.Background(), "preview-environment")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Unlock() }()
	runner.calls = nil
	result := execute(t, s, "check")
	if !result.Passed || strings.Join(runner.calls, ",") != "test" {
		t.Fatalf("live check changed preparation: %+v %v", result, runner.calls)
	}
	if _, err = s.Evidence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(context.Background(), "prepare", "", "", false); err == nil {
		t.Fatal("prepared during preview")
	}
	mustWrite(t, filepath.Join(s.Root, "lock.json"), "dependency changed")
	runner.calls = nil
	if _, err = s.Execute(context.Background(), "check", "focused", "", false); err == nil || len(runner.calls) != 0 {
		t.Fatal("live check repaired dependencies", err, runner.calls)
	}
}

func TestLiveCheckCancellationWhenPreviewOwnerStops(t *testing.T) {
	s, runner, recipe := fixture(t)
	check := recipe.Checks["focused"]
	check.DuringPreview = true
	recipe.Checks["focused"] = check
	writeRecipe(t, s.Root, recipe)
	execute(t, s, "prepare")
	store, _ := conversationlog.NewFileStore(s.Root)
	owner, err := store.TryLock(context.Background(), "preview-environment")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	runner.run = func(ctx context.Context, _ string, c Command) error {
		if c.Name == "test" {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Execute(context.Background(), "check", "focused", "", true) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("check did not start")
	}
	_ = owner.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("preview stopped but check kept running")
	}
	if _, err = s.Evidence(context.Background()); err == nil {
		t.Fatal("canceled check certified candidate")
	}
}

func TestLiveCheckRequiresExplicitCompatibility(t *testing.T) {
	s, runner, _ := fixture(t)
	execute(t, s, "prepare")
	store, _ := conversationlog.NewFileStore(s.Root)
	owner, err := store.TryLock(context.Background(), "environment-lifetime")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Unlock() }()
	runner.calls = nil
	if _, err = s.Execute(context.Background(), "check", "focused", "", false); err == nil || len(runner.calls) != 0 {
		t.Fatal("undeclared check ran alongside preview", err)
	}
}

func TestEvidenceReadersDoNotMasqueradeAsRunningCommands(t *testing.T) {
	s, _, _ := fixture(t)
	execute(t, s, "check")
	results := make(chan error, 4)
	for range 4 {
		go func() { _, err := s.Evidence(context.Background()); results <- err }()
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Fatal("read-only polling blocked candidate validation", err)
		}
	}
}
