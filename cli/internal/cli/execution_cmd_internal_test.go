package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

type executionFakeProvider struct {
	id           string
	capabilities []execution.Capability
	result       execution.Result
	err          error
	calls        int
	requests     []execution.Request
	validate     func(map[string]any) error
	effect       func()
}

func (p *executionFakeProvider) ID() string { return p.id }
func (p *executionFakeProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return p.capabilities, nil
}
func (p *executionFakeProvider) ValidateConfig(_ context.Context, config map[string]any) error {
	if p.validate != nil {
		return p.validate(config)
	}
	return nil
}
func (p *executionFakeProvider) Execute(_ context.Context, request execution.Request) (execution.Result, error) {
	p.calls++
	p.requests = append(p.requests, request)
	// effect stands for the work a real provider makes happen elsewhere. A
	// provider that claims a plan without one is refused by the run command, so a
	// fake that only returns a payload cannot stand in for a successful plan.
	if p.effect != nil {
		p.effect()
	}
	return p.result, p.err
}

// planExecutionSpec persists a plan for the seeded spec through the CLI and the
// configured connector, exactly as the remote agent of a real provider does.
func planExecutionSpec(t *testing.T, deps executionDependencies) {
	t.Helper()
	body, err := json.Marshal(domain.PlanInput{
		PlanBody: "# US-001 — Piano di implementazione\n\nProdotto dal provider.\n",
		Tasks: []domain.Task{{
			ID:     "TASK-01",
			Title:  "Implementa la slice",
			Body:   "## Objective\n\nImplementa la slice pianificata.\n",
			Type:   "Impl",
			Status: "TODO",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if planned := runExecutionRoot(t, deps, "spec", "plan", "US-001", "--file", path); planned.exit != 0 {
		t.Fatalf("seeding the plan failed: exit=%d stderr=%s", planned.exit, planned.stderr.String())
	}
}

type executionCLIResult struct {
	exit           int
	stdout, stderr bytes.Buffer
}

func runExecutionRoot(t *testing.T, deps executionDependencies, args ...string) executionCLIResult {
	t.Helper()
	result := executionCLIResult{}
	root := newRootCmdWithExecution(strings.NewReader(""), &result.stdout, &result.stderr, deps)
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

func executionTestDeps(t *testing.T, providers ...execution.Provider) executionDependencies {
	t.Helper()
	registry := execution.NewRegistry()
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	sequence := 0
	return executionDependencies{registry: registry, newID: func() (string, error) { sequence++; return "exec-00" + string(rune('0'+sequence)), nil }, now: func() time.Time { now = now.Add(time.Second); return now }, storeFactory: func(root string) (execution.Store, error) { return execution.NewFileStore(root) }}
}

func seedExecutionSpec(t *testing.T, deps executionDependencies) {
	t.Helper()
	body := `{"specs":[{"code":"US-001","title":"First","priority":"HIGH","points":3,"status":"TODO","epic":{"code":"EP-001","title":"Epic"}}]}`
	path := filepath.Join(".", "specs.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runExecutionRoot(t, deps, "spec", "add", "--file", path)
	if result.exit != 0 {
		t.Fatalf("seed failed: %s", result.stderr.String())
	}
}

func decodeExecution(t *testing.T, result executionCLIResult) execution.Execution {
	t.Helper()
	if result.exit != 0 {
		t.Fatalf("exit=%d stderr=%s", result.exit, result.stderr.String())
	}
	var envelope struct {
		Kind string              `json:"kind"`
		Data execution.Execution `json:"data"`
	}
	if err := json.Unmarshal(result.stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Kind != "execution" {
		t.Fatalf("kind=%s", envelope.Kind)
	}
	return envelope.Data
}

func decodeExecutionError(t *testing.T, result executionCLIResult) (int, string, string) {
	t.Helper()
	var envelope iox.ErrorEnvelope
	if err := json.Unmarshal(result.stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", result.stderr.String(), err)
	}
	return result.exit, envelope.Error.Code, envelope.Error.Message + " " + envelope.Error.Hint
}

func assertExecutionJSONEqual(t *testing.T, want, got json.RawMessage) {
	t.Helper()
	var wantValue, gotValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("decode expected JSON %q: %v", want, err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode actual JSON %q: %v", got, err)
	}
	if !reflect.DeepEqual(wantValue, gotValue) {
		t.Fatalf("JSON mismatch: want=%v got=%v", wantValue, gotValue)
	}
}

type executionSpecState struct {
	Status  domain.Status
	History []domain.StatusChange
}

func decodeExecutionSpecState(t *testing.T, result executionCLIResult) executionSpecState {
	t.Helper()
	if result.exit != 0 {
		t.Fatalf("spec show exit=%d stderr=%s", result.exit, result.stderr.String())
	}
	var envelope struct {
		Kind string `json:"kind"`
		Data struct {
			Spec domain.Spec `json:"spec"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result.stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode spec show %q: %v", result.stdout.String(), err)
	}
	if envelope.Kind != "spec" {
		t.Fatalf("spec show kind=%q", envelope.Kind)
	}
	return executionSpecState{Status: envelope.Data.Spec.Status, History: envelope.Data.Spec.History}
}

func TestExecutionRunShowSuccess(t *testing.T) {
	t.Chdir(t.TempDir())
	wantPayload := json.RawMessage(`{"artifact":"plan-123"}`)
	success := &executionFakeProvider{id: "fake-success", capabilities: []execution.Capability{execution.CapabilitySpecPlan}, result: execution.Result{Payload: wantPayload}}
	deps := executionTestDeps(t, success)
	seedExecutionSpec(t, deps)
	success.effect = func() { planExecutionSpec(t, deps) }
	run := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--provider", "fake-success"))
	if run.Status != execution.StatusSucceeded || run.ProviderID != "fake-success" || run.SpecCode != "US-001" || run.Capability != execution.CapabilitySpecPlan || run.Result == nil || run.Error != nil || success.calls != 1 {
		t.Fatalf("run=%#v calls=%d", run, success.calls)
	}
	show := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", run.ID))
	if show.ID != run.ID || show.Status != run.Status || show.Result == nil {
		t.Fatalf("show=%#v", show)
	}
	assertExecutionJSONEqual(t, wantPayload, run.Result.Payload)
	assertExecutionJSONEqual(t, run.Result.Payload, show.Result.Payload)
	persistedBody, err := os.ReadFile(filepath.Join(".archetipo", "executions", run.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted execution.Execution
	if err := json.Unmarshal(persistedBody, &persisted); err != nil {
		t.Fatalf("decode persisted execution: %v", err)
	}
	if persisted.Result == nil {
		t.Fatalf("persisted execution has no result: %#v", persisted)
	}
	assertExecutionJSONEqual(t, wantPayload, persisted.Result.Payload)
	entries, err := os.ReadDir(filepath.Join(".archetipo", "executions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("records=%d", len(entries))
	}
}

func TestExecutionRunFailurePreservesSpec(t *testing.T) {
	t.Chdir(t.TempDir())
	failed := &executionFakeProvider{id: "fake-failure", capabilities: []execution.Capability{execution.CapabilitySpecPlan}, err: errors.New("remote failed")}
	deps := executionTestDeps(t, failed)
	seedExecutionSpec(t, deps)
	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	run := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--provider", "fake-failure"))
	after := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if run.Status != execution.StatusFailed || run.Error == nil || failed.calls != 1 {
		t.Fatalf("run=%#v calls=%d", run, failed.calls)
	}
	if before.Status != domain.StatusTodo || !reflect.DeepEqual(before, after) {
		t.Fatalf("spec changed across failure: before=%#v after=%#v", before, after)
	}
	show := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", run.ID))
	if show.Error == nil || show.Status != execution.StatusFailed || !reflect.DeepEqual(show.Error, run.Error) {
		t.Fatalf("show=%#v", show)
	}
}

func TestExecutionRejectedBeforeDispatch(t *testing.T) {
	t.Chdir(t.TempDir())
	unsupported := &executionFakeProvider{id: "unsupported", capabilities: []execution.Capability{"other"}}
	deps := executionTestDeps(t, unsupported)
	seedExecutionSpec(t, deps)
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--provider", "unsupported")
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing || !strings.Contains(text, "spec.plan") || unsupported.calls != 0 {
		t.Fatalf("exit=%d code=%s text=%q calls=%d", exit, code, text, unsupported.calls)
	}
	if _, err := os.Stat(filepath.Join(".archetipo", "executions")); !os.IsNotExist(err) {
		t.Fatalf("store should not exist: %v", err)
	}
	for _, tc := range []struct {
		args []string
		exit int
		code string
	}{{[]string{"execution", "run", "US-001", "plan", "--provider", "missing"}, 4, "E_PRECONDITION"}, {[]string{"execution", "run", "US-001", "unknown", "--provider", "unsupported"}, 2, "E_INVALID_INPUT"}, {[]string{"execution", "run", "US-001", "plan"}, 4, "E_PRECONDITION"}, {[]string{"execution", "show", "missing"}, 1, "E_NOT_FOUND"}} {
		got := runExecutionRoot(t, deps, tc.args...)
		exit, code, _ := decodeExecutionError(t, got)
		if exit != tc.exit || code != tc.code {
			t.Fatalf("%v: exit=%d code=%s", tc.args, exit, code)
		}
	}
}

func writeExecutionProviderPayload(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeExecutionProvider(t *testing.T, result executionCLIResult) executionProviderSelection {
	t.Helper()
	if result.exit != 0 {
		t.Fatalf("exit=%d stderr=%s", result.exit, result.stderr.String())
	}
	var envelope struct {
		Kind string                     `json:"kind"`
		Data executionProviderSelection `json:"data"`
	}
	if err := json.Unmarshal(result.stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Kind != "execution_provider" {
		t.Fatalf("kind=%q", envelope.Kind)
	}
	return envelope.Data
}

func TestExecutionProviderSetShowAndRollback(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".archetipo", 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# preserve me\nconnector: file\ncustom:\n  keep: true\n"
	if err := os.WriteFile(filepath.Join(".archetipo", "config.yaml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &executionFakeProvider{
		id: "fake-valid", capabilities: []execution.Capability{execution.CapabilitySpecPlan},
		validate: func(config map[string]any) error {
			endpoint, _ := config["endpoint"].(string)
			if !strings.HasPrefix(endpoint, "https://") {
				return &execution.ConfigurationError{Field: "endpoint", Reason: "must use https"}
			}
			return nil
		},
	}
	deps := executionTestDeps(t, provider)
	validPath := writeExecutionProviderPayload(t, "valid.yaml", "endpoint: https://runner.test\nnested:\n  region: eu\n")
	set := decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "set-default", "fake-valid", "--file", validPath))
	if set.ID != "fake-valid" || set.Config["endpoint"] != "https://runner.test" {
		t.Fatalf("set=%#v", set)
	}
	show := decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "show-default"))
	if !reflect.DeepEqual(show, set) {
		t.Fatalf("show=%#v set=%#v", show, set)
	}
	validBytes, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(validBytes), "# preserve me") || !strings.Contains(string(validBytes), "custom:") {
		t.Fatalf("config lost unrelated content:\n%s", validBytes)
	}
	invalidPath := writeExecutionProviderPayload(t, "invalid.yaml", "endpoint: http://invalid\n")
	invalid := runExecutionRoot(t, deps, "execution", "provider", "set-default", "fake-valid", "--file", invalidPath)
	exit, code, text := decodeExecutionError(t, invalid)
	if exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput || !strings.Contains(text, "execution.default_provider.config.endpoint") {
		t.Fatalf("exit=%d code=%s text=%q", exit, code, text)
	}
	after, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, validBytes) {
		t.Fatal("invalid selection changed config bytes")
	}
	show = decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "show-default"))
	if show.Config["endpoint"] != "https://runner.test" {
		t.Fatalf("previous selection lost: %#v", show)
	}
}

func TestExecutionProviderRejectsUnknownNonMappingAndMissingDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	provider := &executionFakeProvider{id: "known", capabilities: []execution.Capability{execution.CapabilitySpecPlan}}
	deps := executionTestDeps(t, provider)
	missing := runExecutionRoot(t, deps, "execution", "provider", "show-default")
	if exit, code, _ := decodeExecutionError(t, missing); exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing {
		t.Fatalf("missing exit=%d code=%s", exit, code)
	}
	invalidAction := runExecutionRoot(t, deps, "execution", "run", "US-001", "unknown")
	if exit, code, _ := decodeExecutionError(t, invalidAction); exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput {
		t.Fatalf("invalid action exit=%d code=%s", exit, code)
	}
	mapPath := writeExecutionProviderPayload(t, "map.yaml", "{}\n")
	unknown := runExecutionRoot(t, deps, "execution", "provider", "set-default", "missing", "--file", mapPath)
	if exit, code, text := decodeExecutionError(t, unknown); exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput || !strings.Contains(text, "execution.default_provider.id") {
		t.Fatalf("unknown exit=%d code=%s text=%q", exit, code, text)
	}
	listPath := writeExecutionProviderPayload(t, "list.yaml", "- one\n")
	nonMapping := runExecutionRoot(t, deps, "execution", "provider", "set-default", "known", "--file", listPath)
	if exit, code, text := decodeExecutionError(t, nonMapping); exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput || !strings.Contains(text, "execution.default_provider.config") {
		t.Fatalf("mapping exit=%d code=%s text=%q", exit, code, text)
	}
	nullPath := writeExecutionProviderPayload(t, "null.yaml", "null\n")
	nullConfig := runExecutionRoot(t, deps, "execution", "provider", "set-default", "known", "--file", nullPath)
	if exit, code, _ := decodeExecutionError(t, nullConfig); exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput {
		t.Fatalf("null config exit=%d code=%s", exit, code)
	}
}

func TestExecutionRunUsesDefaultAndExplicitOverrideWins(t *testing.T) {
	t.Chdir(t.TempDir())
	defaultProvider := &executionFakeProvider{id: "fake-default", capabilities: []execution.Capability{execution.CapabilitySpecPlan}, result: execution.Result{Payload: json.RawMessage(`{"provider":"default"}`)}}
	overrideProvider := &executionFakeProvider{id: "fake-override", capabilities: []execution.Capability{execution.CapabilitySpecPlan}, result: execution.Result{Payload: json.RawMessage(`{"provider":"override"}`)}}
	deps := executionTestDeps(t, defaultProvider, overrideProvider)
	seedExecutionSpec(t, deps)
	// Only the first dispatch has to produce the plan: the second one runs
	// against a spec the connector already reports as planned.
	defaultProvider.effect = func() { planExecutionSpec(t, deps) }
	payload := writeExecutionProviderPayload(t, "default.yaml", "endpoint: https://runner.test\n")
	decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "set-default", "fake-default", "--file", payload))
	first := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "plan"))
	if first.ProviderID != "fake-default" || defaultProvider.calls != 1 || len(defaultProvider.requests) != 1 || defaultProvider.requests[0].ProviderConfig["endpoint"] != "https://runner.test" {
		t.Fatalf("first=%#v requests=%#v", first, defaultProvider.requests)
	}
	second := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--provider", "fake-override"))
	if second.ProviderID != "fake-override" || overrideProvider.calls != 1 || len(overrideProvider.requests[0].ProviderConfig) != 0 || defaultProvider.calls != 1 {
		t.Fatalf("second=%#v default calls=%d override=%#v", second, defaultProvider.calls, overrideProvider.requests)
	}
}

func TestExecutionRunRejectsManuallyInvalidDefaultBeforeRecord(t *testing.T) {
	t.Chdir(t.TempDir())
	provider := &executionFakeProvider{
		id: "fake-default", capabilities: []execution.Capability{execution.CapabilitySpecPlan}, result: execution.Result{Payload: json.RawMessage(`{"ok":true}`)},
		validate: func(config map[string]any) error {
			if config["endpoint"] != "https://runner.test" {
				return &execution.ConfigurationError{Field: "endpoint", Reason: "is invalid"}
			}
			return nil
		},
	}
	deps := executionTestDeps(t, provider)
	seedExecutionSpec(t, deps)
	payload := writeExecutionProviderPayload(t, "default.yaml", "endpoint: https://runner.test\n")
	decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "set-default", "fake-default", "--file", payload))
	configPath := filepath.Join(".archetipo", "config.yaml")
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte("https://runner.test"), []byte("http://tampered"), 1)
	if err := os.WriteFile(configPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan")
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput || !strings.Contains(text, "execution.default_provider.config.endpoint") || provider.calls != 0 {
		t.Fatalf("exit=%d code=%s text=%q calls=%d", exit, code, text, provider.calls)
	}
	entries, err := os.ReadDir(filepath.Join(".archetipo", "executions"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("unexpected records: %v", entries)
	}
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// implementExecutionSpec carries the persisted plan out through the CLI exactly
// as an honest remote agent does: every task is closed and the spec reaches
// REVIEW. A provider that only returns a payload cannot stand in for a
// successful implementation, because the run command verifies the effect
// against the connector before reporting success.
func implementExecutionSpec(t *testing.T, deps executionDependencies) {
	t.Helper()
	if done := runExecutionRoot(t, deps, "task", "done", "US-001", "TASK-01"); done.exit != 0 {
		t.Fatalf("closing the task failed: exit=%d stderr=%s", done.exit, done.stderr.String())
	}
	if review := runExecutionRoot(t, deps, "spec", "review", "US-001"); review.exit != 0 {
		t.Fatalf("moving the spec to review failed: exit=%d stderr=%s", review.exit, review.stderr.String())
	}
}

func TestExecutionRunImplementRejectsProviderWithoutCapability(t *testing.T) {
	t.Chdir(t.TempDir())
	planner := &executionFakeProvider{id: "fake-planner", capabilities: []execution.Capability{execution.CapabilitySpecPlan}}
	deps := executionTestDeps(t, planner)
	seedExecutionSpec(t, deps)
	planner.effect = func() { planExecutionSpec(t, deps) }
	if run := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--provider", "fake-planner"); run.exit != 0 {
		t.Fatalf("seeding the plan through the provider failed: %s", run.stderr.String())
	}
	planner.effect = nil
	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	records, err := os.ReadDir(filepath.Join(".archetipo", "executions"))
	if err != nil {
		t.Fatal(err)
	}
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "implement", "--provider", "fake-planner")
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing || !strings.Contains(text, string(execution.CapabilitySpecImplement)) || planner.calls != 1 {
		t.Fatalf("exit=%d code=%s text=%q calls=%d", exit, code, text, planner.calls)
	}
	after := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("spec changed across a refused start: before=%#v after=%#v", before, after)
	}
	nowRecords, err := os.ReadDir(filepath.Join(".archetipo", "executions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nowRecords) != len(records) {
		t.Fatalf("records=%d before=%d", len(nowRecords), len(records))
	}
}

func TestExecutionRunImplementRequiresPersistedPlan(t *testing.T) {
	t.Chdir(t.TempDir())
	implementer := &executionFakeProvider{id: "fake-implementer", capabilities: []execution.Capability{execution.CapabilitySpecImplement}, result: execution.Result{Payload: json.RawMessage(`{"ok":true}`)}}
	deps := executionTestDeps(t, implementer)
	seedExecutionSpec(t, deps)
	if moved := runExecutionRoot(t, deps, "spec", "move", "US-001", "--to", "planned"); moved.exit != 0 {
		t.Fatalf("moving the spec to planned failed: %s", moved.stderr.String())
	}
	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "implement", "--provider", "fake-implementer")
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing || !strings.Contains(text, "US-001") || implementer.calls != 0 {
		t.Fatalf("exit=%d code=%s text=%q calls=%d", exit, code, text, implementer.calls)
	}
	if state := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001")); state.Status != domain.StatusPlanned {
		t.Fatalf("spec moved without a plan: %#v", state)
	}
	entries, err := os.ReadDir(filepath.Join(".archetipo", "executions"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("unexpected records: %v", entries)
	}
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestExecutionRunImplementStartsAndConfirmsTheEffect(t *testing.T) {
	t.Chdir(t.TempDir())
	implementer := &executionFakeProvider{id: "fake-implementer", capabilities: []execution.Capability{execution.CapabilitySpecImplement}, result: execution.Result{Payload: json.RawMessage(`{"tasks_done":1}`)}}
	deps := executionTestDeps(t, implementer)
	seedExecutionSpec(t, deps)
	planExecutionSpec(t, deps)
	var duringDispatch domain.Status
	implementer.effect = func() {
		duringDispatch = decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001")).Status
		implementExecutionSpec(t, deps)
	}
	run := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "implement", "--provider", "fake-implementer"))
	if run.Status != execution.StatusSucceeded || run.Error != nil || run.Result == nil || run.Capability != execution.CapabilitySpecImplement || implementer.calls != 1 {
		t.Fatalf("run=%#v calls=%d", run, implementer.calls)
	}
	// The spec is IN PROGRESS while the agent works, so an accepted start is
	// visible before the run ends rather than only once it succeeds.
	if duringDispatch != domain.StatusInProgress {
		t.Fatalf("spec during dispatch=%s", duringDispatch)
	}
	if state := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001")); state.Status != domain.StatusReview {
		t.Fatalf("spec after success=%#v", state)
	}
	show := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", run.ID))
	if show.Status != execution.StatusSucceeded || show.SpecCode != "US-001" || show.Action != execution.ActionImplement {
		t.Fatalf("show=%#v", show)
	}
}

// Replaying the same --request-id returns the record already created, and it
// must not move the spec on the way there: a spec that already reached REVIEW
// stays there, because the start effect only ever moves a spec forward.
func TestExecutionRunImplementReplayKeepsTheReviewedSpecInReview(t *testing.T) {
	t.Chdir(t.TempDir())
	implementer := &executionFakeProvider{id: "fake-implementer", capabilities: []execution.Capability{execution.CapabilitySpecImplement}, result: execution.Result{Payload: json.RawMessage(`{"tasks_done":1}`)}}
	deps := executionTestDeps(t, implementer)
	seedExecutionSpec(t, deps)
	planExecutionSpec(t, deps)
	implementer.effect = func() { implementExecutionSpec(t, deps) }
	first := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "implement", "--provider", "fake-implementer", "--request-id", "k"))
	if first.Status != execution.StatusSucceeded {
		t.Fatalf("first run=%#v", first)
	}
	replay := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "implement", "--provider", "fake-implementer", "--request-id", "k"))
	if replay.ID != first.ID || replay.Status != execution.StatusSucceeded || implementer.calls != 1 {
		t.Fatalf("replay=%#v calls=%d, want the record already created and no second dispatch", replay, implementer.calls)
	}
	if state := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001")); state.Status != domain.StatusReview {
		t.Fatalf("spec after the replay=%s, want it left in %s", state.Status, domain.StatusReview)
	}
}

func TestExecutionRunImplementFailureKeepsSpecOutOfReview(t *testing.T) {
	t.Chdir(t.TempDir())
	failing := &executionFakeProvider{id: "fake-implementer", capabilities: []execution.Capability{execution.CapabilitySpecImplement}, err: errors.New("remote implementation failed")}
	deps := executionTestDeps(t, failing)
	seedExecutionSpec(t, deps)
	planExecutionSpec(t, deps)
	run := decodeExecution(t, runExecutionRoot(t, deps, "execution", "run", "US-001", "implement", "--provider", "fake-implementer"))
	if run.Status != execution.StatusFailed || run.Error == nil || strings.TrimSpace(run.Error.Message) == "" || failing.calls != 1 {
		t.Fatalf("run=%#v calls=%d", run, failing.calls)
	}
	state := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if state.Status == domain.StatusReview || state.Status != domain.StatusInProgress {
		t.Fatalf("spec after failure=%#v", state)
	}
	show := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", run.ID))
	if show.Status != execution.StatusFailed || show.Error == nil || show.Error.Message != run.Error.Message {
		t.Fatalf("show=%#v", show)
	}
}
