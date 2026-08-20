package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// executeReview has a local Claude session prepare the review evidence of a
// spec that is already waiting under review.
//
// It is a single turn, like planning and implementation: the agent is reading
// an increment, not talking to anybody, so a turn that ends without a receipt
// has failed rather than asked a question.
//
// What makes this action different from every other one is what it must *not*
// do. The provider prepares evidence; it never decides. The prompt says so
// explicitly and the shared receipt refuses anything but the review status, but
// neither is the real guarantee: confirming that the spec really stayed where
// it was happens one layer up, in execution.VerifyActionEffect, where the
// connector is held. The provider still never touches the connector.
func (p *Provider) executeReview(ctx context.Context, req execution.Request, cfg settings, dir string) (execution.Result, error) {
	if err := ensureSkill(dir, reviewSkillRelPath, "review"); err != nil {
		return execution.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	turn, err := p.runSingleTurn(runCtx, req, cfg, dir, buildReviewPrompt(req), "preparing the review of")
	if err != nil {
		return execution.Result{}, err
	}

	// The acceptance rule is the shared one, so a receipt this provider takes
	// and codex refuses cannot exist. It is read from the message the run ends
	// on, which is where the prompt asks for it, and never from the stream.
	receipt, err := execution.AcceptReviewReceipt(turn.final, req.SpecCode)
	if err != nil {
		turn.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without preparing the review of %s", req.SpecCode))
		return execution.Result{}, fmt.Errorf(
			"the claude command %q ended without having prepared the review of %s%s: %w",
			cfg.Command, req.SpecCode, diagnosticSuffix(turn.stderr), err,
		)
	}
	turn.session.Close(execution.RunClosed, "")
	return p.resultForReview(cfg, turn.exitCode, turn.elapsed, receipt)
}

// resultForReview builds the execution payload of a prepared review. Like the
// other payloads it carries no stream — the agent's stdout and stderr never
// enter the record — and result_summary is the receipt alone, re-rendered from
// the parsed value rather than sliced out of the output.
//
// criteria and blockers are what a person reads before opening anything: how
// many acceptance criteria the agent examined, and how many impediments it
// found. They are the agent's own account, deliberately kept as a summary: the
// evidence itself lives in the dossier attached to the spec, which is where a
// reviewer decides from.
func (p *Provider) resultForReview(cfg settings, exitCode int, elapsed time.Duration, receipt execution.ReviewReceipt) (execution.Result, error) {
	summary, err := json.Marshal(receipt)
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude review receipt: %w", err)
	}
	payload, err := json.Marshal(struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		Criteria      int    `json:"criteria"`
		Blockers      int    `json:"blockers"`
		DurationMS    int64  `json:"duration_ms"`
	}{
		Command:       cfg.Command,
		Model:         cfg.Model,
		ExitCode:      exitCode,
		ResultSummary: string(summary),
		Criteria:      receipt.Criteria,
		Blockers:      receipt.Blockers,
		DurationMS:    elapsed.Milliseconds(),
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the claude execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}
