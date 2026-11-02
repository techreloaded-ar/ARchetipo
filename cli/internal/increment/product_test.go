package increment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
)

func productWork(t *testing.T) (*Service, Record) {
	t.Helper()
	s := fixture(t)
	r, err := s.CreateWithMode(context.Background(), "Improve bookings", "Understand and improve bookings", "", Continuous)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = git(context.Background(), s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir)
	})
	return s, r
}

func TestProductWorkWithoutDeliveryArtifacts(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	r, err := s.Propose(ctx, r.ID, Proposal{Intent: "Problem: users cannot cancel. Behavior: allow cancellation before departure."})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != Discovery || r.Revisions[0].Approved {
		t.Fatalf("proposal started development without approval: %+v", r)
	}
	if _, err = s.ApproveIntent(ctx, r.ID, 0); err == nil {
		t.Fatal("stale revision approved")
	}
	cfg, err := config.LoadExact(r.Workdir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := filefs.New(cfg).FetchBacklogItems(ctx, "")
	if err != nil || len(items) != 0 {
		t.Fatalf("synthetic backlog: %+v %v", items, err)
	}
	if _, err = os.Stat(filepath.Join(r.Workdir, ".agents/skills/archetipo-plan")); err == nil {
		t.Fatal("unneeded planning skill installed")
	}
	r = ready(t, s, r)
	candidate := r.ReadySHA
	r, err = s.Learn(ctx, r.ID, "Cancellation is difficult to find", "Make it visible", false)
	if err != nil || r.ReadySHA != candidate {
		t.Fatalf("observation invalidated technical proof: %+v %v", r, err)
	}
	remote(t, s)
	hosting := &fakeHosting{}
	s.Hosting = hosting
	r, err = s.Publish(ctx, r.ID, candidate, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Published) != 0 {
		t.Fatal("published synthetic stories")
	}
	hosting.pr.HeadSHA = "developer-revision"
	hosting.pr.State = "MERGED"
	r, err = s.Refresh(ctx, r.ID)
	if err != nil || r.Status != Delivered || r.ApprovedSHA != candidate || r.IntegrationSHA != "developer-revision" {
		t.Fatalf("lost developer handoff: %+v %v", r, err)
	}
}

func TestProductSpecificationNeedsApprovalOfCurrentRevision(t *testing.T) {
	s, work := productWork(t)
	ctx := context.Background()
	work, err := s.Propose(ctx, work.ID, Proposal{Intent: "First requirements"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Ready(ctx, work.ID, Result{Summary: "Result", Checks: []string{"check"}, SquashCommit: &SquashCommit{Title: "feat: result"}}); err == nil {
		t.Fatal("unapproved requirements passed the development gate")
	}
	if _, err = s.ApproveIntent(ctx, work.ID, 2); err == nil {
		t.Fatal("a revision that was not displayed was approved")
	}
	work, err = s.Propose(ctx, work.ID, Proposal{Intent: "Revised requirements"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApproveIntent(ctx, work.ID, 1); err == nil {
		t.Fatal("stale approval authorized new requirements")
	}
	work, err = s.ApproveIntent(ctx, work.ID, 2)
	if err != nil || work.Status != Developing || !work.Revisions[1].Approved || work.Revisions[1].ApprovalSource != "person" {
		t.Fatalf("current approval not recorded: %+v %v", work, err)
	}
}

func TestProductInvestigationCanConcludeWithoutCode(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	if _, err := s.Learn(ctx, r.ID, "Observation", "", true); err == nil {
		t.Fatal("conclusion without a decision")
	}
	r, err := s.Learn(ctx, r.ID, "The existing filter already answers the need", "Improve team guidance instead of adding a feature", true)
	if err != nil || r.Status != Concluded || r.Conclusion == "" {
		t.Fatalf("missing conclusion: %+v %v", r, err)
	}
	if _, err = s.Publish(ctx, r.ID, "", 0); err == nil {
		t.Fatal("published an investigation")
	}
	if _, err = s.Propose(ctx, r.ID, Proposal{Intent: "New scope"}); err == nil {
		t.Fatal("silently reopened a concluded work")
	}
}

func TestProductRevisionRevokesCandidateWithoutStoryTransitions(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	var err error
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "Original behavior"})
	if err != nil {
		t.Fatal(err)
	}
	r = ready(t, s, r)
	r, err = s.RequestChanges(ctx, r.ID, FeedbackRequest{Feedback: "Change cancellation deadline"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "Revised deadline"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadySHA != "" || r.Status != Discovery || len(r.Revisions) != 2 {
		t.Fatalf("stale version: %+v", r)
	}
}

func TestEnterpriseLinksPreserveOriginalStoriesAndRevisionSnapshots(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	if _, err := s.Link(ctx, r.ID, []string{"US-010", "US-010"}); err == nil {
		t.Fatal("duplicate link accepted")
	}
	r, err := s.Link(ctx, r.ID, []string{"US-010"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "First specification"})
	if err != nil {
		t.Fatal(err)
	}
	first := r.LinkedStories[0]
	r = ready(t, s, r)
	candidate := r.ReadySHA
	if _, err = s.Link(ctx, r.ID, nil); err == nil {
		t.Fatal("approved scope links silently changed")
	}
	// Enterprise backlog content/status is independent from the tested snapshot.
	changed := "Updated enterprise title"
	if _, err = s.Connector.UpdateSpec(ctx, "US-010", domain.SpecUpdate{Title: &changed}); err != nil {
		t.Fatal(err)
	}
	remote(t, s)
	s.Hosting = &fakeHosting{}
	r, err = s.Publish(ctx, r.ID, candidate, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.LinkedStories[0] != first || r.ApprovedAt == nil {
		t.Fatal("lost scope or approval time")
	}
	original, err := s.Connector.ReadSpecDetail(ctx, "US-010")
	if err != nil || original.Status != domain.StatusTodo || original.Title != changed {
		t.Fatal("publication changed enterprise story", err)
	}
	dossier := Dossier(r)
	for _, part := range []string{first.Code, first.Body, "First specification", candidate, "Business approval", "Developer handover"} {
		if !strings.Contains(dossier, part) {
			t.Fatalf("dossier lacks %q", part)
		}
	}
}

func TestSpecificationAndEvidenceHistoryRemainsPortable(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	r, err := s.Propose(ctx, r.ID, Proposal{Intent: "First scope"})
	if err != nil {
		t.Fatal(err)
	}
	r = ready(t, s, r)
	previous := r.ReadySHA
	r, err = s.FeedbackFor(ctx, r.ID, "Change the scope")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Ready(ctx, r.ID, Result{SquashCommit: &SquashCommit{Title: "feat: export customers"}, Summary: "Old scope", Checks: []string{"old proof"}}); err == nil {
		t.Fatal("feedback reused stale specification")
	}
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "Second scope"})
	if err != nil {
		t.Fatal(err)
	}
	dossier := Dossier(r)
	for _, part := range []string{"First scope", "Second scope", previous, "Change the scope", "git diff --check HEAD~1 HEAD"} {
		if !strings.Contains(dossier, part) {
			t.Fatalf("history lost %q", part)
		}
	}
}

func TestEnvironmentRemovalKeepsBusinessDossierAndStoryTrace(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	r, err := s.Link(ctx, r.ID, []string{"US-010"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "Investigation of cancellations"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Learn(ctx, r.ID, "The existing feature covers the need", "Improve documentation", true)
	if err != nil {
		t.Fatal(err)
	}
	expected := Dossier(r)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Delete(ctx, r.ID, DeletionOptions{HeadSHA: plan.HeadSHA, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	archived, err := s.ReadDossier(r.ID)
	if err != nil || Dossier(archived) != expected {
		t.Fatal("product history disappeared with the environment", err)
	}
	history, err := s.History()
	if err != nil || len(history) != 1 || history[0].LinkedStories[0].Code != "US-010" {
		t.Fatal("story trace lost", err)
	}
}

type uncertainPublication struct {
	fakeHosting
	failed bool
}

func (h *uncertainPublication) Create(ctx context.Context, dir, branch, base, title, body string) (*PullRequest, error) {
	pr, err := h.fakeHosting.Create(ctx, dir, branch, base, title, body)
	if err == nil && !h.failed {
		h.failed = true
		return nil, errors.New("connection ended after remote creation")
	}
	return pr, err
}
func TestPublicationRecoversUnknownOutcomeWithoutAnotherPR(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	r, err := s.Propose(ctx, r.ID, Proposal{Intent: "Verified export"})
	if err != nil {
		t.Fatal(err)
	}
	r = ready(t, s, r)
	remote(t, s)
	hosting := &uncertainPublication{}
	s.Hosting = hosting
	if _, err = s.Publish(ctx, r.ID, r.ReadySHA, 1); err == nil {
		t.Fatal("unknown outcome was reported as success")
	}
	pending, err := s.Read(r.ID)
	if err != nil || pending.Status != Publishing || pending.ApprovedAt == nil {
		t.Fatal("lost approved candidate during publication", err)
	}
	published, err := s.Publish(ctx, r.ID, r.ReadySHA, 1)
	if err != nil || published.Status != Proposed || hosting.creates != 1 || !published.ApprovedAt.Equal(*pending.ApprovedAt) {
		t.Fatal("retry duplicated PR or changed human approval", err)
	}
}

func TestCandidateRequiresMachineEvidenceAndRetainsApprovalReports(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	r := specify(t, s, create(t, s))
	if err := os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, r.Workdir, "add", "hello.txt")
	runGit(t, r.Workdir, "commit", "-m", "feat: change")
	if _, err := s.Ready(ctx, r.ID, Result{SquashCommit: &SquashCommit{Title: "feat: export customers"}, Summary: "Looks ready", Checks: []string{"Agent says tests pass"}}); err == nil {
		t.Fatal("narrative certified committed code")
	}
	verifyProduct(t, r)
	var err error
	r, err = s.Ready(ctx, r.ID, Result{SquashCommit: &SquashCommit{Title: "feat: export customers"}, Summary: "Verified", Checks: []string{"Diff check"}})
	if err != nil || len(r.Result.Evidence) != 1 {
		t.Fatal("machine reports missing", err)
	}
	if err = s.VerifyCandidate(ctx, r); err != nil {
		t.Fatal(err)
	}
	original := r.Result.Evidence[0]
	if err = os.Remove(original.Steps[0].LogPath); err != nil {
		t.Fatal(err)
	}
	verifyProduct(t, r) // A new pass must not rewrite the approval's frozen reports.
	if err = s.VerifyCandidate(ctx, r); err == nil {
		t.Fatal("new report silently replaced approval evidence")
	}
	if _, err = s.Publish(ctx, r.ID, r.ReadySHA, 1); err == nil {
		t.Fatal("published without original approval evidence")
	}
}

func TestProductSimulationsAndSquashPreserveDossierAndCandidate(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	var err error
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "A cancellation button"})
	if err != nil {
		t.Fatal(err)
	}
	recipe := []byte(`{"version":1,"directory":".","url":"http://127.0.0.1:3000","simulated":["calendar"]}`)
	if _, err = preview.Configure(r.Workdir, recipe); err != nil {
		t.Fatal(err)
	}
	runGit(t, r.Workdir, "add", preview.RecipePath)
	runGit(t, r.Workdir, "commit", "-m", "test: declare preview integrations")
	r = ready(t, s, r)
	result := *r.Result
	result.Simulated = []string{"invented"}
	result.SquashCommit = &SquashCommit{Title: "feat: cancel bookings", Body: "Allow customers to cancel before departure."}
	r, err = s.Ready(ctx, r.ID, result)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Result.Simulated) != 1 || r.Result.Simulated[0] != "calendar" {
		t.Fatalf("untrusted simulations: %+v", r.Result)
	}
	remote(t, s)
	hosting := &fakeHosting{}
	s.Hosting = hosting
	approved := r.ReadySHA
	r, err = s.Publish(ctx, r.ID, approved, 1)
	if err != nil {
		t.Fatal(err)
	}
	if hosting.title != "feat: cancel bookings" || !strings.Contains(hosting.body, "Allow customers to cancel") || !strings.Contains(hosting.body, "Original request") || !strings.Contains(hosting.body, "calendar") || !strings.Contains(hosting.body, approved) {
		t.Fatalf("squash lost portable dossier: %q %s", hosting.title, hosting.body)
	}
	hosting.pr.State = "MERGED"
	hosting.pr.HeadSHA = "developer-head"
	r, err = s.Refresh(ctx, r.ID)
	if err != nil || r.ApprovedSHA != approved || r.IntegrationSHA != "developer-head" {
		t.Fatalf("lost approval: %+v %v", r, err)
	}
	if _, err = os.Stat(r.Workdir); err != nil {
		t.Fatal("refresh removed the environment", err)
	}
}

func TestSimulationChangesRevokeCandidateAndMalformedSquashIsRejected(t *testing.T) {
	s, r := productWork(t)
	ctx := context.Background()
	var err error
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "Cancellation"})
	if err != nil {
		t.Fatal(err)
	}
	r = ready(t, s, r)
	invalid := *r.Result
	invalid.SquashCommit = nil
	if _, err = s.Ready(ctx, r.ID, invalid); err == nil {
		t.Fatal("accepted missing squash title")
	}
	invalid.SquashCommit = &SquashCommit{Title: "two\nlines"}
	if _, err = s.Ready(ctx, r.ID, invalid); err == nil {
		t.Fatal("accepted multiline squash title")
	}
	if _, err = preview.Configure(r.Workdir, []byte(`{"version":1,"directory":".","url":"http://127.0.0.1:3000","simulated":["new-fake"]}`)); err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyCandidate(ctx, r); err == nil {
		t.Fatal("changed simulations retained approval readiness")
	}
}
