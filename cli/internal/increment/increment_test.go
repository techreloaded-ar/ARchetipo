package increment

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
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
	for name, body := range map[string]string{config.RelativePath: "connector: file\n", ".archetipo/shared-runtime.md": "test runtime\n", "hello.txt": "original\n"} {
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
func proposeApprove(t *testing.T, s *Service, r Record, stories ...domain.Spec) Record {
	t.Helper()
	r, err := s.Propose(context.Background(), r.ID, Proposal{Intent: "Export the current customers.", Stories: stories})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Approve(context.Background(), r.ID, len(r.Revisions)); err != nil {
		t.Fatal(err)
	}
	// The viewer and the unconfined CLI mirror the approval into the original backlog.
	r, err = s.SyncStories(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func complete(t *testing.T, s *Service, r Record) {
	t.Helper()
	local, err := s.local(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range r.Revisions[len(r.Revisions)-1].Stories {
		tasks, taskErr := local.ReadSpecTasks(context.Background(), st.Code)
		if taskErr != nil {
			tasks = nil
		}
		if len(tasks) == 0 {
			_, err = local.SavePlan(context.Background(), st.Code, domain.PlanInput{PlanBody: "Implement and verify.", Tasks: []domain.Task{{ID: "TASK-01", Title: "Implement", Type: domain.TaskType("Impl"), Status: domain.StatusTodo, Body: "Implement and verify"}}})
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err = local.CompleteTask(context.Background(), st.Code, "TASK-01"); err != nil {
			t.Fatal(err)
		}
		if _, err = local.TransitionStatus(context.Background(), st.Code, domain.StatusDone); err != nil {
			t.Fatal(err)
		}
	}
}
func ready(t *testing.T, s *Service, r Record) Record {
	t.Helper()
	complete(t, s, r)
	if err := os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte("increment "+r.ID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, r.Workdir, "add", "hello.txt")
	runGit(t, r.Workdir, "commit", "-m", "feat: export")
	r, err := s.Ready(context.Background(), r.ID, Result{Summary: "Try the customer export.", Checks: []string{"go test ./... — passed"}})
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
	r = proposeApprove(t, s, r, story("US-001", "Export Excel"))
	items, err = s.Connector.FetchBacklogItems(context.Background(), "")
	if err != nil || len(items) != 2 || items[1].Code != "US-011" || items[1].Status != domain.StatusInProgress || !strings.Contains(items[1].Body, sourceMarker(r.ID, "US-001")) {
		t.Fatalf("approved story not shown in progress in the original backlog: %v %v", items, err)
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

func TestRevisionsPreserveUnchangedWorkAndInvalidateChangedStories(t *testing.T) {
	s := fixture(t)
	r := create(t, s)
	first, keep, retire := story("US-001", "CSV"), story("US-002", "Unchanged"), story("US-003", "Retire")
	r = proposeApprove(t, s, r, first, keep, retire)
	complete(t, s, r)
	first.Title = "Excel"
	r, err := s.Propose(context.Background(), r.ID, Proposal{Intent: "Excel replaces CSV.", Stories: []domain.Spec{first, keep}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Approve(context.Background(), r.ID, 1); err == nil {
		t.Fatal("accepted a superseded intent")
	}
	r, err = s.Approve(context.Background(), r.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	local, _ := s.local(r)
	items, err := local.FetchBacklogItems(context.Background(), "")
	if err != nil || len(items) != 2 {
		t.Fatalf("stories: %v %v", items, err)
	}
	for _, st := range items {
		if st.Code == "US-001" && st.Status != domain.StatusTodo {
			t.Fatal("changed story kept old acceptance")
		}
		if st.Code == "US-002" && st.Status != domain.StatusDone {
			t.Fatal("unchanged story lost its work")
		}
	}
	tasks, _ := local.ReadSpecTasks(context.Background(), "US-001")
	if len(tasks) != 0 {
		t.Fatal("stale plan survived revision")
	}
	if len(r.Revisions) != 2 || r.Revisions[0].Stories[0].Title != "CSV" {
		t.Fatal("revision history rewritten")
	}
	if _, err = s.Approve(context.Background(), r.ID, 2); err != nil {
		t.Fatalf("approval retry: %v", err)
	}
	items, _ = local.FetchBacklogItems(context.Background(), "")
	if len(items) != 2 {
		t.Fatal("approval duplicated stories")
	}
}

func TestReadyAndPublishRejectUnfinishedOrStaleEvidence(t *testing.T) {
	s := fixture(t)
	r := proposeApprove(t, s, create(t, s), story("US-001", "Export"))
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

func TestReadyWithoutPlanRetainsCompletionAndSnapshotGates(t *testing.T) {
	for _, scenario := range []string{"ready", "unfinished story", "unfinished task", "malformed plan", "changed intent", "dirty tree", "no product commit", "missing evidence"} {
		t.Run(scenario, func(t *testing.T) {
			s := fixture(t)
			r := proposeApprove(t, s, create(t, s), story("US-001", "Export"))
			local, err := s.local(r)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "unfinished task" {
				_, err = local.SavePlan(context.Background(), "US-001", domain.PlanInput{PlanBody: "Verify export", Tasks: []domain.Task{{ID: "TASK-01", Title: "Verify", Type: "Test", Status: domain.StatusTodo, Body: "Verify export"}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "changed intent" {
				changed := story("US-001", "A different export")
				if _, err = local.UpdateSpec(context.Background(), changed.Code, domain.SpecUpdate{Title: &changed.Title}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "malformed plan" {
				cfg, loadErr := config.LoadExact(r.Workdir)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				planDir := cfg.AbsPath(cfg.File.Planning)
				if err = os.MkdirAll(planDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(planDir, "US-001-plan.yaml"), []byte("tasks: ["), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "unfinished story" {
				if _, err = local.TransitionStatus(context.Background(), "US-001", domain.StatusDone); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "no product commit" {
				if err = os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte("verified export\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				runGit(t, r.Workdir, "add", "hello.txt")
				runGit(t, r.Workdir, "commit", "-m", "feat: export without a plan")
			}
			if scenario == "dirty tree" {
				if err = os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte("unverified export\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			result := Result{Summary: "Try the verified export", Checks: []string{"export check — passed"}}
			if scenario == "missing evidence" {
				result.Checks = nil
			}
			got, err := s.Ready(context.Background(), r.ID, result)
			if scenario != "ready" {
				if err == nil {
					t.Fatal("invalid delivery became reviewable")
				}
				persisted, readErr := s.Read(r.ID)
				if readErr != nil || persisted.Status != Developing || persisted.ReadySHA != "" {
					t.Fatal("rejected delivery changed readiness", persisted, readErr)
				}
				return
			}
			if err != nil || got.Status != Review || got.ReadySHA != runGit(t, r.Workdir, "rev-parse", "HEAD") || got.ReadyStories == "" {
				t.Fatal("planless delivery not frozen", got, err)
			}
			tasks, taskErr := local.ReadSpecTasks(context.Background(), "US-001")
			if !errors.Is(taskErr, os.ErrNotExist) || len(tasks) != 0 {
				t.Fatal("planless delivery acquired tasks", tasks, taskErr)
			}
		})
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

// Remote connectors normalize leading/trailing Markdown whitespace.
type trimmedConnector struct{ connector.Connector }

func (c trimmedConnector) FetchBacklogItems(ctx context.Context, status domain.Status) ([]domain.Spec, error) {
	stories, err := c.Connector.FetchBacklogItems(ctx, status)
	for i := range stories {
		stories[i].Body = strings.TrimSpace(stories[i].Body)
	}
	return stories, err
}
func (c trimmedConnector) ReadSpecDetail(ctx context.Context, ref string) (domain.Spec, error) {
	story, err := c.Connector.ReadSpecDetail(ctx, ref)
	story.Body = strings.TrimSpace(story.Body)
	return story, err
}

type failAfterUpdate struct {
	connector.Connector
	failed bool
}

func (f *failAfterUpdate) UpdateSpec(ctx context.Context, code string, patch domain.SpecUpdate) (domain.WriteResult, error) {
	result, err := f.Connector.UpdateSpec(ctx, code, patch)
	if err == nil && !f.failed {
		f.failed = true
		return result, errors.New("connection lost after write")
	}
	return result, err
}
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
func TestPublicationRecoversPartialWritesAndDeliversOnlyAfterConfirmedMerge(t *testing.T) {
	s := fixture(t)
	remote(t, s)
	r := proposeApprove(t, s, create(t, s), story("US-001", "Excel"), story("US-002", "Area filter"))
	r = ready(t, s, r)
	hosting := &fakeHosting{}
	s.Hosting = hosting
	s.Connector = &failAfterUpdate{Connector: trimmedConnector{Connector: s.Connector}}
	if _, err := s.Publish(context.Background(), r.ID, r.ReadySHA, 1); err == nil {
		t.Fatal("injected partial publication did not fail")
	}
	partial, err := s.Read(r.ID)
	if err != nil || partial.Status != Publishing || partial.PR == nil || len(partial.Published) != 2 {
		t.Fatalf("lost recovery state: %+v %v", partial, err)
	}
	r, err = s.Publish(context.Background(), r.ID, r.ReadySHA, 1)
	if err != nil {
		t.Fatal(err)
	}
	if hosting.creates != 1 || r.Status != Proposed {
		t.Fatalf("duplicate PR or wrong state: %+v creates=%d", r, hosting.creates)
	}
	if _, err = s.Publish(context.Background(), r.ID, r.ReadySHA, 1); err != nil {
		t.Fatal(err)
	}
	specs, _ := s.Connector.FetchBacklogItems(context.Background(), "")
	if len(specs) != 3 {
		t.Fatalf("duplicated stories: %d", len(specs))
	}
	for _, st := range specs {
		if st.Code != "US-010" && (st.Status != domain.StatusReview || !strings.Contains(st.Body, hosting.pr.URL)) {
			t.Fatalf("story with an open PR is not in review: %+v", st)
		}
	}
	if _, err = s.Refresh(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Read(r.ID)
	if current.Status != Proposed {
		t.Fatal("open PR delivered")
	}
	hosting.pr.State = "CLOSED"
	current, err = s.Refresh(context.Background(), r.ID)
	if err != nil || current.Status != Proposed {
		t.Fatal("closed, unmerged PR delivered")
	}
	hosting.pr.State = "MERGED"
	hosting.pr.HeadSHA = "another-unapproved-head"
	if _, err = s.Refresh(context.Background(), r.ID); err == nil {
		t.Fatal("unapproved PR head delivered")
	}
	hosting.pr.HeadSHA = r.ApprovedSHA
	code := r.Published["US-001"]
	original, readErr := s.Connector.ReadSpecDetail(context.Background(), code)
	if readErr != nil {
		t.Fatal(readErr)
	}
	changedBody := original.Body + "A new unapproved requirement."
	if _, err = s.Connector.UpdateSpec(context.Background(), code, domain.SpecUpdate{Body: &changedBody}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Refresh(context.Background(), r.ID); err == nil {
		t.Fatal("changed official story delivered")
	}
	if _, err = s.Connector.UpdateSpec(context.Background(), code, domain.SpecUpdate{Body: &original.Body}); err != nil {
		t.Fatal(err)
	}
	current, err = s.Refresh(context.Background(), r.ID)
	if err != nil || current.Status != Delivered {
		t.Fatalf("merge not reconciled: %+v %v", current, err)
	}
	specs, _ = s.Connector.FetchBacklogItems(context.Background(), "")
	for _, st := range specs {
		if st.Code != "US-010" && st.Status != domain.StatusDone {
			t.Fatal("merged story not delivered")
		}
	}
	if got := runGit(t, s.Config.ProjectRoot, "show", "HEAD:hello.txt"); got != "original" {
		t.Fatal("publication merged the code itself")
	}
}

func TestFeedbackKeepsSameSandboxAndPreservesPriorResult(t *testing.T) {
	s := fixture(t)
	r := ready(t, s, proposeApprove(t, s, create(t, s), story("US-001", "Export")))
	prior := r
	var err error
	r, err = s.FeedbackFor(context.Background(), r.ID, "Use Excel instead of CSV.")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != prior.ID || r.Workdir != prior.Workdir || r.Branch != prior.Branch || r.ReadySHA != "" || r.Status != Discovery || r.Revisions[0].Result == nil || r.Revisions[0].HeadSHA != prior.ReadySHA {
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

func TestTechnicalReworkNotesDoNotChangeTheApprovedProductIntent(t *testing.T) {
	s := fixture(t)
	r := proposeApprove(t, s, create(t, s), story("US-001", "Export"))
	local, err := s.local(r)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := local.ReadSpecDetail(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	body := domain.AppendReworkFeedback(spec.Body, []domain.ReviewComment{{File: "hello.txt", Line: 1, Body: "Fix the implementation to satisfy the existing criterion."}})
	if _, err = local.UpdateSpec(context.Background(), spec.Code, domain.SpecUpdate{Body: &body}); err != nil {
		t.Fatal(err)
	}
	r = ready(t, s, r)
	if r.Status != Review || len(r.Revisions) != 1 {
		t.Fatal("technical fix required another business-intent revision")
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
	r = ready(t, s, proposeApprove(t, s, r, story("US-001", "Export")))
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

func TestIncrementWithoutMainDoesNotCreateWorktree(t *testing.T) {
	s := fixture(t)
	root := s.Config.ProjectRoot
	runGit(t, root, "branch", "-m", "other")
	before := runGit(t, root, "worktree", "list", "--porcelain")
	if _, err := s.Create(context.Background(), "Export", "Prepare export", ""); err == nil {
		t.Fatal("increment silently used a branch other than main")
	}
	if after := runGit(t, root, "worktree", "list", "--porcelain"); after != before {
		t.Fatal("failed creation changed worktrees")
	}
	if _, err := os.Stat(filepath.Join(root, ".archetipo", "increments")); !os.IsNotExist(err) {
		t.Fatalf("failed creation persisted increment state: %v", err)
	}
}

func TestApprovedRevisionsKeepTheOriginalBacklogInStep(t *testing.T) {
	s := fixture(t)
	first, keep, retire := story("US-001", "CSV"), story("US-002", "Unchanged"), story("US-003", "Retire")
	r := proposeApprove(t, s, create(t, s), first, keep, retire)
	official := func() map[string]domain.Spec {
		items, err := s.Connector.FetchBacklogItems(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		byCode := map[string]domain.Spec{}
		for _, st := range items {
			byCode[st.Code] = st
		}
		return byCode
	}
	codes := map[string]string{"US-001": r.Published["US-001"], "US-002": r.Published["US-002"], "US-003": r.Published["US-003"]}
	if codes["US-001"] != "US-011" || codes["US-003"] != "US-013" || len(official()) != 4 {
		t.Fatalf("codes not allocated at approval: %v", codes)
	}
	first.Title = "Excel"
	added := story("US-004", "Area filter")
	r, err := s.Propose(context.Background(), r.ID, Proposal{Intent: "Excel replaces CSV.", Stories: []domain.Spec{first, keep, added}})
	if err != nil {
		t.Fatal(err)
	}
	if official()[codes["US-001"]].Title != "CSV" {
		t.Fatal("an unapproved proposal changed the original backlog")
	}
	if _, err = s.Approve(context.Background(), r.ID, 2); err != nil {
		t.Fatal(err)
	}
	if r, err = s.SyncStories(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	byCode := official()
	if byCode[codes["US-001"]].Title != "Excel" || byCode[codes["US-002"]].Status != domain.StatusInProgress {
		t.Fatalf("approved revision not mirrored: %+v", byCode)
	}
	if _, found := byCode[codes["US-003"]]; found {
		t.Fatal("retired story left in the original backlog")
	}
	if r.Published["US-004"] != "US-014" || byCode["US-014"].Status != domain.StatusInProgress {
		t.Fatalf("added story not published with a fresh code: %v", r.Published)
	}
	if _, err = s.SyncStories(context.Background(), r.ID); err != nil || len(official()) != 4 {
		t.Fatal("approval retry duplicated the official stories", err)
	}
}

// Some connectors (github) list stories without their body: the increment must
// recognise its own stories from the detail, or every retry would conflict.
type bodylessListConnector struct{ connector.Connector }

func (c bodylessListConnector) FetchBacklogItems(ctx context.Context, status domain.Status) ([]domain.Spec, error) {
	stories, err := c.Connector.FetchBacklogItems(ctx, status)
	for i := range stories {
		stories[i].Body = ""
	}
	return stories, err
}

func TestMirroringRecognisesItsStoriesWhenListingsCarryNoBody(t *testing.T) {
	s := fixture(t)
	s.Connector = bodylessListConnector{Connector: s.Connector}
	remote(t, s)
	r := proposeApprove(t, s, create(t, s), story("US-001", "Export"), story("US-002", "Retire"))
	if _, err := s.SyncStories(context.Background(), r.ID); err != nil {
		t.Fatalf("second sync conflicted with its own stories: %v", err)
	}
	r = ready(t, s, r)
	s.Hosting = &fakeHosting{}
	if r, err := s.Publish(context.Background(), r.ID, r.ReadySHA, 1); err != nil || r.Status != Proposed {
		t.Fatalf("publication conflicted with its own stories: %+v %v", r, err)
	}
	plan, err := s.DeletionPlan(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Delete(context.Background(), r.ID, DeletionOptions{HeadSHA: plan.HeadSHA, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	// The wrapper hides SpecDeleter too, like github: retired stories go back
	// to TODO instead of being removed.
	items, _ := s.Connector.FetchBacklogItems(context.Background(), "")
	if len(items) != 3 {
		t.Fatalf("stories of a deleted increment: %v", items)
	}
	for _, st := range items {
		if st.Status != domain.StatusTodo {
			t.Fatalf("an unmerged PR kept the story %s of a deleted increment %s", st.Code, st.Status)
		}
	}
}
