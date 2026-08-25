package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// The head of the conversation names who is answering, and the provider id is
// only the first third of that: the same provider holds a conversation with a
// different model and a different reasoning budget from one workspace to the
// next. These tests are about the two fields that carry the other two thirds to
// the browser — `model` and `model_options` — and they read them out of the raw
// JSON rather than out of the Go view, so a renamed tag cannot pass unnoticed.

// catalogedConversingProvider both holds a conversation and declares a model
// catalog. The two are separate optional interfaces on purpose, and no provider
// of these tests implemented both until now: what the head shows is exactly
// their conjunction.
type catalogedConversingProvider struct {
	*conversingProvider
	models []execution.ModelOption
	err    error
}

func (p *catalogedConversingProvider) Models(_ context.Context, _ map[string]any) ([]execution.ModelOption, error) {
	if p.err != nil {
		return nil, p.err
	}
	return execution.CloneModels(p.models), nil
}

var _ execution.ModelLister = (*catalogedConversingProvider)(nil)

// conversationModels is the catalog these tests declare: one model that accepts
// an option, and one that accepts none.
func conversationModels() []execution.ModelOption {
	return []execution.ModelOption{
		{
			ID: "modello-uno",
			Options: []execution.ModelOptionField{{
				Name:  "sforzo",
				Label: "Sforzo",
				Choices: []execution.ModelOptionChoice{
					{Value: "basso"},
					{Value: "alto"},
				},
			}},
		},
		{ID: "modello-due", Default: true},
	}
}

func newCatalogedConversingProvider(id string, models []execution.ModelOption) *catalogedConversingProvider {
	return &catalogedConversingProvider{
		conversingProvider: newConversingProvider(id, 0),
		models:             models,
	}
}

// configureDefaultProvider rewrites the default provider block of the workspace
// the server is serving, so a test can state the model and the options a
// conversation is held with.
func configureDefaultProvider(t *testing.T, srv *Server, id string, providerConfig map[string]any) {
	t.Helper()
	root := srv.session().cfg.ProjectRoot
	if _, err := config.UpdateDefaultProvider(root, config.DefaultProviderConfig{ID: id, Config: providerConfig}); err != nil {
		t.Fatal(err)
	}
}

// conversationFieldOf reads one top-level key of the conversation payload as it
// travels on the wire. A key the payload omits reads as nil, which is exactly
// what "there is nothing to say about the model here" must look like.
func conversationFieldOf(t *testing.T, body, key string) any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("undecodable conversation payload: %v (%s)", err, body)
	}
	return payload[key]
}

func TestConversationNamesTheModelAndTheOptionsItIsHeldWith(t *testing.T) {
	provider := newCatalogedConversingProvider("conversante", conversationModels())
	srv := newConversationServer(t, provider)
	configureDefaultProvider(t, srv, "conversante", map[string]any{
		"model":  "modello-uno",
		"sforzo": "alto",
	})

	view := openConversationOK(t, srv)
	status, _, body := readConversation(t, srv, view.Conversation.ID, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}

	if got := conversationFieldOf(t, body, "model"); got != "modello-uno" {
		t.Fatalf("model is %#v, want %q", got, "modello-uno")
	}
	options, ok := conversationFieldOf(t, body, "model_options").(map[string]any)
	if !ok {
		t.Fatalf("model_options is not an object: %s", body)
	}
	if options["sforzo"] != "alto" {
		t.Fatalf("model_options[sforzo] is %#v, want %q", options["sforzo"], "alto")
	}
}

// A configuration that names no model still resolves to one: the catalog says
// which model the provider uses when nothing is configured, and that is the
// model the conversation is really held with.
func TestConversationNamesTheDefaultModelWhenNoneIsConfigured(t *testing.T) {
	provider := newCatalogedConversingProvider("conversante", conversationModels())
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	status, _, body := readConversation(t, srv, view.Conversation.ID, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}

	if got := conversationFieldOf(t, body, "model"); got != "modello-due" {
		t.Fatalf("model is %#v, want the catalog default %q", got, "modello-due")
	}
	if got := conversationFieldOf(t, body, "model_options"); got != nil {
		t.Fatalf("model_options is %#v, want it omitted: the default model declares no option", got)
	}
}

// A provider with no catalog at all leaves both fields out, and the payload is
// byte-for-byte the one a client read before these fields existed.
func TestConversationOmitsTheModelWhenTheProviderDeclaresNoCatalog(t *testing.T) {
	provider := newConversingProvider("conversante", 0)
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	status, _, body := readConversation(t, srv, view.Conversation.ID, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}

	if got := conversationFieldOf(t, body, "model"); got != nil {
		t.Fatalf("model is %#v, want it omitted: the provider declares no catalog", got)
	}
	if got := conversationFieldOf(t, body, "model_options"); got != nil {
		t.Fatalf("model_options is %#v, want it omitted: the provider declares no catalog", got)
	}
	// And the provider is still named: the head has something to say even when
	// it cannot say everything.
	if got := conversationFieldOf(t, body, "provider_id"); got != "conversante" {
		t.Fatalf("provider_id is %#v, want %q", got, "conversante")
	}
}
