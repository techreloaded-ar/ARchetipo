package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

// proposalLine is the single JSON line a conforming agent closes a proposal on.
// It is built through the encoder rather than written by hand so the test cannot
// assert a spelling no encoder would ever produce.
func proposalLine(t *testing.T, proposal ActionProposal) string {
	t.Helper()
	line, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("encoding the fixture proposal: %v", err)
	}
	if strings.Contains(string(line), "\n") {
		t.Fatalf("the encoded proposal spans more than one line: %s", line)
	}
	return string(line)
}

func fixtureProposal() ActionProposal {
	return ActionProposal{
		Artifact: ActionProposalArtifact,
		Action:   "azione-fittizia",
		Spec:     "US-777",
	}
}

// A proposal lives inside a conversation, so it arrives surrounded by talk: the
// agent explains what it would do before and after declaring it. Prose must not
// shadow the declaration.
func TestParseActionProposalTakesTheProposalDespiteSurroundingNoise(t *testing.T) {
	want := fixtureProposal()
	text := strings.Join([]string{
		"Ho guardato lo stato del workspace.",
		"Il passo successivo che propongo è questo.",
		proposalLine(t, want),
		"Confermi?",
	}, "\n")

	got, ok := ParseActionProposal(text)
	if !ok {
		t.Fatal("a proposal surrounded by prose was not recognized")
	}
	if got.Action != want.Action {
		t.Fatalf("action = %q, want %q", got.Action, want.Action)
	}
	if got.Spec != want.Spec {
		t.Fatalf("spec = %q, want %q", got.Spec, want.Spec)
	}
}

// An agent that reconsiders inside the same message proposes twice; what it
// would start now is what it said last, not what it said first.
func TestParseActionProposalKeepsTheLastProposal(t *testing.T) {
	first := fixtureProposal()
	first.Action = "prima-azione"
	second := fixtureProposal()
	second.Action = "seconda-azione"
	text := strings.Join([]string{
		proposalLine(t, first),
		"ripensandoci, meglio quest'altra",
		proposalLine(t, second),
	}, "\n")

	got, ok := ParseActionProposal(text)
	if !ok {
		t.Fatal("a text carrying two proposals was not recognized")
	}
	if got.Action != "seconda-azione" {
		t.Fatalf("action = %q, want the last one", got.Action)
	}
}

// Most of what an agent says is talk. Recognizing a proposal where there is none
// would let ordinary conversation reach the confirmation step.
func TestParseActionProposalRejectsWhatIsNotOne(t *testing.T) {
	otherArtifact := fixtureProposal()
	otherArtifact.Artifact = "spec_draft"
	blankAction := fixtureProposal()
	blankAction.Action = "   "

	for name, text := range map[string]string{
		"prose only":       "Il workspace è pulito, non serve avviare nulla.",
		"another artifact": proposalLine(t, otherArtifact),
		"blank action":     proposalLine(t, blankAction),
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := ParseActionProposal(text); ok {
				t.Fatalf("accepted a text carrying no proposal: %#v", got)
			}
		})
	}
}

// Some actions are about the workspace as a whole and name no spec: the absence
// of the spec is the normal form of such a proposal, not a malformed one.
func TestParseActionProposalAcceptsAWorkspaceScopedProposal(t *testing.T) {
	text := `{"artifact":"action_proposal","action":"azione-di-workspace"}`

	got, ok := ParseActionProposal(text)
	if !ok {
		t.Fatal("a workspace-scoped proposal was not recognized")
	}
	if got.Action != "azione-di-workspace" {
		t.Fatalf("action = %q", got.Action)
	}
	if got.Spec != "" {
		t.Fatalf("spec = %q, want it empty", got.Spec)
	}
}
