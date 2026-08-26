package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/claude"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// claudeAuthSentinel stands for whatever authentication material a real Claude
// Code process can print — a token echoed by a debug flag, a credentials path,
// a login banner. It is exported into the environment and deliberately printed
// by the fake runtime so the assertions can prove it is dropped rather than
// merely absent by luck.
const claudeAuthSentinel = "claude-oauth-token-DO-NOT-PERSIST"

// claudeScript drives the fake Claude runtime for one scenario. It stands in
// for both seams the provider offers — the one-shot runner behind the
// availability probe and the live process behind the session — so every test
// here exercises the real handshake, the real translation and the real receipt
// gate without a `claude` binary existing anywhere on the machine.
//
// The two are kept apart on purpose: `--version` is the availability probe, the
// session is the agent run. Scripting them separately is what lets a test say
// "the runtime is not usable" without also saying anything about the run that
// would have followed.
type claudeScript struct {
	mu sync.Mutex

	// versionErr and versionExit describe the availability probe. A non-nil
	// versionErr is a process that never started; a non-zero versionExit is a
	// binary that answered but not with its version.
	versionErr  error
	versionExit int
	versionOut  string

	// exec describes the agent run: the message the run ends on — which is where
	// the receipt is expected — the tail of the process's standard error, its
	// exit code, and a failure to run it at all. It may reach back into the CLI
	// to persist a plan, exactly as the local agent does.
	//
	// A non-zero exit code or a non-nil error describes a process that died
	// without closing its turn, which is how a real failed session ends.
	exec func() (finalMessage string, stderr string, exitCode int, err error)

	versionCalls int
	execCalls    int
	execArgs     []string
	execDir      string
	prompt       string
}

var (
	_ claude.Runner    = (*claudeScript)(nil)
	_ localrun.Starter = (*claudeScript)(nil)
)

// Start opens the fake session. The provider drives it with the real
// stream-json client, so what is asserted downstream is the production
// handshake and not a shortcut.
func (s *claudeScript) Start(_ context.Context, dir string, _ string, args []string) (localrun.Process, error) {
	s.mu.Lock()
	s.execCalls++
	s.execArgs = append([]string(nil), args...)
	s.execDir = dir
	exec := s.exec
	s.mu.Unlock()
	return newClaudeSessionProcess(s, exec), nil
}

// claudeSessionProcess speaks the stream-json protocol and nothing else: NDJSON
// in, NDJSON out, one frame per line.
type claudeSessionProcess struct {
	script *claudeScript
	exec   func() (string, string, int, error)

	mu        sync.Mutex
	ended     bool
	announced bool
	lines     chan []byte
	once      sync.Once
	done      chan struct{}
	stderr    string
	exitCode  int
	failure   error
}

func newClaudeSessionProcess(script *claudeScript, exec func() (string, string, int, error)) *claudeSessionProcess {
	return &claudeSessionProcess{
		script: script,
		exec:   exec,
		lines:  make(chan []byte, 64),
		done:   make(chan struct{}),
	}
}

// Send reads one frame off the process's standard input. The first user frame
// carries the instruction and opens the turn, exactly as the real build treats
// it.
func (p *claudeSessionProcess) Send(line []byte) error {
	var frame struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &frame); err != nil {
		return err
	}
	if frame.Type != "user" {
		return nil
	}
	p.mu.Lock()
	first := !p.announced
	p.announced = true
	p.mu.Unlock()
	if !first {
		return nil
	}
	p.script.recordPrompt(line)
	p.push([]byte(`{"type":"system","subtype":"init","model":"opus"}`))
	go p.work()
	return nil
}

// work is the agent's turn: it does whatever the scenario scripted, leaves the
// message the run ends on, and either closes the turn with a result frame or
// dies the way a failing process dies.
func (p *claudeSessionProcess) work() {
	finalMessage, stderr, exitCode, failure := "", "", 0, error(nil)
	if p.exec != nil {
		finalMessage, stderr, exitCode, failure = p.exec()
	}
	p.mu.Lock()
	p.stderr, p.exitCode, p.failure = stderr, exitCode, failure
	p.mu.Unlock()

	if strings.TrimSpace(finalMessage) != "" {
		payload, err := json.Marshal(map[string]any{
			"type":    "assistant",
			"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": finalMessage}}},
		})
		if err == nil {
			p.push(payload)
		}
	}
	if exitCode != 0 || failure != nil {
		p.end()
		return
	}
	result, err := json.Marshal(map[string]any{
		"type":     "result",
		"subtype":  "success",
		"is_error": false,
		"result":   finalMessage,
	})
	if err == nil {
		p.push(result)
	}
	// The process leaves once its turn is over, which is what makes these
	// scenarios terminal. A run is a conversation now: a turn that ends on
	// something the receipt gate refuses is the agent waiting for an answer, and
	// `execution run` is a batch command with nobody to give one — so a fixture
	// that stayed alive would describe a run bounded only by its timeout instead
	// of the failure each of these cases is about.
	p.end()
}

func (p *claudeSessionProcess) push(payload []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ended {
		return
	}
	p.lines <- payload
}

func (p *claudeSessionProcess) end() {
	p.once.Do(func() {
		p.mu.Lock()
		p.ended = true
		p.mu.Unlock()
		close(p.lines)
		close(p.done)
	})
}

func (p *claudeSessionProcess) Lines() <-chan []byte { return p.lines }

func (p *claudeSessionProcess) Signal() error { p.end(); return nil }

func (p *claudeSessionProcess) Wait() (int, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode, p.stderr, p.failure
}

func (p *claudeSessionProcess) Close() error { p.end(); return nil }

func (s *claudeScript) Run(_ context.Context, dir string, _ string, args []string) (string, string, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(args) == 1 && args[0] == "--version" {
		s.versionCalls++
		if s.versionErr != nil {
			return "", "", -1, s.versionErr
		}
		if s.versionExit != 0 {
			return "", "claude: spawn claude ENOENT", s.versionExit, nil
		}
		out := s.versionOut
		if out == "" {
			out = "0.0.0-test (Claude Code)\n"
		}
		return out, "", 0, nil
	}
	// Everything that is not the probe would be a one-shot agent run, and there
	// is no such thing any more: the agent runs inside a session.
	return "", "", 0, fmt.Errorf("unexpected one-shot invocation: %v", args)
}

// recordPrompt keeps the text the provider really asked the agent to work on,
// which now travels inside the first user frame instead of on the command line.
func (s *claudeScript) recordPrompt(line []byte) {
	var frame struct {
		Message struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &frame) != nil || len(frame.Message.Content) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompt = frame.Message.Content[0].Text
}

func (s *claudeScript) promptSent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prompt
}

func (s *claudeScript) snapshot() (versionCalls, execCalls int, args []string, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.versionCalls, s.execCalls, append([]string(nil), s.execArgs...), s.execDir
}

// claudePlanReceipt renders the receipt line the agent is asked to close its
// run with. It is built from the shared constant, not from a literal, so a test
// can never accept a word the production gate would reject.
func claudePlanReceipt(specCode string, tasks int) string {
	return fmt.Sprintf(`{"spec_code":%q,"status":%q,"tasks":%d}`, specCode, execution.PlannedStatus, tasks)
}

// claudeNoisyOutput wraps a payload in the chatter a real agent prints around
// its receipt, including the authentication sentinel. Nothing here may reach
// the persisted record.
func claudeNoisyOutput(payload string) string {
	return strings.Join([]string{
		"[claude] reading credentials " + os.Getenv("CLAUDE_TEST_AUTH"),
		"[claude] working...",
		payload,
	}, "\n") + "\n"
}

// fakeClaudeBinary creates an executable file that satisfies exec.LookPath and
// nothing else. The provider looks the command up before probing it, so the
// path has to exist; the file is never actually run, because claudeScript stands
// in for every invocation.
func fakeClaudeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude-fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'this fake must never be executed' >&2\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// installClaudePlanSkill writes the marker `archetipo init --tool claude` leaves
// behind. The provider refuses to spawn without it, so a scenario that omitted
// it would be testing the guard instead of the run.
func installClaudePlanSkill(t *testing.T) {
	t.Helper()
	dir := filepath.Join(".claude", "skills", "archetipo-plan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: archetipo-plan\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newClaudeScenario wires a temporary workspace with a seeded spec, the real
// claude provider over the scripted runtime, and the workspace default provider
// — all of it through archetipo commands, so what the tests drive is the same
// path a person drives.
func newClaudeScenario(t *testing.T, script *claudeScript) executionDependencies {
	t.Helper()
	t.Setenv("CLAUDE_TEST_AUTH", claudeAuthSentinel)
	t.Chdir(t.TempDir())
	deps := executionTestDeps(t)
	if err := deps.registry.Register(claude.New(claude.Options{Runner: script, Starter: script})); err != nil {
		t.Fatal(err)
	}
	seedExecutionSpec(t, deps)
	installClaudePlanSkill(t)
	cfgPath := writeExecutionProviderPayload(t, "claude.json", fmt.Sprintf(
		`{"command":%q,"model":"opus","timeout_seconds":60}`, fakeClaudeBinary(t)))
	selection := decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "set-default", "claude", "--file", cfgPath))
	if selection.ID != claude.ProviderID {
		t.Fatalf("the workspace default is %q", selection.ID)
	}
	return deps
}

// readClaudeExecutionRecord reads the file the store wrote, not the envelope the
// command printed. AC-3 is about what survives on disk, so the oracle has to be
// the disk.
func readClaudeExecutionRecord(t *testing.T, id string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(".archetipo", "executions", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func claudeExecutionID(requestID string) string {
	return execution.DeriveID("US-001", execution.ActionPlan, claude.ProviderID, requestID)
}

// assertClaudeSpecUntouched is the AC-4 invariant: whatever went wrong, the spec
// is still where it was and holds no plan.
func assertClaudeSpecUntouched(t *testing.T, deps executionDependencies, before executionSpecState) {
	t.Helper()
	show := runExecutionRoot(t, deps, "spec", "show", "US-001")
	after := decodeExecutionSpecState(t, show)
	if after.Status != domain.StatusTodo || !reflect.DeepEqual(before, after) {
		t.Fatalf("the spec changed across a failed claude run: before=%#v after=%#v", before, after)
	}
	if tasks := decodeExecutionSpecTasks(t, show); tasks != 0 {
		t.Fatalf("a failed claude run left %d plan tasks behind", tasks)
	}
}

// AC-2: a successful local run really plans the spec. The oracle is the
// connector re-read after the fact, not the receipt the agent declared.
func TestClaudePlanLocalHappyPath(t *testing.T) {
	script := &claudeScript{}
	deps := newClaudeScenario(t, script)
	script.exec = func() (string, string, int, error) {
		// The local agent plans the spec through the configured connector,
		// exactly as the prompt instructs it to.
		planExecutionSpec(t, deps)
		return claudeNoisyOutput(claudePlanReceipt("US-001", 1)), "", 0, nil
	}

	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if before.Status != domain.StatusTodo {
		t.Fatalf("the spec did not start in TODO: %#v", before)
	}

	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	run := decodeExecution(t, result)
	if run.Status != execution.StatusSucceeded || run.SpecCode != "US-001" || run.ProviderID != claude.ProviderID || run.RequestID != "r1" {
		t.Fatalf("run = %#v", run)
	}
	if run.Result == nil {
		t.Fatalf("a succeeded run carries no result: %#v", run)
	}

	// The invocation is the real one: probed once, then a session opened once,
	// in the workspace, with the prompt naming the spec.
	versionCalls, execCalls, args, dir := script.snapshot()
	if versionCalls != 1 || execCalls != 1 {
		t.Fatalf("version calls = %d, exec calls = %d", versionCalls, execCalls)
	}
	// The invocation is the streaming one, which is what makes the run a
	// conversation instead of a one-shot command; the prompt no longer travels
	// on the command line but inside the protocol, so it is asserted on the
	// frame the provider wrote.
	joined := strings.Join(args, " ")
	for _, want := range []string{"--print", "--input-format stream-json", "--output-format stream-json", "--replay-user-messages"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the claude invocation does not open a streaming session (%q missing): %#v", want, args)
		}
	}
	if prompt := script.promptSent(); !strings.Contains(prompt, "US-001") || !strings.Contains(prompt, "/archetipo-plan") {
		t.Fatalf("the prompt does not ask to plan US-001: %q", prompt)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(dir); err != nil || resolved != mustEvalSymlinks(t, cwd) {
		t.Fatalf("the agent ran in %q instead of the workspace %q (err=%v)", dir, cwd, err)
	}

	// AC-2: the plan is read back from the connector.
	show := runExecutionRoot(t, deps, "spec", "show", "US-001")
	if state := decodeExecutionSpecState(t, show); state.Status != domain.StatusPlanned {
		t.Fatalf("the spec status after the local run = %s", state.Status)
	}
	if tasks := decodeExecutionSpecTasks(t, show); tasks < 1 {
		t.Fatalf("the connector holds no plan tasks: %d", tasks)
	}

	var payload struct {
		Command   string `json:"command"`
		Model     string `json:"model"`
		ExitCode  int    `json:"exit_code"`
		PlanTasks int    `json:"plan_tasks"`
	}
	decodeExecutionPayload(t, run.Result.Payload, &payload)
	if payload.PlanTasks != 1 {
		t.Fatalf("the receipt was not read: plan_tasks = %d", payload.PlanTasks)
	}
	if payload.Model != "opus" || payload.ExitCode != 0 || payload.Command == "" {
		t.Fatalf("payload = %#v", payload)
	}
}

// AC-3: the record keeps provider, status and outcome, and nothing that came
// out of the agent's own streams. A run that prints its session material must
// leave no trace of it on disk or on the command's streams.
func TestClaudeSuccessfulRunPersistsNoAuthenticationMaterial(t *testing.T) {
	script := &claudeScript{}
	deps := newClaudeScenario(t, script)
	script.exec = func() (string, string, int, error) {
		planExecutionSpec(t, deps)
		return claudeNoisyOutput(claudePlanReceipt("US-001", 1)),
			"[claude] refreshed credentials for " + claudeAuthSentinel, 0, nil
	}

	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	run := decodeExecution(t, result)
	if run.Status != execution.StatusSucceeded {
		t.Fatalf("run = %#v", run)
	}
	// The sentinel really was printed by the runtime, otherwise the assertions
	// below would pass on a scenario that never risked anything.
	if _, execCalls, _, _ := script.snapshot(); execCalls != 1 {
		t.Fatalf("exec calls = %d", execCalls)
	}
	if os.Getenv("CLAUDE_TEST_AUTH") != claudeAuthSentinel {
		t.Fatal("the scenario did not export the sentinel")
	}

	for name, stream := range map[string]string{"stdout": result.stdout.String(), "stderr": result.stderr.String()} {
		if strings.Contains(stream, claudeAuthSentinel) {
			t.Fatalf("the CLI %s leaked the claude session material", name)
		}
	}
	record := readClaudeExecutionRecord(t, run.ID)
	if strings.Contains(record, claudeAuthSentinel) {
		t.Fatalf("the persisted record leaked the claude session material:\n%s", record)
	}
	// The record is still a useful trace: provider, status and receipt survive.
	for _, want := range []string{claude.ProviderID, string(execution.StatusSucceeded), string(execution.PlannedStatus)} {
		if !strings.Contains(record, want) {
			t.Fatalf("the record does not carry %q:\n%s", want, record)
		}
	}
}

// AC-4: every way a local run can go wrong leaves the spec exactly where it
// was, with an explicit reason on the record.
//
// Note on the exit code: a provider failure is a recorded outcome, not a broken
// command, so `execution run` exits 0 and prints the FAILED record — the same
// contract the other providers are held to. The unconfirmed-effect case below
// is the one that fails the command, because there the CLI itself refuses a
// success the connector denies.
func TestClaudeLocalFailuresPreserveTheSpec(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func(*claudeScript)
		// wantText is matched on the diagnostic the record carries.
		wantText []string
		wantExec int
	}{
		{
			name:     "the runtime is not usable",
			arrange:  func(s *claudeScript) { s.versionExit = 127 },
			wantText: []string{"instead of reporting its version", "spawn claude ENOENT"},
			wantExec: 0,
		},
		{
			name: "claude is not authenticated",
			arrange: func(s *claudeScript) {
				s.exec = func() (string, string, int, error) {
					return "", "Invalid API key · Please run /login", 1, nil
				}
			},
			wantText: []string{"exited 1", "without planning US-001", "Please run /login"},
			wantExec: 1,
		},
		{
			name: "the run ends without a receipt",
			arrange: func(s *claudeScript) {
				s.exec = func() (string, string, int, error) {
					return claudeNoisyOutput("All done, I planned everything."), "", 0, nil
				}
			},
			wantText: []string{"ended without having produced a plan for US-001"},
			wantExec: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := &claudeScript{}
			tc.arrange(script)
			deps := newClaudeScenario(t, script)
			before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))

			result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
			if result.exit != 0 {
				t.Fatalf("a recorded provider failure must not fail the command: exit=%d stderr=%s", result.exit, result.stderr.String())
			}
			run := decodeExecution(t, result)
			if run.Status != execution.StatusFailed || run.Result != nil || run.Error == nil || run.Error.Code != "PROVIDER_ERROR" {
				t.Fatalf("run = %#v", run)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(run.Error.Message, want) {
					t.Fatalf("the diagnostic misses %q: %s", want, run.Error.Message)
				}
			}
			if _, execCalls, _, _ := script.snapshot(); execCalls != tc.wantExec {
				t.Fatalf("exec calls = %d, want %d", execCalls, tc.wantExec)
			}
			if strings.Contains(run.Error.Message, claudeAuthSentinel) {
				t.Fatalf("the recorded diagnostic leaked the session material: %s", run.Error.Message)
			}

			// The record is the trace the person goes back to, so the reason has
			// to be on it and not only on the stream that scrolled past.
			shown := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", claudeExecutionID("r1")))
			if shown.Status != execution.StatusFailed || shown.Error == nil || !reflect.DeepEqual(shown.Error, run.Error) {
				t.Fatalf("the persisted record = %#v", shown)
			}
			assertClaudeSpecUntouched(t, deps, before)
		})
	}
}

// AC-4, the case a receipt alone cannot catch: the agent declares a plan it
// never persisted. The provider holds no connector by design, so the refusal
// comes from the shared effect check, and this is the one failure that fails
// the command.
func TestClaudeValidReceiptWithoutAPlanIsNotASuccess(t *testing.T) {
	script := &claudeScript{exec: func() (string, string, int, error) {
		return claudeNoisyOutput(claudePlanReceipt("US-001", 3)), "", 0, nil
	}}
	deps := newClaudeScenario(t, script)
	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))

	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing {
		t.Fatalf("a receipt the connector denies did not fail the command: exit=%d code=%s text=%q", exit, code, text)
	}
	for _, want := range []string{"US-001", string(domain.StatusTodo), string(domain.StatusPlanned)} {
		if !strings.Contains(text, want) {
			t.Fatalf("the message misses %q: %q", want, text)
		}
	}

	record := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", claudeExecutionID("r1")))
	if record.Status != execution.StatusFailed || record.Result != nil || record.Error == nil {
		t.Fatalf("record = %#v", record)
	}
	if record.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("record error = %#v", record.Error)
	}
	if !strings.Contains(record.Error.Message, "US-001") || !strings.Contains(record.Error.Message, string(domain.StatusTodo)) {
		t.Fatalf("the record does not report the reason: %s", record.Error.Message)
	}
	assertClaudeSpecUntouched(t, deps, before)
}

// AC-1: the provider the shipped binary registers is this one. A registry that
// only knows arcipelago and codex would answer "register the requested provider
// before selecting it"; one that knows claude answers with claude's own
// configuration vocabulary — and selecting it must not disturb the provider a
// workspace had already chosen until the new one is accepted.
func TestClaudeProviderIsRegisteredInRealRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	deps := executionTestDeps(t)
	seedExecutionSpec(t, deps)

	// A workspace that already runs on another provider is the realistic
	// starting point, and the failed selection below must leave it in place.
	previous := writeExecutionProviderPayload(t, "codex-good.json", `{"model":"gpt-5-codex"}`)
	if selection := decodeExecutionProvider(t, runRealRoot(t, "execution", "provider", "set-default", "codex", "--file", previous)); selection.ID != "codex" {
		t.Fatalf("the previous default was not set: %#v", selection)
	}

	rejected := writeExecutionProviderPayload(t, "claude-bad.json", `{"nonsense":true}`)
	result := runRealRoot(t, "execution", "provider", "set-default", claude.ProviderID, "--file", rejected)
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput {
		t.Fatalf("exit=%d code=%s text=%q", exit, code, text)
	}
	if strings.Contains(text, "register the requested provider") {
		t.Fatalf("claude is not registered in the real root: %q", text)
	}
	if !strings.Contains(text, "nonsense") {
		t.Fatalf("the message does not name the offending key, so claude's own validation did not run: %q", text)
	}
	if kept := decodeExecutionProvider(t, runRealRoot(t, "execution", "provider", "show-default")); kept.ID != "codex" {
		t.Fatalf("a rejected selection replaced the previous default: %#v", kept)
	}

	// And it is selectable: a valid configuration is accepted and saved, without
	// the machine needing Claude Code installed.
	accepted := writeExecutionProviderPayload(t, "claude-good.json", `{"model":"opus","timeout_seconds":120}`)
	selection := decodeExecutionProvider(t, runRealRoot(t, "execution", "provider", "set-default", claude.ProviderID, "--file", accepted))
	if selection.ID != claude.ProviderID {
		t.Fatalf("selection = %#v", selection)
	}
	shown := decodeExecutionProvider(t, runRealRoot(t, "execution", "provider", "show-default"))
	if shown.ID != claude.ProviderID || shown.Config["model"] != "opus" {
		t.Fatalf("show-default = %#v", shown)
	}
}

// AC-5: the local provider is an addition, not a prerequisite. A workspace
// initialized for Claude plans through the skill with no provider configured at
// all — which is what people who drive ARchetipo from inside Claude Code do
// today.
func TestClaudeWorkspaceWithoutProviderStillPlansThroughTheSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	// codexRepoDataDir resolves the repository root, where the shipped skills
	// live. It is provider-agnostic despite its name, so it is reused rather
	// than copied: a second resolver of the same root is a second thing to fix
	// when the layout moves.
	t.Setenv("ARCHETIPO_DATA_DIR", codexRepoDataDir(t))
	deps := executionTestDeps(t)

	if res := runExecutionRoot(t, deps, "init", "--tool", "claude", "--connector", "file", "--yes"); res.exit != 0 {
		t.Fatalf("init failed: exit=%d stdout=%s stderr=%s", res.exit, res.stdout.String(), res.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(".claude", "skills", "archetipo-plan", "SKILL.md")); err != nil {
		t.Fatalf("init --tool claude did not install the planning skill: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// The shipped template documents `default_provider` in a comment; what must
	// be absent is an active setting, so the commented lines are stripped first.
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, "default_provider") {
			t.Fatalf("init configured an execution provider: %q", line)
		}
	}
	exit, code, _ := decodeExecutionError(t, runExecutionRoot(t, deps, "execution", "provider", "show-default"))
	if exit != iox.ExitPreconditionMissing || code != iox.CodePreconditionMissing {
		t.Fatalf("the fresh workspace already has a default provider: exit=%d code=%s", exit, code)
	}

	seedExecutionSpec(t, deps)
	planExecutionSpec(t, deps)

	show := runExecutionRoot(t, deps, "spec", "show", "US-001")
	if state := decodeExecutionSpecState(t, show); state.Status != domain.StatusPlanned {
		t.Fatalf("the direct skill path did not plan the spec: %s", state.Status)
	}
	if tasks := decodeExecutionSpecTasks(t, show); tasks < 1 {
		t.Fatalf("the connector holds no plan tasks: %d", tasks)
	}
}
