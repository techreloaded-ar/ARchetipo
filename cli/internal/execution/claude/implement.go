package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// executeImplement carries out one persisted plan through a local Claude
// session.
//
// It is a conversation, like every other action of this provider: a turn that
// ends without a receipt is the agent needing something only a person can give,
// and the run stays open for the answer. What ends it is the receipt, a turn
// the agent itself closed with an error, the death of the process, or the
// timeout.
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

	subject := specSubject{gerund: "implementing", outcome: "an implementation of " + req.SpecCode}
	held, err := p.runSpecConversation(runCtx, req, cfg, dir, buildImplementPrompt(req), subject, func(message string) bool {
		_, err := execution.AcceptImplementReceipt(message, req.SpecCode)
		return err == nil
	})
	if err != nil {
		return execution.Result{}, err
	}

	// The acceptance rule is the shared one, so a receipt this provider takes
	// and codex refuses cannot exist. It is read from the message a turn ends
	// on, which is where the prompt asks for it, and never from the stream.
	receipt, err := execution.AcceptImplementReceipt(held.final, req.SpecCode)
	if err != nil {
		held.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without implementing %s", req.SpecCode))
		return execution.Result{}, fmt.Errorf(
			"the claude command %q ended without having implemented %s%s: %w",
			cfg.Command, req.SpecCode, diagnosticSuffix(held.stderr), err,
		)
	}
	held.session.Close(execution.RunClosed, "")
	return p.resultForImplement(cfg, held.exitCode, held.elapsed, receipt)
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
		return execution.Result{}, fmt.Errorf("encoding the claude implementation receipt: %w", err)
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
		return execution.Result{}, fmt.Errorf("encoding the claude execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}
