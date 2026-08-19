package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// plainProvider implements Provider and nothing else: it is the provider that
// executes work without exposing an interactive run.
type plainProvider struct{ testProvider }

// collaboratingProvider implements Provider and RunCollaborator, so the
// discovery helper has something real to find.
type collaboratingProvider struct {
	testProvider
	runID string
}

func (p *collaboratingProvider) ResolveRun(context.Context, Execution, map[string]any) (string, error) {
	return p.runID, nil
}

func (p *collaboratingProvider) ReadRun(context.Context, RunRequest) (RunSnapshot, error) {
	return RunSnapshot{RunID: p.runID, State: RunActive}, nil
}

func (p *collaboratingProvider) ReadRunApprovals(context.Context, RunRequest) ([]PendingApproval, error) {
	return []PendingApproval{}, nil
}

func (p *collaboratingProvider) StreamRunEvents(context.Context, RunRequest, int64, func(RunEvent) error) error {
	return nil
}

func (p *collaboratingProvider) SendRunMessage(context.Context, RunRequest, string) error { return nil }

func (p *collaboratingProvider) RespondRunApproval(context.Context, RunRequest, string, string) error {
	return nil
}

func (p *collaboratingProvider) CancelRun(context.Context, RunRequest) error { return nil }

func TestRunCollaboratorForDiscoversTheCapability(t *testing.T) {
	collaborating := &collaboratingProvider{testProvider: testProvider{id: "collaborating"}, runID: "run-9"}
	collaborator, ok := RunCollaboratorFor(collaborating)
	if !ok {
		t.Fatalf("expected the collaborating provider to be discovered")
	}
	if collaborator == nil {
		t.Fatalf("expected a usable collaborator, got nil")
	}
	// Usable, not merely non-nil: the discovered value must reach the real
	// implementation, otherwise the assertion above would pass on a wrapper that
	// does nothing.
	runID, err := collaborator.ResolveRun(context.Background(), Execution{ID: "exec-1"}, nil)
	if err != nil || runID != "run-9" {
		t.Fatalf("got %q, %v; want \"run-9\", nil", runID, err)
	}

	if collaborator, ok := RunCollaboratorFor(&plainProvider{testProvider{id: "plain"}}); ok || collaborator != nil {
		t.Fatalf("expected a plain provider not to collaborate, got %v, %v", collaborator, ok)
	}

	var missing Provider
	if collaborator, ok := RunCollaboratorFor(missing); ok || collaborator != nil {
		t.Fatalf("expected a nil provider not to collaborate, got %v, %v", collaborator, ok)
	}
}

func TestRefusalOfClassifiesEveryReason(t *testing.T) {
	reasons := []RunRefusalReason{
		RunRefusedNotFound,
		RunRefusedNotActive,
		RunRefusedRunnerOffline,
		RunRefusedUnauthorized,
		RunRefusedUnsupported,
	}
	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			got, ok := RefusalOf(&RunCommandError{Reason: reason, RunID: "run-1"})
			if !ok || got != reason {
				t.Fatalf("got %q, %v; want %q, true", got, ok, reason)
			}
		})
	}
}

func TestRefusalOfIgnoresOtherErrors(t *testing.T) {
	if reason, ok := RefusalOf(errors.New("boom")); ok || reason != "" {
		t.Fatalf("got %q, %v; want \"\", false", reason, ok)
	}
	if reason, ok := RefusalOf(nil); ok || reason != "" {
		t.Fatalf("got %q, %v; want \"\", false", reason, ok)
	}
}

func TestRunCommandErrorWrapsItsCause(t *testing.T) {
	cause := errors.New("the hub said no")
	err := error(&RunCommandError{Reason: RunRefusedNotFound, RunID: "run-1", Err: cause})
	if !errors.Is(err, cause) {
		t.Fatalf("expected the refusal to unwrap to its cause")
	}
	// A refusal is decorated on its way up through the layers; classification
	// must survive that, otherwise every caller would have to unwrap by hand.
	wrapped := fmt.Errorf("reading the run: %w", err)
	reason, ok := RefusalOf(wrapped)
	if !ok || reason != RunRefusedNotFound {
		t.Fatalf("got %q, %v; want %q, true", reason, ok, RunRefusedNotFound)
	}
	if !errors.Is(wrapped, cause) {
		t.Fatalf("expected the wrapped refusal to keep unwrapping to its cause")
	}
}

func TestRunCommandErrorMessageNamesReasonAndRun(t *testing.T) {
	message := (&RunCommandError{Reason: RunRefusedNotActive, RunID: "run-42"}).Error()
	if !strings.Contains(message, string(RunRefusedNotActive)) {
		t.Fatalf("expected %q to name the reason", message)
	}
	if !strings.Contains(message, "run-42") {
		t.Fatalf("expected %q to name the run", message)
	}
}

// failingCapabilitiesProvider is the provider whose capability discovery
// breaks. It exists to prove that DeclaredCapabilities propagates that failure
// instead of substituting an empty list for it.
type failingCapabilitiesProvider struct {
	testProvider
	err error
}

func (p *failingCapabilitiesProvider) Capabilities(context.Context) ([]Capability, error) {
	return nil, p.err
}

func TestDeclaredCapabilitiesDerivesTheDialogueFromTheInterface(t *testing.T) {
	collaborating := &collaboratingProvider{
		testProvider: testProvider{id: "collaborating", capabilities: []Capability{CapabilitySpecPlan}},
		runID:        "run-9",
	}
	got, err := DeclaredCapabilities(context.Background(), collaborating)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if want := []Capability{CapabilityRunDialog, CapabilitySpecPlan}; !equalCapabilities(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}

	plain := &plainProvider{testProvider{id: "plain", capabilities: []Capability{CapabilitySpecPlan}}}
	got, err = DeclaredCapabilities(context.Background(), plain)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if want := []Capability{CapabilitySpecPlan}; !equalCapabilities(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for _, capability := range got {
		if capability == CapabilityRunDialog {
			t.Fatalf("a provider that does not collaborate must not declare %s", CapabilityRunDialog)
		}
	}
}

func TestDeclaredCapabilitiesNeverInventsAList(t *testing.T) {
	sentinel := errors.New("capability discovery is down")
	failing := &failingCapabilitiesProvider{testProvider: testProvider{id: "failing"}, err: sentinel}
	got, err := DeclaredCapabilities(context.Background(), failing)
	if !errors.Is(err, sentinel) {
		t.Fatalf("got error %v; want the provider's own error", err)
	}
	if got != nil {
		t.Fatalf("got %v; want no list when discovery failed", got)
	}
}

func TestDeclaredCapabilitiesAlwaysReturnsAList(t *testing.T) {
	// A collaborating provider that declares nothing still declares the
	// dialogue, and a provider that declares nothing at all yields an empty
	// list rather than a nil one, so a serialized answer is [] and never null.
	collaborating := &collaboratingProvider{testProvider: testProvider{id: "silent"}}
	got, err := DeclaredCapabilities(context.Background(), collaborating)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if want := []Capability{CapabilityRunDialog}; !equalCapabilities(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}

	plain := &plainProvider{testProvider{id: "mute"}}
	got, err = DeclaredCapabilities(context.Background(), plain)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v; want an empty, non-nil list", got)
	}
}

func equalCapabilities(got, want []Capability) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
