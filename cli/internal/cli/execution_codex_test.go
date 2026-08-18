package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/codex"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// codexAuthSentinel stands for whatever authentication material a real Codex
// process can print — a token echoed by a debug flag, a session path, a login
// banner. It is exported into the environment and deliberately printed by the
// fake runtime so the assertions can prove it is dropped rather than merely
// absent by luck.
const codexAuthSentinel = "codex-session-token-DO-NOT-PERSIST"

// codexScript drives the fake Codex runtime for one scenario. It is the single
// seam the provider offers, so every test here exercises the real command
// building, the real availability probe and the real receipt gate without a
// `codex` binary existing anywhere on the machine.
//
// The two invocations the provider makes are kept apart on purpose: `--version`
// is the availability probe, everything else is the agent run. Scripting them
// separately is what lets a test say "the runtime is not usable" without also
// saying anything about the run that would have followed.
type codexScript struct {
	mu sync.Mutex

	// versionErr and versionExit describe the availability probe. A non-nil
	// versionErr is a process that never started; a non-zero versionExit is a
	// binary that answered but not with its version.
	versionErr  error
	versionExit int
	versionOut  string

	// exec describes the agent run. It may reach back into the CLI to persist a
	// plan, exactly as the local agent does.
	exec func() (stdout string, stderr string, exitCode int, err error)

	versionCalls int
	execCalls    int
	execArgs     []string
	execDir      string
}

var _ codex.Runner = (*codexScript)(nil)

func (s *codexScript) Run(_ context.Context, dir string, _ string, args []string) (string, string, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(args) == 1 && args[0] == "--version" {
		s.versionCalls++
		if s.versionErr != nil {
			return "", "", -1, s.versionErr
		}
		if s.versionExit != 0 {
			return "", "codex: spawn codex ENOENT", s.versionExit, nil
		}
		out := s.versionOut
		if out == "" {
			out = "codex-cli 0.0.0-test\n"
		}
		return out, "", 0, nil
	}
	s.execCalls++
	s.execArgs = append([]string(nil), args...)
	s.execDir = dir
	if s.exec == nil {
		return "", "", 0, nil
	}
	return s.exec()
}

func (s *codexScript) snapshot() (versionCalls, execCalls int, args []string, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.versionCalls, s.execCalls, append([]string(nil), s.execArgs...), s.execDir
}

// codexPlanReceipt renders the receipt line the agent is asked to close its run
// with. It is built from the shared constant, not from a literal, so a test can
// never accept a word the production gate would reject.
func codexPlanReceipt(specCode string, tasks int) string {
	return fmt.Sprintf(`{"spec_code":%q,"status":%q,"tasks":%d}`, specCode, execution.PlannedStatus, tasks)
}

// codexNoisyOutput wraps a payload in the chatter a real agent prints around
// its receipt, including the authentication sentinel. Nothing here may reach
// the persisted record.
func codexNoisyOutput(payload string) string {
	return strings.Join([]string{
		"[codex] reading session material " + os.Getenv("CODEX_TEST_AUTH"),
		"[codex] working...",
		payload,
	}, "\n") + "\n"
}

// fakeCodexBinary creates an executable file that satisfies exec.LookPath and
// nothing else. The provider looks the command up before probing it, so the
// path has to exist; the file is never actually run, because codexScript stands
// in for every invocation.
func fakeCodexBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-fake")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'this fake must never be executed' >&2\nexit 97\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// installCodexPlanSkill writes the marker `archetipo init --tool codex` leaves
// behind. The provider refuses to spawn without it, so a scenario that omitted
// it would be testing the guard instead of the run.
func installCodexPlanSkill(t *testing.T) {
	t.Helper()
	dir := filepath.Join(".agents", "skills", "archetipo-plan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: archetipo-plan\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newCodexScenario wires a temporary workspace with a seeded spec, the real
// codex provider over the scripted runtime, and the workspace default provider
// — all of it through archetipo commands, so what the tests drive is the same
// path a person drives.
func newCodexScenario(t *testing.T, script *codexScript) executionDependencies {
	t.Helper()
	t.Setenv("CODEX_TEST_AUTH", codexAuthSentinel)
	t.Chdir(t.TempDir())
	deps := executionTestDeps(t)
	if err := deps.registry.Register(codex.New(codex.Options{Runner: script})); err != nil {
		t.Fatal(err)
	}
	seedExecutionSpec(t, deps)
	installCodexPlanSkill(t)
	cfgPath := writeExecutionProviderPayload(t, "codex.json", fmt.Sprintf(
		`{"command":%q,"model":"gpt-5-codex","timeout_seconds":60}`, fakeCodexBinary(t)))
	selection := decodeExecutionProvider(t, runExecutionRoot(t, deps, "execution", "provider", "set-default", "codex", "--file", cfgPath))
	if selection.ID != codex.ProviderID {
		t.Fatalf("the workspace default is %q", selection.ID)
	}
	return deps
}

// readCodexExecutionRecord reads the file the store wrote, not the envelope the
// command printed. AC-3 is about what survives on disk, so the oracle has to be
// the disk.
func readCodexExecutionRecord(t *testing.T, id string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(".archetipo", "executions", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func codexExecutionID(requestID string) string {
	return execution.DeriveID("US-001", execution.ActionPlan, codex.ProviderID, requestID)
}

// assertCodexSpecUntouched is the AC-4 invariant: whatever went wrong, the spec
// is still where it was and holds no plan.
func assertCodexSpecUntouched(t *testing.T, deps executionDependencies, before executionSpecState) {
	t.Helper()
	show := runExecutionRoot(t, deps, "spec", "show", "US-001")
	after := decodeExecutionSpecState(t, show)
	if after.Status != domain.StatusTodo || !reflect.DeepEqual(before, after) {
		t.Fatalf("the spec changed across a failed codex run: before=%#v after=%#v", before, after)
	}
	if tasks := decodeExecutionSpecTasks(t, show); tasks != 0 {
		t.Fatalf("a failed codex run left %d plan tasks behind", tasks)
	}
}

// AC-2: a successful local run really plans the spec. The oracle is the
// connector re-read after the fact, not the receipt the agent declared.
func TestCodexPlanLocalHappyPath(t *testing.T) {
	script := &codexScript{}
	deps := newCodexScenario(t, script)
	script.exec = func() (string, string, int, error) {
		// The local agent plans the spec through the configured connector,
		// exactly as the prompt instructs it to.
		planExecutionSpec(t, deps)
		return codexNoisyOutput(codexPlanReceipt("US-001", 1)), "", 0, nil
	}

	before := decodeExecutionSpecState(t, runExecutionRoot(t, deps, "spec", "show", "US-001"))
	if before.Status != domain.StatusTodo {
		t.Fatalf("the spec did not start in TODO: %#v", before)
	}

	result := runExecutionRoot(t, deps, "execution", "run", "US-001", "plan", "--request-id", "r1")
	run := decodeExecution(t, result)
	if run.Status != execution.StatusSucceeded || run.SpecCode != "US-001" || run.ProviderID != codex.ProviderID || run.RequestID != "r1" {
		t.Fatalf("run = %#v", run)
	}
	if run.Result == nil {
		t.Fatalf("a succeeded run carries no result: %#v", run)
	}

	// The invocation is the real one: probed once, then run once, in the
	// workspace, with the prompt naming the spec.
	versionCalls, execCalls, args, dir := script.snapshot()
	if versionCalls != 1 || execCalls != 1 {
		t.Fatalf("version calls = %d, exec calls = %d", versionCalls, execCalls)
	}
	if len(args) == 0 || args[0] != "exec" {
		t.Fatalf("the codex invocation is not an exec run: %#v", args)
	}
	if prompt := args[len(args)-1]; !strings.Contains(prompt, "US-001") || !strings.Contains(prompt, "/archetipo-plan") {
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
	if payload.Model != "gpt-5-codex" || payload.ExitCode != 0 || payload.Command == "" {
		t.Fatalf("payload = %#v", payload)
	}
}

// AC-3: the record keeps provider, status and outcome, and nothing that came
// out of the agent's own streams. A run that prints its session material must
// leave no trace of it on disk or on the command's streams.
func TestCodexSuccessfulRunPersistsNoAuthenticationMaterial(t *testing.T) {
	script := &codexScript{}
	deps := newCodexScenario(t, script)
	script.exec = func() (string, string, int, error) {
		planExecutionSpec(t, deps)
		return codexNoisyOutput(codexPlanReceipt("US-001", 1)),
			"[codex] refreshed credentials for " + codexAuthSentinel, 0, nil
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
	if os.Getenv("CODEX_TEST_AUTH") != codexAuthSentinel {
		t.Fatal("the scenario did not export the sentinel")
	}

	for name, stream := range map[string]string{"stdout": result.stdout.String(), "stderr": result.stderr.String()} {
		if strings.Contains(stream, codexAuthSentinel) {
			t.Fatalf("the CLI %s leaked the codex session material", name)
		}
	}
	record := readCodexExecutionRecord(t, run.ID)
	if strings.Contains(record, codexAuthSentinel) {
		t.Fatalf("the persisted record leaked the codex session material:\n%s", record)
	}
	// The record is still a useful trace: provider, status and receipt survive.
	for _, want := range []string{codex.ProviderID, string(execution.StatusSucceeded), string(execution.PlannedStatus)} {
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
// contract the arcipelago provider is held to (see
// TestArcipelagoRemoteFailurePreservesSpec). The unconfirmed-effect case below
// is the one that fails the command, because there the CLI itself refuses a
// success the connector denies.
func TestCodexLocalFailuresPreserveTheSpec(t *testing.T) {
	for _, tc := range []struct {
		name     string
		arrange  func(*codexScript)
		wantText []string
		wantExec int
	}{
		{
			name:     "the runtime is not usable",
			arrange:  func(s *codexScript) { s.versionExit = 127 },
			wantText: []string{"instead of reporting its version", "spawn codex ENOENT"},
			wantExec: 0,
		},
		{
			name: "codex is not authenticated",
			arrange: func(s *codexScript) {
				s.exec = func() (string, string, int, error) {
					return "", "codex: not authenticated. Run `codex login` first.", 1, nil
				}
			},
			wantText: []string{"exited 1", "without planning US-001", "not authenticated"},
			wantExec: 1,
		},
		{
			name: "the run ends without a receipt",
			arrange: func(s *codexScript) {
				s.exec = func() (string, string, int, error) {
					return codexNoisyOutput("All done, I planned everything."), "", 0, nil
				}
			},
			wantText: []string{"exited 0 without having produced a plan for US-001"},
			wantExec: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := &codexScript{}
			tc.arrange(script)
			deps := newCodexScenario(t, script)
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

			// The record is the trace the person goes back to, so the reason has
			// to be on it and not only on the stream that scrolled past.
			shown := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", codexExecutionID("r1")))
			if shown.Status != execution.StatusFailed || shown.Error == nil || !reflect.DeepEqual(shown.Error, run.Error) {
				t.Fatalf("the persisted record = %#v", shown)
			}
			assertCodexSpecUntouched(t, deps, before)
		})
	}
}

// AC-4, the case a receipt alone cannot catch: the agent declares a plan it
// never persisted. The provider holds no connector by design, so the refusal
// comes from the shared effect check, and this is the one failure that fails
// the command.
func TestCodexValidReceiptWithoutAPlanIsNotASuccess(t *testing.T) {
	script := &codexScript{exec: func() (string, string, int, error) {
		return codexNoisyOutput(codexPlanReceipt("US-001", 3)), "", 0, nil
	}}
	deps := newCodexScenario(t, script)
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

	record := decodeExecution(t, runExecutionRoot(t, deps, "execution", "show", codexExecutionID("r1")))
	if record.Status != execution.StatusFailed || record.Result != nil || record.Error == nil {
		t.Fatalf("record = %#v", record)
	}
	if record.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("record error = %#v", record.Error)
	}
	if !strings.Contains(record.Error.Message, "US-001") || !strings.Contains(record.Error.Message, string(domain.StatusTodo)) {
		t.Fatalf("the record does not report the reason: %s", record.Error.Message)
	}
	assertCodexSpecUntouched(t, deps, before)
}

// AC-1: the provider the shipped binary registers is this one. A registry that
// only knows arcipelago would answer "register the requested provider before
// selecting it"; one that knows codex answers with codex's own configuration
// vocabulary.
func TestCodexProviderIsRegisteredInRealRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	deps := executionTestDeps(t)
	seedExecutionSpec(t, deps)

	rejected := writeExecutionProviderPayload(t, "codex-bad.json", `{"nonsense":true}`)
	result := runRealRoot(t, "execution", "provider", "set-default", codex.ProviderID, "--file", rejected)
	exit, code, text := decodeExecutionError(t, result)
	if exit != iox.ExitInvalidInput || code != iox.CodeInvalidInput {
		t.Fatalf("exit=%d code=%s text=%q", exit, code, text)
	}
	if strings.Contains(text, "register the requested provider") {
		t.Fatalf("codex is not registered in the real root: %q", text)
	}
	if !strings.Contains(text, "nonsense") {
		t.Fatalf("the message does not name the offending key, so codex's own validation did not run: %q", text)
	}

	// And it is selectable: a valid configuration is accepted and saved, without
	// the machine needing Codex installed.
	accepted := writeExecutionProviderPayload(t, "codex-good.json", `{"model":"gpt-5-codex","timeout_seconds":120}`)
	selection := decodeExecutionProvider(t, runRealRoot(t, "execution", "provider", "set-default", codex.ProviderID, "--file", accepted))
	if selection.ID != codex.ProviderID {
		t.Fatalf("selection = %#v", selection)
	}
	shown := decodeExecutionProvider(t, runRealRoot(t, "execution", "provider", "show-default"))
	if shown.ID != codex.ProviderID || shown.Config["model"] != "gpt-5-codex" {
		t.Fatalf("show-default = %#v", shown)
	}
}

// AC-5: the local provider is an addition, not a prerequisite. A workspace
// initialized for Codex plans through the skill with no provider configured at
// all — which is what people who drive ARchetipo from inside Codex do today.
func TestCodexWorkspaceWithoutProviderStillPlansThroughTheSkill(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ARCHETIPO_DATA_DIR", codexRepoDataDir(t))
	deps := executionTestDeps(t)

	if res := runExecutionRoot(t, deps, "init", "--tool", "codex", "--connector", "file", "--yes"); res.exit != 0 {
		t.Fatalf("init failed: exit=%d stdout=%s stderr=%s", res.exit, res.stdout.String(), res.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "archetipo-plan", "SKILL.md")); err != nil {
		t.Fatalf("init --tool codex did not install the planning skill: %v", err)
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

// codexRepoDataDir resolves the repository root, where the shipped skills live,
// so `init` can be exercised without a published package.
func codexRepoDataDir(t *testing.T) string {
	t.Helper()
	// This file lives at cli/internal/cli/; the repo root is three levels up.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot resolve the caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// decodeExecutionPayload decodes the provider payload of a record, failing the
// test rather than returning an error nobody would check.
func decodeExecutionPayload(t *testing.T, raw []byte, into any) {
	t.Helper()
	if len(raw) == 0 {
		t.Fatal("the record carries an empty provider payload")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode payload %q: %v", string(raw), err)
	}
}
