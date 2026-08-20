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

// runOutcome is one scripted answer of the fake Runner. It describes the
// availability probe and nothing else: the agent no longer runs one-shot, so
// there is no other invocation for the Runner to answer.
type runOutcome struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
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

func (r *fakeRunner) Run(_ context.Context, dir string, name string, args []string) (string, string, int, error) {
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
//
// It is guarded by a mutex because the clock is read from more than one
// goroutine: the run reads it to date its events while the caller reads it to
// measure the work, and in a conversation the two really do overlap.
func fixedElapsedClock(step time.Duration) func() time.Time {
	base := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	calls := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
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

// newSessionProvider builds a provider whose availability probe is scripted and
// whose session process is the fake Claude. The Runner now serves only the
// probe: the run itself goes through the Starter.
func newSessionProvider(dir string, runner Runner, fake *fakeClaude, now func() time.Time) *Provider {
	if now == nil {
		now = fixedElapsedClock(1500 * time.Millisecond)
	}
	return New(Options{
		Runner:     runner,
		Starter:    fake,
		WorkingDir: func() (string, error) { return dir, nil },
		Now:        now,
	})
}

// plannedSession drives the fake to the outcome of a successful planning: the
// agent works, then closes the turn with the receipt as the message the run
// ends on.
func plannedSession(fake *fakeClaude, specCode string, tasks int) {
	go func() {
		<-fake.started
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"pianifico"}]}}`)
		fake.emit(resultFrame(receiptLine(specCode, tasks), false))
	}()
}

// resultFrame renders the frame the process closes a turn with.
func resultFrame(text string, failed bool) string {
	subtype := "success"
	if failed {
		subtype = "error_during_execution"
	}
	payload, err := json.Marshal(map[string]any{
		"type":     "result",
		"subtype":  subtype,
		"is_error": failed,
		"result":   text,
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
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
	if !reflect.DeepEqual(capabilities, []execution.Capability{execution.CapabilitySpecPlan, execution.CapabilitySpecImplement, execution.CapabilitySpecReview, execution.CapabilityWorkspaceInception, execution.CapabilityWorkspaceBacklog}) {
		t.Fatalf("capabilities = %#v, want spec.plan, spec.implement, workspace.inception and workspace.backlog", capabilities)
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

// The invocation is the streaming one, and it is asserted literally because it
// is the single line that decides whether the process can hold a conversation
// at all: without `--input-format stream-json` no message can reach a live
// turn, and without `--replay-user-messages` a message that did reach it would
// never come back out to enter the history.
//
// The prompt is deliberately absent: a live session is opened before it is told
// what to do, so the instruction travels inside the protocol as the first user
// frame and not as an argument.
func TestBuildArgsStartsTheStreamingSession(t *testing.T) {
	cases := []struct {
		name string
		cfg  settings
		want []string
	}{
		{
			name: "the configured permission mode",
			cfg:  settings{Command: "claude", PermissionMode: "auto"},
			want: []string{
				"--print",
				"--input-format", "stream-json",
				"--output-format", "stream-json",
				"--verbose",
				"--replay-user-messages",
				"--no-session-persistence",
				"--permission-mode", "auto",
			},
		},
		{
			name: "with a model",
			cfg:  settings{Command: "claude", Model: "opus", PermissionMode: "bypassPermissions"},
			want: []string{
				"--print",
				"--input-format", "stream-json",
				"--output-format", "stream-json",
				"--verbose",
				"--replay-user-messages",
				"--no-session-persistence",
				"--permission-mode", "bypassPermissions",
				"--model", "opus",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildArgs(tc.cfg)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %#v, want %#v", got, tc.want)
			}
			for _, forbidden := range []string{buildPrompt(testRequest("claude")), testSpec} {
				for _, arg := range got {
					if strings.Contains(arg, forbidden) {
						t.Fatalf("the prompt travelled on the command line: %#v", got)
					}
				}
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
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	plannedSession(fake, testSpec, 2)
	p := New(Options{
		Runner:     runner,
		Starter:    fake,
		WorkingDir: func() (string, error) { return current, nil },
		Now:        fixedElapsedClock(time.Second),
	})

	current = second
	if _, err := p.Execute(context.Background(), testRequest(fakeCommand(t))); err != nil {
		t.Fatal(err)
	}
	for i, call := range runner.snapshot() {
		if call.dir != second {
			t.Fatalf("probe %d ran in %q, want the directory resolved at call time %q", i, call.dir, second)
		}
	}
	if dir := fake.startedIn(); dir != second {
		t.Fatalf("the session ran in %q, want the directory resolved at call time %q", dir, second)
	}
}

// --- Execute: success ------------------------------------------------------

func TestExecuteReturnsAPayloadBuiltFromTheReceipt(t *testing.T) {
	command := fakeCommand(t)
	dir := workspaceWithSkill(t)
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	plannedSession(fake, testSpec, 9)
	req := testRequest(command)
	req.ProviderConfig["model"] = "opus"

	provider := newSessionProvider(dir, runner, fake, fixedElapsedClock(1500*time.Millisecond))
	got, err := provider.Execute(context.Background(), req)
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

	// The probe is the only thing that still runs one-shot, and it runs where
	// the session runs.
	calls := runner.snapshot()
	if len(calls) != 1 || !reflect.DeepEqual(calls[0].args, []string{"--version"}) {
		t.Fatalf("runner calls = %#v, want only the availability probe", calls)
	}
	if calls[0].dir != dir || calls[0].name != command {
		t.Fatalf("the probe ran %q in %q", calls[0].name, calls[0].dir)
	}
	starts, name, args := fake.spawned()
	if starts != 1 || name != command {
		t.Fatalf("the session was started %d time(s) as %q", starts, name)
	}
	if !reflect.DeepEqual(args, buildArgs(settings{Command: command, Model: "opus", PermissionMode: defaultPermissionMode})) {
		t.Fatalf("session args = %#v", args)
	}
	if fake.startedIn() != dir {
		t.Fatalf("the session ran in %q, want the seam working directory %q", fake.startedIn(), dir)
	}
	// The instruction reached the agent inside the protocol, as the first user
	// frame of the session.
	if got := fake.messagesReceived(); len(got) != 1 || got[0] != buildPrompt(req) {
		t.Fatalf("the process received %v; want exactly the prompt", got)
	}
}

// AC-1, AC-2 — the run is registered and readable while the agent is still
// working, which is what makes it followable at all.
func TestExecuteRegistersTheRunBeforeTheAgentWorks(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	provider := newSessionProvider(workspaceWithSkill(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	done := make(chan error, 1)
	go func() {
		_, err := provider.Execute(context.Background(), testRequest(command))
		done <- err
	}()

	<-fake.started
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"sto lavorando"}]}}`)

	// Everything below happens while Execute is still inside the agent's work.
	var snapshot execution.RunSnapshot
	waitFor(t, func() bool {
		runID, err := provider.ResolveRun(context.Background(), execution.Execution{ID: "exec-1"}, nil)
		if err != nil || runID != "exec-1" {
			return false
		}
		snapshot, err = provider.ReadRun(context.Background(), execution.RunRequest{RunID: runID})
		if err != nil {
			return false
		}
		return historyCarries(provider, runID, "sto lavorando")
	})
	if snapshot.State != execution.RunActive {
		t.Fatalf("the run reported %q while the agent was still working", snapshot.State)
	}

	fake.emit(resultFrame(receiptLine(testSpec, 2), false))
	if err := <-done; err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Once it is over, the run is still readable and its state is the observed
	// one.
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: "exec-1"})
	if err != nil {
		t.Fatalf("ReadRun failed after the run: %v", err)
	}
	if snapshot.State != execution.RunClosed {
		t.Fatalf("state = %q, want the observed closed state", snapshot.State)
	}
}

// historyCarries replays the run's history and reports whether it already holds
// an event carrying that text. The bounded context is what makes the replay
// return once the history is exhausted, so the caller polls a condition instead
// of waiting on a stream that stays open for the whole run.
func historyCarries(provider *Provider, runID, text string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	found := errors.New("found")
	err := provider.StreamRunEvents(ctx, execution.RunRequest{RunID: runID}, 0, func(event execution.RunEvent) error {
		if event.Text == text {
			return found
		}
		return nil
	})
	return errors.Is(err, found)
}

// AC-1 — the dialogue capability is derived from the interface the provider
// implements and never declared by hand: a provider that advertised a
// conversation it cannot hold is exactly what derivation makes impossible.
func TestProviderDeclaresTheDialogueThroughTheInterface(t *testing.T) {
	provider := New(Options{})
	collaborator, ok := execution.RunCollaboratorFor(provider)
	if !ok || collaborator == nil {
		t.Fatal("the provider does not expose an interactive run")
	}
	declared, err := provider.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(declared, []execution.Capability{execution.CapabilitySpecPlan, execution.CapabilitySpecImplement, execution.CapabilitySpecReview, execution.CapabilityWorkspaceInception, execution.CapabilityWorkspaceBacklog}) {
		t.Fatalf("Capabilities = %#v, want the five dispatched actions: run.dialog is derived, not declared", declared)
	}
	got, err := execution.DeclaredCapabilities(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	want := execution.NormalizeCapabilities([]execution.Capability{execution.CapabilitySpecPlan, execution.CapabilitySpecImplement, execution.CapabilitySpecReview, execution.CapabilityWorkspaceInception, execution.CapabilityWorkspaceBacklog, execution.CapabilityRunDialog})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeclaredCapabilities = %#v, want %#v", got, want)
	}
}

// --- Execute: failures -----------------------------------------------------

func TestExecuteFailureModes(t *testing.T) {
	command := fakeCommand(t)
	cases := []struct {
		name      string
		command   string
		skill     bool
		probe     []runOutcome
		drive     func(fake *fakeClaude)
		expired   bool
		wantErr   []string
		wantStart int
	}{
		{
			name:    "runtime absent",
			command: missingCommand(),
			skill:   true,
			wantErr: []string{missingCommand(), "was not found"},
		},
		{
			name:    "runtime present but impossible to start",
			command: command,
			skill:   true,
			probe:   []runOutcome{{exitCode: -1, err: errors.New("spawn claude ENOENT")}},
			wantErr: []string{"was found but could not be run", "spawn claude ENOENT"},
		},
		{
			name:    "the version probe fails",
			command: command,
			skill:   true,
			probe:   []runOutcome{{exitCode: 5, stderr: "claude: broken install"}},
			wantErr: []string{"exited 5 instead of reporting its version", "claude: broken install"},
		},
		{
			name:    "planning skill not installed",
			command: command,
			skill:   false,
			probe:   []runOutcome{probeOK},
			wantErr: []string{"planning skill is not installed", planSkillRelPath, "archetipo init --tool claude"},
		},
		{
			name:    "the process dies before announcing itself",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				fake.silent = true
				go func() {
					waitForFrames(fake, 1)
					fake.end()
				}()
			},
			wantErr:   []string{"ended before announcing itself"},
			wantStart: 1,
		},
		{
			name:    "the process dies before the turn ends",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.exitCode = 1
					fake.stderr = "Invalid API key · Please run /login"
					fake.end()
				}()
			},
			wantErr:   []string{"exited 1", "without planning " + testSpec, "Please run /login"},
			wantStart: 1,
		},
		{
			name:    "a non-zero exit with an empty stderr",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.exitCode = 3
					fake.end()
				}()
			},
			wantErr:   []string{"exited 3", "wrote nothing on standard error"},
			wantStart: 1,
		},
		{
			name:    "no receipt at all",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame("done, I planned everything", false))
				}()
			},
			wantErr:   []string{"ended without having produced a plan for " + testSpec, "did not emit the expected JSON receipt line"},
			wantStart: 1,
		},
		{
			name:      "receipt for another spec",
			command:   command,
			skill:     true,
			probe:     []runOutcome{probeOK},
			drive:     func(fake *fakeClaude) { plannedSession(fake, "US-999", 4) },
			wantErr:   []string{"ended without having produced a plan for " + testSpec, "does not declare a persisted plan for " + testSpec},
			wantStart: 1,
		},
		{
			// An interrupted turn is the one case where the process can close
			// the turn with an error and still exit 0, because stopping it
			// afterwards is an ordinary shutdown. Accepting it would take
			// whatever the agent last said as a plan for work the operator had
			// just cancelled, so the turn — not the exit code — is what decides.
			name:    "an interrupted turn that exits 0 carries no plan",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(receiptLine(testSpec, 4), true))
				}()
			},
			wantErr:   []string{"exited 0", "without planning " + testSpec, "the turn never completed"},
			wantStart: 1,
		},
		{
			name:    "receipt declaring the wrong status",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(`{"spec_code":"`+testSpec+`","status":"TODO","tasks":4}`, false))
				}()
			},
			wantErr:   []string{"does not declare a persisted plan for " + testSpec},
			wantStart: 1,
		},
		{
			name:      "receipt declaring zero tasks",
			command:   command,
			skill:     true,
			probe:     []runOutcome{probeOK},
			drive:     func(fake *fakeClaude) { plannedSession(fake, testSpec, 0) },
			wantErr:   []string{"does not declare a persisted plan for " + testSpec},
			wantStart: 1,
		},
		{
			name:    "malformed receipt JSON",
			command: command,
			skill:   true,
			probe:   []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(`{"spec_code":"`+testSpec+`","status":"PLANNED","tasks":`, false))
				}()
			},
			wantErr:   []string{"did not emit the expected JSON receipt line"},
			wantStart: 1,
		},
		{
			name:      "a turn that never ends",
			command:   command,
			skill:     true,
			probe:     []runOutcome{probeOK},
			expired:   true,
			wantErr:   []string{"did not finish planning " + testSpec, "within 1s"},
			wantStart: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.skill {
				dir = workspaceWithSkill(t)
			}
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			if tc.drive != nil {
				tc.drive(fake)
			}
			runner := &fakeRunner{outcomes: tc.probe}
			req := testRequest(tc.command)
			if tc.expired {
				req.ProviderConfig["timeout_seconds"] = 1
			}
			got, err := newSessionProvider(dir, runner, fake, nil).Execute(context.Background(), req)
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
			if starts, _, _ := fake.spawned(); starts != tc.wantStart {
				t.Fatalf("the session was started %d time(s), want %d", starts, tc.wantStart)
			}
		})
	}
}

// waitForFrames blocks until the process has been written n frames, so a test
// can act on what the client really sent instead of on a delay.
func waitForFrames(fake *fakeClaude, n int) {
	for len(fake.framesReceived()) < n {
		time.Sleep(time.Millisecond)
	}
}

func TestExecuteRejectsAnInvalidConfigurationBeforeSpawning(t *testing.T) {
	runner := &fakeRunner{}
	fake := newFakeClaude()
	req := testRequest("claude")
	req.ProviderConfig["timeout_seconds"] = 0

	_, err := newSessionProvider(workspaceWithSkill(t), runner, fake, nil).Execute(context.Background(), req)
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "timeout_seconds" {
		t.Fatalf("error = %v", err)
	}
	if calls := runner.snapshot(); len(calls) != 0 {
		t.Fatalf("an invalid configuration spawned %d command(s)", len(calls))
	}
	if starts, _, _ := fake.spawned(); starts != 0 {
		t.Fatalf("an invalid configuration opened a session")
	}
}

// The configured timeout is what bounds the run, so a short one must expire on
// its own without the caller's context having a deadline at all — and the
// diagnostic must name the timeout as the cause rather than whatever the
// process happened to exit with. The run is closed as observed, not as
// succeeded.
func TestExecuteReportsItsOwnTimeoutAsTheCause(t *testing.T) {
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	req := testRequest(fakeCommand(t))
	req.ProviderConfig["timeout_seconds"] = 1

	provider := newSessionProvider(workspaceWithSkill(t), runner, fake, nil)
	_, err := provider.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("expected the configured timeout to end the run")
	}
	for _, want := range []string{"did not finish planning " + testSpec, "within 1s"} {
		assertContains(t, err.Error(), want, "execution error")
	}
	snapshot, readErr := provider.ReadRun(context.Background(), execution.RunRequest{RunID: "exec-1"})
	if readErr != nil {
		t.Fatalf("ReadRun failed after the timeout: %v", readErr)
	}
	if snapshot.State != execution.RunCrashed {
		t.Fatalf("state = %q, want the run reported as crashed", snapshot.State)
	}
	if !strings.Contains(snapshot.Error, "was stopped") {
		t.Fatalf("the run does not say why it ended: %q", snapshot.Error)
	}
}

// --- no secret ever reaches the record -------------------------------------

// The provider cannot recognize a secret, so the guarantee it offers is that no
// captured stream is persisted at all. The sentinel stands in for whatever the
// agent may have printed.
func TestSuccessfulExecutionNeverPersistsTheAgentOutput(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	fake.stderr = "warning: " + sentinel
	go func() {
		<-fake.started
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"reading ~/.claude/.credentials.json, token=` + sentinel + `"}]}}`)
		fake.emit(resultFrame(receiptLine(testSpec, 3), false))
	}()

	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	got, err := newSessionProvider(workspaceWithSkill(t), runner, fake, nil).Execute(context.Background(), testRequest(command))
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
// but what the agent said never travels into the diagnostic.
func TestFailedExecutionDoesNotEchoWhatTheAgentSaid(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	fake.stderr = "claude: run aborted"
	go func() {
		<-fake.started
		fake.emit(resultFrame("token="+sentinel, false))
	}()

	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	_, err := newSessionProvider(workspaceWithSkill(t), runner, fake, nil).Execute(context.Background(), testRequest(command))
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
	fake := newFakeClaude()
	fake.stderr = strings.Repeat("x", maxCapturedOutput*3) + sentinel
	go func() {
		<-fake.started
		fake.emit(resultFrame("", false))
	}()

	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	_, err := newSessionProvider(workspaceWithSkill(t), runner, fake, nil).Execute(context.Background(), testRequest(command))
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
