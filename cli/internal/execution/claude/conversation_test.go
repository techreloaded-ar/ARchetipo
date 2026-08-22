package claude

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// A conversation is the one thing this provider does that is not an execution:
// no record, no action, no receipt, and nothing to wait for. These tests drive
// it through the same seam every other test of this package uses — the process
// is a double — so what is asserted is the directory the process was really
// started in, the frames it really exchanged and the state the session really
// observed, and no machine with Claude Code on it is needed to prove any of it.
const conversationID = "conv-1"

// countingClaude is the fake process with one thing added: it counts how many
// times its input was closed and how many times it was signalled. That count is
// the oracle of "closing twice releases nothing the second time", which is
// otherwise indistinguishable from a second release that happened to succeed.
type countingClaude struct {
	*fakeClaude

	mu      sync.Mutex
	closes  int
	signals int
}

var (
	_ localrun.Process = (*countingClaude)(nil)
	_ localrun.Starter = (*countingClaude)(nil)
)

func newCountingClaude() *countingClaude {
	return &countingClaude{fakeClaude: newFakeClaude()}
}

func (c *countingClaude) Start(ctx context.Context, dir, name string, args []string) (localrun.Process, error) {
	if _, err := c.fakeClaude.Start(ctx, dir, name, args); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *countingClaude) Close() error {
	c.mu.Lock()
	c.closes++
	c.mu.Unlock()
	return c.fakeClaude.Close()
}

func (c *countingClaude) Signal() error {
	c.mu.Lock()
	c.signals++
	c.mu.Unlock()
	return c.fakeClaude.Signal()
}

func (c *countingClaude) released() (closes int, signals int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closes, c.signals
}

// refusingStarter is a machine on which the process cannot be started at all.
// It is what a missing binary, an unexecutable file or an exhausted process
// table look like from inside this package.
type refusingStarter struct{}

var _ localrun.Starter = refusingStarter{}

func (refusingStarter) Start(context.Context, string, string, []string) (localrun.Process, error) {
	return nil, fmt.Errorf("no process could be started")
}

// conversationRequest is the workspace-scoped request: no spec, no action and
// no capability to satisfy, because a conversation is none of those things.
func conversationRequest(command, dir string) execution.ConversationRequest {
	return execution.ConversationRequest{
		ConversationID: conversationID,
		WorkingDir:     dir,
		ProviderConfig: map[string]any{"command": command},
	}
}

// openConversationProvider builds a provider whose availability probe is
// scripted and whose session process is the given double. The fallback working
// directory is deliberately *not* the one the conversation will ask for, so a
// process started in the fallback is a failing test rather than a passing one.
func openConversationProvider(t *testing.T, starter localrun.Starter) *Provider {
	t.Helper()
	return New(Options{
		Runner:     &fakeRunner{outcomes: []runOutcome{probeOK}},
		Starter:    starter,
		WorkingDir: func() (string, error) { return t.TempDir(), nil },
		Now:        fixedElapsedClock(1500 * time.Millisecond),
	})
}

func stateOf(t *testing.T, provider *Provider, runID string) execution.RunState {
	t.Helper()
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: runID})
	if err != nil {
		t.Fatalf("ReadRun(%q) failed: %v", runID, err)
	}
	return snapshot.State
}

// --- AC-1: the conversation runs where it was asked to, and opening returns --

// AC-1 — the process is started in the directory the request named, and opening
// hands the conversation back instead of waiting for it.
//
// The fake is deliberately kept mute: it never emits a turn and never ends, so
// an implementation that waited for an outcome would still be inside
// OpenConversation when the test's deadline arrived. Returning at all is
// therefore the oracle of "it does not block", and the recorded directory is
// the oracle of where the work will happen.
func TestOpenConversationStartsTheProcessInTheRequestedDirectoryAndReturns(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), conversationID) })

	if got := fake.startedIn(); got != dir {
		t.Fatalf("the process was started in %q, want the directory the request named (%q)", got, dir)
	}
	if starts, name, _ := fake.spawned(); starts != 1 || name != command {
		t.Fatalf("the process was started %d time(s) as %q, want exactly one %q", starts, name, command)
	}
	// The conversation is live and has produced no turn: nothing but the
	// handshake has happened, which is what "returned without waiting" means.
	if state := stateOf(t, provider, conversationID); state != execution.RunActive {
		t.Fatalf("the conversation reported %q right after it was opened", state)
	}
	if events := collectEvents(provider, conversationID, 0); len(events) != 0 {
		t.Fatalf("the conversation already holds history the process never emitted: %#v", events)
	}
}

// Opening a conversation starts no work: nothing is written to the process, so
// the agent has nothing to answer and the person finds an empty conversation
// waiting for their first message. The instruction the conversation runs under
// is delivered with that message, ahead of it and in the same frame, so it
// still arrives before anything is answered.
func TestOpenConversationWritesNothingUntilTheFirstMessage(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), conversationID) })

	if got := fake.messagesReceived(); len(got) != 0 {
		t.Fatalf("opening the conversation wrote %v to the process; it must write nothing", got)
	}

	const first = "Ciao, di cosa parla questo workspace?"
	if err := provider.SendRunMessage(context.Background(), execution.RunRequest{RunID: conversationID}, first); err != nil {
		t.Fatalf("the first message of the conversation was refused: %v", err)
	}
	got := fake.messagesReceived()
	if len(got) != 2 || got[1] != first {
		t.Fatalf("the process received %v; want the held instruction and then exactly the first message", got)
	}
	if !strings.Contains(got[0], "free conversation") {
		t.Fatalf("what travelled ahead of the first message was not the conversation instruction: %q", got[0])
	}
	// One frame and one turn: the instruction and the message arrive together,
	// so the agent answers once and not twice.
	if frames := len(fake.framesReceived()); frames != 1 {
		t.Fatalf("the first message opened %d frames; want exactly one", frames)
	}

	// The replay puts the message into the history and leaves the instruction
	// out of it.
	fake.emit(userFrame(got[0], true))
	fake.emit(userFrame(first, true))
	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, conversationID, 0), localrun.KindUserMessage) == 1
	})
	events := collectEvents(provider, conversationID, 0)
	if events[0].Text != first {
		t.Fatalf("the history opens on %q; want the first message of the person", events[0].Text)
	}
}

// --- AC-3: the conversation is followable and commandable at once ------------

// The conversation borrows the whole vocabulary of a run without borrowing its
// record: it is read with ReadRun, followed with StreamRunEvents and spoken to
// with SendRunMessage, under its own id and with no execution behind it.
//
// The three facts are one test because they are not independent: history that
// arrives is only meaningful if the conversation is live, and a message that is
// accepted is only meaningful if it reaches the process without being written
// locally first.
func TestOpenConversationIsFollowableAndCommandableAtOnce(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), conversationID) })

	const answer = "Il backlog di questo workspace ha tre epiche."
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":` + quoted(answer) + `}]}}`)
	waitFor(t, func() bool { return countEvents(collectEvents(provider, conversationID, 0), localrun.KindText) == 1 })
	if got := collectEvents(provider, conversationID, 0)[0].Text; got != answer {
		t.Fatalf("the agent's message reached the history as %q", got)
	}

	after := lastEventIDOf(provider, conversationID)
	const question = "Quante spec sono ancora TODO?"
	if err := provider.SendRunMessage(context.Background(), execution.RunRequest{RunID: conversationID}, question); err != nil {
		t.Fatalf("the message was refused by an open conversation: %v", err)
	}
	// The instruction the conversation was opened with travels with this first
	// message, ahead of it, and the person's own words follow it.
	if got := fake.messagesReceived(); len(got) != 2 || got[1] != question {
		t.Fatalf("the process received %v; want the conversation prompt and then exactly the operator's message", got)
	}
	// Nothing was written locally: a message becomes history when the process
	// re-emits it, never when it is sent.
	if pending := collectEvents(provider, conversationID, after); len(pending) != 0 {
		t.Fatalf("the message entered the history before the process re-emitted it: %#v", pending)
	}
	fake.emit(userFrame(question, true))
	waitFor(t, func() bool { return len(collectEvents(provider, conversationID, after)) == 1 })
	replayed := collectEvents(provider, conversationID, after)[0]
	if replayed.Kind != localrun.KindUserMessage || replayed.Text != question {
		t.Fatalf("the re-emitted message = %#v", replayed)
	}
}

// A turn that ends is the agent finishing an answer, not the end of the
// conversation. This is the whole difference with a dispatched action, where
// the end of the turn is the end of the work, and it is asserted explicitly
// because an implementation that reused runSingleTurn would pass every other
// test in this file.
func TestConversationSurvivesTheEndOfATurn(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), conversationID) })

	fake.emit(resultFrame("Eccole.", false))
	waitFor(t, func() bool { return countEvents(collectEvents(provider, conversationID, 0), localrun.KindTurnEnd) == 1 })

	if state := stateOf(t, provider, conversationID); state != execution.RunActive {
		t.Fatalf("the conversation reported %q after a turn ended: a finished answer is not the end of a conversation", state)
	}
	const next = "E le epiche?"
	if err := provider.SendRunMessage(context.Background(), execution.RunRequest{RunID: conversationID}, next); err != nil {
		t.Fatalf("the conversation refused a message after a turn ended: %v", err)
	}
	if got := fake.messagesReceived(); len(got) != 2 || got[1] != next {
		t.Fatalf("the process received %v after the turn ended", got)
	}
}

// --- AC-6: closing releases the process -------------------------------------

// AC-6 — closing releases the process, closes the session and makes the
// conversation no longer commandable. The refusal is branched on its reason and
// never on its message: run_not_active is a different situation, with a
// different remedy, from a run this process never held.
func TestCloseConversationReleasesTheProcessAndStopsTheCommands(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newCountingClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	if err := provider.CloseConversation(context.Background(), conversationID); err != nil {
		t.Fatalf("closing the conversation failed: %v", err)
	}

	if closes, _ := fake.released(); closes != 1 {
		t.Fatalf("the input of the process was closed %d time(s), want exactly one release", closes)
	}
	if fake.alive() {
		t.Fatal("the process is still there after the conversation was closed")
	}
	if state := stateOf(t, provider, conversationID); state == execution.RunActive {
		t.Fatal("the closed conversation still reports itself as active")
	}
	err := provider.SendRunMessage(context.Background(), execution.RunRequest{RunID: conversationID}, "ci sei ancora?")
	if err == nil {
		t.Fatal("a closed conversation accepted a message")
	}
	var refused *execution.RunCommandError
	if !errors.As(err, &refused) || refused.Reason != execution.RunRefusedNotActive {
		t.Fatalf("the refusal = %#v, want the reason run_not_active", err)
	}
	// The history of a conversation that has ended stays readable: it is the
	// registry's rule, and it is what makes the refusal above possible at all.
	if _, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: conversationID}); err != nil {
		t.Fatalf("the history of the closed conversation is gone: %v", err)
	}
}

// AC-6 — closing is idempotent. A second close answers nil and releases
// nothing, which is asserted on the count of releases rather than on the error:
// a second shutdown that happened to succeed would be indistinguishable
// otherwise.
func TestCloseConversationTwiceReleasesNothingTheSecondTime(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newCountingClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	if err := provider.CloseConversation(context.Background(), conversationID); err != nil {
		t.Fatalf("the first close failed: %v", err)
	}
	if err := provider.CloseConversation(context.Background(), conversationID); err != nil {
		t.Fatalf("the second close failed: %v", err)
	}
	if closes, signals := fake.released(); closes != 1 || signals != 0 {
		t.Fatalf("the process was closed %d time(s) and signalled %d time(s), want exactly one release", closes, signals)
	}
	// A conversation this provider never held is the same answer, for the same
	// reason: there is nothing to release.
	if err := provider.CloseConversation(context.Background(), "conv-never-opened"); err != nil {
		t.Fatalf("closing a conversation that was never opened failed: %v", err)
	}
}

// The end of the process is what ends the conversation, and it ends it without
// anybody asking. The state is observed and never deduced, so it carries the
// reason the conversation really ended — a run reported as crashed with no
// reason is a run nobody can act on.
func TestConversationEndsOnItsOwnWhenTheProcessLeaves(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newFakeClaude()
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	fake.end()

	waitFor(t, func() bool { return stateOf(t, provider, conversationID) != execution.RunActive })
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: conversationID})
	if err != nil {
		t.Fatalf("ReadRun failed after the process left: %v", err)
	}
	if snapshot.State != execution.RunCrashed {
		t.Fatalf("state = %q, want the conversation reported as crashed", snapshot.State)
	}
	if strings.TrimSpace(snapshot.Error) == "" {
		t.Fatal("the conversation does not say why it ended")
	}
	// Closing it afterwards is still an ordinary, successful no-op: nobody has
	// to know that the process had already gone.
	if err := provider.CloseConversation(context.Background(), conversationID); err != nil {
		t.Fatalf("closing an already dead conversation failed: %v", err)
	}
}

// --- refusals ---------------------------------------------------------------

// One id, one conversation. A second open under the same id is refused before
// anything is spawned, which is asserted on the number of processes started:
// a second process would be a conversation nobody could ever close, because the
// map holds one entry per id.
func TestOpenConversationRefusesAnIdItAlreadyHolds(t *testing.T) {
	command := fakeCommand(t)
	dir := t.TempDir()
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err != nil {
		t.Fatalf("opening the conversation failed: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), conversationID) })

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, dir)); err == nil {
		t.Fatal("a second conversation was opened under an id that is already held")
	}
	if starts, _, _ := fake.spawned(); starts != 1 {
		t.Fatalf("the process was started %d time(s), want exactly one for one conversation", starts)
	}
	if state := stateOf(t, provider, conversationID); state != execution.RunActive {
		t.Fatalf("the refused second open disturbed the live conversation: state = %q", state)
	}
}

// A conversation without an id cannot be opened, because the id is what it will
// later be read and closed by.
func TestOpenConversationRefusesAnEmptyId(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := openConversationProvider(t, fake)

	req := conversationRequest(command, t.TempDir())
	req.ConversationID = "  "
	if err := provider.OpenConversation(context.Background(), req); err == nil {
		t.Fatal("a conversation without an id was opened")
	}
	if starts, _, _ := fake.spawned(); starts != 0 {
		t.Fatalf("a process was started %d time(s) for a conversation that has no id", starts)
	}
}

// An open that failed leaves nothing behind. A session registered and left
// ACTIVE would be a conversation the viewer would offer to follow and that
// nobody would ever be able to close.
func TestFailedOpenLeavesNoActiveConversation(t *testing.T) {
	command := fakeCommand(t)
	provider := openConversationProvider(t, refusingStarter{})

	if err := provider.OpenConversation(context.Background(), conversationRequest(command, t.TempDir())); err == nil {
		t.Fatal("a conversation was opened on a machine where no process can start")
	}
	if state := stateOf(t, provider, conversationID); state == execution.RunActive {
		t.Fatal("a failed open left an ACTIVE conversation nobody will ever close")
	}
	// The id is free again: a failed attempt must not make the next one
	// impossible.
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider.starter = fake
	if err := provider.OpenConversation(context.Background(), conversationRequest(command, t.TempDir())); err != nil {
		t.Fatalf("the id was still held after a failed open: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), conversationID) })
}
