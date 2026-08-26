package web

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// askingDialogue is an agent process that stops to ask before it uses a tool.
// It is the fakeDialogue every other conversation test uses, plus the two
// methods of localrun.Arbiter: what these tests exercise is the route and the
// payload, never a provider's protocol.
type askingDialogue struct {
	fakeDialogue

	askMu    sync.Mutex
	pending  []execution.PendingApproval
	answered [][2]string
	// refusal, when set, is what the process answers instead of taking the
	// decision. It is how a test reaches the refusal branch of the route without
	// inventing a second kind of failure.
	refusal error
}

func (d *askingDialogue) PendingApprovals() []execution.PendingApproval {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	return append([]execution.PendingApproval(nil), d.pending...)
}

func (d *askingDialogue) RespondApproval(_ context.Context, approvalID, optionID string) error {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	if d.refusal != nil {
		return d.refusal
	}
	d.answered = append(d.answered, [2]string{approvalID, optionID})
	kept := d.pending[:0]
	for _, approval := range d.pending {
		if approval.ID != approvalID {
			kept = append(kept, approval)
		}
	}
	d.pending = kept
	return nil
}

func (d *askingDialogue) ask(approval execution.PendingApproval) {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	d.pending = append(d.pending, approval)
}

func (d *askingDialogue) decisions() [][2]string {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	return append([][2]string(nil), d.answered...)
}

// askOn makes the agent of an open conversation stop and ask, and hands back
// the dialogue so the test can read what was answered.
//
// The dialogue is swapped on the session the provider opened rather than built
// into the provider: what is under test is a conversation whose agent asks, and
// every other conversation in this package must go on not asking.
func askOn(t *testing.T, provider *conversingProvider, id string, approval execution.PendingApproval) *askingDialogue {
	t.Helper()
	dialogue := &askingDialogue{}
	dialogue.ask(approval)
	provider.sessionOf(t, id).AttachDialogue(dialogue)
	return dialogue
}

func toolApproval(id, tool string) execution.PendingApproval {
	return execution.PendingApproval{
		ID:       id,
		ToolName: tool,
		Title:    "TITOLO-DELLA-RICHIESTA",
		Args:     json.RawMessage(`{"command":"git status"}`),
		Options:  localrun.ApprovalOptions(),
	}
}

func respondConversationApproval(t *testing.T, srv *Server, conversationID, approvalID, optionID string) (int, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost,
		conversationsRoute+"/"+conversationID+"/approvals/"+approvalID,
		map[string]any{"option_id": optionID})
	return w.Code, w.Body.String()
}

// A decision the agent of the conversation is waiting on travels in the
// conversation's own payload. Without it the thread would show an agent that
// has simply stopped, and the answer it is waiting for could never be given.
func TestConversationViewCarriesTheDecisionsItsAgentWaitsOn(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	askOn(t, provider, id, toolApproval("appr-1", "Bash"))

	status, read, body := readConversation(t, srv, id, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}
	if len(read.Approvals) != 1 {
		t.Fatalf("approvals = %#v, want the one open decision: %s", read.Approvals, body)
	}
	pending := read.Approvals[0]
	if pending.ID != "appr-1" || pending.ToolName != "Bash" || pending.Title != "TITOLO-DELLA-RICHIESTA" {
		t.Fatalf("the decision does not travel as the process declared it: %#v (%s)", pending, body)
	}
	if len(pending.Options) != 2 || pending.Options[0].ID != localrun.ApprovalAllow || pending.Options[1].ID != localrun.ApprovalDeny {
		t.Fatalf("the decision does not offer allow and deny: %#v (%s)", pending.Options, body)
	}
}

// A conversation whose agent asks nothing answers with an empty list, never a
// null: a client always iterates an array.
func TestAConversationThatWaitsOnNothingCarriesAnEmptyApprovalList(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	view := openConversationOK(t, srv)

	_, _, body := readConversation(t, srv, view.Conversation.ID, 0)
	var raw struct {
		Approvals *[]json.RawMessage `json:"approvals"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Approvals == nil {
		t.Fatalf("approvals is null instead of an empty array: %s", body)
	}
	if len(*raw.Approvals) != 0 {
		t.Fatalf("approvals = %v, want empty: %s", *raw.Approvals, body)
	}
}

// The answer reaches the process, and the payload that comes back no longer
// lists the decision — because the process stopped waiting on it, not because
// the route decided so.
func TestAnsweringAConversationDecisionReachesTheProcess(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	dialogue := askOn(t, provider, id, toolApproval("appr-1", "Bash"))

	status, body := respondConversationApproval(t, srv, id, "appr-1", localrun.ApprovalAllow)
	if status != http.StatusAccepted {
		t.Fatalf("POST approval = %d, want 202: %s", status, body)
	}
	if decisions := dialogue.decisions(); len(decisions) != 1 || decisions[0] != [2]string{"appr-1", localrun.ApprovalAllow} {
		t.Fatalf("the answer did not reach the process: %#v", decisions)
	}
	read := decodeConversation(t, body)
	if len(read.Approvals) != 0 {
		t.Fatalf("the answered decision is still offered: %#v (%s)", read.Approvals, body)
	}
}

// An answer with no option named is the caller's mistake, refused before the
// process is touched at all.
func TestAnsweringAConversationDecisionWithoutAnOptionIsRefused(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	dialogue := askOn(t, provider, id, toolApproval("appr-1", "Bash"))

	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/approvals/appr-1", map[string]any{"option_id": "  "})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST approval = %d, want 400: %s", w.Code, w.Body.String())
	}
	if decisions := dialogue.decisions(); len(decisions) != 0 {
		t.Fatalf("a refused answer still reached the process: %#v", decisions)
	}
}

// A refusal the process expressed is rendered as the refusal it is, with the
// status the reason maps to and never as an internal failure.
func TestAConversationDecisionRefusedByTheProcessIsRenderedAsARefusal(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	dialogue := askOn(t, provider, id, toolApproval("appr-1", "Bash"))
	dialogue.refusal = &execution.RunCommandError{
		Reason: execution.RunRefusedUnsupported,
		RunID:  id,
		Err:    context.Canceled,
	}

	status, body := respondConversationApproval(t, srv, id, "appr-1", localrun.ApprovalAllow)
	if status != http.StatusBadRequest {
		t.Fatalf("POST approval = %d, want 400 for an unsupported refusal: %s", status, body)
	}
}

// A conversation this workspace does not hold is refused with the sentence every
// other route uses for an unknown id, and never with a 500.
func TestAnsweringADecisionOfAnUnknownConversationIsNotFound(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	status, body := respondConversationApproval(t, srv, "conv-nope", "appr-1", localrun.ApprovalAllow)
	if status != http.StatusNotFound {
		t.Fatalf("POST approval = %d, want 404: %s", status, body)
	}
}
