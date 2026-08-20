package web

import (
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// workspaceEntryView is one known workspace as the browser sees it. Status is
// computed on every request rather than stored, so a directory that moved
// while nobody was looking is reported instead of remembered wrongly;
// Reachable is its reduction for the common case, and Current marks the
// workspace *this* viewer is serving, so the list never offers to forget the
// entry being looked at without saying so.
type workspaceEntryView struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
	Status       string    `json:"status"`
	Reachable    bool      `json:"reachable"`
	Current      bool      `json:"current"`
}

type workspaceListView struct {
	Workspaces []workspaceEntryView `json:"workspaces"`
}

// workspaceViews probes each entry and renders it. The slice is never nil: the
// frontend iterates without checking, so the JSON must be [] and not null.
func (s *Server) workspaceViews(entries []workspace.Entry) []workspaceEntryView {
	current := filepath.Clean(s.cfg.ProjectRoot)
	views := make([]workspaceEntryView, 0, len(entries))
	for _, e := range entries {
		status := workspace.Probe(e)
		views = append(views, workspaceEntryView{
			ID:           e.ID,
			Name:         e.Name,
			Path:         e.Path,
			LastOpenedAt: e.LastOpenedAt,
			Status:       string(status),
			Reachable:    status.Reachable(),
			Current:      s.cfg.ProjectRoot != "" && filepath.Clean(e.Path) == current,
		})
	}
	return views
}

// handleListWorkspaces serves GET /api/workspaces: every known workspace, most
// recently opened first, unreachable ones included. It degrades to an empty
// list instead of failing, because the list of other workspaces must never be
// a reason the current one cannot be used.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.workspaces == nil {
		writeJSON(w, http.StatusOK, workspaceListView{Workspaces: []workspaceEntryView{}})
		return
	}
	entries, err := s.workspaces.List()
	if err != nil {
		writeJSON(w, http.StatusOK, workspaceListView{Workspaces: []workspaceEntryView{}})
		return
	}
	writeJSON(w, http.StatusOK, workspaceListView{Workspaces: s.workspaceViews(entries)})
}

// handleAddWorkspace serves POST /api/workspaces: it records a workspace that
// already exists on disk. Here the path arrives from a text field, so it is
// validated and refused per-field — unlike the automatic registrations, whose
// caller already holds a real workspace.
func (s *Server) handleAddWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if s.workspaces == nil {
		writeError(w, registryUnavailable())
		return
	}

	entry, err := s.workspaces.Add(req.Path)
	if err != nil {
		var invalid *workspace.ValidationError
		if errors.As(err, &invalid) {
			writeFieldErrors(w,
				"the workspace could not be added: some fields are invalid",
				"fix the highlighted field and confirm again",
				workspaceFieldErrors(invalid),
			)
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.workspaceViews([]workspace.Entry{entry})[0])
}

// handleRemoveWorkspace serves DELETE /api/workspaces/{id}: it forgets one
// entry. Forgetting is not deleting — the only thing this route writes is the
// registry file, and the workspace directory is never touched.
func (s *Server) handleRemoveWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, iox.NewInvalidInput("the workspace id is required", "name the entry to remove", nil))
		return
	}
	if s.workspaces == nil {
		writeError(w, registryUnavailable())
		return
	}
	if err := s.workspaces.Remove(id); err != nil {
		if errors.Is(err, workspace.ErrEntryNotFound) {
			writeError(w, iox.NewNotFound("no known workspace with that id", "reload the list", err))
			return
		}
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RegisterWorkspace records the workspace this viewer serves as opened now.
// It lives here rather than in NewServer because the constructor is used by
// dozens of tests, and turning it into a user-level disk write would be a side
// effect where nobody looks for one. The caller treats a failure as a printed
// line, never as a reason not to start.
func (s *Server) RegisterWorkspace() error {
	if s.cfg.ProjectRoot == "" {
		return nil
	}
	if s.workspaces == nil {
		return registryUnavailable()
	}
	if _, err := s.workspaces.Touch(s.cfg.ProjectRoot); err != nil {
		return err
	}
	return nil
}

func registryUnavailable() error {
	return iox.NewPrecondition(
		"the workspace registry is unavailable",
		"set "+workspace.EnvStateDir+" to a writable directory",
		nil,
	)
}
