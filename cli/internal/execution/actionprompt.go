package execution

import "strings"

// PlanPrompt renders the common planning request after a provider-specific opening.
func PlanPrompt(opening string, req Request) string {
	return strings.Join([]string{
		opening,
		"Plan the spec " + req.SpecCode + " by invoking the ARchetipo planning skill:",
		"",
		"/archetipo-plan " + req.SpecCode,
		"",
		"Persist the plan through the configured connector, exactly as the skill prescribes. Do not paste the plan into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
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
		"Close your run with a single JSON receipt line and nothing after it:",
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
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + ReviewStatus + `","criteria":<N>,"blockers":<M>}`,
		"",
		"<N> is the number of acceptance criteria you examined and <M> the number of blockers you found. Emit the receipt only after the dossier is persisted and the spec is still " + ReviewStatus + ".",
	}, "\n")
}
