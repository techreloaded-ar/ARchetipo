package execution

import (
	"fmt"
)

// backlogArtifact is the only artifact kind a backlog receipt may declare. A
// receipt for another artifact is a receipt for another run, not a weaker
// success.
const backlogArtifact = "backlog"

// backlogReceiptFields are the keys an object must carry to be recognized as the
// backlog receipt rather than as some other JSON the agent happened to print.
var backlogReceiptFields = []string{"artifact", "status", "epics", "specs"}

// BacklogReceipt is the single JSON line a backlog generation agent must emit as
// its closing message. It is what makes a successful generation observable from
// the ARchetipo side without giving the execution boundary access to the
// connector.
//
// It lives here, and not inside a provider package, because it is the contract
// every backlog provider shares: two copies of the parsing or of the acceptance
// rule would be free to drift, and a provider accepting a receipt another one
// rejects is exactly the defect this type exists to prevent.
//
// Epics and Specs are informative only. They are what the agent claims it
// persisted, useful for diagnosis and for a message shown to a person; they are
// never the authority on what the connector actually contains. That authority
// stays one layer up, where the connector is held and the backlog is re-read.
type BacklogReceipt struct {
	Artifact string `json:"artifact"`
	Status   string `json:"status"`
	Epics    int    `json:"epics"`
	Specs    int    `json:"specs"`
}

// ParseBacklogReceipt extracts the backlog receipt from an agent output, taking
// the last line that carries every backlog receipt key (see parseTrailingReceipt
// for why the scan runs backwards).
//
// It only extracts: the policy on what counts as an acceptable receipt lives in
// AcceptBacklogReceipt.
func ParseBacklogReceipt(output string) (BacklogReceipt, error) {
	got, ok := parseTrailingReceipt[BacklogReceipt](output, backlogReceiptFields)
	if !ok {
		return BacklogReceipt{}, fmt.Errorf("the agent did not emit the expected JSON receipt line")
	}
	return got, nil
}

// AcceptBacklogReceipt parses the output and applies the acceptance rule: the
// receipt must declare the backlog artifact, the written status, and at least
// one epic and one spec.
//
// The two failure modes stay distinguishable on purpose, for the same reason
// documented on AcceptPlanReceipt: "no receipt at all" points at an agent that
// never closed its run, "a receipt that does not declare a written backlog"
// points at an agent that closed it without persisting the backlog. They call
// for different diagnoses, so they must not collapse into one message.
//
// Accepting a receipt is a necessary condition, never a sufficient one: the
// receipt is a declaration of the agent, not an inspection of the connector.
// Confirming that epics and specs really exist is done one layer up, where the
// connector is held.
func AcceptBacklogReceipt(output string) (BacklogReceipt, error) {
	got, err := ParseBacklogReceipt(output)
	if err != nil {
		return BacklogReceipt{}, err
	}
	if got.Artifact != backlogArtifact || got.Status != WrittenStatus || got.Specs <= 0 || got.Epics <= 0 {
		return BacklogReceipt{}, fmt.Errorf("the receipt does not declare a written backlog")
	}
	return got, nil
}
