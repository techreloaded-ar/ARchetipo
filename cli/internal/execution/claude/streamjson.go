package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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
	frameControlRequest  = "control_request"
	frameControlResponse = "control_response"
	frameControlCancel   = "control_cancel_request"

	subtypeInit       = "init"
	subtypeCanUseTool = "can_use_tool"
	controlSuccess    = "success"
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
	// RequestID and Request belong to the control protocol, which travels on
	// the same stream as the history. They are read here rather than in a
	// second decode of the line because the frame type alone does not say what
	// a control frame is: the subtype that does live inside Request.
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
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

// TurnOutcome is what one finished turn says about itself, published the
// instant the process declares the turn over. It carries the two facts a caller
// that has to decide whether to keep the conversation open needs: whether the
// process finished the turn without error, and the text it finished on — which
// is where a receipt is expected, and otherwise the question the agent is
// waiting on an answer for.
type TurnOutcome struct {
	Completed bool
	Final     string
}

// turnBuffer bounds the outcomes kept for a caller that is not reading them.
// A conversation has far fewer turns than this before it ends one way or
// another, and publishing is non-blocking on purpose: the protocol reader must
// never wait on whoever is following the run.
const turnBuffer = 64

// streamSession drives one live `claude` process over stream-json and projects
// its frames into a local session. It is the only place in this package that
// knows the protocol.
type streamSession struct {
	process localrun.Process
	session *localrun.Session
	// conversational says whether the end of a turn ends the session. It is an
	// explicit mode rather than the new behaviour of every session because the
	// two semantics are genuinely different: a single-turn run that ends its
	// turn without a receipt is a failure to be reported at once, while the same
	// moment in a conversation is the agent asking a question and waiting.
	// Making every session multi-turn would silently turn the first diagnostic
	// into a wait for the timeout.
	conversational bool

	mu  sync.Mutex
	seq int
	// openingPrompt is the instruction start wrote as the first user frame,
	// kept only until the process replays it back. `--replay-user-messages`
	// re-emits every user frame, the opening one included, and a replay is what
	// makes a message history: without this the instruction the caller composed
	// would enter the transcript as something the person said — titling a
	// conversation with it, counting it as a message, and handing it to a
	// resumed conversation as a request somebody made.
	openingPrompt string
	// openingHeld says the instruction has not been written yet, because this
	// session opened without starting anything. It is given up to the first
	// message, which carries it.
	openingHeld       bool
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
	// asked holds the permission requests the process is waiting on, in the
	// order it opened them. They live here and nowhere else on purpose: a
	// pending question belongs to the process that asked it, so it dies with it
	// — a question nobody answered before the agent left has no answer left to
	// give. The slice keeps the order and the map keeps the lookup; both are
	// written under mu like every other fact of the session.
	asked   map[string]execution.PendingApproval
	askedIn []string
	// now stamps a question with the instant it was asked. It is a field with a
	// real default rather than a constructor parameter so that the seven places
	// that build a session for a test keep building it unchanged; the provider
	// overrides it before consume starts, which is the only moment at which
	// nothing is reading it yet.
	now func() time.Time

	// turnDone is re-armable: it is closed when the current turn ends and
	// replaced by a fresh one when the next turn opens. It lives under mu, like
	// everything else a turn changes, so the close and the replacement cannot
	// interleave.
	turnDone   chan struct{}
	turnClosed bool
	turns      chan TurnOutcome

	readyOnce sync.Once
	ready     chan struct{}
	gone      chan struct{}
}

var _ localrun.Dialogue = (*streamSession)(nil)

func newStreamSession(process localrun.Process, session *localrun.Session, conversational bool) *streamSession {
	return &streamSession{
		process:        process,
		session:        session,
		conversational: conversational,
		// The first turn is seq 1, not 0. Codex numbers its turns from one, and
		// the seq of an event is served to the caller beside its kind: a run
		// that started at zero would say which provider produced it just as
		// plainly as a provider-specific kind would.
		seq:      1,
		pending:  make(map[string]chan controlOutcome),
		tools:    make(map[string]string),
		asked:    make(map[string]execution.PendingApproval),
		now:      time.Now,
		ready:    make(chan struct{}),
		turnDone: make(chan struct{}),
		turns:    make(chan TurnOutcome, turnBuffer),
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
	// A process that has gone can no longer be answered, so its open questions
	// are withdrawn rather than left on offer. Leaving them would show a
	// decision card, with live buttons, on a run that has stopped.
	defer s.withdrawAll()
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
		if f.Type == frameControlRequest {
			// A question the process asks is not history either: it is a decision
			// waiting for somebody, and it becomes history only through what the
			// process does once it has been answered — a tool that runs, or a tool
			// result reporting the refusal.
			s.ask(f)
			continue
		}
		if f.Type == frameControlCancel {
			// The process no longer needs the answer — the turn was interrupted,
			// or somebody else answered. Keeping the question on offer would let
			// an operator answer something nobody is listening for.
			s.withdraw(f.RequestID)
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
	return s.writeUserBlocks(text)
}

// writeUserBlocks writes one `user` frame carrying several text blocks, which
// is how a held opening instruction travels together with the first message of
// the person: one frame, so it opens exactly one turn, and separate blocks, so
// the replay comes back with the instruction and the message still told apart
// and only the instruction is kept out of the history.
func (s *streamSession) writeUserBlocks(texts ...string) error {
	blocks := make([]any, 0, len(texts))
	for _, text := range texts {
		blocks = append(blocks, map[string]any{"type": "text", "text": text})
	}
	payload, err := json.Marshal(map[string]any{
		"type": frameUser,
		"message": map[string]any{
			"role":    "user",
			"content": blocks,
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
	// Remembered before it is written, never after: the process can replay it
	// while this call is still returning, and a prompt recorded too late would
	// already have entered the history it is remembered to keep out of.
	s.mu.Lock()
	s.openingPrompt = prompt
	s.mu.Unlock()
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

// hold keeps the opening instruction instead of writing it, so that opening the
// session starts no work at all.
//
// It is what a free conversation opens with. The instruction is the caller's,
// not the person's: written now it would open a turn on a workspace nobody has
// asked anything about yet, and the person would find the agent already talking
// in a conversation they have not started. Held, it travels with the first
// message they write — in the same frame, so it still arrives before anything
// is answered and still opens exactly one turn.
//
// There is no handshake to wait for, and none is lost: the process announces
// itself only once a first frame reaches it, so a session that writes nothing
// has nothing to be announced by. What it costs is learning at the first
// message, rather than at the open, that the process never came up — and a
// process that leaves is observed either way, through the end of its output.
//
// The turn is closed because there is none: before the first message nothing is
// in progress, which is what makes a cancel arriving in the meantime end the
// conversation instead of interrupting work that was never started.
func (s *streamSession) hold(prompt string) {
	s.mu.Lock()
	s.openingPrompt = prompt
	s.openingHeld = true
	s.mu.Unlock()
	s.endTurn()
}

// takeHeldOpening hands back the instruction still waiting to be delivered, and
// gives it up: an instruction delivered twice would open the conversation
// twice.
func (s *streamSession) takeHeldOpening() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.openingHeld {
		return "", false
	}
	s.openingHeld = false
	return s.openingPrompt, true
}

// openingEchoOf answers what is left of a replayed block once the opening
// instruction has been taken out of it: the whole block was the instruction and
// nothing remains, or it began with the instruction and the person's own
// message follows it. It consumes the memory of the instruction, so it can
// answer that at most once.
//
// The prefix is looked at and not only the whole text because the instruction
// can be delivered in the same frame as the first message, as its own block: a
// build that replays such a frame as one joined block would otherwise put the
// instruction back into the history with the message glued to it.
//
// It is only ever consulted while the opening instruction is still outstanding:
// a person who wrote, word for word, the instruction the caller composed would
// be an operator quoting a prompt they never saw, and everything they write
// after the replay enters the history as it always did.
func (s *streamSession) openingEchoOf(text string) (rest string, echo bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.openingPrompt == "" || !strings.HasPrefix(text, s.openingPrompt) {
		return "", false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(text, s.openingPrompt))
	s.openingPrompt = ""
	return rest, true
}

// Send hands an operator message to the turn in progress. It writes nothing
// into the history: the message becomes history when the process re-emits it as
// a user frame, which is what `--replay-user-messages` is there for.
//
// A turn that is over is a decision the caller must be able to branch on; a
// write that failed is a fault, because nothing was decided and a retry can
// still change the outcome.
//
// What "over" means is the one thing the mode changes, and it is explicit for
// the reason given on the field: in a single-turn run the end of the turn is
// the end of the work, while in a conversation it is the agent's question, and
// the answer to it opens the next turn.
func (s *streamSession) Send(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.conversational {
		if s.sessionOver() {
			return &execution.RunCommandError{
				Reason: execution.RunRefusedNotActive,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude session is already over"),
			}
		}
		// The turn is re-armed before the frame is written, never after: a
		// process that answers immediately would otherwise end a turn that the
		// re-arming is about to reopen, and the caller would wait for a turn that
		// already finished.
		//
		// A message landing while the previous turn is still being closed out —
		// after its result was recorded, before its wait was closed — finds the
		// turn already claimed by claimTurnEnd, and so re-arms here like any
		// other. That is what keeps the close of the old turn from landing on the
		// new one, which a later Interrupt would otherwise read as "between two
		// turns" and answer by closing the process's input on work in progress.
		s.armTurn()
	} else {
		select {
		case <-s.TurnDone():
			return &execution.RunCommandError{
				Reason: execution.RunRefusedNotActive,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude turn is already over"),
			}
		default:
		}
	}
	// The held instruction, when there is one, travels in the same frame as this
	// message and before it. It is given up before the write and never put back:
	// a write that fails means the process is gone, and a retry that delivered
	// the instruction a second time would open the conversation twice.
	write := func() error { return s.writeUserText(text) }
	if opening, held := s.takeHeldOpening(); held {
		write = func() error { return s.writeUserBlocks(opening, text) }
	}
	if err := write(); err != nil {
		// The guard above and the write are not one atomic act, and they cannot
		// be: the process can leave in between. When it has, the write failed for
		// a reason the caller must be able to branch on — the run is no longer
		// there — and not as the fault a failed write otherwise is. Re-reading
		// `gone` here is what keeps that answer stable whichever side of the
		// window the message arrived on. Only in a conversation: a single-turn run
		// judges liveness by its turn and not by the process, and that judgement
		// is left exactly as it was.
		if s.conversational && s.sessionOver() {
			return &execution.RunCommandError{
				Reason: execution.RunRefusedNotActive,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude session is already over"),
			}
		}
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
	case <-s.TurnDone():
		if !s.conversational {
			return &execution.RunCommandError{
				Reason: execution.RunRefusedNotActive,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude turn is already over"),
			}
		}
		// Between two turns there is no turn to interrupt: what is being
		// cancelled is the conversation itself. Closing the process's standard
		// input ends it at its source, which is the only thing this command can
		// honestly do. It does not make the run terminal: the end of the run
		// stays observed, through the end of the process's output, and is never
		// deduced from the fact that this command succeeded.
		if err := s.process.Close(); err != nil {
			return fmt.Errorf("closing the input of the claude session: %w", err)
		}
		return nil
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

// claimTurnEnd takes ownership of the end of the current turn and hands back the
// wait that has to be closed for it, or nil when the turn was already over.
//
// Claiming and closing are two acts because the end of a turn is not one
// instant: between the result frame and the close there is an outcome to publish
// and an event to append, and a message sent in that gap opens the next turn.
// Marking the turn over here, under the same lock that guards the wait, is what
// keeps the close bound to the turn that produced it: whoever opens the next
// turn in the meantime installs its own wait, and the channel returned here
// stays the one belonging to the turn that ended.
func (s *streamSession) claimTurnEnd() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnClosed {
		return nil
	}
	s.turnClosed = true
	return s.turnDone
}

// endTurn closes the wait for the current turn exactly once. A turn that is
// already over stays over until armTurn opens the next one.
func (s *streamSession) endTurn() {
	if done := s.claimTurnEnd(); done != nil {
		close(done)
	}
}

// armTurn opens a new turn by installing a fresh wait, so that whoever asks for
// TurnDone from now on waits for the turn that is starting and not for the one
// that ended.
func (s *streamSession) armTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.turnClosed {
		return
	}
	s.turnDone = make(chan struct{})
	s.turnClosed = false
}

// sessionOver reports whether the process's output has already ended.
func (s *streamSession) sessionOver() bool {
	select {
	case <-s.gone:
		return true
	default:
		return false
	}
}

// publishTurn records how a turn ended without ever blocking the reader of the
// protocol. A caller that is not listening cannot slow down the process, and an
// outcome dropped because nobody is following the turns is an outcome nobody
// was going to act on.
func (s *streamSession) publishTurn(outcome TurnOutcome) {
	select {
	case s.turns <- outcome:
	default:
	}
}

// Turns yields the outcome of every turn the process itself declared finished.
func (s *streamSession) Turns() <-chan TurnOutcome { return s.turns }

// Completed reports whether the process itself said the turn was over and went
// well, as opposed to the turn ending because the process disappeared or
// because it was interrupted. Only the first of the three can carry a plan.
func (s *streamSession) Completed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completed
}

// TurnDone is closed when the current turn has ended, whichever way it ended.
// In a conversation the next turn brings a new channel, so the value must be
// read again after every answer rather than kept.
func (s *streamSession) TurnDone() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnDone
}

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
