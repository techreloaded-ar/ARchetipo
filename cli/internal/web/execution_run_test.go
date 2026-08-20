package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// runTestProvider is the provider the acceptance tests dispatch to. Only the
// provider is a double: the connector, the record store, the effect
// confirmation and the process Template are the production ones, so nothing in
// these tests hides the oracle behind a fake.
type runTestProvider struct {
	id      string
	entered chan struct{}
	release chan struct{}
	execute func(context.Context, execution.Request) (execution.Result, error)
}

func newRunTestProvider(id string, execute func(context.Context, execution.Request) (execution.Result, error)) *runTestProvider {
	return &runTestProvider{
		id:      id,
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
		execute: execute,
	}
}

func (p *runTestProvider) ID() string { return p.id }
func (p *runTestProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{execution.CapabilitySpecPlan}, nil
}
func (p *runTestProvider) ValidateConfig(context.Context, map[string]any) error { return nil }
func (p *runTestProvider) Execute(ctx context.Context, request execution.Request) (execution.Result, error) {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		return execution.Result{}, ctx.Err()
	}
	if p.execute == nil {
		return execution.Result{Payload: json.RawMessage(`{"ok":true}`)}, nil
	}
	return p.execute(ctx, request)
}

// blockedProvider stays inside Execute until the test releases it, which is how
// the non-terminal state becomes observable without a single sleep.
func blockedProvider(id string) *runTestProvider { return newRunTestProvider(id, nil) }

// releasedProvider is already free to run: Execute returns as soon as it is
// called.
func releasedProvider(id string, execute func(context.Context, execution.Request) (execution.Result, error)) *runTestProvider {
	p := newRunTestProvider(id, execute)
	close(p.release)
	return p
}

func seedRunSpecs(t *testing.T, conn connector.Connector) {
	t.Helper()
	specs := []domain.Spec{
		{Code: "US-901", Title: "Da pianificare", Epic: domain.Epic{Code: "EP-009", Title: "E"}, Priority: domain.PriorityHigh, Points: 3, Status: domain.StatusTodo},
		{Code: "US-902", Title: "Altra spec", Epic: domain.Epic{Code: "EP-009", Title: "E"}, Priority: domain.PriorityMedium, Points: 2, Status: domain.StatusTodo},
	}
	if _, err := conn.SaveInitialBacklog(context.Background(), specs); err != nil {
		t.Fatal(err)
	}
}

// newRunServer builds a viewer over a real filefs workspace. withDefault
// decides whether the workspace has a persisted default provider, which is the
// difference between "can start an action" and "must be configured first".
func newRunServer(t *testing.T, provider execution.Provider, withDefault bool) (*Server, config.Config, connector.Connector) {
	t.Helper()
	return newRunServerWithConnector(t, provider, withDefault, nil)
}

// newRunServerWithConnector is newRunServer with the connector the server reads
// through wrapped by the test. The workspace, the record store, the effect
// confirmation and the Template stay the production ones; only the reads a test
// needs to observe are intercepted.
func newRunServerWithConnector(t *testing.T, provider execution.Provider, withDefault bool, wrap func(connector.Connector) connector.Connector) (*Server, config.Config, connector.Connector) {
	t.Helper()
	return newRunServerWith(t, provider, withDefault, wrap, true)
}

// newEmptyRunServer is newRunServer over a workspace that has never had a
// backlog: no index, no spec file. It is a distinct starting state, not a
// variation of the seeded one — it is exactly the workspace a first backlog
// generation targets, and the one where the connector answers "there is no
// backlog here" as a missing precondition.
func newEmptyRunServer(t *testing.T, provider execution.Provider, withDefault bool) (*Server, config.Config, connector.Connector) {
	t.Helper()
	return newRunServerWith(t, provider, withDefault, nil, false)
}

func newRunServerWith(t *testing.T, provider execution.Provider, withDefault bool, wrap func(connector.Connector) connector.Connector, seed bool) (*Server, config.Config, connector.Connector) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := filefs.New(cfg)
	if seed {
		seedRunSpecs(t, conn)
	}
	served := connector.Connector(conn)
	if wrap != nil {
		served = wrap(conn)
	}
	registry := execution.NewRegistry()
	if provider != nil {
		if err := registry.Register(provider); err != nil {
			t.Fatal(err)
		}
		if withDefault {
			if _, err := config.UpdateDefaultProvider(dir, config.DefaultProviderConfig{ID: provider.ID(), Config: map[string]any{}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	srv, err := NewServer(served, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// A dispatch outlives the request that started it, so a test that returns
	// while one is still running would let a goroutine write into a TempDir the
	// framework is deleting. Draining is the same wait shutdown performs.
	t.Cleanup(func() { srv.session().dispatch.wait(5 * time.Second) })
	return srv, cfg, conn
}

func startAction(t *testing.T, srv *Server, code, action string) (int, map[string]any) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/spec/"+code+"/execution", map[string]any{"action": action})
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("undecodable response (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, body
}

func readExecution(t *testing.T, srv *Server, id string) (int, execution.Execution) {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/execution/"+id, nil)
	var record execution.Execution
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, record
}

// awaitTerminal polls the record the way the browser does, bounded so a stuck
// dispatch fails the test instead of hanging it.
func awaitTerminal(t *testing.T, srv *Server, id string) execution.Execution {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, record := readExecution(t, srv, id)
		if status == http.StatusOK && record.Status != execution.StatusRunning {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("execution %s never reached a terminal state", id)
	return execution.Execution{}
}

func runSpecDetail(t *testing.T, srv *Server, code string) specDetailResponse {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/spec/"+code, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/spec/%s: %d %s", code, w.Code, w.Body.String())
	}
	var detail specDetailResponse
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return detail
}

// specDetailResponse decodes the detail payload the browser receives. It is
// written out rather than reusing specDetailView so the test asserts the wire
// shape — including the flat action fields US-028 shipped — and not the struct.
type specDetailResponse struct {
	Spec     domain.Spec   `json:"spec"`
	PlanBody string        `json:"plan_body"`
	Tasks    []domain.Task `json:"tasks"`
	Actions  []struct {
		ID                string `json:"id"`
		Label             string `json:"label"`
		Skill             string `json:"skill"`
		Runnable          bool   `json:"runnable"`
		UnavailableReason string `json:"unavailable_reason"`
	} `json:"actions"`
	Execution *execution.Execution `json:"execution"`
}

func recordFileCount(t *testing.T, root, specCode string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".archetipo", "executions"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, ".archetipo", "executions", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var record execution.Execution
		if err := json.Unmarshal(body, &record); err != nil {
			t.Fatal(err)
		}
		if record.SpecCode == specCode {
			count++
		}
	}
	return count
}

// planningExecute is the provider behaviour of a remote agent that really did
// the work: it persists the plan through the connector and moves the spec, then
// hands back its receipt.
func planningExecute(conn connector.Connector) func(context.Context, execution.Request) (execution.Result, error) {
	return func(ctx context.Context, request execution.Request) (execution.Result, error) {
		plan := domain.PlanInput{
			PlanBody: "# Piano di " + request.SpecCode + "\n\nGenerato dall'agente remoto.\n",
			Tasks: []domain.Task{{
				ID:     "TASK-01",
				Title:  "Implementare",
				Type:   domain.TaskImpl,
				Status: domain.StatusTodo,
				Body:   "## Objective\nFare la cosa.\n",
			}},
		}
		if _, err := conn.SavePlan(ctx, request.SpecCode, plan); err != nil {
			return execution.Result{}, err
		}
		if _, err := conn.TransitionStatus(ctx, request.SpecCode, domain.StatusPlanned); err != nil {
			return execution.Result{}, err
		}
		return execution.Result{Payload: json.RawMessage(`{"spec_code":"` + request.SpecCode + `","status":"PLANNED","tasks":1}`), ExternalID: "task-remote-1"}, nil
	}
}

// AC-1: one press, one execution. The second request does not create a second
// record, and says which one already holds the spec.
func TestRunSpecActionStartsExactlyOneExecutionPerSpec(t *testing.T) {
	provider := blockedProvider("fake")
	srv, cfg, _ := newRunServer(t, provider, true)

	status, first := startAction(t, srv, "US-901", "plan")
	if status != http.StatusCreated {
		t.Fatalf("first POST: %d %v", status, first)
	}
	if first["status"] != string(execution.StatusRunning) || first["spec_code"] != "US-901" || first["provider_id"] != "fake" {
		t.Fatalf("the started record does not describe the run: %v", first)
	}
	startedID, _ := first["id"].(string)
	if startedID == "" {
		t.Fatalf("the started record has no id: %v", first)
	}

	status, second := startAction(t, srv, "US-901", "plan")
	if status != http.StatusConflict {
		t.Fatalf("second POST: %d %v", status, second)
	}
	message, _ := second["error"].(string)
	if !strings.Contains(message, startedID) {
		t.Fatalf("the refusal does not name the running execution: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 1 {
		t.Fatalf("a second press created %d records", got)
	}

	close(provider.release)
	awaitTerminal(t, srv, startedID)
}

// AC-2: the state is observable while the provider is still working, and the
// rest of the workspace stays reachable — the dispatch does not hold the server.
func TestRunSpecActionKeepsTheServerResponsiveWhileRunning(t *testing.T) {
	provider := blockedProvider("fake")
	srv, _, _ := newRunServer(t, provider, true)

	status, started := startAction(t, srv, "US-901", "plan")
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	<-provider.entered

	code, record := readExecution(t, srv, id)
	if code != http.StatusOK || record.Status != execution.StatusRunning || record.CompletedAt != nil {
		t.Fatalf("the running state is not observable: %d %#v", code, record)
	}
	if w := doJSON(t, srv, http.MethodGet, "/api/board", nil); w.Code != http.StatusOK {
		t.Fatalf("the board is blocked during the dispatch: %d", w.Code)
	}
	other := runSpecDetail(t, srv, "US-902")
	if other.Execution != nil {
		t.Fatalf("US-902 borrowed the execution of another spec: %#v", other.Execution)
	}
	// The spec being worked on advertises why its action cannot be pressed again.
	busy := runSpecDetail(t, srv, "US-901")
	for _, action := range busy.Actions {
		if action.ID == "plan" && (action.Runnable || !strings.Contains(action.UnavailableReason, id)) {
			t.Fatalf("the busy action is still offered: %#v", action)
		}
	}

	close(provider.release)
	awaitTerminal(t, srv, id)
}

// AC-3: success is proved by the spec being planned, not by the receipt the
// provider handed back.
func TestRunSpecActionSuccessIsProvedByThePlannedSpec(t *testing.T) {
	var conn connector.Connector
	provider := releasedProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return planningExecute(conn)(ctx, request)
	})
	srv, _, conn := newRunServer(t, provider, true)

	status, started := startAction(t, srv, "US-901", "plan")
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusSucceeded || record.Result == nil || len(record.Result.Payload) == 0 || record.CompletedAt == nil {
		t.Fatalf("terminal record: %#v", record)
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusPlanned {
		t.Fatalf("the spec is %s, not PLANNED", detail.Spec.Status)
	}
	if len(detail.Tasks) == 0 || strings.TrimSpace(detail.PlanBody) == "" {
		t.Fatalf("the plan is not readable: tasks=%d body=%q", len(detail.Tasks), detail.PlanBody)
	}
	if detail.Execution == nil || detail.Execution.ID != id {
		t.Fatalf("the detail lost the execution: %#v", detail.Execution)
	}
	for _, action := range detail.Actions {
		if action.ID == "plan" {
			t.Fatalf("plan is still offered on a PLANNED spec: %#v", action)
		}
	}
}

// AC-4a: a dispatch that fails leaves a readable reason and does not move the
// spec, and it does not lock the spec out of a retry.
func TestRunSpecActionFailureLeavesTheSpecUnplannedAndRetryable(t *testing.T) {
	provider := releasedProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{}, errRemoteRefused
	})
	srv, cfg, _ := newRunServer(t, provider, true)

	_, started := startAction(t, srv, "US-901", "plan")
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil || record.Error.Message == "" {
		t.Fatalf("failed record: %#v", record)
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusTodo || len(detail.Tasks) != 0 {
		t.Fatalf("a failed run moved the spec: status=%s tasks=%d", detail.Spec.Status, len(detail.Tasks))
	}

	status, retry := startAction(t, srv, "US-901", "plan")
	if status != http.StatusCreated {
		t.Fatalf("a failure locked the spec out of a retry: %d %v", status, retry)
	}
	retryID, _ := retry["id"].(string)
	awaitTerminal(t, srv, retryID)
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 2 {
		t.Fatalf("expected two records after a retry, got %d", got)
	}
}

// AC-4b: a provider that declares success without planning anything is not
// believed. The viewer applies the same confirmation the CLI does.
func TestRunSpecActionDemotesASuccessTheConnectorDenies(t *testing.T) {
	provider := releasedProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{Payload: json.RawMessage(`{"spec_code":"US-901","status":"PLANNED","tasks":3}`), ExternalID: "task-remote-9"}, nil
	})
	srv, _, _ := newRunServer(t, provider, true)

	_, started := startAction(t, srv, "US-901", "plan")
	id, _ := started["id"].(string)
	// The very first terminal state a client can read is already the demoted one:
	// the confirmation runs inside the terminal write, so there is no intermediate
	// SUCCEEDED for a poll to settle on. awaitTerminal is the browser's own rule,
	// which makes this assertion the regression oracle for that ordering.
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil || record.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("an unconfirmed success was believed: %#v", record)
	}
	if record.Result != nil || record.Error.ExternalID != "task-remote-9" {
		t.Fatalf("the demoted record lost the remote identifier: %#v", record)
	}
	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusTodo {
		t.Fatalf("an unconfirmed success moved the spec to %s", detail.Spec.Status)
	}
}

// gatedReadConnector holds ReadSpecDetail open once the test arms it, so the
// window between the provider's answer and the verified outcome stops being a
// race and becomes a state the test can stand still inside.
type gatedReadConnector struct {
	connector.Connector
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (c *gatedReadConnector) ReadSpecDetail(ctx context.Context, code string) (domain.Spec, error) {
	if c.armed.Load() {
		select {
		case c.entered <- struct{}{}:
		default:
		}
		select {
		case <-c.release:
		case <-ctx.Done():
			return domain.Spec{}, ctx.Err()
		}
	}
	return c.Connector.ReadSpecDetail(ctx, code)
}

// AC-4b, at the only instant where believing the provider would be visible: a
// success that has not been checked yet must not be readable as a success.
//
// The provider answers, then the confirmation is held open. Everything the
// browser polls is interrogated inside that window: if the record went to the
// store as SUCCEEDED and were demoted afterwards, the poll would read the
// success, the UI would stop polling on that first terminal status, and the run
// would settle on a screen that a moment later stopped being true.
func TestRunSpecActionNeverPublishesAnUnverifiedSuccess(t *testing.T) {
	gate := &gatedReadConnector{entered: make(chan struct{}, 1), release: make(chan struct{})}
	provider := blockedProvider("fake")
	provider.execute = func(context.Context, execution.Request) (execution.Result, error) {
		// A well-formed receipt for work that was never done: the connector will
		// deny it, so the only honest terminal state is FAILED.
		return execution.Result{Payload: json.RawMessage(`{"spec_code":"US-901","status":"PLANNED","tasks":3}`), ExternalID: "task-remote-9"}, nil
	}
	srv, _, _ := newRunServerWithConnector(t, provider, true, func(conn connector.Connector) connector.Connector {
		gate.Connector = conn
		return gate
	})

	_, started := startAction(t, srv, "US-901", "plan")
	id, _ := started["id"].(string)

	// The gate is armed only now: the POST itself reads the spec, and blocking
	// that read would stall the request instead of the confirmation.
	gate.armed.Store(true)
	close(provider.release)
	select {
	case <-gate.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the confirmation never re-read the spec")
	}

	// Inside the window, on the route the browser polls. The other reads serve
	// this very same store record, so closing it here closes it for all of them
	// — and they cannot be asserted from inside the window anyway, because they
	// would block on the armed gate themselves.
	status, record := readExecution(t, srv, id)
	if status != http.StatusOK {
		t.Fatalf("polling the execution failed with %d", status)
	}
	if record.Status != execution.StatusRunning {
		t.Fatalf("an unverified success was published as %s: %#v", record.Status, record)
	}
	if record.CompletedAt != nil || record.Result != nil {
		t.Fatalf("an unverified success was published as closed: %#v", record)
	}

	gate.armed.Store(false)
	close(gate.release)

	final := awaitTerminal(t, srv, id)
	if final.Status != execution.StatusFailed || final.Error == nil || final.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("the verified outcome is not the demoted one: %#v", final)
	}
	if final.Error.ExternalID != "task-remote-9" {
		t.Fatalf("the demoted record lost the remote identifier: %#v", final.Error)
	}
	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusTodo {
		t.Fatalf("an unconfirmed success moved the spec to %s", detail.Spec.Status)
	}
	if detail.Execution == nil || detail.Execution.Status != execution.StatusFailed {
		t.Fatalf("the spec detail disagrees with the record: %#v", detail.Execution)
	}
}

// AC-5: a viewer restarted on the same workspace finds the execution again, and
// loading the detail starts nothing.
func TestSpecDetailFindsTheExecutionAfterARestart(t *testing.T) {
	var conn connector.Connector
	provider := releasedProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return planningExecute(conn)(ctx, request)
	})
	srv, cfg, conn := newRunServer(t, provider, true)

	_, started := startAction(t, srv, "US-901", "plan")
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	before := recordFileCount(t, cfg.ProjectRoot, "US-901")

	restarted, err := NewServer(conn, cfg, execution.NewRegistry(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	detail := runSpecDetail(t, restarted, "US-901")
	if detail.Execution == nil || detail.Execution.ID != id || detail.Execution.Status != record.Status {
		t.Fatalf("the restarted viewer lost the execution: %#v", detail.Execution)
	}
	if after := recordFileCount(t, cfg.ProjectRoot, "US-901"); after != before {
		t.Fatalf("loading the detail created a record: %d -> %d", before, after)
	}
}

func TestGetExecutionAnswersNotFoundForAnUnknownID(t *testing.T) {
	srv, _, _ := newRunServer(t, releasedProvider("fake", nil), true)
	if code, _ := readExecution(t, srv, "exec-missing"); code != http.StatusNotFound {
		t.Fatalf("unknown execution: %d", code)
	}
}

func TestRunSpecActionRefusals(t *testing.T) {
	t.Run("unknown action", func(t *testing.T) {
		srv, cfg, _ := newRunServer(t, releasedProvider("fake", nil), true)
		status, body := startAction(t, srv, "US-901", "teleport")
		if status != http.StatusBadRequest {
			t.Fatalf("unknown action: %d %v", status, body)
		}
		if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
			t.Fatalf("a refused action created %d records", got)
		}
	})
	t.Run("empty action", func(t *testing.T) {
		srv, _, _ := newRunServer(t, releasedProvider("fake", nil), true)
		if status, body := startAction(t, srv, "US-901", ""); status != http.StatusBadRequest {
			t.Fatalf("empty action: %d %v", status, body)
		}
	})
	t.Run("action the status does not admit", func(t *testing.T) {
		srv, cfg, _ := newRunServer(t, releasedProvider("fake", nil), true)
		status, body := startAction(t, srv, "US-901", "implement")
		if status != http.StatusConflict {
			t.Fatalf("implement on a TODO spec: %d %v", status, body)
		}
		if message, _ := body["error"].(string); !strings.Contains(message, string(domain.StatusTodo)) {
			t.Fatalf("the refusal does not name the current status: %q", message)
		}
		if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
			t.Fatalf("a refused action created %d records", got)
		}
	})
	t.Run("no default provider", func(t *testing.T) {
		srv, cfg, _ := newRunServer(t, releasedProvider("fake", nil), false)
		status, body := startAction(t, srv, "US-901", "plan")
		if status != http.StatusConflict {
			t.Fatalf("missing default provider: %d %v", status, body)
		}
		message, _ := body["error"].(string)
		hint, _ := body["hint"].(string)
		if !strings.Contains(message, "execution.default_provider") || !strings.Contains(hint, "Execution") {
			t.Fatalf("the refusal does not send the user to the configuration: %q / %q", message, hint)
		}
		if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
			t.Fatalf("a refused action created %d records", got)
		}
		detail := runSpecDetail(t, srv, "US-901")
		for _, action := range detail.Actions {
			if action.ID == "plan" && (action.Runnable || action.UnavailableReason == "") {
				t.Fatalf("plan is offered without a configured provider: %#v", action)
			}
		}
	})
}

// The actions of a runnable spec carry the flat fields US-028 shipped plus the
// new ones, so the contract is extended and not replaced.
func TestSpecDetailMarksAdmissibleActionsRunnable(t *testing.T) {
	srv, _, _ := newRunServer(t, releasedProvider("fake", nil), true)
	detail := runSpecDetail(t, srv, "US-901")
	if len(detail.Actions) == 0 {
		t.Fatal("a TODO spec offers no action")
	}
	found := false
	for _, action := range detail.Actions {
		if action.ID != "plan" {
			continue
		}
		found = true
		if action.Label == "" || action.Skill == "" {
			t.Fatalf("the action lost its US-028 fields: %#v", action)
		}
		if !action.Runnable || action.UnavailableReason != "" {
			t.Fatalf("a startable action is not marked runnable: %#v", action)
		}
	}
	if !found {
		t.Fatal("the plan action disappeared from a TODO spec")
	}
}

// errRemoteRefused is declared once so the failing provider does not allocate a
// new error identity on every call.
var errRemoteRefused = &remoteRefusal{}

type remoteRefusal struct{}

func (*remoteRefusal) Error() string { return "the remote hub refused the task" }

// AC-4: a default provider whose runtime cannot be used takes the plan action
// away and says why, in the provider's own words.
func TestSpecDetailBlocksTheActionWhenTheDefaultProviderIsUnusable(t *testing.T) {
	const reason = "the local agent is not authenticated: run its login command"
	provider := newProbeProvider("local", reason)
	srv, _, _ := newRunServer(t, provider, true)

	detail := runSpecDetail(t, srv, "US-901")
	found := false
	for _, action := range detail.Actions {
		if action.ID != "plan" {
			continue
		}
		found = true
		if action.Runnable {
			t.Fatalf("plan is offered on an unusable provider: %#v", action)
		}
		if !strings.Contains(action.UnavailableReason, provider.ID()) {
			t.Fatalf("the reason does not name the provider: %q", action.UnavailableReason)
		}
		if !strings.Contains(action.UnavailableReason, reason) {
			t.Fatalf("the reason drops the provider's own diagnostic: %q", action.UnavailableReason)
		}
	}
	if !found {
		t.Fatal("the plan action disappeared from a TODO spec")
	}
}

// AC-4, the other half: the new check refuses an unusable runtime and nothing
// else. The same workspace with a usable provider still offers the action.
func TestSpecDetailKeepsTheActionRunnableWhenTheProviderIsUsable(t *testing.T) {
	srv, _, _ := newRunServer(t, newProbeProvider("local", ""), true)

	detail := runSpecDetail(t, srv, "US-901")
	for _, action := range detail.Actions {
		if action.ID == "plan" && (!action.Runnable || action.UnavailableReason != "") {
			t.Fatalf("a provider that reports itself available blocks the action: %#v", action)
		}
	}
}

// AC-4: the refusal is enforced on the server, not only in the payload the
// browser renders. A POST against an unusable runtime is answered with the
// reason and creates no execution at all.
func TestRunSpecActionRefusesAnUnusableProviderWithoutCreatingARecord(t *testing.T) {
	const reason = "the local agent is not installed: install it and try again"
	provider := newProbeProvider("local", reason)
	srv, cfg, _ := newRunServer(t, provider, true)

	status, body := startAction(t, srv, "US-901", "plan")
	if status != http.StatusConflict {
		t.Fatalf("unusable provider: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, provider.ID()) || !strings.Contains(message, reason) {
		t.Fatalf("the refusal is not readable: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
		t.Fatalf("a refused action created %d records", got)
	}
	// The spec must also look untouched to the browser: no execution to poll,
	// and the action still offered with its reason instead of a phantom run.
	detail := runSpecDetail(t, srv, "US-901")
	if detail.Execution != nil {
		t.Fatalf("a refused action left an execution on the spec: %#v", detail.Execution)
	}
	if detail.Spec.Status != domain.StatusTodo {
		t.Fatalf("a refused action moved the spec to %s", detail.Spec.Status)
	}
	// A second press is refused the same way, so the refusal never leaves the
	// spec reserved.
	if status, body := startAction(t, srv, "US-901", "plan"); status != http.StatusConflict {
		t.Fatalf("second POST: %d %v", status, body)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
		t.Fatalf("a second refused action created %d records", got)
	}
}

// runModelReceipt is what the stub provider reports about the configuration the
// run handed it. `present` is carried separately from the value because the
// difference this test is about — a configured model versus none at all — is
// the presence of the key, and an absent key and an empty one would otherwise
// arrive as the same empty string.
type runModelReceipt struct {
	Present bool   `json:"model_present"`
	Model   string `json:"model"`
}

// AC-3 and AC-6: the model saved in the workspace configuration is the one the
// run really uses, and no model configured means no model key travels at all.
//
// The oracle is the run record, not the saved configuration. The criterion
// speaks about the model *used* by the run, so a test that stopped at the
// configuration read back would prove something weaker: that the value was
// persisted, while the chain from persistence to Request.ProviderConfig — the
// step that actually decides which model the runtime is asked for — would stay
// unproved. Both halves are asserted here, in order, on the real routes.
//
// Only the provider is a double, and only to stand in for the external CLI:
// the server, the registry, the on-disk workspace configuration and the
// execution service are the production ones.
func TestRunSpecActionUsesTheModelSavedInTheWorkspaceConfiguration(t *testing.T) {
	const chosen = "modello-scelto"

	cases := []struct {
		name    string
		config  map[string]any
		present bool
		model   string
	}{
		// The browser saves the identifier picked from the catalog.
		{name: "a model chosen from the catalog", config: map[string]any{execution.ModelFieldName: chosen}, present: true, model: chosen},
		// AC-6: leaving the model empty omits the key entirely, so "empty" never
		// travels as a model identifier the runtime would be asked to honour.
		{name: "no model configured", config: map[string]any{}, present: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var conn connector.Connector
			provider := releasedProvider("stub", func(ctx context.Context, request execution.Request) (execution.Result, error) {
				// Read as-is: no fallback, no default, so whatever the payload
				// reports is exactly what the run carried in.
				value, present := request.ProviderConfig[execution.ModelFieldName]
				model, _ := value.(string)
				// The plan is really written, because a success the connector
				// cannot confirm is demoted to FAILED and would leave no result
				// payload to read.
				if _, err := planningExecute(conn)(ctx, request); err != nil {
					return execution.Result{}, err
				}
				payload, err := json.Marshal(map[string]any{
					"spec_code":     request.SpecCode,
					"status":        string(domain.StatusPlanned),
					"tasks":         1,
					"model_present": present,
					"model":         model,
				})
				if err != nil {
					return execution.Result{}, err
				}
				return execution.Result{Payload: payload}, nil
			})
			// withDefault is false because the default is written below through
			// the very route the panel uses, instead of being seeded by hand.
			srv, _, conn := newRunServer(t, provider, false)

			w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
				"id":     provider.ID(),
				"config": tc.config,
			})
			if w.Code != http.StatusOK {
				t.Fatalf("saving the default provider: %d %s", w.Code, w.Body.String())
			}

			// First half of AC-3: the model appears in the workspace
			// configuration the panel reads back.
			view := readProviders(t, srv)
			if view.Default == nil || view.Default.ID != provider.ID() {
				t.Fatalf("the saved default is not reported: %#v", view.Default)
			}
			saved, savedPresent := view.Default.Config[execution.ModelFieldName]
			if savedPresent != tc.present {
				t.Fatalf("model key present=%v in the saved configuration, want %v: %#v", savedPresent, tc.present, view.Default.Config)
			}
			if tc.present && saved != tc.model {
				t.Fatalf("the saved configuration carries %#v as model, want %q", saved, tc.model)
			}

			status, started := startAction(t, srv, "US-901", "plan")
			if status != http.StatusCreated {
				t.Fatalf("POST: %d %v", status, started)
			}
			id, _ := started["id"].(string)
			record := awaitTerminal(t, srv, id)
			if record.Status != execution.StatusSucceeded || record.Result == nil {
				t.Fatalf("terminal record: %#v", record)
			}

			// Second half of AC-3: the record of the run reports that very model
			// as the one the run was given.
			var receipt runModelReceipt
			if err := json.Unmarshal(record.Result.Payload, &receipt); err != nil {
				t.Fatalf("undecodable result payload %s: %v", record.Result.Payload, err)
			}
			if receipt.Present != tc.present {
				t.Fatalf("the run received model_present=%v, want %v: %s", receipt.Present, tc.present, record.Result.Payload)
			}
			if receipt.Model != tc.model {
				t.Fatalf("the run used %q as model, want %q", receipt.Model, tc.model)
			}
		})
	}
}
