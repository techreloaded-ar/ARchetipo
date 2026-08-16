package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/arcipelago"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

const arcipelagoTestToken = "secret-token"

// hubBehaviour scripts the simulated ARcipelago hub for one scenario.
type hubBehaviour struct {
	// plansSuccessfully makes the hub simulate the remote agent: on the first
	// read of the task it persists a plan through the very same CLI, exactly as
	// the remote agent would, and only then reports the task completed.
	plansSuccessfully bool
	// terminalStatus and terminalSummary drive the outcome reported when the
	// hub does not simulate a planning agent.
	terminalStatus  string
	terminalSummary string
}

// hubState records what the hub saw. The handler never calls t.Fatal: it stores
// the error so the test can assert it after the run.
type hubState struct {
	mu         sync.Mutex
	posts      int
	gets       int
	createBody map[string]any
	err        error
}

func (s *hubState) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *hubState) snapshot() (posts, gets int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts, s.gets, s.err
}

// runRealRoot drives the CLI through newRootCmd, the root the shipped binary
// builds, so the provider registry under test is the real one. It mirrors
// runExecutionRoot without modifying it.
func runRealRoot(t *testing.T, args ...string) executionCLIResult {
	t.Helper()
	result := executionCLIResult{}
	root := newRootCmd(strings.NewReader(""), &result.stdout, &result.stderr)
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(&result.stdout)
	root.SetErr(&result.stderr)
	if err := root.Execute(); err != nil {
		iox.WriteError(&result.stderr, err)
		result.exit = exitCodeFor(err)
	}
	return result
}

func writeRemotePlanPayload(t *testing.T) string {
	t.Helper()
	payload := domain.PlanInput{
		PlanBody: "# US-001 — Piano di implementazione\n\nProdotto dall'agente remoto.\n",
		Tasks: []domain.Task{{
			ID:     "TASK-01",
			Title:  "Implementa la slice",
			Body:   "## Objective\n\nImplementa la slice pianificata in remoto.\n",
			Type:   "Impl",
			Status: "TODO",
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "remote-plan.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// newRemoteHub serves the shape of the real /api/external namespace.
func newRemoteHub(t *testing.T, deps executionDependencies, opts hubBehaviour) (*httptest.Server, *hubState) {
	t.Helper()
	state := &hubState{}
	planPath := ""
	if opts.plansSuccessfully {
		planPath = writeRemotePlanPayload(t)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+arcipelagoTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/external/tasks":
			state.mu.Lock()
			state.posts++
			body := map[string]any{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				state.err = err
			}
			state.createBody = body
			state.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"task":{"id":"task-remote-1","status":"queued"}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/external/tasks/"):
			state.mu.Lock()
			first := state.gets == 0
			state.gets++
			state.mu.Unlock()
			if opts.plansSuccessfully {
				if first {
					// The remote agent plans the spec through the configured
					// connector, exactly as the prompt instructs it to.
					planned := runExecutionRoot(t, deps, "spec", "plan", "US-001", "--file", planPath)
					if planned.exit != 0 {
						state.fail(fmt.Errorf("remote agent simulation failed: exit=%d stderr=%s", planned.exit, planned.stderr.String()))
					}
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"task":{"id":"task-remote-1","status":"completed","resultSummary":"{\"spec_code\":\"US-001\",\"status\":\"PLANNED\",\"tasks\":1}"}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			body, err := json.Marshal(map[string]any{"task": map[string]any{"id": "task-remote-1", "status": opts.terminalStatus, "resultSummary": opts.terminalSummary}})
			if err != nil {
				state.fail(err)
			}
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, state
}

// newArcipelagoScenario wires a temporary project, the real arcipelago provider
// against the simulated hub, a seeded spec and the workspace default provider —
// all of it through archetipo commands only, which is what makes AC-5 true by
// construction.
func newArcipelagoScenario(t *testing.T, opts hubBehaviour) (executionDependencies, *hubState) {
	t.Helper()
	t.Chdir(t.TempDir())
	// The hub needs the dependencies to replay the remote agent, and the
	// provider needs the hub's client: the registry pointer is shared across
	// copies of deps, so the real provider is registered once the server exists.
	deps := executionTestDeps(t)
	server, state := newRemoteHub(t, deps, opts)
	provider := arcipelago.New(arcipelago.Options{
		Doer:   server.Client(),
		Getenv: func(string) string { return arcipelagoTestToken },
		Sleep:  func(context.Context, time.Duration) error { return nil },
	})
	if err := deps.registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	seedExecutionSpec(t, deps)
	cfgPath := writeExecutionProviderPayload(t, "arcipelago.json", fmt.Sprintf(`{"base_url":%q,"workspace_id":"ws-1","poll_interval_seconds":1,"timeout_seconds":60}`, server.URL))
	decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "set-default", "arcipelago", "--file", cfgPath))
	return deps, state
}

func assertNoSecretInStreams(t *testing.T, result executionCLIResult) {
	t.Helper()
	if strings.Contains(result.stdout.String(), arcipelagoTestToken) || strings.Contains(result.stderr.String(), arcipelagoTestToken) {
		t.Fatal("the CLI streams leaked the application credential")
	}
}

// decodeExecutionSpecTasks reads the plan the connector persisted, which is the
// oracle for AC-2: the state of the connector, not a call counter.
func decodeExecutionSpecTasks(t *testing.T, result executionCLIResult) int {
	t.Helper()
	if result.exit != 0 {
		t.Fatalf("spec show exit=%d stderr=%s", result.exit, result.stderr.String())
	}
	var envelope struct {
		Data struct {
			Tasks []domain.Task `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result.stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return len(envelope.Data.Tasks)
}

func TestArcipelagoPlanRemoteHappyPath(t *testing.T) {
	deps, state := newArcipelagoScenario(t, hubBehaviour{plansSuccessfully: true})
	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if before.Status != domain.StatusTodo {
		t.Fatalf("spec did not start in TODO: %#v", before)
	}
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	assertNoSecretInStreams(t, result)
	run := decodeExecution(t, result)

	// AC-1: the execution links the spec to the identifier ARcipelago assigned.
	if run.Status != execution.StatusSucceeded || run.SpecCode != "US-001" || run.ProviderID != "arcipelago" || run.RequestID != "r1" {
		t.Fatalf("run = %#v", run)
	}
	if run.Result == nil || run.Result.ExternalID != "task-remote-1" {
		t.Fatalf("run result = %#v", run.Result)
	}
	posts, _, hubErr := state.snapshot()
	if hubErr != nil {
		t.Fatalf("the remote agent simulation failed: %v", hubErr)
	}
	state.mu.Lock()
	createBody := state.createBody
	state.mu.Unlock()
	if createBody["source"] != "archetipo" || createBody["workspaceId"] != "ws-1" || createBody["externalId"] != run.ID {
		t.Fatalf("create body = %#v (execution id %q)", createBody, run.ID)
	}
	if posts != 1 {
		t.Fatalf("posts = %d", posts)
	}

	// AC-2: the plan is read back from the connector, not counted.
	after := runExecutionRoot(t, deps, "spec", "show", "US-001")
	if state := decodeExecutionSpecState(t, after); state.Status != domain.StatusPlanned {
		t.Fatalf("spec status after the remote run = %s", state.Status)
	}
	if tasks := decodeExecutionSpecTasks(t, after); tasks < 1 {
		t.Fatalf("the connector holds no plan tasks: %d", tasks)
	}
	var payload struct {
		PlanTasks int `json:"plan_tasks"`
	}
	if err := json.Unmarshal(run.Result.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.PlanTasks != 1 {
		t.Fatalf("the receipt was not read: plan_tasks = %d", payload.PlanTasks)
	}
}

// decodeExecutionReuse reads the flag that tells a fresh dispatch from a record
// handed back untouched. Without it a repeated --request-id is indistinguishable
// from new work, and a record left RUNNING by an interrupted process would come
// back for ever with nothing saying so.
func decodeExecutionReuse(t *testing.T, result executionCLIResult) bool {
	t.Helper()
	if result.exit != 0 {
		t.Fatalf("exit=%d stderr=%s", result.exit, result.stderr.String())
	}
	var envelope struct {
		Data struct {
			Reused bool `json:"reused"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result.stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data.Reused
}

func TestArcipelagoPlanRemoteIsIdempotentPerRequestID(t *testing.T) {
	deps, state := newArcipelagoScenario(t, hubBehaviour{plansSuccessfully: true})
	firstResult := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	secondResult := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	if decodeExecutionReuse(t, firstResult) {
		t.Fatal("a first dispatch was reported as reused")
	}
	if !decodeExecutionReuse(t, secondResult) {
		t.Fatal("the repeated request did not report that it reused the existing execution")
	}
	first := decodeExecution(t, firstResult)
	second := decodeExecution(t, secondResult)
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("a second request created another execution: first=%#v second=%#v", first, second)
	}
	if first.CompletedAt == nil || second.CompletedAt == nil || !second.CompletedAt.Equal(*first.CompletedAt) {
		t.Fatalf("completion timestamps drifted: %v vs %v", first.CompletedAt, second.CompletedAt)
	}
	if !reflect.DeepEqual(first.Result, second.Result) {
		t.Fatalf("results drifted: %#v vs %#v", first.Result, second.Result)
	}
	if posts, _, hubErr := state.snapshot(); posts != 1 || hubErr != nil {
		t.Fatalf("posts = %d hubErr = %v", posts, hubErr)
	}
	entries, err := os.ReadDir(filepath.Join(".archetipo", "executions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("records = %d", len(entries))
	}
}

func TestArcipelagoRemoteFailurePreservesSpec(t *testing.T) {
	deps, _ := newArcipelagoScenario(t, hubBehaviour{terminalStatus: "failed", terminalSummary: "planning aborted: missing tool"})
	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	assertNoSecretInStreams(t, result)
	if result.exit != 0 {
		t.Fatalf("a remote failure must not fail the command: exit=%d stderr=%s", result.exit, result.stderr.String())
	}
	run := decodeExecution(t, result)
	if run.Status != execution.StatusFailed || run.Error == nil || run.Error.Code != "PROVIDER_ERROR" {
		t.Fatalf("run = %#v", run)
	}
	for _, want := range []string{"failed", "planning aborted: missing tool", "task-remote-1"} {
		if !strings.Contains(run.Error.Message, want) {
			t.Fatalf("error message misses %q: %s", want, run.Error.Message)
		}
	}
	after := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if after.Status != domain.StatusTodo || !reflect.DeepEqual(before, after) {
		t.Fatalf("the spec changed across a remote failure: before=%#v after=%#v", before, after)
	}
}

// Without the receipt check a remote task that terminates without planning
// would produce a SUCCEEDED execution on a spec still in TODO.
func TestArcipelagoCompletedWithoutPlanFailsExecution(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary string
	}{
		{"no receipt at all", "done"},
		{"receipt reporting another status", `{"spec_code":"US-001","status":"TODO","tasks":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, _ := newArcipelagoScenario(t, hubBehaviour{terminalStatus: "completed", terminalSummary: tc.summary})
			result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
			assertNoSecretInStreams(t, result)
			if result.exit != 0 {
				t.Fatalf("exit=%d stderr=%s", result.exit, result.stderr.String())
			}
			run := decodeExecution(t, result)
			if run.Status != execution.StatusFailed || run.Error == nil || run.Error.Code != "PROVIDER_ERROR" {
				t.Fatalf("run = %#v", run)
			}
			if !strings.Contains(run.Error.Message, "without having produced a plan") {
				t.Fatalf("error message does not report the missing plan: %s", run.Error.Message)
			}
			if state := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001")); state.Status != domain.StatusTodo {
				t.Fatalf("the spec left TODO without a plan: %s", state.Status)
			}
		})
	}
}

// A receipt is a declaration, not an inspection. A remote skill that dies
// halfway, or an agent that hallucinates its own closure, emits a well-formed
// receipt on a spec nobody planned. The provider cannot tell the difference —
// it has no connector, by design — so the command re-reads the spec and refuses
// a success the connector does not back.
func TestArcipelagoValidReceiptWithoutAPlanIsNotASuccess(t *testing.T) {
	deps, _ := newArcipelagoScenario(t, hubBehaviour{
		terminalStatus:  "completed",
		terminalSummary: `{"spec_code":"US-001","status":"PLANNED","tasks":3}`,
	})
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	assertNoSecretInStreams(t, result)
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing {
		t.Fatalf("a receipt the connector denies did not fail the command: exit=%d code=%s text=%q", exit, code, text)
	}
	for _, want := range []string{"US-001", string(domain.StatusTodo), string(domain.StatusPlanned)} {
		if !strings.Contains(text, want) {
			t.Fatalf("the message misses %q: %q", want, text)
		}
	}

	// The record is the only trace left of the remote task, so it must carry the
	// reason and the remote identifier instead of a success that never happened.
	id := execution.DeriveID("US-001", execution.ActionPlan, "arcipelago", "r1")
	record := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", id))
	if record.Status != execution.StatusFailed || record.Result != nil || record.Error == nil {
		t.Fatalf("record = %#v", record)
	}
	if record.Error.Code != "UNCONFIRMED_EFFECT" || record.Error.ExternalID != "task-remote-1" {
		t.Fatalf("record error = %#v", record.Error)
	}
	if !strings.Contains(record.Error.Message, "US-001") || !strings.Contains(record.Error.Message, string(domain.StatusTodo)) {
		t.Fatalf("the record does not report the reason: %s", record.Error.Message)
	}
	if state := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001")); state.Status != domain.StatusTodo {
		t.Fatalf("the spec left TODO without a plan: %s", state.Status)
	}
}

// The real root must reach the provider: an E_PRECONDITION "is not registered"
// here would mean the registration is missing.
func TestArcipelagoProviderIsRegisteredInRealRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	deps := executionTestDeps(t)
	seedExecutionSpec(t, deps)
	result := runRealRoot(t, "execution", "run", "US-001", "plan", "--provider", "arcipelago")
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput {
		t.Fatalf("exit=%d code=%s text=%q", exit, code, text)
	}
	if !strings.Contains(text, "base_url") {
		t.Fatalf("the message does not name the missing field: %q", text)
	}
	if strings.Contains(text, "execution.default_provider.config") {
		t.Fatalf("the remedy points at a path an explicit --provider does not read: %q", text)
	}
}

func TestArcipelagoRunRejectsEmptyRequestID(t *testing.T) {
	deps, _ := newArcipelagoScenario(t, hubBehaviour{terminalStatus: "completed", terminalSummary: "done"})
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "  ")
	exit, code, _ := decodeExecutionError(t, result)
	if exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput {
		t.Fatalf("exit=%d code=%s", exit, code)
	}
	if _, err := os.Stat(filepath.Join(".archetipo", "executions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an invalid usage created a record: %v", err)
	}
}
