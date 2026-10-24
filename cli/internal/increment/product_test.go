package increment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
	items, err := s.Stories(ctx, r)
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
