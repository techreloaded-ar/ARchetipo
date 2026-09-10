package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// The app-server protocol, as observed against codex-cli 0.147.0. The sequence
// is: `initialize`, the `initialized` notification, `thread/start`, then
// `turn/start`; the turn's history arrives as notifications until
// `turn/completed`. A live turn is steered with `turn/steer` and stopped with
// `turn/interrupt`.
//
// The names below are the ones the binary really answers to. They were taken
// from a session driven by hand, not from the generated schema, because the
// schema describes what the protocol can express and not what this build
// accepts.
const (
	methodInitialize    = "initialize"
	methodInitialized   = "initialized"
	methodModelList     = "model/list"
	methodThreadStart   = "thread/start"
	methodTurnStart     = "turn/start"
	methodTurnSteer     = "turn/steer"
	methodTurnInterrupt = "turn/interrupt"
)

// noiseNotifications are the notifications this provider deliberately drops.
//
// They are enumerated one by one on purpose. The rule everywhere else is that
// an unrecognized notification still becomes an event — a translation that
// discards what it does not know silently loses history the day Codex adds a
// type. These five are not unknown: they are known to carry no history at all
// (billing counters, MCP boot progress, remote-control status), and each was
// observed flooding a real session.
var noiseNotifications = map[string]struct{}{
	"account/rateLimits/updated":      {},
	"account/updated":                 {},
	"mcpServer/startupStatus/updated": {},
	"remoteControl/status/changed":    {},
	"thread/tokenUsage/updated":       {},
}

// initialize performs the protocol handshake shared by a working session and
// by the short-lived client that only asks Codex for its model catalog.
func (a *appServer) initialize(ctx context.Context) error {
	if _, err := a.call(ctx, methodInitialize, map[string]any{
		"clientInfo": map[string]any{"name": "archetipo", "title": "ARchetipo", "version": "1"},
	}); err != nil {
		return fmt.Errorf("the codex app server did not accept the handshake: %w", err)
	}
	if err := a.notify(methodInitialized, map[string]any{}); err != nil {
		return fmt.Errorf("notifying the codex app server that initialization completed: %w", err)
	}
	return nil
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("the codex app server refused the request (%d): %s", e.Code, e.Message)
}

// rpcMessage is every shape that can arrive on the process's standard output: a
// response (id + result/error), a notification (method only), or a request from
// the server (id + method).
type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

// appServer drives one `codex app-server` process and projects its notifications
// into a local session. It is the only place in this package that knows the
// protocol.
type appServer struct {
	process localrun.Process
	session *localrun.Session
	// conversational says whether the end of a turn opens the next one on the
	// same thread instead of ending the work. It is an explicit mode rather
	// than the new behaviour of every session, for the same reason
	// streamSession.conversational is: a single-turn dispatched action that
	// ends its turn without a receipt has failed, while the same moment in a
	// conversation is the agent waiting for the next message.
	conversational bool

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan rpcMessage
	threadID string
	turnID   string
	// turnOpen says whether the turn named by turnID is still the one in
	// progress: true from the moment turn/start answers until turn/completed
	// arrives. It is what tells Send whether to steer the turn in progress or
	// — in a conversation — open the next one, because turnID itself is never
	// cleared and so cannot answer that question on its own.
	turnOpen  bool
	seq       int
	completed bool
	agent     strings.Builder
	lastFull  string
	// openingPrompt is the instruction a conversation opens on, held until the
	// first message and then kept until the process echoes it back, exactly
	// like streamSession.openingPrompt — see hold and openingEchoOf for why.
	openingPrompt string
	openingHeld   bool

	turnDone   chan struct{}
	turnClosed bool
	gone       chan struct{}
}

var _ localrun.Dialogue = (*appServer)(nil)

func newAppServer(process localrun.Process, session *localrun.Session, conversational bool) *appServer {
	return &appServer{
		process:        process,
		session:        session,
		conversational: conversational,
		pending:        make(map[int64]chan rpcMessage),
		turnDone:       make(chan struct{}),
		gone:           make(chan struct{}),
	}
}

// consume reads the process until its output ends. It runs on its own
// goroutine for the whole life of the session and is the only reader.
func (a *appServer) consume() {
	defer close(a.gone)
	defer a.endTurn()
	for line := range a.process.Lines() {
		var message rpcMessage
		if err := json.Unmarshal(line, &message); err != nil {
			// A malformed line is not worth ending a live session over: the
			// process keeps producing history and the next line is very likely
			// readable.
			continue
		}
		switch {
		case len(message.ID) > 0 && message.Method == "":
			a.settle(message)
		case len(message.ID) > 0 && message.Method != "":
			// A request from the server. The session runs with approvals
			// disabled, so nothing here is expected — but a request left
			// unanswered would block the process for ever, so it is declined
			// explicitly.
			a.decline(message)
		case message.Method != "":
			a.project(message.Method, message.Params)
		}
	}
}

func (a *appServer) settle(message rpcMessage) {
	id, err := strconv.ParseInt(strings.TrimSpace(string(message.ID)), 10, 64)
	if err != nil {
		return
	}
	a.mu.Lock()
	waiter, ok := a.pending[id]
	delete(a.pending, id)
	a.mu.Unlock()
	if ok {
		waiter <- message
		close(waiter)
	}
}

func (a *appServer) decline(message rpcMessage) {
	payload, err := json.Marshal(map[string]any{
		"id":    json.RawMessage(message.ID),
		"error": map[string]any{"code": -32601, "message": "ARchetipo runs this session without approvals"},
	})
	if err != nil {
		return
	}
	_ = a.process.Send(payload)
}

// call sends a request and waits for its response.
func (a *appServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	a.mu.Lock()
	a.nextID++
	id := a.nextID
	waiter := make(chan rpcMessage, 1)
	a.pending[id] = waiter
	a.mu.Unlock()

	body := map[string]any{"id": id, "method": method}
	if params != nil {
		body["params"] = params
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding the %s request: %w", method, err)
	}
	if err := a.process.Send(payload); err != nil {
		a.forget(id)
		return nil, fmt.Errorf("sending %s to the codex app server: %w", method, err)
	}

	select {
	case <-ctx.Done():
		a.forget(id)
		return nil, ctx.Err()
	case <-a.gone:
		a.forget(id)
		return nil, fmt.Errorf("the codex app server ended before answering %s", method)
	case message := <-waiter:
		if message.Error != nil {
			return nil, message.Error
		}
		return message.Result, nil
	}
}

func (a *appServer) forget(id int64) {
	a.mu.Lock()
	delete(a.pending, id)
	a.mu.Unlock()
}

func (a *appServer) notify(method string, params any) error {
	body := map[string]any{"method": method}
	if params != nil {
		body["params"] = params
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding the %s notification: %w", method, err)
	}
	return a.process.Send(payload)
}

// handshake performs initialize/initialized/thread/start and returns the
// thread id, without opening any turn.
//
// It is split out of start so a conversation can call it alone: the process
// becomes followable and commandable the moment the thread exists, and
// opening a turn before anybody has said anything would start work nobody
// asked for. A dispatched action has no such moment to wait in, so start
// still does both in one call.
func (a *appServer) handshake(ctx context.Context, cfg settings, dir string) (string, error) {
	if err := a.initialize(ctx); err != nil {
		return "", err
	}

	threadParams := map[string]any{
		"cwd":            dir,
		"sandbox":        cfg.Sandbox,
		"approvalPolicy": "never",
		"ephemeral":      true,
	}
	if cfg.Model != "" {
		threadParams["model"] = cfg.Model
	}
	// The reasoning budget travels as a thread-scoped override of the very key
	// that lives in ~/.codex/config.toml. The `config` key is set only when the
	// option is configured, so a thread opened without it is byte for byte the
	// one this provider has always opened.
	if cfg.ReasoningEffort != "" {
		threadParams["config"] = map[string]any{"model_reasoning_effort": cfg.ReasoningEffort}
	}
	result, err := a.call(ctx, methodThreadStart, threadParams)
	if err != nil {
		return "", fmt.Errorf("the codex app server could not open a thread: %w", err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &thread); err != nil || strings.TrimSpace(thread.Thread.ID) == "" {
		return "", fmt.Errorf("the codex app server opened a thread without an identity")
	}

	a.mu.Lock()
	a.threadID = thread.Thread.ID
	a.mu.Unlock()
	return thread.Thread.ID, nil
}

// start performs the handshake and opens the turn that carries the work. It
// is what every dispatched action calls: unlike a conversation, it has no
// work-free moment to exist in.
func (a *appServer) start(ctx context.Context, cfg settings, dir, prompt string) error {
	threadID, err := a.handshake(ctx, cfg, dir)
	if err != nil {
		return err
	}
	return a.openTurn(ctx, threadID, []string{prompt})
}

// openTurn opens one turn on an already-open thread and blocks until the
// process has given it an identity.
//
// texts becomes the turn's input, one text block per entry. It is a slice
// and not a single string because a conversation's first turn carries two
// blocks in the same call — the held opening instruction ahead of the
// person's own message — exactly as streamSession.writeUserBlocks does for
// the same reason: one frame, so the agent answers once and the turn opens
// exactly once.
//
// The wait for TurnDone is re-armed before the request is sent, never after:
// a process that answers turn/completed immediately would otherwise close a
// wait this call is about to reopen, and a caller already selecting on
// TurnDone would read a turn that had, from its point of view, already
// finished.
func (a *appServer) openTurn(ctx context.Context, threadID string, texts []string) error {
	a.armTurn()
	input := make([]any, 0, len(texts))
	for _, text := range texts {
		input = append(input, map[string]any{"type": "text", "text": text})
	}
	result, err := a.call(ctx, methodTurnStart, map[string]any{
		"threadId": threadID,
		"input":    input,
	})
	if err != nil {
		return fmt.Errorf("the codex app server could not start the turn: %w", err)
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(result, &turn); err != nil || strings.TrimSpace(turn.Turn.ID) == "" {
		return fmt.Errorf("the codex app server started a turn without an identity")
	}
	a.mu.Lock()
	a.turnID = turn.Turn.ID
	a.turnOpen = true
	a.mu.Unlock()
	return nil
}

func (a *appServer) ids() (threadID, turnID string, turnOpen bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.threadID, a.turnID, a.turnOpen
}

// hold keeps the instruction a conversation will open on, without starting
// any turn. It is what OpenConversation calls once the thread exists: the
// instruction is the caller's, not the person's, and writing it now would
// have the agent start talking before anybody has asked it anything.
//
// It travels with the first message, in the same turn, for the reason given
// on openTurn and on streamSession.hold: it must arrive before anything is
// answered, and it must not open a turn of its own.
func (a *appServer) hold(prompt string) {
	a.mu.Lock()
	a.openingPrompt = prompt
	a.openingHeld = true
	a.mu.Unlock()
}

// takeHeldOpening hands back the instruction still waiting to be delivered,
// and gives up the fact that it is still owed: delivered twice, it would open
// the conversation twice. openingPrompt itself survives this call — it is
// still needed to recognize the instruction when the process echoes it back,
// which is what openingEchoOf consumes.
func (a *appServer) takeHeldOpening() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.openingHeld {
		return "", false
	}
	a.openingHeld = false
	return a.openingPrompt, true
}

// openingEchoOf mirrors streamSession.openingEchoOf: the first thread item
// Codex reports for a conversation's opening turn carries the instruction and
// the person's own message joined into one text, and the instruction has to
// be stripped so the conversation's history opens on what the person actually
// said and not on what the caller told the agent to do.
//
// It is only ever consulted while the opening instruction is still
// outstanding, and it consumes it: a person who later writes, word for word,
// the instruction the caller composed is an operator quoting a prompt they
// never saw, and what they write from then on enters the history as it always
// did.
func (a *appServer) openingEchoOf(text string) (rest string, echo bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.openingPrompt == "" || !strings.HasPrefix(text, a.openingPrompt) {
		return "", false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(text, a.openingPrompt))
	a.openingPrompt = ""
	return rest, true
}

// Send hands an operator message to the turn in progress, or — in a
// conversation, once the turn is over — opens the next turn on the same
// thread. It writes nothing into the history: the message becomes history
// when the process re-emits it as a user message item, which is exactly what
// was observed on the real binary.
//
// What "over" means is the one thing conversational changes, for the reason
// written on streamSession.Send: in a single-turn dispatched action the end
// of the turn is the end of the work, and a message that arrives after it is
// refused by the process itself — the app server answers turn/steer with "no
// active turn to steer", which classify turns into the typed refusal. In a
// conversation the end of a turn is the agent's question, and the answer
// opens the next one.
//
// "Once the turn is over" is a fact that arrives, not one that is known: the
// turn is over when the process says turn/completed, and that notification is
// read on another goroutine. A message written in the instant between the
// agent finishing and this side learning it therefore still finds turnOpen
// true and steers a turn that no longer exists — which is exactly the moment
// people write, while the answer they are reading is being finished. In a
// conversation the refusal that earns is not the answer: the turn it was about
// is over, the message still has to be delivered, so it opens the next turn
// instead. Every other protocol error stays an error, and a dispatched action
// keeps refusing, because there the end of the turn really is the end of the
// work.
func (a *appServer) Send(ctx context.Context, text string) error {
	threadID, turnID, turnOpen := a.ids()
	if threadID == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedRunnerOffline,
			RunID:  a.session.RunID(),
			Err:    fmt.Errorf("the codex thread is not open yet"),
		}
	}
	if a.conversational && turnOpen {
		err := a.classify(a.steer(ctx, threadID, turnID, text))
		if err == nil || !refusedAsNotActive(err) {
			return err
		}
		// The turn ended under the message. Fall through and open the next one.
		turnOpen = false
	}
	if a.conversational && !turnOpen {
		texts := []string{text}
		if opening, held := a.takeHeldOpening(); held {
			texts = []string{opening, text}
		}
		return a.classify(a.openTurn(ctx, threadID, texts))
	}
	if turnID == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedRunnerOffline,
			RunID:  a.session.RunID(),
			Err:    fmt.Errorf("the codex turn is not open yet"),
		}
	}
	return a.classify(a.steer(ctx, threadID, turnID, text))
}

// steer delivers a message into the turn in progress and reports the
// protocol's own answer, unclassified: what a refusal means is the caller's
// to decide, and the two modes decide it differently.
func (a *appServer) steer(ctx context.Context, threadID, turnID, text string) error {
	_, err := a.call(ctx, methodTurnSteer, map[string]any{
		"threadId":       threadID,
		"expectedTurnId": turnID,
		"input":          []any{map[string]any{"type": "text", "text": text}},
	})
	return err
}

// refusedAsNotActive reports whether an error is the refusal of a turn that is
// over, as opposed to any other failure.
func refusedAsNotActive(err error) bool {
	var refused *execution.RunCommandError
	return errors.As(err, &refused) && refused.Reason == execution.RunRefusedNotActive
}

// Interrupt asks the process to stop the turn in progress. Between two turns
// of a conversation there is no turn to interrupt — what is being cancelled
// is the conversation itself, and closing the process's standard input ends
// it at its source, exactly as streamSession.Interrupt does and for the same
// reason. It does not make the run terminal: the end of the run stays
// observed, through the end of the process's output.
//
// It otherwise reports only whether the command was delivered: the run is
// over when the process says so.
func (a *appServer) Interrupt(ctx context.Context) error {
	threadID, turnID, turnOpen := a.ids()
	if threadID == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedRunnerOffline,
			RunID:  a.session.RunID(),
			Err:    fmt.Errorf("the codex thread is not open yet"),
		}
	}
	// A conversation with no turn in progress — because none has been opened
	// yet, or because the last one already completed — has nothing to
	// interrupt. What is being cancelled at that point is the conversation
	// itself, so this closes the process's input directly. A dispatched
	// action never reaches this branch: it always has an open turn by the
	// time a caller can reach Interrupt at all.
	if a.conversational && !turnOpen {
		if err := a.process.Close(); err != nil {
			return fmt.Errorf("closing the input of the codex conversation: %w", err)
		}
		return nil
	}
	if turnID == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedRunnerOffline,
			RunID:  a.session.RunID(),
			Err:    fmt.Errorf("the codex turn is not open yet"),
		}
	}
	_, err := a.call(ctx, methodTurnInterrupt, map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	})
	return a.classify(err)
}

// classify turns the process's own refusal into the typed one.
//
// The distinction that matters is between a decision and a fault. The app
// server answers `-32600` with `no active turn to steer` / `no active turn to
// interrupt` when the turn is over — observed on codex-cli 0.147.0 — and that
// is a decision the caller must be able to branch on. Every other protocol
// error stays an error, because nothing was decided and a retry can still
// change the outcome.
func (a *appServer) classify(err error) error {
	if err == nil {
		return nil
	}
	var refused *rpcError
	if !asRPCError(err, &refused) {
		return err
	}
	if strings.Contains(strings.ToLower(refused.Message), "no active turn") {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedNotActive,
			RunID:  a.session.RunID(),
			Err:    refused,
		}
	}
	return err
}

func asRPCError(err error, target **rpcError) bool {
	if typed, ok := err.(*rpcError); ok {
		*target = typed
		return true
	}
	return false
}

// claimTurnEnd takes ownership of the end of the current turn and hands back
// the wait that has to be closed for it, or nil when the turn was already
// over. It mirrors streamSession.claimTurnEnd for the same reason: marking the
// turn over and closing its wait are kept as one atomic act under the lock, so
// a message that opens the next turn in the meantime installs its own wait
// instead of racing the close of the one that just ended.
func (a *appServer) claimTurnEnd() chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.turnClosed {
		return nil
	}
	a.turnClosed = true
	return a.turnDone
}

// endTurn closes the wait for the current turn exactly once. A turn that is
// already over stays over until armTurn opens the next one.
func (a *appServer) endTurn() {
	if done := a.claimTurnEnd(); done != nil {
		close(done)
	}
}

// armTurn opens a new turn by installing a fresh wait, so that whoever asks
// for TurnDone from now on waits for the turn that is starting and not for
// the one that ended. Every dispatched action's single turn also goes through
// it once, on its way in, which is a no-op: the session starts with an open
// wait already, so there is nothing to re-arm the first time.
func (a *appServer) armTurn() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.turnClosed {
		return
	}
	a.turnDone = make(chan struct{})
	a.turnClosed = false
}

// Completed reports whether the process itself said the turn was over, as
// opposed to the turn ending because the process disappeared. The two are
// different outcomes and only the first one can carry a plan.
func (a *appServer) Completed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.completed
}

// TurnDone is closed when the current turn has ended, whichever way it ended.
// In a conversation the next turn brings a new channel, so the value must be
// read again after every answer rather than kept — exactly like
// streamSession.TurnDone.
func (a *appServer) TurnDone() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.turnDone
}

// Gone is closed when the process's output has ended.
func (a *appServer) Gone() <-chan struct{} { return a.gone }

// FinalMessage is the text of the agent's last message, which is where the plan
// receipt is expected. It prefers the completed item, because the deltas of a
// message that was still being written are an incomplete quotation of it.
func (a *appServer) FinalMessage() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.TrimSpace(a.lastFull) != "" {
		return a.lastFull
	}
	return a.agent.String()
}
