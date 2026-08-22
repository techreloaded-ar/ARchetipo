package web

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// stubConversationalist is the smallest thing the set accepts as the owner of a
// conversation: open() refuses without one. It records which ids it was asked
// to release, in order, because "which conversation was closed" is the whole
// question a workspace holding several has to be able to answer — and it can be
// told to fail on one of them, so a shutdown can be watched going on past a
// provider that answers badly.
type stubConversationalist struct {
	closed   []string
	failOn   string
	failWith error
}

func (s *stubConversationalist) OpenConversation(context.Context, execution.ConversationRequest) error {
	return nil
}

func (s *stubConversationalist) CloseConversation(_ context.Context, conversationID string) error {
	s.closed = append(s.closed, conversationID)
	if s.failOn != "" && s.failOn == conversationID {
		if s.failWith != nil {
			return s.failWith
		}
		return errors.New("the provider could not release " + conversationID)
	}
	return nil
}

// openTestConversation opens a set holding one conversation on a stub provider,
// so every test below starts from the one state in which decisions are
// accepted.
func openTestConversation(t *testing.T, id string) *conversationSet {
	t.Helper()
	c := newConversationSet()
	if err := c.open(id, "stub", &stubConversationalist{}, nil, nil, t.TempDir(), time.Now(), "", ""); err != nil {
		t.Fatalf("opening the conversation: %v", err)
	}
	return c
}

// openInto adds one more conversation to a set, on the shared stub, and returns
// the moment it was opened so a test can assert the order of list().
func openInto(t *testing.T, c *conversationSet, provider *stubConversationalist, id, specCode string, openedAt time.Time) {
	t.Helper()
	if err := c.open(id, "stub", provider, nil, nil, t.TempDir(), openedAt, specCode, ""); err != nil {
		t.Fatalf("opening the conversation %s: %v", id, err)
	}
}

func confirmedOutcome(proposalID int64, label, executionID string) conversationOutcome {
	return conversationOutcome{
		ProposalID:  proposalID,
		Decision:    conversationDecisionConfirmed,
		Action:      "implement",
		Label:       label,
		Scope:       template.ScopeWorkspace,
		ExecutionID: executionID,
	}
}

// TestConversationStateKeepsEveryDecisionInOrder is the register itself: two
// steps confirmed in the same conversation are two entries, in the order they
// were decided, and the last one is still what `outcome` points at.
func TestConversationStateKeepsEveryDecisionInOrder(t *testing.T) {
	c := openTestConversation(t, "conv-1")

	first := confirmedOutcome(11, "Implementa US-001", "exec-1")
	if err := c.decide("conv-1", first.ProposalID, first); err != nil {
		t.Fatalf("deciding the first proposal: %v", err)
	}
	second := confirmedOutcome(24, "Rivedi US-001", "exec-2")
	if err := c.decide("conv-1", second.ProposalID, second); err != nil {
		t.Fatalf("deciding the second proposal: %v", err)
	}

	snapshot, open := c.get("conv-1")
	if !open {
		t.Fatalf("the conversation should still be open after two decisions")
	}
	if len(snapshot.outcomes) != 2 {
		t.Fatalf("outcomes = %+v, want two entries", snapshot.outcomes)
	}
	if snapshot.outcomes[0].ProposalID != 11 || snapshot.outcomes[0].ExecutionID != "exec-1" {
		t.Fatalf("first entry = %+v, want the first decision (proposal 11, exec-1)", snapshot.outcomes[0])
	}
	if snapshot.outcomes[1].ProposalID != 24 || snapshot.outcomes[1].ExecutionID != "exec-2" {
		t.Fatalf("second entry = %+v, want the second decision (proposal 24, exec-2)", snapshot.outcomes[1])
	}
	if snapshot.outcome == nil || snapshot.outcome.ProposalID != 24 {
		t.Fatalf("outcome = %+v, want the last decision (proposal 24)", snapshot.outcome)
	}
	if snapshot.decidedProposalID != 24 {
		t.Fatalf("decidedProposalID = %d, want 24", snapshot.decidedProposalID)
	}
}

// TestConversationStateReplacesTheDecisionOfTheSameProposal states that a
// refusal answered again by a confirmation of the same proposal is one gesture:
// the register keeps one entry, the confirmed one.
func TestConversationStateReplacesTheDecisionOfTheSameProposal(t *testing.T) {
	c := openTestConversation(t, "conv-2")

	declined := conversationOutcome{
		ProposalID: 7,
		Decision:   conversationDecisionDeclined,
		Action:     "implement",
		Scope:      template.ScopeSpec,
		SpecCode:   "US-060",
	}
	if err := c.decide("conv-2", declined.ProposalID, declined); err != nil {
		t.Fatalf("declining the proposal: %v", err)
	}
	confirmed := conversationOutcome{
		ProposalID:  7,
		Decision:    conversationDecisionConfirmed,
		Action:      "implement",
		Label:       "Implementa US-060",
		Scope:       template.ScopeSpec,
		SpecCode:    "US-060",
		ExecutionID: "exec-7",
	}
	if err := c.decide("conv-2", confirmed.ProposalID, confirmed); err != nil {
		t.Fatalf("confirming the same proposal: %v", err)
	}

	snapshot, _ := c.get("conv-2")
	if len(snapshot.outcomes) != 1 {
		t.Fatalf("outcomes = %+v, want a single entry for one proposal", snapshot.outcomes)
	}
	if snapshot.outcomes[0].Decision != conversationDecisionConfirmed || snapshot.outcomes[0].ExecutionID != "exec-7" {
		t.Fatalf("entry = %+v, want the confirmed decision with exec-7", snapshot.outcomes[0])
	}
}

// TestConversationStateAnchorsAnExecutionToItsProposal is the anchor: every
// started execution knows the point of the history that asked for it, and an
// execution this conversation never started has no point in it.
func TestConversationStateAnchorsAnExecutionToItsProposal(t *testing.T) {
	c := openTestConversation(t, "conv-3")

	first := confirmedOutcome(11, "Implementa US-001", "exec-1")
	if err := c.decide("conv-3", first.ProposalID, first); err != nil {
		t.Fatalf("deciding the first proposal: %v", err)
	}
	second := confirmedOutcome(24, "Rivedi US-001", "exec-2")
	if err := c.decide("conv-3", second.ProposalID, second); err != nil {
		t.Fatalf("deciding the second proposal: %v", err)
	}

	if id, anchor, ok := c.anchorOf("exec-1"); !ok || anchor != 11 || id != "conv-3" {
		t.Fatalf("anchorOf(exec-1) = (%q, %d, %t), want (conv-3, 11, true)", id, anchor, ok)
	}
	if id, anchor, ok := c.anchorOf("exec-2"); !ok || anchor != 24 || id != "conv-3" {
		t.Fatalf("anchorOf(exec-2) = (%q, %d, %t), want (conv-3, 24, true)", id, anchor, ok)
	}
	if id, anchor, ok := c.anchorOf("exec-unknown"); ok || anchor != 0 || id != "" {
		t.Fatalf("anchorOf(exec-unknown) = (%q, %d, %t), want (\"\", 0, false)", id, anchor, ok)
	}
	if id, anchor, ok := c.anchorOf(""); ok || anchor != 0 || id != "" {
		t.Fatalf("anchorOf(\"\") = (%q, %d, %t), want (\"\", 0, false)", id, anchor, ok)
	}
}

// TestConversationStateForgetsDecisionsWhenTheConversationChanges says the
// register belongs to one conversation: the next one starts with nothing
// decided, and no execution of the previous one is anchored in it.
func TestConversationStateForgetsDecisionsWhenTheConversationChanges(t *testing.T) {
	c := openTestConversation(t, "conv-4")

	outcome := confirmedOutcome(11, "Implementa US-001", "exec-1")
	if err := c.decide("conv-4", outcome.ProposalID, outcome); err != nil {
		t.Fatalf("deciding the proposal: %v", err)
	}
	if err := c.closeOne(context.Background(), "conv-4"); err != nil {
		t.Fatalf("closing the conversation: %v", err)
	}
	if err := c.open("conv-5", "stub", &stubConversationalist{}, nil, nil, t.TempDir(), time.Now(), "", ""); err != nil {
		t.Fatalf("opening the next conversation: %v", err)
	}

	snapshot, open := c.get("conv-5")
	if !open {
		t.Fatalf("the second conversation should be open")
	}
	if len(snapshot.outcomes) != 0 {
		t.Fatalf("outcomes = %+v, want none for a fresh conversation", snapshot.outcomes)
	}
	if snapshot.outcome != nil {
		t.Fatalf("outcome = %+v, want nothing decided yet", snapshot.outcome)
	}
	if snapshot.decidedProposalID != 0 {
		t.Fatalf("decidedProposalID = %d, want 0", snapshot.decidedProposalID)
	}
	if id, anchor, ok := c.anchorOf("exec-1"); ok || anchor != 0 || id != "" {
		t.Fatalf("anchorOf(exec-1) = (%q, %d, %t), want (\"\", 0, false) in a new conversation", id, anchor, ok)
	}
}

// TestConversationStateHandsBackACopyOfItsDecisions guards the copy: a reader
// that mutates what it received must not be rewriting the register the next
// decision appends to.
func TestConversationStateHandsBackACopyOfItsDecisions(t *testing.T) {
	c := openTestConversation(t, "conv-6")

	outcome := confirmedOutcome(11, "Implementa US-001", "exec-1")
	if err := c.decide("conv-6", outcome.ProposalID, outcome); err != nil {
		t.Fatalf("deciding the proposal: %v", err)
	}

	snapshot, _ := c.get("conv-6")
	if len(snapshot.outcomes) != 1 {
		t.Fatalf("outcomes = %+v, want one entry", snapshot.outcomes)
	}
	snapshot.outcomes[0].ExecutionID = "mutated"
	snapshot.outcomes[0].ProposalID = 999

	again, _ := c.get("conv-6")
	if again.outcomes[0].ExecutionID != "exec-1" || again.outcomes[0].ProposalID != 11 {
		t.Fatalf("entry after mutating the received slice = %+v, want the untouched decision (proposal 11, exec-1)", again.outcomes[0])
	}
	if id, anchor, ok := c.anchorOf("exec-1"); !ok || anchor != 11 || id != "conv-6" {
		t.Fatalf("anchorOf(exec-1) = (%q, %d, %t), want (conv-6, 11, true) after the mutation", id, anchor, ok)
	}
}

// TestConversationSetHoldsSeveralConversationsAlive is AC-1 at the level of the
// holder: opening a second conversation while the first is alive leaves the
// first exactly as it was, and the set reports both in the order they were
// opened.
func TestConversationSetHoldsSeveralConversationsAlive(t *testing.T) {
	provider := &stubConversationalist{}
	c := newConversationSet()
	first := time.Now().UTC()
	openInto(t, c, provider, "conv-a", "US-001", first)
	openInto(t, c, provider, "conv-b", "US-002", first.Add(time.Second))

	a, aliveA := c.get("conv-a")
	if !aliveA {
		t.Fatalf("conv-a should still be alive after conv-b was opened")
	}
	if a.specCode != "US-001" || !a.openedAt.Equal(first) {
		t.Fatalf("conv-a = (spec %q, openedAt %s), want (US-001, %s) untouched by the second open", a.specCode, a.openedAt, first)
	}
	b, aliveB := c.get("conv-b")
	if !aliveB || b.specCode != "US-002" {
		t.Fatalf("conv-b = (%+v, alive %t), want a live conversation about US-002", b, aliveB)
	}

	live := c.list()
	if len(live) != 2 {
		t.Fatalf("list() = %d entries, want 2", len(live))
	}
	if live[0].id != "conv-a" || live[1].id != "conv-b" {
		t.Fatalf("list() = [%s %s], want [conv-a conv-b] in the order they were opened", live[0].id, live[1].id)
	}
}

// TestConversationSetKeepsTheDecisionWatermarkPerConversation is AC-2 at its
// lowest level: while the watermark was a field of the holder, a proposal
// decided in one conversation marked as already decided the next proposal of
// another one.
func TestConversationSetKeepsTheDecisionWatermarkPerConversation(t *testing.T) {
	provider := &stubConversationalist{}
	c := newConversationSet()
	now := time.Now().UTC()
	openInto(t, c, provider, "conv-a", "", now)
	openInto(t, c, provider, "conv-b", "", now.Add(time.Second))

	outcome := confirmedOutcome(7, "Implementa US-001", "exec-1")
	if err := c.decide("conv-a", outcome.ProposalID, outcome); err != nil {
		t.Fatalf("deciding in conv-a: %v", err)
	}

	a, _ := c.get("conv-a")
	if a.decidedProposalID != 7 {
		t.Fatalf("conv-a decidedProposalID = %d, want 7", a.decidedProposalID)
	}
	b, _ := c.get("conv-b")
	if b.decidedProposalID != 0 {
		t.Fatalf("conv-b decidedProposalID = %d, want 0: a decision taken in another conversation must not mark this one", b.decidedProposalID)
	}
	if len(b.outcomes) != 0 || b.outcome != nil {
		t.Fatalf("conv-b outcomes = (%+v, %+v), want nothing decided", b.outcomes, b.outcome)
	}
	if id, anchor, ok := c.anchorOf("exec-1"); !ok || id != "conv-a" || anchor != 7 {
		t.Fatalf("anchorOf(exec-1) = (%q, %d, %t), want (conv-a, 7, true)", id, anchor, ok)
	}
}

// TestConversationSetRefusesPastTheLimitNamingTheLiveOnes is AC-5: the refusal
// declares the limit and names every conversation that would have to be closed
// to make room, and the set is left exactly as it was.
func TestConversationSetRefusesPastTheLimitNamingTheLiveOnes(t *testing.T) {
	provider := &stubConversationalist{}
	c := newConversationSet()
	now := time.Now().UTC()
	opened := make([]string, 0, maxLiveConversations)
	for i := 0; i < maxLiveConversations; i++ {
		id := "conv-" + strconv.Itoa(i)
		openInto(t, c, provider, id, "", now.Add(time.Duration(i)*time.Second))
		opened = append(opened, id)
	}

	err := c.open("conv-too-many", "stub", provider, nil, nil, t.TempDir(), now.Add(time.Hour), "", "")
	if err == nil {
		t.Fatalf("opening one past the limit should be refused")
	}
	var limitErr *conversationLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("err = %v (%T), want a *conversationLimitError", err, err)
	}
	if limitErr.Limit != maxLiveConversations {
		t.Fatalf("Limit = %d, want %d", limitErr.Limit, maxLiveConversations)
	}
	if !slices.Equal(limitErr.LiveIDs, opened) {
		t.Fatalf("LiveIDs = %v, want exactly the live ones %v", limitErr.LiveIDs, opened)
	}
	for _, id := range opened {
		if !strings.Contains(limitErr.Error(), id) {
			t.Fatalf("the refusal %q does not name the live conversation %s", limitErr.Error(), id)
		}
	}
	if len(c.list()) != maxLiveConversations {
		t.Fatalf("list() = %d entries after the refusal, want %d: a refused open must change nothing", len(c.list()), maxLiveConversations)
	}
	if _, alive := c.get("conv-too-many"); alive {
		t.Fatalf("the refused conversation must not be held")
	}
	if len(provider.closed) != 0 {
		t.Fatalf("closed = %v, want none: refusing an open releases nothing", provider.closed)
	}
	// canOpen answers with the same refusal, which is what lets the routes check
	// the limit before a process exists.
	var beforeErr *conversationLimitError
	if !errors.As(c.canOpen(), &beforeErr) || beforeErr.Limit != maxLiveConversations {
		t.Fatalf("canOpen() = %v, want the same limit refusal", c.canOpen())
	}
}

// TestConversationSetClosesOneAndLeavesTheOthers is AC-4: closing a
// conversation releases that agent process and no other, and every sibling is
// still held, unchanged.
func TestConversationSetClosesOneAndLeavesTheOthers(t *testing.T) {
	provider := &stubConversationalist{}
	c := newConversationSet()
	now := time.Now().UTC()
	openInto(t, c, provider, "conv-a", "", now)
	openInto(t, c, provider, "conv-b", "", now.Add(time.Second))

	if err := c.closeOne(context.Background(), "conv-b"); err != nil {
		t.Fatalf("closing conv-b: %v", err)
	}
	if !slices.Equal(provider.closed, []string{"conv-b"}) {
		t.Fatalf("closed = %v, want only conv-b", provider.closed)
	}
	if _, alive := c.get("conv-b"); alive {
		t.Fatalf("conv-b should no longer be held")
	}
	a, alive := c.get("conv-a")
	if !alive {
		t.Fatalf("conv-a should still be alive after conv-b was closed")
	}
	if !a.openedAt.Equal(now) {
		t.Fatalf("conv-a openedAt = %s, want %s untouched", a.openedAt, now)
	}
}

// TestConversationSetIgnoresClosingAnUnknownConversation states that closing an
// id the workspace is not holding is innocuous: nothing is released and nothing
// is dropped, exactly like CloseConversation itself.
func TestConversationSetIgnoresClosingAnUnknownConversation(t *testing.T) {
	provider := &stubConversationalist{}
	c := newConversationSet()
	openInto(t, c, provider, "conv-a", "", time.Now().UTC())

	if err := c.closeOne(context.Background(), "conv-unknown"); err != nil {
		t.Fatalf("closing an unknown conversation should be innocuous, got %v", err)
	}
	if len(provider.closed) != 0 {
		t.Fatalf("closed = %v, want none", provider.closed)
	}
	if len(c.list()) != 1 {
		t.Fatalf("list() = %d entries, want the one that was already there", len(c.list()))
	}
}

// TestConversationSetShutdownReleasesEveryConversationDespiteAFailure is AC-6:
// a process left alive because the previous one failed to close is exactly what
// leaving a workspace must not leave behind.
func TestConversationSetShutdownReleasesEveryConversationDespiteAFailure(t *testing.T) {
	failure := errors.New("the runtime did not answer")
	provider := &stubConversationalist{failOn: "conv-b", failWith: failure}
	c := newConversationSet()
	now := time.Now().UTC()
	openInto(t, c, provider, "conv-a", "", now)
	openInto(t, c, provider, "conv-b", "", now.Add(time.Second))
	openInto(t, c, provider, "conv-c", "", now.Add(2*time.Second))

	err := c.shutdown(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("shutdown() = %v, want the failure of conv-b to be reported", err)
	}
	if !slices.Equal(provider.closed, []string{"conv-a", "conv-b", "conv-c"}) {
		t.Fatalf("closed = %v, want every conversation released despite the failure on conv-b", provider.closed)
	}
	if len(c.list()) != 0 {
		t.Fatalf("list() = %d entries after shutdown, want none", len(c.list()))
	}
	if err := c.open("conv-d", "stub", provider, nil, nil, t.TempDir(), now.Add(time.Hour), "", ""); err == nil {
		t.Fatalf("opening a conversation after shutdown should be refused: nothing would be left to close it")
	}
}
