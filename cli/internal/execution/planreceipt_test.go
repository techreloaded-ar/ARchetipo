package execution

import (
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// The prompt of every provider asks for one word and the gate accepts another
// unless both read the same constant, and the canonical spelling belongs to the
// domain.
func TestPlannedStatusIsBoundToTheCanonicalSpecStatus(t *testing.T) {
	if PlannedStatus != string(domain.StatusPlanned) {
		t.Fatalf("PlannedStatus = %q, canonical = %q", PlannedStatus, domain.StatusPlanned)
	}
}

// Taking the last decodable JSON object would let anything printed after the
// receipt — an error dump, a fragment of tool output — shadow a plan that was
// produced correctly.
func TestParsePlanReceiptIgnoresJSONPrintedAfterTheReceipt(t *testing.T) {
	output := strings.Join([]string{
		"planning complete",
		`{"spec_code":"US-001","status":"PLANNED","tasks":7}`,
		`{"level":"error","msg":"post-run telemetry flush failed"}`,
		`{"tool":"bash","exit":0}`,
	}, "\n")
	got, err := ParsePlanReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.SpecCode != "US-001" || got.Status != PlannedStatus || got.Tasks != 7 {
		t.Fatalf("receipt = %#v", got)
	}
}

// A rerun inside the same session prints more than one receipt; the run is
// closed by the last one, not by the first.
func TestParsePlanReceiptKeepsTheLastValidReceipt(t *testing.T) {
	output := strings.Join([]string{
		`{"spec_code":"US-001","status":"PLANNED","tasks":2}`,
		"replanning",
		`{"spec_code":"US-001","status":"PLANNED","tasks":9}`,
	}, "\n")
	got, err := ParsePlanReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tasks != 9 {
		t.Fatalf("receipt = %#v, want the last one", got)
	}
}

func TestParsePlanReceiptRejectsAnOutputWithoutOne(t *testing.T) {
	for _, output := range []string{"", "all done", `{"level":"error","msg":"boom"}`, "{not json"} {
		if _, err := ParsePlanReceipt(output); err == nil {
			t.Fatalf("output %q was accepted as a receipt", output)
		}
	}
}

func TestAcceptPlanReceiptAcceptsAReceiptForTheDispatchedSpec(t *testing.T) {
	got, err := AcceptPlanReceipt(`{"spec_code":"US-001","status":"PLANNED","tasks":4}`, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Tasks != 4 {
		t.Fatalf("receipt = %#v", got)
	}
}

// A missing receipt and a receipt that declares no plan call for different
// diagnoses, so they must not collapse into the same message.
func TestAcceptPlanReceiptDistinguishesItsTwoFailures(t *testing.T) {
	_, missing := AcceptPlanReceipt("all done", "US-001")
	if missing == nil {
		t.Fatal("an output without a receipt was accepted")
	}
	for name, output := range map[string]string{
		"another spec":  `{"spec_code":"US-002","status":"PLANNED","tasks":3}`,
		"no task":       `{"spec_code":"US-001","status":"PLANNED","tasks":0}`,
		"wrong status":  `{"spec_code":"US-001","status":"TODO","tasks":3}`,
		"negative task": `{"spec_code":"US-001","status":"PLANNED","tasks":-1}`,
	} {
		got, err := AcceptPlanReceipt(output, "US-001")
		if err == nil {
			t.Fatalf("%s: receipt %#v was accepted", name, got)
		}
		if err.Error() == missing.Error() {
			t.Fatalf("%s: the error is indistinguishable from a missing receipt: %v", name, err)
		}
		if !strings.Contains(err.Error(), "US-001") {
			t.Fatalf("%s: the error does not name the spec: %v", name, err)
		}
	}
}
