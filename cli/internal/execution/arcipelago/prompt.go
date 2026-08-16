package arcipelago

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// plannedStatus is the spec status the receipt must declare. It is bound to the
// canonical domain constant instead of being spelled twice as a literal: a
// rename of that constant would otherwise leave the prompt asking for one word
// and the gate accepting another, and every valid plan would be rejected. The
// binding is a type conversion, not a connector: the package still cannot read
// or write any spec.
const plannedStatus = string(domain.StatusPlanned)

// receiptFields are the keys an object must carry to be recognized as the
// receipt rather than as some other JSON the agent happened to print.
var receiptFields = [...]string{"spec_code", "status", "tasks"}

// receipt is the single JSON line the remote agent must emit as its closing
// message. It is what makes AC-2 observable from the ARchetipo side without
// giving the execution boundary access to the connector.
type receipt struct {
	SpecCode string `json:"spec_code"`
	Status   string `json:"status"`
	Tasks    int    `json:"tasks"`
}

// buildTask renders the remote task title, prompt and metadata.
//
// It must stay a pure, deterministic function of the fields of req: ARcipelago
// does not key external equivalence on (workspaceId, source, externalId) alone,
// it also compares a canonical SHA-256 fingerprint over title, prompt and
// metadata (packages/hub/src/db/tasks-repository.ts:87,445-459). A repetition
// carrying the same identity but a different payload answers
// 409 external_task_conflict instead of 200, so any timestamp, random value,
// local path or environment lookup in here would break idempotency. Key order
// is irrelevant — the hub canonicalizes objects by sorting keys — but every
// value must be byte-identical across two calls with the same Request.
//
// metadata is always a non-nil map: the remote contract requires a JSON object
// (packages/hub/src/api/app.ts:379 via asRecord), and a nil Go map serializes
// as null, which the hub rejects with 400.
func buildTask(req execution.Request) (title, prompt string, metadata map[string]any) {
	title = "ARchetipo plan " + req.SpecCode
	prompt = strings.Join([]string{
		"Work in the runner working directory: it is a checkout of the ARchetipo project with the archetipo CLI and the ARchetipo skills already installed.",
		"Plan the spec " + req.SpecCode + " by invoking the ARchetipo planning skill:",
		"",
		"/archetipo-plan " + req.SpecCode,
		"",
		"Persist the plan through the configured connector, exactly as the skill prescribes. Do not paste the plan into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + plannedStatus + `","tasks":<N>}`,
		"",
		"<N> is the number of tasks of the plan you actually persisted. Emit the receipt only after the plan is persisted and the spec is " + plannedStatus + ".",
	}, "\n")
	metadata = map[string]any{
		"spec_code":    req.SpecCode,
		"action":       string(req.Action),
		"capability":   string(req.Capability),
		"execution_id": req.ExecutionID,
	}
	return title, prompt, metadata
}

// parseReceipt extracts the receipt from the remote result summary by scanning
// backwards for the last line that decodes as a JSON object carrying all the
// receipt keys. Taking the last decodable object instead would be wrong: an
// error dump or a fragment of tool output printed after the receipt is also a
// JSON object, and it would shadow a receipt that was emitted correctly. It only
// extracts: the policy on what counts as an acceptable receipt lives in Execute.
func parseReceipt(resultSummary string) (receipt, error) {
	lines := strings.Split(resultSummary, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			continue
		}
		if !hasReceiptFields(fields) {
			continue
		}
		var got receipt
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			continue
		}
		return got, nil
	}
	return receipt{}, fmt.Errorf("the remote agent did not emit the expected JSON receipt line")
}

func hasReceiptFields(fields map[string]json.RawMessage) bool {
	for _, field := range receiptFields {
		if _, ok := fields[field]; !ok {
			return false
		}
	}
	return true
}
