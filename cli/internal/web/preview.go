package web

import (
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
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
	s.changeConversationPreview(w, r, false)
}

func (s *Server) handlePrepareConversationPreview(w http.ResponseWriter, r *http.Request) {
	s.changeConversationPreview(w, r, true)
}

func (s *Server) changeConversationPreview(w http.ResponseWriter, r *http.Request, prepareOnly bool) {
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
	var view preview.View
	if prepareOnly {
		view, err = ws.previews.Prepare(record.WorkingDir, record.ID)
	} else {
		view, err = ws.previews.Start(record.WorkingDir, record.ID, origin)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, view)
}
func (s *Server) handleStopConversationPreview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := ws.conversationStore().GetMetadata(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := ws.previews.StopEnvironment(record.WorkingDir, record.ID); err != nil {
		writeError(w, err)
		return
	}
	s.handleConversationPreview(w, r)
}
