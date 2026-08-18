package web

import (
	"context"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// maxFollowedEvents bounds the projection of one run.
//
// The projection is a window, not an archive: the hub owns the history and can
// always replay it from a cursor, so a viewer that kept every event of a run
// that has been talking for hours would grow without a reason. When the window
// overflows the oldest events are dropped and truncated is raised, so the UI
// can say that older history is beyond the window instead of pretending it
// never existed.
const maxFollowedEvents = 2000

// Reconnection backoff. It starts short because the common cause of a dropped
// stream is an idle timeout, and caps low enough that a run stays followable
// without a reload.
const (
	followBackoffInitial = 1 * time.Second
	followBackoffMax     = 15 * time.Second
	// healthyStreamAfter is how long a stream must have lasted to count as
	// having worked, even on a run quiet enough to have published nothing.
	healthyStreamAfter = 15 * time.Second
)

// approvalRefreshInterval is how often a followed run re-reads its pending
// approvals.
//
// It exists because the event stream carries no approval-bearing frame: a
// decision the agent opens mid-run appears only in the approvals resource. A
// projection that read that resource once, on attaching, would leave the
// operator with no card and no options for the whole life of a healthy stream —
// and the run would keep waiting for an answer that could not be given. The
// cadence matches the browser's own poll, so following a run costs one more
// small request per interval and never a missed decision.
const approvalRefreshInterval = 3 * time.Second

// runFollower keeps one server-side projection of a remote run: the ordered,
// deduplicated events, the last snapshot of the run, the pending approvals and
// whether the stream is currently attached.
//
// It exists so the browser polls the viewer instead of the hub. That keeps the
// provider credential inside this process, and it makes reconnection two
// independent cursors — this one towards the hub, the browser's towards the
// viewer — so a browser that reloads never loses events and never replays them.
type runFollower struct {
	mu sync.Mutex

	executionID    string
	runID          string
	providerConfig map[string]any
	// approvalInterval is how often this follower re-reads its pending
	// approvals; it is fixed at construction and never written afterwards.
	approvalInterval time.Duration

	events    []execution.RunEvent
	lastID    int64
	truncated bool
	snapshot  execution.RunSnapshot
	approvals []execution.PendingApproval
	connected bool
	// notice is the last transport-level problem worth showing. It is additive:
	// it never replaces the projection, because a failed read is not an absence.
	notice string

	cancel context.CancelFunc
	done   chan struct{}
	// pollDone is closed when the approvals loop has stopped. It is separate
	// from done because the two goroutines end for different reasons and a
	// shutdown has to wait for both.
	pollDone chan struct{}
}

// runProjection is the read-only view of a follower at one instant.
type runProjection struct {
	Snapshot  execution.RunSnapshot
	Events    []execution.RunEvent
	LastID    int64
	Approvals []execution.PendingApproval
	Connected bool
	Truncated bool
	Notice    string
}

func newRunFollower(executionID, runID string, providerConfig map[string]any, approvalInterval time.Duration) *runFollower {
	if approvalInterval <= 0 {
		approvalInterval = approvalRefreshInterval
	}
	return &runFollower{
		executionID:      executionID,
		runID:            runID,
		providerConfig:   providerConfig,
		approvalInterval: approvalInterval,
		snapshot:         execution.RunSnapshot{RunID: runID},
		approvals:        []execution.PendingApproval{},
		done:             make(chan struct{}),
		pollDone:         make(chan struct{}),
	}
}

// appendEvent adds one event to the projection and reports whether it was new.
//
// The cursor is the event id and the deduplication is `id <= lastID`, which is
// the same rule the hub applies on its own side. It is what makes a
// reconnection invisible: the overlap a resumed stream can legitimately deliver
// is dropped here rather than rendered twice.
func (f *runFollower) appendEvent(event execution.RunEvent) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if event.ID <= f.lastID {
		return false
	}
	f.events = append(f.events, event)
	f.lastID = event.ID
	// Dropping from the head must never move lastID: it is the cursor towards
	// the hub, not an index into this buffer, and rewinding it would make the
	// next reconnection replay events already delivered.
	if len(f.events) > maxFollowedEvents {
		f.events = append([]execution.RunEvent(nil), f.events[len(f.events)-maxFollowedEvents:]...)
		f.truncated = true
	}
	return true
}

// snapshotView returns the projection as of now, with the events after afterID.
// Every slice is copied: handing out the internal ones would let the shared
// state escape the lock that protects it.
func (f *runFollower) snapshotView(afterID int64) runProjection {
	f.mu.Lock()
	defer f.mu.Unlock()
	events := make([]execution.RunEvent, 0, len(f.events))
	for _, event := range f.events {
		if event.ID > afterID {
			events = append(events, event)
		}
	}
	approvals := make([]execution.PendingApproval, len(f.approvals))
	copy(approvals, f.approvals)
	return runProjection{
		Snapshot:  f.snapshot,
		Events:    events,
		LastID:    f.lastID,
		Approvals: approvals,
		Connected: f.connected,
		Truncated: f.truncated,
		Notice:    f.notice,
	}
}

func (f *runFollower) setConnected(connected bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = connected
}

func (f *runFollower) setNotice(notice string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notice = notice
}

func (f *runFollower) cursor() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastID
}

func (f *runFollower) applySnapshot(snapshot execution.RunSnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshot = snapshot
}

func (f *runFollower) applyApprovals(approvals []execution.PendingApproval) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if approvals == nil {
		approvals = []execution.PendingApproval{}
	}
	f.approvals = approvals
}

func (f *runFollower) request() execution.RunRequest {
	return execution.RunRequest{RunID: f.runID, ProviderConfig: f.providerConfig}
}

// refresh re-reads the run state and its pending approvals.
//
// A failed read raises a notice and leaves the previous values in place: a read
// that did not happen is not the statement that the run has no state and no
// approvals.
func (f *runFollower) refresh(ctx context.Context, collaborator execution.RunCollaborator) {
	req := f.request()
	if snapshot, err := collaborator.ReadRun(ctx, req); err == nil {
		f.applySnapshot(snapshot)
	} else if ctx.Err() == nil {
		f.setNotice("the run state could not be read: " + err.Error())
	}
	if approvals, err := collaborator.ReadRunApprovals(ctx, req); err == nil {
		f.applyApprovals(approvals)
	} else if ctx.Err() == nil {
		f.setNotice("the pending approvals could not be read: " + err.Error())
	}
}

// refreshApprovals re-reads only the pending approvals.
//
// Unlike refresh it raises no notice on failure: it runs on a timer, and a
// transient read error is not news the operator has to be told about — the next
// tick will correct it, and the previous list stays in place meanwhile.
func (f *runFollower) refreshApprovals(ctx context.Context, collaborator execution.RunCollaborator) {
	approvals, err := collaborator.ReadRunApprovals(ctx, f.request())
	if err != nil {
		return
	}
	// A read that was already in flight when the run ended must not land after
	// the terminal refresh: it carries the pending list from before the run
	// closed, and writing it now would show a decision card — with live buttons
	// — on a run that has stopped, which the browser would never correct
	// because it stops polling a terminal run.
	select {
	case <-f.done:
		return
	default:
	}
	f.applyApprovals(approvals)
}

// pollApprovals keeps the pending approvals current for as long as the run is
// being followed.
//
// It stops with the follower: once follow returns, the run is terminal or
// unreachable, and a run in either state opens no further decisions. The last
// word on the approvals of a finished run therefore comes from the final
// refresh in follow, not from this loop.
func (f *runFollower) pollApprovals(ctx context.Context, collaborator execution.RunCollaborator) {
	defer close(f.pollDone)
	ticker := time.NewTicker(f.approvalInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.done:
			return
		case <-ticker.C:
			f.refreshApprovals(ctx, collaborator)
		}
	}
}

// follow consumes the run's event stream until the run ends, the refusal is
// terminal, or the context is cancelled.
func (f *runFollower) follow(ctx context.Context, collaborator execution.RunCollaborator) {
	defer close(f.done)
	backoff := followBackoffInitial
	for {
		if ctx.Err() != nil {
			f.setConnected(false)
			return
		}
		f.setConnected(true)
		f.setNotice("")
		// The cursor read here is the whole point of following on the server: the
		// stream always resumes from what this projection already holds, so the
		// browser never has to be right about anything for the history to be
		// complete.
		delivered := false
		startedAt := time.Now()
		err := collaborator.StreamRunEvents(ctx, f.request(), f.cursor(), func(event execution.RunEvent) error {
			delivered = true
			f.appendEvent(event)
			return nil
		})
		f.setConnected(false)
		if ctx.Err() != nil {
			return
		}
		if reason, refused := execution.RefusalOf(err); refused {
			// Retrying cannot change either of these two: the credential will not
			// become valid and the run will not become visible by asking again.
			if reason == execution.RunRefusedUnauthorized || reason == execution.RunRefusedNotFound {
				// The refresh runs first and the notice is written after it: those
				// two reads are about to fail for the very same reason, and letting
				// them overwrite the notice would replace the diagnosis with its
				// symptom.
				f.refresh(ctx, collaborator)
				f.setNotice("the run can no longer be followed (" + string(reason) + ")")
				return
			}
		}
		if err == nil {
			// The stream ended cleanly: the run is over. One last read fixes the
			// terminal state, which is the hub's statement and never a local
			// deduction.
			f.refresh(ctx, collaborator)
			return
		}
		f.setNotice("the run event stream dropped and is being resumed: " + err.Error())
		f.refresh(ctx, collaborator)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff, time.Since(startedAt), delivered)
	}
}

// nextBackoff decides how long to wait before the reconnection after this one.
//
// The point is the reset. A backoff that only ever doubles treats a stream that
// ran healthily for minutes exactly like one refused on connect, so after a
// handful of ordinary reconnections the follower is pinned at the cap for the
// rest of the run, and every event published inside those windows reaches the
// browser that much later. A stream that proved itself — by delivering an
// event, or simply by lasting — starts the sequence again.
func nextBackoff(current, lasted time.Duration, delivered bool) time.Duration {
	if delivered || lasted >= healthyStreamAfter {
		return followBackoffInitial
	}
	next := current * 2
	if next > followBackoffMax {
		return followBackoffMax
	}
	return next
}

// runFollowers holds one follower per execution.
type runFollowers struct {
	mu          sync.Mutex
	byExecution map[string]*runFollower

	// approvalInterval is copied into every follower this set starts. It is a
	// field rather than a package variable so a test can shorten it without
	// writing to state that the followers of another test are already reading.
	approvalInterval time.Duration
}

func newRunFollowers() *runFollowers {
	return &runFollowers{
		byExecution:      map[string]*runFollower{},
		approvalInterval: approvalRefreshInterval,
	}
}

// ensure returns the follower of an execution, starting one when there is none.
//
// It is idempotent on purpose: the browser polls this route every two seconds
// and a second stream towards the hub per poll would be both a leak and a
// source of duplicated events. A follower pointing at a different run is
// replaced, because an execution that changed run is a different subject.
func (r *runFollowers) ensure(ctx context.Context, executionID, runID string, providerConfig map[string]any, collaborator execution.RunCollaborator) *runFollower {
	if r == nil {
		return nil
	}
	// The swap is done under one hold of the lock. Releasing it between the
	// delete and the insert would let a concurrent ensure — the 2s GET racing a
	// POST is routine — publish its own follower in the gap, which this call
	// would then overwrite: the orphan would no longer be reachable from the
	// map, so closeAll could not stop it and its SSE connection to the hub would
	// outlive the viewer's shutdown.
	r.mu.Lock()
	existing, ok := r.byExecution[executionID]
	if ok && existing.runID == runID {
		r.mu.Unlock()
		return existing
	}
	follower := newRunFollower(executionID, runID, providerConfig, r.approvalInterval)
	followCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	follower.cancel = cancel
	r.byExecution[executionID] = follower
	r.mu.Unlock()
	if ok {
		// Outside the lock, and safe there: close only cancels a context.
		existing.close()
	}

	// The first read happens before the stream so a projection is available on
	// the very first poll, instead of only once an event arrives — a quiet run
	// would otherwise look like a missing one.
	go func() {
		follower.refresh(followCtx, collaborator)
		follower.follow(followCtx, collaborator)
	}()
	// The approvals have their own loop because the stream cannot carry them.
	go follower.pollApprovals(followCtx, collaborator)
	return follower
}

func (r *runFollowers) get(executionID string) (*runFollower, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	follower, ok := r.byExecution[executionID]
	return follower, ok
}

func (f *runFollower) close() {
	if f.cancel != nil {
		f.cancel()
	}
}

// closeAll cancels every follower and waits for them, bounded by the same
// window the dispatch drain uses and for the same reason: a provider that
// ignores cancellation must delay shutdown, not prevent it.
func (r *runFollowers) closeAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	followers := make([]*runFollower, 0, len(r.byExecution))
	for id, follower := range r.byExecution {
		followers = append(followers, follower)
		delete(r.byExecution, id)
	}
	r.mu.Unlock()

	for _, follower := range followers {
		follower.close()
	}
	deadline := time.After(dispatchDrainTimeout)
	for _, follower := range followers {
		// Both goroutines: the stream consumer and the approvals loop. Waiting
		// only for the first would let the second outlive the shutdown it is
		// supposed to be part of.
		select {
		case <-follower.done:
		case <-deadline:
			return
		}
		select {
		case <-follower.pollDone:
		case <-deadline:
			return
		}
	}
}
