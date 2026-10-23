package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
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
	writeJSON(w, http.StatusOK, conversationPreviewView(record.WorkingDir, view))
}
func (s *Server) handleStartConversationPreview(w http.ResponseWriter, r *http.Request) {
	s.changeConversationPreview(w, r, false)
}

func (s *Server) handleConfigureConversationPreview(w http.ResponseWriter, r *http.Request) {
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
	if strings.HasSuffix(r.URL.Path, "/configure") {
		err = s.queueEnvironment(r.Context(), ws, record.WorkingDir, true, true, func(ctx context.Context) error {
			_, startErr := ws.previews.Start(record.WorkingDir, record.ID, origin)
			return startErr
		})
		if err != nil {
			writeError(w, err)
			return
		}
		s.handleConversationPreview(w, r)
		return
	}
	_, loadErr := preview.Load(record.WorkingDir)
	_, commonErr := environmentrecipe.Load(record.WorkingDir)
	_, envErr := verification.Load(record.WorkingDir)
	if loadErr != nil || (commonErr != nil && envErr == nil) {
		err = s.queueEnvironment(r.Context(), ws, record.WorkingDir, true, false, func(ctx context.Context) error {
			if prepareOnly {
				return nil
			}
			_, startErr := ws.previews.Start(record.WorkingDir, record.ID, origin)
			return startErr
		})
		if err != nil {
			writeError(w, err)
			return
		}
		s.handleConversationPreview(w, r)
		return
	}
	if value := readEnvironmentOperation(record.WorkingDir); value.State == "preparing" || value.State == "configuring" || value.State == "waiting" {
		if lease, leaseErr := environmentLease(r.Context(), record.WorkingDir); leaseErr != nil {
			writeError(w, iox.NewConflict("preparazione ambiente in corso", "attendi nel pannello Preview", leaseErr))
			return
		} else {
			_ = lease.Unlock()
		}
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
	writeJSON(w, http.StatusAccepted, conversationPreviewView(record.WorkingDir, view))
}
func (s *Server) handleStopConversationPreview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := ws.conversationStore().GetMetadata(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	stopCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err = ws.dispatch.cancelAndWait(stopCtx, "environment:"+record.WorkingDir); err != nil {
		writeError(w, err)
		return
	}
	if err := ws.previews.StopEnvironment(record.WorkingDir, record.ID); err != nil {
		writeError(w, err)
		return
	}
	s.handleConversationPreview(w, r)
}

type environmentPreviewView struct {
	preview.View
	Environment environmentOperation `json:"environment"`
}

func conversationPreviewView(root string, view preview.View) environmentPreviewView {
	value := readEnvironmentOperation(root)
	if value.State == "configuring" || value.State == "preparing" || value.State == "waiting" {
		if lease, err := environmentLease(context.Background(), root); err == nil {
			_ = lease.Unlock()
			value.State = "error"
			value.Error = "Preparazione interrotta. Riprova Preview o riprendi la sessione tecnica dai dettagli ambiente."
		}
		view.State = value.State
		view.Error = value.Error
	} else if value.State == "error" && view.State != "ready" {
		view.State = "error"
		view.Error = value.Error
	}
	if view.Preparation == nil {
		view.Preparation = value.Preparation
	}
	if view.Preparation != nil {
		if logs, err := verification.New(root, "").Logs(*view.Preparation, 16000); err == nil {
			value.Log = logs
		} else {
			value.Log = "Log ambiente non disponibile: " + err.Error()
		}
	}
	return environmentPreviewView{View: view, Environment: value}
}
