package web

import (
	"context"
	"errors"
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
	backlogSpecNotes := func() {
		if code := plan.Increment.RemovesBacklogSpec; code != "" {
			plan.Message += fmt.Sprintf(" Verrà rimossa dal backlog anche la spec %s, registrata dalla pubblicazione dell’incremento.", code)
		}
		if code := plan.Increment.KeepsBacklogSpec; code != "" {
			plan.Message += fmt.Sprintf(" La spec %s resta nel backlog: questo connettore non permette di rimuoverla da qui.", code)
		}
	}
	if plan.Increment != nil && plan.Increment.Resuming {
		// The first confirmation already covered what goes, and the checkout
		// can no longer be kept: the only way forward is to finish.
		plan.Message = "Una rimozione già confermata di questo incremento si è interrotta dopo aver salvato i commit in Git. Completandola vengono rimossi i file rimasti, il branch e i record locali."
		backlogSpecNotes()
		label := "Completa la rimozione dell’incremento"
		if archive {
			label = "Archivia e completa la rimozione"
		}
		plan.Choices = []deletionChoice{{ID: "delete", Label: label}}
	} else if plan.Increment != nil {
		plan.Message += " La pulizia completa ferma agente e preview, esegue il cleanup dei servizi e dei dati di test dell’incremento e rimuove worktree, branch e file locali. Restano recuperabili solo i commit salvati in Git, senza archivi dell’ambiente; eventuali PR e branch remoti restano invariati."
		if plan.Increment.Dirty {
			plan.Message += " Le modifiche locali non salvate in un commit verranno eliminate definitivamente."
		}
		if plan.Increment.UnmergedCommits > 0 {
			plan.Message += fmt.Sprintf(" Ci sono %d commit non integrati in %s.", plan.Increment.UnmergedCommits, plan.Increment.Record.BaseBranch)
		}
		backlogSpecNotes()
		if pr := plan.Increment.Record.PR; pr != nil && pr.State == "OPEN" {
			plan.Message += fmt.Sprintf(" La PR #%d resta aperta su GitHub: chiudila lì se non deve essere integrata.", pr.Number)
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
		// Name what is wrong with the checkout instead of a generic refusal.
		_, planErr := s.incrementService(ws).DeletionPlan(r.Context(), record.ID)
		if planErr == nil {
			planErr = iox.NewConflict("increment environment cannot be removed safely", "inspect its Git registration before retrying", nil)
		}
		writeError(w, planErr)
		return
	}
	plan.Title = "Eliminare l’incremento?"
	plan.Message = "Verranno eliminate anche le conversazioni collegate. " + strings.TrimPrefix(plan.Message, "Questa conversazione è collegata a un worktree. Puoi eliminare solo la chat e conservare il lavoro. ")
	if !plan.Increment.Resuming {
		plan.Choices = []deletionChoice{{ID: "delete", Label: "Elimina incremento e ambiente"}}
	}
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

// environmentRemovalChoice reads how the request treats a cleanup that fails:
// nothing, which preserves the worktree, or the explicit choice to remove the
// increment leaving its services and test data in place.
func environmentRemovalChoice(w http.ResponseWriter, r *http.Request) (abandon, valid bool) {
	switch r.URL.Query().Get("environment") {
	case "":
		return false, true
	case "abandon":
		return true, true
	}
	writeError(w, iox.NewInvalidInput("unknown environment removal choice", "omit it or choose abandon", nil))
	return false, false
}

// writeRemovalError answers a failed removal. A cleanup that ran and failed is
// not a dead end: the answer carries its reason and the choice to proceed
// anyway, in the same shape as the deletion plan, so the page asks the person
// instead of leaving them with an increment that cannot be closed.
func writeRemovalError(w http.ResponseWriter, err error, archive bool) {
	var cleanup *increment.EnvironmentCleanupError
	if !errors.As(err, &cleanup) {
		writeError(w, err)
		return
	}
	message := "Non è stato possibile fermare e rimuovere i servizi e i dati di test dell’incremento. Motivo: " + cleanup.Reason + ". L’incremento è intatto: se la causa si può sistemare, ad esempio avviando il motore dei container, fallo e ripeti l’operazione. Se procedi comunque, worktree, branch e file locali vengono rimossi e i servizi e i dati di test restano su questo computer"
	if len(cleanup.Resources) > 0 {
		message += " (" + strings.Join(cleanup.Resources, ", ") + ")"
	}
	message += ": andranno rimossi a mano."
	label := "Elimina comunque l’incremento"
	if archive {
		label = "Archivia e rimuovi comunque l’incremento"
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": "Il cleanup dell’ambiente non è riuscito: " + cleanup.Reason,
		"code":  iox.CodeConflict,
		"hint":  "sistema la causa e riprova, oppure conferma la rimozione lasciando servizi e dati di test",
		"removal": conversationDeletionPlan{
			RequiresChoice: true,
			Title:          "Il cleanup dell’ambiente non è riuscito",
			Message:        message,
			Choices:        []deletionChoice{{ID: "abandon", Label: label}},
		},
	})
}

func (s *Server) deleteIncrementEnvironment(ctx context.Context, ws *workspaceSession, id, head string, archive, abandonEnvironment bool) (increment.DeletionResult, error) {
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
	return service.Delete(ctx, id, increment.DeletionOptions{HeadSHA: head, Confirm: true, PreserveConversations: archive, AbandonEnvironment: abandonEnvironment})
}

func (s *Server) handleDeleteIncrement(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRemoval(w, r) {
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, iox.NewInvalidInput("explicit confirmation required", "read the deletion plan before removing the increment", nil))
		return
	}
	abandonEnvironment, valid := environmentRemovalChoice(w, r)
	if !valid {
		return
	}
	result, err := s.deleteIncrementEnvironment(r.Context(), s.session(), r.PathValue("id"), r.URL.Query().Get("head"), false, abandonEnvironment)
	if err != nil {
		writeRemovalError(w, err, false)
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
