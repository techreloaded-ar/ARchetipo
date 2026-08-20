package execution

import (
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// ReviewStatus is the spec status an implementation receipt must declare. Like
// PlannedStatus it is bound to the canonical domain constant instead of being
// spelled twice as a literal: a rename of that constant would otherwise leave a
// provider prompt asking for one word and the gate accepting another, and every
// valid implementation would be rejected. The binding is a type conversion, not
// a connector: the execution boundary still cannot read or write any spec.
const ReviewStatus = string(domain.StatusReview)

// implementReceiptFields are the keys an object must carry to be recognized as
// the implementation receipt rather than as some other JSON the agent happened
// to print.
var implementReceiptFields = []string{"spec_code", "status", "tasks_done", "tests"}

// ImplementReceipt is the single JSON line an implementing agent must emit as
// its closing message, once the spec has reached review.
//
// It lives here, and not inside a provider package, because it is the contract
// every implementing provider shares: two copies of the parsing or of the
// acceptance rule would be free to drift, and a provider accepting a receipt
// another one rejects is exactly the defect this type exists to prevent.
//
// TasksDone and Tests are informative: they are the agent's own account of the
// work and of the final suite, carried into the execution record so a reviewer
// can read a summary without opening the run. They are never the authority on
// what actually happened — the authority is the connector, and it is held one
// layer up, where the effect of the action is confirmed.
type ImplementReceipt struct {
	SpecCode  string `json:"spec_code"`
	Status    string `json:"status"`
	TasksDone int    `json:"tasks_done"`
	Tests     string `json:"tests"`
}

// ParseImplementReceipt extracts the implementation receipt from an agent
// output, taking the last line that carries every implementation receipt key
// (see parseTrailingReceipt for why the scan runs backwards).
//
// It only extracts: the policy on what counts as an acceptable receipt lives in
// AcceptImplementReceipt.
func ParseImplementReceipt(output string) (ImplementReceipt, error) {
	got, ok := parseTrailingReceipt[ImplementReceipt](output, implementReceiptFields)
	if !ok {
		return ImplementReceipt{}, fmt.Errorf("the agent did not emit the expected JSON receipt line")
	}
	return got, nil
}

// AcceptImplementReceipt parses the output and applies the acceptance rule: the
// receipt must declare the review status, at least one completed task, a
// non-empty test summary, and the very spec that was dispatched. A receipt for
// another spec is a receipt for another run, not a weaker success.
//
// The two failure modes stay distinguishable on purpose: "no receipt at all"
// points at an agent that never closed its run, "a receipt that does not
// declare a completed implementation for <specCode>" points at an agent that
// closed it without finishing the work. They call for different diagnoses, so
// they must not collapse into one message.
//
// Accepting a receipt is a necessary condition, never a sufficient one: the
// receipt is a declaration of the agent, not an inspection of the connector.
// Confirming that the spec really is in REVIEW with its plan carried out is
// done one layer up, where the connector is held.
func AcceptImplementReceipt(output, specCode string) (ImplementReceipt, error) {
	got, err := ParseImplementReceipt(output)
	if err != nil {
		return ImplementReceipt{}, err
	}
	if got.Status != ReviewStatus || got.TasksDone <= 0 || strings.TrimSpace(got.Tests) == "" || got.SpecCode != specCode {
		return ImplementReceipt{}, fmt.Errorf("the receipt does not declare a completed implementation for %s", specCode)
	}
	return got, nil
}
