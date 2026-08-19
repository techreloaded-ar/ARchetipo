package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// The three ways an inception conversation can end without a PRD. They are
// sentinels rather than messages because the message is composed one level up,
// where the elapsed time, the exit code and the process's standard error are
// known — and because the caller has to be able to tell them apart: a turn the
// agent closed with an error, a process that left, and a run that was stopped
// are three different things to fix.
var (
	errTurnFailed    = errors.New("the turn ended with an error")
	errProcessGone   = errors.New("the process ended")
	errRunTerminated = errors.New("the run was stopped")
)

// executeInception runs one inception as a conversation.
//
// The difference with planning is one rule: a turn that ends without a receipt
// is not a failure here, it is the agent asking a question and waiting for the
// answer the operator will send through the run's dialogue. So the run does not
// end with the turn — it ends on the receipt, on a turn the agent itself closed
// with an error, on the death of the process, or on the timeout. Nothing else
// closes it, which is exactly what makes a multi-turn conversation possible.
//
// The receipt is a necessary condition and never a sufficient one: it is what
// the agent declares, not what the filesystem holds. Confirming that a PRD
// really exists at the configured path happens one layer up, where the
// connector is.
func (p *Provider) executeInception(ctx context.Context, req execution.Request, cfg settings, dir string) (execution.Result, error) {
	if err := ensureSkill(dir, inceptionSkillRelPath, "inception"); err != nil {
		return execution.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	live, err := p.openSession(runCtx, req, cfg, dir, buildInceptionPrompt(req), true)
	if err != nil {
		return execution.Result{}, err
	}

	receipt, turns, convErr := converse(runCtx, live.client)

	exitCode, stderr, waitErr := p.shutdown(live.process)
	elapsed := p.now().Sub(live.startedAt)

	if convErr != nil {
		return execution.Result{}, p.failInception(live, cfg, runCtx.Err(), convErr, exitCode, stderr, elapsed)
	}
	if waitErr != nil {
		live.session.Close(execution.RunCrashed, waitErr.Error())
		return execution.Result{}, fmt.Errorf("the claude command %q could not be run to completion after %s: %w", cfg.Command, elapsed.Round(time.Millisecond), waitErr)
	}
	live.session.Close(execution.RunClosed, "")
	return p.resultForInception(cfg, exitCode, elapsed, turns, receipt)
}

// converse follows the conversation turn by turn until it can be decided, and
// reports the receipt it ended on together with how many turns it took.
//
// The three channels are the three ways the conversation can end, and they are
// read in one select because they are genuinely concurrent: the process can
// die in the middle of a turn and the timeout can fire while the operator is
// typing.
func converse(runCtx context.Context, client *streamSession) (execution.PRDReceipt, int, error) {
	turns := 0
	for {
		select {
		case outcome := <-client.Turns():
			turns++
			// The receipt is looked for before the outcome of the turn is
			// judged, because an agent that emitted it has done the work
			// whatever the process did next.
			if receipt, err := execution.AcceptPRDReceipt(outcome.Final); err == nil {
				return receipt, turns, nil
			}
			if !outcome.Completed {
				return execution.PRDReceipt{}, turns, errTurnFailed
			}
			// A completed turn with no receipt is the agent's question. The
			// conversation stays open: the answer will arrive through the run's
			// dialogue and re-arm the next turn.
		case <-client.Gone():
			// The output of the process has ended, but a turn published just
			// before it left is still worth reading: the buffered outcomes are
			// drained first, so a run that ended on its receipt is a success no
			// matter how quickly the process exited afterwards.
			receipt, drained, found := drainTurns(client)
			turns += drained
			if found {
				return receipt, turns, nil
			}
			return execution.PRDReceipt{}, turns, errProcessGone
		case <-runCtx.Done():
			// The deadline is drained exactly like the death of the process, and
			// for the same reason. A select picks at random among the cases that
			// are ready, so a receipt published in the very instant the deadline
			// fires would otherwise be thrown away — and the run would be closed
			// FAILED, with the partial-PRD cleanup then deleting a document that
			// was complete. A receipt that was published was earned, whatever
			// happened to the clock immediately afterwards.
			receipt, drained, found := drainTurns(client)
			turns += drained
			if found {
				return receipt, turns, nil
			}
			return execution.PRDReceipt{}, turns, errRunTerminated
		}
	}
}

// drainTurns reads every outcome already published and reports the first
// receipt among them, together with how many turns it counted.
//
// It never waits: what is drained is what the process has already said, and a
// conversation that is ending — because the process left or because the
// deadline fired — has nothing more to say.
func drainTurns(client *streamSession) (execution.PRDReceipt, int, bool) {
	turns := 0
	for {
		select {
		case outcome := <-client.Turns():
			turns++
			if receipt, err := execution.AcceptPRDReceipt(outcome.Final); err == nil {
				return receipt, turns, true
			}
		default:
			return execution.PRDReceipt{}, turns, false
		}
	}
}

// failInception closes the run and composes the diagnostic for a conversation
// that ended without a PRD. Every branch names the inception, never the
// planning, and keeps the four causes apart: the deadline, an operator's
// cancellation, a turn the agent closed with an error, and a process that left.
func (p *Provider) failInception(live *liveSession, cfg settings, runErr, convErr error, exitCode int, stderr string, elapsed time.Duration) error {
	rounded := elapsed.Round(time.Millisecond)
	switch {
	case errors.Is(convErr, errRunTerminated) && errors.Is(runErr, context.DeadlineExceeded):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude inception was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q did not finish the inception within %s", cfg.Command, cfg.Timeout)
	case errors.Is(convErr, errRunTerminated):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude inception was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q was stopped after %s without having produced a PRD: %v", cfg.Command, rounded, runErr)
	case errors.Is(convErr, errTurnFailed):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude inception ended on a failed turn after %s", rounded))
		return fmt.Errorf(
			"the claude command %q ended the inception on a turn that did not complete after %s, without having produced a PRD%s",
			cfg.Command, rounded, diagnosticSuffix(stderr),
		)
	default:
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude process exited %d without having produced a PRD", exitCode))
		return fmt.Errorf(
			"the claude command %q exited %d after %s without having produced a PRD: the inception ended without a receipt%s",
			cfg.Command, exitCode, rounded, diagnosticSuffix(stderr),
		)
	}
}

// resultForInception builds the execution payload of an inception. Like the
// planning one it carries no stream: the agent's stdout and stderr never enter
// the record, and result_summary is the receipt alone, re-rendered from the
// parsed value rather than sliced out of the output.
//
// turns is the one field planning has no use for, and it is the cheapest
// honest measure of a conversation: how many times the agent finished speaking
// before it was done.
func (p *Provider) resultForInception(cfg settings, exitCode int, elapsed time.Duration, turns int, receipt execution.PRDReceipt) (execution.Result, error) {
	summary, err := json.Marshal(receipt)
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude prd receipt: %w", err)
	}
	payload, err := json.Marshal(struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		PRDPath       string `json:"prd_path"`
		Turns         int    `json:"turns"`
		DurationMS    int64  `json:"duration_ms"`
	}{
		Command:       cfg.Command,
		Model:         cfg.Model,
		ExitCode:      exitCode,
		ResultSummary: string(summary),
		PRDPath:       receipt.Path,
		Turns:         turns,
		DurationMS:    elapsed.Milliseconds(),
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}
