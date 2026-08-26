package localrun

import (
	"context"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// The two answers a local approval accepts, and the only two. A local process
// asks whether it may use a tool, and the question has exactly one shape: yes
// or no. Richer vocabularies — "always allow", "allow with a different input" —
// exist in the protocol underneath, and are deliberately not offered here: an
// option the viewer renders is an option somebody will press, and every one of
// them would have to mean the same thing on every local provider or the buttons
// of a run would depend on which agent happens to be behind it.
const (
	ApprovalAllow = "allow"
	ApprovalDeny  = "deny"
)

// ApprovalOptions is the answer set every local approval declares. It is built
// per call rather than shared, because the slice travels to a caller that
// serializes it and a shared one would be a piece of package state anybody
// could write into.
//
// The kinds are the ones the viewer already renders for a remote run — "allow"
// paints the affirmative button, "deny" the negative — so a decision looks the
// same whether the agent behind it is on this machine or on a hub.
func ApprovalOptions() []execution.ApprovalOption {
	return []execution.ApprovalOption{
		{ID: ApprovalAllow, Label: "Consenti", Kind: "allow"},
		{ID: ApprovalDeny, Label: "Nega", Kind: "deny"},
	}
}

// Arbiter is the optional half of a Dialogue: a live process that stops to ask
// whether it may use a tool, and can be answered.
//
// It is optional and discovered rather than required, for the same reason
// RunCollaborator is optional on a Provider: a local agent that never asks is
// not a broken one, and forcing every Dialogue to carry two methods it answers
// with an empty list would make the absence of a question look like an
// implementation gap. A dialogue that is not an Arbiter keeps exactly the
// behaviour this package has always had — no pending approvals, and a decision
// refused as unsupported.
//
// Both methods are about the process and nothing else. The pending approvals
// die with it, and that is correct: a question nobody answered before the agent
// left is a question that has no answer left to give.
type Arbiter interface {
	// PendingApprovals lists the decisions the process is waiting on, oldest
	// first. It never returns nil, so a caller that serializes the result
	// produces an empty array.
	PendingApprovals() []execution.PendingApproval
	// RespondApproval answers one pending decision. It returns an
	// *execution.RunCommandError when the answer is refused — an unknown
	// approval id, an option the approval does not offer — and any other error
	// when the answer could not be delivered at all.
	RespondApproval(ctx context.Context, approvalID, optionID string) error
}

// ArbiterOf discovers the capability on a dialogue. It returns (nil, false) for
// a dialogue that never asks and for a nil one, so a caller never has to test
// for nil before asking.
func ArbiterOf(dialogue Dialogue) (Arbiter, bool) {
	if dialogue == nil {
		return nil, false
	}
	arbiter, ok := dialogue.(Arbiter)
	if !ok {
		return nil, false
	}
	return arbiter, true
}
