package execution

import (
	"strings"
	"testing"
)

// A review receipt must be found even when the agent keeps printing after it,
// which is the same reason the other receipts scan backwards: telemetry, log
// flushes and tool output legitimately follow the closing line.
func TestParseReviewReceiptTakesTheLastUsefulLine(t *testing.T) {
	output := strings.Join([]string{
		"reading the increment",
		`{"spec_code":"US-001","status":"REVIEW","criteria":2,"blockers":1}`,
		"writing the dossier",
		`{"spec_code":"US-001","status":"REVIEW","criteria":5,"blockers":0}`,
		`{"level":"error","msg":"post-run telemetry flush failed"}`,
	}, "\n")
	got, err := ParseReviewReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.SpecCode != "US-001" || got.Status != ReviewStatus {
		t.Fatalf("receipt = %#v, want the last complete one", got)
	}
	if got.Criteria != 5 || got.Blockers != 0 {
		t.Fatalf("receipt counts = criteria %d, blockers %d", got.Criteria, got.Blockers)
	}
}

// The three receipts travel through the same providers and the same scanner, so
// a plan or an implementation closing line must never be read as a review one.
func TestParseReviewReceiptRejectsAnOutputWithoutOne(t *testing.T) {
	for _, output := range []string{
		"",
		"dossier ready",
		`{"level":"error","msg":"boom"}`,
		"{not json",
		`{"spec_code":"US-001","status":"PLANNED","tasks":7}`,
		`{"spec_code":"US-001","status":"REVIEW","tasks_done":4,"tests":"go test ./...: 843 passed"}`,
	} {
		if _, err := ParseReviewReceipt(output); err == nil {
			t.Fatalf("output %q was accepted as a review receipt", output)
		}
	}
}

func TestAcceptReviewReceiptAcceptsAPreparedDossier(t *testing.T) {
	output := strings.Join([]string{
		"evidence persisted",
		`{"spec_code":"US-001","status":"REVIEW","criteria":5,"blockers":2}`,
	}, "\n")
	got, err := AcceptReviewReceipt(output, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Criteria != 5 || got.Blockers != 2 {
		t.Fatalf("receipt = %#v", got)
	}
}

// Blockers is informative: an increment with nothing in its way is an ordinary
// outcome, not a weaker one.
func TestAcceptReviewReceiptAcceptsZeroBlockers(t *testing.T) {
	if _, err := AcceptReviewReceipt(`{"spec_code":"US-001","status":"REVIEW","criteria":3,"blockers":0}`, "US-001"); err != nil {
		t.Fatal(err)
	}
}

// The one case the whole story turns on: a receipt declaring DONE is the
// signature of an agent that decided in the person's place, and it is refused
// before the effect of the action is even looked at.
func TestAcceptReviewReceiptRejectsAReceiptThatClaimsTheSpecWasClosed(t *testing.T) {
	_, err := AcceptReviewReceipt(`{"spec_code":"US-001","status":"DONE","criteria":5,"blockers":0}`, "US-001")
	if err == nil {
		t.Fatal("a receipt declaring DONE was accepted")
	}
	if !strings.Contains(err.Error(), "US-001") {
		t.Fatalf("the rejection does not name the spec: %v", err)
	}
}

func TestAcceptReviewReceiptRejectsAnEmptyExamination(t *testing.T) {
	if _, err := AcceptReviewReceipt(`{"spec_code":"US-001","status":"REVIEW","criteria":0,"blockers":0}`, "US-001"); err == nil {
		t.Fatal("a receipt examining no criterion was accepted")
	}
}

func TestAcceptReviewReceiptRejectsAReceiptForAnotherSpec(t *testing.T) {
	if _, err := AcceptReviewReceipt(`{"spec_code":"US-002","status":"REVIEW","criteria":4,"blockers":0}`, "US-001"); err == nil {
		t.Fatal("a receipt for another spec was accepted")
	}
}

// "No receipt at all" and "a receipt that does not declare a prepared dossier"
// call for different diagnoses, so the two messages must stay apart.
func TestAcceptReviewReceiptKeepsItsTwoFailureModesDistinguishable(t *testing.T) {
	_, missing := AcceptReviewReceipt("nothing here", "US-001")
	_, wrong := AcceptReviewReceipt(`{"spec_code":"US-001","status":"DONE","criteria":5,"blockers":0}`, "US-001")
	if missing == nil || wrong == nil {
		t.Fatal("both outputs should have been rejected")
	}
	if missing.Error() == wrong.Error() {
		t.Fatalf("both failures report %q", missing.Error())
	}
}
