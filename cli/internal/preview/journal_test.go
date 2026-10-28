package preview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
)

func TestInterruptedPreviewIsVisibleAndUnknownServerIsPreserved(t *testing.T) {
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("unrelated")) }))
	defer server.Close()
	state, err := newServerState(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveServerState(root, state); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Close()
	view, err := manager.Status(root, "recovered")
	if err != nil || view.State != "error" || view.Server == nil || view.Server.OwnerActive || view.CleanupError == "" {
		t.Fatalf("lost interrupted preview evidence: %+v %v", view, err)
	}
	if err = manager.StopEnvironment(root, "recovered"); err == nil {
		t.Fatal("declared a listening server stopped without its owner")
	}
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal("unknown listener was stopped", err)
	}
	_ = response.Body.Close()
	server.Close()
	if err = manager.StopEnvironment(root, "recovered"); err != nil {
		t.Fatal(err)
	}
	reconciled, err := readServerState(root)
	if err != nil || reconciled.State != "stopped" || reconciled.RunID != state.RunID || reconciled.Error != "" {
		t.Fatal("shutdown was not retained", reconciled, err)
	}
}

func TestRetainedStopRequestCannotTargetANewRun(t *testing.T) {
	root := t.TempDir()
	first, err := newServerState("http://127.0.0.1:1234")
	if err != nil {
		t.Fatal(err)
	}
	second, err := newServerState(first.URL)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(serverStopRequest{RunID: first.RunID})
	if err = os.MkdirAll(filepath.Join(root, ".archetipo", "environment"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, serverStopPath), request, 0o600); err != nil {
		t.Fatal(err)
	}
	if serverStopRequested(root, second.RunID) || !serverStopRequested(root, first.RunID) {
		t.Fatal("stop request did not identify its original run")
	}
	first.State = "stopped"
	if err = saveServerState(root, first); err != nil {
		t.Fatal(err)
	}
	instance := &Instance{runID: second.RunID}
	instance.commandEnded(root, os.ErrNotExist)
	if instance.Snapshot().State != "error" {
		t.Fatal("an older successful stop hid the newer command's startup failure")
	}
}

func TestRecoveryRefusesLiveSupervisorLease(t *testing.T) {
	root := t.TempDir()
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := store.TryLock(context.Background(), "preview-command")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	if err = ReconcileStopped(root); err == nil {
		t.Fatal("reconciled while a supervisor was still owning the command tree")
	}
}
