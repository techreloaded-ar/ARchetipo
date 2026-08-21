package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// executeBacklog runs one backlog generation as a conversation.
//
// It is the inception's semantics applied to another artifact, and it is
// written as its own function for the same reason the inception is: what
// changes between the two is the skill, the prompt, the receipt and every word
// of every diagnostic, so folding them into one function parameterized on all
// four would hide the differences rather than share the rule. What is really
// shared — the conversation loop — is shared, in converse.
//
// The rule that makes it a conversation is the same one: a turn that ends
// without a receipt is not a failure, it is the agent asking a question about
// the product and waiting for the answer the operator will send through the
// run's dialogue. The run ends on the receipt, on a turn the agent itself
// closed with an error, on the death of the process, or on the timeout.
//
// The receipt is a necessary condition and never a sufficient one: it says how
// many epics and specs the agent claims it persisted, not what the connector
// holds. Confirming that the backlog really exists happens one layer up, where
// the connector is.
func (p *Provider) executeBacklog(ctx context.Context, req execution.Request, cfg settings, dir string) (execution.Result, error) {
	if err := ensureSkill(dir, backlogSkillRelPath, "spec"); err != nil {
		return execution.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	live, err := p.openSession(runCtx, req, cfg, dir, buildBacklogPrompt(req), true, 0)
	if err != nil {
		return execution.Result{}, err
	}

	final, turns, convErr := converse(runCtx, live.client, func(message string) bool {
		_, err := execution.AcceptBacklogReceipt(message)
		return err == nil
	})

	exitCode, stderr, waitErr := p.shutdown(live.process)
	elapsed := p.now().Sub(live.startedAt)

	if convErr != nil {
		return execution.Result{}, p.failBacklog(live, cfg, runCtx.Err(), convErr, exitCode, stderr, elapsed)
	}
	if waitErr != nil {
		live.session.Close(execution.RunCrashed, waitErr.Error())
		return execution.Result{}, fmt.Errorf("the claude command %q could not be run to completion after %s: %w", cfg.Command, elapsed.Round(time.Millisecond), waitErr)
	}
	// The typed receipt is parsed back here, for the reason documented in
	// executeInception: converse recognizes an acceptable closing message, and
	// which artifact that message describes is the caller's knowledge.
	receipt, err := execution.AcceptBacklogReceipt(final)
	if err != nil {
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without a backlog: %v", err))
		return execution.Result{}, fmt.Errorf("the claude command %q ended without having produced a backlog%s: %w", cfg.Command, diagnosticSuffix(stderr), err)
	}
	live.session.Close(execution.RunClosed, "")
	return p.resultForBacklog(cfg, exitCode, elapsed, turns, receipt)
}

// failBacklog closes the run and composes the diagnostic for a conversation
// that ended without a backlog. Every branch names the backlog — never the PRD
// and never the planning — and keeps the four causes apart: the deadline, an
// operator's cancellation, a turn the agent closed with an error, and a process
// that left.
func (p *Provider) failBacklog(live *liveSession, cfg settings, runErr, convErr error, exitCode int, stderr string, elapsed time.Duration) error {
	rounded := elapsed.Round(time.Millisecond)
	switch {
	case errors.Is(convErr, errRunTerminated) && errors.Is(runErr, context.DeadlineExceeded):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude backlog generation was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q did not finish the backlog within %s", cfg.Command, cfg.Timeout)
	case errors.Is(convErr, errRunTerminated):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude backlog generation was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q was stopped after %s without having produced a backlog: %v", cfg.Command, rounded, runErr)
	case errors.Is(convErr, errTurnFailed):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude backlog generation ended on a failed turn after %s", rounded))
		return fmt.Errorf(
			"the claude command %q ended the backlog generation on a turn that did not complete after %s, without having produced a backlog%s",
			cfg.Command, rounded, diagnosticSuffix(stderr),
		)
	default:
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude process exited %d without having produced a backlog", exitCode))
		return fmt.Errorf(
			"the claude command %q exited %d after %s without having produced a backlog: the backlog generation ended without a receipt%s",
			cfg.Command, exitCode, rounded, diagnosticSuffix(stderr),
		)
	}
}

// resultForBacklog builds the execution payload of a backlog generation. Like
// the planning and inception ones it carries no stream: the agent's stdout and
// stderr never enter the record, and result_summary is the receipt alone,
// re-rendered from the parsed value rather than sliced out of the output.
//
// The two counts are reported because they are the cheapest useful thing to
// show a person about a backlog that was just written — and they stay what the
// agent declared, never an inspection of the connector.
func (p *Provider) resultForBacklog(cfg settings, exitCode int, elapsed time.Duration, turns int, receipt execution.BacklogReceipt) (execution.Result, error) {
	summary, err := json.Marshal(receipt)
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude backlog receipt: %w", err)
	}
	payload, err := json.Marshal(struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		BacklogEpics  int    `json:"backlog_epics"`
		BacklogSpecs  int    `json:"backlog_specs"`
		Turns         int    `json:"turns"`
		DurationMS    int64  `json:"duration_ms"`
	}{
		Command:       cfg.Command,
		Model:         cfg.Model,
		ExitCode:      exitCode,
		ResultSummary: string(summary),
		BacklogEpics:  receipt.Epics,
		BacklogSpecs:  receipt.Specs,
		Turns:         turns,
		DurationMS:    elapsed.Milliseconds(),
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}
