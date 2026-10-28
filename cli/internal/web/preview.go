package web

import (
	"context"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

func (s *Server) handleConversationPreview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	target, err := s.resolvePreviewTarget(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	view, err := ws.previews.Status(target.Root, target.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.previewViewForTarget(r.Context(), ws, target, view))
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
	origin, originErr := previewCommandOrigin(r)
	if originErr != nil {
		writeError(w, originErr)
		return
	}
	ws := s.session()
	target, err := s.resolvePreviewTarget(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/configure") {
		err = s.queueEnvironment(r.Context(), ws, target.Root, true, true, func(ctx context.Context) error {
			_, startErr := ws.previews.Start(target.Root, target.ID, origin)
			return startErr
		})
		if err != nil {
			writeError(w, err)
			return
		}
		s.handleConversationPreview(w, r)
		return
	}
	_, loadErr := preview.Load(target.Root)
	_, commonErr := environmentrecipe.Load(target.Root)
	_, envErr := verification.Load(target.Root)
	if loadErr != nil || (commonErr != nil && envErr == nil) {
		err = s.queueEnvironment(r.Context(), ws, target.Root, true, false, func(ctx context.Context) error {
			if prepareOnly {
				return nil
			}
			_, startErr := ws.previews.Start(target.Root, target.ID, origin)
			return startErr
		})
		if err != nil {
			writeError(w, err)
			return
		}
		s.handleConversationPreview(w, r)
		return
	}
	if value := readEnvironmentOperation(target.Root); value.State == "preparing" || value.State == "configuring" || value.State == "waiting" {
		if lease, leaseErr := environmentLease(r.Context(), target.Root); leaseErr != nil {
			writeError(w, iox.NewConflict("preparazione ambiente in corso", "attendi nel pannello Preview", leaseErr))
			return
		} else {
			_ = lease.Unlock()
		}
	}
	var view preview.View
	if prepareOnly {
		view, err = ws.previews.Prepare(target.Root, target.ID)
	} else {
		view, err = ws.previews.Start(target.Root, target.ID, origin)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.previewViewForTarget(r.Context(), ws, target, view))
}
func (s *Server) handleStopConversationPreview(w http.ResponseWriter, r *http.Request) {
	if _, err := previewCommandOrigin(r); err != nil {
		writeError(w, err)
		return
	}
	ws := s.session()
	target, err := s.resolvePreviewTarget(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	stopCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err = ws.dispatch.cancelAndWait(stopCtx, "environment:"+target.Root); err != nil {
		writeError(w, err)
		return
	}
	if err := ws.previews.StopEnvironment(target.Root, target.ID); err != nil {
		writeError(w, err)
		return
	}
	s.handleConversationPreview(w, r)
}

type environmentPreviewView struct {
	preview.View
	Environment environmentOperation            `json:"environment"`
	Context     previewContext                  `json:"context"`
	Headline    string                          `json:"headline"`
	ChecksLabel string                          `json:"checks_label"`
	Checks      []verification.CheckObservation `json:"checks"`
	ChecksError string                          `json:"checks_error,omitempty"`
	Attention   *environmentAttention           `json:"attention,omitempty"`
}

func conversationPreviewView(root string, view preview.View) environmentPreviewView {
	value := readEnvironmentOperation(root)
	if view.State == "ready" && view.Preparation != nil && view.Preparation.Passed && view.Preparation.Stable {
		value.PreviousError = value.Error
		value.Error = ""
		value.State = "ready"
		value.Preparation = view.Preparation
	}
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

// The target is a checkout, independently of whether a product conversation
// already exists. Both routes use the same preparation and preview manager.
type previewTarget struct {
	Root    string
	ID      string
	Context previewContext
}
type previewContext struct {
	Label             string `json:"label"`
	WorkingDirectory  string `json:"working_directory"`
	Branch            string `json:"branch,omitempty"`
	Commit            string `json:"commit,omitempty"`
	Worktree          bool   `json:"worktree"`
	ConversationID    string `json:"conversation_id,omitempty"`
	ConversationTitle string `json:"conversation_title,omitempty"`
	IncrementID       string `json:"increment_id,omitempty"`
	WorkTitle         string `json:"work_title,omitempty"`
}
type workspacePreviewEntry struct {
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
	Context     previewContext         `json:"context"`
	Target      workspacePreviewTarget `json:"target"`
}
type workspacePreviewTarget struct {
	ID          string `json:"id"`
	Scope       string `json:"scope"`
	Root        string `json:"root"`
	Writable    bool   `json:"writable"`
	Commentable bool   `json:"commentable"`
	Compact     bool   `json:"compact"`
}

func checkoutPreviewContext(parent context.Context, root string) previewContext {
	actualRoot, _ := filepath.EvalSymlinks(root)
	commonRoot, _ := filepath.EvalSymlinks(environmentrecipe.Workspace(root))
	result := previewContext{WorkingDirectory: root, Worktree: actualRoot != "" && commonRoot != "" && actualRoot != commonRoot}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	branch, err := exec.CommandContext(ctx, "git", "-C", root, "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err == nil {
		result.Branch = strings.TrimSpace(string(branch))
	}
	commit, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	if err == nil {
		result.Commit = strings.TrimSpace(string(commit))
	}
	result.Label = "Checkout principale"
	if result.Worktree {
		result.Label = "Worktree"
	}
	if result.Branch != "" {
		result.Label += " · " + result.Branch
	} else if result.Commit != "" {
		result.Label += " · commit " + result.Commit + " (HEAD scollegato)"
	} else {
		result.Label += " · branch non disponibile"
	}
	return result
}
func (s *Server) resolvePreviewTarget(ctx context.Context, ws *workspaceSession, id string) (previewTarget, error) {
	target := previewTarget{Root: ws.cfg.ProjectRoot, ID: "workspace-preview"}
	if id != "" {
		record, err := ws.conversationStore().GetMetadata(ctx, id)
		if err != nil {
			return target, err
		}
		target.Root, target.ID = record.WorkingDir, record.ID
		target.Context = checkoutPreviewContext(ctx, target.Root)
		target.Context.ConversationID, target.Context.ConversationTitle = record.ID, record.Title
		target.Context.IncrementID = record.IncrementID
		if record.IncrementID != "" {
			if increment, readErr := s.incrementService(ws).Read(record.IncrementID); readErr == nil {
				target.Context.WorkTitle = increment.Title
			}
		}
	} else {
		target.Context = checkoutPreviewContext(ctx, target.Root)
	}
	return target, nil
}
func (s *Server) previewViewForTarget(ctx context.Context, ws *workspaceSession, target previewTarget, view preview.View) environmentPreviewView {
	result := conversationPreviewView(target.Root, view)
	result.Context = target.Context
	s.presentEnvironment(ctx, ws, target, &result)
	return result
}
func (s *Server) workspacePreviewEntry(ctx context.Context, ws *workspaceSession) workspacePreviewEntry {
	return workspacePreviewEntry{Label: "Apri l’app attuale", Description: "Esplora il checkout del progetto. Se necessario, l’AI configura l’ambiente.", Context: checkoutPreviewContext(ctx, ws.cfg.ProjectRoot), Target: workspacePreviewTarget{ID: "workspace-preview", Scope: "workspace", Root: ws.cfg.ProjectRoot, Writable: true, Compact: true}}
}

func previewCommandOrigin(r *http.Request) (string, error) {
	origin := "http://" + r.Host
	if r.TLS != nil {
		origin = "https://" + r.Host
	}
	if supplied := r.Header.Get("Origin"); supplied != "" && supplied != origin {
		return "", iox.NewInvalidInput("cross-origin preview command", "use the viewer", nil)
	}
	return origin, nil
}
