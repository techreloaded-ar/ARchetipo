package web

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// nativeRuntimeHolds is the set of native conversations whose runtime *this*
// View process owns.
//
// A native session is not attached to, it is re-started on: resuming one spawns
// a harness process against the provider's own session id. Two View processes
// opened on the same workspace would each spawn one, and the two would then be
// writing the same native transcript — which is the one thing a durable session
// cannot survive. The journal is already safe, because its appender takes a
// cross-process lock; the harness runtime was not.
//
// The hold is that very same conversation lock, taken for as long as this
// process holds the session and released with it, so there is no second
// ownership mechanism to learn. A View that died holding one leaves a lock
// whose owner pid is gone, and the next process recovers it — a crash cannot
// make a conversation permanently unusable.
type nativeRuntimeHolds struct {
	mu    sync.Mutex
	locks map[string]*conversationlog.ConversationLock
}

func newNativeRuntimeHolds() *nativeRuntimeHolds {
	return &nativeRuntimeHolds{locks: map[string]*conversationlog.ConversationLock{}}
}

// runtimeLockID is the lock a native runtime is owned through. It is a sibling
// of the record lock and of the journal's append lock, and deliberately not
// either of them: those two are held for the length of one write and of one
// stream, while this one is held for as long as the process is alive.
func runtimeLockID(id string) string { return id + "-runtime" }

// take claims the runtime of one conversation for this process. Claiming one
// already held by this process succeeds and changes nothing, so a caller never
// has to know whether it is the first to ask.
func (h *nativeRuntimeHolds) take(ctx context.Context, store *conversationlog.FileStore, id string) error {
	if h == nil || store == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.locks == nil {
		h.locks = map[string]*conversationlog.ConversationLock{}
	}
	if _, held := h.locks[id]; held {
		return nil
	}
	lock, err := store.TryLock(ctx, runtimeLockID(id))
	if err != nil {
		return err
	}
	h.locks[id] = lock
	return nil
}

func (h *nativeRuntimeHolds) owns(id string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, held := h.locks[id]
	return held
}

func (h *nativeRuntimeHolds) drop(id string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	lock := h.locks[id]
	delete(h.locks, id)
	h.mu.Unlock()
	if lock != nil {
		_ = lock.Unlock()
	}
}

// dropAll releases every runtime this process owned. It is what the end of a
// workspace session calls, after the harness runtimes themselves have been
// released: a lock left behind by a process that is still alive is one no other
// View could ever recover.
func (h *nativeRuntimeHolds) dropAll() {
	if h == nil {
		return
	}
	h.mu.Lock()
	locks := h.locks
	h.locks = map[string]*conversationlog.ConversationLock{}
	h.mu.Unlock()
	for _, lock := range locks {
		_ = lock.Unlock()
	}
}

// takeNativeRuntime claims the runtime of one conversation and turns a refusal
// into the sentence a person can act on. Every route that is about to start or
// resume a harness process for an existing conversation goes through it.
func (ws *workspaceSession) takeNativeRuntime(ctx context.Context, id string) error {
	err := ws.nativeRuntime.take(ctx, ws.conversationStore(), id)
	if err == nil {
		return nil
	}
	if errors.Is(err, conversationlog.ErrLockHeld) {
		return iox.NewConflict(
			"the conversation "+id+" is held by another View process on this workspace",
			"continue it there, or stop that viewer: two runtimes on one native session would write the same transcript",
			nil,
		)
	}
	return iox.NewInternal("claiming the native runtime of the conversation "+id, err)
}

// heldByAnotherViewer reports whether a live process other than this one owns
// the runtime of a conversation.
//
// It is a probe and not a claim: a lock it manages to take is handed straight
// back, because owning a session this process is not going to run would keep
// every other viewer out of it for nothing. It exists so a refusal can say
// "another viewer is holding this" instead of "this conversation has ended",
// which is what a viewer that did not restore the thread would otherwise report
// about a thread that is very much alive next door.
func (ws *workspaceSession) heldByAnotherViewer(ctx context.Context, id string) bool {
	if ws == nil || ws.nativeRuntime.owns(id) {
		return false
	}
	store := ws.conversationStore()
	if store == nil {
		return false
	}
	lock, err := store.TryLock(ctx, runtimeLockID(id))
	if err != nil {
		return errors.Is(err, conversationlog.ErrLockHeld)
	}
	_ = lock.Unlock()
	return false
}

// requireSessionDirectory refuses to start or resume a native runtime whose
// working directory is gone.
//
// The directory a session runs in is frozen when the session is created and is
// never moved, so a git worktree that has since been removed leaves the
// conversation pointing at a path that no longer exists. Without this check the
// refusal comes from the operating system through the harness, as a chdir
// failure naming neither the conversation nor the gesture that has to be
// undone; and the only alternative to refusing would be to pick another
// directory, running the work somewhere nobody chose. Nothing is deleted: the
// record, the journal and the native reference stay exactly where they are, and
// restoring the directory is enough to go on.
func requireSessionDirectory(snapshot conversationSnapshot) error {
	dir := strings.TrimSpace(snapshot.workingDir)
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err == nil && info.IsDir() {
		return nil
	}
	return iox.NewConflict(
		"the working directory "+dir+" of the conversation "+snapshot.id+" is no longer reachable",
		"restore that directory — a git worktree that was removed can be recreated — and try again; the conversation and its native session are kept meanwhile",
		err,
	)
}

// reconcileInterruptedTurn closes a turn that the durable record still calls
// running, at the only moment it is certain nobody is running it: the start of
// a View process that has just claimed the runtime.
//
// A harness runtime is a child of the View process that started it, and the
// provider's handle on it lives in that process's memory. Whichever way View
// ended — a clean shutdown that could not reach the provider, a crash, a
// machine that was switched off — a turn left ACTIVE in the record is a turn no
// observer survives. Resuming the session later starts a *new* process on the
// same native id, and that process knows nothing of the turn that was in
// flight.
//
// Without this the record would keep saying TURN_ACTIVE for ever, and an action
// carried out in that turn would keep an execution RUNNING that nothing could
// ever close. Reconciling here runs before the provider is resolved on purpose:
// a conversation whose provider is gone, whose directory has vanished or whose
// session cannot be resumed is exactly the one that would otherwise be left
// out.
func (ws *workspaceSession) reconcileInterruptedTurn(ctx context.Context, record conversationlog.Record) {
	turnRunning := record.CurrentTurn != nil && !terminalTurn(record.CurrentTurn.State)
	working := record.Work != "" && record.Work != execution.SessionIdle
	if !turnRunning && !working {
		return
	}
	lock, err := ws.conversationStore().Lock(ctx, record.ID)
	if err != nil {
		return
	}
	stored, err := ws.conversationStore().GetMetadata(ctx, record.ID)
	if err != nil {
		_ = lock.Unlock()
		return
	}
	if stored.CurrentTurn != nil && !terminalTurn(stored.CurrentTurn.State) {
		stored.CurrentTurn.State = execution.TurnFailed
		stored.CurrentTurn.Error = "View was restarted while this turn was running: the harness runtime that was carrying it did not survive"
		stored.Turns = upsertTurn(stored.Turns, *stored.CurrentTurn)
	}
	stored.Work = execution.SessionIdle
	// A runtime the operator released stays released: what the restart found is
	// a turn that nobody was carrying any more, not a session that dropped.
	// Overwriting it would tell a person their thread had fallen over when they
	// had closed it themselves.
	if stored.Connection != execution.ConnectionReleased {
		stored.Connection = execution.ConnectionDisconnected
	}
	_ = ws.conversationStore().Save(ctx, stored)
	_ = lock.Unlock()
	// Only now, and outside the lock the settle path takes for itself: the
	// action the turn was carrying out is closed from the state just written,
	// so no execution is left RUNNING with nobody able to end it.
	ws.settleSessionActions(ctx, record.ID)
}
