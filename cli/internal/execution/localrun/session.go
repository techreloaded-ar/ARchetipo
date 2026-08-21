// Package localrun holds the rules of a run that happens on this machine: the
// history a caller can follow, the cursor it resumes from, the commands it may
// send, and the state it is allowed to report.
//
// The rules live here and not in a provider because they are not one provider's
// business. Codex and Claude both run locally and both have to behave the same
// way for the same reasons — a history that never repeats an event, a message
// that becomes history only when the process re-emits it, a cancellation that
// never invents a terminal state. Written twice, they would drift; written
// here, a provider adds only the two things that really are its own: how it
// starts its process and how it speaks to it.
//
// One invariant runs through the whole package: **the state of a session is
// observed, never derived**. Nothing but the end of the process moves a session
// out of ACTIVE. A command can be delivered, refused or fail, and none of those
// outcomes writes a state — which is what keeps the viewer from showing a run
// as closed while the process is still working.
package localrun

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// subscriberBuffer bounds what one follower may lag behind before events start
// being dropped for it. It is deliberately generous and deliberately finite: an
// unbounded channel would let a stalled browser tab grow the process without
// limit, while a blocking send would let that same tab stop the agent.
const subscriberBuffer = 256

// Dialogue is the only thing a provider has to bring: a way to hand a message
// to its live process and a way to ask it to stop. Everything else about a
// local run is already in this package.
//
// An implementation returns an *execution.RunCommandError when the process
// itself declined the command; any other error means the command could not be
// delivered, which is a fault and not a decision.
type Dialogue interface {
	Send(ctx context.Context, text string) error
	Interrupt(ctx context.Context) error
}

// Session is the history and the state of one local run.
type Session struct {
	runID string
	now   func() time.Time

	mu     sync.Mutex
	events []execution.RunEvent
	// nextID is the authority on identifiers: it counts every event the session
	// ever accepted, so an id stays monotonic and is never reused even after the
	// retention window has dropped the event that carried it. Deriving the id
	// from len(events) would reuse it the moment the window drops something.
	nextID int64
	// retain bounds how many of the most recent events the history keeps.
	// Zero — the value NewSession leaves — means unlimited.
	retain int
	// dropped counts the events the window discarded, which is how a caller can
	// tell a partial history from a whole one.
	dropped     int64
	state       execution.RunState
	reason      string
	closedAt    *time.Time
	dialogue    Dialogue
	nextSub     int
	subscribers map[int]chan execution.RunEvent
}

// NewSession opens a session in ACTIVE state with an empty history. now is
// injectable so a test owns the timestamps; it defaults to time.Now.
func NewSession(runID string, now func() time.Time) *Session {
	if now == nil {
		now = time.Now
	}
	return &Session{
		runID:       runID,
		now:         now,
		state:       execution.RunActive,
		subscribers: make(map[int]chan execution.RunEvent),
	}
}

// NewBoundedSession opens a session that keeps only the last retain events of
// its history. retain <= 0 means unlimited, which is exactly NewSession.
//
// The window exists for one reason: to let a caller declare a history as
// partial instead of showing it as if it were whole. It does not make the
// history cheaper to read and it is not a performance knob — it is what allows
// FirstID to answer "where does this story really begin", so a conversation
// held only in memory can say so rather than pretend.
func NewBoundedSession(runID string, now func() time.Time, retain int) *Session {
	session := NewSession(runID, now)
	if retain > 0 {
		session.retain = retain
	}
	return session
}

func (s *Session) RunID() string { return s.runID }

// AttachDialogue names the channel toward the live process. It is set once the
// process is up and left nil until then, so a command that arrives too early is
// refused rather than delivered into nothing.
func (s *Session) AttachDialogue(dialogue Dialogue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dialogue = dialogue
}

func (s *Session) dialogueOf() Dialogue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dialogue
}

// Append adds one event to the history and returns it as it was stored.
//
// The caller fills Kind, Text, Tool, Seq and Raw; ID and At belong to the
// session. ID is assigned under the same lock that serves a replay, which is
// what makes a reconnection unable to see an event twice: there is no instant
// at which an event is visible to one path and not yet to the other.
//
// Seq travels from the caller because only the provider knows what a "turn"
// means for its process. It is not a cursor and must never be used as one — the
// reason is written out in execution.RunEvent.
func (s *Session) Append(event execution.RunEvent) execution.RunEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A closed session still records what arrives late: the process can emit its
	// last lines after its end was observed, and dropping them would leave a
	// history that stops before the run did.
	s.nextID++
	event.ID = s.nextID
	if event.At.IsZero() {
		event.At = s.now().UTC()
	}
	s.events = append(s.events, event)
	for _, subscriber := range s.subscribers {
		select {
		case subscriber <- event:
		default:
			// A follower that does not read must not be able to stop the run. It
			// loses this event on its live channel and finds it again on its next
			// replay, because the history itself is never lossy — unless a
			// retention window has since dropped it, and then FirstID says so.
		}
	}
	// The window discards only after the id has been assigned and after the
	// event has been published: a connected follower therefore never misses an
	// event just because the history no longer keeps it, and the identifiers of
	// what survives stay exactly the ones the followers saw.
	if s.retain > 0 && len(s.events) > s.retain {
		excess := len(s.events) - s.retain
		s.events = append(s.events[:0], s.events[excess:]...)
		s.dropped += int64(excess)
	}
	return event
}

// FirstID returns the ID of the oldest event still in the history, and 0 when
// the history is empty.
//
// It is the fact a caller compares against its own cursor to decide whether
// what it is about to show is the whole story or only its tail: when FirstID is
// greater than afterID+1, the events between the two are gone for good.
func (s *Session) FirstID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return 0
	}
	return s.events[0].ID
}

// Dropped returns how many events the retention window has discarded. It is 0
// for an unlimited session, always.
func (s *Session) Dropped() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

// Close records the terminal state, once. A second call changes nothing: the
// first observation of the end is the one that counts, and a later one would
// only be a guess about a process that has already gone.
func (s *Session) Close(state execution.RunState, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != execution.RunActive {
		return
	}
	s.state = state
	s.reason = reason
	closedAt := s.now().UTC()
	s.closedAt = &closedAt
	for id, subscriber := range s.subscribers {
		close(subscriber)
		delete(s.subscribers, id)
	}
}

// Snapshot reports the run as the session observed it.
func (s *Session) Snapshot() execution.RunSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := execution.RunSnapshot{RunID: s.runID, State: s.state, Error: s.reason}
	if s.closedAt != nil {
		closedAt := *s.closedAt
		snapshot.ClosedAt = &closedAt
	}
	return snapshot
}

// Active reports whether the session can still accept a command.
func (s *Session) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == execution.RunActive
}

// Events returns the history after afterID, exclusive. It never returns nil, so
// a caller that serializes the result produces an empty array.
func (s *Session) Events(afterID int64) []execution.RunEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventsAfterLocked(afterID)
}

func (s *Session) eventsAfterLocked(afterID int64) []execution.RunEvent {
	out := make([]execution.RunEvent, 0, len(s.events))
	for _, event := range s.events {
		if event.ID > afterID {
			out = append(out, event)
		}
	}
	return out
}

// Stream replays the history after afterID and then follows the run.
//
// The replay and the subscription happen under one lock, which is the whole
// point: an event produced between the two would otherwise be delivered twice
// or not at all. It returns when the context ends, when the session closes, or
// when sink returns an error — which it propagates unchanged, because that is
// how a consumer stops a stream with an error it recognizes.
func (s *Session) Stream(ctx context.Context, afterID int64, sink func(execution.RunEvent) error) error {
	s.mu.Lock()
	backlog := s.eventsAfterLocked(afterID)
	closed := s.state != execution.RunActive
	var id int
	var live chan execution.RunEvent
	if !closed {
		live = make(chan execution.RunEvent, subscriberBuffer)
		s.nextSub++
		id = s.nextSub
		s.subscribers[id] = live
	}
	s.mu.Unlock()

	if live != nil {
		defer s.unsubscribe(id)
	}

	last := afterID
	for _, event := range backlog {
		if err := sink(event); err != nil {
			return err
		}
		last = event.ID
	}
	if live == nil {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-live:
			if !ok {
				return nil
			}
			// An event already delivered by the replay is skipped rather than
			// re-sent: the two paths overlap by at most the events produced while
			// the backlog was being drained.
			if event.ID <= last {
				continue
			}
			if err := sink(event); err != nil {
				return err
			}
			last = event.ID
		}
	}
}

func (s *Session) unsubscribe(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if subscriber, ok := s.subscribers[id]; ok {
		delete(s.subscribers, id)
		close(subscriber)
	}
}

// RawOf is a small helper for providers that keep the original payload of an
// event: it returns nil for an empty message rather than an empty json.
func RawOf(payload []byte) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(payload))
	copy(out, payload)
	return out
}
