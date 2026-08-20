// Package web serves the local Kanban viewer for the ARchetipo backlog.
//
// The server exposes a small JSON API on top of the existing connector and
// serves a single-page UI from assets embedded in the binary. It is intended
// for local single-user use: it binds to 127.0.0.1 by default and ships no
// authentication.
package web

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// Server wires the connector backend to HTTP handlers and the embedded UI.
//
// Everything that depends on which project root is being served lives in the
// current workspaceSession, reachable only through session(). The fields kept
// here are the ones that must survive a workspace switch: the execution
// provider registry, the SSE broker whose clients stay connected, the registry
// of known workspaces, and the HTTP plumbing itself.
type Server struct {
	registry *execution.Registry
	mux      *http.ServeMux
	httpSrv  *http.Server
	broker   *Broker

	// workspaces is the user-level registry of known workspaces. It is named
	// apart from `registry` above, which is the execution provider registry of
	// this viewer: two different registries, one field each. It may be nil — a
	// machine whose state directory cannot be resolved must still get a working
	// viewer, so the routes degrade instead of the constructor failing.
	workspaces *workspace.Registry

	// mu guards cur. Handlers take it for the length of one pointer read, so a
	// switch never observes a half-migrated server and a request never observes
	// half of each workspace.
	mu  sync.RWMutex
	cur *workspaceSession

	// switchMu serialises SwitchWorkspace. It is separate from mu because a
	// switch spans building and starting a session, which must not hold the lock
	// every handler takes.
	switchMu sync.Mutex

	// baseCtx is the context Run is serving on, kept so that a session created
	// after start-up is anchored to the same tree. Before Run it is nil, which
	// start reads as Background.
	baseCtx context.Context

	// OnWorkspaceSwitch is called with the new project root after a successful
	// switch. It exists so the process that owns the viewer entry — `archetipo
	// view` — can keep that entry truthful; nil is the normal case.
	OnWorkspaceSwitch func(root string)
}

// session returns the workspace the server is serving right now. Handlers call
// it once, at the top, and use the returned value for the whole request: that
// single read is what makes a request see one workspace and not a mixture.
func (s *Server) session() *workspaceSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// NewServer constructs a Server bound to addr (e.g. "127.0.0.1:8080").
// The returned server has all routes registered but is not listening yet:
// call Run to start serving. cfg is used to resolve the on-disk location of
// design mockups served under /mockups/. registry is the execution provider
// registry the viewer offers for selection; a nil registry simply offers no
// provider, which is not an error.
func NewServer(conn connector.Connector, cfg config.Config, registry *execution.Registry, addr string) (*Server, error) {
	mux := http.NewServeMux()
	session, err := newWorkspaceSession(cfg, conn, registry)
	if err != nil {
		return nil, err
	}
	s := &Server{
		registry: registry,
		mux:      mux,
		broker:   NewBroker(),
		cur:      session,
	}
	// A registry we cannot open is not a reason to refuse to serve the current
	// workspace: the field stays nil and the workspace routes say so.
	if reg, regErr := workspace.OpenRegistry(); regErr == nil {
		s.workspaces = reg
	}
	s.registerRoutes()
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// Addr returns the address the server listens on.
func (s *Server) Addr() string { return s.httpSrv.Addr }

// dispatchDrainTimeout bounds how long shutdown waits for in-flight dispatches.
// It is a window, not a promise: an execution the provider is still polling gets
// its context cancelled, closes its record as FAILED, and is done well inside
// it; a provider that ignores cancellation must not hold the process hostage.
const dispatchDrainTimeout = 5 * time.Second

// Run starts listening and blocks until ctx is done or the server errors.
// When ctx is cancelled the server is shut down with a 5s grace period.
func (s *Server) Run(ctx context.Context, onReady func(url string)) error {
	s.mu.Lock()
	s.baseCtx = ctx
	s.mu.Unlock()

	// The session owns the context its dispatches and its watcher run on, so
	// stopping it is what turns an interrupted execution into a FAILED record
	// with a reason instead of one left RUNNING for ever.
	s.session().start(ctx, s.broker)
	defer func() { s.session().stop(dispatchDrainTimeout) }()

	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.httpSrv.Addr, err)
	}
	// Capture the resolved port (in case Addr was ":0").
	s.httpSrv.Addr = ln.Addr().String()
	if onReady != nil {
		onReady("http://" + s.httpSrv.Addr)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		s.broker.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		s.broker.Close()
		return err
	}
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/board", s.handleGetBoard)
	s.mux.HandleFunc("GET /api/board/stream", s.handleStreamBoard)
	s.mux.HandleFunc("GET /api/metrics", s.handleGetMetrics)
	s.mux.HandleFunc("GET /api/spec/{code}", s.handleGetSpec)
	s.mux.HandleFunc("POST /api/spec", s.handleCreateSpec)
	s.mux.HandleFunc("PUT /api/spec/{code}", s.handleUpdateSpec)
	s.mux.HandleFunc("DELETE /api/spec/{code}", s.handleDeleteSpec)
	s.mux.HandleFunc("PUT /api/spec/{code}/plan", s.handleSavePlan)
	s.mux.HandleFunc("POST /api/board/move", s.handleMoveCard)
	s.mux.HandleFunc("GET /api/spec/{code}/diff", s.handleGetDiff)
	s.mux.HandleFunc("GET /api/spec/{code}/review", s.handleGetReview)
	s.mux.HandleFunc("PUT /api/spec/{code}/review", s.handleSaveReview)
	s.mux.HandleFunc("POST /api/spec/{code}/request-changes", s.handleRequestChanges)
	s.mux.HandleFunc("POST /api/spec/{code}/approve", s.handleApprove)
	s.mux.HandleFunc("POST /api/spec/{code}/integrate", s.handleIntegrate)
	s.mux.HandleFunc("GET /api/prd", s.handleGetPRD)
	s.mux.HandleFunc("PUT /api/prd", s.handleSavePRD)
	s.mux.HandleFunc("GET /api/execution/providers", s.handleListExecutionProviders)
	s.mux.HandleFunc("PUT /api/execution/provider/default", s.handleSaveDefaultExecutionProvider)
	s.mux.HandleFunc("GET /api/execution/model-choice", s.handleGetExecutionModelChoice)
	s.mux.HandleFunc("POST /api/spec/{code}/execution", s.handleRunSpecAction)
	s.mux.HandleFunc("GET /api/execution/{id}", s.handleGetExecution)
	s.mux.HandleFunc("GET /api/execution/{id}/run", s.handleGetExecutionRun)
	s.mux.HandleFunc("POST /api/execution/{id}/run/messages", s.handleSendRunMessage)
	s.mux.HandleFunc("POST /api/execution/{id}/run/approvals/{approvalId}", s.handleRespondRunApproval)
	s.mux.HandleFunc("POST /api/execution/{id}/run/cancel", s.handleCancelRun)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handleSaveConfig)
	s.mux.HandleFunc("POST /api/config/test", s.handleTestConfig)
	s.mux.HandleFunc("GET /api/workspace/options", s.handleGetWorkspaceOptions)
	s.mux.HandleFunc("POST /api/workspace", s.handleCreateWorkspace)
	s.mux.HandleFunc("GET /api/workspace/actions", s.handleGetWorkspaceActions)
	s.mux.HandleFunc("GET /api/workspace/status", s.handleGetWorkspaceStatus)
	s.mux.HandleFunc("POST /api/workspace/execution", s.handleRunWorkspaceAction)
	s.mux.HandleFunc("GET /api/workspaces", s.handleListWorkspaces)
	s.mux.HandleFunc("POST /api/workspaces", s.handleAddWorkspace)
	s.mux.HandleFunc("DELETE /api/workspaces/{id}", s.handleRemoveWorkspace)
	s.mux.HandleFunc("POST /api/workspaces/{id}/open", s.handleOpenWorkspace)
	s.mux.HandleFunc("GET /api/mockups", s.handleListMockups)

	// Serve design mockups from the configured paths.mockups directory.
	// The directory is resolved per request rather than frozen at registration,
	// because after a workspace switch a frozen one would keep serving the
	// mockups of the workspace that was left.
	s.mux.HandleFunc("/mockups/", func(w http.ResponseWriter, r *http.Request) {
		dir := s.session().mockupsDir
		if dir == "" {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix("/mockups/", http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
	})

	// Static assets (HTML/CSS/JS + vendor). Served from the embedded FS.
	assets, err := fs.Sub(assetsFS, "assets")
	if err == nil {
		s.mux.Handle("/", http.FileServer(http.FS(assets)))
	}
}
