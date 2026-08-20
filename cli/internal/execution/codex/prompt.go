package codex

import (
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// plannedStatus is the spec status the receipt must declare. It is an alias of
// the shared execution constant so that the prompt and the acceptance gate can
// never ask for one word and accept another.
const plannedStatus = execution.PlannedStatus

// reviewStatus is the same fact for the implementation receipt: the status the
// prompt asks for and the status the shared gate accepts are one constant, so
// they cannot drift into asking for one word and accepting another.
const reviewStatus = execution.ReviewStatus

// buildPrompt renders the single instruction given to the local Codex process.
//
// It is a pure, deterministic function of the fields of req: no timestamp, no
// random value, no environment or filesystem lookup. That is what makes the
// invocation reproducible and testable without a machine that has Codex on it,
// and it is also what keeps the prompt free of anything that could carry local
// or authentication material into the agent's context.
//
// The wording mirrors the arcipelago prompt on purpose. The two providers ask
// for the same work and accept the same receipt, so a divergence in the request
// would mean one provider asking for something the shared acceptance gate does
// not recognize.
func buildPrompt(req execution.Request) string {
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Plan the spec " + req.SpecCode + " by invoking the ARchetipo planning skill:",
		"",
		"/archetipo-plan " + req.SpecCode,
		"",
		"Persist the plan through the configured connector, exactly as the skill prescribes. Do not paste the plan into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + plannedStatus + `","tasks":<N>}`,
		"",
		"<N> is the number of tasks of the plan you actually persisted. Emit the receipt only after the plan is persisted and the spec is " + plannedStatus + ".",
	}, "\n")
}

// buildImplementPrompt renders the single instruction that carries out a
// persisted plan.
//
// It is pure and deterministic for the same reasons buildPrompt is, and it is a
// single turn for the same reason: the work ends on a receipt, so a turn that
// ends without one has failed rather than asked a question.
//
// It is the same request the claude provider makes, word for word, and that is
// the point: the two providers accept the same receipt, so a provider asking
// for something different would produce receipts the other one refuses.
//
// What it does not say is as deliberate as what it says. It names no status
// transition and no connector command, because the archetipo-implement skill
// already owns both ends of that — it starts by taking the spec to IN PROGRESS
// and it closes with `archetipo spec review`. Restating them here would be a
// second copy of the workflow, free to drift from the skill that actually runs
// it. The one thing this prompt does insist on is *when* the receipt may be
// emitted: only once the spec has really reached the review status, because a
// receipt emitted before that would declare an implementation that the
// confirmation of the effect will then refuse.
func buildImplementPrompt(req execution.Request) string {
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Implement the spec " + req.SpecCode + " by invoking the ARchetipo implementation skill:",
		"",
		"/archetipo-implement " + req.SpecCode,
		"",
		"Carry out the persisted plan to the end — every task of it — and run the tests the plan requires. Do not paste code, diffs or file contents into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + reviewStatus + `","tasks_done":<N>,"tests":"<summary>"}`,
		"",
		"<N> is the number of tasks you actually completed and <summary> is one line on the outcome of the final test suite. Emit the receipt only after the spec is " + reviewStatus + ", and never before.",
	}, "\n")
}

// buildArgs renders the full argument list of the Codex invocation. It is the
// single place where the invocation lives, so changing how Codex is started
// never means touching Execute.
//
// The session runs through `codex app-server --listen stdio://`: a JSON-RPC
// server on standard input and output, which is the only surface of codex-cli
// 0.147.0 that keeps a conversation alive — `codex exec` runs to completion and
// reads nothing after its prompt, and `codex proto` no longer exists. Every
// tunable of the session (working directory, sandbox, model, approvals) travels
// inside the protocol rather than on the command line, which is why this
// function takes no settings.
func buildArgs() []string {
	return []string{"app-server", "--listen", "stdio://"}
}
