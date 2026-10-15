package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
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

func TestRevisionUpdatesSameSpecAndPreservesWorkflowArtifacts(t *testing.T) {
	srv, conn, dir := newFileTestServer(t)
	ctx := context.Background()
	first := execution.SpecDraftProposal{DraftID: "trip-mode", Title: "Mezzo del viaggio", EpicTitle: "Itinerari", Priority: "MEDIUM", Points: 3, Body: validCreateBody}
	second := first
	second.DraftID, second.Title = "trip-search", "Cercare viaggi in bici"
	line := func(drafts ...execution.SpecDraftProposal) string {
		t.Helper()
		raw, err := json.Marshal(execution.SpecDraftReceipt{Artifact: "spec_draft", Status: execution.ProposedStatus, Proposals: drafts})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	record := conversationlog.Record{ID: "revision-review", WorkingDir: dir, FinalState: "closed", Events: []execution.RunEvent{{ID: 1, Kind: localrun.KindText, TurnID: "first", Text: line(first, second)}}}
	if err := srv.session().conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	for i, draft := range []execution.SpecDraftProposal{first, second} {
		req := validCreateReq()
		req["title"], req["epic_code"], req["epic_title"] = draft.Title, "", draft.EpicTitle
		req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 1, Index: i}
		if w := postSpec(t, srv, req); w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
	}
	branch, worktree, fork, rework := "archetipo/US-001", "/tmp/preserved-worktree", "abc123", true
	if _, err := conn.UpdateSpec(ctx, "US-001", domain.SpecUpdate{Branch: &branch, Worktree: &worktree, ForkBase: &fork, Rework: &rework}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.SavePlan(ctx, "US-001", domain.PlanInput{PlanBody: "Existing plan", Tasks: []domain.Task{{Ref: "T-1", Title: "Existing task"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.TransitionStatus(ctx, "US-001", domain.StatusPlanned); err != nil {
		t.Fatal(err)
	}
	beforeTasks, err := conn.ReadSpecTasks(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	first.Title = "Mezzo e tipi di bicicletta"
	first.Body = validCreateBody + "\nTipi: MTB, Gravel, Stradale.\n"
	second.Body = validCreateBody + "\nFiltro per tipo di bicicletta.\n"
	record, err = srv.session().conversationStore().Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Reordering cannot retarget a renamed story: the stable identities carry
	// its lineage, while a same-title revision must still update its body.
	record.Events = append(record.Events, execution.RunEvent{ID: 2, Kind: localrun.KindText, TurnID: "revision", Text: line(second, first)})
	if err := srv.session().conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	view := srv.session().specDraftsView(ctx, record.ID, record.Events)
	if len(view.Versions) != 2 || view.Items[0].DestinationID != "US-002" || view.Items[1].DestinationID != "US-001" {
		t.Fatalf("lineage: %+v", view)
	}
	for i, draft := range []execution.SpecDraftProposal{second, first} {
		item := view.Items[i]
		var destination conversationSpecDraftDestination
		for _, candidate := range item.Destinations {
			if candidate.ID == item.DestinationID {
				destination = candidate
			}
		}
		req := validCreateReq()
		req["title"], req["body"], req["epic_code"], req["epic_title"] = draft.Title, draft.Body, "", draft.EpicTitle
		req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 2, Index: i, TargetSpec: destination.ID, BaseHash: destination.BaseHash}
		w := postSpec(t, srv, req)
		if w.Code != http.StatusOK {
			t.Fatalf("approve: %d %s", w.Code, w.Body.String())
		}
		got := decodeCreateView(t, w)
		if !got.Updated || got.Created || got.Spec.Code != item.DestinationID || got.Spec.Body != draft.Body || got.Spec.Title != draft.Title {
			t.Fatalf("revision: %+v", got)
		}
		if retry := postSpec(t, srv, req); retry.Code != http.StatusOK || decodeCreateView(t, retry).Updated {
			t.Fatalf("revision retry: %d %s", retry.Code, retry.Body.String())
		}
	}
	saved, err := conn.ReadSpecDetail(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != domain.StatusPlanned || saved.Branch != branch || saved.Worktree != worktree || saved.ForkBase != fork || !saved.Rework {
		t.Fatalf("workflow metadata changed: %+v", saved)
	}
	afterTasks, err := conn.ReadSpecTasks(ctx, "US-001")
	if err != nil || len(afterTasks) != len(beforeTasks) || afterTasks[0].Title != beforeTasks[0].Title {
		t.Fatalf("plan changed: %+v %v", afterTasks, err)
	}
	summary, err := conn.ReadExistingBacklog(ctx)
	if err != nil || len(summary.Codes) != 2 {
		t.Fatalf("duplicate story: %+v %v", summary, err)
	}
}

func TestLegacyRenamedRevisionRequiresDestinationAndRejectsConcurrentEdits(t *testing.T) {
	srv, conn, dir := newFileTestServer(t)
	ctx := context.Background()
	record := conversationlog.Record{ID: "legacy-revision", WorkingDir: dir, FinalState: "closed", Events: []execution.RunEvent{{ID: 1, Kind: localrun.KindText, TurnID: "first", Text: draftSetLine(t, "Prima")}}}
	if err := srv.session().conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	req := validCreateReq()
	req["title"], req["epic_code"], req["epic_title"] = "Prima", "", "Nuova epica"
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 1, Index: 0}
	if w := postSpec(t, srv, req); w.Code != http.StatusCreated {
		t.Fatalf("create: %s", w.Body.String())
	}
	record, _ = srv.session().conversationStore().Get(ctx, record.ID)
	record.Events = append(record.Events, execution.RunEvent{ID: 2, Kind: localrun.KindText, TurnID: "revision", Text: draftSetLine(t, "Prima rinominata")})
	if err := srv.session().conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	view := srv.session().specDraftsView(ctx, record.ID, record.Events)
	if view.Items[0].DestinationID != "" || len(view.Items[0].Destinations) != 2 {
		t.Fatalf("legacy target was guessed: %+v", view.Items[0])
	}
	req["title"] = "Prima rinominata"
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 2, Index: 0}
	if w := postSpec(t, srv, req); w.Code != http.StatusConflict {
		t.Fatalf("ambiguous proposal wrote: %d %s", w.Code, w.Body.String())
	}
	ref := specDraftReference{ConversationID: record.ID, EventID: 2, Index: 0, TargetSpec: "US-001", BaseHash: view.Items[0].Destinations[1].BaseHash}
	req["proposal"] = ref
	concurrentBody := validCreateBody + "\nUna modifica esterna.\n"
	if _, err := conn.UpdateSpec(ctx, "US-001", domain.SpecUpdate{Body: &concurrentBody}); err != nil {
		t.Fatal(err)
	}
	if w := postSpec(t, srv, req); w.Code != http.StatusConflict {
		t.Fatalf("overwrote concurrent edit: %d %s", w.Code, w.Body.String())
	}
	saved, _ := conn.ReadSpecDetail(ctx, "US-001")
	if !strings.Contains(saved.Body, "modifica esterna") {
		t.Fatal("concurrent edit lost")
	}
	ref.BaseHash = specDraftHash(saved)
	req["proposal"] = ref
	if w := postSpec(t, srv, req); w.Code != http.StatusOK || !decodeCreateView(t, w).Updated {
		t.Fatalf("explicit approval: %d %s", w.Code, w.Body.String())
	}
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

func TestDraftVersionsDeduplicateNativeFinalMessageWithoutLosingLaterIdenticalVersions(t *testing.T) {
	line := draftSetLine(t, "Prima")
	events := []execution.RunEvent{{ID: 1, Kind: localrun.KindText, TurnID: "one", Text: line}, {ID: 2, Kind: localrun.KindTurnEnd, TurnID: "one", FinalMessage: line}, {ID: 3, Kind: localrun.KindUserMessage, Text: "Confermiamo questa versione"}, {ID: 4, Kind: localrun.KindText, TurnID: "two", Text: line}, {ID: 5, Kind: localrun.KindTurnEnd, TurnID: "two", FinalMessage: line}}
	view := proposedSpecDrafts(events)
	if len(view.Versions) != 2 || view.Versions[0].EventID != 2 || view.EventID != 5 {
		t.Fatalf("native versions: %+v", view.Versions)
	}
	events = []execution.RunEvent{{ID: 1, Kind: localrun.KindText, Text: line}, {ID: 2, Kind: localrun.KindUserMessage, Text: "Ripeti la proposta"}, {ID: 3, Kind: localrun.KindText, Text: line}}
	view = proposedSpecDrafts(events)
	if len(view.Versions) != 2 || view.Versions[0].EventID != 1 || view.EventID != 3 {
		t.Fatalf("legacy versions: %+v", view.Versions)
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
	req["proposal"] = specDraftReference{ConversationID: record.ID, EventID: 2, Index: 0, TargetSpec: "__new__"}
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
