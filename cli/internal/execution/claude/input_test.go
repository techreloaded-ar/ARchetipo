package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// prefaceTextFrame is the message a person can read before a structured
// question: without one the question is sent back (see
// TestStructuredQuestionWithoutAVisibleMessageIsSentBackOnce).
const prefaceTextFrame = `{"type":"assistant","message":{"content":[{"type":"text","text":"Costanza propone di risparmiare tempo, Livia di ridurre gli errori."}]}}`

const discoveryInputFrame = `{"type":"control_request","request_id":"discovery","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"header":"Costanza","question":"Quale risultato conta?","multiSelect":false,"options":[{"label":"Risparmiare tempo","description":"Ridurre il lavoro manuale","preview":"![Variante A](/abs/mockups/1/a.png)"},{"label":"Meno errori","description":"Aumentare la precisione"}]},{"header":"Livia","question":"Per chi?","multiSelect":true,"options":[{"label":"Clienti"},{"label":"Colleghi"}]}],"metadata":{"keep":"original"}}}}`

func TestDiscoveryQuestionsRoundTripWithoutBecomingApprovals(t *testing.T) {
	fake := newFakeClaude()
	client, session := openConversation(t, fake)
	fake.emit(prefaceTextFrame)
	fake.emit(discoveryInputFrame)
	waitFor(t, func() bool { return len(client.pendingInputs()) == 1 })
	input := client.pendingInputs()[0]
	if len(client.PendingApprovals()) != 0 || input.Questions[0].Header != "Costanza" || !input.Questions[0].AllowText || !input.Questions[1].Multiple || input.Questions[0].Options[0].Description != "Ridurre il lavoro manuale" || input.Questions[0].Options[0].Preview != "![Variante A](/abs/mockups/1/a.png)" {
		t.Fatalf("discovery input = %#v", input)
	}
	if err := client.RespondApproval(context.Background(), "discovery", "allow"); err == nil || len(client.pendingInputs()) != 1 {
		t.Fatal("a permission response consumed a product question")
	}
	// Invalid answers keep the question pending for a corrected submission.
	for _, answers := range []map[string][]string{
		{"0": {"Risparmiare tempo"}},
		{"unknown": {"x"}, "1": {"Clienti"}},
		{"0": {" "}, "1": {"Clienti"}},
		{"0": {"Risparmiare tempo", "Meno errori"}, "1": {"Clienti"}},
	} {
		if err := client.respondInput(context.Background(), "discovery", answers); err == nil || len(client.pendingInputs()) != 1 {
			t.Fatal("invalid answer accepted or input lost")
		}
	}
	answers := map[string][]string{"0": {"Un risultato diverso"}, "1": {"Clienti", "Colleghi"}}
	if err := client.respondInput(context.Background(), "discovery", answers); err != nil {
		t.Fatal(err)
	}
	decisions := decisionsWritten(fake)
	if len(decisions) != 1 || decisions[0]["behavior"] != "allow" || len(client.pendingInputs()) != 0 {
		t.Fatalf("response = %#v", decisions)
	}
	updated := decisions[0]["updatedInput"].(map[string]any)
	if updated["answers"].(map[string]any)["Quale risultato conta?"] != "Un risultato diverso" || updated["answers"].(map[string]any)["Per chi?"] != "Clienti, Colleghi" || updated["metadata"].(map[string]any)["keep"] != "original" || len(updated["questions"].([]any)) != 2 {
		t.Fatalf("native response lost original context or choices: %#v", updated)
	}
	if err := client.respondInput(context.Background(), "discovery", answers); err == nil {
		t.Fatal("same question answered twice")
	}
	// The answer enters the history as the person's words, so the timeline
	// shows what was chosen and a free answer typed in the composer finds its
	// own echo.
	events := session.Events(0)
	last := events[len(events)-1]
	if last.Kind != "user_message" || last.Text != "Un risultato diverso\nClienti, Colleghi" {
		t.Fatalf("answer did not enter the history as the person's words: %#v", last)
	}
}

func TestDiscoveryQuestionWithdrawAndUnknownPayload(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	for _, payload := range []string{`{}`, `{"questions":[{"question":""}]}`, `{"questions":[{"question":"Same?"},{"question":"Same?"}]}`} {
		client.askInput("unknown", json.RawMessage(payload))
		if len(client.pendingInputs()) != 0 {
			t.Fatal("invented questions for an unknown payload")
		}
	}
	fake.emit(prefaceTextFrame)
	fake.emit(discoveryInputFrame)
	waitFor(t, func() bool { return len(client.pendingInputs()) == 1 })
	fake.emit(`{"type":"control_cancel_request","request_id":"discovery"}`)
	waitFor(t, func() bool { return len(client.pendingInputs()) == 0 })
	fake.emit(discoveryInputFrame)
	waitFor(t, func() bool { return len(client.pendingInputs()) == 1 })
	client.withdrawAll()
	if len(client.pendingInputs()) != 0 {
		t.Fatal("question survived process cleanup")
	}
}

// The guidance asks for a readable message before every structured question;
// the agent keeps the explanation in its reasoning instead, which the person
// never sees, and the question lands bare. So a question that nothing visible
// preceded since the person's last message is sent back with the reason, and
// never enters the history: the one the agent asks after writing is the one the
// person reads, and the row stays above the answer once the card is gone.
func TestStructuredQuestionWithoutAVisibleMessageIsSentBackOnce(t *testing.T) {
	fake := newFakeClaude()
	client, session := openConversation(t, fake)
	before := lastEventID(session)
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Andrea e Costanza hanno preferenze diverse."}]}}`)
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_q1","name":"AskUserQuestion","input":{"questions":[{"question":"Quale risultato conta?","header":"Costanza","options":[{"label":"Risparmiare tempo"},{"label":"Meno errori"}]}]}}]}}`)
	fake.emit(discoveryInputFrame)
	waitFor(t, func() bool { return len(decisionsWritten(fake)) == 1 })
	refusal := decisionsWritten(fake)[0]
	if refusal["request_id"] != "discovery" || refusal["behavior"] != "deny" || refusal["message"] != execution.QuestionPrefaceMessage {
		t.Fatalf("refusal = %#v", refusal)
	}
	if len(client.pendingInputs()) != 0 {
		t.Fatal("a question sent back is still pending")
	}
	fake.emit(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_q1","content":"Permission denied","is_error":true}]}}`)
	fake.emit(prefaceTextFrame)
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_q2","name":"AskUserQuestion","input":{"questions":[{"question":"Quale risultato conta?","header":"Costanza","options":[{"label":"Risparmiare tempo"},{"label":"Meno errori"}]}]}}]}}`)
	fake.emit(`{"type":"control_request","request_id":"discovery-2","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"question":"Quale risultato conta?","header":"Costanza","options":[{"label":"Risparmiare tempo"},{"label":"Meno errori"}]}]}}}`)
	waitFor(t, func() bool { return len(client.pendingInputs()) == 1 })
	if len(decisionsWritten(fake)) != 1 {
		t.Fatalf("the second question was answered by this layer: %#v", decisionsWritten(fake))
	}
	questions := 0
	for _, event := range session.Events(before) {
		if event.Kind == "question" {
			questions++
		}
	}
	if questions != 1 {
		t.Fatalf("question rows = %d; want exactly the one the person reads: %#v", questions, session.Events(before))
	}
}

// One refusal was not enough: the agent reasoned again, wrote nothing and
// asked the same question, which went through bare. So the second bare
// attempt of an exchange is sent back too, with the words that name that
// mistake. Twice and no more: an agent that asks a third time without writing
// anything is let through, because a question refused forever would leave the
// person with no question at all.
func TestStructuredQuestionIsSentBackAtMostTwiceInOneExchange(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	const bareQuestion = `{"type":"control_request","request_id":"%s","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"question":"Per chi?","header":"Livia","options":[{"label":"Clienti"},{"label":"Colleghi"}]}]}}}`
	fake.emit(discoveryInputFrame)
	waitFor(t, func() bool { return len(decisionsWritten(fake)) == 1 })
	fake.emit(fmt.Sprintf(bareQuestion, "discovery-2"))
	waitFor(t, func() bool { return len(decisionsWritten(fake)) == 2 })
	if second := decisionsWritten(fake)[1]; second["request_id"] != "discovery-2" || second["behavior"] != "deny" || second["message"] != execution.QuestionPrefaceInsistence {
		t.Fatalf("second refusal = %#v", second)
	}
	if len(client.pendingInputs()) != 0 {
		t.Fatal("a question sent back twice is pending")
	}
	fake.emit(fmt.Sprintf(bareQuestion, "discovery-3"))
	waitFor(t, func() bool { return len(client.pendingInputs()) == 1 })
	if len(decisionsWritten(fake)) != 2 {
		t.Fatalf("the third attempt was answered by this layer: %#v", decisionsWritten(fake))
	}
	// The person's answer opens a new exchange: the next bare question is
	// sent back again, starting from the first refusal.
	if err := client.respondInput(context.Background(), "discovery-3", map[string][]string{"0": {"Clienti"}}); err != nil {
		t.Fatal(err)
	}
	fake.emit(fmt.Sprintf(bareQuestion, "discovery-4"))
	waitFor(t, func() bool { return len(decisionsWritten(fake)) == 4 })
	if last := decisionsWritten(fake)[3]; last["request_id"] != "discovery-4" || last["behavior"] != "deny" || last["message"] != execution.QuestionPrefaceMessage {
		t.Fatalf("refusal of the new exchange = %#v", last)
	}
}

// The refusal asks for the message and must not cost the option descriptions:
// the wording that asked to name only the choices was answered by a question
// with no message before it and «Proposta di Andrea» as the whole description.
func TestQuestionPrefaceRefusalsAskForTheMessageAndKeepOptionDescriptions(t *testing.T) {
	for _, refusal := range []string{execution.QuestionPrefaceMessage, execution.QuestionPrefaceInsistence} {
		if !strings.Contains(refusal, "description") || strings.Contains(refusal, "only the choices") {
			t.Fatalf("refusal invites bare options: %q", refusal)
		}
	}
	if _, refuse := execution.QuestionPrefaceRefusal(2); refuse {
		t.Fatal("a third bare attempt must be let through")
	}
}
