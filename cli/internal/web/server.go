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
	"path/filepath"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// Server wires the connector backend to HTTP handlers and the embedded UI.
type Server struct {
	conn       connector.Connector
	cfg        config.Config
	registry   *execution.Registry
	mux        *http.ServeMux
	httpSrv    *http.Server
	mockupsDir string
	broker     *Broker
	watchRoot  string

	// store and service dispatch spec actions. The store is always present (it
	// only needs the project root); the service is nil when no provider registry
	// was supplied, which makes the run route answer "no provider" instead of
	// panicking.
	store    execution.Store
	service  *execution.Service
	dispatch *dispatchGroup

	// followers holds the server-side projection of the remote run behind an
	// execution, one per execution and only for the executions someone is
	// actually looking at.
	followers *runFollowers
}

// NewServer constructs a Server bound to addr (e.g. "127.0.0.1:8080").
// The returned server has all routes registered but is not listening yet:
// call Run to start serving. cfg is used to resolve the on-disk location of
// design mockups served under /mockups/. registry is the execution provider
// registry the viewer offers for selection; a nil registry simply offers no
// provider, which is not an error.
func NewServer(conn connector.Connector, cfg config.Config, registry *execution.Registry, addr string) (*Server, error) {
	mux := http.NewServeMux()
	store, err := execution.NewFileStore(cfg.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("creating the execution store: %w", err)
	}
	s := &Server{
		conn:       conn,
		cfg:        cfg,
		registry:   registry,
		mux:        mux,
		mockupsDir: cfg.AbsPath(cfg.Paths.Mockups),
		broker:     NewBroker(),
		watchRoot:  resolveWatchRoot(cfg),
		store:      store,
		dispatch:   newDispatchGroup(),
		followers:  newRunFollowers(),
	}
	// A nil registry is not an error: the viewer simply has no provider to run
	// with, and the run route says so. Building a service over it would be the
	// only way to turn that into a panic.
	if registry != nil {
		service, serviceErr := execution.NewService(registry, store, execution.RandomID, time.Now)
		if serviceErr != nil {
			return nil, fmt.Errorf("creating the execution service: %w", serviceErr)
		}
		s.service = service
	}
	s.registerRoutes()
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// resolveWatchRoot picks the directory the filesystem watcher should observe.
// The viewer cares about anything that affects the rendered board, so we watch
// the parent of the backlog file (typically .archetipo/), which also contains
// stories/ and plans/.
func resolveWatchRoot(cfg config.Config) string {
	if cfg.File.Backlog == "" {
		return ""
	}
	return cfg.AbsPath(filepath.Dir(cfg.File.Backlog))
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
	// Dispatches outlive the request that started them, so they run on a context
	// owned by the server rather than by any client. Cancelling it on the way out
	// is what turns an interrupted execution into a FAILED record with a reason
	// instead of one left RUNNING for ever.
	dispatchCtx, cancelDispatch := context.WithCancel(context.WithoutCancel(ctx))
	s.dispatch.bind(dispatchCtx)
	defer func() {
		cancelDispatch()
		s.dispatch.wait(dispatchDrainTimeout)
	}()
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.httpSrv.Addr, err)
	}
	// Capture the resolved port (in case Addr was ":0").
	s.httpSrv.Addr = ln.Addr().String()
	if onReady != nil {
		onReady("http://" + s.httpSrv.Addr)
	}

	// Real-time refresh: start the filesystem watcher if a watch root is set.
	// A watcher failure is non-fatal — the viewer keeps working, just without
	// live updates (clients fall back to the manual refresh button).
	if s.watchRoot != "" {
		if w, werr := NewWatcher(s.watchRoot, s.broker); werr == nil {
			go func() { _ = w.Run(ctx) }()
		}
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
		s.followers.closeAll()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		s.broker.Close()
		s.followers.closeAll()
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
	s.mux.HandleFunc("POST /api/spec/{code}/integrate", s.handleIntegrate)
	s.mux.HandleFunc("GET /api/prd", s.handleGetPRD)
	s.mux.HandleFunc("PUT /api/prd", s.handleSavePRD)
	s.mux.HandleFunc("GET /api/execution/providers", s.handleListExecutionProviders)
	s.mux.HandleFunc("PUT /api/execution/provider/default", s.handleSaveDefaultExecutionProvider)
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
	s.mux.HandleFunc("GET /api/mockups", s.handleListMockups)

	// Serve design mockups from the configured paths.mockups directory.
	// The handler is registered unconditionally; a missing directory just
	// produces 404s, which the frontend already tolerates.
	if s.mockupsDir != "" {
		s.mux.Handle("/mockups/", http.StripPrefix("/mockups/", http.FileServer(http.Dir(s.mockupsDir))))
	}

	// Static assets (HTML/CSS/JS + vendor). Served from the embedded FS.
	assets, err := fs.Sub(assetsFS, "assets")
	if err == nil {
		s.mux.Handle("/", http.FileServer(http.FS(assets)))
	}
}
