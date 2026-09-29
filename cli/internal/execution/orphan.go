package execution

import (
	"context"
	"errors"
	"fmt"
)

// OrphanedCode is the error code of a record closed because the process that
// was carrying it is gone.
const OrphanedCode = "ORPHANED"

// DetachedWork is implemented by a provider whose work does not live in the
// process that dispatched it — a remote hub whose task keeps running whatever
// happens to the viewer. A RUNNING record of such a provider is never an
// orphan by the rules of this package: whether that work is over is for the
// remote system to say, not for a local process to infer from its own pids.
type DetachedWork interface {
	WorkOutlivesDispatcher() bool
}

// WorkOutlivesDispatcher reports whether the provider declares its work as
// detached from the process that dispatched it.
func WorkOutlivesDispatcher(provider Provider) bool {
	detached, ok := provider.(DetachedWork)
	return ok && detached.WorkOutlivesDispatcher()
}

// Orphaned reports whether a RUNNING record has lost the process carrying it.
//
// The carrier is the process named by OwnerPID. A record owned by the caller
// itself is never an orphan here — the caller is the one that knows whether it
// still holds the dispatch — and neither is one owned by another live process,
// which may be a second viewer on the same workspace still working on it. A
// record with no owner was written before owners were recorded; the process
// that wrote it is an older binary, and it is treated as gone.
//
// Whether the record belongs to a native session, or to a provider whose work
// is detached, is a question for the caller: those records are closed by the
// session or by the remote system, never by this rule.
func Orphaned(record Execution, selfPID int, alive func(pid int) bool) bool {
	if record.Status != StatusRunning {
		return false
	}
	if record.OwnerPID == 0 {
		return true
	}
	if record.OwnerPID == selfPID {
		return false
	}
	return alive == nil || !alive(record.OwnerPID)
}

// SettleOrphan closes a RUNNING record whose carrier is gone.
//
// It never claims a success and never takes anything back. The receipt that
// would have proved the work complete died with the process, so a success
// cannot be backed; and the snapshot a rollback needs — what the workspace held
// *before* the run — was held only in that process's memory, so an artifact
// found now cannot be told apart from one the workspace already had. The
// record is therefore closed FAILED, and what the run's artifact looks like
// now is written into the message, so a person knows whether there is
// something on disk to check before pressing the action again.
//
// It is idempotent through Settle: a record another observer closed first is
// returned untouched with false.
func (s *Service) SettleOrphan(ctx context.Context, executionID string, reader SpecStateReader, reason string) (Execution, bool, error) {
	confirm := func(confirmCtx context.Context, outcome *Execution) {
		if outcome.Error == nil {
			return
		}
		outcome.Error.Code = OrphanedCode
		if note := orphanArtifactNote(confirmCtx, reader, *outcome); note != "" {
			outcome.Error.Message += "; " + note
		}
	}
	return s.Settle(ctx, executionID, nil, errors.New(reason), confirm)
}

// orphanArtifactNote says whether the effect the action was meant to leave is
// on disk now. It asks the same question VerifyActionEffect asks of a claimed
// success, so an orphan and a finished run are read by one rule.
func orphanArtifactNote(ctx context.Context, reader SpecStateReader, record Execution) string {
	if reader == nil {
		return ""
	}
	if record.Action == ActionSpecDraft {
		return "any spec this run may have added to the backlog was kept: check the backlog before drafting again"
	}
	probe := record
	probe.Status = StatusSucceeded
	probe.Error = nil
	if VerifyActionEffect(ctx, reader, record.Action, record.SpecCode, &probe) != nil {
		return "no " + artifactOf(record.Action) + " it should have produced was found, and nothing was removed"
	}
	return fmt.Sprintf("the %s found on disk was kept: nothing proves this run completed it, check it before going on", artifactOf(record.Action))
}

func artifactOf(action ActionID) string {
	switch action {
	case ActionInception:
		return "PRD"
	case ActionBacklog:
		return "backlog"
	case ActionPlan:
		return "plan"
	case ActionImplement:
		return "implementation"
	case ActionReview:
		return "review dossier"
	default:
		return "result"
	}
}
