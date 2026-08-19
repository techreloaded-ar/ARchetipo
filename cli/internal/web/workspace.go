package web

import (
	"errors"
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// createWorkspaceReq is the JSON body of POST /api/workspace.
//
// There is deliberately no `template` field: exactly one process Archetype is
// installed, so the workspace is initialized on it and the browser is never
// asked to choose. decodeJSON refuses unknown fields, so a client that tried
// to pick one would be told, rather than silently ignored.
type createWorkspaceReq struct {
	Dir       string                `json:"dir"`
	Connector string                `json:"connector"`
	Tools     []string              `json:"tools"`
	Paths     domain.ConfigPaths    `json:"paths"`
	Worktree  domain.WorktreeConfig `json:"worktree"`
}

// createWorkspaceView is the JSON response of POST /api/workspace. Hint says
// what the viewer cannot do yet: opening the new workspace in this session is
// a separate capability, so the answer points at the way that works today.
type createWorkspaceView struct {
	workspace.Result
	Hint string `json:"hint"`
}

// handleGetWorkspaceOptions serves GET /api/workspace/options: the choices an
// initialization actually accepts, read from the same registries the
// initialization itself reads. It is the reason the frontend carries no list
// of its own.
func (s *Server) handleGetWorkspaceOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, workspace.Available())
}

// handleCreateWorkspace serves POST /api/workspace: it initializes a workspace
// in the requested directory. The workspace this viewer is serving is not
// touched — the new one is created elsewhere and opened separately.
func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req createWorkspaceReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}

	result, err := workspace.Initialize(r.Context(), workspace.Options{
		Dir:       req.Dir,
		Connector: req.Connector,
		Tools:     req.Tools,
		Paths:     req.Paths,
		Worktree:  req.Worktree,
	})
	if err != nil {
		var invalid *workspace.ValidationError
		if errors.As(err, &invalid) {
			writeFieldErrors(w,
				"the workspace could not be created: some fields are invalid",
				"fix the highlighted fields and confirm again",
				workspaceFieldErrors(invalid),
			)
			return
		}
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, createWorkspaceView{
		Result: result,
		Hint:   "open the new workspace by running `archetipo view` in " + result.Dir,
	})
}

// workspaceFieldErrors converts the package's refusals into the per-field
// shape the viewer already renders for the spec form.
func workspaceFieldErrors(invalid *workspace.ValidationError) []fieldError {
	out := make([]fieldError, 0, len(invalid.Fields))
	for _, f := range invalid.Fields {
		out = append(out, fieldError{Field: f.Field, Code: f.Code, Message: f.Message})
	}
	return out
}

// writeFieldErrors answers 400 keeping the error/code/hint keys the rest of the
// viewer already understands, and adds the per-field detail under "fields" so
// an older client still shows the message.
func writeFieldErrors(w http.ResponseWriter, message, hint string, fields []fieldError) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":  message,
		"code":   iox.CodeInvalidInput,
		"hint":   hint,
		"fields": fields,
	})
}
