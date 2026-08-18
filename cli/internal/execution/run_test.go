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
