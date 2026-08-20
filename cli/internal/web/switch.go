package web

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// switchReason turns a probe status into words a person can act on. The status
// is kept in the message rather than reduced to "unreachable" because the three
// ways a workspace stops being one — moved, unreadable, stripped of its
// configuration — call for three different remedies.
func switchReason(status workspace.Status) string {
	switch status {
	case workspace.StatusMissing:
		return "the directory no longer exists"
	case workspace.StatusNotDirectory:
		return "the path is not a directory"
	case workspace.StatusNotReadable:
		return "the directory cannot be read"
	case workspace.StatusNotWorkspace:
		return "the directory is not a workspace: " + config.RelativePath + " is missing"
	default:
		return string(status)
	}
}

// SwitchWorkspace replaces the workspace this viewer serves, without restarting
// the process.
//
// The order is the whole point: the new session is built and started *before*
// the current one is touched, so every failure — a directory that moved, a
// configuration that will not parse, a connector that refuses — returns leaving
// the previous workspace exactly where it was, still served and still writable.
// The swap itself is a single assignment under the lock, so a request sees one
// workspace or the other and never a mixture of the two.
func (s *Server) SwitchWorkspace(root string) error {
	abs, switched, err := s.swapSession(root)
	if err != nil {
		return err
	}
	if !switched {
		return nil
	}
	// Wake the connected browsers: their board now describes another project.
	s.broker.Publish()
	// The hook is called with switchMu released, so a caller that reacts by
	// opening another workspace cannot deadlock on a non-reentrant mutex.
	if s.OnWorkspaceSwitch != nil {
		s.OnWorkspaceSwitch(abs)
	}
	return nil
}

// swapSession is SwitchWorkspace without its observable side effects. It
// returns the normalised root and whether a swap actually happened — reopening
// the workspace already served is a no-op, not a change to announce.
func (s *Server) swapSession(root string) (string, bool, error) {
	// One switch at a time. Two concurrent opens of the same root would
	// otherwise both pass the same-root check, both build a session and both
	// start a watcher, for one of them to be stopped a moment later.
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	root = strings.TrimSpace(root)
	if root == "" {
		return "", false, iox.NewInvalidInput("the workspace path is required", "name the directory to open", nil)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false, iox.NewInvalidInput("cannot resolve the workspace path", "provide an absolute path", err)
	}
	abs = filepath.Clean(abs)

	// Probe before loading: config.LoadExact walks up the directory tree, so on a
	// directory that is not a workspace it would find a parent's configuration
	// and silently open the wrong project. Asking the disk about *this*
	// directory first is what makes the declared reason correct rather than
	// lucky, and the order is covered by a test.
	if status := workspace.Probe(workspace.Entry{Path: abs}); !status.Reachable() {
		return "", false, iox.NewPrecondition(
			"the workspace cannot be opened: "+switchReason(status),
			"check the directory and try again; the current workspace is still open",
			nil,
		)
	}

	// Reopening the workspace already served is not an error, and rebuilding it
	// would cancel the work in flight for no reason.
	if cur := s.session(); cur != nil && filepath.Clean(cur.cfg.ProjectRoot) == abs {
		return abs, false, nil
	}

	cfg, err := config.LoadExact(abs)
	if err != nil {
		return "", false, iox.NewInvalidInput(
			fmt.Sprintf("the workspace cannot be opened: %s could not be read", config.RelativePath),
			"fix the configuration file and try again; the current workspace is still open",
			err,
		)
	}
	cfg.ProjectRoot = abs

	conn, err := connector.New(cfg)
	if err != nil {
		return "", false, iox.NewInvalidInput(
			fmt.Sprintf("the workspace cannot be opened: the connector declared in %s is not usable", config.RelativePath),
			"fix the connector setting and try again; the current workspace is still open",
			err,
		)
	}

	next, err := newWorkspaceSession(cfg, conn, s.registry)
	if err != nil {
		return "", false, iox.NewInternal("the workspace cannot be opened", err)
	}

	s.mu.RLock()
	baseCtx := s.baseCtx
	s.mu.RUnlock()
	next.start(baseCtx, s.broker)

	s.mu.Lock()
	previous := s.cur
	s.cur = next
	s.mu.Unlock()

	// Stopping happens outside the lock on purpose: stop waits for the drain,
	// and holding the lock for that long would block every handler.
	if previous != nil {
		previous.stop(dispatchDrainTimeout)
	}
	return abs, true, nil
}
