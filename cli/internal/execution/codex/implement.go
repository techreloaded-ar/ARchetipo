package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// executeImplement carries out one persisted plan through a local Codex
// session.
//
// It is a single turn, exactly like planning: the agent is not talking to
// anybody, so a turn that ends without a receipt has failed rather than asked a
// question, and that must be said now instead of waiting for an answer nobody
// is going to send.
//
// The provider still never touches the connector. Taking the spec to review is
// the skill's own last step, and confirming that it really got there — with its
// plan carried out — happens one layer up, in execution.VerifyActionEffect,
// where the connector is held. The receipt accepted here is a declaration of
// the agent: it rules out a session that simply ended, never one that lied.
func (p *Provider) executeImplement(ctx context.Context, req execution.Request, cfg settings, dir string) (execution.Result, error) {
	if err := ensureSkill(dir, implementSkillRelPath, "implementation"); err != nil {
		return execution.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	turn, err := p.runSingleTurn(runCtx, req, cfg, dir, buildImplementPrompt(req), "implementing")
	if err != nil {
		return execution.Result{}, err
	}

	// The acceptance rule is the shared one, so a receipt this provider takes
	// and claude refuses cannot exist. It is read from the message the run ends
	// on, which is where the prompt asks for it, and never from the stream.
	receipt, err := execution.AcceptImplementReceipt(turn.final, req.SpecCode)
	if err != nil {
		turn.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without implementing %s", req.SpecCode))
		return execution.Result{}, fmt.Errorf(
			"the codex command %q ended without having implemented %s%s: %w",
			cfg.Command, req.SpecCode, localrun.DiagnosticSuffix(turn.stderr), err,
		)
	}
	turn.session.Close(execution.RunClosed, "")
	return p.resultForImplement(cfg, turn.exitCode, turn.elapsed, receipt)
}

// resultForImplement builds the execution payload of an implementation. Like
// the planning one it carries no stream — the agent's stdout and stderr never
// enter the record — and result_summary is the receipt alone, re-rendered from
// the parsed value rather than sliced out of the output.
//
// tasks_done and tests are the two fields planning has no use for, and they are
// what a reviewer reads instead of opening the run: how much of the plan the
// agent says it carried out, and how the final suite went. They are the agent's
// account and are stored as such; the authority on what really happened is the
// connector, one layer up.
func (p *Provider) resultForImplement(cfg settings, exitCode int, elapsed time.Duration, receipt execution.ImplementReceipt) (execution.Result, error) {
	summary, err := json.Marshal(receipt)
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the codex implementation receipt: %w", err)
	}
	payload, err := json.Marshal(struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		TasksDone     int    `json:"tasks_done"`
		Tests         string `json:"tests"`
		DurationMS    int64  `json:"duration_ms"`
	}{
		Command:       cfg.Command,
		Model:         cfg.Model,
		ExitCode:      exitCode,
		ResultSummary: string(summary),
		TasksDone:     receipt.TasksDone,
		Tests:         receipt.Tests,
		DurationMS:    elapsed.Milliseconds(),
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the codex execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}
