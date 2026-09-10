package codex

import "github.com/techreloaded-ar/ARchetipo/cli/internal/execution"

const localOpening = "Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed."

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
	return execution.PlanPrompt(localOpening, req)
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
	return execution.ImplementPrompt(localOpening, req)
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

// buildReviewPrompt renders the single instruction that prepares the review
// evidence of a spec already waiting under review.
//
// It is pure and deterministic for the same reasons buildPrompt is, and it is a
// single turn for the same reason: the work ends on a receipt.
//
// Unlike every other prompt, this one states what must *not* happen, and names
// the three commands by hand. That repetition is deliberate. Elsewhere the
// prompt stays silent about transitions because the skill owns them; here the
// whole point of the action is that no transition may occur, and the failure
// mode being guarded against is an agent that helpfully finishes the job. The
// skill's prepared-dossier mode says the same thing, the shared receipt refuses
// any status but REVIEW, and the confirmation of the effect refuses a spec that
// moved: three independent guards, because a single one that the model talks
// itself past leaves a spec closed without anybody deciding.
//
// The execution id travels in the prompt because this is the only point where
// the provider knows the identity of the run. Carried into the dossier, it is
// what later lets a human verdict name the execution that prepared the evidence
// it was decided on.
func buildReviewPrompt(req execution.Request) string {
	return execution.ReviewPrompt(localOpening, req)
}
