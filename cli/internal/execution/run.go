package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RunState is the provider-neutral lifecycle of a remote run. A provider
// translates its own vocabulary into these three values; nothing above this
// package ever learns the original spelling.
type RunState string

const (
	RunActive  RunState = "ACTIVE"
	RunClosed  RunState = "CLOSED"
	RunCrashed RunState = "CRASHED"
)

// RunSnapshot is what a caller may say about a run without having followed it:
// which run it is, whether it is still live, and — when it is not — why and
// when it ended.
type RunSnapshot struct {
	RunID    string     `json:"run_id"`
	State    RunState   `json:"state"`
	Error    string     `json:"error,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// RunEvent is one entry of a run's history, translated out of whatever the
// provider emits.
//
// ID is the only admissible cursor. Seq is deliberately not one: a message sent
// by the operator reuses the run's current seq, so two distinct rows can
// legitimately carry the same Seq. A cursor built on Seq would therefore either
// skip a row (when it treats the duplicate as already seen) or repeat one (when
// it does not) — both are visible defects in a timeline. ID is monotonic per
// run, so "everything after ID" is a total, gap-free statement.
type RunEvent struct {
	ID   int64           `json:"id"`
	Seq  int             `json:"seq"`
	At   time.Time       `json:"at"`
	Kind string          `json:"kind"`
	Text string          `json:"text,omitempty"`
	Tool string          `json:"tool,omitempty"`
	Raw  json.RawMessage `json:"raw,omitempty"`
}

// ApprovalOption is one answer a pending approval accepts.
type ApprovalOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// PendingApproval is a decision the remote agent is waiting on. It carries the
// options verbatim: which answers exist is the provider's statement, never a
// local guess.
type PendingApproval struct {
	ID        string           `json:"id"`
	ToolName  string           `json:"tool_name"`
	Title     string           `json:"title"`
	Args      json.RawMessage  `json:"args,omitempty"`
	Options   []ApprovalOption `json:"options"`
	CreatedAt time.Time        `json:"created_at"`
}

// RunRefusalReason names why a run command was refused. It is a closed set on
// purpose: a caller branches on the reason, never on the text of an error.
type RunRefusalReason string

const (
	// RunRefusedNotFound means the run is not visible to this credential. On the
	// external boundary of a hub that is the same answer as "does not exist",
	// which is why it is a refusal and not a fault.
	RunRefusedNotFound RunRefusalReason = "not_found"
	// RunRefusedNotActive means the run has already reached a terminal state.
	RunRefusedNotActive RunRefusalReason = "run_not_active"
	// RunRefusedRunnerOffline means the run is live but nothing is currently
	// attached to execute the command. It is transient, unlike the other four.
	RunRefusedRunnerOffline RunRefusalReason = "runner_offline"
	// RunRefusedUnauthorized means the credential was rejected.
	RunRefusedUnauthorized RunRefusalReason = "unauthorized"
	// RunRefusedUnsupported means the command itself is not admissible — an
	// empty message, an option the run does not offer. It is the caller's
	// mistake, not the remote system's state.
	RunRefusedUnsupported RunRefusalReason = "unsupported"
)

// RunCommandError is a refusal: the remote system understood the command and
// declined it. Every caller must branch on Reason and never on the message,
// which exists only to stay readable in a log.
type RunCommandError struct {
	Reason RunRefusalReason
	RunID  string
	Err    error
}

func (e *RunCommandError) Error() string {
	reason := string(e.Reason)
	if reason == "" {
		reason = "refused"
	}
	message := fmt.Sprintf("remote run %q refused the command (%s)", e.RunID, reason)
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *RunCommandError) Unwrap() error { return e.Err }

// RefusalOf reports the reason a command was refused, or ("", false) when the
// error is not a refusal at all. It unwraps, so a refusal keeps being
// classifiable after it has been decorated with context on its way up.
func RefusalOf(err error) (RunRefusalReason, bool) {
	if err == nil {
		return "", false
	}
	var refusal *RunCommandError
	if !errors.As(err, &refusal) {
		return "", false
	}
	return refusal.Reason, true
}

// RunRequest names the run a call acts on together with the provider
// configuration it needs. The configuration travels per call because the
// workspace default can change while the viewer runs.
type RunRequest struct {
	RunID          string
	ProviderConfig map[string]any
}

// RunCollaborator is the optional capability of observing and commanding a
// remote run. It is deliberately separate from Provider, on the same reasoning
// already written for ConfigDescriber: Provider is a stable contract and adding
// seven methods to it would break every existing implementation for the sake of
// one caller. The cost is that the capability is discovered at runtime and the
// caller must handle a provider that does not collaborate — which is a real
// state, not an artefact, because a provider can legitimately execute work
// without exposing an interactive run.
type RunCollaborator interface {
	// ResolveRun returns the id of the remote run backing an execution record.
	// An empty id with a nil error means the remote work exists but has not been
	// assigned a run yet: absence is an answer, not a failure.
	ResolveRun(ctx context.Context, exec Execution, providerConfig map[string]any) (string, error)
	ReadRun(ctx context.Context, req RunRequest) (RunSnapshot, error)
	ReadRunApprovals(ctx context.Context, req RunRequest) ([]PendingApproval, error)
	// StreamRunEvents consumes the run's event stream from afterID exclusive,
	// handing every event to sink. It returns when the context is done, when the
	// run ends, or when sink returns an error — which it propagates unchanged,
	// so the caller can stop the stream with an error it recognizes.
	StreamRunEvents(ctx context.Context, req RunRequest, afterID int64, sink func(RunEvent) error) error
	SendRunMessage(ctx context.Context, req RunRequest, message string) error
	RespondRunApproval(ctx context.Context, req RunRequest, approvalID, optionID string) error
	CancelRun(ctx context.Context, req RunRequest) error
}

// RunCollaboratorFor discovers the optional capability on a provider. It
// returns (nil, false) for a provider that does not collaborate and for a nil
// provider, so a caller never has to test for nil before asking.
func RunCollaboratorFor(provider Provider) (RunCollaborator, bool) {
	if provider == nil {
		return nil, false
	}
	collaborator, ok := provider.(RunCollaborator)
	if !ok {
		return nil, false
	}
	return collaborator, true
}
