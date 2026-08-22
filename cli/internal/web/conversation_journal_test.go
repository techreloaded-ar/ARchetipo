package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// journalTestSpecCode is a spec the seeded backlog of newRunServer really
// holds, so a conversation bound to it is bound to something a reader could
// open — and not to a code the connector happens to accept.
const journalTestSpecCode = "US-901"

// journalTestMissingSpecCode is a code no backlog of these fixtures carries.
const journalTestMissingSpecCode = "US-ZZZ"

// journalTestBoundView decodes only what this file asserts about the payload
// the browser receives: the identity of the conversation and the spec it was
// opened about. It is written out here rather than added to conversationResponse
// because these tests are about the binding, and a shared struct grown for one
// file is a struct every other file has to re-read.
type journalTestBoundView struct {
	Conversation *struct {
		ID       string `json:"id"`
		SpecCode string `json:"spec_code"`
	} `json:"conversation"`
}

func journalTestDecodeBound(t *testing.T, body string) journalTestBoundView {
	t.Helper()
	var view journalTestBoundView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("undecodable conversation view: %v (%s)", err, body)
	}
	return view
}

// journalTestOpenWith opens a conversation with an explicit body, which is the
// only way to ask for one bound to a spec. A nil payload sends no body at all —
// the request a viewer makes when it wants a free conversation.
func journalTestOpenWith(t *testing.T, srv *Server, payload any) (int, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversations", payload)
	return w.Code, w.Body.String()
}

// journalTestDir is where the records of the open workspace are written.
func journalTestDir(t *testing.T, srv *Server) string {
	t.Helper()
	return filepath.Join(srv.session().cfg.ProjectRoot, ".archetipo", "conversations")
}

// journalTestFiles lists the record files of the workspace. A directory that
// does not exist reads as no records at all, which is what a workspace nobody
// has talked to holds.
func journalTestFiles(t *testing.T, srv *Server) []string {
	t.Helper()
	entries, err := os.ReadDir(journalTestDir(t, srv))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}

// journalTestRecordOf reads the record back through the very store the journal
// writes with, so what is asserted is what survived to disk and not what the
// journal still holds in memory.
func journalTestRecordOf(t *testing.T, srv *Server, id string) conversationlog.Record {
	t.Helper()
	store, err := conversationlog.NewFileStore(srv.session().cfg.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("reading the record of %s: %v", id, err)
	}
	return record
}

func journalTestReadFile(t *testing.T, srv *Server, id string) ([]byte, os.FileInfo) {
	t.Helper()
	path := filepath.Join(journalTestDir(t, srv), id+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return body, info
}

// journalTestServer is a viewer on a workspace whose provider can converse.
func journalTestServer(t *testing.T) (*Server, *conversingProvider) {
	t.Helper()
	provider := newConversingProvider("journalling", 0)
	return newConversationServer(t, provider), provider
}

// journalTestOpenedID opens a free conversation and answers with its id.
func journalTestOpenedID(t *testing.T, srv *Server) string {
	t.Helper()
	return openConversationOK(t, srv).Conversation.ID
}

// TestJournalWritesTheThreadAsSoonAsItIsOpened is AC-1: a conversation exists
// in the index from the moment it was opened, so one that nobody spoke in is
// still something a person finds again after a restart.
func TestJournalWritesTheThreadAsSoonAsItIsOpened(t *testing.T) {
	srv, _ := journalTestServer(t)

	id := journalTestOpenedID(t, srv)

	files := journalTestFiles(t, srv)
	if len(files) != 1 {
		t.Fatalf("%s holds %v, want exactly one record", journalTestDir(t, srv), files)
	}
	if files[0] != id+".json" {
		t.Errorf("the record file is %q, want it named after the conversation %q", files[0], id)
	}
	record := journalTestRecordOf(t, srv, id)
	if record.ID != id {
		t.Errorf("record id = %q, want the id the payload announced %q", record.ID, id)
	}
	if record.Events == nil {
		t.Error("the events of a conversation nobody has spoken in are null, want an empty list")
	}
	if len(record.Events) != 0 {
		t.Errorf("a conversation just opened already carries %d events", len(record.Events))
	}
	if record.WorkingDir != srv.session().cfg.ProjectRoot {
		t.Errorf("working_dir = %q, want the project root of the workspace %q", record.WorkingDir, srv.session().cfg.ProjectRoot)
	}
}

// TestJournalTitlesAThreadWithTheFirstHumanMessage is AC-2: a reader picks a
// past conversation out of a list by what they themselves opened it with, so
// the title comes from the person's first message and never from the agent's
// answer — and a long one is cut where a reader would cut it, in runes.
func TestJournalTitlesAThreadWithTheFirstHumanMessage(t *testing.T) {
	srv, provider := journalTestServer(t)
	id := journalTestOpenedID(t, srv)

	spoken := strings.Repeat("però ", 40) + "fine"
	provider.emit(t, id, localrun.KindUserMessage, spoken)
	provider.emit(t, id, localrun.KindText, "la risposta dell'agente, che non dà il titolo a nulla")

	if status, _, body := readConversation(t, srv, id, 0); status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}

	normalized := strings.Join(strings.Fields(spoken), " ")
	want := string([]rune(normalized)[:conversationTitleLimit]) + conversationTitleEllipsis
	record := journalTestRecordOf(t, srv, id)
	if record.Title != want {
		t.Errorf("title = %q, want the first human message cut at %d runes: %q", record.Title, conversationTitleLimit, want)
	}
	if len([]rune(record.Title)) != conversationTitleLimit+1 {
		t.Errorf("the title is %d runes long, want %d plus the ellipsis", len([]rune(record.Title)), conversationTitleLimit)
	}
	if strings.Contains(record.Title, "risposta dell'agente") {
		t.Errorf("the agent named the conversation: %q", record.Title)
	}
}

// TestJournalTitlesAnUnspokenThreadWithItsDate is AC-2 on the thread nobody
// used: an index of such threads is only readable if each one says which one it
// is, so the fallback is dated and not generic.
func TestJournalTitlesAnUnspokenThreadWithItsDate(t *testing.T) {
	srv, _ := journalTestServer(t)

	id := journalTestOpenedID(t, srv)

	record := journalTestRecordOf(t, srv, id)
	if !strings.HasPrefix(record.Title, "Conversazione del ") {
		t.Errorf("title = %q, want it to open with %q", record.Title, "Conversazione del ")
	}
	if record.Title == "Conversazione del " {
		t.Error("the fallback title carries no date, so an index of unspoken threads cannot be read")
	}
}

// TestJournalRecordsWhatWasSaidAndWhen is AC-2: the index shows the moment of
// the last message, and how much was said. Tool calls are part of the
// transcript and not of the exchange, so they move the instant without
// inflating the count.
func TestJournalRecordsWhatWasSaidAndWhen(t *testing.T) {
	srv, provider := journalTestServer(t)
	id := journalTestOpenedID(t, srv)

	provider.emit(t, id, localrun.KindUserMessage, "ciao")
	provider.emit(t, id, localrun.KindText, "ciao a te")
	last := provider.emit(t, id, localrun.KindToolStart, "Read")

	if status, _, body := readConversation(t, srv, id, 0); status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}

	record := journalTestRecordOf(t, srv, id)
	if record.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2: the tool call is transcript, not exchange", record.MessageCount)
	}
	if !record.LastMessageAt.Equal(last.At) {
		t.Errorf("last_message_at = %s, want the instant of the last recorded event %s", record.LastMessageAt, last.At)
	}
	if len(record.Events) != 3 {
		t.Errorf("the record carries %d events, want the whole transcript of 3", len(record.Events))
	}
}

// TestJournalDoesNotRewriteAThreadWithNothingNewToSay is the reason the journal
// keeps a watermark: the reading route is polled for as long as a conversation
// lives, and a journal that rewrote an unchanged history on every poll would
// rewrite it hundreds of times.
func TestJournalDoesNotRewriteAThreadWithNothingNewToSay(t *testing.T) {
	srv, provider := journalTestServer(t)
	id := journalTestOpenedID(t, srv)

	provider.emit(t, id, localrun.KindUserMessage, "una cosa sola")
	if status, _, body := readConversation(t, srv, id, 0); status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}
	before, beforeInfo := journalTestReadFile(t, srv, id)

	for range 2 {
		if status, _, body := readConversation(t, srv, id, 0); status != http.StatusOK {
			t.Fatalf("GET conversation = %d, want 200: %s", status, body)
		}
	}

	after, afterInfo := journalTestReadFile(t, srv, id)
	if string(after) != string(before) {
		t.Errorf("two reads with nothing new rewrote the record:\nbefore: %s\nafter:  %s", before, after)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Errorf("the record was rewritten at %s, want it untouched since %s", afterInfo.ModTime(), beforeInfo.ModTime())
	}
}

// TestOpeningAConversationOnAnUnknownSpecWritesNothing is the other half of
// AC-2: a binding that names a spec this backlog does not hold is refused
// before any process exists, so it leaves neither a record nor a conversation.
func TestOpeningAConversationOnAnUnknownSpecWritesNothing(t *testing.T) {
	srv, provider := journalTestServer(t)

	status, body := journalTestOpenWith(t, srv, map[string]any{"spec_code": journalTestMissingSpecCode})
	if status != http.StatusConflict {
		t.Fatalf("POST conversation on an unknown spec = %d, want 409: %s", status, body)
	}
	if refused := refusalMessage(t, body); !strings.Contains(refused, journalTestMissingSpecCode) {
		t.Errorf("the refusal says %q, want it to name %q", refused, journalTestMissingSpecCode)
	}
	if files := journalTestFiles(t, srv); len(files) != 0 {
		t.Errorf("%s holds %v after a refused open, want nothing", journalTestDir(t, srv), files)
	}
	if openings := provider.openings(); len(openings) != 0 {
		t.Errorf("the provider was asked to open %d conversations for a spec that does not exist", len(openings))
	}

	if live := liveConversationIDs(srv); len(live) != 0 {
		t.Errorf("a conversation is open after a refused binding: %v", live)
	}
}

// TestOpeningAConversationOnASpecBindsTheThreadToIt is AC-2: a conversation
// born from a spec carries that code, on disk and in the payload, so the index
// can say which spec it is about.
func TestOpeningAConversationOnASpecBindsTheThreadToIt(t *testing.T) {
	srv, _ := journalTestServer(t)

	status, body := journalTestOpenWith(t, srv, map[string]any{"spec_code": journalTestSpecCode})
	if status != http.StatusCreated {
		t.Fatalf("POST conversation on %s = %d, want 201: %s", journalTestSpecCode, status, body)
	}
	view := journalTestDecodeBound(t, body)
	if view.Conversation == nil {
		t.Fatalf("the open conversation is null: %s", body)
	}
	if view.Conversation.SpecCode != journalTestSpecCode {
		t.Errorf("the payload reports spec_code %q, want %q: %s", view.Conversation.SpecCode, journalTestSpecCode, body)
	}
	record := journalTestRecordOf(t, srv, view.Conversation.ID)
	if record.SpecCode != journalTestSpecCode {
		t.Errorf("record spec_code = %q, want %q", record.SpecCode, journalTestSpecCode)
	}
}

// TestAConversationIsFreeUnlessASpecIsAskedFor is AC-5: the reviewer can start
// a conversation tied to nothing, and that is the default — an open with no
// body and one with an empty code are the same request.
func TestAConversationIsFreeUnlessASpecIsAskedFor(t *testing.T) {
	cases := []struct {
		name    string
		payload any
	}{
		{"no body at all", nil},
		{"an empty spec code", map[string]any{"spec_code": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := journalTestServer(t)

			status, body := journalTestOpenWith(t, srv, tc.payload)
			if status != http.StatusCreated {
				t.Fatalf("POST conversation = %d, want 201: %s", status, body)
			}
			view := journalTestDecodeBound(t, body)
			if view.Conversation == nil {
				t.Fatalf("the open conversation is null: %s", body)
			}
			if view.Conversation.SpecCode != "" {
				t.Errorf("the payload binds a free conversation to %q: %s", view.Conversation.SpecCode, body)
			}
			record := journalTestRecordOf(t, srv, view.Conversation.ID)
			if record.SpecCode != "" {
				t.Errorf("record spec_code = %q, want it empty for a free conversation", record.SpecCode)
			}
		})
	}
}

// TestClosingAConversationSealsItsRecord is AC-3: what a reader reopens later
// is the whole conversation as it ended, with the state it was left in — so the
// last words said before the close are on disk, and the record says it is over.
func TestClosingAConversationSealsItsRecord(t *testing.T) {
	srv, provider := journalTestServer(t)
	id := journalTestOpenedID(t, srv)

	provider.emit(t, id, localrun.KindUserMessage, "l'ultima domanda")
	provider.emit(t, id, localrun.KindText, "l'ultima risposta")

	status, _, body := closeConversation(t, srv, id)
	if status != http.StatusOK {
		t.Fatalf("DELETE conversation = %d, want 200: %s", status, body)
	}

	record := journalTestRecordOf(t, srv, id)
	if record.FinalState != string(execution.RunClosed) {
		t.Errorf("final_state = %q, want %q", record.FinalState, execution.RunClosed)
	}
	if len(record.Events) != 2 {
		t.Fatalf("the sealed record carries %d events, want the 2 said before the close: %#v", len(record.Events), record.Events)
	}
	if record.Events[0].Text != "l'ultima domanda" || record.Events[1].Text != "l'ultima risposta" {
		t.Errorf("the sealed record holds %q and %q, want what was said in order", record.Events[0].Text, record.Events[1].Text)
	}
	if record.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", record.MessageCount)
	}
}

// TestJournalRefusesEventsOfAnotherConversation pins the identity check on
// record().
//
// The race it stands for is real: a reader takes a snapshot of one live
// conversation, then reads its session outside the journal's lock, and in
// between another one can be begun. Without the lookup those stale events would
// be written into the *other* record — its history, its title and its instant
// overwritten by somebody else's — and the watermark would be pushed past
// events that conversation has not emitted yet, so its own history could never
// be written at all.
func TestJournalRefusesEventsOfAnotherConversation(t *testing.T) {
	root := t.TempDir()
	journal, err := newConversationJournal(root)
	if err != nil {
		t.Fatalf("newConversationJournal: %v", err)
	}
	ctx := context.Background()
	openedAt := journalTestInstant(t, "2026-08-22T10:00:00Z")

	old := conversationSnapshot{id: "conv-old", workingDir: root, openedAt: openedAt}
	if err := journal.begin(ctx, old, "", ""); err != nil {
		t.Fatalf("begin(old): %v", err)
	}
	staleEvents := []execution.RunEvent{
		{ID: 7, Kind: localrun.KindUserMessage, Text: "detto nella vecchia", At: openedAt.Add(time.Minute)},
	}
	if err := journal.record(ctx, "conv-old", staleEvents, true); err != nil {
		t.Fatalf("record(old): %v", err)
	}

	fresh := conversationSnapshot{id: "conv-new", workingDir: root, openedAt: openedAt.Add(time.Hour)}
	if err := journal.begin(ctx, fresh, "", "conv-old"); err != nil {
		t.Fatalf("begin(new): %v", err)
	}
	// The in-flight reader arrives now, still holding the old conversation's
	// history. The journal is keeping the new one.
	if err := journal.record(ctx, "conv-old", staleEvents, true); err != nil {
		t.Fatalf("record(stale into new): %v", err)
	}

	written, err := journal.store.Get(ctx, "conv-new")
	if err != nil {
		t.Fatalf("Get(conv-new): %v", err)
	}
	if len(written.Events) != 0 {
		t.Fatalf("the new record absorbed %d event(s) of the old conversation: %#v", len(written.Events), written.Events)
	}
	if strings.Contains(written.Title, "detto nella vecchia") {
		t.Errorf("the new record was titled from the old conversation: %q", written.Title)
	}

	// And the watermark did not move, so the new conversation can still write
	// its own history — including events with ids below the stale ones.
	ownEvents := []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "detto nella nuova", At: openedAt.Add(2 * time.Hour)},
	}
	if err := journal.record(ctx, "conv-new", ownEvents, true); err != nil {
		t.Fatalf("record(new): %v", err)
	}
	written, err = journal.store.Get(ctx, "conv-new")
	if err != nil {
		t.Fatalf("Get(conv-new) after its own events: %v", err)
	}
	if len(written.Events) != 1 || written.Events[0].Text != "detto nella nuova" {
		t.Fatalf("the new record holds %#v, want only what was said in it", written.Events)
	}

	// The old record kept what was said in it, untouched by any of the above.
	kept, err := journal.store.Get(ctx, "conv-old")
	if err != nil {
		t.Fatalf("Get(conv-old): %v", err)
	}
	if len(kept.Events) != 1 || kept.Events[0].Text != "detto nella vecchia" {
		t.Fatalf("the old record holds %#v, want the single event said in it", kept.Events)
	}
}

// journalTestInstant parses an instant the test writes in full, so the
// assertions above read as moments rather than as offsets from time.Now().
func journalTestInstant(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}
	return parsed.UTC()
}

// TestJournalKeepsTwoConversationsInTwoRecords is AC-1 and AC-2 at the level of
// the journal: two conversations journalled together produce two files, and
// nothing said in one reaches the other.
func TestJournalKeepsTwoConversationsInTwoRecords(t *testing.T) {
	root := t.TempDir()
	journal, err := newConversationJournal(root)
	if err != nil {
		t.Fatalf("newConversationJournal: %v", err)
	}
	ctx := context.Background()
	openedAt := journalTestInstant(t, "2026-08-22T10:00:00Z")

	a := conversationSnapshot{id: "conv-a", workingDir: root, openedAt: openedAt}
	b := conversationSnapshot{id: "conv-b", workingDir: root, openedAt: openedAt.Add(time.Minute)}
	if err := journal.begin(ctx, a, "US-001", ""); err != nil {
		t.Fatalf("begin(conv-a): %v", err)
	}
	if err := journal.begin(ctx, b, "US-002", ""); err != nil {
		t.Fatalf("begin(conv-b): %v", err)
	}

	eventsA := []execution.RunEvent{{ID: 1, Kind: localrun.KindUserMessage, Text: "detto in A", At: openedAt.Add(time.Minute)}}
	eventsB := []execution.RunEvent{{ID: 1, Kind: localrun.KindUserMessage, Text: "detto in B", At: openedAt.Add(2 * time.Minute)}}
	if err := journal.record(ctx, "conv-a", eventsA, true); err != nil {
		t.Fatalf("record(conv-a): %v", err)
	}
	if err := journal.record(ctx, "conv-b", eventsB, true); err != nil {
		t.Fatalf("record(conv-b): %v", err)
	}

	recordA, err := journal.store.Get(ctx, "conv-a")
	if err != nil {
		t.Fatalf("Get(conv-a): %v", err)
	}
	recordB, err := journal.store.Get(ctx, "conv-b")
	if err != nil {
		t.Fatalf("Get(conv-b): %v", err)
	}
	if len(recordA.Events) != 1 || recordA.Events[0].Text != "detto in A" {
		t.Fatalf("conv-a holds %#v, want only what was said in it", recordA.Events)
	}
	if len(recordB.Events) != 1 || recordB.Events[0].Text != "detto in B" {
		t.Fatalf("conv-b holds %#v, want only what was said in it", recordB.Events)
	}
	if recordA.SpecCode != "US-001" || recordB.SpecCode != "US-002" {
		t.Fatalf("spec codes = (%q, %q), want (US-001, US-002)", recordA.SpecCode, recordB.SpecCode)
	}
}

// TestJournalDoesNotLetOneTitleContaminateTheOther is AC-2 on the name of a
// thread: each conversation is named by how *it* started, and a second message
// in one renames neither.
func TestJournalDoesNotLetOneTitleContaminateTheOther(t *testing.T) {
	root := t.TempDir()
	journal, err := newConversationJournal(root)
	if err != nil {
		t.Fatalf("newConversationJournal: %v", err)
	}
	ctx := context.Background()
	openedAt := journalTestInstant(t, "2026-08-22T10:00:00Z")
	if err := journal.begin(ctx, conversationSnapshot{id: "conv-a", workingDir: root, openedAt: openedAt}, "", ""); err != nil {
		t.Fatalf("begin(conv-a): %v", err)
	}
	if err := journal.begin(ctx, conversationSnapshot{id: "conv-b", workingDir: root, openedAt: openedAt}, "", ""); err != nil {
		t.Fatalf("begin(conv-b): %v", err)
	}

	if err := journal.record(ctx, "conv-a", []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "titolo di A", At: openedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("record(conv-a): %v", err)
	}
	if err := journal.record(ctx, "conv-b", []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "titolo di B", At: openedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("record(conv-b): %v", err)
	}
	if err := journal.record(ctx, "conv-a", []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "titolo di A", At: openedAt.Add(time.Minute)},
		{ID: 2, Kind: localrun.KindUserMessage, Text: "seconda cosa detta in A", At: openedAt.Add(2 * time.Minute)},
	}, true); err != nil {
		t.Fatalf("record(conv-a) again: %v", err)
	}

	recordA, err := journal.store.Get(ctx, "conv-a")
	if err != nil {
		t.Fatalf("Get(conv-a): %v", err)
	}
	recordB, err := journal.store.Get(ctx, "conv-b")
	if err != nil {
		t.Fatalf("Get(conv-b): %v", err)
	}
	if recordA.Title != "titolo di A" {
		t.Fatalf("conv-a title = %q, want the first thing said in it", recordA.Title)
	}
	if recordB.Title != "titolo di B" {
		t.Fatalf("conv-b title = %q, want the first thing said in it", recordB.Title)
	}
}

// TestJournalWatermarkOfOneConversationDoesNotBlockTheOther is the regression a
// single shared lastWrittenID would cause: a conversation whose events carry
// high ids would silently stop a sibling with lower ones from ever being
// written.
func TestJournalWatermarkOfOneConversationDoesNotBlockTheOther(t *testing.T) {
	root := t.TempDir()
	journal, err := newConversationJournal(root)
	if err != nil {
		t.Fatalf("newConversationJournal: %v", err)
	}
	ctx := context.Background()
	openedAt := journalTestInstant(t, "2026-08-22T10:00:00Z")
	if err := journal.begin(ctx, conversationSnapshot{id: "conv-high", workingDir: root, openedAt: openedAt}, "", ""); err != nil {
		t.Fatalf("begin(conv-high): %v", err)
	}
	if err := journal.begin(ctx, conversationSnapshot{id: "conv-low", workingDir: root, openedAt: openedAt}, "", ""); err != nil {
		t.Fatalf("begin(conv-low): %v", err)
	}

	if err := journal.record(ctx, "conv-high", []execution.RunEvent{
		{ID: 900, Kind: localrun.KindUserMessage, Text: "detto molto avanti", At: openedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("record(conv-high): %v", err)
	}
	if err := journal.record(ctx, "conv-low", []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "detto all'inizio", At: openedAt.Add(2 * time.Minute)},
	}, true); err != nil {
		t.Fatalf("record(conv-low): %v", err)
	}

	low, err := journal.store.Get(ctx, "conv-low")
	if err != nil {
		t.Fatalf("Get(conv-low): %v", err)
	}
	if len(low.Events) != 1 || low.Events[0].Text != "detto all'inizio" {
		t.Fatalf("conv-low holds %#v, want its own event: a watermark of another conversation must not block it", low.Events)
	}
}

// TestJournalFinishSealsOnlyTheConversationItNames is AC-4 on disk: sealing one
// conversation stops later writes into *it* and leaves every other one being
// journalled exactly as it was.
func TestJournalFinishSealsOnlyTheConversationItNames(t *testing.T) {
	root := t.TempDir()
	journal, err := newConversationJournal(root)
	if err != nil {
		t.Fatalf("newConversationJournal: %v", err)
	}
	ctx := context.Background()
	openedAt := journalTestInstant(t, "2026-08-22T10:00:00Z")
	if err := journal.begin(ctx, conversationSnapshot{id: "conv-a", workingDir: root, openedAt: openedAt}, "", ""); err != nil {
		t.Fatalf("begin(conv-a): %v", err)
	}
	if err := journal.begin(ctx, conversationSnapshot{id: "conv-b", workingDir: root, openedAt: openedAt}, "", ""); err != nil {
		t.Fatalf("begin(conv-b): %v", err)
	}
	if err := journal.record(ctx, "conv-a", []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "detto in A", At: openedAt.Add(time.Minute)},
	}, true); err != nil {
		t.Fatalf("record(conv-a): %v", err)
	}

	if err := journal.finish(ctx, "conv-a", execution.RunClosed); err != nil {
		t.Fatalf("finish(conv-a): %v", err)
	}
	// Idempotent on an id it is no longer keeping, which is what lets a route
	// and the ending session both seal the same conversation.
	if err := journal.finish(ctx, "conv-a", execution.RunClosed); err != nil {
		t.Fatalf("finish(conv-a) twice: %v", err)
	}
	if err := journal.finish(ctx, "conv-unknown", execution.RunClosed); err != nil {
		t.Fatalf("finish on an unknown id should be innocuous, got %v", err)
	}

	sealed, err := journal.store.Get(ctx, "conv-a")
	if err != nil {
		t.Fatalf("Get(conv-a): %v", err)
	}
	if sealed.FinalState != string(execution.RunClosed) {
		t.Fatalf("conv-a final_state = %q, want %q", sealed.FinalState, execution.RunClosed)
	}

	// A late read of the sealed conversation rewrites nothing: what is sealed is
	// history.
	if err := journal.record(ctx, "conv-a", []execution.RunEvent{
		{ID: 2, Kind: localrun.KindUserMessage, Text: "arrivato dopo la chiusura", At: openedAt.Add(time.Hour)},
	}, true); err != nil {
		t.Fatalf("record(conv-a) after finish: %v", err)
	}
	again, err := journal.store.Get(ctx, "conv-a")
	if err != nil {
		t.Fatalf("Get(conv-a) after the late write: %v", err)
	}
	if len(again.Events) != 1 || again.Events[0].Text != "detto in A" {
		t.Fatalf("conv-a holds %#v after being sealed, want only what was said before", again.Events)
	}

	// And the conversation still being journalled goes on being written.
	if err := journal.record(ctx, "conv-b", []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "detto in B", At: openedAt.Add(2 * time.Hour)},
	}, true); err != nil {
		t.Fatalf("record(conv-b) after conv-a was sealed: %v", err)
	}
	recordB, err := journal.store.Get(ctx, "conv-b")
	if err != nil {
		t.Fatalf("Get(conv-b): %v", err)
	}
	if len(recordB.Events) != 1 || recordB.Events[0].Text != "detto in B" {
		t.Fatalf("conv-b holds %#v, want its own event: sealing a sibling must not stop it", recordB.Events)
	}
	if recordB.FinalState != "" {
		t.Fatalf("conv-b final_state = %q, want none: it was never sealed", recordB.FinalState)
	}
}
