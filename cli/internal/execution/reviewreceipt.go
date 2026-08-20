package execution

import (
	"fmt"
	"strings"
)

// reviewReceiptFields are the keys an object must carry to be recognized as the
// review receipt rather than as some other JSON the agent happened to print.
//
// The set is disjoint from the plan receipt's ("tasks") and from the
// implementation receipt's ("tasks_done", "tests"): no closing line of one
// action can be mistaken for the receipt of another, which matters here more
// than elsewhere because the three actions run through the same providers and
// the same scanner.
var reviewReceiptFields = []string{"spec_code", "status", "criteria", "blockers"}

// ReviewReceipt is the single JSON line a reviewing agent must emit as its
// closing message, once the review dossier of a spec has been persisted.
//
// It lives here, and not inside a provider package, because it is the contract
// every reviewing provider shares: two copies of the parsing or of the
// acceptance rule would be free to drift, and a provider accepting a receipt
// another one rejects is exactly the defect this type exists to prevent.
//
// Criteria and Blockers are the agent's own count of the acceptance criteria it
// examined and of the impediments it found. They are carried into the execution
// record so a reviewer can read a summary without opening the run; the evidence
// itself lives in the dossier attached to the spec.
type ReviewReceipt struct {
	SpecCode string `json:"spec_code"`
	Status   string `json:"status"`
	Criteria int    `json:"criteria"`
	Blockers int    `json:"blockers"`
}

// ParseReviewReceipt extracts the review receipt from an agent output, taking
// the last line that carries every review receipt key (see parseTrailingReceipt
// for why the scan runs backwards).
//
// It only extracts: the policy on what counts as an acceptable receipt lives in
// AcceptReviewReceipt.
func ParseReviewReceipt(output string) (ReviewReceipt, error) {
	got, ok := parseTrailingReceipt[ReviewReceipt](output, reviewReceiptFields)
	if !ok {
		return ReviewReceipt{}, fmt.Errorf("the agent did not emit the expected JSON receipt line")
	}
	return got, nil
}

// AcceptReviewReceipt parses the output and applies the acceptance rule: the
// receipt must declare the review status, at least one examined criterion, and
// the very spec that was dispatched. A receipt for another spec is a receipt
// for another run, not a weaker success. Blockers is informative and may be
// zero: an increment with nothing in its way is a normal outcome.
//
// The required status is the heart of the whole story. Every other receipt
// declares the status the spec has *reached*; this one declares the status the
// spec has *stayed in*. A receipt claiming DONE would be the signature of an
// agent that decided in the person's place, and it is rejected here, before the
// effect of the action is even confirmed.
//
// The two failure modes stay distinguishable on purpose: "no receipt at all"
// points at an agent that never closed its run, "a receipt that does not
// declare a prepared review dossier for <specCode>" points at an agent that
// closed it without preparing the evidence. They call for different diagnoses,
// so they must not collapse into one message.
//
// Accepting a receipt is a necessary condition, never a sufficient one: the
// receipt is a declaration of the agent, not an inspection of the connector.
// Confirming that the spec really stayed in REVIEW with a readable dossier is
// done one layer up, where the connector is held.
func AcceptReviewReceipt(output, specCode string) (ReviewReceipt, error) {
	got, err := ParseReviewReceipt(output)
	if err != nil {
		return ReviewReceipt{}, err
	}
	if got.Status != ReviewStatus || got.Criteria <= 0 || strings.TrimSpace(got.SpecCode) != specCode {
		return ReviewReceipt{}, fmt.Errorf("the receipt does not declare a prepared review dossier for %s", specCode)
	}
	return got, nil
}
