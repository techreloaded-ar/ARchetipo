package execution

import (
	"errors"
	"strings"
	"testing"
)

// A receipt closing the run is still the receipt even when the agent keeps
// talking afterwards: a human-readable epilogue or a post-run error dump must
// not shadow a backlog that was persisted correctly.
func TestParseBacklogReceiptTakesTheReceiptDespiteSurroundingNoise(t *testing.T) {
	for name, output := range map[string]string{
		"receipt alone": `{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`,
		"followed by prose": strings.Join([]string{
			`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`,
			"Il backlog è stato generato. Buon lavoro!",
		}, "\n"),
		"preceded by unrelated json": strings.Join([]string{
			`{"level":"info","msg":"session started"}`,
			`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`,
		}, "\n"),
		"followed by an error dump": strings.Join([]string{
			`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`,
			`{"level":"error","msg":"post-run telemetry flush failed"}`,
		}, "\n"),
		"followed by json without the receipt keys": strings.Join([]string{
			`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`,
			`{"artifact":"backlog","status":"WRITTEN"}`,
		}, "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseBacklogReceipt(output)
			if err != nil {
				t.Fatal(err)
			}
			if got.Artifact != "backlog" || got.Status != WrittenStatus || got.Epics != 2 || got.Specs != 3 {
				t.Fatalf("receipt = %#v", got)
			}
		})
	}
}

// A rerun inside the same session prints more than one receipt; the run is
// closed by the last one.
func TestParseBacklogReceiptKeepsTheLastReceipt(t *testing.T) {
	output := strings.Join([]string{
		`{"artifact":"backlog","status":"DRAFTED","epics":0,"specs":0}`,
		"rewriting",
		`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`,
	}, "\n")
	got, err := ParseBacklogReceipt(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != WrittenStatus || got.Specs != 3 {
		t.Fatalf("receipt = %#v, want the last one", got)
	}
}

func TestParseBacklogReceiptRejectsAnOutputWithoutOne(t *testing.T) {
	for _, output := range []string{
		"",
		"backlog generato",
		`{"level":"error","msg":"boom"}`,
		"{not json",
		`{"artifact":"backlog","status":"WRITTEN","specs":3}`,
	} {
		if got, err := ParseBacklogReceipt(output); err == nil {
			t.Fatalf("output %q was accepted as a receipt: %#v", output, got)
		}
	}
}

func TestAcceptBacklogReceiptAcceptsAWrittenBacklog(t *testing.T) {
	got, err := AcceptBacklogReceipt(`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Epics != 2 || got.Specs != 3 {
		t.Fatalf("receipt = %#v", got)
	}
}

// A missing receipt says the agent never closed its run; a receipt that
// declares no written backlog says it closed without persisting anything. The
// two diagnoses are different, so the two messages must stay different.
func TestAcceptBacklogReceiptDistinguishesItsTwoFailures(t *testing.T) {
	_, missing := AcceptBacklogReceipt("backlog generato")
	if missing == nil {
		t.Fatal("an output without a receipt was accepted")
	}
	if !strings.Contains(missing.Error(), "did not emit the expected JSON receipt line") {
		t.Fatalf("missing receipt error = %v", missing)
	}
	for name, output := range map[string]string{
		"another artifact": `{"artifact":"prd","status":"WRITTEN","epics":2,"specs":3}`,
		"another status":   `{"artifact":"backlog","status":"DRAFTED","epics":2,"specs":3}`,
		"no specs":         `{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":0}`,
		"no epics":         `{"artifact":"backlog","status":"WRITTEN","epics":0,"specs":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := AcceptBacklogReceipt(output)
			if err == nil {
				t.Fatalf("receipt %#v was accepted", got)
			}
			if !strings.Contains(err.Error(), "does not declare a written backlog") {
				t.Fatalf("error = %v", err)
			}
			if err.Error() == missing.Error() {
				t.Fatalf("the error is indistinguishable from a missing receipt: %v", err)
			}
		})
	}
}

// The backlog action is a workspace action needing its own capability, and an
// invented action still answers with an *ActionError rather than a silent
// default.
func TestBacklogActionMapsToItsCapabilityAndScope(t *testing.T) {
	capability, err := RequiredCapability(ActionBacklog)
	if err != nil || capability != CapabilityWorkspaceBacklog {
		t.Fatalf("RequiredCapability(backlog) = %q, %v; want %q", capability, err, CapabilityWorkspaceBacklog)
	}
	scope, err := ActionScope(ActionBacklog)
	if err != nil || scope != ScopeWorkspace {
		t.Fatalf("ActionScope(backlog) = %q, %v; want %q", scope, err, ScopeWorkspace)
	}
	for _, action := range []ActionID{"", "unknown", "workspace.backlog"} {
		var actionErr *ActionError
		if got, err := RequiredCapability(action); !errors.As(err, &actionErr) || got != "" {
			t.Fatalf("RequiredCapability(%q) = %q, %v; want an *ActionError", action, got, err)
		}
		if got, err := ActionScope(action); !errors.As(err, &actionErr) || got != "" {
			t.Fatalf("ActionScope(%q) = %q, %v; want an *ActionError", action, got, err)
		}
	}
}
