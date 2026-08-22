package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// fakeClaude is a Claude Code process that never runs: it speaks the
// stream-json protocol the real binary was observed to speak — NDJSON in,
// NDJSON out, one frame per line — and emits exactly the frames a test tells it
// to. It replaces the process and nothing else: the client, the translation,
// the session and the refusals under test are the production ones.
//
// Nothing here advances on its own beyond the two answers the protocol requires
// immediately: the `system`/`init` frame that announces the process, and the
// `control_response` that acknowledges a control request. Everything else is
// emitted by the test, so no assertion can pass on a frame nobody asked for.
type fakeClaude struct {
	mu        sync.Mutex
	sent      []json.RawMessage
	lines     chan []byte
	closeOnce sync.Once
	ended     bool
	announced bool

	exitCode int
	stderr   string
	waitErr  error

	// silent keeps the process from announcing itself, which is how a process
	// that dies before the handshake behaves.
	silent bool
	// sendHook, when set, answers a user frame instead of the ordinary handling.
	// It is how a test reproduces a process that leaves in the middle of a write,
	// which is otherwise a window too narrow to hit on purpose.
	sendHook func(line []byte) error
	// controlSubtype and controlError describe how a control request is
	// answered. The default is the acknowledgement the real build sends.
	controlSubtype string
	controlError   string

	// starts, startDir, startName and startArgs record how the process was
	// spawned, which is what the provider's own tests assert.
	starts    int
	startDir  string
	startName string
	startArgs []string

	started chan struct{}
	done    chan struct{}
}

var (
	_ localrun.Process = (*fakeClaude)(nil)
	_ localrun.Starter = (*fakeClaude)(nil)
)

func newFakeClaude() *fakeClaude {
	return &fakeClaude{
		lines:          make(chan []byte, 512),
		controlSubtype: controlSuccess,
		started:        make(chan struct{}),
		done:           make(chan struct{}),
	}
}

func (f *fakeClaude) Start(_ context.Context, dir, name string, args []string) (localrun.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.startDir, f.startName = dir, name
	f.startArgs = append([]string(nil), args...)
	return f, nil
}

// spawned reports how many times the process was started, and with what.
func (f *fakeClaude) spawned() (starts int, name string, args []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.startName, append([]string(nil), f.startArgs...)
}

func (f *fakeClaude) startedIn() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startDir
}

// Send reads one frame from the process's standard input, exactly as the real
// binary does, and answers only what the protocol answers by itself.
func (f *fakeClaude) Send(line []byte) error {
	var incoming struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(line, &incoming); err != nil {
		return err
	}
	f.mu.Lock()
	hook := f.sendHook
	f.mu.Unlock()
	if hook != nil && incoming.Type == frameUser {
		return hook(line)
	}
	f.mu.Lock()
	f.sent = append(f.sent, append(json.RawMessage(nil), line...))
	announce := incoming.Type == frameUser && !f.announced && !f.silent
	if announce {
		f.announced = true
	}
	subtype, failure := f.controlSubtype, f.controlError
	f.mu.Unlock()

	if announce {
		f.emit(`{"type":"system","subtype":"init","cwd":"/workspace","model":"opus"}`)
		close(f.started)
	}
	if incoming.Type == "control_request" {
		body := map[string]any{"subtype": subtype, "request_id": incoming.RequestID}
		if failure != "" {
			body["error"] = failure
		}
		payload, err := json.Marshal(map[string]any{"type": frameControlResponse, "response": body})
		if err != nil {
			return err
		}
		f.push(payload)
	}
	return nil
}

// emit publishes one raw frame, exactly as the process would write it.
func (f *fakeClaude) emit(frame string) { f.push([]byte(frame)) }

func (f *fakeClaude) push(payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended {
		return
	}
	f.lines <- payload
}

// end closes the process's output, which is how a real process disappears.
func (f *fakeClaude) end() {
	f.closeOnce.Do(func() {
		f.mu.Lock()
		f.ended = true
		f.mu.Unlock()
		close(f.lines)
		close(f.done)
	})
}

// onSend installs the answer to the next user frames.
func (f *fakeClaude) onSend(hook func(line []byte) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendHook = hook
}

// alive reports whether the process is still there, which is what a cancellation
// that closed its standard input would have changed.
func (f *fakeClaude) alive() bool {
	select {
	case <-f.done:
		return false
	default:
		return true
	}
}

func (f *fakeClaude) Lines() <-chan []byte { return f.lines }

func (f *fakeClaude) Signal() error { f.end(); return nil }

func (f *fakeClaude) Wait() (int, string, error) { return f.exitCode, f.stderr, f.waitErr }

func (f *fakeClaude) Close() error { f.end(); return nil }

// framesReceived is what the process was really written, frame by frame.
func (f *fakeClaude) framesReceived() []json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]json.RawMessage(nil), f.sent...)
}

// messagesReceived is the text of every user frame the process was written,
// which is how an operator message reaches a live turn.
func (f *fakeClaude) messagesReceived() []string {
	out := make([]string, 0, 4)
	for _, frame := range f.framesReceived() {
		var payload struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(frame, &payload) != nil || payload.Type != frameUser {
			continue
		}
		if payload.Message.Role != "user" || len(payload.Message.Content) == 0 {
			continue
		}
		// Every text block, and not only the first: one frame can carry the held
		// opening instruction and the first message of the person, and a helper
		// that read only the head of it would report the message as never sent.
		for _, block := range payload.Message.Content {
			if block.Type == "text" {
				out = append(out, block.Text)
			}
		}
	}
	return out
}

// userFrame renders the frame the process re-emits for an operator message when
// it runs with --replay-user-messages.
func userFrame(text string, replay bool) string {
	payload, err := json.Marshal(map[string]any{
		"type":     frameUser,
		"message":  map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": text}}},
		"isReplay": replay,
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// openStreamSession starts a client against the fake and returns both, already
// past the handshake.
func openStreamSession(t *testing.T, fake *fakeClaude) (*streamSession, *localrun.Session) {
	t.Helper()
	return openStreamSessionMode(t, fake, false)
}

// openConversation is the same handshake in the mode where the end of a turn is
// the agent's question and not the end of the work.
func openConversation(t *testing.T, fake *fakeClaude) (*streamSession, *localrun.Session) {
	t.Helper()
	return openStreamSessionMode(t, fake, true)
}

func openStreamSessionMode(t *testing.T, fake *fakeClaude, conversational bool) (*streamSession, *localrun.Session) {
	t.Helper()
	session := localrun.NewSession("run-1", nil)
	client := newStreamSession(fake, session, conversational)
	go client.consume()
	if err := client.start(context.Background(), "PROMPT"); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	session.AttachDialogue(client)
	t.Cleanup(fake.end)
	return client, session
}

// lastEventID is the cursor a test resumes from, so what is asserted is what
// the case itself produced and not the frame that announced the process.
func lastEventID(session *localrun.Session) int64 {
	events := session.Events(0)
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].ID
}

// waitFor polls a condition instead of sleeping for an arbitrary time.
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the expected state never arrived")
}

// The session is opened by writing the instruction as the first user frame, and
// the client waits for the process to announce itself before considering the
// run live: a dialogue attached to a process that never came up would deliver
// every command into nothing.
func TestStreamSessionStartsWithTheUserFrameAndWaitsForTheAnnouncement(t *testing.T) {
	fake := newFakeClaude()
	_, session := openStreamSession(t, fake)

	if got := fake.messagesReceived(); len(got) != 1 || got[0] != "PROMPT" {
		t.Fatalf("the process received %v; want exactly one user frame carrying the prompt", got)
	}
	var first struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"message"`
	}
	frames := fake.framesReceived()
	if err := json.Unmarshal(frames[0], &first); err != nil {
		t.Fatal(err)
	}
	if first.Type != frameUser || first.Message.Role != "user" || len(first.Message.Content) != 1 || first.Message.Content[0].Type != "text" {
		t.Fatalf("the first frame is not a user text frame: %s", frames[0])
	}
	// The announcement is protocol, not history: it says the session is open
	// and nothing about what the agent did. It must leave the history empty,
	// because a Claude run whose first event is one only Claude can produce
	// would read differently from the same work run through Codex — and the
	// history of a local run is deliberately neutral about where it happened.
	if events := session.Events(0); len(events) != 0 {
		t.Fatalf("the announcement entered the history: %#v", events)
	}
}

// A process that dies before announcing itself fails the start, and the
// diagnostic says what happened instead of leaving the caller in a run that
// never began.
func TestStreamSessionFailsWhenTheProcessDiesBeforeAnnouncingItself(t *testing.T) {
	fake := newFakeClaude()
	fake.silent = true
	session := localrun.NewSession("run-1", nil)
	client := newStreamSession(fake, session, false)
	go client.consume()

	failed := make(chan error, 1)
	go func() { failed <- client.start(context.Background(), "PROMPT") }()
	waitFor(t, func() bool { return len(fake.messagesReceived()) == 1 })
	fake.end()

	err := <-failed
	if err == nil {
		t.Fatal("expected the start to fail when the process never came up")
	}
	if !strings.Contains(err.Error(), "ended before announcing itself") {
		t.Fatalf("the diagnostic does not name the cause: %v", err)
	}
}

// AC-2 — a whole conversation becomes a history in the order it arrived, with
// the kinds and the tool names the run really had.
func TestStreamSessionTranslatesAWholeConversation(t *testing.T) {
	fake := newFakeClaude()
	_, session := openStreamSession(t, fake)
	after := lastEventID(session)

	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"pianifico US-039"}]}}`)
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}}]}}`)
	fake.emit(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok","is_error":false}]}}`)
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"fatto"}]}}`)
	fake.emit(`{"type":"result","subtype":"success","is_error":false,"result":"tutto a posto"}`)

	waitFor(t, func() bool { return len(session.Events(after)) == 5 })

	want := []struct {
		kind string
		text string
		tool string
	}{
		{localrun.KindText, "pianifico US-039", ""},
		{localrun.KindToolStart, "", "Bash"},
		{localrun.KindToolEnd, "ok", "Bash"},
		{localrun.KindText, "fatto", ""},
		{localrun.KindTurnEnd, "", ""},
	}
	events := session.Events(after)
	if len(events) != len(want) {
		t.Fatalf("the conversation produced %d events: %#v", len(events), events)
	}
	for i, expected := range want {
		event := events[i]
		if event.Kind != expected.kind || event.Text != expected.text || event.Tool != expected.tool {
			t.Fatalf("event %d = %#v; want kind=%q text=%q tool=%q", i, event, expected.kind, expected.text, expected.tool)
		}
		if len(event.Raw) == 0 {
			t.Fatalf("event %d lost the original frame", i)
		}
	}
}

// A tool that failed is a different moment from a tool that succeeded, and the
// history has to keep them apart.
func TestStreamSessionReportsAFailedToolResultAsAnError(t *testing.T) {
	fake := newFakeClaude()
	_, session := openStreamSession(t, fake)
	after := lastEventID(session)

	fake.emit(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_7","name":"Bash"}]}}`)
	fake.emit(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_7","content":"exit status 1","is_error":true}]}}`)
	waitFor(t, func() bool { return len(session.Events(after)) == 2 })

	last := session.Events(after)[1]
	if last.Kind != localrun.KindToolError || last.Text != "exit status 1" || last.Tool != "Bash" {
		t.Fatalf("the failed tool result = %#v", last)
	}
}

// A frame this build has never seen still produces an event: a translation that
// dropped it would silently lose history the day Claude Code adds a type.
func TestStreamSessionNeverDropsAnUnknownFrame(t *testing.T) {
	fake := newFakeClaude()
	_, session := openStreamSession(t, fake)
	after := lastEventID(session)

	const frame = `{"type":"somethingNobodyHasSeen","payload":{"deeply":"nested"}}`
	fake.emit(frame)
	waitFor(t, func() bool { return len(session.Events(after)) == 1 })

	event := session.Events(after)[0]
	if event.Kind != "somethingNobodyHasSeen" {
		t.Fatalf("kind = %q, want the frame type itself", event.Kind)
	}
	if string(event.Raw) != frame {
		t.Fatalf("raw = %s, want the original line untouched", event.Raw)
	}
}

// The two frames that are known to carry no history at all are the only ones
// dropped: a billing counter and the incremental counter re-sent while the
// agent thinks would flood a history without adding a moment to it.
func TestStreamSessionIgnoresTheFramesThatCarryNoHistory(t *testing.T) {
	fake := newFakeClaude()
	_, session := openStreamSession(t, fake)
	after := lastEventID(session)

	fake.emit(`{"type":"rate_limit_event","rate_limits":{"used_pct":12}}`)
	fake.emit(`{"type":"system","subtype":"thinking_tokens","thinking_tokens":128}`)
	// A frame that does carry history follows, so the assertion below fails on a
	// dropped frame instead of merely on a slow one.
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"ci sono"}]}}`)
	waitFor(t, func() bool { return len(session.Events(after)) == 1 })

	events := session.Events(after)
	if len(events) != 1 || events[0].Kind != localrun.KindText || events[0].Text != "ci sono" {
		t.Fatalf("the noise entered the history: %#v", events)
	}
}

// The opening instruction is replayed like every other user frame and must not
// enter the history: a conversation opens on an empty transcript, and what a
// reader sees first is what a person really wrote.
func TestStreamSessionKeepsTheOpeningPromptOutOfTheHistory(t *testing.T) {
	fake := newFakeClaude()
	client, session := openConversation(t, fake)

	fake.emit(userFrame("PROMPT", true))
	// A frame that does carry history follows, so the assertion below fails on a
	// dropped frame instead of merely on a slow one.
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"eccomi"}]}}`)
	waitFor(t, func() bool { return countEvents(session.Events(0), localrun.KindText) == 1 })

	if n := countEvents(session.Events(0), localrun.KindUserMessage); n != 0 {
		t.Fatalf("the opening prompt entered the history: %#v", session.Events(0))
	}

	// Only the opening one is silent. A message the person writes — even the
	// same words the prompt happened to use — is history like any other.
	if err := client.Send(context.Background(), "PROMPT"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	fake.emit(userFrame("PROMPT", true))
	waitFor(t, func() bool { return countEvents(session.Events(0), localrun.KindUserMessage) == 1 })
}

// A held instruction and the first message travel in one frame, and a build
// that replayed that frame as a single joined block must still leave only the
// instruction out: what the person wrote keeps its place in the history.
func TestStreamSessionKeepsAJoinedOpeningEchoOutOfTheHistory(t *testing.T) {
	fake := newFakeClaude()
	session := localrun.NewSession("run-held", nil)
	client := newStreamSession(fake, session, true)
	go client.consume()
	client.hold("PROMPT")
	session.AttachDialogue(client)
	t.Cleanup(fake.end)

	const first = "di cosa parla questo workspace?"
	if err := client.Send(context.Background(), first); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	fake.emit(userFrame("PROMPT\n\n"+first, true))
	waitFor(t, func() bool { return countEvents(session.Events(0), localrun.KindUserMessage) == 1 })

	if got := session.Events(0)[0].Text; got != first {
		t.Fatalf("the history opens on %q; want only what the person wrote", got)
	}
}

// AC-2, AC-3 — the message travels to the process and becomes history only when
// the process re-emits it, once.
func TestStreamSessionSendsTheMessageAndWaitsForTheReEmission(t *testing.T) {
	fake := newFakeClaude()
	client, session := openStreamSession(t, fake)
	after := lastEventID(session)

	const sentinel = "cambia il criterio due"
	if err := client.Send(context.Background(), sentinel); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if got := fake.messagesReceived(); len(got) != 2 || got[1] != sentinel {
		t.Fatalf("the process received %v; want the prompt and then %q", got, sentinel)
	}
	if events := session.Events(after); len(events) != 0 {
		t.Fatalf("the message entered the history before the process re-emitted it: %#v", events)
	}

	fake.emit(userFrame(sentinel, true))
	waitFor(t, func() bool { return len(session.Events(after)) == 1 })
	event := session.Events(after)[0]
	if event.Kind != localrun.KindUserMessage || event.Text != sentinel {
		t.Fatalf("the re-emitted message = %#v", event)
	}

	// It appears once and not twice: a client that also wrote it locally would
	// show the operator two copies of the same moment.
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"ricevuto"}]}}`)
	waitFor(t, func() bool { return len(session.Events(after)) == 2 })
	messages := 0
	for _, event := range session.Events(after) {
		if event.Kind == localrun.KindUserMessage {
			messages++
		}
	}
	if messages != 1 {
		t.Fatalf("the message appears %d times in the history", messages)
	}
}

// A turn that is over is a decision the caller branches on, not a fault.
func TestStreamSessionRefusesAMessageAfterTheTurnEnded(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openStreamSession(t, fake)
	fake.emit(`{"type":"result","subtype":"success","is_error":false,"result":"finito"}`)
	<-client.TurnDone()

	err := client.Send(context.Background(), "ci sei ancora?")
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("Send got %v; want a run_not_active refusal", err)
	}
}

// AC-4 — interrupting is a delivery, not a verdict: the control request is
// written, its answer is correlated by request id, and nothing about the state
// of the run is written.
func TestStreamSessionInterruptsWithoutClosingTheSession(t *testing.T) {
	fake := newFakeClaude()
	client, session := openStreamSession(t, fake)
	after := lastEventID(session)

	if err := client.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	var request struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Request   struct {
			Subtype string `json:"subtype"`
		} `json:"request"`
	}
	frames := fake.framesReceived()
	if err := json.Unmarshal(frames[len(frames)-1], &request); err != nil {
		t.Fatal(err)
	}
	if request.Type != "control_request" || request.Request.Subtype != "interrupt" || request.RequestID == "" {
		t.Fatalf("the interrupt frame = %s", frames[len(frames)-1])
	}
	if !session.Active() {
		t.Fatal("interrupting must not close the session: the run is over when the process says so")
	}
	select {
	case <-client.TurnDone():
		t.Fatal("interrupting ended the turn on its own instead of waiting for the process")
	default:
	}
	if events := session.Events(after); len(events) != 0 {
		t.Fatalf("interrupting wrote %#v into the history", events)
	}
}

// A control response that is not a success is the process declining the
// command, which is a decision the caller branches on by reason.
func TestStreamSessionClassifiesARefusedInterrupt(t *testing.T) {
	fake := newFakeClaude()
	fake.controlSubtype = "error"
	fake.controlError = "no turn to interrupt"
	client, session := openStreamSession(t, fake)

	err := client.Interrupt(context.Background())
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedUnsupported {
		t.Fatalf("Interrupt got %v; want an unsupported refusal", err)
	}
	if !strings.Contains(err.Error(), "no turn to interrupt") {
		t.Fatalf("the diagnostic lost the reason the process gave: %v", err)
	}
	if !session.Active() {
		t.Fatal("a refused interrupt closed the session")
	}
}

// AC-2 — only a result the process declared without an error can carry a plan,
// and the message the run ends on is the one the receipt is looked for in.
func TestStreamSessionReportsHowTheTurnEndedAndWhatItEndedOn(t *testing.T) {
	t.Run("a result that reports no error", func(t *testing.T) {
		fake := newFakeClaude()
		client, _ := openStreamSession(t, fake)
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"un testo intermedio"}]}}`)
		fake.emit(`{"type":"result","subtype":"success","is_error":false,"result":"il messaggio finale"}`)
		<-client.TurnDone()

		if !client.Completed() {
			t.Fatal("a successful result did not report the turn as completed")
		}
		if got := client.FinalMessage(); got != "il messaggio finale" {
			t.Fatalf("FinalMessage = %q, want the result field", got)
		}
	})

	t.Run("a result that reports an error", func(t *testing.T) {
		fake := newFakeClaude()
		client, _ := openStreamSession(t, fake)
		fake.emit(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":""}`)
		<-client.TurnDone()

		if client.Completed() {
			t.Fatal("a result carrying an error reported the turn as completed")
		}
	})

	t.Run("a turn that ended without a result text", func(t *testing.T) {
		fake := newFakeClaude()
		client, _ := openStreamSession(t, fake)
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"primo"}]}}`)
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"l'ultimo che ha detto"}]}}`)
		waitFor(t, func() bool { return client.FinalMessage() == "l'ultimo che ha detto" })
		fake.emit(`{"type":"result","subtype":"success","is_error":false,"result":""}`)
		<-client.TurnDone()

		if got := client.FinalMessage(); got != "l'ultimo che ha detto" {
			t.Fatalf("FinalMessage = %q, want the last assistant text as the fallback", got)
		}
	})
}

// A process whose output has ended has ended the turn too: waiting for a result
// that can no longer arrive would leave the caller inside a run that is over.
func TestStreamSessionEndsTheTurnWhenTheProcessDies(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openStreamSession(t, fake)
	fake.end()

	select {
	case <-client.TurnDone():
	case <-time.After(2 * time.Second):
		t.Fatal("the death of the process did not end the wait")
	}
	select {
	case <-client.Gone():
	case <-time.After(2 * time.Second):
		t.Fatal("the end of the output was not observed")
	}
	if client.Completed() {
		t.Fatal("a process that disappeared reported a completed turn")
	}
}

// The history keeps the order of arrival and numbers it from one, whatever the
// frames were.
func TestStreamSessionKeepsTheOrderOfArrival(t *testing.T) {
	fake := newFakeClaude()
	_, session := openStreamSession(t, fake)
	after := lastEventID(session)
	for i := 1; i <= 20; i++ {
		fake.emit(fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":"%d"}]}}`, i))
	}
	waitFor(t, func() bool { return len(session.Events(after)) == 20 })

	for i, event := range session.Events(after) {
		if event.ID != after+int64(i)+1 {
			t.Fatalf("event %d has id %d", i, event.ID)
		}
		if event.Text != fmt.Sprintf("%d", i+1) {
			t.Fatalf("event %d carries %q", i, event.Text)
		}
	}
}

// The end of a turn is not one instant: the translation records the result,
// publishes the outcome and appends the turn_end event before it closes the
// wait. A message sent inside that window opens the next turn, and the close
// that follows belongs to the turn that ended — never to the one just opened.
//
// The window is entered through the real translation and nothing is simulated:
// the message is sent from the session's own clock, which is called while the
// turn_end event is being appended — after the result of the turn has been
// taken and before its wait is closed. There is no sleep and no interleaving to
// hope for; the moment is reached by construction every time.
//
// A caller that lost this race would find the fresh turn already over, and a
// later Interrupt would answer it by closing the process's standard input on
// work that is still running.
func TestStreamSessionOpensTheNextTurnWhileThePreviousOneIsBeingClosed(t *testing.T) {
	fake := newFakeClaude()

	var client *streamSession
	var once sync.Once
	sent := make(chan error, 1)
	// The handshake appends nothing — the announcement is protocol and not
	// history — so the first event of this run is the turn_end below, and the
	// message is written exactly inside its window.
	session := localrun.NewSession("run-1", func() time.Time {
		once.Do(func() { sent <- client.Send(context.Background(), "la risposta alla domanda") })
		return time.Now().UTC()
	})
	client = newStreamSession(fake, session, true)
	go client.consume()
	if err := client.start(context.Background(), "PROMPT"); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	session.AttachDialogue(client)
	t.Cleanup(fake.end)

	fake.emit(`{"type":"result","subtype":"success","is_error":false,"result":"di che colore lo vuoi?"}`)
	if err := <-sent; err != nil {
		t.Fatalf("Send failed inside the window: %v", err)
	}
	// A frame that arrives after the result is the proof the translation has
	// finished closing the turn that ended, which is the close under test.
	after := lastEventID(session)
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"rosso"}]}}`)
	waitFor(t, func() bool { return len(session.Events(after)) == 1 })

	select {
	case <-client.TurnDone():
		t.Fatal("the end of the previous turn closed the turn the message had just opened")
	default:
	}

	// And the consequence the operator would see: a cancellation now stops the
	// turn instead of ending the conversation at its source.
	if err := client.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}
	if !fake.alive() {
		t.Fatal("the interrupt closed the input of a process whose turn was in progress")
	}
	frames := fake.framesReceived()
	var request struct {
		Type    string `json:"type"`
		Request struct {
			Subtype string `json:"subtype"`
		} `json:"request"`
	}
	if err := json.Unmarshal(frames[len(frames)-1], &request); err != nil {
		t.Fatal(err)
	}
	if request.Type != "control_request" || request.Request.Subtype != "interrupt" {
		t.Fatalf("the last frame is not an interrupt: %s", frames[len(frames)-1])
	}
}

// Between two turns there really is no turn, and the conversational Interrupt
// still ends the conversation at its source. The test above must not have
// turned that branch off.
func TestStreamSessionInterruptBetweenTwoTurnsClosesTheInput(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)
	fake.emit(`{"type":"result","subtype":"success","is_error":false,"result":"una domanda"}`)
	<-client.TurnDone()

	if err := client.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}
	if fake.alive() {
		t.Fatal("cancelling between two turns left the conversation open")
	}
}

// The guard on a session that is over and the write of the message cannot be one
// act: the process can leave in between. When it has, the caller must still be
// told the run is no longer there — a decision it branches on — and not handed
// the fault a failed write otherwise is.
func TestStreamSessionRefusesAMessageWhenTheProcessLeavesWhileItIsWritten(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.onSend(func([]byte) error {
		// The process leaves exactly between the guard and the write, and the end
		// of its output is observed before the write reports its failure.
		fake.end()
		<-client.Gone()
		return fmt.Errorf("write |1: broken pipe")
	})

	err := client.Send(context.Background(), "ci sei ancora?")
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("Send got %v; want a run_not_active refusal", err)
	}
}

// A write that failed while the process is still there stays a fault: nothing
// was decided, and a retry can still change the outcome.
func TestStreamSessionReportsAFailedWriteAsAFaultWhileTheProcessIsAlive(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.onSend(func([]byte) error { return fmt.Errorf("write |1: no space left on device") })

	err := client.Send(context.Background(), "ci sei ancora?")
	if err == nil {
		t.Fatal("a failed write was reported as a success")
	}
	if _, refused := execution.RefusalOf(err); refused {
		t.Fatalf("a failed write on a live process was turned into a refusal: %v", err)
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Fatalf("the diagnostic lost the cause: %v", err)
	}
}
