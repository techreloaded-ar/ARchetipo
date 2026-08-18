package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// sentinel stands for anything the agent may have printed that must never be
// persisted: a token it read, a header it echoed, a session file it dumped. The
// provider does not know what a secret looks like, so the guarantee it can give
// is stronger and simpler — no stream ever enters the record.
const sentinel = "sk-CODEX-SENTINEL-DO-NOT-STORE"

const testSpec = "US-032"

// runOutcome is one scripted answer of the fake Runner.
type runOutcome struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	// waitForContext makes the call block until the context passed by the
	// provider is done, which is how a run that exceeds its timeout behaves
	// without any test having to sleep.
	waitForContext bool
}

// runCall records what the provider asked the operating system to do.
type runCall struct {
	dir  string
	name string
	args []string
}

// fakeRunner replaces the single seam between the provider and the operating
// system: it records every invocation and replays scripted outcomes in order,
// the last one repeating. Nothing here spawns a process, so the whole suite runs
// on a machine that has no Codex installed.
type fakeRunner struct {
	mu       sync.Mutex
	outcomes []runOutcome
	calls    []runCall
}

var _ Runner = (*fakeRunner)(nil)

func (r *fakeRunner) Run(ctx context.Context, dir string, name string, args []string) (string, string, int, error) {
	r.mu.Lock()
	index := len(r.calls)
	r.calls = append(r.calls, runCall{dir: dir, name: name, args: append([]string(nil), args...)})
	outcome := runOutcome{}
	if len(r.outcomes) > 0 {
		if index >= len(r.outcomes) {
			outcome = r.outcomes[len(r.outcomes)-1]
		} else {
			outcome = r.outcomes[index]
		}
	}
	r.mu.Unlock()

	if outcome.waitForContext {
		<-ctx.Done()
		return "", "", -1, ctx.Err()
	}
	return outcome.stdout, outcome.stderr, outcome.exitCode, outcome.err
}

func (r *fakeRunner) snapshot() []runCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runCall(nil), r.calls...)
}

// probeOK is the scripted answer of the availability probe every Execute test
// has to get past before reaching the behaviour it is about.
var probeOK = runOutcome{stdout: "codex 1.0.0"}

// fakeCommand writes an executable file and returns its absolute path, so
// exec.LookPath — which the provider calls for real — finds a command without
// the test depending on what is installed on the machine or on PATH.
func fakeCommand(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// missingCommand is a name that cannot resolve on any machine.
func missingCommand() string {
	return "archetipo-codex-that-does-not-exist"
}

// workspaceWithSkill is a working directory that has the planning skill
// installed, which is the precondition Execute checks before spawning.
func workspaceWithSkill(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, planSkillRelPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# archetipo-plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fixedElapsedClock reports a start instant and then an instant one step later,
// so the recorded duration is asserted instead of measured.
func fixedElapsedClock(step time.Duration) func() time.Time {
	base := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	calls := 0
	return func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return base.Add(step)
	}
}

func newTestProvider(dir string, runner Runner, now func() time.Time) *Provider {
	if now == nil {
		now = fixedElapsedClock(1500 * time.Millisecond)
	}
	return New(Options{
		Runner:     runner,
		WorkingDir: func() (string, error) { return dir, nil },
		Now:        now,
	})
}

func testRequest(command string) execution.Request {
	return execution.Request{
		ExecutionID:    "exec-1",
		SpecCode:       testSpec,
		Action:         execution.ActionPlan,
		Capability:     execution.CapabilitySpecPlan,
		ProviderConfig: map[string]any{"command": command},
	}
}

func receiptLine(specCode string, tasks int) string {
	return fmt.Sprintf(`{"spec_code":%q,"status":%q,"tasks":%d}`, specCode, execution.PlannedStatus, tasks)
}

func assertContains(t *testing.T, got, want, what string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%s does not mention %q: %s", what, want, got)
	}
}

// --- identity and contract -------------------------------------------------

func TestProviderDeclaresIdentityAndPlanCapability(t *testing.T) {
	p := New(Options{})
	if p.ID() != ProviderID || p.ID() != "codex" {
		t.Fatalf("id = %q", p.ID())
	}
	capabilities, err := p.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capabilities, []execution.Capability{execution.CapabilitySpecPlan}) {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}

// ValidateConfig must stay runnable on the machine a person configures before
// installing Codex, so it may not look the command up on PATH.
func TestValidateConfigAcceptsAnAbsentCommandAndRejectsAnUnknownKey(t *testing.T) {
	p := New(Options{})
	if err := p.ValidateConfig(context.Background(), map[string]any{"command": missingCommand()}); err != nil {
		t.Fatalf("validating a not-yet-installed command failed: %v", err)
	}
	err := p.ValidateConfig(context.Background(), map[string]any{"nope": "x"})
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "nope" {
		t.Fatalf("error = %v", err)
	}
}

// --- buildArgs -------------------------------------------------------------

func TestBuildArgsKeepsExecFirstAndThePromptLast(t *testing.T) {
	const prompt = "PROMPT-BODY"
	cases := []struct {
		name string
		cfg  settings
		want []string
	}{
		{
			name: "defaults",
			cfg:  settings{Command: "codex"},
			want: []string{"exec", "--full-auto", "--skip-git-repo-check", prompt},
		},
		{
			name: "with model",
			cfg:  settings{Command: "codex", Model: "gpt-5-codex"},
			want: []string{"exec", "--full-auto", "--skip-git-repo-check", "--model", "gpt-5-codex", prompt},
		},
		{
			name: "exec_args replaces the intermediate flags",
			cfg:  settings{Command: "codex", ExecArgs: []string{"--yolo", "--sandbox", "none"}},
			want: []string{"exec", "--yolo", "--sandbox", "none", prompt},
		},
		{
			name: "exec_args and model",
			cfg:  settings{Command: "codex", Model: "gpt-5-codex", ExecArgs: []string{"--yolo"}},
			want: []string{"exec", "--yolo", "--model", "gpt-5-codex", prompt},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildArgs(tc.cfg, prompt)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %#v, want %#v", got, tc.want)
			}
			if got[0] != "exec" {
				t.Fatalf("first argument = %q, want the exec subcommand", got[0])
			}
			if got[len(got)-1] != prompt {
				t.Fatalf("last argument = %q, want the prompt", got[len(got)-1])
			}
		})
	}
}

// --- buildPrompt -----------------------------------------------------------

func TestBuildPromptIsDeterministicAndAsksForTheSharedReceipt(t *testing.T) {
	req := testRequest("codex")
	first := buildPrompt(req)
	if second := buildPrompt(req); first != second {
		t.Fatalf("prompt is not deterministic:\n%s\n---\n%s", first, second)
	}
	assertContains(t, first, testSpec, "prompt")
	assertContains(t, first, "/archetipo-plan "+testSpec, "prompt")
	assertContains(t, first, `{"spec_code":"`+testSpec+`","status":"`+execution.PlannedStatus+`","tasks":<N>}`, "prompt")
	if plannedStatus != execution.PlannedStatus {
		t.Fatalf("the prompt status %q drifted from the shared one %q", plannedStatus, execution.PlannedStatus)
	}
}

// --- Available -------------------------------------------------------------

func TestAvailableReportsTheFourOutcomes(t *testing.T) {
	present := fakeCommand(t)
	cases := []struct {
		name    string
		command string
		outcome runOutcome
		wantErr []string
	}{
		{
			name:    "absent from PATH",
			command: missingCommand(),
			wantErr: []string{missingCommand(), "was not found"},
		},
		{
			name:    "present but impossible to start",
			command: present,
			outcome: runOutcome{exitCode: -1, err: errors.New("spawn codex ENOENT")},
			wantErr: []string{present, "was found but could not be run", "spawn codex ENOENT"},
		},
		{
			name:    "started but failing",
			command: present,
			outcome: runOutcome{exitCode: 127, stderr: "codex: command not found"},
			wantErr: []string{present, "exited 127", "codex: command not found"},
		},
		{
			name:    "available",
			command: present,
			outcome: probeOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{outcomes: []runOutcome{tc.outcome}}
			p := newTestProvider(t.TempDir(), runner, nil)
			err := p.Available(context.Background(), map[string]any{"command": tc.command})
			if len(tc.wantErr) == 0 {
				if err != nil {
					t.Fatalf("expected the runtime to be available: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tc.wantErr {
				assertContains(t, err.Error(), want, "availability error")
			}
		})
	}
}

// A command that never resolves must not be spawned at all: the probe answers
// from PATH alone.
func TestAvailableDoesNotSpawnAnAbsentCommand(t *testing.T) {
	runner := &fakeRunner{}
	p := newTestProvider(t.TempDir(), runner, nil)
	if err := p.Available(context.Background(), map[string]any{"command": missingCommand()}); err == nil {
		t.Fatal("expected an error")
	}
	if calls := runner.snapshot(); len(calls) != 0 {
		t.Fatalf("the probe spawned %d command(s): %#v", len(calls), calls)
	}
}

func TestAvailableRejectsAnInvalidConfiguration(t *testing.T) {
	p := newTestProvider(t.TempDir(), &fakeRunner{}, nil)
	err := p.Available(context.Background(), map[string]any{"command": "relative/codex"})
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "command" {
		t.Fatalf("error = %v", err)
	}
}

// --- Execute: success ------------------------------------------------------

func TestExecuteReturnsAPayloadBuiltFromTheReceipt(t *testing.T) {
	command := fakeCommand(t)
	dir := workspaceWithSkill(t)
	runner := &fakeRunner{outcomes: []runOutcome{
		probeOK,
		{stdout: "thinking...\ntool: read\n" + receiptLine(testSpec, 9) + "\n"},
	}}
	req := testRequest(command)
	req.ProviderConfig["model"] = "gpt-5-codex"

	got, err := newTestProvider(dir, runner, fixedElapsedClock(1500*time.Millisecond)).Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalID != "" {
		t.Fatalf("external id = %q, want empty: a local run outlives nothing", got.ExternalID)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got.Payload, &fields); err != nil {
		t.Fatalf("payload is not valid JSON (%s): %v", got.Payload, err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"command", "duration_ms", "exit_code", "model", "plan_tasks", "result_summary"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("payload fields = %v, want %v", keys, want)
	}

	var payload struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		PlanTasks     int    `json:"plan_tasks"`
		DurationMS    int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != command || payload.Model != "gpt-5-codex" || payload.ExitCode != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.PlanTasks != 9 {
		t.Fatalf("plan_tasks = %d, want the 9 declared by the receipt", payload.PlanTasks)
	}
	if payload.DurationMS != 1500 {
		t.Fatalf("duration_ms = %d", payload.DurationMS)
	}
	var summary execution.PlanReceipt
	if err := json.Unmarshal([]byte(payload.ResultSummary), &summary); err != nil {
		t.Fatalf("result_summary is not the re-rendered receipt (%s): %v", payload.ResultSummary, err)
	}
	if summary != (execution.PlanReceipt{SpecCode: testSpec, Status: execution.PlannedStatus, Tasks: 9}) {
		t.Fatalf("result_summary = %#v", summary)
	}

	calls := runner.snapshot()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want the probe and the run", len(calls))
	}
	if !reflect.DeepEqual(calls[0].args, []string{"--version"}) {
		t.Fatalf("probe args = %#v", calls[0].args)
	}
	for i, call := range calls {
		if call.dir != dir {
			t.Fatalf("call %d ran in %q, want the seam working directory %q", i, call.dir, dir)
		}
		if call.name != command {
			t.Fatalf("call %d ran %q, want the configured command %q", i, call.name, command)
		}
	}
	if !reflect.DeepEqual(calls[1].args, buildArgs(settings{Command: command, Model: "gpt-5-codex"}, buildPrompt(req))) {
		t.Fatalf("run args = %#v", calls[1].args)
	}
}

// --- Execute: failures -----------------------------------------------------

func TestExecuteFailureModes(t *testing.T) {
	command := fakeCommand(t)
	cases := []struct {
		name     string
		command  string
		skill    bool
		outcomes []runOutcome
		expired  bool
		wantErr  []string
	}{
		{
			name:    "runtime unavailable",
			command: missingCommand(),
			skill:   true,
			wantErr: []string{missingCommand(), "was not found"},
		},
		{
			name:     "planning skill not installed",
			command:  command,
			skill:    false,
			outcomes: []runOutcome{probeOK},
			wantErr:  []string{"planning skill is not installed", planSkillRelPath, "archetipo init --tool codex"},
		},
		{
			name:     "non-zero exit code",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {exitCode: 3, stderr: "codex: run aborted"}},
			wantErr:  []string{"exited 3", testSpec, "codex: run aborted"},
		},
		{
			name:     "no receipt line at all",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: "done, I planned everything\n"}},
			wantErr:  []string{"exited 0 without having produced a plan for " + testSpec, "did not emit the expected JSON receipt line"},
		},
		{
			name:     "receipt for another spec",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: receiptLine("US-999", 4) + "\n"}},
			wantErr:  []string{"exited 0 without having produced a plan for " + testSpec, "does not declare a persisted plan for " + testSpec},
		},
		{
			name:     "receipt declaring zero tasks",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: receiptLine(testSpec, 0) + "\n"}},
			wantErr:  []string{"does not declare a persisted plan for " + testSpec},
		},
		{
			name:     "timeout",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {waitForContext: true}},
			expired:  true,
			wantErr:  []string{"could not be run to completion", context.DeadlineExceeded.Error()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.skill {
				dir = workspaceWithSkill(t)
			}
			runner := &fakeRunner{outcomes: tc.outcomes}
			ctx := context.Background()
			if tc.expired {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			got, err := newTestProvider(dir, runner, nil).Execute(ctx, testRequest(tc.command))
			if err == nil {
				t.Fatalf("expected an error, got payload %s", got.Payload)
			}
			if got.Payload != nil {
				t.Fatalf("a failed execution returned a payload: %s", got.Payload)
			}
			var remote *execution.RemoteError
			if errors.As(err, &remote) {
				t.Fatalf("a local run reported a remote unit of work: %v", err)
			}
			for _, want := range tc.wantErr {
				assertContains(t, err.Error(), want, "execution error")
			}
		})
	}
}

func TestExecuteRejectsAnInvalidConfigurationBeforeSpawning(t *testing.T) {
	runner := &fakeRunner{}
	req := testRequest("codex")
	req.ProviderConfig["timeout_seconds"] = 0

	_, err := newTestProvider(workspaceWithSkill(t), runner, nil).Execute(context.Background(), req)
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "timeout_seconds" {
		t.Fatalf("error = %v", err)
	}
	if calls := runner.snapshot(); len(calls) != 0 {
		t.Fatalf("an invalid configuration spawned %d command(s)", len(calls))
	}
}

// --- no secret ever reaches the record -------------------------------------

// The provider cannot recognize a secret, so the guarantee it offers is that no
// captured stream is persisted at all. The sentinel stands in for whatever the
// agent may have printed.
func TestSuccessfulExecutionNeverPersistsTheAgentOutput(t *testing.T) {
	command := fakeCommand(t)
	runner := &fakeRunner{outcomes: []runOutcome{
		probeOK,
		{
			stdout: "reading ~/.codex/auth.json\ntoken=" + sentinel + "\n" + receiptLine(testSpec, 3) + "\n",
			stderr: "warning: " + sentinel,
		},
	}}
	got, err := newTestProvider(workspaceWithSkill(t), runner, nil).Execute(context.Background(), testRequest(command))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got.Payload), sentinel) {
		t.Fatalf("the payload carried the agent output: %s", got.Payload)
	}
}

// A failure may quote the tail of stderr, which is what makes it diagnosable,
// but stdout — the stream where the agent does its talking — never travels.
func TestFailedExecutionDoesNotEchoTheAgentStdout(t *testing.T) {
	command := fakeCommand(t)
	runner := &fakeRunner{outcomes: []runOutcome{
		probeOK,
		{exitCode: 2, stdout: "token=" + sentinel + "\n", stderr: "codex: run aborted"},
	}}
	_, err := newTestProvider(workspaceWithSkill(t), runner, nil).Execute(context.Background(), testRequest(command))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("the diagnostic carried the agent stdout: %v", err)
	}
	assertContains(t, err.Error(), "codex: run aborted", "execution error")
}

// A stderr larger than the limit is quoted only up to it, so no diagnostic can
// carry a whole stream.
func TestDiagnosticTruncatesAVeryLongStderr(t *testing.T) {
	command := fakeCommand(t)
	long := strings.Repeat("x", maxCapturedOutput*3) + sentinel
	runner := &fakeRunner{outcomes: []runOutcome{
		probeOK,
		{exitCode: 4, stderr: long},
	}}
	_, err := newTestProvider(workspaceWithSkill(t), runner, nil).Execute(context.Background(), testRequest(command))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("the tail beyond the limit was echoed: %v", err)
	}
	if len(err.Error()) > maxCapturedOutput*2 {
		t.Fatalf("the diagnostic is %d bytes long, the stream was not bounded", len(err.Error()))
	}
}

func TestDiagnosticSuffixNamesAnEmptyStream(t *testing.T) {
	if got := diagnosticSuffix("   \n"); !strings.Contains(got, "wrote nothing") {
		t.Fatalf("suffix = %q", got)
	}
}
