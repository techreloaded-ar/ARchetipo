package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

// nominalDraftLine is the receipt a conforming agent closes a proposal on. It is
// built through the encoder rather than written by hand so the test cannot
// accidentally assert a JSON string that no encoder would ever produce.
func nominalDraftLine(t *testing.T, receipt SpecDraftReceipt) string {
	t.Helper()
	line, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encoding the fixture receipt: %v", err)
	}
	if strings.Contains(string(line), "\n") {
		t.Fatalf("the encoded receipt spans more than one line: %s", line)
	}
	return string(line)
}

func fixtureDraft() SpecDraftReceipt {
	return SpecDraftReceipt{
		Artifact:  "spec_draft",
		Status:    ProposedStatus,
		Title:     "Esportare il backlog in CSV",
		EpicCode:  "EP-005",
		Priority:  "MEDIUM",
		Points:    3,
		Scope:     "MVP",
		BlockedBy: []string{"US-001", "US-002"},
		Body:      "**User Story**\nCome analista, voglio esportare il backlog.",
	}
}

// A receipt closing the run is still the receipt even when the agent keeps
// talking afterwards: an epilogue or a post-run error dump must not shadow a
// proposal that was produced correctly.
func TestParseSpecDraftReceiptTakesTheReceiptDespiteSurroundingNoise(t *testing.T) {
	line := nominalDraftLine(t, fixtureDraft())
	for name, output := range map[string]string{
		"receipt alone": line,
		"followed by prose": strings.Join([]string{
			line,
			"Ecco la spec che propongo. Fammi sapere se va bene.",
		}, "\n"),
		"preceded by unrelated json": strings.Join([]string{
			`{"level":"info","msg":"session started"}`,
			line,
		}, "\n"),
		"followed by an error dump": strings.Join([]string{
			line,
			`{"level":"error","msg":"post-run telemetry flush failed"}`,
		}, "\n"),
		"followed by json without the receipt keys": strings.Join([]string{
			line,
			`{"artifact":"spec_draft","status":"PROPOSED"}`,
		}, "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseSpecDraftReceipt(output)
			if err != nil {
				t.Fatal(err)
			}
			if got.Title != fixtureDraft().Title || got.EpicCode != "EP-005" {
				t.Fatalf("receipt = %#v", got)
			}
		})
	}
}

// A rerun inside the same conversation prints more than one receipt; the run is
// closed by the last one.
func TestParseSpecDraftReceiptKeepsTheLastReceipt(t *testing.T) {
	first := fixtureDraft()
	first.Title = "Prima proposta"
	second := fixtureDraft()
	second.Title = "Seconda proposta"
	output := strings.Join([]string{
		nominalDraftLine(t, first),
		"ho rivisto la proposta",
		nominalDraftLine(t, second),
	}, "\n")

	got, err := ParseSpecDraftReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Seconda proposta" {
		t.Fatalf("receipt = %#v, want the last one", got)
	}
}

// The whole proposal must survive the trip, because it is what a person will
// read and edit before confirming: every field of the creation form is carried
// by the receipt and none of them may be dropped or altered on the way.
func TestAcceptSpecDraftReceiptReturnsTheWholeProposal(t *testing.T) {
	want := fixtureDraft()

	got, err := AcceptSpecDraftReceipt(nominalDraftLine(t, want))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != want.Title {
		t.Fatalf("title = %q, want %q", got.Title, want.Title)
	}
	if got.EpicCode != want.EpicCode {
		t.Fatalf("epic_code = %q, want %q", got.EpicCode, want.EpicCode)
	}
	if got.Priority != want.Priority {
		t.Fatalf("priority = %q, want %q", got.Priority, want.Priority)
	}
	if got.Points != want.Points {
		t.Fatalf("points = %d, want %d", got.Points, want.Points)
	}
	if got.Scope != want.Scope {
		t.Fatalf("scope = %q, want %q", got.Scope, want.Scope)
	}
	if strings.Join(got.BlockedBy, ",") != strings.Join(want.BlockedBy, ",") {
		t.Fatalf("blocked_by = %v, want %v", got.BlockedBy, want.BlockedBy)
	}
	if got.Body != want.Body {
		t.Fatalf("body = %q, want %q", got.Body, want.Body)
	}
}

// The markdown body is the field the transport could plausibly damage: it holds
// real line breaks and markdown punctuation while travelling on a single JSON
// line. Asserting the round trip is what makes "the proposal is shown in full"
// a proven property rather than an assumption about the encoder.
func TestAcceptSpecDraftReceiptRestoresAMultilineMarkdownBody(t *testing.T) {
	body := strings.Join([]string{
		"**User Story**",
		"Come analista, voglio esportare il backlog in CSV,",
		"così da analizzarlo fuori dallo strumento.",
		"",
		"**Criteri di accettazione**",
		"- [ ] AC-1 — L'esportazione produce un file `backlog.csv`.",
		"- [ ] AC-2 — Le colonne includono #, codice e stato.",
	}, "\n")
	receipt := fixtureDraft()
	receipt.Body = body

	got, err := AcceptSpecDraftReceipt(nominalDraftLine(t, receipt))
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != body {
		t.Fatalf("body = %q, want %q", got.Body, body)
	}
	if lines := strings.Split(got.Body, "\n"); len(lines) != 7 {
		t.Fatalf("body has %d lines, want 7", len(lines))
	}
}

// A proposal without points or blockers is still a proposal a person can read,
// edit and confirm: refusing it would turn an incomplete suggestion into a
// failed run.
func TestAcceptSpecDraftReceiptAcceptsAProposalWithoutOptionalFields(t *testing.T) {
	receipt := SpecDraftReceipt{
		Artifact: "spec_draft",
		Status:   ProposedStatus,
		Title:    "Esportare il backlog in CSV",
		EpicCode: "EP-005",
		Body:     "**User Story**\nCome analista...",
	}

	got, err := AcceptSpecDraftReceipt(nominalDraftLine(t, receipt))
	if err != nil {
		t.Fatal(err)
	}
	if got.Points != 0 || len(got.BlockedBy) != 0 || got.Priority != "" {
		t.Fatalf("receipt = %#v, want the optional fields left empty", got)
	}
}

// The two failure modes must stay distinguishable: an agent that never closed
// its conversation and an agent that closed it without proposing anything call
// for different diagnoses.
func TestAcceptSpecDraftReceiptKeepsItsTwoFailureModesApart(t *testing.T) {
	t.Run("no receipt at all", func(t *testing.T) {
		for name, output := range map[string]string{
			"empty":                 "",
			"prose only":            "Ho preparato la proposta, dimmi tu.",
			"malformed json":        `{"artifact":"spec_draft","status":"PROPOSED","title":"x","epic_code":"EP-1","body":`,
			"missing every key":     `{"level":"info","msg":"done"}`,
			"missing the body key":  `{"artifact":"spec_draft","status":"PROPOSED","title":"x","epic_code":"EP-1"}`,
			"not alone on its line": "prefisso " + `{"artifact":"spec_draft","status":"PROPOSED","title":"x","epic_code":"EP-1","body":"y"}`,
		} {
			t.Run(name, func(t *testing.T) {
				if _, err := AcceptSpecDraftReceipt(output); err == nil {
					t.Fatal("accepted an output carrying no receipt")
				} else if !strings.Contains(err.Error(), "did not emit the expected JSON receipt line") {
					t.Fatalf("error = %v, want the missing-receipt diagnosis", err)
				}
			})
		}
	})

	t.Run("a receipt that does not declare a proposal", func(t *testing.T) {
		cases := map[string]func(*SpecDraftReceipt){
			"another artifact": func(r *SpecDraftReceipt) { r.Artifact = "backlog" },
			"another status":   func(r *SpecDraftReceipt) { r.Status = WrittenStatus },
			"empty title":      func(r *SpecDraftReceipt) { r.Title = "" },
			"blank title":      func(r *SpecDraftReceipt) { r.Title = "   " },
			"empty epic":       func(r *SpecDraftReceipt) { r.EpicCode = "" },
			"blank epic":       func(r *SpecDraftReceipt) { r.EpicCode = "\t" },
			"empty body":       func(r *SpecDraftReceipt) { r.Body = "" },
			"blank body":       func(r *SpecDraftReceipt) { r.Body = "\n  \n" },
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				receipt := fixtureDraft()
				mutate(&receipt)
				if _, err := AcceptSpecDraftReceipt(nominalDraftLine(t, receipt)); err == nil {
					t.Fatal("accepted a receipt that declares no proposal")
				} else if !strings.Contains(err.Error(), "does not declare a proposed spec") {
					t.Fatalf("error = %v, want the no-proposal diagnosis", err)
				}
			})
		}
	})
}
