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
	"strings"
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

// TestARunStartedNamingNoThreadIsItsOwnThread is the boundary, and it moved.
//
// Only a press that came from a conversation is *adopted* by one: a start that
// names none is anchored to no discourse, and inventing "the" conversation for
// it would file a run under a history that never asked for it. That is
// unchanged.
//
// What changed is that the run is not therefore threadless. A run is a
// conversation with a preconfigured prompt, so it is held as one under its own
// execution id: the session the provider registered for it *is* the thread, and
// there is one agent process rather than the two the viewer used to light.
func TestARunStartedNamingNoThreadIsItsOwnThread(t *testing.T) {
	srv, provider := journalTestPlanningServer(t)
	id := journalTestOpenOnSpec(t, srv, "US-901")
	provider.emit(t, id, localrun.KindUserMessage, "Pianifica la US-901")

	status, started := adoptTestStart(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("POST spec execution = %d, want 201: %v", status, started)
	}
	executionID, _ := started["id"].(string)
	if executionID == "" {
		t.Fatalf("the start does not name the execution: %v", started)
	}

	_, view, body := readConversation(t, srv, id, 0)
	if len(view.Runs) != 0 {
		t.Fatalf("the conversation adopted a run nobody asked it for: %s", body)
	}
	live := liveConversationIDs(srv)
	if len(live) != 2 {
		t.Fatalf("the workspace holds %v, want the conversation opened by hand and the run's own thread", live)
	}
	if !containsString(live, id) || !containsString(live, executionID) {
		t.Fatalf("the workspace holds %v, want %q and %q", live, id, executionID)
	}

	// The run's thread is the run: it is held under the execution's id, it says
	// which step it is doing, and it carries that execution as its outcome.
	_, own, ownBody := readConversation(t, srv, executionID, 0)
	if own.Conversation == nil {
		t.Fatalf("the run has no thread of its own: %s", ownBody)
	}
	if own.Conversation.Action != "plan" || own.Conversation.ExecutionID != executionID {
		t.Fatalf("the thread does not say it is the plan run: %#v (%s)", own.Conversation, ownBody)
	}
	if own.Conversation.SpecCode != "US-901" {
		t.Fatalf("the thread is not about the spec it plans: %#v", own.Conversation)
	}

	runs := adoptTestWorkspaceRuns(t, srv)
	if len(runs.Runs) != 1 {
		t.Fatalf("the strip lists %d runs, want the one just started: %#v", len(runs.Runs), runs.Runs)
	}
	if runs.Runs[0].ConversationID != "" {
		t.Errorf("the strip ties an unanchored run to %q, want no conversation", runs.Runs[0].ConversationID)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestAThreadOpenedForAStepCarriesTheNameItWasGiven is the other side of the
// same gesture: the viewer opens the thread first, names it after the step, and
// starts the run in it. The name matters because nobody is going to type a
// first message into that thread, and an index full of "Conversazione del
// <date>" answers "which one is the planning of US-901?" with nothing.
func TestAThreadOpenedForAStepCarriesTheNameItWasGiven(t *testing.T) {
	srv, _ := journalTestPlanningServer(t)
	status, body := journalTestOpenWith(t, srv, map[string]any{"spec_code": "US-901", "title": "Pianifica"})
	if status != http.StatusCreated {
		t.Fatalf("POST conversation = %d, want 201: %s", status, body)
	}
	view := journalTestDecodeBound(t, body)
	if view.Conversation == nil {
		t.Fatalf("the open conversation is null: %s", body)
	}
	id := view.Conversation.ID

	status, started := adoptTestStart(t, srv, "US-901", "plan", id)
	if status != http.StatusCreated {
		t.Fatalf("POST spec execution = %d, want 201: %v", status, started)
	}
	executionID, _ := started["id"].(string)

	_, threadView, threadBody := readConversation(t, srv, id, 0)
	if len(threadView.Runs) != 1 || threadView.Runs[0].ExecutionID != executionID {
		t.Fatalf("the thread does not carry the run %q it was opened for: %s", executionID, threadBody)
	}

	index, indexBody := conversationsRouteTestReadIndex(t, srv)
	entry := conversationsRouteTestEntryOf(t, index, id)
	if !entry.Live {
		t.Errorf("the index does not list the thread of a run in flight as live: %s", indexBody)
	}
	if entry.SpecCode != "US-901" {
		t.Errorf("the index files the thread under %q, want US-901", entry.SpecCode)
	}
	if entry.Title != "Pianifica" {
		t.Errorf("the index calls the thread %q, want the name the open gave it", entry.Title)
	}
}

// TestAThreadOpenedWithNoNameKeepsTheDatedOne is the boundary of the name: an
// open that asks for none is named exactly as it always was, so the addition
// changes nothing for the thread a person opens by hand.
func TestAThreadOpenedWithNoNameKeepsTheDatedOne(t *testing.T) {
	srv, _ := journalTestPlanningServer(t)
	id := journalTestOpenOnSpec(t, srv, "US-901")
	index, indexBody := conversationsRouteTestReadIndex(t, srv)
	entry := conversationsRouteTestEntryOf(t, index, id)
	if !strings.HasPrefix(entry.Title, "Conversazione del ") {
		t.Fatalf("the index calls the unnamed thread %q, want the dated fallback: %s", entry.Title, indexBody)
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
