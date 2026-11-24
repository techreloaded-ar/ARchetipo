package increment

import (
	"context"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

const backlogSpecIntent = "Problem: users cannot cancel. Behavior: allow cancellation before departure."

func publishedWork(t *testing.T, storyCodes []string) (*Service, Record, *fakeHosting) {
	t.Helper()
	s, r := productWork(t)
	ctx := context.Background()
	r, err := s.Bind(ctx, r.ID, "conv-increment-thread")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Propose(ctx, r.ID, Proposal{Intent: backlogSpecIntent, StoryCodes: storyCodes})
	if err != nil {
		t.Fatal(err)
	}
	r = ready(t, s, r)
	if items, listErr := s.Connector.FetchBacklogItems(ctx, ""); listErr != nil || len(items) != 1 {
		t.Fatalf("a candidate that nobody approved reached the backlog: %+v %v", items, listErr)
	}
	remote(t, s)
	hosting := &fakeHosting{}
	s.Hosting = hosting
	r, err = s.Publish(ctx, r.ID, r.ReadySHA, 1)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, hosting
}

func TestPublicationRecordsTheIncrementAsBacklogSpecUnderReview(t *testing.T) {
	s, r, hosting := publishedWork(t, nil)
	ctx := context.Background()
	if r.Status != Proposed || r.BacklogSpecCode != "US-011" {
		t.Fatalf("publication left no backlog spec: %+v", r)
	}
	spec, err := s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Status != domain.StatusReview || spec.Title != "Improve bookings" {
		t.Fatalf("backlog spec is not waiting under review: %+v", spec)
	}
	if spec.Epic.Title != BacklogSpecEpicTitle || spec.Epic.Code != "EP-002" {
		t.Fatalf("increment without stories was not filed under its own epic: %+v", spec.Epic)
	}
	if !strings.HasPrefix(spec.Body, backlogSpecIntent) {
		t.Fatalf("backlog spec does not carry the approved specification verbatim: %q", spec.Body)
	}
	for _, expected := range []string{r.ID, "(?conversation=conv-increment-thread)", "(" + hosting.pr.URL + ")"} {
		if !strings.Contains(spec.Body, expected) {
			t.Fatalf("backlog spec does not lead back to the work, missing %q in %q", expected, spec.Body)
		}
	}

	// An interrupted publication is retried with the same approval: it must
	// adopt the spec already written rather than leave two of them.
	r.BacklogSpecCode, r.Status = "", Publishing
	if err = s.save(r); err != nil {
		t.Fatal(err)
	}
	r, err = s.Publish(ctx, r.ID, r.ReadySHA, 1)
	if err != nil || r.Status != Proposed || r.BacklogSpecCode != "US-011" || hosting.creates != 1 {
		t.Fatalf("retried publication did not converge: %+v creates=%d %v", r, hosting.creates, err)
	}
	if items, listErr := s.Connector.FetchBacklogItems(ctx, ""); listErr != nil || len(items) != 2 {
		t.Fatalf("retried publication duplicated the backlog spec: %+v %v", items, listErr)
	}

	// The PR is still open: nothing closes the spec yet.
	if r, err = s.Refresh(ctx, r.ID); err != nil || r.Status != Proposed {
		t.Fatalf("open PR changed the increment: %+v %v", r, err)
	}
	if spec, err = s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode); err != nil || spec.Status != domain.StatusReview {
		t.Fatalf("backlog spec left review before the merge: %+v %v", spec, err)
	}

	hosting.pr.State = "MERGED"
	if r, err = s.Refresh(ctx, r.ID); err != nil || r.Status != Delivered {
		t.Fatalf("merge did not deliver the increment: %+v %v", r, err)
	}
	if spec, err = s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode); err != nil || spec.Status != domain.StatusDone {
		t.Fatalf("merge did not close the backlog spec: %+v %v", spec, err)
	}
}

func TestBacklogSpecSitsBesideTheLinkedStoryWithoutTouchingIt(t *testing.T) {
	s, r, hosting := publishedWork(t, []string{"US-010"})
	ctx := context.Background()
	spec, err := s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Epic.Code != "EP-001" || spec.Epic.Title != "Product" {
		t.Fatalf("backlog spec was not filed beside the story it answers: %+v", spec.Epic)
	}
	hosting.pr.State = "MERGED"
	if r, err = s.Refresh(ctx, r.ID); err != nil || r.Status != Delivered {
		t.Fatalf("merge did not deliver the increment: %+v %v", r, err)
	}
	original, err := s.Connector.ReadSpecDetail(ctx, "US-010")
	if err != nil || original.Status != domain.StatusTodo {
		t.Fatalf("the original business story changed status with the merge: %+v %v", original, err)
	}
}

func TestMergeDeliversAnIncrementWhoseBacklogSpecWasRemoved(t *testing.T) {
	s, r, hosting := publishedWork(t, nil)
	ctx := context.Background()
	r.BacklogSpecCode = "US-999"
	if err := s.save(r); err != nil {
		t.Fatal(err)
	}
	hosting.pr.State = "MERGED"
	r, err := s.Refresh(ctx, r.ID)
	if err != nil || r.Status != Delivered {
		t.Fatalf("a removed backlog spec kept the increment open: %+v %v", r, err)
	}
}

func TestBacklogSpecLinksTheConversationOnlyWhereItCanBeOpened(t *testing.T) {
	r := Record{ID: "inc-1", ConversationID: "conv-1", Revisions: []Revision{{Number: 1, Proposal: Proposal{Intent: "Approved behavior"}}}, PR: &PullRequest{URL: "https://github.com/example/product/pull/7", Number: 7}}
	local := BacklogSpecBody(r, true)
	if !strings.HasPrefix(local, "Approved behavior\n") || !strings.Contains(local, "(?conversation=conv-1)") || !strings.Contains(local, "[Pull request #7](https://github.com/example/product/pull/7)") {
		t.Fatalf("local backlog spec body: %q", local)
	}
	shared := BacklogSpecBody(r, false)
	if strings.Contains(shared, "conversation") || !strings.Contains(shared, "inc-1") || !strings.Contains(shared, "pull/7") {
		t.Fatalf("shared tracker received a link only one machine can open: %q", shared)
	}
}
