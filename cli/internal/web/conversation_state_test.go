package web

import (
	"context"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// stubConversationalist is the smallest thing the holder accepts as the owner
// of a conversation: open() refuses without one, and these tests are about the
// register of decisions, not about what closing releases.
type stubConversationalist struct {
	closed []string
}

func (s *stubConversationalist) OpenConversation(context.Context, execution.ConversationRequest) error {
	return nil
}

func (s *stubConversationalist) CloseConversation(_ context.Context, conversationID string) error {
	s.closed = append(s.closed, conversationID)
	return nil
}

// openTestConversation opens a holder on a stub provider, so every test below
// starts from the one state in which decisions are accepted.
func openTestConversation(t *testing.T, id string) *conversationState {
	t.Helper()
	c := newConversationState()
	if err := c.open(id, "stub", &stubConversationalist{}, nil, nil, t.TempDir(), time.Now(), "", ""); err != nil {
		t.Fatalf("opening the conversation: %v", err)
	}
	return c
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
	if err := c.decide(first.ProposalID, first); err != nil {
		t.Fatalf("deciding the first proposal: %v", err)
	}
	second := confirmedOutcome(24, "Rivedi US-001", "exec-2")
	if err := c.decide(second.ProposalID, second); err != nil {
		t.Fatalf("deciding the second proposal: %v", err)
	}

	snapshot, open := c.current()
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
	if err := c.decide(declined.ProposalID, declined); err != nil {
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
	if err := c.decide(confirmed.ProposalID, confirmed); err != nil {
		t.Fatalf("confirming the same proposal: %v", err)
	}

	snapshot, _ := c.current()
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
	if err := c.decide(first.ProposalID, first); err != nil {
		t.Fatalf("deciding the first proposal: %v", err)
	}
	second := confirmedOutcome(24, "Rivedi US-001", "exec-2")
	if err := c.decide(second.ProposalID, second); err != nil {
		t.Fatalf("deciding the second proposal: %v", err)
	}

	if anchor, ok := c.anchorOf("exec-1"); !ok || anchor != 11 {
		t.Fatalf("anchorOf(exec-1) = (%d, %t), want (11, true)", anchor, ok)
	}
	if anchor, ok := c.anchorOf("exec-2"); !ok || anchor != 24 {
		t.Fatalf("anchorOf(exec-2) = (%d, %t), want (24, true)", anchor, ok)
	}
	if anchor, ok := c.anchorOf("exec-unknown"); ok || anchor != 0 {
		t.Fatalf("anchorOf(exec-unknown) = (%d, %t), want (0, false)", anchor, ok)
	}
	if anchor, ok := c.anchorOf(""); ok || anchor != 0 {
		t.Fatalf("anchorOf(\"\") = (%d, %t), want (0, false)", anchor, ok)
	}
}

// TestConversationStateForgetsDecisionsWhenTheConversationChanges says the
// register belongs to one conversation: the next one starts with nothing
// decided, and no execution of the previous one is anchored in it.
func TestConversationStateForgetsDecisionsWhenTheConversationChanges(t *testing.T) {
	c := openTestConversation(t, "conv-4")

	outcome := confirmedOutcome(11, "Implementa US-001", "exec-1")
	if err := c.decide(outcome.ProposalID, outcome); err != nil {
		t.Fatalf("deciding the proposal: %v", err)
	}
	if err := c.close(context.Background()); err != nil {
		t.Fatalf("closing the conversation: %v", err)
	}
	if err := c.open("conv-5", "stub", &stubConversationalist{}, nil, nil, t.TempDir(), time.Now(), "", ""); err != nil {
		t.Fatalf("opening the next conversation: %v", err)
	}

	snapshot, open := c.current()
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
	if anchor, ok := c.anchorOf("exec-1"); ok || anchor != 0 {
		t.Fatalf("anchorOf(exec-1) = (%d, %t), want (0, false) in a new conversation", anchor, ok)
	}
}

// TestConversationStateHandsBackACopyOfItsDecisions guards the copy: a reader
// that mutates what it received must not be rewriting the register the next
// decision appends to.
func TestConversationStateHandsBackACopyOfItsDecisions(t *testing.T) {
	c := openTestConversation(t, "conv-6")

	outcome := confirmedOutcome(11, "Implementa US-001", "exec-1")
	if err := c.decide(outcome.ProposalID, outcome); err != nil {
		t.Fatalf("deciding the proposal: %v", err)
	}

	snapshot, _ := c.current()
	if len(snapshot.outcomes) != 1 {
		t.Fatalf("outcomes = %+v, want one entry", snapshot.outcomes)
	}
	snapshot.outcomes[0].ExecutionID = "mutated"
	snapshot.outcomes[0].ProposalID = 999

	again, _ := c.current()
	if again.outcomes[0].ExecutionID != "exec-1" || again.outcomes[0].ProposalID != 11 {
		t.Fatalf("entry after mutating the received slice = %+v, want the untouched decision (proposal 11, exec-1)", again.outcomes[0])
	}
	if anchor, ok := c.anchorOf("exec-1"); !ok || anchor != 11 {
		t.Fatalf("anchorOf(exec-1) = (%d, %t), want (11, true) after the mutation", anchor, ok)
	}
}
