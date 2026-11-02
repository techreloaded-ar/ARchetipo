package claude

import (
	"context"
	"encoding/json"
	"testing"
)

const discoveryInputFrame = `{"type":"control_request","request_id":"discovery","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"header":"Costanza","question":"Quale risultato conta?","multiSelect":false,"options":[{"label":"Risparmiare tempo","description":"Ridurre il lavoro manuale"},{"label":"Meno errori","description":"Aumentare la precisione"}]},{"header":"Livia","question":"Per chi?","multiSelect":true,"options":[{"label":"Clienti"},{"label":"Colleghi"}]}],"metadata":{"keep":"original"}}}}`

func TestDiscoveryQuestionsRoundTripWithoutBecomingApprovals(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	fake.emit(discoveryInputFrame)
	waitFor(t, func() bool { return len(client.pendingInputs()) == 1 })
	input := client.pendingInputs()[0]
	if len(client.PendingApprovals()) != 0 || input.Questions[0].Header != "Costanza" || !input.Questions[0].AllowText || !input.Questions[1].Multiple || input.Questions[0].Options[0].Description != "Ridurre il lavoro manuale" {
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
