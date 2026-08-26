package localrun

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// Collaborator implements execution.RunCollaborator over a registry of local
// sessions. A local provider embeds one and becomes able to be followed and
// commanded without writing a single rule of its own.
//
// The assertion below is what keeps the seven methods from drifting away from
// the interface: a missing or diverging one stops the build here rather than at
// a type assertion that quietly returns false at runtime.
type Collaborator struct {
	registry *Registry
}

var _ execution.RunCollaborator = (*Collaborator)(nil)

func NewCollaborator(registry *Registry) *Collaborator {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Collaborator{registry: registry}
}

// Registry exposes the sessions so a provider can register the one it just
// opened.
func (c *Collaborator) Registry() *Registry { return c.registry }

func refusal(reason execution.RunRefusalReason, runID string, cause error) error {
	return &execution.RunCommandError{Reason: reason, RunID: runID, Err: cause}
}

// session resolves a run id, or refuses. A run this process never opened is
// not_found; a run it opened and that has ended is run_not_active, which is a
// different situation with a different remedy.
func (c *Collaborator) session(runID string) (*Session, error) {
	id := strings.TrimSpace(runID)
	if id == "" {
		return nil, refusal(execution.RunRefusedNotFound, runID, fmt.Errorf("no run was named"))
	}
	session, ok := c.registry.Lookup(id)
	if !ok {
		return nil, refusal(execution.RunRefusedNotFound, id, fmt.Errorf("this process holds no such local run"))
	}
	return session, nil
}

// commandable resolves a run that may still be commanded.
func (c *Collaborator) commandable(runID string) (*Session, error) {
	session, err := c.session(runID)
	if err != nil {
		return nil, err
	}
	if !session.Active() {
		return nil, refusal(execution.RunRefusedNotActive, runID, fmt.Errorf("the local run has already ended"))
	}
	dialogue := session.dialogueOf()
	if dialogue == nil {
		return nil, refusal(execution.RunRefusedRunnerOffline, runID, fmt.Errorf("the local process is not attached yet"))
	}
	return session, nil
}

// ResolveRun maps an execution record onto the local run that carries it. A
// local run is identified by the execution it serves, so the id is the record's
// own. An execution with no session is answered with an empty id and no error:
// absence is an answer, exactly as the interface prescribes.
func (c *Collaborator) ResolveRun(_ context.Context, exec execution.Execution, _ map[string]any) (string, error) {
	id := strings.TrimSpace(exec.ID)
	if id == "" {
		return "", nil
	}
	if _, ok := c.registry.Lookup(id); !ok {
		return "", nil
	}
	return id, nil
}

func (c *Collaborator) ReadRun(_ context.Context, req execution.RunRequest) (execution.RunSnapshot, error) {
	session, err := c.session(req.RunID)
	if err != nil {
		return execution.RunSnapshot{}, err
	}
	return session.Snapshot(), nil
}

// ReadRunApprovals lists the decisions the live process is waiting on.
//
// It resolves the run rather than requiring it to be commandable: a run that
// has ended has no pending decision, and that is an answer — an empty list —
// and not the refusal a command sent to it would deserve. The same is true of
// a dialogue that never asks: absence of questions is not absence of an answer.
// Returning a non-nil slice keeps a caller that serializes the result producing
// [] instead of null.
func (c *Collaborator) ReadRunApprovals(_ context.Context, req execution.RunRequest) ([]execution.PendingApproval, error) {
	session, err := c.session(req.RunID)
	if err != nil {
		return nil, err
	}
	arbiter, asks := ArbiterOf(session.dialogueOf())
	if !asks {
		return []execution.PendingApproval{}, nil
	}
	pending := arbiter.PendingApprovals()
	if pending == nil {
		return []execution.PendingApproval{}, nil
	}
	return pending, nil
}

func (c *Collaborator) StreamRunEvents(ctx context.Context, req execution.RunRequest, afterID int64, sink func(execution.RunEvent) error) error {
	session, err := c.session(req.RunID)
	if err != nil {
		return err
	}
	return session.Stream(ctx, afterID, sink)
}

// SendRunMessage hands a message to the live process and writes nothing.
//
// The message becomes history when the process re-emits it, never before. That
// is not a detail of presentation: a line written locally would show the
// operator something the agent may never have received, and it is also what
// makes a refused command free of consequences — a command that wrote nothing
// has nothing to undo.
func (c *Collaborator) SendRunMessage(ctx context.Context, req execution.RunRequest, message string) error {
	text := strings.TrimSpace(message)
	if text == "" {
		// The caller's mistake, decided here: sending it would spend a round trip
		// to be told so.
		return refusal(execution.RunRefusedUnsupported, req.RunID, fmt.Errorf("the message is empty"))
	}
	session, err := c.commandable(req.RunID)
	if err != nil {
		return err
	}
	return deliver(session.dialogueOf().Send(ctx, text))
}

// RespondRunApproval hands one decision back to the live process.
//
// It goes through commandable for the same reason SendRunMessage does: an
// answer is a command, and a run that has ended or has nothing attached cannot
// take one. A dialogue that never asks keeps the refusal it has always given —
// there is no decision of its to answer — and it is a refusal and not a fault,
// because nothing about the run is broken.
//
// It writes no history. The decision becomes visible through what the process
// does with it: a tool that runs, or a tool result reporting the refusal. That
// is the same rule SendRunMessage follows, and for the same reason — a line
// written locally would show the operator something the agent may never have
// received.
func (c *Collaborator) RespondRunApproval(ctx context.Context, req execution.RunRequest, approvalID, optionID string) error {
	session, err := c.commandable(req.RunID)
	if err != nil {
		return err
	}
	arbiter, asks := ArbiterOf(session.dialogueOf())
	if !asks {
		return refusal(execution.RunRefusedUnsupported, req.RunID, fmt.Errorf("this local run asks for no approval"))
	}
	if strings.TrimSpace(approvalID) == "" {
		return refusal(execution.RunRefusedUnsupported, req.RunID, fmt.Errorf("no approval was named"))
	}
	if strings.TrimSpace(optionID) == "" {
		return refusal(execution.RunRefusedUnsupported, req.RunID, fmt.Errorf("no option was chosen"))
	}
	return deliver(arbiter.RespondApproval(ctx, approvalID, optionID))
}

// CancelRun asks the process to stop and reports nothing about the outcome.
//
// It writes no state, on any path. The run is over when the process ends, and
// this package observes that end elsewhere; deriving a terminal state from the
// command would show a closed run the instant the request left — which is
// exactly the lie the acceptance criterion forbids.
func (c *Collaborator) CancelRun(ctx context.Context, req execution.RunRequest) error {
	session, err := c.commandable(req.RunID)
	if err != nil {
		return err
	}
	return deliver(session.dialogueOf().Interrupt(ctx))
}

// deliver classifies what the process answered. A refusal it already expressed
// travels back unchanged, so the caller keeps branching on the reason; anything
// else stays a fault, because nothing was decided.
func deliver(err error) error {
	if err == nil {
		return nil
	}
	var refused *execution.RunCommandError
	if errors.As(err, &refused) {
		return err
	}
	return fmt.Errorf("delivering the command to the local run: %w", err)
}
