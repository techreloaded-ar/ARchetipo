package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// conversationsRouteTestEntry is the wire shape of one line of the index,
// written out here on purpose: these assertions are about the JSON the rail
// reads, not about the server's own struct, and the two are allowed to be
// different things.
type conversationsRouteTestEntry struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	SpecCode      string `json:"spec_code"`
	OpenedAt      string `json:"opened_at"`
	LastMessageAt string `json:"last_message_at"`
	MessageCount  int    `json:"message_count"`
	ResumedFrom   string `json:"resumed_from"`
	State         string `json:"state"`
	Live          bool   `json:"live"`
}

type conversationsRouteTestIndex struct {
	Conversations []conversationsRouteTestEntry `json:"conversations"`
}

type conversationsRouteTestTranscript struct {
	conversationsRouteTestEntry
	Events []struct {
		ID   int64  `json:"id"`
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"events"`
}

// conversationsRouteTestStore is a second store opened on the very directory of
// the workspace the server is serving. Writing through it is how a test
// produces conversations that the running process never held — which is the
// only honest way to express "these survived a restart".
func conversationsRouteTestStore(t *testing.T, srv *Server) *conversationlog.FileStore {
	t.Helper()
	ws := srv.session()
	if ws == nil {
		t.Fatal("the server is serving no workspace")
	}
	store, err := conversationlog.NewFileStore(ws.cfg.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func conversationsRouteTestSave(t *testing.T, store *conversationlog.FileStore, record conversationlog.Record) {
	t.Helper()
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

// conversationsRouteTestReadIndex reads the index and hands back both the
// decoded payload and the raw body, because some of what has to be proved lives
// only in the bytes.
func conversationsRouteTestReadIndex(t *testing.T, srv *Server) (conversationsRouteTestIndex, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/conversations", nil)
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("GET conversations = %d, want 200: %s", w.Code, body)
	}
	var view conversationsRouteTestIndex
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("undecodable conversations index: %v (%s)", err, body)
	}
	return view, body
}

func conversationsRouteTestReadTranscript(t *testing.T, srv *Server, id string) (int, conversationsRouteTestTranscript, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/conversations/"+id, nil)
	body := w.Body.String()
	if w.Code != http.StatusOK {
		return w.Code, conversationsRouteTestTranscript{}, body
	}
	var view conversationsRouteTestTranscript
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("undecodable transcript: %v (%s)", err, body)
	}
	return w.Code, view, body
}

func conversationsRouteTestEntryOf(t *testing.T, view conversationsRouteTestIndex, id string) conversationsRouteTestEntry {
	t.Helper()
	for _, entry := range view.Conversations {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("the index carries no conversation %q: %#v", id, view.Conversations)
	return conversationsRouteTestEntry{}
}

// conversationsRouteTestInstant is the wire spelling of an instant, the same
// one the routes use. A test that formatted it its own way would be asserting
// against a second implementation of the same rule.
func conversationsRouteTestInstant(at time.Time) string {
	return at.UTC().Format("2006-01-02T15:04:05.000Z")
}

// TestConversationsIndexOnAWorkspaceWithNoConversationsIsAnEmptyArray is AC-7.
//
// The assertion is on the raw JSON and not on len(): a nil slice and an empty
// one are the same length in Go and opposite answers on the wire, and only
// "conversations":[] lets the rail say "there are none" without having to
// interpret a missing key.
func TestConversationsIndexOnAWorkspaceWithNoConversationsIsAnEmptyArray(t *testing.T) {
	srv := newConversationServer(t, newConversingProvider("chatty", 0))

	view, body := conversationsRouteTestReadIndex(t, srv)
	if !strings.Contains(body, `"conversations":[]`) {
		t.Errorf(`the index of an untouched workspace is not "conversations":[]: %s`, body)
	}
	if len(view.Conversations) != 0 {
		t.Errorf("conversations = %#v, want none", view.Conversations)
	}
}

// TestConversationsIndexListsRecentFirstWithTitleAndSpecCode is AC-2: every
// line carries what a person picks a thread by — a title, when it was last
// spoken in, and the spec it was about when it was about one — and the most
// recent one comes first.
func TestConversationsIndexListsRecentFirstWithTitleAndSpecCode(t *testing.T) {
	srv := newConversationServer(t, newConversingProvider("chatty", 0))
	store := conversationsRouteTestStore(t, srv)

	older := time.Date(2026, 8, 20, 9, 30, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 22, 17, 5, 0, 0, time.UTC)
	conversationsRouteTestSave(t, store, conversationlog.Record{
		ID:            "conv-libera",
		Title:         "Come rinominiamo la board",
		OpenedAt:      older.Add(-time.Hour),
		LastMessageAt: older,
		MessageCount:  4,
		FinalState:    string(execution.RunClosed),
		Events:        []execution.RunEvent{},
	})
	conversationsRouteTestSave(t, store, conversationlog.Record{
		ID:            "conv-legata",
		SpecCode:      "US-058",
		Title:         "Perché il rail sta a sinistra",
		OpenedAt:      newer.Add(-time.Hour),
		LastMessageAt: newer,
		MessageCount:  2,
		FinalState:    string(execution.RunClosed),
		Events:        []execution.RunEvent{},
	})

	view, body := conversationsRouteTestReadIndex(t, srv)
	if len(view.Conversations) != 2 {
		t.Fatalf("the index carries %d conversations, want 2: %s", len(view.Conversations), body)
	}
	if view.Conversations[0].ID != "conv-legata" || view.Conversations[1].ID != "conv-libera" {
		t.Errorf("the index is ordered %q, %q, want the most recent first", view.Conversations[0].ID, view.Conversations[1].ID)
	}
	for _, entry := range view.Conversations {
		if strings.TrimSpace(entry.Title) == "" {
			t.Errorf("the conversation %q has no title: %s", entry.ID, body)
		}
	}
	bound := conversationsRouteTestEntryOf(t, view, "conv-legata")
	if bound.SpecCode != "US-058" {
		t.Errorf("spec_code = %q, want US-058", bound.SpecCode)
	}
	if bound.LastMessageAt != conversationsRouteTestInstant(newer) {
		t.Errorf("last_message_at = %q, want %q", bound.LastMessageAt, conversationsRouteTestInstant(newer))
	}
	if bound.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", bound.MessageCount)
	}
	free := conversationsRouteTestEntryOf(t, view, "conv-libera")
	if free.SpecCode != "" {
		t.Errorf("spec_code = %q on a free conversation, want empty — AC-5 tells the two apart by this field alone", free.SpecCode)
	}
	if free.LastMessageAt != conversationsRouteTestInstant(older) {
		t.Errorf("last_message_at = %q, want %q", free.LastMessageAt, conversationsRouteTestInstant(older))
	}
}

// TestConversationsIndexMarksOnlyTheHeldConversationLive: liveness is a fact of
// the process that is holding a conversation, so exactly the one the workspace
// holds is live — and once it is released nothing is, while everything stays
// listed.
func TestConversationsIndexMarksOnlyTheHeldConversationLive(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	store := conversationsRouteTestStore(t, srv)

	conversationsRouteTestSave(t, store, conversationlog.Record{
		ID:            "conv-passata",
		Title:         "Una conversazione di ieri",
		OpenedAt:      time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
		LastMessageAt: time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
		FinalState:    string(execution.RunClosed),
		Events:        []execution.RunEvent{},
	})

	opened := openConversationOK(t, srv)
	id := opened.Conversation.ID

	view, body := conversationsRouteTestReadIndex(t, srv)
	if len(view.Conversations) != 2 {
		t.Fatalf("the index carries %d conversations, want 2: %s", len(view.Conversations), body)
	}
	for _, entry := range view.Conversations {
		want := entry.ID == id
		if entry.Live != want {
			t.Errorf("the conversation %q has live = %v, want %v: %s", entry.ID, entry.Live, want, body)
		}
	}

	if status, _, closeBody := closeConversation(t, srv); status != http.StatusOK {
		t.Fatalf("DELETE conversation = %d, want 200: %s", status, closeBody)
	}

	after, afterBody := conversationsRouteTestReadIndex(t, srv)
	if len(after.Conversations) != 2 {
		t.Fatalf("closing a conversation dropped it from the index: %s", afterBody)
	}
	for _, entry := range after.Conversations {
		if entry.Live {
			t.Errorf("the conversation %q is still live after the workspace released it: %s", entry.ID, afterBody)
		}
	}
	if held := conversationsRouteTestEntryOf(t, after, id); held.State != string(execution.RunClosed) {
		t.Errorf("state = %q on a released conversation, want %q", held.State, execution.RunClosed)
	}
}

// TestConversationsIndexDoesNotResurrectAnActiveRecord is the invariant that
// makes AC-1 honest: a record left behind by a process that died — no final
// state written, "active" as far as the file knows — is history and not a
// running conversation. Liveness is read from the holder, and the holder here
// is empty.
func TestConversationsIndexDoesNotResurrectAnActiveRecord(t *testing.T) {
	srv := newConversationServer(t, newConversingProvider("chatty", 0))
	store := conversationsRouteTestStore(t, srv)

	conversationsRouteTestSave(t, store, conversationlog.Record{
		ID:            "conv-orfana",
		Title:         "Interrotta da uno spegnimento",
		OpenedAt:      time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC),
		LastMessageAt: time.Date(2026, 8, 22, 8, 20, 0, 0, time.UTC),
		FinalState:    "",
		Events:        []execution.RunEvent{},
	})

	view, body := conversationsRouteTestReadIndex(t, srv)
	entry := conversationsRouteTestEntryOf(t, view, "conv-orfana")
	if entry.Live {
		t.Errorf("a record with no final state was declared live with nothing holding it: %s", body)
	}
	if entry.State != "" {
		t.Errorf("state = %q, want the record's own empty final state travelling uninterpreted", entry.State)
	}
	if _, open := srv.session().conversation.current(); open {
		t.Error("reading the index installed a conversation on the workspace")
	}
}

// TestConversationTranscriptReturnsEveryEventInOrder is AC-3: the whole of what
// was said, in the order it was said, with nothing dropped for being of the
// wrong kind — a transcript read out of order, or short of a few lines, is a
// different conversation from the one that happened.
func TestConversationTranscriptReturnsEveryEventInOrder(t *testing.T) {
	srv := newConversationServer(t, newConversingProvider("chatty", 0))
	store := conversationsRouteTestStore(t, srv)

	at := time.Date(2026, 8, 22, 11, 0, 0, 0, time.UTC)
	written := []execution.RunEvent{
		{ID: 1, Seq: 1, At: at, Kind: localrun.KindUserMessage, Text: "come stiamo con US-058?"},
		{ID: 2, Seq: 2, At: at.Add(time.Second), Kind: localrun.KindThinking, Text: "leggo il piano"},
		{ID: 3, Seq: 3, At: at.Add(2 * time.Second), Kind: localrun.KindToolStart, Text: "read cli/internal/web", Tool: "read"},
		{ID: 4, Seq: 4, At: at.Add(3 * time.Second), Kind: localrun.KindToolEnd, Text: "letto"},
		{ID: 5, Seq: 5, At: at.Add(4 * time.Second), Kind: localrun.KindText, Text: "mancano i test delle rotte"},
		{ID: 6, Seq: 6, At: at.Add(5 * time.Second), Kind: localrun.KindTurnEnd, Text: ""},
	}
	conversationsRouteTestSave(t, store, conversationlog.Record{
		ID:            "conv-trascritta",
		SpecCode:      "US-058",
		Title:         "come stiamo con US-058?",
		OpenedAt:      at.Add(-time.Minute),
		LastMessageAt: at.Add(5 * time.Second),
		MessageCount:  2,
		FinalState:    string(execution.RunClosed),
		Events:        written,
	})

	status, view, body := conversationsRouteTestReadTranscript(t, srv, "conv-trascritta")
	if status != http.StatusOK {
		t.Fatalf("GET transcript = %d, want 200: %s", status, body)
	}
	if len(view.Events) != len(written) {
		t.Fatalf("the transcript carries %d events, want all %d: %s", len(view.Events), len(written), body)
	}
	var previous int64
	for i, event := range view.Events {
		if event.ID <= previous {
			t.Errorf("event %d has id %d, not strictly after %d: the transcript is out of order", i, event.ID, previous)
		}
		previous = event.ID
		if event.ID != written[i].ID || event.Kind != written[i].Kind || event.Text != written[i].Text {
			t.Errorf("event %d = {%d %q %q}, want {%d %q %q}", i, event.ID, event.Kind, event.Text, written[i].ID, written[i].Kind, written[i].Text)
		}
	}
	// The header travels with the transcript, so a deep link that lands here
	// without having read the index still knows what it is showing.
	if view.ID != "conv-trascritta" || view.SpecCode != "US-058" || strings.TrimSpace(view.Title) == "" {
		t.Errorf("the transcript header lost the conversation it belongs to: %s", body)
	}
	if view.Live {
		t.Errorf("a past conversation is declared live: %s", body)
	}
}

// TestLiveConversationTranscriptComesFromTheSession: for the conversation the
// workspace is holding, the session in memory is the more recent of the two
// histories, and the two must not be allowed to disagree under the eyes of
// whoever is reading them.
func TestLiveConversationTranscriptComesFromTheSession(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	opened := openConversationOK(t, srv)
	id := opened.Conversation.ID
	provider.emit(t, id, localrun.KindUserMessage, "ciao")
	provider.emit(t, id, localrun.KindText, "eccomi")
	last := provider.emit(t, id, localrun.KindText, "l'ultima parola")

	// The file is deliberately a moment behind: nothing has re-read the live
	// conversation since it was opened, so the record still holds no event.
	store := conversationsRouteTestStore(t, srv)
	record, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Events) >= 3 {
		t.Fatalf("the record on disk is already up to date (%d events): this test can no longer tell the two histories apart", len(record.Events))
	}

	status, view, body := conversationsRouteTestReadTranscript(t, srv, id)
	if status != http.StatusOK {
		t.Fatalf("GET transcript = %d, want 200: %s", status, body)
	}
	if len(view.Events) != 3 {
		t.Fatalf("the transcript carries %d events, want the 3 the session holds: %s", len(view.Events), body)
	}
	if got := view.Events[len(view.Events)-1]; got.ID != last.ID || got.Text != "l'ultima parola" {
		t.Errorf("the transcript ends on {%d %q}, want the last event emitted {%d %q}", got.ID, got.Text, last.ID, "l'ultima parola")
	}
	if !view.Live {
		t.Errorf("the conversation the workspace is holding is not declared live: %s", body)
	}
}

// TestConversationTranscriptOfAnUnknownIDIsNotFound: a thread that does not
// exist here is a 404 that names it, not an empty transcript that would look
// like a conversation in which nobody ever said anything.
func TestConversationTranscriptOfAnUnknownIDIsNotFound(t *testing.T) {
	srv := newConversationServer(t, newConversingProvider("chatty", 0))

	status, _, body := conversationsRouteTestReadTranscript(t, srv, "conv-inesistente")
	if status != http.StatusNotFound {
		t.Fatalf("GET unknown transcript = %d, want 404: %s", status, body)
	}
	if message := refusalMessage(t, body); !strings.Contains(message, "conv-inesistente") {
		t.Errorf("the refusal says %q, want it to name the conversation that was asked for", message)
	}
}

// TestConversationsRoutesRefuseWithoutAnOpenWorkspace: both routes are about
// the workspace that is open, so with none open they refuse instead of
// answering with an empty index that would read as "this project has no
// history".
func TestConversationsRoutesRefuseWithoutAnOpenWorkspace(t *testing.T) {
	srv, _ := homeServer(t)

	routes := []scopedRoute{
		{"GET /api/workspace/conversations", http.MethodGet, "/api/workspace/conversations", ""},
		{"GET /api/workspace/conversations/{id}", http.MethodGet, "/api/workspace/conversations/conv-1", ""},
	}
	for _, route := range routes {
		t.Run(route.pattern, func(t *testing.T) {
			rec := callRoute(t, srv, route)
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s = %d, want 409: %s", route.pattern, rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding the refusal: %v (%s)", err, rec.Body.String())
			}
			if open, ok := body["workspaceOpen"].(bool); !ok || open {
				t.Errorf("workspaceOpen = %v, want false", body["workspaceOpen"])
			}
			if strings.Contains(rec.Body.String(), `"conversations"`) {
				t.Errorf("the refusal carries an index: %s", rec.Body.String())
			}
		})
	}
}
