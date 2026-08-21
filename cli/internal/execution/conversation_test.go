package execution

import (
	"context"
	"testing"
)

// conversingProvider implements Provider and Conversationalist and nothing
// else: it is the provider that can hold a free conversation without exposing
// an interactive run.
type conversingProvider struct {
	testProvider
	opened ConversationRequest
	closed string
}

func (p *conversingProvider) OpenConversation(_ context.Context, req ConversationRequest) error {
	p.opened = req
	return nil
}

func (p *conversingProvider) CloseConversation(_ context.Context, conversationID string) error {
	p.closed = conversationID
	return nil
}

// conversingCollaboratingProvider implements both optional interfaces, so the
// two derived capabilities have a provider on which they can be observed
// together.
type conversingCollaboratingProvider struct {
	collaboratingProvider
}

func (p *conversingCollaboratingProvider) OpenConversation(context.Context, ConversationRequest) error {
	return nil
}

func (p *conversingCollaboratingProvider) CloseConversation(context.Context, string) error {
	return nil
}

func TestConversationalistForDiscoversTheCapability(t *testing.T) {
	conversing := &conversingProvider{testProvider: testProvider{id: "conversing"}}
	conversationalist, ok := ConversationalistFor(conversing)
	if !ok {
		t.Fatalf("expected the conversing provider to be discovered")
	}
	if conversationalist == nil {
		t.Fatalf("expected a usable conversationalist, got nil")
	}
	// Usable, not merely non-nil: the discovered value must reach the real
	// implementation, otherwise the assertion above would pass on a wrapper that
	// does nothing.
	if err := conversationalist.OpenConversation(context.Background(), ConversationRequest{
		ConversationID: "conv-1",
		WorkingDir:     "/tmp/workspace",
	}); err != nil {
		t.Fatalf("OpenConversation failed: %v", err)
	}
	if conversing.opened.ConversationID != "conv-1" || conversing.opened.WorkingDir != "/tmp/workspace" {
		t.Fatalf("got %+v; want the request the caller passed", conversing.opened)
	}

	if conversationalist, ok := ConversationalistFor(&plainProvider{testProvider{id: "plain"}}); ok || conversationalist != nil {
		t.Fatalf("expected a plain provider not to converse, got %v, %v", conversationalist, ok)
	}

	var missing Provider
	if conversationalist, ok := ConversationalistFor(missing); ok || conversationalist != nil {
		t.Fatalf("expected a nil provider not to converse, got %v, %v", conversationalist, ok)
	}
}

func TestDeclaredCapabilitiesDerivesTheConversationFromTheInterface(t *testing.T) {
	conversing := &conversingProvider{
		testProvider: testProvider{id: "conversing", capabilities: []Capability{CapabilitySpecPlan}},
	}
	got, err := DeclaredCapabilities(context.Background(), conversing)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	// The whole list is the oracle, so the assertion also observes that the
	// derivation neither duplicates nor reorders what NormalizeCapabilities
	// guarantees.
	if want := []Capability{CapabilitySpecPlan, CapabilityWorkspaceConverse}; !equalCapabilities(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestDeclaredCapabilitiesWithholdsTheConversationFromAProviderThatCannotHoldOne(t *testing.T) {
	plain := &plainProvider{testProvider{id: "plain", capabilities: []Capability{CapabilitySpecPlan}}}
	got, err := DeclaredCapabilities(context.Background(), plain)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if want := []Capability{CapabilitySpecPlan}; !equalCapabilities(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	if Supports(got, CapabilityWorkspaceConverse) {
		t.Fatalf("a provider that cannot converse must not declare %s", CapabilityWorkspaceConverse)
	}
}

func TestTheConversationCapabilityIsNeverDeclaredByHand(t *testing.T) {
	// Whatever a provider says about itself, the conversation capability comes
	// from the interface and from nowhere else: neither fake names it in its own
	// Capabilities, yet exactly one of the two ends up declaring it.
	conversing := &conversingProvider{
		testProvider: testProvider{id: "conversing", capabilities: []Capability{CapabilitySpecPlan}},
	}
	plain := &plainProvider{testProvider{id: "plain", capabilities: []Capability{CapabilitySpecPlan}}}

	for _, provider := range []Provider{conversing, plain} {
		own, err := provider.Capabilities(context.Background())
		if err != nil {
			t.Fatalf("Capabilities failed: %v", err)
		}
		if Supports(own, CapabilityWorkspaceConverse) {
			t.Fatalf("provider %q declares %s by hand", provider.ID(), CapabilityWorkspaceConverse)
		}
	}

	declaredByConversing, err := DeclaredCapabilities(context.Background(), conversing)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if !Supports(declaredByConversing, CapabilityWorkspaceConverse) {
		t.Fatalf("got %v; want the derived %s", declaredByConversing, CapabilityWorkspaceConverse)
	}
	declaredByPlain, err := DeclaredCapabilities(context.Background(), plain)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if Supports(declaredByPlain, CapabilityWorkspaceConverse) {
		t.Fatalf("got %v; want no %s", declaredByPlain, CapabilityWorkspaceConverse)
	}
}

func TestDeclaredCapabilitiesDerivesTheTwoCapabilitiesIndependently(t *testing.T) {
	both := &conversingCollaboratingProvider{
		collaboratingProvider: collaboratingProvider{
			testProvider: testProvider{id: "both", capabilities: []Capability{CapabilitySpecPlan}},
			runID:        "run-9",
		},
	}
	got, err := DeclaredCapabilities(context.Background(), both)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	want := []Capability{CapabilityRunDialog, CapabilitySpecPlan, CapabilityWorkspaceConverse}
	if !equalCapabilities(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}

	// Independent: a provider that only collaborates declares the dialogue and
	// not the conversation, and a provider that only converses does the reverse.
	collaborating := &collaboratingProvider{
		testProvider: testProvider{id: "collaborating", capabilities: []Capability{CapabilitySpecPlan}},
		runID:        "run-9",
	}
	onlyDialogue, err := DeclaredCapabilities(context.Background(), collaborating)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if !Supports(onlyDialogue, CapabilityRunDialog) || Supports(onlyDialogue, CapabilityWorkspaceConverse) {
		t.Fatalf("got %v; want the dialogue alone", onlyDialogue)
	}

	conversing := &conversingProvider{
		testProvider: testProvider{id: "conversing", capabilities: []Capability{CapabilitySpecPlan}},
	}
	onlyConversation, err := DeclaredCapabilities(context.Background(), conversing)
	if err != nil {
		t.Fatalf("DeclaredCapabilities failed: %v", err)
	}
	if !Supports(onlyConversation, CapabilityWorkspaceConverse) || Supports(onlyConversation, CapabilityRunDialog) {
		t.Fatalf("got %v; want the conversation alone", onlyConversation)
	}
}
