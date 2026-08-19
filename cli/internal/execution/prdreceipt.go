package execution

import (
	"fmt"
	"strings"
)

// WrittenStatus is the status a PRD receipt must declare. It is the word an
// inception agent uses to state that the document has been persisted through
// `archetipo prd write`, not merely drafted in the conversation.
const WrittenStatus = "WRITTEN"

// prdArtifact is the only artifact kind an inception receipt may declare. A
// receipt for another artifact is a receipt for another run, not a weaker
// success.
const prdArtifact = "prd"

// prdReceiptFields are the keys an object must carry to be recognized as the
// PRD receipt rather than as some other JSON the agent happened to print.
var prdReceiptFields = []string{"artifact", "status", "path"}

// PRDReceipt is the single JSON line an inception agent must emit as its closing
// message. It is what makes a successful inception observable from the ARchetipo
// side without giving the execution boundary access to the connector.
//
// It lives here, and not inside a provider package, because it is the contract
// every inception provider shares: two copies of the parsing or of the
// acceptance rule would be free to drift, and a provider accepting a receipt
// another one rejects is exactly the defect this type exists to prevent.
//
// Path is informative only. It is what the agent claims it wrote, useful for
// diagnosis and for a message shown to a person; it is never the authority on
// where the PRD lives. That authority stays with the configuration read through
// the connector, one layer up, which is also where the receipt is confirmed
// against the filesystem.
type PRDReceipt struct {
	Artifact string `json:"artifact"`
	Status   string `json:"status"`
	Path     string `json:"path"`
}

// ParsePRDReceipt extracts the PRD receipt from an agent output, taking the last
// line that carries every PRD receipt key (see parseTrailingReceipt for why the
// scan runs backwards).
//
// It only extracts: the policy on what counts as an acceptable receipt lives in
// AcceptPRDReceipt.
func ParsePRDReceipt(output string) (PRDReceipt, error) {
	got, ok := parseTrailingReceipt[PRDReceipt](output, prdReceiptFields)
	if !ok {
		return PRDReceipt{}, fmt.Errorf("the agent did not emit the expected JSON receipt line")
	}
	return got, nil
}

// AcceptPRDReceipt parses the output and applies the acceptance rule: the
// receipt must declare the PRD artifact, the written status, and a non-empty
// path.
//
// The two failure modes stay distinguishable on purpose, for the same reason
// documented on AcceptPlanReceipt: "no receipt at all" points at an agent that
// never closed its run, "a receipt that does not declare a written PRD" points
// at an agent that closed it without persisting the document. They call for
// different diagnoses, so they must not collapse into one message.
//
// Accepting a receipt is a necessary condition, never a sufficient one: the
// receipt is a declaration of the agent, not an inspection of the filesystem.
// Confirming that a PRD really exists at the configured path is done one layer
// up, where the connector is held.
func AcceptPRDReceipt(output string) (PRDReceipt, error) {
	got, err := ParsePRDReceipt(output)
	if err != nil {
		return PRDReceipt{}, err
	}
	if got.Artifact != prdArtifact || got.Status != WrittenStatus || strings.TrimSpace(got.Path) == "" {
		return PRDReceipt{}, fmt.Errorf("the receipt does not declare a written PRD")
	}
	return got, nil
}
