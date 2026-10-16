package web

import (
	"context"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workflowrun"
)

type workflowProgress struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

func (ws *workspaceSession) attachWorkflow(record conversationlog.Record, view *conversationView) {
	runs, err := workflowrun.New(record.WorkingDir).List()
	if err != nil {
		view.Notice = "Impossibile leggere lo stato del lavoro: " + err.Error()
		return
	}
	for _, run := range runs {
		if run.ConversationID != record.ID || run.Status == workflowrun.Complete {
			continue
		}
		labels := map[string]string{workflowrun.Running: "Lavoro in corso", workflowrun.Recovering: "Recupero del lavoro in corso", workflowrun.Blocked: "È necessario un intervento per proseguire"}
		view.Workflow = &workflowProgress{Label: labels[run.Status]}
		if record.CurrentTurn != nil && record.StopRequestedTurnID == record.CurrentTurn.ID {
			view.Workflow.Label = "Lavoro interrotto dalla persona"
			return
		}
		if run.Status == workflowrun.Running && record.CurrentTurn != nil && record.CurrentTurn.State == execution.TurnFailed {
			view.Workflow.Label = "Il lavoro si è interrotto"
			view.Workflow.Detail = record.CurrentTurn.Error
		}
		if run.Status == workflowrun.Blocked {
			view.Workflow.Detail = run.LastError
		}
		// Increment context already reports phase and session activity. Keep
		// separate notices only for recovery, blocked work or a failed turn.
		if record.IncrementID != "" && run.Status == workflowrun.Running && (record.CurrentTurn == nil || record.CurrentTurn.State != execution.TurnFailed) {
			view.Workflow = nil
		}
		return
	}
	if record.CurrentTurn != nil && record.CurrentTurn.State == execution.TurnFailed {
		view.Workflow = &workflowProgress{Label: "Il lavoro si è interrotto", Detail: record.CurrentTurn.Error}
	}
}

// A continuation is a new reconciliation turn, never a replay of a command
// whose delivery is uncertain. Only an unfinished, conversation-bound skill
// invocation opts into it. Ordinary chat and human interrupts do not.
func (ws *workspaceSession) recoverWorkflow(ctx context.Context, snapshot conversationSnapshot) bool {
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil || record.Archive != execution.ArchiveOpen || record.CurrentTurn == nil || record.CurrentTurn.State != execution.TurnFailed || record.StopRequestedTurnID == record.CurrentTurn.ID {
		return false
	}
	for _, delivery := range record.Deliveries {
		if delivery.State == execution.DeliveryUncertain {
			return false
		}
	}
	store := workflowrun.New(snapshot.workingDir)
	runs, err := store.List()
	if err != nil {
		return false
	}
	for _, run := range runs {
		if run.ConversationID != snapshot.id || run.Status == workflowrun.Complete {
			continue
		}
		// Oldest is the outermost invocation. Its skill reconciles its children;
		// the runtime knows nothing about their names, scope or phase ordering.
		run, err = store.Recover(ctx, run.ID, record.CurrentTurn.ID, record.CurrentTurn.Error)
		if err != nil || run.Status != workflowrun.Recovering {
			return false
		}
		for attempt := 0; attempt < 3; attempt++ {
			if ctx.Err() != nil {
				return false
			}
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return false
				case <-time.After(time.Duration(attempt) * time.Second):
				}
			}
			observed, resumeErr := snapshot.sessionProvider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: snapshot.session})
			if resumeErr != nil {
				err = resumeErr
				continue
			}
			if observed.Work != execution.SessionIdle {
				// Some native runtimes resume work already in flight. Follow it
				// (including any pending input) instead of starting another turn.
				return ws.persistNativeSnapshot(ctx, snapshot.id, observed) == nil
			}
			if err = ws.persistNativeSnapshot(ctx, snapshot.id, observed); err != nil {
				return false
			}
			message := fmt.Sprintf("%s\n\nResume workflow run %s from its CLI checkpoint after a transport interruption. Reconcile persisted artifacts, pending workers and verification evidence before continuing. Preserve completed work and existing approvals. Do not replay uncertain operations or publish without the required human approval.\nWorkflow conversation: %s", run.Invocation, run.ID, snapshot.id)
			err = ws.sendNativeConversationMessage(ctx, snapshot, sendConversationMessageReq{Message: message, display: "Riprendo il lavoro dal punto salvato dopo un’interruzione della connessione.", recoveryForTurnID: record.CurrentTurn.ID})
			if err == nil {
				return true
			}
			// StartTurn may already have reached the harness. Its delivery is
			// recorded by sendNativeConversationMessage: never resend it here.
			break
		}
		if err != nil {
			_, _ = store.Update(context.WithoutCancel(ctx), run.ID, workflowrun.Checkpoint{Revision: run.Revision, Status: workflowrun.Blocked, Phase: run.Phase, WorkerID: run.WorkerID, State: run.State, LastError: err.Error()})
		}
		return false
	}
	return false
}
