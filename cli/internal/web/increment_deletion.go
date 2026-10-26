package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/gitwt"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

type deletionChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type conversationDeletionPlan struct {
	RequiresChoice bool                    `json:"requires_choice"`
	Title          string                  `json:"title"`
	Message        string                  `json:"message"`
	Choices        []deletionChoice        `json:"choices"`
	Increment      *increment.DeletionPlan `json:"increment,omitempty"`
}

func (s *Server) conversationRemovalPlan(ctx context.Context, ws *workspaceSession, record conversationlog.Record, archive bool) conversationDeletionPlan {
	plan := conversationDeletionPlan{Choices: []deletionChoice{}}
	if record.IncrementID != "" {
		current, err := s.incrementService(ws).DeletionPlan(ctx, record.IncrementID)
		if err == nil {
			plan.Increment = &current
			plan.RequiresChoice = true
		} else if _, statErr := os.Stat(record.WorkingDir); !os.IsNotExist(statErr) {
			plan.RequiresChoice = true
		}
	}
	if !plan.RequiresChoice && record.WorkingDir != "" {
		plan.RequiresChoice = gitwt.IsLinkedWorktree(ctx, ws.cfg.ProjectRoot, record.WorkingDir)
	}
	if !plan.RequiresChoice {
		return plan
	}
	plan.Title = "Eliminare la conversazione?"
	plan.Message = "Questa conversazione è collegata a un worktree. Puoi eliminare solo la chat e conservare il lavoro."
	plan.Choices = append(plan.Choices, deletionChoice{ID: "keep", Label: "Elimina solo la conversazione"})
	if archive {
		plan.Title = "Archiviare la conversazione?"
		plan.Message = "Questa conversazione è collegata a un worktree. Puoi archiviare la chat e conservare l’ambiente per riprendere il lavoro."
		plan.Choices[0].Label = "Archivia e conserva l’ambiente"
	}
	if plan.Increment != nil {
		plan.Message += " La pulizia completa ferma agente e preview, esegue il cleanup dei servizi e dei dati di test dell’incremento e rimuove worktree, branch e file locali. Restano recuperabili solo i commit salvati in Git, senza archivi dell’ambiente; eventuali PR e branch remoti restano invariati."
		if plan.Increment.Dirty {
			plan.Message += " Le modifiche locali non salvate in un commit verranno eliminate definitivamente."
		}
		if plan.Increment.UnmergedCommits > 0 {
			plan.Message += fmt.Sprintf(" Ci sono %d commit non integrati in %s.", plan.Increment.UnmergedCommits, plan.Increment.Record.BaseBranch)
		}
		label := "Elimina anche l’incremento e il suo ambiente"
		if archive {
			label = "Archivia e ripulisci tutto l’ambiente"
		}
		plan.Choices = append(plan.Choices, deletionChoice{ID: "delete", Label: label})
	}
	return plan
}

func (s *Server) handleConversationDeletionPlan(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := s.readPastConversation(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.conversationRemovalPlan(r.Context(), ws, record, r.URL.Query().Get("action") == "archive"))
}

// A linked worktree always requires an explicit scope, even for API clients.
func (s *Server) requireRemovalChoice(w http.ResponseWriter, r *http.Request, ws *workspaceSession, record conversationlog.Record, archive bool) (conversationDeletionPlan, bool) {
	plan := s.conversationRemovalPlan(r.Context(), ws, record, archive)
	scope := r.URL.Query().Get("worktree")
	if scope != "" && scope != "keep" && scope != "delete" {
		writeError(w, iox.NewInvalidInput("unknown worktree removal choice", "choose keep or delete", nil))
		return plan, false
	}
	if plan.RequiresChoice && scope == "" {
		writeError(w, iox.NewConflict("scegli se conservare o eliminare l’ambiente collegato", "leggi il piano di eliminazione e conferma una scelta esplicita", nil))
		return plan, false
	}
	if scope == "delete" && plan.Increment == nil {
		writeError(w, iox.NewConflict("no removable increment belongs to this conversation", "preserve the worktree and inspect its owning workflow", nil))
		return plan, false
	}
	return plan, true
}

func (s *Server) handleIncrementDeletionPlan(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := s.incrementService(ws).Read(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	plan := s.conversationRemovalPlan(r.Context(), ws, conversationlog.Record{IncrementID: record.ID, WorkingDir: record.Workdir}, false)
	if plan.Increment == nil {
		writeError(w, iox.NewConflict("increment environment cannot be removed safely", "inspect its Git registration before retrying", nil))
		return
	}
	plan.Title = "Eliminare l’incremento?"
	plan.Message = "Verranno eliminate anche le conversazioni collegate. " + strings.TrimPrefix(plan.Message, "Questa conversazione è collegata a un worktree. Puoi eliminare solo la chat e conservare il lavoro. ")
	plan.Choices = []deletionChoice{{ID: "delete", Label: "Elimina incremento e ambiente"}}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) stopConversationForRemoval(ctx context.Context, ws *workspaceSession, record conversationlog.Record) error {
	if _, err := ws.dispatch.cancelAndWait(ctx, "environment:"+record.WorkingDir); err != nil {
		return err
	}
	if record.Native() {
		if err := s.stopNativeConversationWork(ctx, ws, record.ID); err != nil {
			return err
		}
		if snapshot, live := ws.conversation.get(record.ID); live {
			if err := ws.releaseNativeConversation(ctx, snapshot); err != nil {
				return err
			}
		}
	} else {
		if record.ExecutionID != "" {
			if err := s.cancelExecutionBeforeRemoval(ctx, ws, record.ExecutionID); err != nil {
				return err
			}
		}
		if snapshot, live := ws.conversation.get(record.ID); live {
			ws.sealConversation(ctx, snapshot)
			if err := ws.conversation.closeOne(ctx, record.ID); err != nil {
				return err
			}
		}
	}
	if err := ws.previews.Stop(record.ID); err != nil {
		return err
	}
	return nil
}

func (s *Server) deleteIncrementEnvironment(ctx context.Context, ws *workspaceSession, id, head string, archive bool) (increment.DeletionResult, error) {
	service := s.incrementService(ws)
	record, err := service.Read(id)
	if err != nil {
		return increment.DeletionResult{}, err
	}
	// Check the confirmation before stopping any running work.
	plan, err := service.DeletionPlan(ctx, id)
	if err != nil {
		return increment.DeletionResult{}, err
	}
	if head == "" || head != plan.HeadSHA {
		return increment.DeletionResult{}, iox.NewConflict("increment changed since confirmation", "inspect the latest deletion plan and confirm again", nil)
	}
	conversations, err := ws.conversationStore().List(ctx)
	if err != nil {
		return increment.DeletionResult{}, err
	}
	for _, conv := range conversations {
		if conv.IncrementID == id || filepath.Clean(conv.WorkingDir) == filepath.Clean(record.Workdir) {
			if err := s.stopConversationForRemoval(ctx, ws, conv); err != nil {
				return increment.DeletionResult{}, err
			}
		}
	}
	return service.Delete(ctx, id, increment.DeletionOptions{HeadSHA: head, Confirm: true, PreserveConversations: archive})
}

// removeDeliveredIncrement is the archive-with-removal choice taken on the
// person's behalf once GitHub confirmed the merge: the work is on main.
func (s *Server) removeDeliveredIncrement(ctx context.Context, ws *workspaceSession, id string) error {
	plan, err := s.incrementService(ws).DeletionPlan(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.deleteIncrementEnvironment(ctx, ws, id, plan.HeadSHA, true)
	return err
}

func (s *Server) handleDeleteIncrement(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRemoval(w, r) {
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, iox.NewInvalidInput("explicit confirmation required", "read the deletion plan before removing the increment", nil))
		return
	}
	result, err := s.deleteIncrementEnvironment(r.Context(), s.session(), r.PathValue("id"), r.URL.Query().Get("head"), false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func sameOriginRemoval(w http.ResponseWriter, r *http.Request) bool {
	origin := "http://" + r.Host
	if r.TLS != nil {
		origin = "https://" + r.Host
	}
	if supplied := r.Header.Get("Origin"); supplied != "" && supplied != origin {
		writeError(w, iox.NewInvalidInput("cross-origin removal command", "use the viewer", nil))
		return false
	}
	return true
}
