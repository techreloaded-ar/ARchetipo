package claude

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

// buildPrompt renders the single instruction given to the local Claude process.
//
// It is a pure, deterministic function of the fields of req: no timestamp, no
// random value, no environment or filesystem lookup. That is what makes the
// invocation reproducible and testable without a machine that has Claude Code
// on it, and it is also what keeps the prompt free of anything that could carry
// local or authentication material into the agent's context.
//
// The wording mirrors the arcipelago and codex prompts on purpose. The three
// providers ask for the same work and accept the same receipt, so a divergence
// in the request would mean one provider asking for something the shared
// acceptance gate does not recognize.
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
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Prepare the review evidence of the spec " + req.SpecCode + " by invoking the ARchetipo review skill in its prepared dossier mode:",
		"",
		"/archetipo-review " + req.SpecCode,
		"",
		"Prepared dossier mode: you gather the evidence, a person decides. You must NOT run `archetipo spec move`, `archetipo spec integrate` or `archetipo spec request-changes`, and you must leave the spec in " + reviewStatus + ".",
		"Persist the evidence with:",
		"",
		"archetipo spec review-dossier " + req.SpecCode + " --file <payload>",
		"",
		`The payload must carry "execution_id": "` + req.ExecutionID + `", a "summary" of the increment, one entry in "criteria" per acceptance criterion with a verdict of "met", "unclear" or "not_verifiable", and one entry in "blockers" per impediment found. Do not paste the dossier into your final message.`,
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + reviewStatus + `","criteria":<N>,"blockers":<M>}`,
		"",
		"<N> is the number of acceptance criteria you examined and <M> the number of blockers you found. Emit the receipt only after the dossier is persisted and the spec is still " + reviewStatus + ".",
	}, "\n")
}

// buildArgs renders the full argument list of the Claude invocation. It is the
// single place where the flags live, so changing how Claude is called never
// means touching Execute.
//
// Every flag here is load-bearing, which is why none of them is configurable.
// The dialogue rests on the streaming protocol: `--input-format stream-json`
// is what lets a message reach the agent while it works, `--output-format
// stream-json` with `--verbose` is what makes its work observable frame by
// frame, and `--replay-user-messages` is what sends an operator's message back
// out so it can enter the history — the shared rule is that a message becomes
// history when the process re-emits it, never when it is sent, and without this
// flag Claude would simply never re-emit it. `--no-session-persistence` keeps a
// managed run from leaving a resumable session on disk. Verified against Claude
// Code 2.1.235.
//
// The prompt is deliberately absent: it travels inside the protocol as the
// first user frame, not as an argument, because a live session is opened before
// it is told what to do.
func buildArgs(cfg settings) []string {
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--replay-user-messages",
		"--no-session-persistence",
		"--permission-mode", cfg.PermissionMode,
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Effort != "" {
		args = append(args, "--effort", cfg.Effort)
	}
	return args
}

// buildInceptionPrompt renders the single instruction that opens an inception
// conversation.
//
// It is pure and deterministic for the same reasons buildPrompt is, and it
// differs from it in exactly the two ways the work differs. It asks for one
// question at a time, because the person answering reads them in a chat and can
// only answer the last one; and it asks for the receipt only once the PRD has
// been persisted through `archetipo prd write`, because the receipt is the only
// thing that ends the conversation and a receipt emitted early would end it on
// a document that does not exist.
//
// The path is asked for, not dictated: where the PRD lives is a fact of the
// workspace configuration, which this package deliberately cannot read. The
// value is informative anyway — the confirmation of the effect happens one
// layer up, against the connector.
func buildInceptionPrompt(_ execution.Request) string {
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Run the product inception for this workspace by invoking the ARchetipo inception skill:",
		"",
		"/archetipo-inception",
		"",
		"You are talking to a person through a chat, one message at a time: ask a single question per message and wait for the answer before asking the next one. Never bundle several questions into one message.",
		"Persist the PRD with `archetipo prd write`, exactly as the skill prescribes. Do not paste the PRD into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"artifact":"prd","status":"` + execution.WrittenStatus + `","path":"<path>"}`,
		"",
		"<path> is the configured PRD path you actually wrote, as reported by `archetipo config show`. Emit the receipt only after the PRD is persisted, and never before: it is what ends the conversation.",
	}, "\n")
}

// buildBacklogPrompt renders the single instruction that opens a backlog
// generation conversation.
//
// It is pure and deterministic for the same reasons buildPrompt and
// buildInceptionPrompt are, and it is a conversation for the same reason the
// inception is: generating a backlog from a PRD raises questions only the
// person who owns the product can answer, so a turn that ends on a question is
// not a failure. It asks for one question per message because the person
// answering reads them in a chat, and it asks for them only when they are
// really needed, because a backlog the skill can derive from the PRD on its own
// is not worth interrupting anyone for.
//
// Nothing here dictates where the backlog is persisted: that is a fact of the
// workspace configuration, which this package deliberately cannot read, and the
// skill already knows to go through `archetipo spec add`. The counts asked for
// in the receipt are informative — confirming that the epics and the specs
// really exist happens one layer up, against the connector.
func buildBacklogPrompt(_ execution.Request) string {
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Generate the initial product backlog for this workspace from its PRD by invoking the ARchetipo spec skill:",
		"",
		"/archetipo-spec",
		"",
		"You are talking to a person through a chat, one message at a time: ask a single question per message and wait for the answer before asking the next one. Never bundle several questions into one message, and ask only when the answer is really necessary to write the backlog.",
		"Persist every epic and every spec with `archetipo spec add`, exactly as the skill prescribes. Do not paste the backlog into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"artifact":"backlog","status":"` + execution.WrittenStatus + `","epics":<N>,"specs":<M>}`,
		"",
		"<N> and <M> are the number of epics and of specs you actually persisted. Emit the receipt only after the backlog is persisted, and never before: it is what ends the conversation.",
	}, "\n")
}

// buildSpecDraftPrompt renders the single instruction that opens an assisted
// spec authoring conversation.
//
// It is pure and deterministic for the same reasons every other prompt here is,
// and it is a conversation for the same reason the inception and the backlog
// generation are: what makes acceptance criteria verifiable is knowing what the
// person actually wants, and that is asked, not guessed.
//
// It is the only prompt of this package that forbids a persistence, and the
// repetition is deliberate — the same reasoning as buildReviewPrompt. The
// failure mode being guarded against is an agent that helpfully finishes the
// job: writing the spec would take a decision that belongs to the person who
// asked for the proposal, and would consume a progressive code that is derived
// from the persisted backlog. The prohibition is stated here, the shared
// receipt refuses any status but PROPOSED, and the confirmation of the effect
// refuses a backlog that grew: three independent guards, because a single one
// the model talks itself past leaves a spec in the backlog nobody confirmed.
//
// Nothing here dictates which epics exist: that is a fact of the workspace the
// agent reads for itself, and a value invented at this layer would be a value
// the backlog does not know.
func buildSpecDraftPrompt(_ execution.Request) string {
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Propose ONE new spec for the backlog of this workspace by invoking the ARchetipo spec skill:",
		"",
		"/archetipo-spec",
		"",
		"Do NOT persist anything. You must not run `archetipo spec add`, must not write into the backlog and must not create any spec file: the spec will be reviewed, edited and created by a person, and writing it yourself would both take that decision for them and consume a spec code.",
		"You are talking to a person through a chat, one message at a time: ask a single question per message and wait for the answer before asking the next one. Never bundle several questions into one message, and ask only what you really need to write acceptance criteria a reviewer can verify.",
		"File the spec under one of the epics the backlog already declares: read them yourself and never invent one.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"artifact":"spec_draft","status":"` + execution.ProposedStatus + `","title":"<title>","epic_code":"<EP-XXX>","priority":"HIGH|MEDIUM|LOW","points":<N>,"scope":"<scope>","blocked_by":[],"body":"<markdown>"}`,
		"",
		"<markdown> is the complete markdown body of the spec on a single line, with every line break written as \\n. Emit the receipt only when the proposal is complete, and never before: it is what ends the conversation.",
	}, "\n")
}
