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

// workspaceListView is the whole answer the home needs. Open and CurrentPath
// live here, on the list, rather than on a route of their own because the home
// asks a single question — "what do I show?" — and must get a single answer:
// with two routes the page would have to infer "no workspace is open" from the
// failure of the other one, which is exactly the inference this spec removes.
// CurrentName is here for the same reason: the page that must *name* the open
// workspace asks one question and gets one answer, instead of hunting for the
// entry marked current in a list that may not have been readable at all.
type workspaceListView struct {
	Workspaces  []workspaceEntryView `json:"workspaces"`
	Open        bool                 `json:"open"`
	CurrentPath string               `json:"currentPath"`
	CurrentName string               `json:"currentName"`
}

// workspaceViews probes each entry and renders it. The slice is never nil: the
// frontend iterates without checking, so the JSON must be [] and not null.
//
// ws may be nil — no workspace open. Then no entry is Current: the home must
// not point at a workspace nobody is serving.
func (s *Server) workspaceViews(ws *workspaceSession, entries []workspace.Entry) []workspaceEntryView {
	var root string
	if ws != nil {
		root = ws.cfg.ProjectRoot
	}
	current := filepath.Clean(root)
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
			Current:      root != "" && filepath.Clean(e.Path) == current,
		})
	}
	return views
}

// handleListWorkspaces serves GET /api/workspaces: every known workspace, most
// recently opened first, unreachable ones included. It degrades to an empty
// list instead of failing, because the list of other workspaces must never be
// a reason the current one cannot be used.
// It also declares whether a workspace is open, in every branch: a registry
// that cannot be read must not make the page believe no workspace is open.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	name, path := s.openWorkspaceIdentity()
	if s.workspaces == nil {
		writeJSON(w, http.StatusOK, workspaceListView{
			Workspaces:  []workspaceEntryView{},
			Open:        path != "",
			CurrentPath: path,
			CurrentName: name,
		})
		return
	}
	entries, err := s.workspaces.List()
	if err != nil {
		writeJSON(w, http.StatusOK, workspaceListView{
			Workspaces:  []workspaceEntryView{},
			Open:        path != "",
			CurrentPath: path,
			CurrentName: name,
		})
		return
	}
	writeJSON(w, http.StatusOK, workspaceListView{
		Workspaces:  s.workspaceViews(s.session(), entries),
		Open:        path != "",
		CurrentPath: path,
		CurrentName: name,
	})
}

// openWorkspacePath is the project root of the open workspace, or "" when none
// is. It exists so the three branches of handleListWorkspaces do not each
// restate the nil-session condition.
func (s *Server) openWorkspacePath() string {
	ws := s.session()
	if ws == nil || ws.cfg.ProjectRoot == "" {
		return ""
	}
	return filepath.Clean(ws.cfg.ProjectRoot)
}

// openWorkspaceIdentity is name and path of the open workspace, or two empty
// strings when none is. The name comes from the registry entry when there is
// one, because a directory renamed away must stay recognisable as "the one
// that was called this"; it falls back to the base name of the path, which is
// exactly what Touch would have recorded, so an unreadable registry costs the
// stored name and never the name itself.
func (s *Server) openWorkspaceIdentity() (name, path string) {
	path = s.openWorkspacePath()
	if path == "" {
		return "", ""
	}
	if s.workspaces != nil {
		// The error is deliberately ignored: a registry that cannot be listed
		// is a reason to lose the *stored* name, never the name itself.
		if entries, err := s.workspaces.List(); err == nil {
			for _, e := range entries {
				// An entry whose name is empty is no name at all: falling
				// through to the base of the path is better than telling the
				// page that the workspace it is serving has no name, which the
				// page can only read as "no workspace is open".
				if filepath.Clean(e.Path) == path && e.Name != "" {
					return e.Name, path
				}
			}
		}
	}
	return filepath.Base(path), path
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
	writeJSON(w, http.StatusCreated, s.workspaceViews(s.session(), []workspace.Entry{entry})[0])
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

// openWorkspaceView is the answer to a successful open: the entry exactly as
// GET /api/workspaces renders it — so the browser needs no second request to
// learn which workspace is now current — plus the warning that the registry
// could not record the access. The warning is a field and not an error because
// the workspace is already open: an unwritable registry must not undo it.
type openWorkspaceView struct {
	workspaceEntryView
	RegistryWarning string `json:"registryWarning,omitempty"`
}

// handleOpenWorkspace serves POST /api/workspaces/{id}/open: it makes the
// viewer serve another known workspace, without restarting the process.
//
// The order is what the acceptance criteria are made of. The switch happens
// first and the registry is touched only after it succeeded, so a refused open
// leaves both the served workspace and the recorded access exactly as they
// were.
func (s *Server) handleOpenWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, iox.NewInvalidInput("the workspace id is required", "name the entry to open", nil))
		return
	}
	if s.workspaces == nil {
		writeError(w, registryUnavailable())
		return
	}
	entry, err := s.workspaces.Get(id)
	if err != nil {
		if errors.Is(err, workspace.ErrEntryNotFound) {
			writeError(w, iox.NewNotFound("no known workspace with that id", "reload the list", err))
			return
		}
		writeError(w, err)
		return
	}

	if err := s.SwitchWorkspace(entry.Path); err != nil {
		writeError(w, err)
		return
	}

	var warning string
	touched, touchErr := s.workspaces.Touch(entry.Path)
	if touchErr != nil {
		warning = "the workspace is open, but the known-workspaces list could not be updated: " + touchErr.Error()
	} else {
		entry = touched
	}

	writeJSON(w, http.StatusOK, openWorkspaceView{
		workspaceEntryView: s.workspaceViews(s.session(), []workspace.Entry{entry})[0],
		RegistryWarning:    warning,
	})
}

// RegisterWorkspace records the workspace this viewer serves as opened now.
// It lives here rather than in NewServer because the constructor is used by
// dozens of tests, and turning it into a user-level disk write would be a side
// effect where nobody looks for one. The caller treats a failure as a printed
// line, never as a reason not to start.
func (s *Server) RegisterWorkspace() error {
	ws := s.session()
	// No workspace open means there is nothing to record — never an error.
	if ws == nil {
		return nil
	}
	if ws.cfg.ProjectRoot == "" {
		return nil
	}
	if s.workspaces == nil {
		return registryUnavailable()
	}
	if _, err := s.workspaces.Touch(ws.cfg.ProjectRoot); err != nil {
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
