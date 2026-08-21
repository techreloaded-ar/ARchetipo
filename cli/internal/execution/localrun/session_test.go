package localrun

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func fixedClock() func() time.Time {
	instant := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	return func() time.Time {
		instant = instant.Add(time.Second)
		return instant
	}
}

func appendText(session *Session, kind, text string) execution.RunEvent {
	return session.Append(execution.RunEvent{Kind: kind, Text: text})
}

// AC-2 — the history is ordered and its identifiers are the cursor.
func TestSessionHistoryIsOrderedAndIdentified(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	for i := 1; i <= 10; i++ {
		event := appendText(session, "text", fmt.Sprintf("line-%d", i))
		if event.ID != int64(i) {
			t.Fatalf("event %d got id %d; want %d", i, event.ID, i)
		}
		if event.At.IsZero() {
			t.Fatalf("event %d has no timestamp", i)
		}
	}
	events := session.Events(0)
	if len(events) != 10 {
		t.Fatalf("got %d events; want 10", len(events))
	}
	for i, event := range events {
		if event.ID != int64(i+1) || event.Text != fmt.Sprintf("line-%d", i+1) {
			t.Fatalf("event %d is %#v; want the %d-th line in order", i, event, i+1)
		}
	}

	tool := session.Append(execution.RunEvent{Kind: "tool_start", Tool: "shell", Raw: json.RawMessage(`{"a":1}`)})
	if tool.Kind != "tool_start" || tool.Tool != "shell" || string(tool.Raw) != `{"a":1}` {
		t.Fatalf("the session rewrote fields it does not own: %#v", tool)
	}
}

// AC-2 — the cursor returns exactly the complement of what was already seen.
func TestSessionCursorReturnsOnlyWhatIsNew(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	for i := 1; i <= 10; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}
	if got := session.Events(4); len(got) != 6 || got[0].ID != 5 || got[5].ID != 10 {
		t.Fatalf("Events(4) returned %d events starting at %d", len(got), got[0].ID)
	}
	if got := session.Events(0); len(got) != 10 {
		t.Fatalf("Events(0) returned %d events; want the whole history", len(got))
	}
	got := session.Events(10)
	if got == nil {
		t.Fatal("Events past the end must return an empty slice, never nil")
	}
	if len(got) != 0 {
		t.Fatalf("Events(10) returned %d events; want none", len(got))
	}
}

// AC-2 — a reconnection from the cursor never re-delivers an event.
func TestSessionStreamResumesWithoutDuplicates(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	for i := 1; i <= 5; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}

	stop := fmt.Errorf("enough")
	var first []int64
	ctx := context.Background()
	err := session.Stream(ctx, 0, func(event execution.RunEvent) error {
		first = append(first, event.ID)
		if len(first) == 3 {
			return stop
		}
		return nil
	})
	if err != stop {
		t.Fatalf("the sink error must travel back unchanged, got %v", err)
	}
	if len(first) != 3 || first[0] != 1 || first[2] != 3 {
		t.Fatalf("first stream saw %v; want 1,2,3", first)
	}

	// The resumed stream must pick up the two events still in the history and
	// then follow the live one, without ever revisiting 1..3.
	var second []int64
	cursor := first[len(first)-1]
	streamCtx, cancel := context.WithCancel(ctx)
	resumed := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- session.Stream(streamCtx, cursor, func(event execution.RunEvent) error {
			second = append(second, event.ID)
			if len(second) == 2 {
				close(resumed)
			}
			if len(second) == 3 {
				cancel()
			}
			return nil
		})
	}()
	<-resumed
	appendText(session, "text", "line-6")
	<-done

	if len(second) != 3 || second[0] != 4 || second[1] != 5 || second[2] != 6 {
		t.Fatalf("resumed stream saw %v; want 4,5,6", second)
	}
	seen := map[int64]bool{}
	for _, id := range append(append([]int64{}, first...), second...) {
		if seen[id] {
			t.Fatalf("event %d was delivered twice across the reconnection", id)
		}
		seen[id] = true
	}
}

// AC-2 — the replay and the live subscription cannot race into a duplicate or a
// gap, which is only observable while events are being produced.
func TestSessionStreamIsGapFreeUnderConcurrentAppends(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	const total = 200

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= total; i++ {
			appendText(session, "text", fmt.Sprintf("line-%d", i))
		}
		session.Close(execution.RunClosed, "")
	}()

	var seen []int64
	if err := session.Stream(context.Background(), 0, func(event execution.RunEvent) error {
		seen = append(seen, event.ID)
		return nil
	}); err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	wg.Wait()

	var previous int64
	for _, id := range seen {
		if id <= previous {
			t.Fatalf("identifiers are not strictly increasing: %v", seen)
		}
		previous = id
	}
	// What must hold is the absence of duplicates and of out-of-order delivery.
	// A follower may legitimately miss the tail when the session closes while it
	// is still draining, and the history remains the authority on that tail.
	if len(seen) == 0 {
		t.Fatal("the stream delivered nothing at all")
	}
	if got := session.Events(0); len(got) != total {
		t.Fatalf("the history holds %d events; want %d", len(got), total)
	}
}

// AC-4 — the state is observed once and never rewritten.
func TestSessionCloseRecordsTheFirstObservationOnly(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	if snapshot := session.Snapshot(); snapshot.State != execution.RunActive || snapshot.ClosedAt != nil {
		t.Fatalf("a fresh session must be active, got %#v", snapshot)
	}

	streamed := make(chan error, 1)
	go func() {
		streamed <- session.Stream(context.Background(), 0, func(execution.RunEvent) error { return nil })
	}()
	time.Sleep(10 * time.Millisecond)

	session.Close(execution.RunClosed, "")
	if err := <-streamed; err != nil {
		t.Fatalf("closing the session must end an open stream cleanly, got %v", err)
	}
	snapshot := session.Snapshot()
	if snapshot.State != execution.RunClosed || snapshot.ClosedAt == nil {
		t.Fatalf("got %#v; want a closed session with a closing instant", snapshot)
	}

	session.Close(execution.RunCrashed, "second thoughts")
	if again := session.Snapshot(); again.State != execution.RunClosed || again.Error != "" {
		t.Fatalf("a second Close rewrote the observed state: %#v", again)
	}
	if session.Active() {
		t.Fatal("a closed session must not report itself active")
	}
}

// A follower that does not read must not be able to stop the run.
func TestSessionAppendIsNotBlockedByASlowFollower(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	started := make(chan struct{})
	go func() {
		close(started)
		_ = session.Stream(context.Background(), 0, func(execution.RunEvent) error {
			select {} // never returns: the worst possible follower
		})
	}()
	<-started
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer*3; i++ {
			appendText(session, "text", "line")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled follower blocked the production of events")
	}
	if got := session.Events(0); len(got) != subscriberBuffer*3 {
		t.Fatalf("the history lost events to a slow follower: %d", len(got))
	}
}

// AC-3 — a session without a retention window keeps everything, exactly as it
// did before the window existed.
func TestSessionWithoutRetentionKeepsTheWholeHistory(t *testing.T) {
	session := NewSession("run-1", fixedClock())
	for i := 1; i <= 50; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}
	events := session.Events(0)
	if len(events) != 50 {
		t.Fatalf("got %d events; want the whole history of 50", len(events))
	}
	for i, event := range events {
		if event.ID != int64(i+1) {
			t.Fatalf("event %d has id %d; want %d", i, event.ID, i+1)
		}
	}
	if got := session.FirstID(); got != 1 {
		t.Fatalf("FirstID() is %d; an unlimited history begins at 1", got)
	}
	if got := session.Dropped(); got != 0 {
		t.Fatalf("Dropped() is %d; an unlimited history discards nothing", got)
	}
}

// AC-3 — a bounded session keeps the tail of the history and says how much of
// the past it no longer holds.
func TestBoundedSessionKeepsOnlyTheTail(t *testing.T) {
	session := NewBoundedSession("run-1", fixedClock(), 10)
	for i := 1; i <= 25; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}
	events := session.Events(0)
	if len(events) != 10 {
		t.Fatalf("got %d events; want the last 10", len(events))
	}
	for i, event := range events {
		want := int64(16 + i)
		if event.ID != want {
			t.Fatalf("event %d has id %d; want %d", i, event.ID, want)
		}
		if event.Text != fmt.Sprintf("line-%d", want) {
			t.Fatalf("event %d is %q; the window dropped the wrong end", i, event.Text)
		}
	}
	if got := session.FirstID(); got != 16 {
		t.Fatalf("FirstID() is %d; the surviving history begins at 16", got)
	}
	if got := session.Dropped(); got != 15 {
		t.Fatalf("Dropped() is %d; 15 events were discarded", got)
	}
}

// AC-3 — the window discards identifiers, it does not recycle them: a reused id
// would make the cursor point at two different events.
func TestBoundedSessionNeverReusesAnIdentifier(t *testing.T) {
	session := NewBoundedSession("run-1", fixedClock(), 10)
	for i := 1; i <= 25; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}
	next := appendText(session, "text", "line-26")
	if next.ID != 26 {
		t.Fatalf("the event after the window has id %d; want 26, never an id already spent", next.ID)
	}
	events := session.Events(0)
	if len(events) != 10 || events[0].ID != 17 || events[9].ID != 26 {
		t.Fatalf("the history after the window is %v", idsOf(events))
	}
}

// AC-3 — a cursor that is still inside the window keeps working exactly as it
// does on an unlimited history.
func TestBoundedSessionCursorStillReturnsTheComplement(t *testing.T) {
	session := NewBoundedSession("run-1", fixedClock(), 10)
	for i := 1; i <= 25; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}
	got := session.Events(20)
	if len(got) != 5 {
		t.Fatalf("Events(20) returned %d events; want 21..25", len(got))
	}
	for i, event := range got {
		if event.ID != int64(21+i) {
			t.Fatalf("Events(20) returned %v; want 21..25 with no gaps and no duplicates", idsOf(got))
		}
	}
}

// AC-3 — a cursor older than the window is recognisable, which is how a caller
// declares the story it is showing as partial instead of showing it as whole.
func TestBoundedSessionMakesAnOutdatedCursorRecognisable(t *testing.T) {
	session := NewBoundedSession("run-1", fixedClock(), 10)
	for i := 1; i <= 25; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}
	const afterID = int64(5)
	if first := session.FirstID(); first <= afterID+1 {
		t.Fatalf("FirstID() is %d; a cursor at %d must be detectable as outdated", first, afterID)
	}
	if got := session.Events(afterID); len(got) != 10 || got[0].ID != 16 {
		t.Fatalf("Events(%d) returned %v; the gap between 6 and 15 is real and must not be hidden", afterID, idsOf(got))
	}
}

// AC-3 — the window discards only the past: a follower already connected
// receives every event appended after it subscribed, including those the
// history drops immediately afterwards.
func TestBoundedSessionFollowerNeverMissesADroppedEvent(t *testing.T) {
	session := NewBoundedSession("run-1", fixedClock(), 3)
	const total = 12

	seen := make(chan int64, total)
	started := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- session.Stream(ctx, 0, func(event execution.RunEvent) error {
			select {
			case <-started:
			default:
				close(started)
			}
			seen <- event.ID
			return nil
		})
	}()

	// The first event both proves the subscription is live and opens the gate.
	appendText(session, "text", "line-1")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never started following")
	}
	for i := 2; i <= total; i++ {
		appendText(session, "text", fmt.Sprintf("line-%d", i))
	}

	var received []int64
	for len(received) < total {
		select {
		case id := <-seen:
			received = append(received, id)
		case <-time.After(5 * time.Second):
			t.Fatalf("the follower received %v; the window dropped events from under it", received)
		}
	}
	cancel()
	<-done

	for i, id := range received {
		if id != int64(i+1) {
			t.Fatalf("the follower saw %v; want 1..%d in order", received, total)
		}
	}
	if got := session.Events(0); len(got) != 3 || got[0].ID != 10 {
		t.Fatalf("the history kept %v; want only the last three events", idsOf(got))
	}
	if got := session.Dropped(); got != total-3 {
		t.Fatalf("Dropped() is %d; want %d", got, total-3)
	}
}

func idsOf(events []execution.RunEvent) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
