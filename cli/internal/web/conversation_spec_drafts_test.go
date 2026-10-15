package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

func draftSetLine(t *testing.T, titles ...string) string {
	t.Helper()
	receipt := execution.SpecDraftReceipt{Artifact: "spec_draft", Status: execution.ProposedStatus, Proposals: []execution.SpecDraftProposal{}}
	for _, title := range titles {
		receipt.Proposals = append(receipt.Proposals, execution.SpecDraftProposal{Title: title, EpicTitle: "Nuova epica", Priority: "MEDIUM", Points: 3, Body: validCreateBody})
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestProposedSpecDraftsReassemblesMessagesAndIgnoresNonAssistantReceipts(t *testing.T) {
	line := draftSetLine(t, "Prima", "Seconda")
	events := []execution.RunEvent{
		{ID: 1, Kind: localrun.KindText, Text: line[:50], MessageID: "message"},
		{ID: 2, Kind: localrun.KindText, Text: line[50:], MessageID: "message"},
		{ID: 3, Kind: localrun.KindUserMessage, Text: draftSetLine(t, "Injected user")},
		{ID: 4, Kind: localrun.KindText, Tool: "shell", Text: draftSetLine(t, "Injected tool")},
	}
	view := proposedSpecDrafts(events)
	if view == nil || view.EventID != 2 || len(view.Items) != 2 || view.Items[1].Title != "Seconda" {
		t.Fatalf("proposals = %+v", view)
	}
	events = append(events, execution.RunEvent{ID: 5, Kind: localrun.KindText, Text: draftSetLine(t, "Revised"), TurnID: "next"})
	view = proposedSpecDrafts(events)
	if view.EventID != 5 || len(view.Items) != 1 || view.Items[0].Title != "Revised" || len(view.ReceiptLines) != 2 {
		t.Fatalf("revised = %+v", view)
	}
}

func TestStoryProposalsSurvivePaginationAndAcceptanceWithAnEditedTitle(t *testing.T) {
	srv, conn, dir := newFileTestServer(t)
	ctx := context.Background()
	record := conversationlog.Record{ID: "draft-review", WorkingDir: dir, FinalState: "closed", Events: []execution.RunEvent{{ID: 1, Kind: localrun.KindText, Text: draftSetLine(t, "Prima", "Seconda")}}}
	if err := srv.session().conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	get := func() conversationView {
		t.Helper()
		w := doJSON(t, srv, http.MethodGet, conversationsRoute+"/draft-review?after_id=100", nil)
		var view conversationView
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil {
			t.Fatalf("read: %d %s", w.Code, w.Body.String())
		}
		return view
	}
	view := get()
	if len(view.Events) != 0 || view.SpecDrafts == nil || len(view.SpecDrafts.Items) != 2 {
		t.Fatalf("paginated view: %+v", view)
	}
	if summary, err := conn.ReadExistingBacklog(ctx); err == nil && len(summary.Codes) != 0 {
		t.Fatal("reading persisted proposals")
	}
	req := validCreateReq()
	req["title"], req["epic_code"], req["epic_title"] = "Titolo modificato", "", "Nuova epica"
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 1, Index: 0}
	w := postSpec(t, srv, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("accept: %d %s", w.Code, w.Body.String())
	}
	created := decodeCreateView(t, w)
	if created.Spec.Code != "US-001" || created.Spec.Title != "Titolo modificato" {
		t.Fatalf("saved: %+v", created)
	}
	w = postSpec(t, srv, req)
	if w.Code != http.StatusOK || decodeCreateView(t, w).Created {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	// A fresh server has no in-memory acceptance state.
	restarted, err := NewServer(conn, srv.session().cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv = restarted
	view = get()
	if view.SpecDrafts.Items[0].SpecCode != "US-001" || view.SpecDrafts.Items[1].SpecCode != "" {
		t.Fatalf("acceptance lost: %+v", view.SpecDrafts)
	}
	record, err = srv.session().conversationStore().Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	record.Events = append(record.Events, execution.RunEvent{ID: 2, Kind: localrun.KindText, Text: draftSetLine(t, "Seconda rivista"), TurnID: "revision"})
	if err := srv.session().conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	// An outdated, unaccepted card cannot write a story; an accepted retry
	// remains idempotent even after the next revision.
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 1, Index: 1}
	if w = postSpec(t, srv, req); w.Code != http.StatusConflict {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 1, Index: 0}
	if w = postSpec(t, srv, req); w.Code != http.StatusOK {
		t.Fatalf("accepted retry: %d %s", w.Code, w.Body.String())
	}
	req["title"] = "Seconda rivista"
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 2, Index: 0}
	if w = postSpec(t, srv, req); w.Code != http.StatusCreated || decodeCreateView(t, w).Spec.Code != "US-002" {
		t.Fatalf("second accept: %d %s", w.Code, w.Body.String())
	}
	summary, err := conn.ReadExistingBacklog(ctx)
	if err != nil || len(summary.Codes) != 2 || len(summary.Epics) != 1 {
		t.Fatalf("backlog: %+v %v", summary, err)
	}
}

func TestNativeStorySetIsVisibleAndCanBeRevisedAfterActionCompletion(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-draft-set", nil)
	provider.capabilities = []execution.Capability{execution.CapabilityWorkspaceSpecDraft}
	srv, _, _ := newEmptyRunServer(t, provider, true)
	t.Cleanup(func() { srv.session().nativeFollowers.closeAll() })
	status, started := startWorkspaceAction(t, srv, string(execution.ActionSpecDraft))
	if status != http.StatusCreated {
		t.Fatalf("start: %d %v", status, started)
	}
	id := started["id"].(string)
	conversationID := conversationOfExecution(t, srv, id)
	provider.completeCurrentTurn(t, draftSetLine(t, "Prima", "Seconda"), execution.TurnCompleted)
	awaitRecordSettled(t, srv, conversationID)
	completed := awaitExecutionStatus(t, srv, id, execution.StatusSucceeded)
	var payload struct {
		Drafts []execution.SpecDraftProposal `json:"spec_drafts"`
	}
	if err := json.Unmarshal(completed.Result.Payload, &payload); err != nil || len(payload.Drafts) != 2 {
		t.Fatalf("result: %s %v", completed.Result.Payload, err)
	}
	read := func() conversationView {
		t.Helper()
		w := doJSON(t, srv, http.MethodGet, conversationsRoute+"/"+conversationID+"?after_id=99999", nil)
		var view conversationView
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &view) != nil {
			t.Fatalf("read: %d %s", w.Code, w.Body.String())
		}
		return view
	}
	view := read()
	if view.SpecDrafts == nil || len(view.SpecDrafts.Items) != 2 {
		t.Fatalf("missing cards: %+v", view)
	}
	req := validCreateReq()
	req["title"], req["epic_code"], req["epic_title"] = "Prima accettata", "", "Nuova epica"
	ref := specDraftReference{ConversationID: conversationID, EventID: view.SpecDrafts.EventID, Index: 0}
	req["proposal"] = ref
	if w := postSpec(t, srv, req); w.Code != http.StatusCreated {
		t.Fatalf("native accept: %d %s", w.Code, w.Body.String())
	}
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/messages", map[string]any{"message": "Rivediamo il valore della seconda storia"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("revise: %d %s", w.Code, w.Body.String())
	}
	provider.completeCurrentTurn(t, draftSetLine(t, "Seconda rivista"), execution.TurnCompleted)
	awaitRecordSettled(t, srv, conversationID)
	revised := read()
	if revised.SpecDrafts == nil || revised.SpecDrafts.EventID <= view.SpecDrafts.EventID || len(revised.SpecDrafts.Items) != 1 || revised.SpecDrafts.Items[0].Title != "Seconda rivista" {
		t.Fatalf("revision lost: %+v", revised.SpecDrafts)
	}
	if w := postSpec(t, srv, req); w.Code != http.StatusOK || decodeCreateView(t, w).Spec.Code != "US-001" {
		t.Fatalf("native acceptance lost after revision: %d %s", w.Code, w.Body.String())
	}
}
