package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFileStore(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	running := Execution{ID: "exec-one", SpecCode: "US-001", Status: StatusRunning, CreatedAt: now}
	if err := store.Create(context.Background(), running); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), running); err == nil {
		t.Fatal("exclusive create accepted duplicate")
	} else {
		var se *StoreError
		if !errors.As(err, &se) || se.Kind != StoreAlreadyExist {
			t.Fatalf("wrong duplicate error: %v", err)
		}
	}
	completed := now.Add(time.Second)
	running.Status = StatusSucceeded
	running.Result = &Result{Payload: json.RawMessage(`{"ok":true}`)}
	running.CompletedAt = &completed
	if err := store.Update(context.Background(), running); err != nil {
		t.Fatal(err)
	}
	reader, _ := NewFileStore(root)
	got, err := reader.Get(context.Background(), running.ID)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]bool
	if got.Result != nil {
		_ = json.Unmarshal(got.Result.Payload, &payload)
	}
	if got.Status != StatusSucceeded || got.Result == nil || !payload["ok"] {
		t.Fatalf("round trip: %#v", got)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".archetipo", "executions"))
	if len(entries) != 1 {
		t.Fatalf("expected one record, got %d", len(entries))
	}
	if runtime.GOOS != "windows" {
		dirInfo, _ := os.Stat(filepath.Join(root, ".archetipo", "executions"))
		fileInfo, _ := os.Stat(filepath.Join(root, ".archetipo", "executions", "exec-one.json"))
		if dirInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("modes dir=%o file=%o", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
		}
	}
	for _, id := range []string{"", "../escape", "a/b"} {
		if _, err := store.Get(context.Background(), id); err == nil {
			t.Fatalf("invalid id %q accepted", id)
		}
	}
	if _, err := store.Get(context.Background(), "missing"); err == nil {
		t.Fatal("missing record accepted")
	}
	if err := os.WriteFile(filepath.Join(root, ".archetipo", "executions", "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), "broken"); err == nil {
		t.Fatal("corrupt record accepted")
	}
	missing := Execution{ID: "missing"}
	if err := store.Update(context.Background(), missing); err == nil {
		t.Fatal("update missing accepted")
	}
}

func TestFileStoreFailedRoundTrip(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	now := time.Now().UTC()
	exec := Execution{ID: "exec-failed", Status: StatusRunning, CreatedAt: now}
	if err := store.Create(context.Background(), exec); err != nil {
		t.Fatal(err)
	}
	exec.Status = StatusFailed
	exec.Error = &ExecutionError{Code: "PROVIDER_ERROR", Message: "boom"}
	exec.CompletedAt = &now
	if err := store.Update(context.Background(), exec); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), exec.ID)
	if err != nil || got.Error == nil || got.Result != nil {
		t.Fatalf("failed round trip: %#v %v", got, err)
	}
}

func writeRecord(t *testing.T, store *FileStore, record Execution) {
	t.Helper()
	if err := store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

// AC-5: a client that kept no identifier finds the history of its spec, newest
// first, and never sees another spec's records.
func TestFileStoreListBySpecReturnsOnlyThatSpecNewestFirst(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	writeRecord(t, store, Execution{ID: "exec-old", SpecCode: "US-001", Status: StatusFailed, CreatedAt: base})
	writeRecord(t, store, Execution{ID: "exec-mid", SpecCode: "US-001", Status: StatusSucceeded, CreatedAt: base.Add(time.Minute)})
	writeRecord(t, store, Execution{ID: "exec-new", SpecCode: "US-001", Status: StatusRunning, CreatedAt: base.Add(2 * time.Minute)})
	writeRecord(t, store, Execution{ID: "exec-other", SpecCode: "US-002", Status: StatusRunning, CreatedAt: base.Add(3 * time.Minute)})

	got, err := store.ListBySpec(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, record := range got {
		ids = append(ids, record.ID)
	}
	want := []string{"exec-new", "exec-mid", "exec-old"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("order: got %v, want %v", ids, want)
	}
	if got[0].Status != StatusRunning {
		t.Fatalf("the most recent record lost its state: %#v", got[0])
	}
}

func TestFileStoreListBySpecTreatsAbsenceAsAnEmptyList(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ListBySpec(context.Background(), "US-001")
	if err != nil {
		t.Fatalf("a spec with no execution is not a failure: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("absence must be an empty non-nil slice: %#v", got)
	}
}

// A truncated history is indistinguishable from an absent one, so a record that
// cannot be decoded must fail the read and name the file.
func TestFileStoreListBySpecFailsNamingTheUnreadableRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	writeRecord(t, store, Execution{ID: "exec-good", SpecCode: "US-001", Status: StatusSucceeded, CreatedAt: time.Now().UTC()})
	if err := os.WriteFile(filepath.Join(root, ".archetipo", "executions", "exec-broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListBySpec(context.Background(), "US-001")
	if err == nil {
		t.Fatalf("a corrupt record produced a partial list: %#v", got)
	}
	if !strings.Contains(err.Error(), "exec-broken.json") {
		t.Fatalf("the error does not name the file: %v", err)
	}
	if got != nil {
		t.Fatalf("a failed read still returned records: %#v", got)
	}
}

func TestFileStoreListBySpecIgnoresNonRecordFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	writeRecord(t, store, Execution{ID: "exec-one", SpecCode: "US-001", Status: StatusSucceeded, CreatedAt: time.Now().UTC()})
	dir := filepath.Join(root, ".archetipo", "executions")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a record"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".execution-123.tmp"), []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListBySpec(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "exec-one" {
		t.Fatalf("foreign files leaked into the result: %#v", got)
	}
}

// US-040: an empty spec code is not a wildcard, it is the workspace. A
// workspace-scoped execution never appears in the history of a spec, and a
// spec's executions never appear in the workspace history.
func TestFileStoreListBySpecKeepsWorkspaceAndSpecExecutionsApart(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	writeRecord(t, store, Execution{ID: "exec-spec", SpecCode: "US-901", Action: ActionPlan, Capability: CapabilitySpecPlan, Status: StatusSucceeded, CreatedAt: base})
	writeRecord(t, store, Execution{ID: "exec-workspace", SpecCode: "", Action: ActionInception, Capability: CapabilityWorkspaceInception, Status: StatusRunning, CreatedAt: base.Add(time.Minute)})

	spec, err := store.ListBySpec(context.Background(), "US-901")
	if err != nil {
		t.Fatal(err)
	}
	if len(spec) != 1 || spec[0].ID != "exec-spec" {
		t.Fatalf("the spec history is not only its own: %#v", spec)
	}

	workspace, err := store.ListBySpec(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace) != 1 || workspace[0].ID != "exec-workspace" || workspace[0].Action != ActionInception {
		t.Fatalf("the workspace history is not only its own: %#v", workspace)
	}
	if workspace[0].Status != StatusRunning {
		t.Fatalf("the workspace record lost its state: %#v", workspace[0])
	}

	// A blank spec code is the same read as the empty one, not a third history.
	blank, err := store.ListBySpec(context.Background(), "   ")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(blank, workspace) {
		t.Fatalf("a padded empty spec code produced a different history: %#v", blank)
	}

	// A spec that never ran is empty even though a workspace execution exists.
	other, err := store.ListBySpec(context.Background(), "US-902")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("an unrelated spec inherited records: %#v", other)
	}
}

func recordIDs(records []Execution) []string {
	var ids []string
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

// AC-3: the workspace run list is built on an enumeration that hides nothing —
// every record, whatever its scope, newest first.
func TestFileStoreListReturnsEveryRecordNewestFirst(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)
	writeRecord(t, store, Execution{ID: "exec-spec-one", SpecCode: "US-001", Status: StatusSucceeded, CreatedAt: base})
	writeRecord(t, store, Execution{ID: "exec-workspace", SpecCode: "", Action: ActionInception, Status: StatusRunning, CreatedAt: base.Add(time.Minute)})
	writeRecord(t, store, Execution{ID: "exec-spec-two", SpecCode: "US-002", Status: StatusRunning, CreatedAt: base.Add(2 * time.Minute)})

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"exec-spec-two", "exec-workspace", "exec-spec-one"}
	if !reflect.DeepEqual(recordIDs(got), want) {
		t.Fatalf("order: got %v, want %v", recordIDs(got), want)
	}
	if got[1].SpecCode != "" || got[1].Action != ActionInception {
		t.Fatalf("the workspace record lost its scope: %#v", got[1])
	}
}

func TestFileStoreListBreaksTiesById(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	same := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	writeRecord(t, store, Execution{ID: "exec-aaa", SpecCode: "US-001", Status: StatusRunning, CreatedAt: same})
	writeRecord(t, store, Execution{ID: "exec-bbb", SpecCode: "US-001", Status: StatusRunning, CreatedAt: same})

	got, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"exec-bbb", "exec-aaa"}
	if !reflect.DeepEqual(recordIDs(got), want) {
		t.Fatalf("two records written in the same instant came out unordered: got %v, want %v", recordIDs(got), want)
	}
}

func TestFileStoreListTreatsAbsenceAsAnEmptyList(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("a workspace that never ran anything is not a failure: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("absence must be an empty non-nil slice: %#v", got)
	}
}

func TestFileStoreListFailsNamingTheUnreadableRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	writeRecord(t, store, Execution{ID: "exec-good", SpecCode: "US-001", Status: StatusSucceeded, CreatedAt: time.Now().UTC()})
	if err := os.WriteFile(filepath.Join(root, ".archetipo", "executions", "exec-broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.List(context.Background())
	if err == nil {
		t.Fatalf("a corrupt record produced a partial list: %#v", got)
	}
	if !strings.Contains(err.Error(), "exec-broken.json") {
		t.Fatalf("the error does not name the file: %v", err)
	}
	if got != nil {
		t.Fatalf("a failed read still returned records: %#v", got)
	}
}

func TestFileStoreListIgnoresNonRecordFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	writeRecord(t, store, Execution{ID: "exec-one", SpecCode: "US-001", Status: StatusSucceeded, CreatedAt: time.Now().UTC()})
	dir := filepath.Join(root, ".archetipo", "executions")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a record"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "archive.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recordIDs(got), []string{"exec-one"}) {
		t.Fatalf("foreign entries leaked into the result: %#v", got)
	}
}
