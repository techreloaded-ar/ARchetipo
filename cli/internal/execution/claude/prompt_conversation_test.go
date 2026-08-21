package claude

import (
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// --- the conversation prompt ------------------------------------------------
//
// The conversation prompt is the only place where the agent learns that it may
// propose an action, and it is a pure function of the vocabulary it is handed.
// What is asserted here is the contract it has to satisfy — the ids it was
// given, the shape the parser recognizes, and the prohibition proposing did not
// remove — never the prose around them, which is free to change.

// conversationActions is the two-scope vocabulary these tests hand the prompt:
// one step about a spec and one about the workspace as a whole, which is the
// smallest list that exercises both branches of the rendered list.
func conversationActions() []execution.ConversationAction {
	return []execution.ConversationAction{
		{ID: "plan-spec", Label: "Pianifica la spec", Scope: "spec"},
		{ID: "workspace-status", Label: "Stato del workspace", Scope: "workspace"},
	}
}

// The process is knowledge of the caller, never of this package: the prompt has
// to name the ids and the labels it was handed, because an agent that cannot
// read the list is an agent that invents one.
func TestConversationPromptNamesTheDeclaredActions(t *testing.T) {
	got := buildConversationPrompt(conversationActions())

	for _, action := range conversationActions() {
		assertContains(t, got, action.ID, "conversation prompt")
		assertContains(t, got, action.Label, "conversation prompt")
		assertContains(t, got, action.Scope, "conversation prompt")
	}
}

// The proposal line the prompt asks for must be the line ParseActionProposal
// recognizes: the two are one contract, and a prompt asking for another shape
// would produce proposals the viewer silently drops.
func TestConversationPromptAsksForOneProposalLine(t *testing.T) {
	got := buildConversationPrompt(conversationActions())

	assertContains(t, got, `"artifact"`, "conversation prompt")
	assertContains(t, got, execution.ActionProposalArtifact, "conversation prompt")
	assertContains(t, got, `"action"`, "conversation prompt")

	// The oracle is the parser itself: a concrete line of the asked-for shape
	// has to be recognized as a proposal of a declared action.
	line := `{"artifact":"` + execution.ActionProposalArtifact + `","action":"plan-spec","spec":"US-054"}`
	proposal, ok := execution.ParseActionProposal("Avvierei la pianificazione.\n" + line)
	if !ok {
		t.Fatalf("the line the prompt asks for is not recognized as a proposal:\n%s", got)
	}
	if proposal.Action != "plan-spec" {
		t.Fatalf("parsed action = %q, want %q", proposal.Action, "plan-spec")
	}
}

// Proposing is not acting. Authorizing the proposal must not have loosened the
// prohibition, which is the courtesy layer above the structural guarantee that
// a conversation writes no execution record.
func TestConversationPromptStillForbidsActing(t *testing.T) {
	got := buildConversationPrompt(conversationActions())

	assertContains(t, got, "Do NOT act on the workspace", "conversation prompt")
	assertContains(t, got, "must not start any action of the process", "conversation prompt")
	assertContains(t, got, "must not invoke any `archetipo-*` skill", "conversation prompt")
	assertContains(t, got, "must not change the status of any spec", "conversation prompt")
}

// With no vocabulary there is nothing to propose, and a proposal block naming
// no id is an invitation to invent one: the whole block has to disappear.
func TestConversationPromptWithoutActionsProposesNothing(t *testing.T) {
	for _, actions := range [][]execution.ConversationAction{nil, {}} {
		got := buildConversationPrompt(actions)

		if strings.Contains(got, execution.ActionProposalArtifact) {
			t.Fatalf("a prompt with no declared action still asks for a proposal:\n%s", got)
		}
		// The prohibition does not depend on the vocabulary.
		assertContains(t, got, "Do NOT act on the workspace", "conversation prompt")
	}
}

// Like every other prompt of this file it is pure: the same vocabulary always
// renders the same string, in the order the process declared it.
func TestConversationPromptIsDeterministic(t *testing.T) {
	actions := conversationActions()

	first := buildConversationPrompt(actions)
	if second := buildConversationPrompt(actions); first != second {
		t.Fatalf("the conversation prompt is not deterministic:\n%s\n---\n%s", first, second)
	}

	// The declared order is the process's own and is never re-sorted here.
	if strings.Index(first, actions[0].ID) > strings.Index(first, actions[1].ID) {
		t.Fatalf("the prompt re-sorted the declared actions:\n%s", first)
	}
}
