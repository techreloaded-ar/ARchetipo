package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
