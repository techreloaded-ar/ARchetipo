package web

// The oracle of "a run started from a thread is read in that thread".
//
// Nothing on the start path is doubled here either: the record store, the
// connector, the process Template and the start functions are the production
// ones, and what is asserted is the JSON the browser reads — the run blocks of
// the conversation and the rows of the runs strip — never the server's structs.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// adoptTestRunsView decodes the rows of GET /api/workspace/runs this file
// asserts on: which run, and which conversation it says asked for it.
type adoptTestRunsView struct {
	Runs []struct {
		ID             string `json:"id"`
		SpecCode       string `json:"spec_code"`
		ConversationID string `json:"conversation_id"`
		AnchorEventID  int64  `json:"anchor_event_id"`
	} `json:"runs"`
}

func adoptTestWorkspaceRuns(t *testing.T, srv *Server) adoptTestRunsView {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/runs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET workspace runs = %d, want 200: %s", w.Code, w.Body.String())
	}
	var view adoptTestRunsView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("undecodable runs view: %v (%s)", err, w.Body.String())
	}
	return view
}

// adoptTestStart presses the spec-scoped start the way the viewer does, with
// the conversation the press came from when there is one.
func adoptTestStart(t *testing.T, srv *Server, code, action, conversationID string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{"action": action}
	if conversationID != "" {
		body["conversation_id"] = conversationID
	}
	w := doJSON(t, srv, http.MethodPost, "/api/spec/"+code+"/execution", body)
	var decoded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("undecodable response (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, decoded
}

// TestARunStartedFromTheThreadIsReadInTheThread is the fault this feature
// removes: the recommended step is pressed at the tail of a conversation, and
// the run it starts has to be in that conversation — anchored where it was
// asked for — instead of existing only in the workspace strip.
func TestARunStartedFromTheThreadIsReadInTheThread(t *testing.T) {
	srv, provider := journalTestPlanningServer(t)
	id := journalTestOpenOnSpec(t, srv, "US-901")
	provider.emit(t, id, localrun.KindUserMessage, "Pianifica la US-901")
	said := provider.emit(t, id, localrun.KindText, "Va bene.")

	status, started := adoptTestStart(t, srv, "US-901", "plan", id)
	if status != http.StatusCreated {
		t.Fatalf("POST spec execution = %d, want 201: %v", status, started)
	}
	executionID, _ := started["id"].(string)
	if executionID == "" {
		t.Fatalf("the start does not name the execution: %v", started)
	}

	_, view, body := readConversation(t, srv, id, 0)
	if len(view.Runs) != 1 {
		t.Fatalf("the conversation carries %d run blocks, want the one it started: %s", len(view.Runs), body)
	}
	run := view.Runs[0]
	if run.ExecutionID != executionID {
		t.Errorf("the block names %q, want the execution the press started %q", run.ExecutionID, executionID)
	}
	if run.SpecCode != "US-901" || run.Action != "plan" {
		t.Errorf("the block says %s/%s, want plan on US-901", run.SpecCode, run.Action)
	}
	if run.Decision != "confirmed" {
		t.Errorf("decision = %q, want a confirmation: pressing the step is starting it", run.Decision)
	}
	if run.AnchorEventID != said.ID {
		t.Errorf("anchor = %d, want the last event said before the press %d", run.AnchorEventID, said.ID)
	}

	// And the strip leads back to the very thread it was pressed in.
	runs := adoptTestWorkspaceRuns(t, srv)
	if len(runs.Runs) != 1 {
		t.Fatalf("the strip lists %d runs, want the one just started: %#v", len(runs.Runs), runs.Runs)
	}
	if runs.Runs[0].ConversationID != id {
		t.Errorf("the strip ties the run to %q, want the conversation it was pressed in %q", runs.Runs[0].ConversationID, id)
	}
	if runs.Runs[0].AnchorEventID != said.ID {
		t.Errorf("the strip anchors at %d, want %d", runs.Runs[0].AnchorEventID, said.ID)
	}
}

// TestARunStartedFromTheBoardBelongsToNoThread is the boundary: only a press
// that came from a conversation is tied to one. A start from the board carries
// no id, and inventing "the" conversation for it would file a run under a
// discourse that never asked for it.
func TestARunStartedFromTheBoardBelongsToNoThread(t *testing.T) {
	srv, provider := journalTestPlanningServer(t)
	id := journalTestOpenOnSpec(t, srv, "US-901")
	provider.emit(t, id, localrun.KindUserMessage, "Pianifica la US-901")

	status, started := adoptTestStart(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("POST spec execution = %d, want 201: %v", status, started)
	}

	_, view, body := readConversation(t, srv, id, 0)
	if len(view.Runs) != 0 {
		t.Fatalf("the conversation adopted a run nobody asked it for: %s", body)
	}
	runs := adoptTestWorkspaceRuns(t, srv)
	if len(runs.Runs) != 1 {
		t.Fatalf("the strip lists %d runs, want the one just started: %#v", len(runs.Runs), runs.Runs)
	}
	if runs.Runs[0].ConversationID != "" {
		t.Errorf("the strip ties a board run to %q, want no conversation", runs.Runs[0].ConversationID)
	}
}

// TestAdoptingARunLeavesAPendingProposalPending is why adoption is not a
// decision: the agent may have just proposed something the person has not read,
// and pressing the recommended step must not answer it on their behalf.
func TestAdoptingARunLeavesAPendingProposalPending(t *testing.T) {
	srv, provider := journalTestPlanningServer(t)
	id := journalTestOpenOnSpec(t, srv, "US-901")
	proposalID := emitProposal(t, provider, id, "Posso pianificare US-902.", "plan", "US-902")

	if status, started := adoptTestStart(t, srv, "US-901", "plan", id); status != http.StatusCreated {
		t.Fatalf("POST spec execution = %d, want 201: %v", status, started)
	}

	view := readProposalConversation(t, srv, id)
	if view.Proposal == nil || view.Proposal.EventID != proposalID {
		t.Fatalf("the pending proposal was answered by a press that decided nothing: %#v", view.Proposal)
	}
	if view.Proposal.SpecCode != "US-902" {
		t.Errorf("the pending proposal is about %q, want US-902", view.Proposal.SpecCode)
	}
}
