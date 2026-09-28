package web

import (
	"context"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// An idle conversation holds a harness process for nothing: a process between
// two turns costs no tokens, but it is a quarter of a gigabyte of memory and a
// child of the viewer for every thread somebody stopped answering. Releasing
// it is not closing the conversation — the rule the session contract already
// states — and resuming is native, so the next message finds the same thread
// with the same context a moment later.
//
// The delay comes from execution.idle_release_seconds, and zero turns the
// releaser off. What is never released: a turn in flight, a turn waiting for
// an answer or an approval (releasing it would fail the turn), an archived
// conversation (nothing to release), and one whose runtime is already gone.

// idleReleaseTick is how often the releaser looks, bounded so that a short
// delay is honoured within a fraction of itself and a long one does not wake
// the viewer every few seconds for nothing.
func idleReleaseTick(idle time.Duration) time.Duration {
	tick := idle / 4
	if tick < 5*time.Second {
		return 5 * time.Second
	}
	if tick > time.Minute {
		return time.Minute
	}
	return tick
}

// runIdleReleaser releases idle conversations until the context ends.
func (ws *workspaceSession) runIdleReleaser(ctx context.Context) {
	idle := ws.cfg.Execution.IdleRelease()
	if idle <= 0 {
		return
	}
	ticker := time.NewTicker(idleReleaseTick(idle))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			ws.releaseIdleConversations(ctx, now.UTC(), idle)
		}
	}
}

// releaseIdleConversations releases every held conversation that has been
// idle for at least idle, as of now. It answers how many it released, so a
// caller can tell "nothing was idle" from "nothing was held".
func (ws *workspaceSession) releaseIdleConversations(ctx context.Context, now time.Time, idle time.Duration) int {
	if ws == nil || idle <= 0 {
		return 0
	}
	store := ws.conversationStore()
	if store == nil {
		return 0
	}
	released := 0
	for _, snapshot := range ws.conversation.list() {
		if snapshot.sessionProvider == nil {
			continue
		}
		record, err := store.GetMetadata(ctx, snapshot.id)
		if err != nil || !idleSince(record, now, idle) {
			continue
		}
		if err := ws.releaseNativeConversation(ctx, snapshot); err == nil {
			released++
		}
	}
	return released
}

// idleSince says whether a conversation has been idle for at least idle as of
// now. Idle means: a runtime attached, no turn in flight or waiting, not
// archived, and nothing said since the last turn ended longer ago than idle.
func idleSince(record conversationlog.Record, now time.Time, idle time.Duration) bool {
	if record.Connection != execution.ConnectionConnected {
		return false
	}
	if record.Archive == execution.ArchiveArchived {
		return false
	}
	if record.Work != "" && record.Work != execution.SessionIdle {
		return false
	}
	last := record.LastMessageAt
	if record.CurrentTurn != nil && record.CurrentTurn.CompletedAt != nil && record.CurrentTurn.CompletedAt.After(last) {
		last = *record.CurrentTurn.CompletedAt
	}
	if last.IsZero() {
		last = record.OpenedAt
	}
	if last.IsZero() {
		return false
	}
	return now.Sub(last) >= idle
}
