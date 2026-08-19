package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// The stream-json protocol, as observed against Claude Code 2.1.235. The
// process reads NDJSON on its standard input and writes NDJSON on its standard
// output, one frame per line: it announces itself with a `system` frame of
// subtype `init`, streams the turn as `assistant` and `user` frames, and closes
// it with a `result` frame. A live turn is steered by writing another `user`
// frame and stopped with a `control_request`.
//
// The names below are the ones the binary really answers to. They were taken
// from sessions driven by hand against the installed build, not from
// documentation, because what matters here is what this build accepts and not
// what the format can express.
const (
	frameSystem          = "system"
	frameAssistant       = "assistant"
	frameUser            = "user"
	frameResult          = "result"
	frameControlResponse = "control_response"

	subtypeInit    = "init"
	controlSuccess = "success"
)

// frame is the part of a stream-json frame this client interprets. Everything
// else survives untouched in the raw line, which is what every event carries:
// narrowing what is read must never narrow what is kept.
type frame struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`
	Result  string          `json:"result"`
	IsError bool            `json:"is_error"`
}

// controlResponse is the answer to a control request. The correlation key is
// inside the nested object, not next to the frame type.
type controlResponse struct {
	Response struct {
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	} `json:"response"`
}

// controlOutcome is what a waiter on a control request receives.
type controlOutcome struct {
	subtype string
	err     string
}

// streamSession drives one live `claude` process over stream-json and projects
// its frames into a local session. It is the only place in this package that
// knows the protocol.
type streamSession struct {
	process localrun.Process
	session *localrun.Session

	mu                sync.Mutex
	seq               int
	completed         bool
	finalMessage      string
	lastAssistantText string
	nextControl       int
	pending           map[string]chan controlOutcome
	// tools maps a tool_use_id to the name of the tool that opened it, so the
	// result frame can name the tool it belongs to. It is cleared at the end of
	// every turn: a map that lived for the whole session would grow with the
	// work instead of with the turn.
	tools map[string]string

	readyOnce sync.Once
	ready     chan struct{}
	turnOnce  sync.Once
	turnDone  chan struct{}
	gone      chan struct{}
}

var _ localrun.Dialogue = (*streamSession)(nil)

func newStreamSession(process localrun.Process, session *localrun.Session) *streamSession {
	return &streamSession{
		process: process,
		session: session,
		// The first turn is seq 1, not 0. Codex numbers its turns from one, and
		// the seq of an event is served to the caller beside its kind: a run
		// that started at zero would say which provider produced it just as
		// plainly as a provider-specific kind would.
		seq:      1,
		pending:  make(map[string]chan controlOutcome),
		tools:    make(map[string]string),
		ready:    make(chan struct{}),
		turnDone: make(chan struct{}),
		gone:     make(chan struct{}),
	}
}

// consume reads the process until its output ends. It runs on its own goroutine
// for the whole life of the session and is the only reader.
//
// A process whose output has ended has ended the turn too: waiting for a
// `result` that can no longer arrive would leave the caller inside a run that
// is already over.
func (s *streamSession) consume() {
	defer close(s.gone)
	defer s.endTurn()
	for line := range s.process.Lines() {
		var f frame
		if err := json.Unmarshal(line, &f); err != nil {
			// A malformed line is not worth ending a live session over: the
			// process keeps producing history and the next line is very likely
			// readable.
			continue
		}
		if f.Type == frameControlResponse {
			// The answer to a command is not history: it says whether the command
			// was taken, and what the process then does with it arrives as
			// ordinary frames.
			s.settle(line)
			continue
		}
		s.project(f, line)
		if f.Type == frameSystem && f.Subtype == subtypeInit {
			s.readyOnce.Do(func() { close(s.ready) })
		}
	}
}

// settle delivers a control response to whoever is waiting for that request id.
func (s *streamSession) settle(line []byte) {
	var answer controlResponse
	if json.Unmarshal(line, &answer) != nil {
		return
	}
	id := strings.TrimSpace(answer.Response.RequestID)
	if id == "" {
		return
	}
	s.mu.Lock()
	waiter, ok := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if ok {
		waiter <- controlOutcome{subtype: answer.Response.Subtype, err: answer.Response.Error}
		close(waiter)
	}
}

func (s *streamSession) forget(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

// writeUserText hands one operator message to the process as a `user` frame,
// which is the only shape the streaming input accepts.
func (s *streamSession) writeUserText(text string) error {
	payload, err := json.Marshal(map[string]any{
		"type": frameUser,
		"message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "text": text}},
		},
	})
	if err != nil {
		return fmt.Errorf("encoding the message for the claude session: %w", err)
	}
	return s.process.Send(payload)
}

// start opens the work: it writes the first user frame and waits until the
// process has announced itself.
//
// The wait is the handshake. Without it a dialogue could be attached to a
// process that never came up, and every command sent to it would be delivered
// into nothing while the run looked live.
func (s *streamSession) start(ctx context.Context, prompt string) error {
	if err := s.writeUserText(prompt); err != nil {
		return fmt.Errorf("the claude session could not be given its instruction: %w", err)
	}
	select {
	case <-s.ready:
		return nil
	case <-s.gone:
		return fmt.Errorf("the claude session ended before announcing itself")
	case <-ctx.Done():
		return fmt.Errorf("the claude session did not announce itself: %w", ctx.Err())
	}
}

// Send hands an operator message to the turn in progress. It writes nothing
// into the history: the message becomes history when the process re-emits it as
// a user frame, which is what `--replay-user-messages` is there for.
//
// A turn that is over is a decision the caller must be able to branch on; a
// write that failed is a fault, because nothing was decided and a retry can
// still change the outcome.
func (s *streamSession) Send(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-s.turnDone:
		return &execution.RunCommandError{
			Reason: execution.RunRefusedNotActive,
			RunID:  s.session.RunID(),
			Err:    fmt.Errorf("the claude turn is already over"),
		}
	default:
	}
	if err := s.writeUserText(text); err != nil {
		return fmt.Errorf("sending the message to the claude session: %w", err)
	}
	return nil
}

// Interrupt asks the process to stop the turn. It reports only whether the
// command was taken: the run is over when the process says so, which is why
// nothing here writes state.
func (s *streamSession) Interrupt(ctx context.Context) error {
	// The same guard Send carries, for the same reason: once the turn is over
	// there is nothing left to interrupt, and Codex refuses that command at the
	// protocol level. Without it the two local providers would answer a cancel
	// differently in the instant between the end of the turn and the end of the
	// session.
	select {
	case <-s.turnDone:
		return &execution.RunCommandError{
			Reason: execution.RunRefusedNotActive,
			RunID:  s.session.RunID(),
			Err:    fmt.Errorf("the claude turn is already over"),
		}
	default:
	}
	s.mu.Lock()
	s.nextControl++
	id := fmt.Sprintf("req_%d", s.nextControl)
	waiter := make(chan controlOutcome, 1)
	s.pending[id] = waiter
	s.mu.Unlock()

	payload, err := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": id,
		"request":    map[string]any{"subtype": "interrupt"},
	})
	if err != nil {
		s.forget(id)
		return fmt.Errorf("encoding the interrupt for the claude session: %w", err)
	}
	if err := s.process.Send(payload); err != nil {
		s.forget(id)
		return fmt.Errorf("sending the interrupt to the claude session: %w", err)
	}

	select {
	case <-ctx.Done():
		s.forget(id)
		return fmt.Errorf("the claude session did not answer the interrupt: %w", ctx.Err())
	case <-s.gone:
		s.forget(id)
		return fmt.Errorf("the claude session ended before answering the interrupt")
	case outcome := <-waiter:
		if outcome.subtype != controlSuccess {
			// The process understood the command and declined it. That is a
			// decision, and the caller branches on the reason rather than on this
			// sentence.
			return &execution.RunCommandError{
				Reason: execution.RunRefusedUnsupported,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude session refused the interrupt: %s", refusalText(outcome)),
			}
		}
		return nil
	}
}

func refusalText(outcome controlOutcome) string {
	if body := strings.TrimSpace(outcome.err); body != "" {
		return body
	}
	if body := strings.TrimSpace(outcome.subtype); body != "" {
		return body
	}
	return "no reason given"
}

// endTurn closes the wait for the turn exactly once.
func (s *streamSession) endTurn() {
	s.turnOnce.Do(func() { close(s.turnDone) })
}

// Completed reports whether the process itself said the turn was over and went
// well, as opposed to the turn ending because the process disappeared or
// because it was interrupted. Only the first of the three can carry a plan.
func (s *streamSession) Completed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completed
}

// TurnDone is closed when the turn has ended, whichever way it ended.
func (s *streamSession) TurnDone() <-chan struct{} { return s.turnDone }

// Gone is closed when the process's output has ended.
func (s *streamSession) Gone() <-chan struct{} { return s.gone }

// FinalMessage is the text the run ends on, which is where the plan receipt is
// expected. The `result` frame is preferred because it is the process's own
// statement of what it finished with; the last assistant text is the fallback
// for a turn that ended without one.
func (s *streamSession) FinalMessage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.finalMessage) != "" {
		return s.finalMessage
	}
	return s.lastAssistantText
}
