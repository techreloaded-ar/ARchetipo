package web

import (
	"context"
	"os"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// orphanRestartReason is the failure written on a record whose carrier died
// with a previous viewer.
const orphanRestartReason = "the process that was carrying this execution is gone: View was restarted or stopped while it was running"

// orphanCancelReason is the failure written on an orphan closed by a person.
const orphanCancelReason = "cancelled from View: the process that was carrying this execution is gone"

// reconcileOrphanedExecutions closes the RUNNING records that nothing will ever
// close, at the start of a workspace session.
//
// A record left RUNNING by a dead carrier blocks every action of its scope
// with "already running", and before this there was no gesture that could end
// it. The rule is deliberately narrow, because closing a record whose work is
// still going on is worse than leaving an orphan:
//   - an action carried out in a native session is left to the session, which
//     reconcileInterruptedTurn and settleSessionActions already close;
//   - a provider whose work is detached (a remote hub) is left to the remote
//     system, since the task may still be running there;
//   - a record owned by this process or by another live one is still carried.
//
// It must run after restoreNativeConversations, so the actions of a session are
// settled by their own path before this one looks at what is left.
func (ws *workspaceSession) reconcileOrphanedExecutions(ctx context.Context, registry *execution.Registry) {
	if ws == nil || ws.service == nil || ws.store == nil {
		return
	}
	records, err := ws.store.List(ctx)
	if err != nil {
		return
	}
	var linked map[string]bool
	for _, record := range records {
		if record.Status != execution.StatusRunning {
			continue
		}
		if linked == nil {
			var known bool
			if linked, known = ws.executionsOfConversations(ctx); !known {
				// Without the conversations, an action of a session could not be
				// told from an orphan: nothing is closed rather than the wrong one.
				return
			}
		}
		if !orphanedHere(record, linked, registry) {
			continue
		}
		_, _, _ = ws.service.SettleOrphan(ctx, record.ID, ws.conn, orphanRestartReason)
	}
}

// closeOrphanedExecution is the gesture a person has when a record is RUNNING
// with no run behind it: the cancel of the run routes, which otherwise has
// nothing to cancel. It closes the record only under the same rule the restart
// applies, and reports whether it did.
func (ws *workspaceSession) closeOrphanedExecution(ctx context.Context, registry *execution.Registry, executionID string) (execution.Execution, bool) {
	if ws == nil || ws.service == nil || ws.store == nil {
		return execution.Execution{}, false
	}
	record, err := ws.store.Get(ctx, executionID)
	if err != nil {
		return execution.Execution{}, false
	}
	linked, known := ws.executionsOfConversations(ctx)
	if !known || !orphanedHere(record, linked, registry) {
		return execution.Execution{}, false
	}
	closed, settled, err := ws.service.SettleOrphan(ctx, executionID, ws.conn, orphanCancelReason)
	if err != nil || !settled {
		return execution.Execution{}, false
	}
	return closed, true
}

func orphanedHere(record execution.Execution, linked map[string]bool, registry *execution.Registry) bool {
	if linked[record.ID] {
		return false
	}
	if registry != nil {
		if provider, err := registry.Resolve(record.ProviderID); err == nil && execution.WorkOutlivesDispatcher(provider) {
			return false
		}
	}
	return execution.Orphaned(record, os.Getpid(), conversationlog.ProcessAlive)
}

// executionsOfConversations is every execution a conversation record names as
// its own: the actions carried out in a native session. The bool is false when
// the conversations could not be read, which is not the same as there being
// none.
func (ws *workspaceSession) executionsOfConversations(ctx context.Context) (map[string]bool, bool) {
	linked := map[string]bool{}
	store := ws.conversationStore()
	if store == nil {
		return linked, true
	}
	conversations, err := store.List(ctx)
	if err != nil {
		return nil, false
	}
	for _, conversation := range conversations {
		for _, id := range conversation.ExecutionIDs {
			linked[id] = true
		}
	}
	return linked, true
}
