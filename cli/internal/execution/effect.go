package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// SpecStateReader is the minimal view of the backlog that effect confirmation
// needs: re-read the spec, re-read its plan tasks. It is declared here, as an
// interface of the consumer, so this package never imports connector — the
// execution boundary deliberately does not know about the backlog backend, and
// a real connector satisfies this shape without any adapter.
type SpecStateReader interface {
	ReadSpecDetail(context.Context, string) (domain.Spec, error)
	ReadSpecTasks(context.Context, string) ([]domain.Task, error)
}

// PRDStateReader is the minimal view of the workspace that confirming an
// inception needs: re-read the PRD. It is declared here, as an interface of the
// consumer and for the same reason as SpecStateReader, so this package never
// imports connector; a connector exposing the PRDReader capability satisfies it
// without any adapter.
type PRDStateReader interface {
	ReadPRD(ctx context.Context) (string, error)
}

// PRDDiscarder is the workspace's own undo, mirrored here as a consumer
// interface: it removes the PRD and reports whether one was there. A connector
// exposing the PRDDiscarder capability satisfies it as is.
type PRDDiscarder interface {
	DiscardPRD(ctx context.Context) (bool, error)
}

// BacklogStateReader is the minimal view of the workspace that confirming a
// backlog generation needs: re-read the backlog. It is declared here, as an
// interface of the consumer and for the same reason as SpecStateReader, so this
// package never imports connector. ReadExistingBacklog belongs to the base
// connector interface, so every real connector satisfies this shape without any
// adapter and without an optional capability.
type BacklogStateReader interface {
	ReadExistingBacklog(ctx context.Context) (domain.BacklogSummary, error)
}

// BacklogDiscarder is the workspace's own undo for the backlog, twin of
// PRDDiscarder: it removes the generated specs and their index and reports
// whether there was anything to remove. A connector exposing the
// BacklogDiscarder capability satisfies it as is.
type BacklogDiscarder interface {
	DiscardBacklog(ctx context.Context) (bool, error)
}

// UnconfirmedEffectError reports an execution that declared success while the
// connector does not back the claim. It is a typed error rather than a rendered
// envelope because each caller renders it with its own remedy: the CLI suggests
// a new --request-id, the viewer suggests pressing the action again.
type UnconfirmedEffectError struct {
	ExecutionID string
	Message     string
}

func (e *UnconfirmedEffectError) Error() string {
	return "execution " + e.ExecutionID + ": " + e.Message
}

// ConfirmActionEffect turns a self-declared success into a verified one.
//
// The provider only ever sees a receipt written by the remote agent: it cannot
// read the connector, and the execution-provider boundary deliberately keeps it
// that way, so that a remote failure can never move a spec. But a receipt is a
// declaration. A remote skill that fails halfway, or an agent that hallucinates
// its own closure, produces a well-formed receipt with the spec still TODO and
// no plan persisted anywhere.
//
// The check cannot live in the provider, which by design holds no connector,
// and it cannot be duplicated in the web layer, which would let the viewer and
// the CLI drift into disagreeing about what a success is. It lives here, over a
// reader the caller supplies, so both callers apply the very same rule.
//
// A claim the state denies is not a success: the record is rewritten as FAILED
// with the reason, and the caller receives an *UnconfirmedEffectError.
//
// This is the synchronous form, for a caller that already holds a closed record
// and updates it in place. A caller whose record can be read by someone else
// while it works must verify before closing it instead — see VerifyActionEffect.
func ConfirmActionEffect(ctx context.Context, reader SpecStateReader, store Store, action ActionID, specCode string, outcome *Execution) error {
	verdict := VerifyActionEffect(ctx, reader, action, specCode, outcome)
	if verdict == nil {
		return nil
	}
	if err := store.Update(context.WithoutCancel(ctx), *outcome); err != nil {
		return fmt.Errorf("recording the unconfirmed execution %s: %w", outcome.ID, err)
	}
	return verdict
}

// VerifyActionEffect is ConfirmActionEffect's verdict without its write: it
// applies exactly the same rule and performs exactly the same demotion on
// outcome, but persists nothing and leaves the write to the caller.
//
// It exists because *when* the demotion reaches the store is not a detail. The
// viewer dispatches on a goroutine while a browser polls the record every two
// seconds, so a record closed as SUCCEEDED and demoted a moment later is a
// record a client can read — and settle on — in between: the UI stops polling
// at the first terminal status it sees, and would keep showing a success next
// to a spec that never moved. Verifying before the terminal write closes that
// window entirely, because there is only ever one terminal write.
//
// It returns nil when the claim holds (or when there is nothing to verify), and
// an *UnconfirmedEffectError once it has rewritten outcome as FAILED.
func VerifyActionEffect(ctx context.Context, reader SpecStateReader, action ActionID, specCode string, outcome *Execution) error {
	if outcome.Status != StatusSucceeded {
		return nil
	}
	var reason error
	switch action {
	case ActionPlan:
		reason = planEffect(ctx, reader, specCode)
	case ActionInception:
		// The parameter keeps SpecStateReader as its static type so the two
		// callers pass the same connector for every action; what an inception
		// needs from it is the PRD, asked for at the only point that knows the
		// action. A reader that cannot answer is not a success it can back.
		prd, ok := reader.(PRDStateReader)
		if !ok {
			reason = fmt.Errorf("the connector cannot read the PRD back")
		} else {
			reason = inceptionEffect(ctx, prd)
		}
	case ActionBacklog:
		// Same narrowing as the inception above, for the same reason: the
		// static type stays SpecStateReader so both callers keep passing one
		// connector for every action, and what a backlog generation needs from
		// it is asked for at the only point that knows the action.
		backlog, ok := reader.(BacklogStateReader)
		if !ok {
			reason = fmt.Errorf("the connector cannot read the backlog back")
		} else {
			reason = backlogEffect(ctx, backlog)
		}
	default:
		return nil
	}
	if reason == nil {
		return nil
	}
	message := fmt.Sprintf(
		"the execution reported success but the connector does not confirm it: %v",
		reason,
	)
	remoteID := ""
	if outcome.Result != nil {
		remoteID = outcome.Result.ExternalID
	}
	// Result and Error are mutually exclusive on a record, so the external
	// identifier moves into the error rather than being lost with the result.
	outcome.Status = StatusFailed
	outcome.Result = nil
	outcome.Error = &ExecutionError{Code: "UNCONFIRMED_EFFECT", Message: message, ExternalID: remoteID}
	return &UnconfirmedEffectError{ExecutionID: outcome.ID, Message: message}
}

// planEffect reports why the connector does not back a claimed plan, or nil when
// it does. Both halves matter: a spec that is PLANNED with an empty task list is
// as unusable as one still sitting in TODO.
func planEffect(ctx context.Context, reader SpecStateReader, specCode string) error {
	spec, err := reader.ReadSpecDetail(ctx, specCode)
	if err != nil {
		return fmt.Errorf("re-reading %s failed: %w", specCode, err)
	}
	if spec.Status != domain.StatusPlanned {
		return fmt.Errorf("%s is %s, not %s", specCode, spec.Status, domain.StatusPlanned)
	}
	tasks, err := reader.ReadSpecTasks(ctx, specCode)
	if err != nil {
		return fmt.Errorf("reading the plan tasks of %s failed: %w", specCode, err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("%s is %s but holds no plan task", specCode, domain.StatusPlanned)
	}
	return nil
}

// inceptionEffect reports why the connector does not back a claimed inception,
// or nil when it does. The whole condition is "a non-empty PRD can be read back
// from the configured path": the receipt is the agent's word, this is the
// workspace's. Structural validation of the document is deliberately out of
// scope here — `archetipo validate prd` and the skill own that — because this
// boundary only answers whether the run produced a document at all.
func inceptionEffect(ctx context.Context, reader PRDStateReader) error {
	body, err := reader.ReadPRD(ctx)
	if err != nil {
		return fmt.Errorf("re-reading the PRD failed: %w", err)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("no PRD was persisted at the configured path")
	}
	return nil
}

// backlogEffect reports why the connector does not back a claimed backlog
// generation, or nil when it does. The whole condition is "a backlog with at
// least one spec and at least one epic can be read back": the receipt is the
// agent's word, this is the workspace's. Structural validation of the generated
// specs is deliberately out of scope here — the skill and `archetipo validate`
// own that — because this boundary only answers whether the run produced a
// backlog at all.
//
// A connector that answers "there is no backlog here" does so as a missing
// precondition rather than as an empty summary, and that is an answer, not an
// infrastructure failure: it is exactly the "nothing was persisted" verdict.
func backlogEffect(ctx context.Context, reader BacklogStateReader) error {
	summary, err := reader.ReadExistingBacklog(ctx)
	if err != nil {
		var coded *iox.CodedError
		if errors.As(err, &coded) && coded.Code == iox.CodePreconditionMissing {
			return fmt.Errorf("no backlog was persisted")
		}
		return fmt.Errorf("re-reading the backlog failed: %w", err)
	}
	if len(summary.Codes) == 0 {
		return fmt.Errorf("no backlog was persisted")
	}
	if len(summary.Epics) == 0 {
		return fmt.Errorf("the backlog holds %d spec(s) but no epic", len(summary.Codes))
	}
	return nil
}

// DiscardPartialPRD takes back a PRD that was born inside a run that ended
// badly, so a first inception either lands whole or leaves no trace (AC-4).
//
// The rollback is deliberately narrow, and both halves of the condition are
// load-bearing:
//   - existedBefore captures, before the run starts, whether the workspace
//     already had a PRD. A pre-existing document belongs to the workspace, not
//     to this run, and is never removed whatever the outcome — which is also
//     half of the "nothing is implicitly overwritten" guarantee (AC-5).
//   - only a FAILED outcome rolls back. A succeeded (and, above, confirmed)
//     execution is precisely the one whose document must stay.
//
// A discarder that fails does not hide why the run failed: the note is appended
// to the existing message, never substituted for it. The caller is expected to
// pass nil when the connector exposes no discarder — skipping the rollback is
// not itself a failure.
func DiscardPartialPRD(ctx context.Context, discarder PRDDiscarder, existedBefore bool, outcome *Execution) {
	var discard func(context.Context) (bool, error)
	if discarder != nil {
		discard = discarder.DiscardPRD
	}
	discardPartial(
		ctx, discard,
		"the partial PRD written by this run has been removed",
		"the partial PRD could not be removed",
		existedBefore, outcome,
	)
}

// DiscardPartialBacklog is DiscardPartialPRD's twin for the backlog: a backlog
// born inside a run that ended badly is taken back, so a first generation
// either lands whole or leaves no trace (AC-4). It applies the very same two
// load-bearing conditions — the workspace had no backlog before the run, and
// the outcome is FAILED — because they are enforced by the same helper and
// therefore cannot drift apart from the PRD rule.
//
// The caller is expected to pass nil when the connector exposes no backlog
// discarder: skipping the rollback is a valid answer, not a failure.
func DiscardPartialBacklog(ctx context.Context, discarder BacklogDiscarder, existedBefore bool, outcome *Execution) {
	var discard func(context.Context) (bool, error)
	if discarder != nil {
		discard = discarder.DiscardBacklog
	}
	discardPartial(
		ctx, discard,
		"the partial backlog written by this run has been removed",
		"the partial backlog could not be removed",
		existedBefore, outcome,
	)
}

// discardPartial holds the rollback rule once, so the PRD and the backlog can
// never enforce it differently: an artifact that predates the run is never
// touched, only a FAILED outcome rolls back, a missing discarder is tolerated,
// and every note is appended to the failure message instead of replacing it.
func discardPartial(
	ctx context.Context,
	discard func(context.Context) (bool, error),
	removedNote, failedNote string,
	existedBefore bool,
	outcome *Execution,
) {
	if discard == nil || existedBefore || outcome == nil || outcome.Status != StatusFailed {
		return
	}
	removed, err := discard(ctx)
	if err != nil {
		appendErrorNote(outcome, fmt.Sprintf("%s: %v", failedNote, err))
		return
	}
	if removed {
		appendErrorNote(outcome, removedNote)
	}
}

// appendErrorNote adds a sentence to a failed record's message without ever
// losing what was already there: the original reason is the diagnosis, the note
// is only what was done about it.
func appendErrorNote(outcome *Execution, note string) {
	if outcome.Error == nil {
		// Defensive: a FAILED record normally already carries its reason. If it
		// does not, the note is still worth recording rather than dropping.
		outcome.Error = &ExecutionError{Code: "PARTIAL_PRD_DISCARDED", Message: note}
		return
	}
	if strings.TrimSpace(outcome.Error.Message) == "" {
		outcome.Error.Message = note
		return
	}
	outcome.Error.Message = outcome.Error.Message + "; " + note
}
