package localrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// ActionOpener is the protocol boundary. Each adapter starts its own runtime;
// the shared runner owns receipt acceptance and the batch action lifecycle.
type ActionOpener interface {
	OpenAction(context.Context, execution.Request, string) (*ActionRun, error)
}

type ActionRun struct {
	Source    TurnSource
	Process   Process
	Session   *Session
	StartedAt time.Time
}

type ActionOptions struct {
	ProviderID    string
	Command       string
	Model         string
	SkillsDir     string
	Opening       string
	Timeout       time.Duration
	ShutdownGrace time.Duration
	Now           func() time.Time
}

type actionSubject struct {
	skill        string
	skillLabel   string
	label        string
	outcome      string
	timeoutLabel string
	work         string
}

func subjectFor(req execution.Request) (actionSubject, error) {
	switch req.Action {
	case execution.ActionPlan:
		return actionSubject{"archetipo-plan", "planning", "the planning", "having produced a plan for " + req.SpecCode, "planning " + req.SpecCode, "planning " + req.SpecCode}, nil
	case execution.ActionImplement:
		return actionSubject{"archetipo-implement", "implementation", "the implementation", "having implemented " + req.SpecCode, "implementing " + req.SpecCode, "implementing " + req.SpecCode}, nil
	case execution.ActionReview:
		return actionSubject{"archetipo-review", "review", "the review", "having prepared the review of " + req.SpecCode, "preparing the review of " + req.SpecCode, "preparing the review of " + req.SpecCode}, nil
	case execution.ActionInception:
		return actionSubject{"archetipo-inception", "inception", "the inception", "having produced a PRD", "the inception", "the inception"}, nil
	case execution.ActionBacklog:
		return actionSubject{"archetipo-spec", "spec", "the backlog generation", "having produced a backlog", "the backlog", "the backlog generation"}, nil
	case execution.ActionWikiCreate, execution.ActionWikiRefresh, execution.ActionWikiLint, execution.ActionWikiQuery:
		return actionSubject{"archetipo-wiki", "Wiki", "the Wiki operation", "having completed the Wiki operation", "the Wiki operation", "the Wiki operation"}, nil
	case execution.ActionSpecDraft:
		return actionSubject{"archetipo-spec", "spec", "the spec proposal", "having proposed a spec", "the spec proposal", "the conversation"}, nil
	default:
		return actionSubject{}, &execution.ActionError{Action: req.Action}
	}
}

// ExecuteAction runs any local workflow action through the same contract. This
// is the batch path: its timeout and ephemeral process never affect a native
// conversation, which is owned separately by SessionProvider.
func ExecuteAction(ctx context.Context, req execution.Request, opts ActionOptions, opener ActionOpener) (execution.Result, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	subject, err := subjectFor(req)
	if err != nil {
		return execution.Result{}, err
	}
	rel := filepath.Join(opts.SkillsDir, subject.skill, "SKILL.md")
	if _, err := os.Stat(filepath.Join(req.WorkingDir, rel)); err != nil {
		return execution.Result{}, fmt.Errorf("the ARchetipo %s skill is not installed for %s in %s (expected %s): run `archetipo init --tool %s` in that directory", subject.skillLabel, opts.ProviderID, req.WorkingDir, filepath.ToSlash(rel), opts.ProviderID)
	}
	prompt, err := execution.ActionPrompt(opts.Opening, req.Action, req)
	if err != nil {
		return execution.Result{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	live, err := opener.OpenAction(runCtx, req, prompt)
	if err != nil {
		return execution.Result{}, err
	}
	final, turns, cause := Converse(runCtx, live.Source, func(message string) bool {
		_, found, _ := execution.AcceptActionReceipt(req.Action, req.SpecCode, message)
		return found
	})
	exitCode, stderr, waitErr := Shutdown(live.Process, opts.ProviderID, opts.ShutdownGrace)
	elapsed := opts.Now().Sub(live.StartedAt)
	scope, _ := execution.ActionScope(req.Action)
	// Preserve the useful diagnostic of a completed spec turn that closed its
	// process without a valid receipt, instead of reducing it to process exit.
	if errors.Is(cause, ErrProcessGone) && (live.Source.Completed() || exitCode == 0) && scope == execution.ScopeSpec {
		final, cause = live.Source.FinalMessage(), nil
	}
	if cause != nil {
		err := actionFailure(opts, subject, scope, runCtx.Err(), cause, exitCode, stderr, elapsed)
		reason := err.Error()
		if runCtx.Err() != nil {
			reason = fmt.Sprintf("the %s session was stopped after %s: %v", opts.ProviderID, elapsed.Round(time.Millisecond), runCtx.Err())
		}
		live.Session.Close(execution.RunCrashed, reason)
		return execution.Result{}, err
	}
	if waitErr != nil {
		live.Session.Close(execution.RunCrashed, waitErr.Error())
		return execution.Result{}, fmt.Errorf("the %s command %q could not be run to completion after %s: %w", opts.ProviderID, opts.Command, elapsed.Round(time.Millisecond), waitErr)
	}
	receipt, _, err := execution.AcceptActionReceipt(req.Action, req.SpecCode, final)
	if err != nil {
		live.Session.Close(execution.RunCrashed, err.Error())
		return execution.Result{}, fmt.Errorf("the %s command %q ended without %s%s: %w", opts.ProviderID, opts.Command, subject.outcome, DiagnosticSuffix(stderr), err)
	}
	fields, err := execution.ActionResultFields(req.Action, receipt)
	if err != nil {
		live.Session.Close(execution.RunCrashed, err.Error())
		return execution.Result{}, err
	}
	fields["command"], fields["model"], fields["exit_code"] = opts.Command, opts.Model, exitCode
	fields["result_summary"], fields["duration_ms"] = string(receipt), elapsed.Milliseconds()
	if scope == execution.ScopeWorkspace {
		fields["turns"] = turns
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		live.Session.Close(execution.RunCrashed, err.Error())
		return execution.Result{}, err
	}
	live.Session.Close(execution.RunClosed, "")
	return execution.Result{Payload: payload}, nil
}

func actionFailure(opts ActionOptions, subject actionSubject, scope execution.Scope, runErr, cause error, code int, stderr string, elapsed time.Duration) error {
	rounded := elapsed.Round(time.Millisecond)
	switch {
	case errors.Is(cause, ErrRunTerminated) && errors.Is(runErr, context.DeadlineExceeded):
		return fmt.Errorf("the %s command %q did not finish %s within %s", opts.ProviderID, opts.Command, subject.timeoutLabel, opts.Timeout)
	case errors.Is(cause, ErrRunTerminated):
		return fmt.Errorf("the %s command %q was stopped after %s without %s: %w", opts.ProviderID, opts.Command, rounded, subject.outcome, runErr)
	case scope == execution.ScopeSpec:
		return fmt.Errorf("the %s command %q exited %d after %s without %s: the turn never completed%s", opts.ProviderID, opts.Command, code, rounded, subject.work, DiagnosticSuffix(stderr))
	case errors.Is(cause, ErrTurnFailed):
		return fmt.Errorf("the %s command %q ended %s on a turn that did not complete after %s, without %s%s", opts.ProviderID, opts.Command, subject.label, rounded, subject.outcome, DiagnosticSuffix(stderr))
	default:
		return fmt.Errorf("the %s command %q exited %d after %s without %s: %s ended without a receipt%s", opts.ProviderID, opts.Command, code, rounded, subject.outcome, subject.work, DiagnosticSuffix(stderr))
	}
}
