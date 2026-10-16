package web

import (
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func (s *Server) handleConversationPreview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := ws.conversationStore().GetMetadata(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := ws.previews.Status(record.WorkingDir, record.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
func (s *Server) handleStartConversationPreview(w http.ResponseWriter, r *http.Request) {
	// A site displayed in the preview must not be able to start local commands
	// by submitting a cross-origin form or fetch to the viewer.
	origin := "http://" + r.Host
	if r.TLS != nil {
		origin = "https://" + r.Host
	}
	if supplied := r.Header.Get("Origin"); supplied != "" && supplied != origin {
		writeError(w, iox.NewInvalidInput("cross-origin preview command", "use the viewer", nil))
		return
	}
	ws := s.session()
	record, err := ws.conversationStore().GetMetadata(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := ws.previews.Start(record.WorkingDir, record.ID, origin)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, view)
}
func (s *Server) handleStopConversationPreview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	if _, err := ws.conversationStore().GetMetadata(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	ws.previews.Stop(r.PathValue("id"))
	s.handleConversationPreview(w, r)
}
