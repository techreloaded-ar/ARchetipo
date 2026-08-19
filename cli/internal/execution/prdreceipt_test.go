package execution

import (
	"strings"
	"testing"
)

// The prompt asks the agent for one word and the gate accepts another unless
// both read the same constant.
func TestPRDReceiptDeclaresTheWrittenStatus(t *testing.T) {
	if WrittenStatus != "WRITTEN" {
		t.Fatalf("WrittenStatus = %q", WrittenStatus)
	}
}

// A receipt closing the run is still the receipt even when the agent keeps
// talking afterwards: a human-readable epilogue must not shadow a PRD that was
// persisted correctly.
func TestParsePRDReceiptTakesTheReceiptDespiteSurroundingNoise(t *testing.T) {
	for name, output := range map[string]string{
		"receipt alone": `{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`,
		"followed by prose": strings.Join([]string{
			`{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`,
			"Il PRD è stato scritto. Buona lettura!",
		}, "\n"),
		"preceded by unrelated json": strings.Join([]string{
			`{"level":"info","msg":"session started"}`,
			`{"tool":"bash","exit":0}`,
			`{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`,
		}, "\n"),
		"followed by unrelated json": strings.Join([]string{
			`{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`,
			`{"level":"error","msg":"post-run telemetry flush failed"}`,
		}, "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParsePRDReceipt(output)
			if err != nil {
				t.Fatal(err)
			}
			if got.Artifact != "prd" || got.Status != WrittenStatus || got.Path != "docs/PRD.md" {
				t.Fatalf("receipt = %#v", got)
			}
		})
	}
}

// A rerun inside the same session prints more than one receipt; the run is
// closed by the last one.
func TestParsePRDReceiptKeepsTheLastReceipt(t *testing.T) {
	output := strings.Join([]string{
		`{"artifact":"prd","status":"DRAFTED","path":""}`,
		"rewriting",
		`{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`,
	}, "\n")
	got, err := ParsePRDReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != WrittenStatus || got.Path != "docs/PRD.md" {
		t.Fatalf("receipt = %#v, want the last one", got)
	}
}

func TestParsePRDReceiptRejectsAnOutputWithoutOne(t *testing.T) {
	for _, output := range []string{
		"",
		"inception completata",
		`{"level":"error","msg":"boom"}`,
		"{not json",
		`{"artifact":"prd","status":"WRITTEN"}`,
	} {
		if got, err := ParsePRDReceipt(output); err == nil {
			t.Fatalf("output %q was accepted as a receipt: %#v", output, got)
		}
	}
}

func TestAcceptPRDReceiptAcceptsAWrittenPRD(t *testing.T) {
	got, err := AcceptPRDReceipt(`{"artifact":"prd","status":"WRITTEN","path":"docs/PRD.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "docs/PRD.md" {
		t.Fatalf("receipt = %#v", got)
	}
}

// A missing receipt says the agent never closed its run; a receipt that
// declares no written PRD says it closed without persisting the document. The
// two diagnoses are different, so the two messages must stay different.
func TestAcceptPRDReceiptDistinguishesItsTwoFailures(t *testing.T) {
	_, missing := AcceptPRDReceipt("inception completata")
	if missing == nil {
		t.Fatal("an output without a receipt was accepted")
	}
	for name, output := range map[string]string{
		"another artifact": `{"artifact":"plan","status":"WRITTEN","path":"docs/PRD.md"}`,
		"another status":   `{"artifact":"prd","status":"DRAFTED","path":"docs/PRD.md"}`,
		"empty path":       `{"artifact":"prd","status":"WRITTEN","path":""}`,
		"blank path":       `{"artifact":"prd","status":"WRITTEN","path":"   "}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := AcceptPRDReceipt(output)
			if err == nil {
				t.Fatalf("receipt %#v was accepted", got)
			}
			if err.Error() == missing.Error() {
				t.Fatalf("the error is indistinguishable from a missing receipt: %v", err)
			}
		})
	}
}
