package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// executeSpecDraft runs one assisted spec authoring as a conversation.
//
// It is the backlog generation's shape applied to a different outcome, and it
// is written as its own function for the same reason that one is: what changes
// between them is the prompt, the receipt and every word of every diagnostic,
// so folding them into one function parameterized on all three would hide the
// differences rather than share the rule. What is really shared — the
// conversation loop — is shared, in converse.
//
// The rule that makes it a conversation is the same one: a turn that ends
// without a receipt is not a failure, it is the agent asking about the story
// and waiting for the answer the operator will send through the run's dialogue.
// The run ends on the receipt, on a turn the agent itself closed with an error,
// on the death of the process, or on the timeout.
//
// What it does *not* end on is a spec appearing in the backlog, and that is the
// difference that matters. This run is asked to write nothing at all: the
// receipt carries the proposal back so a person can review, edit and confirm
// it. Whether the run really left the backlog alone is established one layer
// up, where the connector is, by comparing it with the snapshot taken before
// the run started.
func (p *Provider) executeSpecDraft(ctx context.Context, req execution.Request, cfg settings, dir string) (execution.Result, error) {
	if err := ensureSkill(dir, backlogSkillRelPath, "spec"); err != nil {
		return execution.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	live, err := p.openSession(runCtx, req, cfg, dir, buildSpecDraftPrompt(req), true, false, 0)
	if err != nil {
		return execution.Result{}, err
	}

	final, turns, convErr := converse(runCtx, live.client, func(message string) bool {
		_, err := execution.AcceptSpecDraftReceipt(message)
		return err == nil
	})

	exitCode, stderr, waitErr := p.shutdown(live.process)
	elapsed := p.now().Sub(live.startedAt)

	if convErr != nil {
		return execution.Result{}, p.failSpecDraft(live, cfg, runCtx.Err(), convErr, exitCode, stderr, elapsed)
	}
	if waitErr != nil {
		live.session.Close(execution.RunCrashed, waitErr.Error())
		return execution.Result{}, fmt.Errorf("the claude command %q could not be run to completion after %s: %w", cfg.Command, elapsed.Round(time.Millisecond), waitErr)
	}
	// The typed receipt is parsed back here, for the reason documented in
	// executeInception: converse recognizes an acceptable closing message, and
	// which artifact that message describes is the caller's knowledge.
	receipt, err := execution.AcceptSpecDraftReceipt(final)
	if err != nil {
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without a proposed spec: %v", err))
		return execution.Result{}, fmt.Errorf("the claude command %q ended without having proposed a spec%s: %w", cfg.Command, localrun.DiagnosticSuffix(stderr), err)
	}
	live.session.Close(execution.RunClosed, "")
	return p.resultForSpecDraft(cfg, exitCode, elapsed, turns, receipt)
}

// failSpecDraft closes the run and composes the diagnostic for a conversation
// that ended without a proposal. Every branch names the proposed spec — never
// the backlog, never the PRD and never the planning — and keeps the four causes
// apart: the deadline, an operator's cancellation, a turn the agent closed with
// an error, and a process that left.
func (p *Provider) failSpecDraft(live *liveSession, cfg settings, runErr, convErr error, exitCode int, stderr string, elapsed time.Duration) error {
	rounded := elapsed.Round(time.Millisecond)
	switch {
	case errors.Is(convErr, errRunTerminated) && errors.Is(runErr, context.DeadlineExceeded):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude spec proposal was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q did not finish the spec proposal within %s", cfg.Command, cfg.Timeout)
	case errors.Is(convErr, errRunTerminated):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude spec proposal was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q was stopped after %s without having proposed a spec: %v", cfg.Command, rounded, runErr)
	case errors.Is(convErr, errTurnFailed):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude spec proposal ended on a failed turn after %s", rounded))
		return fmt.Errorf(
			"the claude command %q ended the spec proposal on a turn that did not complete after %s, without having proposed a spec%s",
			cfg.Command, rounded, localrun.DiagnosticSuffix(stderr),
		)
	default:
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude process exited %d without having proposed a spec", exitCode))
		return fmt.Errorf(
			"the claude command %q exited %d after %s without having proposed a spec: the conversation ended without a receipt%s",
			cfg.Command, exitCode, rounded, localrun.DiagnosticSuffix(stderr),
		)
	}
}

// specDraftPayload is the proposal as it travels to whoever will review it. It
// is the shape of the creation form and nothing else: a client that receives it
// can fill every field of that form without knowing anything about receipts.
type specDraftPayload struct {
	Title     string   `json:"title"`
	EpicCode  string   `json:"epic_code"`
	Priority  string   `json:"priority"`
	Points    int      `json:"points"`
	Scope     string   `json:"scope"`
	BlockedBy []string `json:"blocked_by"`
	Body      string   `json:"body"`
}

// resultForSpecDraft builds the execution payload of a spec proposal. Like the
// planning, inception and backlog ones it carries no stream: the agent's stdout
// and stderr never enter the record, and result_summary is the receipt alone,
// re-rendered from the parsed value rather than sliced out of the output.
//
// The spec_draft field is what makes this action useful at all. Every other
// action leaves its outcome in the workspace and the record only points at it;
// here the outcome *is* the record, because the proposal is deliberately
// nowhere else until a person confirms it.
func (p *Provider) resultForSpecDraft(cfg settings, exitCode int, elapsed time.Duration, turns int, receipt execution.SpecDraftReceipt) (execution.Result, error) {
	summary, err := json.Marshal(receipt)
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude spec draft receipt: %w", err)
	}
	blocked := receipt.BlockedBy
	if blocked == nil {
		blocked = []string{}
	}
	payload, err := json.Marshal(struct {
		Command       string           `json:"command"`
		Model         string           `json:"model"`
		ExitCode      int              `json:"exit_code"`
		ResultSummary string           `json:"result_summary"`
		SpecDraft     specDraftPayload `json:"spec_draft"`
		Turns         int              `json:"turns"`
		DurationMS    int64            `json:"duration_ms"`
	}{
		Command:       cfg.Command,
		Model:         cfg.Model,
		ExitCode:      exitCode,
		ResultSummary: string(summary),
		SpecDraft: specDraftPayload{
			Title:     receipt.Title,
			EpicCode:  receipt.EpicCode,
			Priority:  receipt.Priority,
			Points:    receipt.Points,
			Scope:     receipt.Scope,
			BlockedBy: blocked,
			Body:      receipt.Body,
		},
		Turns:      turns,
		DurationMS: elapsed.Milliseconds(),
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}
