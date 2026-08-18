package web

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// fakeCollaborator is a run collaborator the test drives step by step.
//
// Its central job is recording every afterID StreamRunEvents was called with:
// that slice is the oracle of "the follower resumes from its own cursor", which
// no assertion on the resulting projection alone could establish.
type fakeCollaborator struct {
	mu sync.Mutex

	cursors     []int64
	streamCalls int
	contexts    []context.Context

	snapshot  execution.RunSnapshot
	approvals []execution.PendingApproval
	readErr   error

	runID string

	// Commands: what the double received, and what it refuses. They exist so an
	// assertion can be made on the option id and the message text that really
	// reached the provider, not only on the status the route answered.
	messages          []string
	approvalResponses [][2]string
	cancels           int
	messageErr        error
	approvalErr       error
	cancelErr         error

	// streams hands the test one channel per subscription, so it can emit events
	// and end the stream exactly when it wants.
	streams chan *fakeStream
}

// fakeStream is one open subscription.
type fakeStream struct {
	events chan execution.RunEvent
	// end carries the error the stream returns; a nil value ends it cleanly.
	end chan error
}

func newFakeCollaborator() *fakeCollaborator {
	return &fakeCollaborator{
		snapshot:  execution.RunSnapshot{RunID: "run-9", State: execution.RunActive},
		approvals: []execution.PendingApproval{},
		runID:     "run-9",
		streams:   make(chan *fakeStream, 16),
	}
}

func (c *fakeCollaborator) ResolveRun(context.Context, execution.Execution, map[string]any) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runID, nil
}

func (c *fakeCollaborator) ReadRun(context.Context, execution.RunRequest) (execution.RunSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return execution.RunSnapshot{}, c.readErr
	}
	return c.snapshot, nil
}

func (c *fakeCollaborator) ReadRunApprovals(context.Context, execution.RunRequest) ([]execution.PendingApproval, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return nil, c.readErr
	}
	out := make([]execution.PendingApproval, len(c.approvals))
	copy(out, c.approvals)
	return out, nil
}

func (c *fakeCollaborator) StreamRunEvents(ctx context.Context, _ execution.RunRequest, afterID int64, sink func(execution.RunEvent) error) error {
	stream := &fakeStream{events: make(chan execution.RunEvent, 64), end: make(chan error, 1)}
	c.mu.Lock()
	c.cursors = append(c.cursors, afterID)
	c.streamCalls++
	c.contexts = append(c.contexts, ctx)
	c.mu.Unlock()
	select {
	case c.streams <- stream:
	default:
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-stream.events:
			if err := sink(event); err != nil {
				return err
			}
		case err := <-stream.end:
			return err
		}
	}
}

func (c *fakeCollaborator) SendRunMessage(_ context.Context, _ execution.RunRequest, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.messageErr != nil {
		return c.messageErr
	}
	c.messages = append(c.messages, message)
	return nil
}

func (c *fakeCollaborator) RespondRunApproval(_ context.Context, _ execution.RunRequest, approvalID, optionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.approvalErr != nil {
		return c.approvalErr
	}
	c.approvalResponses = append(c.approvalResponses, [2]string{approvalID, optionID})
	return nil
}

func (c *fakeCollaborator) CancelRun(context.Context, execution.RunRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelErr != nil {
		return c.cancelErr
	}
	c.cancels++
	return nil
}

func (c *fakeCollaborator) setApprovals(approvals []execution.PendingApproval) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.approvals = approvals
}

func (c *fakeCollaborator) refuseAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageErr, c.approvalErr, c.cancelErr = err, err, err
}

func (c *fakeCollaborator) sentMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.messages))
	copy(out, c.messages)
	return out
}

func (c *fakeCollaborator) answeredApprovals() [][2]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][2]string, len(c.approvalResponses))
	copy(out, c.approvalResponses)
	return out
}

func (c *fakeCollaborator) cancelCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancels
}

func (c *fakeCollaborator) recordedCursors() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]int64, len(c.cursors))
	copy(out, c.cursors)
	return out
}

func (c *fakeCollaborator) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streamCalls
}

func (c *fakeCollaborator) setSnapshot(snapshot execution.RunSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshot = snapshot
}

func (c *fakeCollaborator) streamContexts() []context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]context.Context, len(c.contexts))
	copy(out, c.contexts)
	return out
}

// nextStream waits for the next subscription the follower opens.
func (c *fakeCollaborator) nextStream(t *testing.T) *fakeStream {
	t.Helper()
	select {
	case stream := <-c.streams:
		return stream
	case <-time.After(5 * time.Second):
		t.Fatal("the follower did not open a stream")
		return nil
	}
}

func event(id int64, text string) execution.RunEvent {
	return execution.RunEvent{ID: id, Seq: 1, At: time.UnixMilli(1755000000000).UTC(), Kind: "text", Text: text}
}

// waitFor polls a condition instead of sleeping for a fixed span, so the tests
// stay fast when the follower is quick and stable when the machine is slow.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startFollower(t *testing.T, collaborator execution.RunCollaborator) (*runFollowers, *runFollower) {
	t.Helper()
	followers := newRunFollowers()
	t.Cleanup(followers.closeAll)
	follower := followers.ensure(context.Background(), "exec-1", "run-9", map[string]any{}, collaborator)
	return followers, follower
}

func eventIDs(projection runProjection) []int64 {
	out := make([]int64, 0, len(projection.Events))
	for _, event := range projection.Events {
		out = append(out, event.ID)
	}
	return out
}

func sameIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestFollowerKeepsEventsOrderedAndDeduplicated(t *testing.T) {
	collaborator := newFakeCollaborator()
	_, follower := startFollower(t, collaborator)
	stream := collaborator.nextStream(t)

	for _, id := range []int64{1, 2, 2, 3, 2} {
		stream.events <- event(id, "e")
	}
	waitFor(t, "the three distinct events", func() bool {
		return len(follower.snapshotView(0).Events) == 3
	})
	projection := follower.snapshotView(0)
	if !sameIDs(eventIDs(projection), []int64{1, 2, 3}) {
		t.Fatalf("events = %v, want 1, 2, 3 exactly once each", eventIDs(projection))
	}
	if projection.LastID != 3 {
		t.Fatalf("last id = %d, want 3", projection.LastID)
	}
}

func TestFollowerResumesFromItsCursorAfterADrop(t *testing.T) {
	collaborator := newFakeCollaborator()
	_, follower := startFollower(t, collaborator)
	stream := collaborator.nextStream(t)

	for _, id := range []int64{1, 2, 3} {
		stream.events <- event(id, "e")
	}
	waitFor(t, "the first three events", func() bool {
		return follower.snapshotView(0).LastID == 3
	})
	stream.end <- errors.New("the connection dropped")

	resumed := collaborator.nextStream(t)
	for _, id := range []int64{4, 5} {
		resumed.events <- event(id, "e")
	}
	waitFor(t, "the events after the reconnection", func() bool {
		return follower.snapshotView(0).LastID == 5
	})

	cursors := collaborator.recordedCursors()
	if len(cursors) < 2 {
		t.Fatalf("cursors = %v, want at least two subscriptions", cursors)
	}
	// The oracle: the value the follower really handed the collaborator when it
	// resumed, not the projection it ended up with.
	if cursors[1] != 3 {
		t.Fatalf("the resumed subscription used cursor %d, want 3 (cursors: %v)", cursors[1], cursors)
	}
	if got := eventIDs(follower.snapshotView(0)); !sameIDs(got, []int64{1, 2, 3, 4, 5}) {
		t.Fatalf("events = %v, want 1..5 with no gap and no repetition", got)
	}
}

// An approval the agent opens while the stream is healthy must become visible
// without anything going wrong first.
//
// This is the normal shape of an approval: the agent asks for a decision in the
// middle of a run, on a connection that is working. The event stream carries no
// approval-bearing frame, so the only way the projection can learn about it is
// by re-reading the approvals resource on its own. The test deliberately never
// drops the stream and never answers another approval — the two events that
// used to be the only triggers — because a run that needs a reconnection before
// it can be answered is a run the operator cannot answer at all.
func TestFollowerSurfacesAnApprovalOpenedOnALiveStream(t *testing.T) {
	collaborator := newFakeCollaborator()
	// The interval belongs to this set of followers alone, so shortening it here
	// cannot be read by a follower another test is still running.
	followers := newRunFollowers()
	followers.approvalInterval = 10 * time.Millisecond
	t.Cleanup(followers.closeAll)
	follower := followers.ensure(context.Background(), "exec-1", "run-9", map[string]any{}, collaborator)
	stream := collaborator.nextStream(t)

	// The run is live and quiet: the follower attached and read no approval.
	stream.events <- event(1, "working")
	waitFor(t, "the first event", func() bool {
		return follower.snapshotView(0).LastID == 1
	})
	if got := follower.snapshotView(0).Approvals; len(got) != 0 {
		t.Fatalf("approvals = %v, want none before the agent asks", got)
	}

	// The agent now asks for a decision. Nothing else happens: the stream stays
	// attached and keeps delivering events.
	collaborator.setApprovals([]execution.PendingApproval{{
		ID:       "appr-1",
		ToolName: "Bash",
		Title:    "Run the migration?",
		Options: []execution.ApprovalOption{
			{ID: "allow-once", Label: "Allow once", Kind: "allow"},
			{ID: "deny", Label: "Deny", Kind: "deny"},
		},
	}})
	stream.events <- event(2, "still working")

	waitFor(t, "the approval opened mid-run", func() bool {
		return len(follower.snapshotView(0).Approvals) == 1
	})
	projection := follower.snapshotView(0)
	if projection.Approvals[0].ID != "appr-1" {
		t.Fatalf("approval id = %q, want appr-1", projection.Approvals[0].ID)
	}
	if len(projection.Approvals[0].Options) != 2 {
		t.Fatalf("options = %v, want the two the provider declared", projection.Approvals[0].Options)
	}
	// The stream was never interrupted: one subscription, and no reconnection.
	if calls := collaborator.recordedCursors(); len(calls) != 1 {
		t.Fatalf("subscriptions = %v, want exactly one — the approval must not need a reconnection", calls)
	}

	// And an approval that is resolved remotely disappears the same way.
	collaborator.setApprovals(nil)
	waitFor(t, "the approval to be withdrawn", func() bool {
		return len(follower.snapshotView(0).Approvals) == 0
	})
}

func TestFollowerDoesNotRetryAfterATerminalRefusal(t *testing.T) {
	collaborator := newFakeCollaborator()
	_, follower := startFollower(t, collaborator)
	stream := collaborator.nextStream(t)

	stream.end <- &execution.RunCommandError{Reason: execution.RunRefusedUnauthorized, RunID: "run-9"}
	waitFor(t, "the follower to stop", func() bool {
		select {
		case <-follower.done:
			return true
		default:
			return false
		}
	})
	// Well past the initial backoff: a retry would have shown up by now.
	time.Sleep(followBackoffInitial + 200*time.Millisecond)

	if calls := collaborator.calls(); calls != 1 {
		t.Fatalf("stream opened %d times, want exactly 1 after a terminal refusal", calls)
	}
	projection := follower.snapshotView(0)
	if projection.Connected {
		t.Fatal("a follower that gave up must not report itself connected")
	}
	if projection.Notice == "" {
		t.Fatal("the reason the run can no longer be followed must be visible")
	}
}

func TestFollowerFixesTheTerminalStateWhenTheRunCloses(t *testing.T) {
	collaborator := newFakeCollaborator()
	_, follower := startFollower(t, collaborator)
	stream := collaborator.nextStream(t)

	collaborator.setSnapshot(execution.RunSnapshot{RunID: "run-9", State: execution.RunClosed})
	stream.end <- nil

	waitFor(t, "the follower to end", func() bool {
		select {
		case <-follower.done:
			return true
		default:
			return false
		}
	})
	if state := follower.snapshotView(0).Snapshot.State; state != execution.RunClosed {
		t.Fatalf("state = %q, want %q read back from the collaborator", state, execution.RunClosed)
	}
}

func TestFollowerWindowDropsFromTheHeadWithoutMovingTheCursor(t *testing.T) {
	follower := newRunFollower("exec-1", "run-9", map[string]any{}, 0)
	total := int64(maxFollowedEvents + 10)
	for id := int64(1); id <= total; id++ {
		if !follower.appendEvent(event(id, "e")) {
			t.Fatalf("event %d was rejected", id)
		}
	}
	projection := follower.snapshotView(0)
	if len(projection.Events) != maxFollowedEvents {
		t.Fatalf("kept %d events, want at most %d", len(projection.Events), maxFollowedEvents)
	}
	if projection.Events[0].ID == 1 {
		t.Fatal("the window must have dropped the oldest events")
	}
	if !projection.Truncated {
		t.Fatal("a truncated window must say so, instead of pretending the history never existed")
	}
	// The cursor is not an index into the buffer: rewinding it here would make
	// the next reconnection replay events already delivered.
	if projection.LastID != total {
		t.Fatalf("last id = %d, want %d", projection.LastID, total)
	}
}

func TestFollowerSnapshotViewFiltersByAfterID(t *testing.T) {
	follower := newRunFollower("exec-1", "run-9", map[string]any{}, 0)
	for id := int64(1); id <= 5; id++ {
		follower.appendEvent(event(id, "e"))
	}
	projection := follower.snapshotView(3)
	if !sameIDs(eventIDs(projection), []int64{4, 5}) {
		t.Fatalf("events after 3 = %v, want 4, 5", eventIDs(projection))
	}
	if projection.LastID != 5 {
		t.Fatalf("last id = %d, want 5", projection.LastID)
	}
	if got := follower.snapshotView(5); len(got.Events) != 0 {
		t.Fatalf("events after 5 = %v, want none", eventIDs(got))
	}
}

func TestFollowersEnsureIsIdempotent(t *testing.T) {
	collaborator := newFakeCollaborator()
	followers, first := startFollower(t, collaborator)
	collaborator.nextStream(t)

	second := followers.ensure(context.Background(), "exec-1", "run-9", map[string]any{}, collaborator)
	if first != second {
		t.Fatal("two ensures for the same execution must return the same follower")
	}
	time.Sleep(50 * time.Millisecond)
	if calls := collaborator.calls(); calls != 1 {
		t.Fatalf("stream opened %d times, want exactly 1", calls)
	}

	third := followers.ensure(context.Background(), "exec-1", "run-10", map[string]any{}, collaborator)
	if third == first {
		t.Fatal("a different run is a different subject and needs its own follower")
	}
	waitFor(t, "the replaced follower to stop", func() bool {
		select {
		case <-first.done:
			return true
		default:
			return false
		}
	})
	waitFor(t, "the second stream", func() bool { return collaborator.calls() == 2 })
}

func TestFollowersCloseAllStopsEveryFollower(t *testing.T) {
	collaborator := newFakeCollaborator()
	followers := newRunFollowers()
	one := followers.ensure(context.Background(), "exec-1", "run-9", map[string]any{}, collaborator)
	two := followers.ensure(context.Background(), "exec-2", "run-9", map[string]any{}, collaborator)
	collaborator.nextStream(t)
	collaborator.nextStream(t)

	followers.closeAll()

	for _, follower := range []*runFollower{one, two} {
		select {
		case <-follower.done:
		case <-time.After(5 * time.Second):
			t.Fatal("closeAll returned with a follower still running")
		}
	}
	for i, ctx := range collaborator.streamContexts() {
		if ctx.Err() == nil {
			t.Fatalf("the context of stream %d was not cancelled", i)
		}
	}
	if _, ok := followers.get("exec-1"); ok {
		t.Fatal("a closed follower must not stay in the registry")
	}
}

// TestNextBackoffResetsAfterAHealthyStream pins the reconnection policy.
//
// The regression it guards is not theoretical: a backoff that only doubles
// pins the follower at the cap after a handful of ordinary reconnections, so a
// run left open for minutes spends a fixed share of its life disconnected and
// delivers every event published in those windows late. Only a long-lived run
// exposes it, which is why it is asserted here on the decision itself.
func TestNextBackoffResetsAfterAHealthyStream(t *testing.T) {
	cases := []struct {
		name      string
		current   time.Duration
		lasted    time.Duration
		delivered bool
		want      time.Duration
	}{
		{"a stream that delivered an event is healthy", followBackoffMax, time.Millisecond, true, followBackoffInitial},
		{"a stream that simply lasted is healthy", followBackoffMax, healthyStreamAfter, false, followBackoffInitial},
		{"a stream refused on connect escalates", followBackoffInitial, 5 * time.Millisecond, false, 2 * followBackoffInitial},
		{"escalation is capped", followBackoffMax, 5 * time.Millisecond, false, followBackoffMax},
		{"escalation stops exactly at the cap", followBackoffMax / 2, 5 * time.Millisecond, false, followBackoffMax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextBackoff(tc.current, tc.lasted, tc.delivered); got != tc.want {
				t.Fatalf("nextBackoff(%v, %v, %v) = %v, want %v", tc.current, tc.lasted, tc.delivered, got, tc.want)
			}
		})
	}
}

// TestFollowerKeepsTheTerminalDiagnosisOverItsSymptom guards the notice a user
// actually reads when a run stops being followable: the reason it stopped, not
// the failure of the two reads that were attempted afterwards for the very same
// reason.
func TestFollowerKeepsTheTerminalDiagnosisOverItsSymptom(t *testing.T) {
	collaborator := newFakeCollaborator()
	collaborator.mu.Lock()
	collaborator.readErr = errors.New("401 from the hub")
	collaborator.mu.Unlock()
	_, follower := startFollower(t, collaborator)
	stream := collaborator.nextStream(t)

	stream.end <- &execution.RunCommandError{Reason: execution.RunRefusedUnauthorized, RunID: "run-9"}
	waitFor(t, "the follower to stop", func() bool {
		select {
		case <-follower.done:
			return true
		default:
			return false
		}
	})
	notice := follower.snapshotView(0).Notice
	if !strings.Contains(notice, string(execution.RunRefusedUnauthorized)) {
		t.Fatalf("notice = %q, want it to name the reason the run cannot be followed", notice)
	}
}
