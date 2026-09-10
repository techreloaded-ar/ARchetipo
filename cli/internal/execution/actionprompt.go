package execution

import (
	"fmt"
	"strings"
)

// PlanPrompt renders the common planning request after a provider-specific opening.
func PlanPrompt(opening string, req Request) string {
	return strings.Join([]string{
		opening,
		"Plan the spec " + req.SpecCode + " by invoking the ARchetipo planning skill:",
		"",
		"/archetipo-plan " + req.SpecCode,
		"",
		"Persist the plan through the configured connector, exactly as the skill prescribes. Do not paste the plan into your final message.",
		"Close the action with a single JSON receipt line, as the last line of that message and with nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + PlannedStatus + `","tasks":<N>}`,
		"",
		"<N> is the number of tasks of the plan you actually persisted. Emit the receipt only after the plan is persisted and the spec is " + PlannedStatus + ".",
	}, "\n")
}

// ImplementPrompt renders the common implementation request.
func ImplementPrompt(opening string, req Request) string {
	return strings.Join([]string{
		opening,
		"Implement the spec " + req.SpecCode + " by invoking the ARchetipo implementation skill:",
		"",
		"/archetipo-implement " + req.SpecCode,
		"",
		"Carry out the persisted plan to the end — every task of it — and run the tests the plan requires. Do not paste code, diffs or file contents into your final message.",
		"Close the action with a single JSON receipt line, as the last line of that message and with nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + ReviewStatus + `","tasks_done":<N>,"tests":"<summary>"}`,
		"",
		"<N> is the number of tasks you actually completed and <summary> is one line on the outcome of the final test suite. Emit the receipt only after the spec is " + ReviewStatus + ", and never before.",
	}, "\n")
}

// ReviewPrompt renders the common prepared-dossier review request.
func ReviewPrompt(opening string, req Request) string {
	return strings.Join([]string{
		opening,
		"Prepare the review evidence of the spec " + req.SpecCode + " by invoking the ARchetipo review skill in its prepared dossier mode:",
		"",
		"/archetipo-review " + req.SpecCode,
		"",
		"Prepared dossier mode: you gather the evidence, a person decides. You must NOT run `archetipo spec move`, `archetipo spec integrate` or `archetipo spec request-changes`, and you must leave the spec in " + ReviewStatus + ".",
		"Persist the evidence with:",
		"",
		"archetipo spec review-dossier " + req.SpecCode + " --file <payload>",
		"",
		`The payload must carry "execution_id": "` + req.ExecutionID + `", a "summary" of the increment, one entry in "criteria" per acceptance criterion with a verdict of "met", "unclear" or "not_verifiable", and one entry in "blockers" per impediment found. Do not paste the dossier into your final message.`,
		"Close the action with a single JSON receipt line, as the last line of that message and with nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + ReviewStatus + `","criteria":<N>,"blockers":<M>}`,
		"",
		"<N> is the number of acceptance criteria you examined and <M> the number of blockers you found. Emit the receipt only after the dossier is persisted and the spec is still " + ReviewStatus + ".",
	}, "\n")
}

// InceptionPrompt renders the common product inception request.
//
// It differs from the three spec-scoped prompts in exactly the two ways the
// work differs. It asks for one question at a time, because the person
// answering reads them in a chat and can only answer the last one; and it asks
// for the receipt only once the PRD has been persisted through `archetipo prd
// write`, because a receipt emitted early would declare a document that does
// not exist.
//
// The path is asked for, not dictated: where the PRD lives is a fact of the
// workspace configuration, which this package deliberately cannot read. The
// value is informative anyway — the confirmation of the effect happens one
// layer up, against the connector.
func InceptionPrompt(opening string, _ Request) string {
	return strings.Join([]string{
		opening,
		"Run the product inception for this workspace by invoking the ARchetipo inception skill:",
		"",
		"/archetipo-inception",
		"",
		"You are talking to a person through a chat, one message at a time: ask a single question per message and wait for the answer before asking the next one. Never bundle several questions into one message.",
		"Persist the PRD with `archetipo prd write`, exactly as the skill prescribes. Do not paste the PRD into your final message.",
		"Close the action with a single JSON receipt line, as the last line of that message and with nothing after it:",
		"",
		`{"artifact":"prd","status":"` + WrittenStatus + `","path":"<path>"}`,
		"",
		"<path> is the configured PRD path you actually wrote, as reported by `archetipo config show`. Emit the receipt only after the PRD is persisted, and never before.",
	}, "\n")
}

// BacklogPrompt renders the common initial backlog generation request.
//
// It is a conversation for the same reason the inception is: generating a
// backlog from a PRD raises questions only the person who owns the product can
// answer, so a turn that ends on a question is not a failure. It asks for one
// question per message because the person answering reads them in a chat, and
// only when they are really needed, because a backlog the skill can derive from
// the PRD on its own is not worth interrupting anyone for.
//
// Nothing here dictates where the backlog is persisted: that is a fact of the
// workspace configuration, which this package deliberately cannot read, and the
// skill already knows to go through `archetipo spec add`. The counts asked for
// in the receipt are informative — confirming that the epics and the specs
// really exist happens one layer up, against the connector.
func BacklogPrompt(opening string, _ Request) string {
	return strings.Join([]string{
		opening,
		"Generate the initial product backlog for this workspace from its PRD by invoking the ARchetipo spec skill:",
		"",
		"/archetipo-spec",
		"",
		"You are talking to a person through a chat, one message at a time: ask a single question per message and wait for the answer before asking the next one. Never bundle several questions into one message, and ask only when the answer is really necessary to write the backlog.",
		"Persist every epic and every spec with `archetipo spec add`, exactly as the skill prescribes. Do not paste the backlog into your final message.",
		"Close the action with a single JSON receipt line, as the last line of that message and with nothing after it:",
		"",
		`{"artifact":"backlog","status":"` + WrittenStatus + `","epics":<N>,"specs":<M>}`,
		"",
		"<N> and <M> are the number of epics and of specs you actually persisted. Emit the receipt only after the backlog is persisted, and never before.",
	}, "\n")
}

// SpecDraftPrompt renders the common assisted spec authoring request.
//
// It is the only shared prompt that forbids a persistence, and the repetition
// is deliberate. The failure mode being guarded against is an agent that
// helpfully finishes the job: writing the spec would take a decision that
// belongs to the person who asked for the proposal, and would consume a
// progressive code derived from the persisted backlog. The prohibition is
// stated here, the shared receipt refuses any status but PROPOSED, and the
// confirmation of the effect refuses a backlog that grew: three independent
// guards, because a single one the model talks itself past leaves a spec in the
// backlog nobody confirmed.
//
// Nothing here dictates which epics exist: that is a fact of the workspace the
// agent reads for itself, and a value invented at this layer would be a value
// the backlog does not know.
func SpecDraftPrompt(opening string, _ Request) string {
	return strings.Join([]string{
		opening,
		"Propose ONE new spec for the backlog of this workspace by invoking the ARchetipo spec skill:",
		"",
		"/archetipo-spec",
		"",
		"Do NOT persist anything. You must not run `archetipo spec add`, must not write into the backlog and must not create any spec file: the spec will be reviewed, edited and created by a person, and writing it yourself would both take that decision for them and consume a spec code.",
		"You are talking to a person through a chat, one message at a time: ask a single question per message and wait for the answer before asking the next one. Never bundle several questions into one message, and ask only what you really need to write acceptance criteria a reviewer can verify.",
		"File the spec under one of the epics the backlog already declares: read them yourself and never invent one.",
		"Close the action with a single JSON receipt line, as the last line of that message and with nothing after it:",
		"",
		`{"artifact":"spec_draft","status":"` + ProposedStatus + `","title":"<title>","epic_code":"<EP-XXX>","priority":"HIGH|MEDIUM|LOW","points":<N>,"scope":"<scope>","blocked_by":[],"body":"<markdown>"}`,
		"",
		"<markdown> is the complete markdown body of the spec on a single line, with every line break written as \\n. Emit the receipt only when the proposal is complete, and never before.",
	}, "\n")
}

// ActionPrompt renders the request of one action, whichever action it is.
//
// It exists because an action carried out inside a native session is chosen at
// run time: the caller holds an ActionID and a session, not a compiled-in call
// to one of the six prompts above. A switch in the viewer would be a second
// table of which prompt belongs to which action, free to drift from this one.
//
// An action this table does not name is an error rather than an empty prompt: a
// turn started with no instruction would look like work being done and would
// produce nothing.
func ActionPrompt(opening string, action ActionID, req Request) (string, error) {
	switch action {
	case ActionPlan:
		return PlanPrompt(opening, req), nil
	case ActionImplement:
		return ImplementPrompt(opening, req), nil
	case ActionReview:
		return ReviewPrompt(opening, req), nil
	case ActionInception:
		return InceptionPrompt(opening, req), nil
	case ActionBacklog:
		return BacklogPrompt(opening, req), nil
	case ActionSpecDraft:
		return SpecDraftPrompt(opening, req), nil
	}
	return "", fmt.Errorf("no prompt is defined for the %q action", action)
}
