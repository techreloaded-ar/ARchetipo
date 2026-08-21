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

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// serverOnWorkspace builds a viewer that serves a real workspace on disk and is
// able to start actions on it: the provider is registered and persisted as the
// workspace default, exactly as a person configures it from the UI.
//
// It exists next to serverOn because these tests need what serverOn leaves out
// — a provider registry — and next to newRunServer because they need what that
// one leaves out: a project root that is a real workspace of its own, so two of
// them can be told apart by the directory a run executes in.
func serverOnWorkspace(t *testing.T, root string, provider execution.Provider) *Server {
	t.Helper()
	registry := execution.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if _, err := config.UpdateDefaultProvider(root, config.DefaultProviderConfig{ID: provider.ID(), Config: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadExact(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ProjectRoot = root
	srv, err := NewServer(filefs.New(cfg), cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.session().dispatch.wait(5 * time.Second) })
	return srv
}

// executionWorkingDir reads back the working directory of a record through the
// very route the browser polls, so the assertion is on what a client can see
// and not on an internal value.
func executionWorkingDir(t *testing.T, srv *Server, id string) string {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/execution/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/execution/%s: %d %s", id, w.Code, w.Body.String())
	}
	var record struct {
		WorkingDir string `json:"working_dir"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	return record.WorkingDir
}

// startedID starts an action and returns the id of the record it created.
func startedID(t *testing.T, srv *Server, code, action string) string {
	t.Helper()
	status, body := startAction(t, srv, code, action)
	if status != http.StatusCreated {
		t.Fatalf("POST /api/spec/%s/execution = %d: %v", code, status, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("the started record has no id: %v", body)
	}
	return id
}

// AC-1: the run executes in the project root of the workspace that is open, and
// says so on the record the browser reads — not in the directory the viewer
// process happens to have been launched from.
func TestRunExecutesInTheProjectRootOfTheOpenWorkspace(t *testing.T) {
	b := realWorkspace(t, "beta", "US-B01", false)
	provider := releasedProvider("fake", nil)
	srv := serverOnWorkspace(t, b, provider)

	id := startedID(t, srv, "US-B01", "plan")
	srv.session().dispatch.wait(5 * time.Second)

	requests := provider.requests()
	if len(requests) != 1 {
		t.Fatalf("the provider received %d dispatches, want 1", len(requests))
	}
	if requests[0].WorkingDir != b {
		t.Fatalf("the run was dispatched to run in %q, want the project root of the open workspace %q", requests[0].WorkingDir, b)
	}
	if got := executionWorkingDir(t, srv, id); got != b {
		t.Fatalf("the record served to the browser reports %q, want %q", got, b)
	}
}

// AC-2: after another workspace is opened, the first run started afterwards
// executes in the newly opened project root — on the same server object, which
// is the in-process form of "without restarting the process".
func TestFirstRunAfterOpeningAnotherWorkspaceUsesTheNewRoot(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	provider := releasedProvider("fake", nil)
	srv := serverOnWorkspace(t, a, provider)
	// B has to be configured too: opening it means serving its own configuration.
	if _, err := config.UpdateDefaultProvider(b, config.DefaultProviderConfig{ID: provider.ID(), Config: map[string]any{}}); err != nil {
		t.Fatal(err)
	}

	before := srv
	if err := srv.SwitchWorkspace(b); err != nil {
		t.Fatalf("SwitchWorkspace(B) = %v, want nil", err)
	}
	if srv != before {
		t.Fatal("the viewer was rebuilt: the run has to follow the workspace on the same process")
	}

	id := startedID(t, srv, "US-B01", "plan")
	srv.session().dispatch.wait(5 * time.Second)

	requests := provider.requests()
	if len(requests) != 1 {
		t.Fatalf("the provider received %d dispatches, want 1", len(requests))
	}
	if requests[0].WorkingDir != b {
		t.Fatalf("the first run after the switch was dispatched to %q, want the newly opened root %q", requests[0].WorkingDir, b)
	}
	if got := executionWorkingDir(t, srv, id); got != b {
		t.Fatalf("the record reports %q, want %q", got, b)
	}
}

// AC-3: a run already in flight when another workspace is opened keeps the root
// it was started with. The provider is held inside Execute across the switch,
// so the assertion is about a run that really was running at that moment.
func TestRunInFlightKeepsItsRootAcrossAWorkspaceSwitch(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	inFlight := blockedProvider("fake")
	srv := serverOnWorkspace(t, b, inFlight)
	if _, err := config.UpdateDefaultProvider(a, config.DefaultProviderConfig{ID: inFlight.ID(), Config: map[string]any{}}); err != nil {
		t.Fatal(err)
	}

	id := startedID(t, srv, "US-B01", "plan")
	select {
	case <-inFlight.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the provider was never dispatched")
	}
	previous := srv.session()

	// The switch drains the workspace it leaves, so the held run is released
	// from another goroutine: what is under test is the root the run carries,
	// not who unblocks it.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(inFlight.release)
	}()
	if err := srv.SwitchWorkspace(a); err != nil {
		t.Fatalf("SwitchWorkspace(A) = %v, want nil", err)
	}
	previous.dispatch.wait(5 * time.Second)

	requests := inFlight.requests()
	if len(requests) != 1 || requests[0].WorkingDir != b {
		t.Fatalf("the in-flight run carries %#v, want a single dispatch on %q", requests, b)
	}
	record, err := previous.store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if record.WorkingDir != b {
		t.Fatalf("the closed record of the in-flight run reports %q, want the root it started with %q", record.WorkingDir, b)
	}

	// And a run started after the switch carries the newly opened root, so the
	// two runs are told apart by where each of them executes.
	afterID := startedID(t, srv, "US-A01", "plan")
	srv.session().dispatch.wait(5 * time.Second)
	if got := executionWorkingDir(t, srv, afterID); got != a {
		t.Fatalf("the run started after the switch reports %q, want %q", got, a)
	}
}

// renameAway moves a directory out of the way and gives back the function that
// puts it back, which is how the story's demonstration is reproduced: the
// session keeps pointing at the path it was opened on.
func renameAway(t *testing.T, dir string) func() {
	t.Helper()
	moved := dir + "-renamed"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		if err := os.Rename(moved, dir); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(restore)
	return restore
}

// AC-4: with the project root gone, both start routes refuse with a message
// that names the directory, and no execution is created at all.
func TestStartIsRefusedWhenTheProjectRootIsUnreachable(t *testing.T) {
	t.Run("spec route", func(t *testing.T) {
		b := realWorkspace(t, "beta", "US-B01", false)
		provider := releasedProvider("fake", nil)
		srv := serverOnWorkspace(t, b, provider)
		statusBefore := runSpecDetail(t, srv, "US-B01").Spec.Status

		restore := renameAway(t, b)
		status, body := startAction(t, srv, "US-B01", "plan")
		if status != http.StatusConflict {
			t.Fatalf("POST /api/spec/US-B01/execution = %d: %v", status, body)
		}
		message, _ := body["error"].(string)
		if !strings.Contains(message, b) {
			t.Fatalf("the refusal does not name the directory: %q", message)
		}
		if len(provider.requests()) != 0 {
			t.Fatalf("a refused start reached the provider: %#v", provider.requests())
		}

		restore()
		if got := recordFileCount(t, b, "US-B01"); got != 0 {
			t.Fatalf("a refused start created %d records", got)
		}
		if got := runSpecDetail(t, srv, "US-B01").Spec.Status; got != statusBefore {
			t.Fatalf("a refused start moved the spec from %q to %q", statusBefore, got)
		}
		// The refusal held no reservation: with the directory back, the same spec
		// starts. If the guard had reserved and not released, this would be a 409.
		_ = startedID(t, srv, "US-B01", "plan")
		srv.session().dispatch.wait(5 * time.Second)
	})

	t.Run("workspace route", func(t *testing.T) {
		b := realWorkspace(t, "beta", "US-B01", false)
		provider := releasedProvider("fake", nil)
		srv := serverOnWorkspace(t, b, provider)

		renameAway(t, b)
		status, body := startWorkspaceAction(t, srv, string(execution.ActionInception))
		if status != http.StatusConflict {
			t.Fatalf("POST /api/workspace/execution = %d: %v", status, body)
		}
		message, _ := body["error"].(string)
		if !strings.Contains(message, b) {
			t.Fatalf("the refusal does not name the directory: %q", message)
		}
		if len(provider.requests()) != 0 {
			t.Fatalf("a refused start reached the provider: %#v", provider.requests())
		}
	})
}

// The guard reads the root the session is serving, so a directory that exists
// but is no longer a workspace is refused with the same vocabulary the workspace
// switch already uses for it.
func TestStartRefusalNamesTheCauseTheSwitchWouldName(t *testing.T) {
	b := realWorkspace(t, "beta", "US-B01", false)
	provider := releasedProvider("fake", nil)
	srv := serverOnWorkspace(t, b, provider)

	if err := os.RemoveAll(filepath.Join(b, ".archetipo")); err != nil {
		t.Fatal(err)
	}
	status, body := startAction(t, srv, "US-B01", "plan")
	if status != http.StatusConflict {
		t.Fatalf("POST /api/spec/US-B01/execution = %d: %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, b) || !strings.Contains(message, "not a workspace") {
		t.Fatalf("the refusal does not name the directory and the cause: %q", message)
	}
}
