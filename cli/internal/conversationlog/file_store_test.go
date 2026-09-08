package conversationlog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func conversationsDir(root string) string {
	return filepath.Join(root, ".archetipo", "conversations")
}

func newStore(t *testing.T, root string) *FileStore {
	t.Helper()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func saveRecord(t *testing.T, store *FileStore, record Record) {
	t.Helper()
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

// AC-1: a conversation written by one store is read back whole by a different
// store built on the same root, so nothing survives only in memory.
func TestFileStoreRoundTripsAWholeConversationAcrossStores(t *testing.T) {
	root := t.TempDir()
	writer := newStore(t, root)
	base := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	want := Record{
		ID:            "conv-one",
		SpecCode:      "US-058",
		Title:         "Ritrovare le conversazioni passate",
		WorkingDir:    root,
		ProviderID:    "claude-code",
		OpenedAt:      base,
		LastMessageAt: base.Add(3 * time.Minute),
		MessageCount:  3,
		ResumedFrom:   "conv-zero",
		FinalState:    "released",
		Events: []execution.RunEvent{
			{ID: 1, Seq: 1, At: base, Kind: "user_message", Text: "come sta il workspace?"},
			{ID: 2, Seq: 2, At: base.Add(time.Minute), Kind: "tool_use", Tool: "Read", Text: "cli/internal/conversationlog/file_store.go", Raw: json.RawMessage(`{"path":"file_store.go"}`)},
			{ID: 3, Seq: 3, At: base.Add(3 * time.Minute), Kind: "assistant_message", Text: "lo store scrive un file per conversazione"},
		},
	}
	saveRecord(t, writer, want)

	reader := newStore(t, root)
	got, err := reader.Get(context.Background(), want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Title != want.Title || got.SpecCode != want.SpecCode || got.ResumedFrom != want.ResumedFrom {
		t.Fatalf("identity lost in round trip: %#v", got)
	}
	if got.WorkingDir != want.WorkingDir || got.ProviderID != want.ProviderID || got.FinalState != want.FinalState || got.MessageCount != want.MessageCount {
		t.Fatalf("metadata lost in round trip: %#v", got)
	}
	if !got.OpenedAt.Equal(want.OpenedAt) || !got.LastMessageAt.Equal(want.LastMessageAt) {
		t.Fatalf("instants lost in round trip: opened=%v last=%v", got.OpenedAt, got.LastMessageAt)
	}
	if len(got.Events) != len(want.Events) {
		t.Fatalf("expected %d events, got %d", len(want.Events), len(got.Events))
	}
	for i, event := range got.Events {
		expected := want.Events[i]
		if event.ID != expected.ID || event.Seq != expected.Seq || event.Kind != expected.Kind || event.Text != expected.Text || event.Tool != expected.Tool {
			t.Fatalf("event %d differs: got %#v want %#v", i, event, expected)
		}
	}

	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(conversationsDir(root))
		if err != nil {
			t.Fatal(err)
		}
		fileInfo, err := os.Stat(filepath.Join(conversationsDir(root), "conv-one.json"))
		if err != nil {
			t.Fatal(err)
		}
		if dirInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("modes dir=%o file=%o", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
		}
	}
}

// Saving the same conversation twice replaces it in place: the journal rewrites
// a record on every round, and a second file would be a second history.
func TestFileStoreSaveReplacesTheRecordInPlace(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	base := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	saveRecord(t, store, Record{ID: "conv-one", Title: "prima stesura", LastMessageAt: base, MessageCount: 1})
	saveRecord(t, store, Record{
		ID:            "conv-one",
		Title:         "seconda stesura",
		LastMessageAt: base.Add(time.Hour),
		MessageCount:  4,
		Events:        []execution.RunEvent{{ID: 9, Seq: 1, At: base, Kind: "assistant_message", Text: "aggiornato"}},
	})

	entries, err := os.ReadDir(conversationsDir(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one record file, got %d", len(entries))
	}
	got, err := store.Get(context.Background(), "conv-one")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "seconda stesura" || got.MessageCount != 4 || len(got.Events) != 1 {
		t.Fatalf("second save did not win: %#v", got)
	}
}

// AC-2/AC-3: the list is ordered by recency so the conversation last talked to
// is the first one offered, with the id breaking ties for a stable order.
func TestFileStoreListReturnsMostRecentFirst(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	base := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	saveRecord(t, store, Record{ID: "conv-old", LastMessageAt: base})
	saveRecord(t, store, Record{ID: "conv-new", LastMessageAt: base.Add(2 * time.Hour)})
	saveRecord(t, store, Record{ID: "conv-mid", LastMessageAt: base.Add(time.Hour)})

	records, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	if strings.Join(ids, ",") != "conv-new,conv-mid,conv-old" {
		t.Fatalf("wrong recency order: %v", ids)
	}
}

func TestFileStoreListBreaksTiesByDescendingID(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	same := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	saveRecord(t, store, Record{ID: "conv-a", LastMessageAt: same})
	saveRecord(t, store, Record{ID: "conv-b", LastMessageAt: same})

	records, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != "conv-b" || records[1].ID != "conv-a" {
		t.Fatalf("wrong tie break: %#v", records)
	}
}

// AC-7: a workspace nobody has talked to yet answers with an empty list, never
// with a failure — and never with a nil slice a JSON encoder would turn to null.
func TestFileStoreListOnUntouchedWorkspaceIsEmptyNotNil(t *testing.T) {
	store := newStore(t, t.TempDir())
	records, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("absent directory reported as failure: %v", err)
	}
	if records == nil {
		t.Fatal("expected a non-nil empty slice")
	}
	if len(records) != 0 {
		t.Fatalf("expected no records, got %d", len(records))
	}
}

func TestFileStoreGetUnknownIDIsTypedNotFound(t *testing.T) {
	store := newStore(t, t.TempDir())
	_, err := store.Get(context.Background(), "never-saved")
	var storeErr *StoreError
	if !errors.As(err, &storeErr) || storeErr.Kind != StoreNotFound {
		t.Fatalf("expected a typed not-found, got %v", err)
	}
}

// An id is a file name and only a file name: nothing that could point outside
// the workspace directory is accepted, and a rejected id writes nothing.
func TestFileStoreRejectsIDsThatCouldEscapeTheDirectory(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	for _, id := range []string{"", "..", "../escape", "nested/conv", `nested\conv`} {
		err := store.Save(context.Background(), Record{ID: id})
		var storeErr *StoreError
		if !errors.As(err, &storeErr) || storeErr.Kind != StoreInvalidID {
			t.Fatalf("Save accepted invalid id %q: %v", id, err)
		}
		if _, err := store.Get(context.Background(), id); !errors.As(err, &storeErr) || storeErr.Kind != StoreInvalidID {
			t.Fatalf("Get accepted invalid id %q: %v", id, err)
		}
	}
	if entries, err := os.ReadDir(conversationsDir(root)); err == nil && len(entries) != 0 {
		t.Fatalf("rejected ids left %d files behind", len(entries))
	}
}

// A history silently missing one conversation is indistinguishable from a
// complete one, so a damaged record fails the scan and names the file.
func TestFileStoreListFailsNamingTheDamagedFile(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	saveRecord(t, store, Record{ID: "conv-one", LastMessageAt: time.Now().UTC()})
	if err := os.WriteFile(filepath.Join(conversationsDir(root), "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := store.List(context.Background())
	if err == nil {
		t.Fatal("damaged record accepted")
	}
	if !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("error does not name the damaged file: %v", err)
	}
}

// Delete erases the record and only that one, and an id the store never wrote
// comes back as a typed not-found: "there is nothing to erase" and "the erase
// failed" are opposite answers, and the route above tells them apart.
func TestFileStoreDeleteRemovesOnlyTheNamedRecord(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	saveRecord(t, store, Record{ID: "conv-one", LastMessageAt: time.Now().UTC()})
	saveRecord(t, store, Record{ID: "conv-two", LastMessageAt: time.Now().UTC()})

	if err := store.Delete(context.Background(), "conv-one"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	records, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "conv-two" {
		t.Fatalf("after Delete the store holds %#v, want only conv-two", records)
	}

	var storeErr *StoreError
	err = store.Delete(context.Background(), "conv-one")
	if !errors.As(err, &storeErr) || storeErr.Kind != StoreNotFound {
		t.Fatalf("deleting an absent record = %v, want not_found", err)
	}
	err = store.Delete(context.Background(), "../escape")
	if !errors.As(err, &storeErr) || storeErr.Kind != StoreInvalidID {
		t.Fatalf("Delete accepted an invalid id: %v", err)
	}
}

func TestNativeRecordKeepsMetadataAtomicAndPaginatesMoreThanTwoThousandEvents(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	record := Record{
		Version: CurrentVersion,
		ID:      "conv-native",
		Session: &execution.SessionMetadata{
			ConversationID: "conv-native",
			ProviderID:     "fake",
			Environment:    execution.SessionEnvironment{WorkingDir: root, Location: "local"},
			Native:         execution.NativeSessionReference{Kind: "fake.thread", ID: "native-1"},
		},
		Archive: execution.ArchiveOpen,
	}
	saveRecord(t, store, record)
	events := make([]execution.RunEvent, 2105)
	for i := range events {
		events[i] = execution.RunEvent{ID: int64(i + 1), Kind: "text", Text: strconv.Itoa(i + 1)}
	}
	if err := store.AppendEvents(context.Background(), record.ID, events); err != nil {
		t.Fatal(err)
	}

	firstPage, err := store.ReadEvents(context.Background(), record.ID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := store.ReadEvents(context.Background(), record.ID, firstPage.LastID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	thirdPage, err := store.ReadEvents(context.Background(), record.ID, secondPage.LastID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Events) != 1000 || !firstPage.HasMore || firstPage.LastID != 1000 {
		t.Fatalf("first page = %#v", firstPage)
	}
	if len(secondPage.Events) != 1000 || !secondPage.HasMore || secondPage.Events[0].ID != 1001 {
		t.Fatalf("second page has %d events, first=%d, more=%v", len(secondPage.Events), secondPage.Events[0].ID, secondPage.HasMore)
	}
	if len(thirdPage.Events) != 105 || thirdPage.HasMore || thirdPage.Events[0].ID != 2001 || thirdPage.LastID != 2105 {
		t.Fatalf("third page = %#v", thirdPage)
	}
	metadataBody, err := os.ReadFile(filepath.Join(conversationsDir(root), record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadataBody), "\"events\"") {
		t.Fatalf("native metadata embeds its timeline: %s", metadataBody)
	}
	got, err := newStore(t, root).Get(context.Background(), record.ID)
	if err != nil || len(got.Events) != len(events) || got.Session.Native.ID != "native-1" {
		t.Fatalf("Get after restart = events %d, record %#v, err %v", len(got.Events), got, err)
	}
}

func TestAppendEventsIsIdempotentAndRejectsGaps(t *testing.T) {
	store := newStore(t, t.TempDir())
	saveRecord(t, store, Record{Version: CurrentVersion, ID: "conv-native", Session: &execution.SessionMetadata{
		ConversationID: "conv-native", Native: execution.NativeSessionReference{ID: "native-1"},
	}})
	events := []execution.RunEvent{{ID: 1, Text: "one"}, {ID: 2, Text: "two"}}
	if err := store.AppendEvents(context.Background(), "conv-native", events); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvents(context.Background(), "conv-native", events); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if err := store.AppendEvents(context.Background(), "conv-native", []execution.RunEvent{{ID: 4}}); err == nil {
		t.Fatal("event gap accepted")
	}
	page, err := store.ReadEvents(context.Background(), "conv-native", 0, 10)
	if err != nil || len(page.Events) != 2 {
		t.Fatalf("events after replay = %#v, %v", page, err)
	}
}

func TestAppendEventsRecoversAnIncompleteCrashTail(t *testing.T) {
	root := t.TempDir()
	store := newStore(t, root)
	saveRecord(t, store, Record{Version: CurrentVersion, ID: "conv-native", Session: &execution.SessionMetadata{
		ConversationID: "conv-native", Native: execution.NativeSessionReference{ID: "native-1"},
	}})
	if err := store.AppendEvents(context.Background(), "conv-native", []execution.RunEvent{{ID: 1, Text: "complete"}}); err != nil {
		t.Fatal(err)
	}
	eventsPath, err := store.eventsPath("conv-native")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"id":2,"text":"cut`); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err := store.AppendEvents(context.Background(), "conv-native", []execution.RunEvent{{ID: 2, Text: "recovered"}}); err != nil {
		t.Fatalf("append after crash tail: %v", err)
	}
	page, err := store.ReadEvents(context.Background(), "conv-native", 0, 10)
	if err != nil || len(page.Events) != 2 || page.Events[1].Text != "recovered" {
		t.Fatalf("recovered events = %#v, %v", page, err)
	}
}

func TestConversationLockRecoversADeadOwnerAndSerializesAConversation(t *testing.T) {
	store := newStore(t, t.TempDir())
	lock, err := store.Lock(context.Background(), "conv-one")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := store.Lock(ctx, "conv-one"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock = %v, want deadline", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	path, err := store.path("conv-dead")
	if err != nil {
		t.Fatal(err)
	}
	deadLock := strings.TrimSuffix(path, ".json") + ".lock"
	if err := os.MkdirAll(deadLock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deadLock, "owner"), []byte("999999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Lock(context.Background(), "conv-dead")
	if err != nil {
		t.Fatalf("dead owner was not recovered: %v", err)
	}
	_ = recovered.Unlock()
}
