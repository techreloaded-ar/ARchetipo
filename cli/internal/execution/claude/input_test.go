package claude

import (
	"context"
	"encoding/json"
	"testing"
)

// askUserQuestionFrame renders the permission request Claude Code writes for an
// AskUserQuestion call: the same `can_use_tool` envelope as any other tool, with
// the questions as its input.
func askUserQuestionFrame(requestID string) string {
	payload, err := json.Marshal(map[string]any{
		"type":       frameControlRequest,
		"request_id": requestID,
		"request": map[string]any{
			"subtype":      subtypeCanUseTool,
			"tool_name":    askUserQuestionTool,
			"display_name": askUserQuestionTool,
			"input": map[string]any{"questions": []map[string]any{
				{"question": "Quale formato vuoi?", "header": "Formato", "multiSelect": false, "options": []map[string]any{
					{"label": "Sintesi", "description": "Breve"}, {"label": "Dettaglio", "description": "Completo"},
				}},
				{"question": "Quali sezioni includo?", "header": "Sezioni", "multiSelect": true, "options": []map[string]any{
					{"label": "Introduzione", "description": ""}, {"label": "Conclusione", "description": ""},
				}},
			}},
			"tool_use_id": "toolu_" + requestID,
		},
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// A question for the person is not a permission: it is offered as a pending
// input with its questions and options, and never as an approval card whose
// arguments would be the question itself.
func TestAskUserQuestionBecomesAPendingInputWithItsQuestions(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(askUserQuestionFrame("req-q"))
	waitFor(t, func() bool { return len(client.PendingInputs()) == 1 })

	if approvals := client.PendingApprovals(); len(approvals) != 0 {
		t.Fatalf("the question was also offered as an approval: %#v", approvals)
	}
	input := client.PendingInputs()[0]
	if input.ID != "req-q" || len(input.Questions) != 2 {
		t.Fatalf("pending input = %#v, want req-q with two questions", input)
	}
	first := input.Questions[0]
	if first.ID != "Quale formato vuoi?" || first.Header != "Formato" || !first.AllowText || first.MultiSelect || len(first.Options) != 2 || first.Options[1].Label != "Dettaglio" {
		t.Fatalf("first question projected as %#v", first)
	}
	if second := input.Questions[1]; !second.MultiSelect || !second.AllowText {
		t.Fatalf("the multi-select question lost its flag: %#v", second)
	}
}

// A single-select question takes one value: two of them are not merged into
// an answer the person never gave, and the question stays open.
func TestTwoValuesOnASingleSelectQuestionAreRefused(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	fake.emit(askUserQuestionFrame("req-q"))
	waitFor(t, func() bool { return len(client.PendingInputs()) == 1 })

	err := client.RespondInput(context.Background(), "req-q", map[string][]string{
		"Quale formato vuoi?":    {"Sintesi", "Dettaglio"},
		"Quali sezioni includo?": {"Introduzione"},
	})
	if err == nil {
		t.Fatal("two values on a single-select question were accepted")
	}
	if len(decisionsWritten(fake)) != 0 || len(client.PendingInputs()) != 1 {
		t.Fatalf("a refused answer reached the process or closed the question")
	}
}

// The answers reach the process inside the allow decision, keyed by the text
// of each question, with the original questions handed back unchanged and a
// multi-select answer joined with a comma; the input then stops being pending.
func TestAnsweringAQuestionWritesTheAnswersIntoTheAllowDecision(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	fake.emit(askUserQuestionFrame("req-q"))
	waitFor(t, func() bool { return len(client.PendingInputs()) == 1 })

	err := client.RespondInput(context.Background(), "req-q", map[string][]string{
		"Quale formato vuoi?":    {"Sintesi"},
		"Quali sezioni includo?": {"Introduzione", "Conclusione"},
	})
	if err != nil {
		t.Fatalf("answering failed: %v", err)
	}
	decisions := decisionsWritten(fake)
	if len(decisions) != 1 || decisions[0]["request_id"] != "req-q" || decisions[0]["behavior"] != "allow" {
		t.Fatalf("decisions written = %#v, want one allow for req-q", decisions)
	}
	updated, _ := decisions[0]["updatedInput"].(map[string]any)
	answers, _ := updated["answers"].(map[string]any)
	if answers["Quale formato vuoi?"] != "Sintesi" || answers["Quali sezioni includo?"] != "Introduzione, Conclusione" {
		t.Fatalf("answers = %#v", answers)
	}
	if questions, _ := updated["questions"].([]any); len(questions) != 2 {
		t.Fatalf("the original questions were not handed back: %#v", updated)
	}
	if pending := client.PendingInputs(); len(pending) != 0 {
		t.Fatalf("the input is still pending after being answered: %#v", pending)
	}
}

// An incomplete answer is refused and leaves the question open.
func TestAPartialAnswerIsRefusedAndLeavesTheInputOpen(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	fake.emit(askUserQuestionFrame("req-q"))
	waitFor(t, func() bool { return len(client.PendingInputs()) == 1 })

	if err := client.RespondInput(context.Background(), "req-q", map[string][]string{"Quale formato vuoi?": {"Sintesi"}}); err == nil {
		t.Fatal("a partial answer was accepted")
	}
	if len(decisionsWritten(fake)) != 0 || len(client.PendingInputs()) != 1 {
		t.Fatalf("a refused answer reached the process or closed the question")
	}
}
