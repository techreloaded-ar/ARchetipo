package codex

import (
	"context"
	"encoding/json"
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

	mu        sync.Mutex
	nextID    int64
	pending   map[int64]chan rpcMessage
	threadID  string
	turnID    string
	seq       int
	completed bool
	agent     strings.Builder
	lastFull  string

	turnOnce sync.Once
	turnDone chan struct{}
	gone     chan struct{}
}

var _ localrun.Dialogue = (*appServer)(nil)

func newAppServer(process localrun.Process, session *localrun.Session) *appServer {
	return &appServer{
		process:  process,
		session:  session,
		pending:  make(map[int64]chan rpcMessage),
		turnDone: make(chan struct{}),
		gone:     make(chan struct{}),
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

// start performs the handshake and opens the turn that carries the work.
func (a *appServer) start(ctx context.Context, cfg settings, dir, prompt string) error {
	if _, err := a.call(ctx, methodInitialize, map[string]any{
		"clientInfo": map[string]any{"name": "archetipo", "title": "ARchetipo", "version": "1"},
	}); err != nil {
		return fmt.Errorf("the codex app server did not accept the handshake: %w", err)
	}
	if err := a.notify(methodInitialized, map[string]any{}); err != nil {
		return err
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
	result, err := a.call(ctx, methodThreadStart, threadParams)
	if err != nil {
		return fmt.Errorf("the codex app server could not open a thread: %w", err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &thread); err != nil || strings.TrimSpace(thread.Thread.ID) == "" {
		return fmt.Errorf("the codex app server opened a thread without an identity")
	}

	a.mu.Lock()
	a.threadID = thread.Thread.ID
	a.mu.Unlock()

	result, err = a.call(ctx, methodTurnStart, map[string]any{
		"threadId": thread.Thread.ID,
		"input":    []any{map[string]any{"type": "text", "text": prompt}},
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
	a.mu.Unlock()
	return nil
}

func (a *appServer) ids() (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.threadID, a.turnID
}

// Send hands an operator message to the turn in progress. It writes nothing
// into the history: the message becomes history when the process re-emits it as
// a user message item, which is exactly what was observed on the real binary.
func (a *appServer) Send(ctx context.Context, text string) error {
	threadID, turnID := a.ids()
	if threadID == "" || turnID == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedRunnerOffline,
			RunID:  a.session.RunID(),
			Err:    fmt.Errorf("the codex turn is not open yet"),
		}
	}
	_, err := a.call(ctx, methodTurnSteer, map[string]any{
		"threadId":       threadID,
		"expectedTurnId": turnID,
		"input":          []any{map[string]any{"type": "text", "text": text}},
	})
	return a.classify(err)
}

// Interrupt asks the process to stop the turn. It reports only whether the
// command was delivered: the run is over when the process says so.
func (a *appServer) Interrupt(ctx context.Context) error {
	threadID, turnID := a.ids()
	if threadID == "" || turnID == "" {
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

// endTurn closes the wait for the turn exactly once.
func (a *appServer) endTurn() {
	a.turnOnce.Do(func() { close(a.turnDone) })
}

// Completed reports whether the process itself said the turn was over, as
// opposed to the turn ending because the process disappeared. The two are
// different outcomes and only the first one can carry a plan.
func (a *appServer) Completed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.completed
}

// TurnDone is closed when the turn has ended, whichever way it ended.
func (a *appServer) TurnDone() <-chan struct{} { return a.turnDone }

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
