package claude

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
	"unicode/utf8"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// sentinel stands for anything the agent may have printed that must never be
// persisted: a token it read, a header it echoed, a session file it dumped. The
// provider does not know what a secret looks like, so the guarantee it can give
// is stronger and simpler — no stream ever enters the record.
const sentinel = "sk-CLAUDE-SENTINEL-DO-NOT-STORE"

const testSpec = "US-033"

// runOutcome is one scripted answer of the fake Runner.
type runOutcome struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	// waitForContext makes the call block until the context passed by the
	// provider is done, which is how a run that exceeds its timeout behaves
	// without any test having to sleep. It then answers with the shape the real
	// runner produces for a process killed by a context — exit code -1 and no
	// error, because os/exec reports `signal: killed` as an ExitError — and not
	// with the context error, which the real runner never returns from a
	// process that had actually started. TestRunnerReportsAProcessKilledByThe
	// ContextAsAnOrdinaryExit pins that shape against os/exec itself.
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
// on a machine that has no Claude Code installed.
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
		return "", "", -1, nil
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
var probeOK = runOutcome{stdout: "2.1.234 (Claude Code)"}

// fakeCommand writes an executable file and returns its absolute path, so
// exec.LookPath — which the provider calls for real — finds a command without
// the test depending on what is installed on the machine or on PATH.
func fakeCommand(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude-fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// missingCommand is a name that cannot resolve on any machine.
func missingCommand() string {
	return "archetipo-claude-that-does-not-exist"
}

// workspaceWithSkill is a working directory that has the planning skill
// installed, which is the precondition Execute checks before spawning.
func workspaceWithSkill(t *testing.T) string {
	t.Helper()
	return installSkillIn(t, t.TempDir())
}

func installSkillIn(t *testing.T, dir string) string {
	t.Helper()
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
	base := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
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
	if p.ID() != ProviderID || p.ID() != "claude" {
		t.Fatalf("id = %q", p.ID())
	}
	capabilities, err := p.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(capabilities, []execution.Capability{execution.CapabilitySpecPlan}) {
		t.Fatalf("capabilities = %#v, want spec.plan alone", capabilities)
	}
}

// ValidateConfig must stay runnable on the machine a person configures before
// installing Claude Code, so it may not look the command up on PATH.
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

func TestBuildArgsKeepsPrintFirstAndThePromptLast(t *testing.T) {
	const prompt = "PROMPT-BODY"
	cases := []struct {
		name string
		cfg  settings
		want []string
	}{
		{
			name: "defaults",
			cfg:  settings{Command: "claude"},
			want: []string{"--print", "--no-session-persistence", "--permission-mode", "auto", prompt},
		},
		{
			name: "with model",
			cfg:  settings{Command: "claude", Model: "opus"},
			want: []string{"--print", "--no-session-persistence", "--permission-mode", "auto", "--model", "opus", prompt},
		},
		{
			name: "print_args replaces the intermediate flags",
			cfg:  settings{Command: "claude", PrintArgs: []string{"--permission-mode", "bypassPermissions"}},
			want: []string{"--print", "--permission-mode", "bypassPermissions", prompt},
		},
		{
			name: "print_args and model",
			cfg:  settings{Command: "claude", Model: "sonnet", PrintArgs: []string{"--permission-mode", "acceptEdits"}},
			want: []string{"--print", "--permission-mode", "acceptEdits", "--model", "sonnet", prompt},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildArgs(tc.cfg, prompt)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %#v, want %#v", got, tc.want)
			}
			if got[0] != "--print" {
				t.Fatalf("first argument = %q, want the non-interactive print flag", got[0])
			}
			if got[len(got)-1] != prompt {
				t.Fatalf("last argument = %q, want the prompt", got[len(got)-1])
			}
		})
	}
}

// --- buildPrompt -----------------------------------------------------------

func TestBuildPromptIsDeterministicAndAsksForTheSharedReceipt(t *testing.T) {
	req := testRequest("claude")
	first := buildPrompt(req)
	if second := buildPrompt(req); first != second {
		t.Fatalf("prompt is not deterministic:\n%s\n---\n%s", first, second)
	}
	assertContains(t, first, testSpec, "prompt")
	assertContains(t, first, "/archetipo-plan "+testSpec, "prompt")
	assertContains(t, first, "Persist the plan through the configured connector", "prompt")
	assertContains(t, first, execution.PlannedStatus, "prompt")
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
			outcome: runOutcome{exitCode: -1, err: errors.New("spawn claude ENOENT")},
			wantErr: []string{present, "was found but could not be run", "spawn claude ENOENT"},
		},
		{
			name:    "started but failing",
			command: present,
			outcome: runOutcome{exitCode: 127, stderr: "claude: command not found"},
			wantErr: []string{present, "exited 127", "claude: command not found"},
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
				calls := runner.snapshot()
				if len(calls) != 1 || !reflect.DeepEqual(calls[0].args, []string{"--version"}) {
					t.Fatalf("the probe is not a --version call: %#v", calls)
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
	err := p.Available(context.Background(), map[string]any{"command": "relative/claude"})
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "command" {
		t.Fatalf("error = %v", err)
	}
}

// The working directory is the workspace the run happens in, so it must be read
// when the run happens and not frozen when the provider is built: the CLI
// constructs the registry before it resolves `-C`.
func TestWorkingDirectoryIsResolvedAtCallTime(t *testing.T) {
	first := t.TempDir()
	second := installSkillIn(t, t.TempDir())
	current := first
	runner := &fakeRunner{outcomes: []runOutcome{probeOK, {stdout: receiptLine(testSpec, 2) + "\n"}}}
	p := New(Options{
		Runner:     runner,
		WorkingDir: func() (string, error) { return current, nil },
		Now:        fixedElapsedClock(time.Second),
	})

	current = second
	if _, err := p.Execute(context.Background(), testRequest(fakeCommand(t))); err != nil {
		t.Fatal(err)
	}
	for i, call := range runner.snapshot() {
		if call.dir != second {
			t.Fatalf("call %d ran in %q, want the directory resolved at call time %q", i, call.dir, second)
		}
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
	req.ProviderConfig["model"] = "opus"

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
	if payload.Command != command || payload.Model != "opus" || payload.ExitCode != 0 {
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
	if !reflect.DeepEqual(calls[1].args, buildArgs(settings{Command: command, Model: "opus"}, buildPrompt(req))) {
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
		wantRuns int
	}{
		{
			name:     "runtime absent",
			command:  missingCommand(),
			skill:    true,
			wantErr:  []string{missingCommand(), "was not found"},
			wantRuns: 0,
		},
		{
			name:     "runtime present but impossible to start",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{{exitCode: -1, err: errors.New("spawn claude ENOENT")}},
			wantErr:  []string{"was found but could not be run", "spawn claude ENOENT"},
			wantRuns: 1,
		},
		{
			name:     "the version probe fails",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{{exitCode: 5, stderr: "claude: broken install"}},
			wantErr:  []string{"exited 5 instead of reporting its version", "claude: broken install"},
			wantRuns: 1,
		},
		{
			name:     "planning skill not installed",
			command:  command,
			skill:    false,
			outcomes: []runOutcome{probeOK},
			wantErr:  []string{"planning skill is not installed", planSkillRelPath, "archetipo init --tool claude"},
			wantRuns: 1,
		},
		{
			name:     "not authenticated",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {exitCode: 1, stderr: "Invalid API key · Please run /login"}},
			wantErr:  []string{"exited 1", "without planning " + testSpec, "Please run /login"},
			wantRuns: 2,
		},
		{
			name:     "non-zero exit code with an empty stderr",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {exitCode: 3, stdout: "I gave up."}},
			wantErr:  []string{"exited 3", "wrote nothing on standard error"},
			wantRuns: 2,
		},
		{
			name:     "no receipt line at all",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: "done, I planned everything\n"}},
			wantErr:  []string{"exited 0 without having produced a plan for " + testSpec, "did not emit the expected JSON receipt line"},
			wantRuns: 2,
		},
		{
			name:     "receipt for another spec",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: receiptLine("US-999", 4) + "\n"}},
			wantErr:  []string{"exited 0 without having produced a plan for " + testSpec, "does not declare a persisted plan for " + testSpec},
			wantRuns: 2,
		},
		{
			name:     "receipt declaring the wrong status",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: `{"spec_code":"` + testSpec + `","status":"TODO","tasks":4}` + "\n"}},
			wantErr:  []string{"does not declare a persisted plan for " + testSpec},
			wantRuns: 2,
		},
		{
			name:     "receipt declaring zero tasks",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: receiptLine(testSpec, 0) + "\n"}},
			wantErr:  []string{"does not declare a persisted plan for " + testSpec},
			wantRuns: 2,
		},
		{
			name:     "malformed receipt JSON",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {stdout: `{"spec_code":"` + testSpec + `","status":"PLANNED","tasks":` + "\n"}},
			wantErr:  []string{"did not emit the expected JSON receipt line"},
			wantRuns: 2,
		},
		{
			name:     "the request context ends while the agent is running",
			command:  command,
			skill:    true,
			outcomes: []runOutcome{probeOK, {waitForContext: true}},
			expired:  true,
			wantErr:  []string{"was stopped", "the request context ended", context.DeadlineExceeded.Error()},
			wantRuns: 2,
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
			if calls := runner.snapshot(); len(calls) != tc.wantRuns {
				t.Fatalf("runner calls = %d, want %d: %#v", len(calls), tc.wantRuns, calls)
			}
		})
	}
}

func TestExecuteRejectsAnInvalidConfigurationBeforeSpawning(t *testing.T) {
	runner := &fakeRunner{}
	req := testRequest("claude")
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

// The configured timeout is what bounds the run, so a short one must expire on
// its own without the caller's context having a deadline at all — and the
// diagnostic must name the timeout as the cause. The runner reports a killed
// process as a plain exit code, so a provider that classified this from the
// exit code alone would report "exited -1" and say nothing about why.
func TestExecuteReportsItsOwnTimeoutAsTheCause(t *testing.T) {
	runner := &fakeRunner{outcomes: []runOutcome{probeOK, {waitForContext: true}}}
	req := testRequest(fakeCommand(t))
	req.ProviderConfig["timeout_seconds"] = 1

	_, err := newTestProvider(workspaceWithSkill(t), runner, nil).Execute(context.Background(), req)
	if err == nil {
		t.Fatal("expected the configured timeout to end the run")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the timeout is not reported as a deadline: %v", err)
	}
	for _, want := range []string{"did not plan " + testSpec, "within its 1s timeout", "was stopped"} {
		assertContains(t, err.Error(), want, "execution error")
	}
	if strings.Contains(err.Error(), "exited -1") {
		t.Fatalf("the timeout was classified from the exit code instead of the deadline: %v", err)
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
			stdout: "reading ~/.claude/.credentials.json\ntoken=" + sentinel + "\n" + receiptLine(testSpec, 3) + "\n",
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
	var payload struct {
		ResultSummary string `json:"result_summary"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ResultSummary != `{"spec_code":"`+testSpec+`","status":"`+execution.PlannedStatus+`","tasks":3}` {
		t.Fatalf("result_summary is not the re-rendered receipt: %q", payload.ResultSummary)
	}
}

// A failure may quote the tail of stderr, which is what makes it diagnosable,
// but stdout — the stream where the agent does its talking — never travels.
func TestFailedExecutionDoesNotEchoTheAgentStdout(t *testing.T) {
	command := fakeCommand(t)
	runner := &fakeRunner{outcomes: []runOutcome{
		probeOK,
		{exitCode: 2, stdout: "token=" + sentinel + "\n", stderr: "claude: run aborted"},
	}}
	_, err := newTestProvider(workspaceWithSkill(t), runner, nil).Execute(context.Background(), testRequest(command))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("the diagnostic carried the agent stdout: %v", err)
	}
	assertContains(t, err.Error(), "claude: run aborted", "execution error")
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

// Cutting a stream at a byte offset would split a multi-byte rune in half; the
// diagnostic has to stay valid text whatever the agent wrote.
func TestTruncateCutsOnARuneBoundary(t *testing.T) {
	body := strings.Repeat("è", maxCapturedOutput)
	got := truncate(body)
	if !utf8.ValidString(got) {
		t.Fatalf("the truncated body is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("a truncated body does not say it was cut: %q", got)
	}
	if short := truncate("abc"); short != "abc" {
		t.Fatalf("a body within the limit was altered: %q", short)
	}
}

func TestDiagnosticSuffixNamesAnEmptyStream(t *testing.T) {
	if got := diagnosticSuffix("   \n"); !strings.Contains(got, "wrote nothing") {
		t.Fatalf("suffix = %q", got)
	}
}
