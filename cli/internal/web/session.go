package web

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// workspaceSession is everything the viewer holds that depends on which project
// root it is serving.
//
// It exists because the server can change workspace while it is answering
// requests. Mutating the connector, the config, the store and the service one
// field at a time would be a data race by construction; collecting them in one
// immutable value that is swapped in a single assignment turns the change into
// something a handler either sees whole or does not see at all.
//
// What is *not* here is as deliberate as what is: the execution provider
// registry, the SSE broker and the registry of known workspaces do not depend
// on the project root, and the connected browsers must survive the switch.
type workspaceSession struct {
	cfg        config.Config
	conn       connector.Connector
	store      execution.Store
	service    *execution.Service
	mockupsDir string
	watchRoot  string

	// dispatch and followers are the work in flight *of this workspace*. They
	// belong to the session rather than to the server because the store they
	// write to is this workspace's store: a dispatch left running after a switch
	// would write into the `.archetipo/executions/` of a project the viewer no
	// longer serves.
	dispatch  *dispatchGroup
	followers *runFollowers

	// startOnce, stopOnce and cancel govern the lifecycle. cancel is nil until
	// start runs, so a session built and never started can still be stopped.
	// Both guards are Once because the two callers can legitimately race: a
	// switch that happens before Run has reached its own start would otherwise
	// start the same session twice, leaking a watcher and overwriting cancel.
	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc

	// mu guards stopped, which start reads and stop writes. A session that has
	// been stopped must never start: it would install a fresh context nothing
	// holds a cancel for, and a watcher goroutine that outlives the workspace.
	mu      sync.Mutex
	stopped bool
}

// newWorkspaceSession builds the session for one project root. conn is supplied
// by the caller rather than derived here because the constructor is shared by
// NewServer — whose caller already holds a connector, and whose tests hold an
// in-memory one — and by SwitchWorkspace, which builds a real connector from
// the configuration it has just loaded.
//
// providers may be nil: the viewer then simply has no provider to run with,
// which the run route reports instead of panicking.
func newWorkspaceSession(cfg config.Config, conn connector.Connector, providers *execution.Registry) (*workspaceSession, error) {
	store, err := execution.NewFileStore(cfg.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("creating the execution store: %w", err)
	}
	ws := &workspaceSession{
		cfg:        cfg,
		conn:       conn,
		store:      store,
		mockupsDir: cfg.AbsPath(cfg.Paths.Mockups),
		watchRoot:  resolveWatchRoot(cfg),
		dispatch:   newDispatchGroup(),
		followers:  newRunFollowers(),
	}
	if providers != nil {
		service, serviceErr := execution.NewService(providers, store, execution.RandomID, time.Now)
		if serviceErr != nil {
			return nil, fmt.Errorf("creating the execution service: %w", serviceErr)
		}
		ws.service = service
	}
	return ws, nil
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

// start binds the session's work to a context of its own and starts watching
// this workspace's files.
//
// The context is derived with WithoutCancel on purpose: a dispatch outlives the
// request that started it, so it must not die with the caller's context — but
// it must die with the session, which is exactly what the derived cancel gives.
func (ws *workspaceSession) start(parent context.Context, broker *Broker) {
	ws.startOnce.Do(func() {
		ws.mu.Lock()
		alreadyStopped := ws.stopped
		ws.mu.Unlock()
		if alreadyStopped {
			return
		}
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
		ws.cancel = cancel
		ws.dispatch.bind(ctx)

		// A watcher failure is non-fatal — the viewer keeps working, just without
		// live updates (clients fall back to the manual refresh button).
		if ws.watchRoot != "" && broker != nil {
			if w, werr := NewWatcher(ws.watchRoot, broker); werr == nil {
				go func() { _ = w.Run(ctx) }()
			}
		}
	})
}

// stop ends the session: it cancels the watcher and the dispatches, waits for
// the drain within the given window, and closes the followers. It is
// idempotent, because a session can be stopped both by a workspace switch and
// by the shutdown that follows it.
func (ws *workspaceSession) stop(drain time.Duration) {
	ws.stopOnce.Do(func() {
		ws.mu.Lock()
		ws.stopped = true
		ws.mu.Unlock()
		if ws.cancel != nil {
			ws.cancel()
		}
		ws.dispatch.wait(drain)
		ws.followers.closeAll()
	})
}
