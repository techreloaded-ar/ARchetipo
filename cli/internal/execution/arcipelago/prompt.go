package arcipelago

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

// runnerOpening is the one sentence every task prompt of this provider starts
// with. It says the thing that is genuinely different about work done through
// the hub: the agent stands in the runner's checkout, not in the directory the
// person has open. It lives in one place because the difference between the
// prompts is the work they ask for, never where the work happens.
const runnerOpening = "Work in the runner working directory: it is a checkout of the ARchetipo project with the archetipo CLI and the ARchetipo skills already installed."

// buildTask renders the remote task title, prompt and metadata for one spec
// action. The fork is on the action and nothing else: both actions are
// single-turn work that ends on a receipt, and they differ only in the skill
// they invoke and the receipt they close with.
//
// It must stay a pure, deterministic function of the fields of req: ARcipelago
// does not key external equivalence on (workspaceId, source, externalId) alone,
// it also compares a canonical SHA-256 fingerprint over title, prompt and
// metadata (packages/hub/src/db/tasks-repository.ts:87,445-459). A repetition
// carrying the same identity but a different payload answers
// 409 external_task_conflict instead of 200, so any timestamp, random value,
// local path or environment lookup in here would break idempotency. Key order
// is irrelevant — the hub canonicalizes objects by sorting keys — but every
// value must be byte-identical across two calls with the same Request.
//
// metadata is always a non-nil map: the remote contract requires a JSON object
// (packages/hub/src/api/app.ts:379 via asRecord), and a nil Go map serializes
// as null, which the hub rejects with 400.
func buildTask(req execution.Request) (title, prompt string, metadata map[string]any) {
	if req.Action == execution.ActionImplement {
		title, prompt = buildImplementTask(req)
	} else {
		title, prompt = buildPlanTask(req)
	}
	metadata = map[string]any{
		"spec_code":    req.SpecCode,
		"action":       string(req.Action),
		"capability":   string(req.Capability),
		"execution_id": req.ExecutionID,
	}
	return title, prompt, metadata
}

// buildPlanTask renders the remote task that plans a spec.
func buildPlanTask(req execution.Request) (title, prompt string) {
	return "ARchetipo plan " + req.SpecCode, strings.Join([]string{
		runnerOpening,
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

// buildImplementTask renders the remote task that carries out a persisted plan.
//
// It is the same request the codex and claude providers make, word for word
// past the opening sentence, and that is the point: the three accept the same
// receipt, so a provider asking for something different would produce receipts
// the others refuse.
//
// What it does not say is as deliberate as what it says. It names no status
// transition and no connector command, because the archetipo-implement skill
// already owns both ends of that — it starts by taking the spec to IN PROGRESS
// and it closes with `archetipo spec review`. The one thing this prompt does
// insist on is *when* the receipt may be emitted: only once the spec has really
// reached the review status.
func buildImplementTask(req execution.Request) (title, prompt string) {
	return "ARchetipo implement " + req.SpecCode, strings.Join([]string{
		runnerOpening,
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

// buildConversationTask renders the remote task that carries one conversation:
// its title, its opening prompt and its metadata.
//
// It is pure and deterministic for the same reason buildTask is — the hub keys
// external equivalence on a fingerprint over exactly these three fields — and
// it matters more here, not less: a conversation is opened under an id the
// viewer generated, so a repetition can only ever be a retry of the same open,
// and it must be recognized as one instead of answering 409.
//
// Everything the agent is told about *being* in a conversation is the shared
// declaration in execution.ConversationPrompt. Only the opening sentence is
// this provider's, and it says the one thing that is genuinely different here:
// the agent is standing in the runner's checkout, not in the directory the
// person has open. That is a fact about this deployment and not a limitation to
// hide — a conversation held on the hub talks about the workspace the hub has.
func buildConversationTask(conversationID string, req execution.ConversationRequest) (title, prompt string, metadata map[string]any) {
	title = "ARchetipo conversation " + conversationID
	prompt = execution.ConversationPrompt(conversationOpening, req.ProcessActions, req.Context)
	metadata = map[string]any{
		"kind":            "conversation",
		"conversation_id": conversationID,
	}
	return title, prompt, metadata
}

// conversationOpening says where a conversation held through the hub is
// standing: the runner's working directory, which is a checkout of the project
// and not the folder open in the viewer.
const conversationOpening = "Work in the runner working directory: it is a checkout of the ARchetipo workspace this conversation is about, with the archetipo CLI and the ARchetipo skills already installed."
