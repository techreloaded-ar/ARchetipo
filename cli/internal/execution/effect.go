package execution

import (
	"context"
	"fmt"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
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
	if outcome.Status != StatusSucceeded || action != ActionPlan {
		return nil
	}
	reason := planEffect(ctx, reader, specCode)
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
