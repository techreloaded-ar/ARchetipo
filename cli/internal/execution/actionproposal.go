package execution

import "strings"

// ActionProposalArtifact is the only artifact kind an action proposal may
// declare. A line declaring another artifact is another kind of message, not a
// weaker proposal.
const ActionProposalArtifact = "action_proposal"

// actionProposalFields are the keys an object must carry to be recognized as an
// action proposal rather than as some other JSON the agent happened to print.
// The spec is deliberately not among them: a proposal about the workspace as a
// whole carries no spec, and requiring one would make it unrecognizable.
var actionProposalFields = []string{"artifact", "action"}

// ActionProposal is the single JSON line with which a conversation agent
// declares what it would start. It is a declaration of intent and nothing else:
// emitting it starts no run, writes no execution record and touches no spec —
// the conversation stays as sterile as it was without it, and the proposal
// becomes an action only after the workspace resolves it and a person confirms
// it.
//
// It reuses the receipt mechanics of this package on purpose. Recognizing the
// closing JSON line of an agent message is a rule that must not be free to
// drift between the plan receipt, the spec draft receipt and this proposal:
// a second scanning rule would be a second definition of "what the agent said
// last".
//
// Spec empty means a proposal of workspace scope. It is a legitimate absence,
// not a defect: some actions are about the workspace and have no spec to name.
type ActionProposal struct {
	Artifact string `json:"artifact"`
	Action   string `json:"action"`
	Spec     string `json:"spec,omitempty"`
}

// ParseActionProposal extracts the action proposal from an agent text, taking
// the last line that carries every action proposal key (see parseTrailingReceipt
// for why the scan runs backwards).
//
// It reports its outcome with a bool and not with an error because a message
// without a proposal is the ordinary case of a conversation, not a fault to
// diagnose: most of what an agent says is talk, and an error value would ask
// every caller to explain away a perfectly normal message.
//
// It only recognizes. Whether the proposed action exists, is allowed on this
// workspace or applies to that spec is knowledge of the process, and it lives
// where the process is known — never here.
func ParseActionProposal(text string) (ActionProposal, bool) {
	got, ok := parseTrailingReceipt[ActionProposal](text, actionProposalFields)
	if !ok {
		return ActionProposal{}, false
	}
	if got.Artifact != ActionProposalArtifact || strings.TrimSpace(got.Action) == "" {
		return ActionProposal{}, false
	}
	got.Action = strings.TrimSpace(got.Action)
	got.Spec = strings.TrimSpace(got.Spec)
	return got, true
}
