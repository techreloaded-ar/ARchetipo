package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// TestNewServerBuildsSessionForConfiguredWorkspace pins the invariant the whole
// switch rests on: the server holds one session, and that session describes the
// workspace the server was built for.
func TestNewServerBuildsSessionForConfiguredWorkspace(t *testing.T) {
	srv, cfg := newFileServer(t)

	ws := srv.session()
	if ws == nil {
		t.Fatal("the server has no session")
	}
	if ws.cfg.ProjectRoot != cfg.ProjectRoot {
		t.Errorf("session root = %q, want %q", ws.cfg.ProjectRoot, cfg.ProjectRoot)
	}
	if ws.conn == nil {
		t.Error("the session has no connector")
	}
	if ws.store == nil {
		t.Error("the session has no execution store")
	}
	if ws.dispatch == nil || ws.followers == nil {
		t.Error("the session has no dispatch group or no followers")
	}
	root := filepath.Clean(cfg.ProjectRoot)
	for name, dir := range map[string]string{"mockupsDir": ws.mockupsDir, "watchRoot": ws.watchRoot} {
		if dir == "" {
			t.Errorf("%s is empty", name)
			continue
		}
		if !strings.HasPrefix(filepath.Clean(dir), root) {
			t.Errorf("%s = %q, want it inside %q", name, dir, root)
		}
	}
}

// TestMockupsRouteResolvesThroughSession proves the mockups route reads the
// directory from the session on every request instead of from a directory
// frozen when the routes were registered.
func TestMockupsRouteResolvesThroughSession(t *testing.T) {
	srv, cfg := newFileServer(t)
	dir := cfg.AbsPath(cfg.Paths.Mockups)
	if err := os.MkdirAll(filepath.Join(dir, "app-home"), 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "body{}"
	if err := os.WriteFile(filepath.Join(dir, "app-home", "app.css"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mockups/app-home/app.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /mockups/app-home/app.css = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}

	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mockups/nope/app.css", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET of a missing mockup = %d, want 404", rec.Code)
	}
}

// TestSessionWithoutProviderRegistryHasNoService keeps the nil service explicit:
// it is what makes the run route answer "no provider" instead of panicking.
func TestSessionWithoutProviderRegistryHasNoService(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir

	without, err := NewServer(filefs.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if without.session().service != nil {
		t.Error("a server with no provider registry must have no execution service")
	}

	with, err := NewServer(filefs.New(cfg), cfg, execution.NewRegistry(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if with.session().service == nil {
		t.Error("a server with a provider registry must have an execution service")
	}
}

// TestSessionStopCancelsDispatch proves stopping a session really ends the work
// in flight of that session, rather than merely dropping the reference to it.
func TestSessionStopCancelsDispatch(t *testing.T) {
	srv, _ := newFileServer(t)
	ws := srv.session()
	ws.start(context.Background(), srv.broker)

	done := make(chan struct{})
	ws.dispatch.run(func(ctx context.Context) {
		<-ctx.Done()
		close(done)
	})

	ws.stop(2 * time.Second)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopping the session did not cancel its dispatch context")
	}
}

// TestSessionStopIsIdempotent covers the ordinary sequence of a switch followed
// by a shutdown: the session that was left is stopped twice.
func TestSessionStopIsIdempotent(t *testing.T) {
	srv, _ := newFileServer(t)
	ws := srv.session()
	ws.start(context.Background(), srv.broker)

	returned := make(chan struct{})
	go func() {
		ws.stop(time.Second)
		ws.stop(time.Second)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("a second stop blocked")
	}
}

// TestServerShutdownStopsSession is the regression guard on shutdown: Run must
// still serve, still return nil on cancellation, and still stop the session it
// started.
func TestServerShutdownStopsSession(t *testing.T) {
	srv, conn := newTestServer(t)
	seedSpecs(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan string, 1)
	runErr := make(chan error, 1)
	go func() { runErr <- srv.Run(ctx, func(url string) { ready <- url }) }()

	var url string
	select {
	case url = <-ready:
	case err := <-runErr:
		t.Fatalf("the server stopped before being ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the server never became ready")
	}

	resp, err := http.Get(url + "/api/board")
	if err != nil {
		t.Fatalf("GET /api/board: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/board = %d, want 200", resp.StatusCode)
	}

	blocked := make(chan struct{})
	srv.session().dispatch.run(func(ctx context.Context) {
		<-ctx.Done()
		close(blocked)
	})

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not stop the session's dispatches")
	}
}
