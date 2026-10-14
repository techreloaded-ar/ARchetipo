package web

import (
	"context"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Stopping work is distinct from releasing a connection. Archive and deletion
// explicitly stop first; ordinary connection release keeps its native contract.
func (s *Server) stopNativeConversationWork(ctx context.Context, ws *workspaceSession, id string) error {
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		return conversationNotFound(id, err)
	}
	if !record.Native() {
		return nil
	}
	owned := ws.nativeRuntime.owns(id)
	if err := ws.takeNativeRuntime(ctx, id); err != nil {
		return err
	}
	defer func() {
		if _, live := ws.conversation.get(id); !live && !owned {
			ws.nativeRuntime.drop(id)
		}
	}()
	snapshot, live := ws.conversation.get(id)
	if !live && record.CurrentTurn != nil && !terminalTurn(record.CurrentTurn.State) && record.Work != execution.SessionIdle {
		snapshot, err = s.reopenNativeConversation(ctx, ws, id, true)
		if err != nil {
			return err
		}
	}
	lock, err := ws.conversationStore().Lock(ctx, id)
	if err != nil {
		return iox.NewInternal("locking the conversation "+id, err)
	}
	defer func() { _ = lock.Unlock() }()
	record, err = ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		return conversationNotFound(id, err)
	}
	if record.CurrentTurn != nil && !terminalTurn(record.CurrentTurn.State) && record.Work != execution.SessionIdle {
		submissionID, idErr := nativeOperationID("submission-")
		if idErr != nil {
			return iox.NewInternal("generating the interrupt correlation", idErr)
		}
		delivery := execution.SessionDelivery{SubmissionID: submissionID, TurnID: record.CurrentTurn.ID, State: execution.DeliveryUncertain}
		record.Deliveries = upsertDelivery(record.Deliveries, delivery)
		if err := ws.conversationStore().Save(ctx, record); err != nil {
			return iox.NewInternal("persisting the interrupt", err)
		}
		delivery, interruptErr := snapshot.sessionProvider.InterruptTurn(ctx, execution.SessionCommandRequest{
			Session: snapshot.session, TurnID: record.CurrentTurn.ID, SubmissionID: submissionID,
		})
		if delivery.State != "" {
			record.Deliveries = upsertDelivery(record.Deliveries, delivery)
		}
		if err := ws.conversationStore().Save(context.WithoutCancel(ctx), record); err != nil {
			return iox.NewInternal("persisting the interrupt delivery", err)
		}
		if interruptErr != nil {
			return iox.NewConflict("the native turn was not confirmed interrupted", "retry stopping it before removing its data", interruptErr)
		}
		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			observed, readErr := snapshot.sessionProvider.ReadSession(stopCtx, execution.SessionRequest{Session: snapshot.session})
			if readErr == nil && observed.Work == execution.SessionIdle {
				record.Work = observed.Work
				if observed.CurrentTurn != nil {
					turn := *observed.CurrentTurn
					if turn.ID == record.CurrentTurn.ID {
						turn.ExecutionID = record.CurrentTurn.ExecutionID
					}
					record.CurrentTurn = &turn
					record.Turns = upsertTurn(record.Turns, turn)
				} else {
					record.CurrentTurn = nil
				}
				if err := ws.conversationStore().Save(ctx, record); err != nil {
					return iox.NewInternal("persisting the interrupted turn", err)
				}
				break
			}
			select {
			case <-stopCtx.Done():
				return iox.NewConflict("the native turn is still stopping", "retry once the turn has stopped", stopCtx.Err())
			case <-ticker.C:
			}
		}
	}
	// Completed turns without a receipt deliberately leave an action waiting.
	// An explicit stop must close those records as well.
	for _, executionID := range record.ExecutionIDs {
		ws.settleSessionAction(ctx, executionID, nil, fmt.Errorf("the action was cancelled in the conversation %s", id))
	}
	return nil
}

func (s *Server) cancelExecution(ctx context.Context, ws *workspaceSession, id string) error {
	record, err := ws.store.Get(ctx, id)
	if err != nil {
		return iox.NewInternal("reading the execution "+id, err)
	}
	if record.Status != execution.StatusRunning {
		return nil
	}
	if thread := nativeThreadOfAction(ws, id); thread != "" {
		return s.stopNativeConversationWork(ctx, ws, thread)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	target, notice, err := s.resolveRunTarget(ctx, ws, id)
	if err != nil {
		// A local batch does not need an interactive run to stop: cancelling
		// its own dispatch context ends the process and closes its record.
		provider, providerErr := s.registry.Resolve(record.ProviderID)
		if providerErr == nil && !execution.WorkOutlivesDispatcher(provider) {
			if stopped, stopErr := ws.dispatch.cancelAndWait(stopCtx, id); stopped {
				if stopErr != nil {
					return iox.NewConflict("the execution is still stopping", "retry once the execution has stopped", stopErr)
				}
				return nil
			}
			if _, ok := ws.closeOrphanedExecution(ctx, s.registry, id); ok {
				return nil
			}
		}
		return err
	}
	if target.collaborator != nil {
		if err := target.collaborator.CancelRun(ctx, target.request); err != nil {
			return mapRunRefusal(err)
		}
		provider, providerErr := s.registry.Resolve(record.ProviderID)
		if providerErr == nil && execution.WorkOutlivesDispatcher(provider) {
			// Detached work can continue after its dispatcher exits. Confirm the
			// worker's end before cancelling that dispatcher or deleting data.
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				snapshot, readErr := target.collaborator.ReadRun(stopCtx, target.request)
				if readErr == nil && (snapshot.State == execution.RunClosed || snapshot.State == execution.RunCrashed) {
					target.follower.applySnapshot(snapshot)
					break
				}
				select {
				case <-stopCtx.Done():
					return iox.NewConflict("the remote execution is still stopping", "retry once the remote worker has stopped", stopCtx.Err())
				case <-ticker.C:
				}
			}
		}
	}
	if stopped, err := ws.dispatch.cancelAndWait(stopCtx, id); stopped {
		if err != nil {
			return iox.NewConflict("the execution is still stopping", "retry once the execution has stopped", err)
		}
		return nil
	}
	if _, ok := ws.closeOrphanedExecution(ctx, s.registry, id); ok {
		return nil
	}
	if target.collaborator != nil {
		if snapshot, err := target.collaborator.ReadRun(ctx, target.request); err == nil && (snapshot.State == execution.RunClosed || snapshot.State == execution.RunCrashed) {
			ws.settleSessionAction(ctx, id, nil, fmt.Errorf("the execution was cancelled after its worker stopped"))
		}
		return nil
	}
	return iox.NewConflict("the execution has no run to cancel", notice, nil)
}

func (s *Server) cancelSpecExecutions(ctx context.Context, ws *workspaceSession, code string) error {
	records, err := ws.store.ListBySpec(ctx, code)
	if err != nil {
		return iox.NewInternal("listing the executions of "+code, err)
	}
	for _, record := range records {
		if record.Status == execution.StatusRunning {
			if err := s.cancelExecutionBeforeRemoval(ctx, ws, record.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Deletion waits for terminal work; an accepted remote cancel alone is not
// evidence that the remote agent stopped writing the artifact being removed.
func (s *Server) cancelExecutionBeforeRemoval(ctx context.Context, ws *workspaceSession, id string) error {
	if err := s.cancelExecution(ctx, ws, id); err != nil {
		return err
	}
	settled, err := ws.store.Get(ctx, id)
	if err != nil {
		return iox.NewInternal("reading the stopped execution", err)
	}
	if settled.Status == execution.StatusRunning {
		return iox.NewConflict("the execution is still stopping", "retry deletion once the execution has stopped", nil)
	}
	return nil
}
