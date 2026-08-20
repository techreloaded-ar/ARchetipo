package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// realWorkspace creates a workspace the viewer can actually open: a directory
// with `.archetipo/config.yaml` and a backlog holding one recognisable spec
// code. The connector is the real file one, because the oracle of these tests
// is the backlog that gets served — a fake connector would hide exactly what
// the acceptance criteria assert.
func realWorkspace(t *testing.T, name, specCode string, withPRD bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.RelativePath), []byte("connector: file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := filefs.New(cfg)
	ctx := context.Background()
	if _, err := conn.SaveInitialBacklog(ctx, []domain.Spec{{
		Code:     specCode,
		Title:    name + " story",
		Epic:     domain.Epic{Code: "EP-001", Title: "F"},
		Priority: domain.PriorityHigh,
		Points:   3,
		Status:   domain.StatusTodo,
	}}); err != nil {
		t.Fatal(err)
	}
	if withPRD {
		if _, err := conn.SavePRD(ctx, "# PRD of "+name+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// serverOn builds a viewer serving one real workspace, with the same wiring
// SwitchWorkspace will rebuild.
func serverOn(t *testing.T, root string) *Server {
	t.Helper()
	cfg, err := config.LoadExact(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ProjectRoot = root
	srv, err := NewServer(filefs.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

// boardCodes returns every spec code the board currently serves.
func boardCodes(t *testing.T, srv *Server) map[string]bool {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/board = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var view boardView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the board: %v", err)
	}
	codes := map[string]bool{}
	for _, col := range view.Columns {
		for _, spec := range col.Specs {
			codes[spec.Code] = true
		}
	}
	return codes
}

// configPath returns the configuration file path the viewer reports, which is
// the shortest honest answer to "which workspace are you serving".
func configPath(t *testing.T, srv *Server) string {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var view configView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the config: %v", err)
	}
	return view.Path
}

func specStatus(t *testing.T, srv *Server, code string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/spec/"+code, nil))
	return rec.Code
}

// TestSwitchWorkspaceServesNewBacklog covers AC-1, AC-2 and AC-3 at the level
// where they are decided: the routes, on one process that was never restarted.
func TestSwitchWorkspaceServesNewBacklog(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	srv := serverOn(t, a)

	if codes := boardCodes(t, srv); !codes["US-A01"] {
		t.Fatalf("the board of A does not contain US-A01: %v", codes)
	}
	if got := configPath(t, srv); !strings.HasPrefix(got, a) {
		t.Fatalf("config path = %q, want it under %q", got, a)
	}

	if err := srv.SwitchWorkspace(b); err != nil {
		t.Fatalf("SwitchWorkspace(B) = %v, want nil", err)
	}

	codes := boardCodes(t, srv)
	if !codes["US-B01"] {
		t.Errorf("the board does not contain US-B01 after the switch: %v", codes)
	}
	if codes["US-A01"] {
		t.Errorf("the board still contains US-A01 after the switch: %v", codes)
	}
	if got := specStatus(t, srv, "US-A01"); got != http.StatusNotFound {
		t.Errorf("GET /api/spec/US-A01 = %d, want 404", got)
	}
	if got := configPath(t, srv); !strings.HasPrefix(got, b) {
		t.Errorf("config path = %q, want it under %q", got, b)
	}
}

// TestSwitchWorkspaceRejectsUnreachable is AC-4 stated from the observer's
// side: what matters is not that the call returned an error, but that A is
// still being served afterwards.
func TestSwitchWorkspaceRejectsUnreachable(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)

	notADirectory := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plainDir := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		path   string
		reason string
	}{
		{"missing directory", filepath.Join(t.TempDir(), "gone"), "no longer exists"},
		{"path is a file", notADirectory, "not a directory"},
		{"directory is not a workspace", plainDir, "not a workspace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := serverOn(t, a)
			err := srv.SwitchWorkspace(tc.path)
			if err == nil {
				t.Fatal("SwitchWorkspace = nil, want an error")
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("error = %q, want it to name %q", err.Error(), tc.reason)
			}
			if codes := boardCodes(t, srv); !codes["US-A01"] {
				t.Errorf("A is no longer served after the refusal: %v", codes)
			}
			if got := configPath(t, srv); !strings.HasPrefix(got, a) {
				t.Errorf("config path = %q, want it still under %q", got, a)
			}
		})
	}
}

// TestSwitchWorkspaceRejectsInvalidConfig covers the case the probe cannot
// catch: the file is there, and it is broken.
func TestSwitchWorkspaceRejectsInvalidConfig(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	broken := filepath.Join(t.TempDir(), "broken")
	if err := os.MkdirAll(filepath.Join(broken, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, config.RelativePath), []byte("connector: [file\n  bad: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := serverOn(t, a)
	err := srv.SwitchWorkspace(broken)
	if err == nil {
		t.Fatal("SwitchWorkspace on a broken configuration = nil, want an error")
	}
	if !strings.Contains(err.Error(), config.RelativePath) {
		t.Errorf("error = %q, want it to name %q", err.Error(), config.RelativePath)
	}
	if codes := boardCodes(t, srv); !codes["US-A01"] {
		t.Errorf("A is no longer served after the refusal: %v", codes)
	}
}

// TestSwitchWorkspaceProbesBeforeLoadingConfig pins decision 4 of the plan:
// LoadExact walks up the tree, so a directory nested inside a workspace but not
// a workspace itself must be refused, not silently resolved to its parent.
func TestSwitchWorkspaceProbesBeforeLoadingConfig(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	nested := filepath.Join(a, "docs", "notes")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	b := realWorkspace(t, "beta", "US-B01", false)

	srv := serverOn(t, b)
	err := srv.SwitchWorkspace(nested)
	if err == nil {
		t.Fatal("a directory inside a workspace is not a workspace: want an error")
	}
	if !strings.Contains(err.Error(), "not a workspace") {
		t.Errorf("error = %q, want it to name the probed reason", err.Error())
	}
	if codes := boardCodes(t, srv); !codes["US-B01"] {
		t.Errorf("B is no longer served after the refusal: %v", codes)
	}
}

// TestSwitchWorkspaceToSameRootIsNoop keeps reopening the current workspace
// harmless: it must not tear down the work in flight.
func TestSwitchWorkspaceToSameRootIsNoop(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	srv := serverOn(t, a)
	before := srv.session()

	if err := srv.SwitchWorkspace(a); err != nil {
		t.Fatalf("SwitchWorkspace on the current root = %v, want nil", err)
	}
	if srv.session() != before {
		t.Error("reopening the current workspace rebuilt the session")
	}
}

// TestSwitchWorkspaceUnderConcurrentRequests is the guard on the locking
// decision: without it, reading the session would be a convention rather than
// an invariant. A response mixing codes of both workspaces would mean a request
// read half of each.
func TestSwitchWorkspaceUnderConcurrentRequests(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	srv := serverOn(t, a)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rec := httptest.NewRecorder()
				srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board", nil))
				if rec.Code != http.StatusOK {
					mu.Lock()
					failures = append(failures, rec.Body.String())
					mu.Unlock()
					return
				}
				var view boardView
				if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
					mu.Lock()
					failures = append(failures, err.Error())
					mu.Unlock()
					return
				}
				sawA, sawB := false, false
				for _, col := range view.Columns {
					for _, spec := range col.Specs {
						switch spec.Code {
						case "US-A01":
							sawA = true
						case "US-B01":
							sawB = true
						}
					}
				}
				if sawA && sawB {
					mu.Lock()
					failures = append(failures, "one board mixed the codes of both workspaces")
					mu.Unlock()
					return
				}
			}
		}()
	}

	for _, root := range []string{b, a, b, a} {
		if err := srv.SwitchWorkspace(root); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("SwitchWorkspace(%s) = %v, want nil", root, err)
		}
	}
	close(stop)
	wg.Wait()

	if len(failures) > 0 {
		t.Fatalf("concurrent requests failed during the switch: %v", failures)
	}
}

// TestDispatchRunAfterDrainDoesNotPanic pins the fix for the window a workspace
// switch opened: stopping a session drains its dispatch group, and a request
// that was already inside the handler can still reach run afterwards. Before
// the guard that was Add racing Wait on the same WaitGroup, which panics the
// whole viewer.
func TestDispatchRunAfterDrainDoesNotPanic(t *testing.T) {
	group := newDispatchGroup()
	ctx, cancel := context.WithCancel(context.Background())
	group.bind(ctx)
	cancel()
	group.wait(time.Second)

	ran := make(chan struct{})
	group.run(func(dispatchCtx context.Context) {
		// The late dispatch still runs, on the cancelled context, so a
		// continuation closes its record instead of leaving it RUNNING.
		if dispatchCtx.Err() == nil {
			t.Error("a dispatch started after the drain must run on a cancelled context")
		}
		close(ran)
	})
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("the late dispatch never ran")
	}
	group.wait(time.Second)
}

// TestSwitchWorkspaceLeavesTheNewWorkspaceRunnable is the observable form of
// the invariant that a reservation is taken and released on one session: a
// spec busy on the workspace being left must not look busy on the one being
// opened, or the new workspace would refuse that action for the whole life of
// the process.
func TestSwitchWorkspaceLeavesTheNewWorkspaceRunnable(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	srv := serverOn(t, a)

	previous := srv.session()
	if _, reserved := previous.dispatch.reserve("US-A01"); !reserved {
		t.Fatal("the spec should have been free on the workspace being left")
	}

	if err := srv.SwitchWorkspace(b); err != nil {
		t.Fatalf("SwitchWorkspace(B) = %v, want nil", err)
	}

	if id, busy := srv.session().dispatch.current("US-A01"); busy {
		t.Errorf("the opened workspace inherited a reservation (%q): the guard and the handler are reading different sessions", id)
	}
	previous.dispatch.release("US-A01")
}

// TestSwitchWorkspaceUnderConcurrentActionRequests is the concurrency guard on
// the read paths that resolve the session more than once per request:
// /api/workspace/actions goes through the template, the availability and the
// latest execution. A response is only coherent if all three saw one workspace.
func TestSwitchWorkspaceUnderConcurrentActionRequests(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	srv := serverOn(t, a)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, route := range []string{"/api/workspace/actions", "/api/spec/US-A01", "/api/spec/US-B01"} {
					rec := httptest.NewRecorder()
					srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, route, nil))
					if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
						mu.Lock()
						failures = append(failures, route+": "+rec.Body.String())
						mu.Unlock()
						return
					}
				}
			}
		}()
	}

	for _, root := range []string{b, a, b, a} {
		if err := srv.SwitchWorkspace(root); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("SwitchWorkspace(%s) = %v, want nil", root, err)
		}
	}
	close(stop)
	wg.Wait()

	if len(failures) > 0 {
		t.Fatalf("concurrent requests failed during the switch: %v", failures)
	}
}

// TestSwitchWorkspaceRebuildsTheExecutionService covers the switch on a server
// that actually has a provider registry, which every other switch test lacks:
// the new session must have its own service, bound to the new store.
func TestSwitchWorkspaceRebuildsTheExecutionService(t *testing.T) {
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)

	cfg, err := config.LoadExact(a)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ProjectRoot = a
	srv, err := NewServer(filefs.New(cfg), cfg, execution.NewRegistry(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	before := srv.session()
	if before.service == nil {
		t.Fatal("the initial session has no execution service")
	}
	if err := srv.SwitchWorkspace(b); err != nil {
		t.Fatalf("SwitchWorkspace(B) = %v, want nil", err)
	}
	after := srv.session()
	if after.service == nil {
		t.Error("the opened workspace has no execution service")
	}
	if after.service == before.service || after.store == before.store {
		t.Error("the opened workspace reuses the service or the store of the workspace it replaced")
	}
	if codes := boardCodes(t, srv); !codes["US-B01"] {
		t.Errorf("board = %v, want the codes of B", codes)
	}
}

// TestRunFollowersEnsureAfterCloseStartsNothing is the follower twin of
// TestDispatchRunAfterDrainDoesNotPanic: a request already inside the run route
// when its workspace is left must not be able to open a stream towards the hub
// that nothing in this process could ever close.
func TestRunFollowersEnsureAfterCloseStartsNothing(t *testing.T) {
	followers := newRunFollowers()
	followers.closeAll()

	got := followers.ensure(context.Background(), "EX-1", "RUN-1", nil, nil)
	if got != nil {
		t.Fatal("a closed follower set started a new follower")
	}
	if _, ok := followers.get("EX-1"); ok {
		t.Error("a closed follower set recorded a new follower")
	}
}
