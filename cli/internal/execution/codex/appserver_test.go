package codex

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

// fakeCodex is a Codex app server that never runs: it answers the protocol the
// real binary was observed to speak and emits exactly the notifications a test
// tells it to. It replaces the process and nothing else — the client, the
// translation, the session and the refusals under test are the production ones.
type fakeCodex struct {
	mu           sync.Mutex
	requests     []rpcMessage
	steered      []string
	interrupts   int
	signals      int
	lines        chan []byte
	closeOnce    sync.Once
	ended        bool
	exitCode     int
	stderr       string
	waitErr      error
	waitForClose bool

	threadErr    *rpcError
	turnErr      *rpcError
	steerErr     *rpcError
	interruptErr *rpcError
	reemitSteer  bool

	turnStarted chan struct{}
	done        chan struct{}

	// startDir is the directory the process was really started in — the
	// cmd.Dir the Starter receives — so a test can assert where the run
	// executes instead of asserting a configuration value that stands for it.
	startDir string
}

var _ localrun.Process = (*fakeCodex)(nil)

func newFakeCodex() *fakeCodex {
	return &fakeCodex{
		lines:       make(chan []byte, 512),
		turnStarted: make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func (f *fakeCodex) Start(_ context.Context, dir, _ string, _ []string) (localrun.Process, error) {
	f.mu.Lock()
	f.startDir = dir
	f.mu.Unlock()
	return f, nil
}

func (f *fakeCodex) startedIn() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startDir
}

func (f *fakeCodex) Send(line []byte) error {
	var message rpcMessage
	if err := json.Unmarshal(line, &message); err != nil {
		return err
	}
	f.mu.Lock()
	f.requests = append(f.requests, message)
	f.mu.Unlock()

	switch message.Method {
	case methodInitialize:
		f.reply(message.ID, `{"userAgent":"fake"}`, nil)
	case methodInitialized:
	case methodThreadStart:
		f.reply(message.ID, `{"thread":{"id":"thread-1"}}`, f.threadErr)
	case methodTurnStart:
		f.reply(message.ID, `{"turn":{"id":"turn-1"}}`, f.turnErr)
		close(f.turnStarted)
	case methodTurnSteer:
		text := textOfInput(message.Params)
		f.mu.Lock()
		f.steered = append(f.steered, text)
		reemit := f.reemitSteer
		f.mu.Unlock()
		f.reply(message.ID, `{}`, f.steerErr)
		if reemit && f.steerErr == nil {
			f.emit("item/started", fmt.Sprintf(`{"item":{"type":"userMessage","content":[{"type":"text","text":%q}]}}`, text))
		}
	case methodTurnInterrupt:
		f.mu.Lock()
		f.interrupts++
		f.mu.Unlock()
		f.reply(message.ID, `{}`, f.interruptErr)
	}
	return nil
}

func (f *fakeCodex) reply(id json.RawMessage, result string, failure *rpcError) {
	body := map[string]any{"id": json.RawMessage(id)}
	if failure != nil {
		body["error"] = failure
	} else {
		body["result"] = json.RawMessage(result)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return
	}
	f.push(payload)
}

// emit publishes one notification, exactly as the real server would.
func (f *fakeCodex) emit(method, params string) {
	payload, err := json.Marshal(map[string]any{"method": method, "params": json.RawMessage(params)})
	if err != nil {
		return
	}
	f.push(payload)
}

func (f *fakeCodex) push(payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended {
		return
	}
	f.lines <- payload
}

func (f *fakeCodex) completeTurn() {
	f.emit("turn/completed", `{"turn":{"id":"turn-1"}}`)
}

// end closes the process's output, which is how a real process disappears.
func (f *fakeCodex) end() {
	f.closeOnce.Do(func() {
		f.mu.Lock()
		f.ended = true
		f.mu.Unlock()
		close(f.lines)
		close(f.done)
	})
}

func (f *fakeCodex) Lines() <-chan []byte { return f.lines }

func (f *fakeCodex) Signal() error {
	f.mu.Lock()
	f.signals++
	f.mu.Unlock()
	f.end()
	return nil
}

func (f *fakeCodex) Wait() (int, string, error) {
	if f.waitForClose {
		<-f.done
	}
	return f.exitCode, f.stderr, f.waitErr
}

func (f *fakeCodex) Close() error {
	f.end()
	return nil
}

func (f *fakeCodex) methodsCalled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.requests))
	for _, request := range f.requests {
		out = append(out, request.Method)
	}
	return out
}

func (f *fakeCodex) paramsOf(method string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, request := range f.requests {
		if request.Method == method {
			var params map[string]any
			if json.Unmarshal(request.Params, &params) != nil {
				return nil
			}
			return params
		}
	}
	return nil
}

func (f *fakeCodex) messagesSteered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.steered...)
}

func textOfInput(params json.RawMessage) string {
	var payload struct {
		Input []struct {
			Text string `json:"text"`
		} `json:"input"`
	}
	if json.Unmarshal(params, &payload) != nil || len(payload.Input) == 0 {
		return ""
	}
	return payload.Input[0].Text
}

// openSession starts a client against the fake and returns both, already past
// the handshake.
func openSession(t *testing.T, fake *fakeCodex, cfg settings) (*appServer, *localrun.Session) {
	t.Helper()
	session := localrun.NewSession("run-1", nil)
	client := newAppServer(fake, session)
	go client.consume()
	if err := client.start(context.Background(), cfg, "/workspace", "PROMPT"); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	session.AttachDialogue(client)
	t.Cleanup(fake.end)
	return client, session
}

func defaultSettings() settings {
	return settings{Command: "codex", Sandbox: defaultSandbox, Timeout: time.Minute}
}

// The handshake is the sequence observed on the real binary, in that order.
func TestAppServerHandshakeFollowsTheObservedSequence(t *testing.T) {
	fake := newFakeCodex()
	cfg := defaultSettings()
	cfg.Model = "gpt-5-codex"
	openSession(t, fake, cfg)

	want := []string{methodInitialize, methodInitialized, methodThreadStart, methodTurnStart}
	got := fake.methodsCalled()
	if len(got) != len(want) {
		t.Fatalf("methods = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("methods = %v, want %v", got, want)
		}
	}

	thread := fake.paramsOf(methodThreadStart)
	if thread["cwd"] != "/workspace" {
		t.Fatalf("thread/start cwd = %v", thread["cwd"])
	}
	if thread["sandbox"] != defaultSandbox {
		t.Fatalf("thread/start sandbox = %v, want %q", thread["sandbox"], defaultSandbox)
	}
	if thread["approvalPolicy"] != "never" {
		t.Fatalf("thread/start approvalPolicy = %v, want never: a session that stops to ask would hang", thread["approvalPolicy"])
	}
	if thread["model"] != "gpt-5-codex" {
		t.Fatalf("thread/start model = %v", thread["model"])
	}
	if turn := fake.paramsOf(methodTurnStart); textOfInputMap(turn) != "PROMPT" {
		t.Fatalf("turn/start input = %v", turn["input"])
	}
}

// Without a configured model the field is absent, not empty: an empty model
// would ask Codex for a model called "".
func TestAppServerOmitsAnUnconfiguredModel(t *testing.T) {
	fake := newFakeCodex()
	openSession(t, fake, defaultSettings())
	if thread := fake.paramsOf(methodThreadStart); thread["model"] != nil {
		t.Fatalf("thread/start carried a model = %#v", thread["model"])
	}
}

func textOfInputMap(params map[string]any) string {
	input, ok := params["input"].([]any)
	if !ok || len(input) == 0 {
		return ""
	}
	entry, ok := input[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := entry["text"].(string)
	return text
}

// AC-2, AC-3 — every notification that carries history becomes one event, with
// the kind, the text and the tool the run really had.
func TestAppServerTranslatesEveryNotificationThatCarriesHistory(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		params   string
		wantKind string
		wantText string
		wantTool string
		silent   bool
	}{
		{
			name:     "the operator message the process re-emitted",
			method:   "item/started",
			params:   `{"item":{"type":"userMessage","content":[{"type":"text","text":"fermati al punto due"}]}}`,
			wantKind: localrun.KindUserMessage,
			wantText: "fermati al punto due",
		},
		{
			name:     "a fragment of the agent's answer",
			method:   "item/agentMessage/delta",
			params:   `{"delta":"pianifico "}`,
			wantKind: localrun.KindText,
			wantText: "pianifico ",
		},
		{
			name:     "a fragment of the agent's reasoning",
			method:   "item/reasoning/summaryTextDelta",
			params:   `{"delta":"valuto le opzioni"}`,
			wantKind: localrun.KindThinking,
			wantText: "valuto le opzioni",
		},
		{
			name:     "a command starting",
			method:   "item/started",
			params:   `{"item":{"type":"commandExecution","command":["go","test","./..."],"status":"inProgress"}}`,
			wantKind: localrun.KindToolStart,
			wantText: "go test ./...",
			wantTool: "commandExecution",
		},
		{
			name:     "a command that succeeded",
			method:   "item/completed",
			params:   `{"item":{"type":"commandExecution","command":"ls","status":"completed","exitCode":0}}`,
			wantKind: localrun.KindToolEnd,
			wantText: "ls",
			wantTool: "commandExecution",
		},
		{
			name:     "a command that failed",
			method:   "item/completed",
			params:   `{"item":{"type":"commandExecution","command":"ls","status":"failed","exitCode":2}}`,
			wantKind: localrun.KindToolError,
			wantText: "ls",
			wantTool: "commandExecution",
		},
		{
			name:     "an MCP tool call, named by its server",
			method:   "item/started",
			params:   `{"item":{"type":"mcpToolCall","tool":"search","server":"docs"}}`,
			wantKind: localrun.KindToolStart,
			wantTool: "docs.search",
		},
		{
			name:     "the end of the turn",
			method:   "turn/completed",
			params:   `{"turn":{"id":"turn-1"}}`,
			wantKind: localrun.KindTurnEnd,
		},
		{
			name:     "an error the server reported",
			method:   "error",
			params:   `{"message":"model unavailable"}`,
			wantKind: localrun.KindError,
			wantText: "model unavailable",
		},
		{
			name:   "the agent message opening, already rendered by its deltas",
			method: "item/started",
			params: `{"item":{"type":"agentMessage","text":""}}`,
			silent: true,
		},
		{
			name:   "the operator message closing, already rendered when it opened",
			method: "item/completed",
			params: `{"item":{"type":"userMessage","content":[{"type":"text","text":"ciao"}]}}`,
			silent: true,
		},
		{
			name:   "a billing counter, which is not history",
			method: "account/rateLimits/updated",
			params: `{"rateLimits":{}}`,
			silent: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session := localrun.NewSession("run-1", nil)
			client := newAppServer(newFakeCodex(), session)
			client.project(tc.method, json.RawMessage(tc.params))

			events := session.Events(0)
			if tc.silent {
				if len(events) != 0 {
					t.Fatalf("expected no event, got %#v", events)
				}
				return
			}
			if len(events) != 1 {
				t.Fatalf("expected exactly one event, got %#v", events)
			}
			event := events[0]
			if event.Kind != tc.wantKind || event.Text != tc.wantText || event.Tool != tc.wantTool {
				t.Fatalf("event = %#v; want kind=%q text=%q tool=%q", event, tc.wantKind, tc.wantText, tc.wantTool)
			}
			if len(event.Raw) == 0 {
				t.Fatal("the original payload must survive in Raw")
			}
		})
	}
}

// A notification this build has never seen still produces an event: a
// translation that dropped it would lose history the day Codex adds a type.
func TestAppServerNeverDropsAnUnknownNotification(t *testing.T) {
	session := localrun.NewSession("run-1", nil)
	client := newAppServer(newFakeCodex(), session)
	const params = `{"something":"entirely new"}`
	client.project("thread/somethingNobodyHasSeen", json.RawMessage(params))

	events := session.Events(0)
	if len(events) != 1 {
		t.Fatalf("expected the unknown notification to become an event, got %#v", events)
	}
	if events[0].Kind != "thread/somethingNobodyHasSeen" {
		t.Fatalf("kind = %q, want the method itself", events[0].Kind)
	}
	if string(events[0].Raw) != params {
		t.Fatalf("raw = %s, want the original payload untouched", events[0].Raw)
	}
}

// AC-2 — the history keeps the order of arrival and numbers it from one.
func TestAppServerKeepsTheOrderOfArrival(t *testing.T) {
	fake := newFakeCodex()
	_, session := openSession(t, fake, defaultSettings())
	for i := 1; i <= 20; i++ {
		fake.emit("item/agentMessage/delta", fmt.Sprintf(`{"delta":"%d"}`, i))
	}
	waitFor(t, func() bool { return len(session.Events(0)) == 20 })

	events := session.Events(0)
	for i, event := range events {
		if event.ID != int64(i+1) {
			t.Fatalf("event %d has id %d", i, event.ID)
		}
		if event.Text != fmt.Sprintf("%d", i+1) {
			t.Fatalf("event %d carries %q", i, event.Text)
		}
	}
}

// AC-3 — the message travels to the process as a steer, and becomes history
// only when the process re-emits it.
func TestAppServerSteersTheTurnAndWaitsForTheReEmission(t *testing.T) {
	fake := newFakeCodex()
	client, session := openSession(t, fake, defaultSettings())
	before := len(session.Events(0))

	const sentinel = "cambia il criterio due"
	if err := client.Send(context.Background(), sentinel); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if got := fake.messagesSteered(); len(got) != 1 || got[0] != sentinel {
		t.Fatalf("the process was steered with %v; want %q", got, sentinel)
	}
	steer := fake.paramsOf(methodTurnSteer)
	if steer["threadId"] != "thread-1" || steer["expectedTurnId"] != "turn-1" {
		t.Fatalf("turn/steer named %v", steer)
	}
	if got := len(session.Events(0)); got != before {
		t.Fatalf("the message entered the history before the process re-emitted it: %d events", got)
	}

	fake.emit("item/started", fmt.Sprintf(`{"item":{"type":"userMessage","content":[{"type":"text","text":%q}]}}`, sentinel))
	waitFor(t, func() bool {
		events := session.Events(0)
		return len(events) > before && events[len(events)-1].Kind == localrun.KindUserMessage
	})
	events := session.Events(0)
	if last := events[len(events)-1]; last.Text != sentinel {
		t.Fatalf("the re-emitted message is %#v", last)
	}
}

// AC-4 — interrupting is a delivery, not a verdict.
func TestAppServerInterruptsWithoutClosingTheSession(t *testing.T) {
	fake := newFakeCodex()
	client, session := openSession(t, fake, defaultSettings())

	if err := client.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}
	interrupt := fake.paramsOf(methodTurnInterrupt)
	if interrupt["threadId"] != "thread-1" || interrupt["turnId"] != "turn-1" {
		t.Fatalf("turn/interrupt named %v", interrupt)
	}
	if !session.Active() {
		t.Fatal("interrupting must not close the session: the run is over when the process says so")
	}
}

// AC-5 — the refusal the process expressed becomes the typed one, and a
// protocol error that decided nothing does not.
func TestAppServerClassifiesTheProcessRefusal(t *testing.T) {
	fake := newFakeCodex()
	fake.steerErr = &rpcError{Code: -32600, Message: "no active turn to steer"}
	fake.interruptErr = &rpcError{Code: -32600, Message: "no active turn to interrupt"}
	client, _ := openSession(t, fake, defaultSettings())

	sendErr := client.Send(context.Background(), "ci sei?")
	reason, refused := execution.RefusalOf(sendErr)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("Send got %v; want a run_not_active refusal", sendErr)
	}
	cancelErr := client.Interrupt(context.Background())
	reason, refused = execution.RefusalOf(cancelErr)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("Interrupt got %v; want a run_not_active refusal", cancelErr)
	}

	other := newFakeCodex()
	other.steerErr = &rpcError{Code: -32000, Message: "the model provider is unreachable"}
	otherClient, _ := openSession(t, other, defaultSettings())
	err := otherClient.Send(context.Background(), "ci sei?")
	if _, refused := execution.RefusalOf(err); refused {
		t.Fatalf("a protocol failure must not become a refusal: %v", err)
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("the diagnostic lost the cause: %v", err)
	}
}

// A handshake the server refuses is a failure with its own diagnostic, and the
// session never pretends the turn started.
func TestAppServerReportsARefusedHandshake(t *testing.T) {
	fake := newFakeCodex()
	fake.threadErr = &rpcError{Code: -32602, Message: "invalid sandbox"}
	session := localrun.NewSession("run-1", nil)
	client := newAppServer(fake, session)
	go client.consume()
	t.Cleanup(fake.end)

	err := client.start(context.Background(), defaultSettings(), "/workspace", "PROMPT")
	if err == nil {
		t.Fatal("expected the refused handshake to fail")
	}
	if !strings.Contains(err.Error(), "invalid sandbox") {
		t.Fatalf("the diagnostic lost the cause: %v", err)
	}
}

// The turn ends when the process says so, and also when the process disappears
// without saying anything — otherwise a caller would wait for ever.
func TestAppServerEndsTheWaitOnTurnCompletionAndOnDeath(t *testing.T) {
	completed := newFakeCodex()
	client, _ := openSession(t, completed, defaultSettings())
	completed.completeTurn()
	select {
	case <-client.TurnDone():
	case <-time.After(2 * time.Second):
		t.Fatal("turn/completed did not end the wait")
	}

	silent := newFakeCodex()
	other, _ := openSession(t, silent, defaultSettings())
	silent.end()
	select {
	case <-other.TurnDone():
	case <-time.After(2 * time.Second):
		t.Fatal("the death of the process did not end the wait")
	}
	select {
	case <-other.Gone():
	case <-time.After(2 * time.Second):
		t.Fatal("the end of the output was not observed")
	}
}

// The receipt is looked for in the agent's finished message, which is a
// complete quotation of it — the deltas of a message still being written are
// not.
func TestAppServerPrefersTheCompletedAgentMessage(t *testing.T) {
	fake := newFakeCodex()
	client, session := openSession(t, fake, defaultSettings())
	fake.emit("item/agentMessage/delta", `{"delta":"parzi"}`)
	waitFor(t, func() bool { return len(session.Events(0)) >= 1 })
	if got := client.FinalMessage(); got != "parzi" {
		t.Fatalf("FinalMessage = %q; want the deltas seen so far", got)
	}
	fake.emit("item/completed", `{"item":{"type":"agentMessage","text":"parziale e poi completo"}}`)
	waitFor(t, func() bool { return client.FinalMessage() == "parziale e poi completo" })
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

// The reasoning budget travels as a thread-scoped override of the key that
// lives in ~/.codex/config.toml, so it has to arrive inside the `config` field
// of thread/start under its own name.
func TestAppServerSendsTheConfiguredReasoningEffort(t *testing.T) {
	fake := newFakeCodex()
	cfg := defaultSettings()
	cfg.ReasoningEffort = "high"
	openSession(t, fake, cfg)

	thread := fake.paramsOf(methodThreadStart)
	overrides, ok := thread["config"].(map[string]any)
	if !ok {
		t.Fatalf("thread/start config = %#v, want a map of overrides", thread["config"])
	}
	if overrides["model_reasoning_effort"] != "high" {
		t.Fatalf("thread/start config.model_reasoning_effort = %#v, want %q", overrides["model_reasoning_effort"], "high")
	}
}

// Without the option the `config` key is absent, not empty: a thread opened
// without a reasoning budget must be the very one this provider always opened.
func TestAppServerOmitsAnUnconfiguredReasoningEffort(t *testing.T) {
	fake := newFakeCodex()
	openSession(t, fake, defaultSettings())
	if thread := fake.paramsOf(methodThreadStart); thread["config"] != nil {
		t.Fatalf("thread/start carried config = %#v", thread["config"])
	}
}
