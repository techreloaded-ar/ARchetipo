package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// handleGetWorkspaceFile serves one image the agent wrote into the workspace,
// named by its path relative to the directory the agent works in, so a
// Markdown image in the agent's answer can be shown. That directory is the
// project root or, when the request names a conversation, the working
// directory of that conversation: an increment in a worktree writes and cites
// its paths from there, and resolving them against the root would show nothing
// or, worse, an older file of the same name. It is confined on purpose: a
// relative path only, resolved inside that directory symlinks included, and one
// of the image formats a conversation carries. Everything else is a 404, never
// a listing and never a reason that would say what is there.
func (s *Server) handleGetWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	root := s.session().cfg.ProjectRoot
	if id := strings.TrimSpace(r.URL.Query().Get("conversation")); id != "" {
		record, err := s.session().conversationStore().GetMetadata(r.Context(), id)
		if err != nil || strings.TrimSpace(record.WorkingDir) == "" {
			writeError(w, iox.NewNotFound("no such file", "", err))
			return
		}
		root = record.WorkingDir
	}
	path, mediaType, err := resolveWorkspaceImage(root, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, iox.NewNotFound("no such file", "", err))
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

// resolveWorkspaceImage turns a relative path into the regular image file it
// names under root, or an error when it names anything else.
func resolveWorkspaceImage(root, requested string) (string, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" || filepath.IsAbs(requested) || strings.HasPrefix(requested, "/") {
		return "", "", fmt.Errorf("a workspace file is named by a relative path")
	}
	mediaType := workspaceImageMediaType(requested)
	if mediaType == "" {
		return "", "", fmt.Errorf("not an image")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	realPath, err := filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(requested)))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("outside the workspace")
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("not a regular file")
	}
	return realPath, mediaType, nil
}

// workspaceImageMediaType is the media type a file's extension declares, among
// the formats a conversation carries, or empty.
func workspaceImageMediaType(path string) string {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".jpeg" {
		extension = ".jpg"
	}
	for mediaType, known := range attachmentMediaTypes {
		if extension == known {
			return mediaType
		}
	}
	return ""
}
