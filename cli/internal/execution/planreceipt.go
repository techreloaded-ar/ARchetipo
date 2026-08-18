package execution

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// PlannedStatus is the spec status a plan receipt must declare. It is bound to
// the canonical domain constant instead of being spelled twice as a literal: a
// rename of that constant would otherwise leave a provider prompt asking for one
// word and the gate accepting another, and every valid plan would be rejected.
// The binding is a type conversion, not a connector: the execution boundary
// still cannot read or write any spec.
const PlannedStatus = string(domain.StatusPlanned)

// planReceiptFields are the keys an object must carry to be recognized as the
// receipt rather than as some other JSON the agent happened to print.
var planReceiptFields = [...]string{"spec_code", "status", "tasks"}

// PlanReceipt is the single JSON line a planning agent must emit as its closing
// message. It is what makes a successful planning observable from the ARchetipo
// side without giving the execution boundary access to the connector.
//
// It lives here, and not inside a provider package, because it is the contract
// every planning provider shares: two copies of the parsing or of the acceptance
// rule would be free to drift, and a provider accepting a receipt another one
// rejects is exactly the defect this type exists to prevent.
type PlanReceipt struct {
	SpecCode string `json:"spec_code"`
	Status   string `json:"status"`
	Tasks    int    `json:"tasks"`
}

// ParsePlanReceipt extracts the receipt from an agent output by scanning
// backwards for the last line that decodes as a JSON object carrying all the
// receipt keys. Taking the last decodable object instead would be wrong: an
// error dump or a fragment of tool output printed after the receipt is also a
// JSON object, and it would shadow a receipt that was emitted correctly.
//
// It only extracts: the policy on what counts as an acceptable receipt lives in
// AcceptPlanReceipt.
func ParsePlanReceipt(output string) (PlanReceipt, error) {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			continue
		}
		if !hasPlanReceiptFields(fields) {
			continue
		}
		var got PlanReceipt
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			continue
		}
		return got, nil
	}
	return PlanReceipt{}, fmt.Errorf("the agent did not emit the expected JSON receipt line")
}

// AcceptPlanReceipt parses the output and applies the acceptance rule: the
// receipt must declare the planned status, at least one persisted task, and the
// very spec that was dispatched. A receipt for another spec is a receipt for
// another run, not a weaker success.
//
// The two failure modes stay distinguishable on purpose: "no receipt at all"
// points at an agent that never closed its run, "a receipt that does not declare
// a plan for <specCode>" points at an agent that closed it without producing the
// plan. They call for different diagnoses, so they must not collapse into one
// message.
//
// Accepting a receipt is a necessary condition, never a sufficient one: the
// receipt is a declaration of the agent, not an inspection of the connector.
// Confirming that the spec really is PLANNED with a readable plan is done one
// layer up, where the connector is held.
func AcceptPlanReceipt(output, specCode string) (PlanReceipt, error) {
	got, err := ParsePlanReceipt(output)
	if err != nil {
		return PlanReceipt{}, err
	}
	if got.Status != PlannedStatus || got.Tasks <= 0 || got.SpecCode != specCode {
		return PlanReceipt{}, fmt.Errorf("the receipt does not declare a persisted plan for %s", specCode)
	}
	return got, nil
}

func hasPlanReceiptFields(fields map[string]json.RawMessage) bool {
	for _, field := range planReceiptFields {
		if _, ok := fields[field]; !ok {
			return false
		}
	}
	return true
}
