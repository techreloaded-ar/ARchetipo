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
	if r.Status != Developing || r.Revisions[0].Approved {
		t.Fatalf("invented acceptance: %+v", r)
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
	if r.ReadySHA != "" || r.Status != Developing || len(r.Revisions) != 2 {
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
	if _, err = s.Ready(ctx, r.ID, Result{Summary: "Old scope", Checks: []string{"old proof"}}); err == nil {
		t.Fatal("feedback reused stale specification")
	}
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: "Second scope"})
	if err != nil {
		t.Fatal(err)
	}
	dossier := Dossier(r)
	for _, part := range []string{"First scope", "Second scope", previous, "Change the scope", "go test ./..."} {
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
