package increment

import (
	"context"
	"reflect"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

func TestCorrectionReopensOnlyLinkedStoriesWithoutChangingApprovedRequirements(t *testing.T) {
	s := fixture(t)
	r := ready(t, s, proposeApprove(t, s, create(t, s), story("US-001", "Export"), story("US-002", "Summary")))
	before := r
	local, err := s.local(r)
	if err != nil {
		t.Fatal(err)
	}
	oldStory, err := local.ReadSpecDetail(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := local.ReadPlanBody(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.RequestChanges(context.Background(), r.ID, FeedbackRequest{Feedback: "The export includes unselected customers", Kind: "correction", StoryCodes: []string{"US-001"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != before.ID || r.Workdir != before.Workdir || r.Branch != before.Branch || r.Status != Developing || r.ReadySHA != "" || r.Result != nil || len(r.Revisions) != 2 {
		t.Fatalf("correction lost increment identity: %+v", r)
	}
	latest := r.Revisions[1]
	if !latest.Approved || !reflect.DeepEqual(latest.Proposal, before.Revisions[0].Proposal) || latest.Change == nil || latest.Change.StoryCodes[0] != "US-001" || latest.Result != nil || latest.HeadSHA != "" {
		t.Fatalf("correction changed requirements or retained acceptance: %+v", latest)
	}
	if !reflect.DeepEqual(r.Revisions[0], before.Revisions[0]) {
		t.Fatal("correction overwrote the previous verified result")
	}
	changed, _ := local.ReadSpecDetail(context.Background(), "US-001")
	unrelated, _ := local.ReadSpecDetail(context.Background(), "US-002")
	if changed.Status != domain.StatusTodo || unrelated.Status != domain.StatusDone || changed.Body != oldStory.Body || len(changed.History) != len(oldStory.History)+1 {
		t.Fatalf("incorrect rework state/history: %+v %+v", changed, unrelated)
	}
	retainedPlan, err := local.ReadPlanBody(context.Background(), "US-001")
	if err != nil || retainedPlan != plan {
		t.Fatal("correction discarded the original plan", err)
	}
	if _, err = s.Ready(context.Background(), r.ID, Result{Summary: "premature", Checks: []string{"old tasks were done"}}); err == nil {
		t.Fatal("accepted a correction before its linked story was finished again")
	}
	if _, err = local.TransitionStatus(context.Background(), "US-001", domain.StatusDone); err != nil {
		t.Fatal(err)
	}
	r, err = s.Ready(context.Background(), r.ID, Result{Summary: "Corrected", Checks: []string{"selection regression passed"}})
	if err != nil || r.Status != Review {
		t.Fatalf("correction could not return to review: %+v %v", r, err)
	}
	if _, err = s.Publish(context.Background(), r.ID, before.ReadySHA, 1); err == nil {
		t.Fatal("old approval published the corrected result")
	}
}

func TestFeedbackValidationPreservesVerifiedResult(t *testing.T) {
	s := fixture(t)
	r := ready(t, s, proposeApprove(t, s, create(t, s), story("US-001", "Export")))
	for _, change := range []FeedbackRequest{
		{Feedback: "fix", Kind: "guess"},
		{Feedback: "fix", Kind: "correction"},
		{Feedback: "fix", Kind: "refinement"},
		{Feedback: "fix", Kind: "correction", StoryCodes: []string{"US-099"}},
		{Feedback: "fix", Kind: "correction", StoryCodes: []string{"US-001", "US-001"}},
	} {
		if _, err := s.RequestChanges(context.Background(), r.ID, change); err == nil {
			t.Fatalf("accepted invalid change: %+v", change)
		}
		stored, err := s.Read(r.ID)
		if err != nil || !reflect.DeepEqual(stored, r) {
			t.Fatal("invalid feedback changed verified result", err)
		}
	}
}

func TestInterruptedCorrectionReopeningIsRetryableWithoutAnotherRevision(t *testing.T) {
	s := fixture(t)
	r := ready(t, s, proposeApprove(t, s, create(t, s), story("US-001", "Export"), story("US-002", "Summary")))
	change := FeedbackRequest{Feedback: "Correct the approved selection criteria", Kind: "correction", StoryCodes: []string{"US-001", "US-002"}}
	r, err := s.RequestChanges(context.Background(), r.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct the durable checkpoint after saving the request and reopening
	// only the first story, before the process could reopen the second one.
	local, err := s.local(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = local.TransitionStatus(context.Background(), "US-002", domain.StatusDone); err != nil {
		t.Fatal(err)
	}
	r.PendingChange = &change
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	first, _ := local.ReadSpecDetail(context.Background(), "US-001")
	if _, err = s.Ready(context.Background(), r.ID, Result{Summary: "premature", Checks: []string{"old checks"}}); err == nil {
		t.Fatal("incomplete correction could become ready")
	}
	r, err = s.RequestChanges(context.Background(), r.ID, change)
	if err != nil || r.PendingChange != nil || len(r.Revisions) != 2 || len(r.Feedback) != 1 {
		t.Fatalf("correction retry duplicated or lost work: %+v %v", r, err)
	}
	firstAgain, _ := local.ReadSpecDetail(context.Background(), "US-001")
	second, _ := local.ReadSpecDetail(context.Background(), "US-002")
	if !reflect.DeepEqual(firstAgain, first) || second.Status != domain.StatusTodo {
		t.Fatal("retry duplicated the first reopening or failed to finish the second")
	}
}

func TestRefinementMustUpdateLinkedStoryAndNeedsApproval(t *testing.T) {
	s := fixture(t)
	r := ready(t, s, proposeApprove(t, s, create(t, s), story("US-001", "Export"), story("US-002", "Summary")))
	before := r
	local, err := s.local(r)
	if err != nil {
		t.Fatal(err)
	}
	oldStory, err := local.ReadSpecDetail(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.RequestChanges(context.Background(), r.ID, FeedbackRequest{Feedback: "Use a red badge", Kind: "refinement", StoryCodes: []string{"US-001"}})
	if err != nil || r.Status != Discovery || r.PendingChange == nil {
		t.Fatalf("missing classified pending change: %+v %v", r, err)
	}
	p := before.Revisions[0].Proposal
	p.Intent += "\nRed badge requested."
	if _, err = s.Propose(context.Background(), r.ID, p); err == nil {
		t.Fatal("refinement was recorded only in intent, leaving the story unchanged")
	}
	p.Stories = append([]domain.Spec(nil), p.Stories...)
	p.Stories[0].Body += "\n- [ ] The badge is red.\n"
	r, err = s.Propose(context.Background(), r.ID, p)
	if err != nil || r.Revisions[1].Approved || r.Revisions[1].Change == nil || r.PendingChange != nil {
		t.Fatalf("refinement skipped approval or lost its story link: %+v %v", r, err)
	}
	if _, err = s.Ready(context.Background(), r.ID, Result{Summary: "not approved", Checks: []string{"test"}}); err == nil {
		t.Fatal("unapproved refinement became ready")
	}
	if _, err = s.Approve(context.Background(), r.ID, 1); err == nil {
		t.Fatal("old revision approved a changed requirement")
	}
	if _, err = s.Approve(context.Background(), r.ID, 2); err != nil {
		t.Fatal(err)
	}
	stories, err := s.Stories(context.Background(), r)
	if err != nil || len(stories) != 2 {
		t.Fatal("refinement replaced the story set", err)
	}
	for _, st := range stories {
		if st.Code == "US-001" && (st.Body != p.Stories[0].Body || st.Status != domain.StatusTodo) {
			t.Fatal("changed criteria not persisted on the existing story")
		}
		if st.Code == "US-002" && st.Status != domain.StatusDone {
			t.Fatal("unrelated completed story was reopened")
		}
		if st.Code == "US-001" && len(st.History) != len(oldStory.History)+1 {
			t.Fatal("refinement discarded the existing story's status history")
		}
	}
}
