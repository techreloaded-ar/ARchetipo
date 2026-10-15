package execution

import (
	"fmt"
	"strings"
)

// ProposedStatus is the status a spec draft receipt must declare. It is the one
// receipt status in this package that states the opposite of a persistence: the
// agent has finished proposing and has deliberately written nothing, because
// the spec is created later, by a person, through the ordinary creation route.
const ProposedStatus = "PROPOSED"

// specDraftArtifact is the only artifact kind a spec draft receipt may declare.
// A receipt for another artifact is a receipt for another run, not a weaker
// success.
const specDraftArtifact = "spec_draft"

// specDraftReceiptFields are the keys an object must carry to be recognized as
// the spec draft receipt rather than as some other JSON the agent happened to
// print. Title, epic and body are part of the recognition and not only of the
// acceptance: a line that carries none of them is not this receipt at all.
var specDraftReceiptFields = []string{"artifact", "status", "title", "epic_code", "body"}

// SpecDraftReceipt is the single JSON line a spec drafting agent must emit as
// its closing message. It is what makes a finished proposal observable from the
// ARchetipo side without giving the execution boundary access to the connector.
//
// It lives here, and not inside a provider package, because it is the contract
// every drafting provider shares: two copies of the parsing or of the
// acceptance rule would be free to drift, and a provider accepting a receipt
// another one rejects is exactly the defect this type exists to prevent.
//
// Every field beyond Artifact and Status is *the proposal itself*: what the
// agent suggests the spec should say, carried back so a person can review and
// edit it. None of it is an assertion about what the workspace contains — the
// backlog is deliberately untouched when this receipt is emitted — and none of
// it is validated here: the structural rules of a spec belong to the creation
// route that will eventually persist it.
//
// Body travels on one line with escaped newlines, which is a requirement of the
// prompt rather than a limitation of the format: JSON carries the markdown
// intact, and the parsed value holds real line breaks again.
type SpecDraftReceipt struct {
	// Proposals carries a set of independently reviewable stories. The flat
	// fields remain supported for receipts emitted by already installed skills.
	Proposals []SpecDraftProposal `json:"proposals,omitempty"`
	Artifact  string              `json:"artifact"`
	Status    string              `json:"status"`
	Title     string              `json:"title"`
	EpicCode  string              `json:"epic_code"`
	EpicTitle string              `json:"epic_title,omitempty"`
	Priority  string              `json:"priority"`
	Points    int                 `json:"points"`
	Scope     string              `json:"scope"`
	BlockedBy []string            `json:"blocked_by"`
	Body      string              `json:"body"`
}

// ParseSpecDraftReceipt extracts the spec draft receipt from an agent output,
// taking the last line that carries every spec draft receipt key (see
// parseTrailingReceipt for why the scan runs backwards).
//
// It only extracts: the policy on what counts as an acceptable receipt lives in
// AcceptSpecDraftReceipt.
func ParseSpecDraftReceipt(output string) (SpecDraftReceipt, error) {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		for _, fields := range [][]string{{"artifact", "status", "proposals"}, specDraftReceiptFields} {
			if got, ok := parseTrailingReceipt[SpecDraftReceipt](lines[i], fields); ok {
				return got, nil
			}
		}
	}
	return SpecDraftReceipt{}, fmt.Errorf("the agent did not emit the expected JSON receipt line")
}

// Drafts normalizes the old single-story receipt and the proposed story set.
func (r SpecDraftReceipt) Drafts() []SpecDraftProposal {
	if r.Proposals != nil {
		drafts := append([]SpecDraftProposal{}, r.Proposals...)
		for i := range drafts {
			drafts[i].BlockedBy = append([]string{}, drafts[i].BlockedBy...)
		}
		return drafts
	}
	return []SpecDraftProposal{{Title: r.Title, EpicCode: r.EpicCode, EpicTitle: r.EpicTitle, Priority: r.Priority, Points: r.Points, Scope: r.Scope, BlockedBy: append([]string{}, r.BlockedBy...), Body: r.Body}}
}

// AcceptSpecDraftReceipt parses the output and applies the acceptance rule: the
// receipt must declare the spec draft artifact, the proposed status, and the
// three fields without which there is no proposal to review — a title, an epic
// code or a title for a new epic, and a body.
//
// The remaining fields are deliberately not required. A proposal without points
// or without blockers is a proposal a person can still read, edit and confirm;
// refusing it here would turn an incomplete suggestion into a failed run.
//
// The two failure modes stay distinguishable on purpose, for the same reason
// documented on AcceptPlanReceipt: "no receipt at all" points at an agent that
// never closed its conversation, "a receipt that does not declare a proposal"
// points at an agent that closed it without proposing anything. They call for
// different diagnoses, so they must not collapse into one message.
//
// Accepting a receipt is a necessary condition, never a sufficient one: the
// receipt is a declaration of the agent, not an inspection of the connector.
// Confirming that the run really left the backlog alone is done one layer up,
// where the connector is held.
func AcceptSpecDraftReceipt(output string) (SpecDraftReceipt, error) {
	got, err := ParseSpecDraftReceipt(output)
	if err != nil {
		return SpecDraftReceipt{}, err
	}
	if got.Artifact != specDraftArtifact || got.Status != ProposedStatus {
		return SpecDraftReceipt{}, fmt.Errorf("the receipt does not declare a proposed spec")
	}
	drafts := got.Drafts()
	if len(drafts) == 0 || (got.Proposals != nil && (got.Title != "" || got.Body != "")) {
		return SpecDraftReceipt{}, fmt.Errorf("the receipt does not declare a proposed spec")
	}
	for _, draft := range drafts {
		if strings.TrimSpace(draft.Title) == "" || (strings.TrimSpace(draft.EpicCode) == "" && strings.TrimSpace(draft.EpicTitle) == "") || strings.TrimSpace(draft.Body) == "" {
			return SpecDraftReceipt{}, fmt.Errorf("the receipt does not declare a proposed spec")
		}
	}
	return got, nil
}
