package execution

import (
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// The prompt of every provider asks for one word and the gate accepts another
// unless both read the same constant, and the canonical spelling belongs to the
// domain.
func TestReviewStatusIsBoundToTheCanonicalSpecStatus(t *testing.T) {
	if ReviewStatus != string(domain.StatusReview) {
		t.Fatalf("ReviewStatus = %q, canonical = %q", ReviewStatus, domain.StatusReview)
	}
}

// Taking the last decodable JSON object would let anything printed after the
// receipt — an error dump, a fragment of tool output — shadow an implementation
// that was completed correctly.
func TestParseImplementReceiptTakesTheLastUsefulLine(t *testing.T) {
	output := strings.Join([]string{
		"implementation complete",
		`{"spec_code":"US-001","status":"REVIEW","tasks_done":1,"tests":"stale"}`,
		"re-running the suite",
		`{"spec_code":"US-001","status":"REVIEW","tasks_done":4,"tests":"go test ./...: 843 passed"}`,
		`{"level":"error","msg":"post-run telemetry flush failed"}`,
	}, "\n")
	got, err := ParseImplementReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.SpecCode != "US-001" || got.Status != ReviewStatus || got.TasksDone != 4 {
		t.Fatalf("receipt = %#v, want the last complete one", got)
	}
	if got.Tests != "go test ./...: 843 passed" {
		t.Fatalf("receipt tests = %q", got.Tests)
	}
}

func TestParseImplementReceiptRejectsAnOutputWithoutOne(t *testing.T) {
	for _, output := range []string{
		"",
		"all done",
		`{"level":"error","msg":"boom"}`,
		"{not json",
		// A plan receipt is not an implementation receipt: it carries neither
		// tasks_done nor tests.
		`{"spec_code":"US-001","status":"PLANNED","tasks":7}`,
	} {
		if _, err := ParseImplementReceipt(output); err == nil {
			t.Fatalf("output %q was accepted as a receipt", output)
		}
	}
}

func TestAcceptImplementReceiptAcceptsACompletedImplementation(t *testing.T) {
	output := strings.Join([]string{
		"work finished, spec moved to review",
		`{"spec_code":"US-001","status":"REVIEW","tasks_done":3,"tests":"go test ./...: ok"}`,
	}, "\n")
	got, err := AcceptImplementReceipt(output, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.TasksDone != 3 || got.Tests != "go test ./...: ok" {
		t.Fatalf("receipt = %#v", got)
	}
}

func TestAcceptImplementReceiptRejectsAnIncompleteDeclaration(t *testing.T) {
	cases := []struct {
		name    string
		receipt string
	}{
		{"a status other than REVIEW", `{"spec_code":"US-001","status":"IN PROGRESS","tasks_done":3,"tests":"ok"}`},
		{"no completed task", `{"spec_code":"US-001","status":"REVIEW","tasks_done":0,"tests":"ok"}`},
		{"an empty test summary", `{"spec_code":"US-001","status":"REVIEW","tasks_done":3,"tests":""}`},
		{"a blank test summary", `{"spec_code":"US-001","status":"REVIEW","tasks_done":3,"tests":"   "}`},
		{"another spec", `{"spec_code":"US-002","status":"REVIEW","tasks_done":3,"tests":"ok"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := AcceptImplementReceipt(tc.receipt, "US-001")
			if err == nil {
				t.Fatalf("receipt %s was accepted", tc.receipt)
			}
			if !strings.Contains(err.Error(), "does not declare a completed implementation for US-001") {
				t.Fatalf("error = %q, want the unacceptable-receipt message", err)
			}
		})
	}
}

// The two ways of failing call for different diagnoses — an agent that never
// closed its run, versus one that closed it without finishing the work — so
// they must not collapse into the same message.
func TestAcceptImplementReceiptKeepsItsTwoFailuresDistinguishable(t *testing.T) {
	_, missing := AcceptImplementReceipt("nothing to see here", "US-001")
	if missing == nil || !strings.Contains(missing.Error(), "did not emit the expected JSON receipt line") {
		t.Fatalf("missing-receipt error = %v", missing)
	}
	_, refused := AcceptImplementReceipt(`{"spec_code":"US-001","status":"REVIEW","tasks_done":0,"tests":"ok"}`, "US-001")
	if refused == nil || !strings.Contains(refused.Error(), "does not declare a completed implementation") {
		t.Fatalf("unacceptable-receipt error = %v", refused)
	}
}
