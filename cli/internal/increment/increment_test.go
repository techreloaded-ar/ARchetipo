package increment

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, raw, err)
	}
	return strings.TrimSpace(string(raw))
}
func fixture(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "tests@example.test")
	runGit(t, root, "config", "user.name", "Tests")
	if err := os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{config.RelativePath: "connector: file\n", ".archetipo/shared-runtime.md": "test runtime\n", "hello.txt": "original\n", verification.RecipePath: `{"version":1,"probe":[{"name":"git","argv":["git","--version"],"timeout_seconds":5}],"checks":{"diff":{"name":"diff","argv":["git","diff","--check","HEAD~1","HEAD"],"timeout_seconds":5,"inputs":["hello.txt"],"cache":true,"during_preview":true}}}`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.LoadExact(root)
	if err != nil {
		t.Fatal(err)
	}
	conn := filefs.New(cfg)
	if _, err = conn.SaveInitialBacklog(context.Background(), []domain.Spec{story("US-010", "Existing product story")}); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "initial")
	return New(cfg, conn)
}
func story(code, title string) domain.Spec {
	return domain.Spec{Code: code, Title: title, Epic: domain.Epic{Code: "EP-001", Title: "Product"}, Priority: domain.Priority("MEDIUM"), Points: 3, Scope: domain.Scope("MVP"), Status: domain.StatusTodo, Body: "## User story\nA business result.\n## Demonstrates\nObserve the result.\n## Acceptance\n- [ ] It works.\n"}
}

func TestSandboxCopiesRecipeWithoutEnvironmentCheckpoint(t *testing.T) {
	s := fixture(t)
	recipe := []byte(`{"version":1,"probe":[{"name":"ready","argv":["node","--version"],"timeout_seconds":5}]}`)
	if err := os.WriteFile(filepath.Join(s.Config.ProjectRoot, ".archetipo", "environment.json"), recipe, 0o600); err != nil {
		t.Fatal(err)
	}
	previewRecipe := []byte(`{"version":1,"directory":".","argv":["server","{port}"],"url":"http://127.0.0.1:{port}","prepare_environment":true}`)
	if err := os.WriteFile(filepath.Join(s.Config.ProjectRoot, ".archetipo", "preview.json"), previewRecipe, 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(s.Config.ProjectRoot, ".archetipo", "environment")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(`{"preparation_id":"parent-only"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, s.Config.ProjectRoot, "add", verification.RecipePath)
	runGit(t, s.Config.ProjectRoot, "commit", "-m", "test: recipe")
	r := create(t, s)
	got, err := os.ReadFile(filepath.Join(r.Workdir, ".archetipo", "environment.json"))
	if err != nil || string(got) != string(recipe) {
		t.Fatal("project recipe unavailable in sandbox", err)
	}
	got, err = os.ReadFile(filepath.Join(r.Workdir, ".archetipo", "preview.json"))
	if err != nil || string(got) != string(previewRecipe) {
		t.Fatal("portable preview recipe unavailable in sandbox", err)
	}
	if _, err := os.Stat(filepath.Join(r.Workdir, ".archetipo", "environment", "state.json")); !os.IsNotExist(err) {
		t.Fatal("parent resource ownership checkpoint copied")
	}
}

func TestSandboxInheritsBundlePublishedOnlyFromAnotherIncrement(t *testing.T) {
	s := fixture(t)
	helper := "scripts/archetipo/preview.sh"
	if err := os.MkdirAll(filepath.Join(s.Config.ProjectRoot, "scripts/archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Config.ProjectRoot, helper), []byte("old committed launcher\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, s.Config.ProjectRoot, "add", helper)
	runGit(t, s.Config.ProjectRoot, "commit", "-m", "test: original launcher")
	first := create(t, s)
	preview := []byte(`{"version":1,"directory":".","argv":["sh","scripts/archetipo/preview.sh","{port}"],"url":"http://127.0.0.1:{port}","prepare_environment":true}`)
	if err := os.WriteFile(filepath.Join(first.Workdir, environmentrecipe.Preview), preview, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(first.Workdir, "scripts/archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first.Workdir, helper), []byte("exec actual-server \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := environmentrecipe.Publish(first.Workdir, []string{helper}); err != nil {
		t.Fatal(err)
	}
	parentPreview := filepath.Join(s.Config.ProjectRoot, environmentrecipe.Preview)
	if _, err := os.Stat(parentPreview); !os.IsNotExist(err) {
		t.Fatal("fixture unexpectedly materialized the parent preview", err)
	}
	second := create(t, s)
	for path, expected := range map[string]string{environmentrecipe.Preview: string(preview), helper: "exec actual-server \"$1\"\n"} {
		got, err := os.ReadFile(filepath.Join(second.Workdir, path))
		if err != nil || string(got) != expected {
			t.Fatalf("published asset missing in the new worktree: %s, %v", path, err)
		}
	}
	if _, err := os.Stat(parentPreview); !os.IsNotExist(err) {
		t.Fatal("new increment changed the parent checkout", err)
	}
}

func TestSandboxDoesNotBorrowWorkspacePreviewEndpoint(t *testing.T) {
	s := fixture(t)
	if err := os.WriteFile(filepath.Join(s.Config.ProjectRoot, ".archetipo", "preview.json"), []byte(`{"version":1,"directory":".","url":"http://127.0.0.1:41987"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := create(t, s)
	if _, err := os.Stat(filepath.Join(r.Workdir, ".archetipo", "preview.json")); !os.IsNotExist(err) {
		t.Fatal("inherited another checkout's fixed endpoint", err)
	}
}

func TestSandboxKeepsPreviewRecipeFromMain(t *testing.T) {
	s := fixture(t)
	path := filepath.Join(s.Config.ProjectRoot, ".archetipo", "preview.json")
	baseline := `{"version":1,"directory":".","argv":["server","--main","{port}"],"url":"http://127.0.0.1:{port}"}`
	if err := os.WriteFile(path, []byte(baseline), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, s.Config.ProjectRoot, "add", ".archetipo/preview.json")
	runGit(t, s.Config.ProjectRoot, "commit", "-m", "test: project preview recipe")
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(baseline, "--main", "--uncommitted")), 0o600); err != nil {
		t.Fatal(err)
	}
	r := create(t, s)
	got, err := os.ReadFile(filepath.Join(r.Workdir, ".archetipo", "preview.json"))
	if err != nil || string(got) != baseline {
		t.Fatal("overwrote increment's versioned recipe with workspace changes", err)
	}
}

func create(t *testing.T, s *Service) Record {
	t.Helper()
	r, err := s.Create(context.Background(), "Customer export", "Prepare the sales review", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = git(context.Background(), s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir)
	})
	return r
}
func specify(t *testing.T, s *Service, r Record, _ ...domain.Spec) Record {
	t.Helper()
	r, err := s.Propose(context.Background(), r.ID, Proposal{Intent: "Export the current customers."})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func ready(t *testing.T, s *Service, r Record) Record {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte("increment "+r.ID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, r.Workdir, "add", "hello.txt")
	runGit(t, r.Workdir, "commit", "-m", "feat: export")
	verifyProduct(t, r)
	r, err := s.Ready(context.Background(), r.ID, Result{Summary: "Try the customer export.", Checks: []string{"git diff --check HEAD~1 HEAD — passed"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSandboxKeepsCodeAndProcessAwayFromParent(t *testing.T) {
	s := fixture(t)
	parentHEAD := runGit(t, s.Config.ProjectRoot, "rev-parse", "HEAD")
	configBefore, err := os.ReadFile(filepath.Join(s.Config.ProjectRoot, config.RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	r := create(t, s)
	cfg, err := config.Load(filepath.Join(r.Workdir, ".archetipo"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectRoot != r.Workdir || cfg.Increment == nil || cfg.Increment.ID != r.ID || cfg.Worktree.Enabled || cfg.Connector != "file" {
		t.Fatalf("sandbox config: %+v", cfg)
	}
	local := filefs.New(cfg)
	items, err := local.FetchBacklogItems(context.Background(), "")
	if err != nil || len(items) != 0 {
		t.Fatalf("local backlog = %v, %v", items, err)
	}
	r = specify(t, s, r, story("US-001", "Export Excel"))
	items, err = s.Connector.FetchBacklogItems(context.Background(), "")
	if err != nil || len(items) != 1 || items[0].Code != "US-010" {
		t.Fatalf("parent backlog touched: %v %v", items, err)
	}
	r = ready(t, s, r)
	if got := runGit(t, s.Config.ProjectRoot, "rev-parse", "HEAD"); got != parentHEAD {
		t.Fatalf("parent moved: %s", got)
	}
	original, _ := os.ReadFile(filepath.Join(s.Config.ProjectRoot, "hello.txt"))
	if string(original) != "original\n" {
		t.Fatal("parent code changed")
	}
	unchanged, _ := os.ReadFile(filepath.Join(r.Workdir, config.RelativePath))
	if string(unchanged) != string(configBefore) {
		t.Fatal("sandbox rewrote tracked configuration")
	}
	diff := runGit(t, r.Workdir, "diff", "--name-only", r.BaseSHA+"...HEAD")
	if diff != "hello.txt" {
		t.Fatalf("runtime escaped into code diff: %s", diff)
	}
	restarted := New(s.Config, s.Connector)
	saved, err := restarted.Read(r.ID)
	if err != nil || saved.ReadySHA != r.ReadySHA || saved.Status != Review {
		t.Fatalf("restart lost state: %+v %v", saved, err)
	}
}

func TestReadyAndPublishRejectUnfinishedOrStaleEvidence(t *testing.T) {
	s := fixture(t)
	r := specify(t, s, create(t, s), story("US-001", "Export"))
	if _, err := s.Ready(context.Background(), r.ID, Result{Summary: "Result", Checks: []string{"check"}}); err == nil {
		t.Fatal("unfinished story was reviewable")
	}
	r = ready(t, s, r)
	if err := os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte("unverified"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(context.Background(), r.ID, r.ReadySHA, 1); err == nil {
		t.Fatal("dirty result published")
	}
	runGit(t, r.Workdir, "add", "hello.txt")
	runGit(t, r.Workdir, "commit", "-m", "changed after review")
	if _, err := s.Publish(context.Background(), r.ID, r.ReadySHA, 1); err == nil {
		t.Fatal("new head reused old acceptance")
	}
}

type fakeHosting struct {
	pr         *PullRequest
	creates    int
	findBase   string
	createBase string
}

func (f *fakeHosting) Find(_ context.Context, _, _, base string) (*PullRequest, error) {
	f.findBase = base
	return f.pr, nil
}
func (f *fakeHosting) Create(_ context.Context, dir, _, base, _, _ string) (*PullRequest, error) {
	f.createBase = base
	sha, err := git(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	f.creates++
	f.pr = &PullRequest{URL: "https://github.com/example/product/pull/1", Number: 1, State: "OPEN", HeadSHA: sha}
	return f.pr, nil
}
func (f *fakeHosting) Read(context.Context, string, int) (*PullRequest, error) { return f.pr, nil }

func remote(t *testing.T, s *Service) {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, bare, "init", "--bare")
	runGit(t, s.Config.ProjectRoot, "remote", "add", "origin", bare)
	runGit(t, s.Config.ProjectRoot, "push", "-u", "origin", "main")
}

func TestFeedbackKeepsSameSandboxAndPreservesPriorResult(t *testing.T) {
	s := fixture(t)
	r := ready(t, s, specify(t, s, create(t, s), story("US-001", "Export")))
	prior := r
	var err error
	r, err = s.FeedbackFor(context.Background(), r.ID, "Use Excel instead of CSV.")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != prior.ID || r.Workdir != prior.Workdir || r.Branch != prior.Branch || r.ReadySHA != "" || r.Status != Developing || r.Revisions[0].Result == nil || r.Revisions[0].HeadSHA != prior.ReadySHA {
		t.Fatalf("feedback lost identity/evidence: %+v", r)
	}
}

func TestUnusedSandboxRollbackNeverDiscardsProductWork(t *testing.T) {
	s := fixture(t)
	unused := create(t, s)
	if err := s.RemoveUnused(context.Background(), unused.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unused.Workdir); !os.IsNotExist(err) {
		t.Fatal("unused worktree survived rollback")
	}
	changed := create(t, s)
	if err := os.WriteFile(filepath.Join(changed.Workdir, "hello.txt"), []byte("keep my work"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveUnused(context.Background(), changed.ID); err == nil {
		t.Fatal("discarded a modified sandbox")
	}
	if _, err := os.Stat(changed.Workdir); err != nil {
		t.Fatal("product work was removed")
	}
}
func TestSandboxRetargetsAbsoluteArtifactPathsAndRejectsExternalOnes(t *testing.T) {
	s := fixture(t)
	path := filepath.Join(s.Config.ProjectRoot, config.RelativePath)
	body := "connector: file\npaths:\n  wiki: " + filepath.ToSlash(filepath.Join(s.Config.ProjectRoot, "docs/wiki")) + "\nworktree:\n  enabled: true\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := create(t, s)
	cfg, err := config.Load(r.Workdir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AbsPath(cfg.Paths.Wiki) != filepath.Join(r.Workdir, "docs/wiki") || cfg.Worktree.Enabled {
		t.Fatalf("unsafe target: %+v", cfg)
	}
	body = "connector: file\npaths:\n  wiki: " + filepath.ToSlash(filepath.Join(t.TempDir(), "external-wiki")) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = config.Load(r.Workdir); err == nil {
		t.Fatal("external Wiki path escaped sandbox")
	}
}

func TestMissingSandboxMarkerNeverFallsBackToTheParentConnector(t *testing.T) {
	s := fixture(t)
	r := create(t, s)
	if err := os.Remove(filepath.Join(r.Workdir, config.IncrementWorkspacePath)); err != nil {
		t.Fatal(err)
	}
	for _, load := range []func(string) (config.Config, error){config.Load, config.LoadExact} {
		if _, err := load(r.Workdir); err == nil {
			t.Fatal("missing isolation metadata resolved a writable connector")
		}
	}
	original, err := s.Connector.FetchBacklogItems(context.Background(), "")
	if err != nil || len(original) != 1 {
		t.Fatal("parent backlog was affected")
	}
}

func TestIncrementFromAnotherBranchStartsAndPublishesAgainstMain(t *testing.T) {
	s := fixture(t)
	remote(t, s)
	root := s.Config.ProjectRoot
	mainSHA := runGit(t, root, "rev-parse", "main")
	runGit(t, root, "checkout", "-b", "feature/unrelated")
	if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("other work"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "unrelated.txt")
	runGit(t, root, "commit", "-m", "unrelated work")
	parentSHA := runGit(t, root, "rev-parse", "HEAD")
	r := create(t, s)
	if r.BaseBranch != "main" || r.BaseSHA != mainSHA || runGit(t, r.Workdir, "rev-parse", "HEAD") != mainSHA {
		t.Fatalf("increment did not fork main: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(r.Workdir, "unrelated.txt")); !os.IsNotExist(err) {
		t.Fatalf("unrelated branch content leaked into increment: %v", err)
	}
	r = ready(t, s, specify(t, s, r, story("US-001", "Export")))
	hosting := &fakeHosting{}
	s.Hosting = hosting
	if _, err := s.Publish(context.Background(), r.ID, r.ReadySHA, 1); err != nil {
		t.Fatal(err)
	}
	if hosting.findBase != "main" || hosting.createBase != "main" {
		t.Fatalf("PR targets wrong branch: find=%q create=%q", hosting.findBase, hosting.createBase)
	}
	if runGit(t, root, "branch", "--show-current") != "feature/unrelated" || runGit(t, root, "rev-parse", "HEAD") != parentSHA {
		t.Fatal("parent checkout changed")
	}
}

func TestInvalidDevelopmentModeDoesNotCreateWorktree(t *testing.T) {
	s := fixture(t)
	if _, err := s.CreateWithMode(context.Background(), "App", "Build", "", "unknown"); err == nil {
		t.Fatal("unknown mode accepted")
	}
	records, err := s.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("unexpected side effects: %v %v", records, err)
	}
}

func TestContinuousUsesCurrentBranchWhenMainAbsent(t *testing.T) {
	s := fixture(t)
	runGit(t, s.Config.ProjectRoot, "branch", "-m", "product")
	r, err := s.CreateWithMode(context.Background(), "App", "Change app", "", Continuous)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = git(context.Background(), s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir)
	})
	if r.BaseBranch != "product" {
		t.Fatalf("wrong base: %s", r.BaseBranch)
	}

}

func TestContinuousBootstrapsEmptyWorkspaceWithoutCommittingUntrackedFiles(t *testing.T) {
	s := fixture(t)
	if err := os.RemoveAll(filepath.Join(s.Config.ProjectRoot, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.Config.ProjectRoot, "hello.txt")); err != nil {
		t.Fatal(err)
	}
	runGit(t, s.Config.ProjectRoot, "init", "-b", "main")
	runGit(t, s.Config.ProjectRoot, "add", ".archetipo/config.yaml")
	r, err := s.CreateWithMode(context.Background(), "New app", "Build app", "", Continuous)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = git(context.Background(), s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir)
	})
	if files := runGit(t, s.Config.ProjectRoot, "ls-tree", "--name-only", "HEAD"); files != "" {
		t.Fatalf("unexpected committed files: %s", files)
	}
	if r.BaseBranch != "main" {
		t.Fatal(r.BaseBranch)
	}
}

func TestContinuousRefusesUnversionedExistingCode(t *testing.T) {
	s := fixture(t)
	if err := os.RemoveAll(filepath.Join(s.Config.ProjectRoot, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWithMode(context.Background(), "Existing", "Change app", "", Continuous); err == nil {
		t.Fatal("unversioned source silently omitted")
	}
	if _, err := os.Stat(filepath.Join(s.Config.ProjectRoot, ".git")); !os.IsNotExist(err) {
		t.Fatalf("created Git after refusal: %v", err)
	}
}

// This runner executes real fixture checks without spawning the CLI supervisor
// from a Go test binary (the supervisor itself has dedicated integration tests).
type productRunner struct{}

func (productRunner) Run(ctx context.Context, root string, env []string, c verification.Command, out io.Writer) error {
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
func verifyProduct(t *testing.T, r Record) {
	t.Helper()
	service := verification.New(r.Workdir, "")
	service.Runner = productRunner{}
	result, err := service.Execute(context.Background(), "check", "diff", "", false)
	if err != nil || !result.Passed || !result.Stable {
		t.Fatalf("fixture verification: %+v %v", result, err)
	}
}
