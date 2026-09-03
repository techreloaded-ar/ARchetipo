package conversationlog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
