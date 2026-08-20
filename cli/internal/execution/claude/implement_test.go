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
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// Implementing a spec is the second single-turn action of this provider, and
// these tests are about what makes it a different action rather than a second
// name for planning: its own skill, its own prompt, its own receipt and its own
// payload. Everything they touch goes through the injectable seams, so the
// whole file runs on a machine that has no Claude Code on it.

// testTests is the one-line account of the final suite the agent reports. It is
// informative — the authority on what really ran is the connector, one layer up
// — but it must survive into the record, which is what the payload test proves.
const testTests = "go test ./... — 843 passed"

// implementWorkspace is a working directory with the implementation skill
// installed, which is the precondition executeImplement checks before spawning.
func implementWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, implementSkillRelPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# archetipo-implement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func implementRequest(command string) execution.Request {
	return execution.Request{
		ExecutionID:    "exec-1",
		SpecCode:       testSpec,
		Action:         execution.ActionImplement,
		Capability:     execution.CapabilitySpecImplement,
		ProviderConfig: map[string]any{"command": command},
	}
}

func implementReceiptLine(specCode string, tasksDone int, tests string) string {
	return fmt.Sprintf(`{"spec_code":%q,"status":%q,"tasks_done":%d,"tests":%q}`, specCode, execution.ReviewStatus, tasksDone, tests)
}

// implementedSession drives the fake to the outcome of a successful
// implementation: the agent works, then closes the turn with the receipt as the
// message the run ends on.
func implementedSession(fake *fakeClaude, specCode string, tasksDone int, tests string) {
	go func() {
		<-fake.started
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"implemento"}]}}`)
		fake.emit(resultFrame(implementReceiptLine(specCode, tasksDone, tests), false))
	}()
}

// --- buildImplementPrompt --------------------------------------------------

func TestBuildImplementPromptIsDeterministicAndAsksForTheSharedReceipt(t *testing.T) {
	req := implementRequest("claude")
	first := buildImplementPrompt(req)
	if second := buildImplementPrompt(req); first != second {
		t.Fatalf("prompt is not deterministic:\n%s\n---\n%s", first, second)
	}
	assertContains(t, first, testSpec, "prompt")
	assertContains(t, first, "/archetipo-implement "+testSpec, "prompt")
	assertContains(t, first, execution.ReviewStatus, "prompt")
	assertContains(t, first, `{"spec_code":"`+testSpec+`","status":"`+execution.ReviewStatus+`","tasks_done":<N>,"tests":"<summary>"}`, "prompt")
	if reviewStatus != execution.ReviewStatus {
		t.Fatalf("the prompt status %q drifted from the shared one %q", reviewStatus, execution.ReviewStatus)
	}
	// The two single-turn actions must not ask for the same thing: a prompt
	// that still named the planning skill would run the wrong work under the
	// right action.
	if strings.Contains(first, "/archetipo-plan") {
		t.Fatalf("the implementation prompt invokes the planning skill:\n%s", first)
	}
}

// --- the skill gate --------------------------------------------------------

// AC-2, AC-5 — a workspace without the implementation skill cannot run the
// action, and that must be said before anything is spawned: the run would
// otherwise burn a whole timeout to fail on an unknown command.
func TestExecuteImplementRefusesAMissingSkillBeforeSpawning(t *testing.T) {
	command := fakeCommand(t)
	// The planning skill is installed and the implementation one is not, so a
	// gate that checked the wrong path would pass here.
	dir := workspaceWithSkill(t)
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	t.Cleanup(fake.end)

	_, err := newSessionProvider(dir, runner, fake, nil).Execute(context.Background(), implementRequest(command))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"implementation skill is not installed", implementSkillRelPath, "archetipo init --tool claude"} {
		assertContains(t, err.Error(), want, "execution error")
	}
	if starts, _, _ := fake.spawned(); starts != 0 {
		t.Fatalf("a missing skill started %d session(s)", starts)
	}
}

// --- Execute: success ------------------------------------------------------

// AC-4 — a valid receipt produces a payload that carries the summary of the
// work and of the tests, so a reviewer can read it without opening the run.
func TestExecuteImplementReturnsAPayloadBuiltFromTheReceipt(t *testing.T) {
	command := fakeCommand(t)
	dir := implementWorkspace(t)
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	implementedSession(fake, testSpec, 7, testTests)
	req := implementRequest(command)
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
	want := []string{"command", "duration_ms", "exit_code", "model", "result_summary", "tasks_done", "tests"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("payload fields = %v, want %v", keys, want)
	}

	var payload struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		TasksDone     int    `json:"tasks_done"`
		Tests         string `json:"tests"`
		DurationMS    int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != command || payload.Model != "opus" || payload.ExitCode != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.TasksDone != 7 {
		t.Fatalf("tasks_done = %d, want the 7 declared by the receipt", payload.TasksDone)
	}
	if payload.Tests != testTests {
		t.Fatalf("tests = %q, want the summary declared by the receipt", payload.Tests)
	}
	if payload.DurationMS != 1500 {
		t.Fatalf("duration_ms = %d", payload.DurationMS)
	}
	// result_summary is the receipt re-rendered from the parsed value, never a
	// slice of the output, so nothing the agent printed around it travels with
	// it.
	var summary execution.ImplementReceipt
	if err := json.Unmarshal([]byte(payload.ResultSummary), &summary); err != nil {
		t.Fatalf("result_summary is not the re-rendered receipt (%s): %v", payload.ResultSummary, err)
	}
	if summary != (execution.ImplementReceipt{SpecCode: testSpec, Status: execution.ReviewStatus, TasksDone: 7, Tests: testTests}) {
		t.Fatalf("result_summary = %#v", summary)
	}

	// The instruction the process really received is the implementation one.
	if got := fake.messagesReceived(); len(got) != 1 || got[0] != buildImplementPrompt(req) {
		t.Fatalf("the process received %v; want exactly the implementation prompt", got)
	}
}

// --- Execute: failures -----------------------------------------------------

// AC-5 — every way an implementation run can end without having implemented
// the spec produces an error that names the command and the dispatched spec,
// and none of them returns a payload.
func TestExecuteImplementFailureModes(t *testing.T) {
	command := fakeCommand(t)
	cases := []struct {
		name    string
		drive   func(fake *fakeClaude)
		expired bool
		wantErr []string
	}{
		{
			name: "the process dies before the turn ends",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.exitCode = 1
					fake.stderr = "Invalid API key · Please run /login"
					fake.end()
				}()
			},
			wantErr: []string{"exited 1", "without implementing " + testSpec, "Please run /login"},
		},
		{
			name: "no receipt at all",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame("done, I implemented everything", false))
				}()
			},
			wantErr: []string{"ended without having implemented " + testSpec, "did not emit the expected JSON receipt line"},
		},
		{
			name:    "receipt for another spec",
			drive:   func(fake *fakeClaude) { implementedSession(fake, "US-999", 4, testTests) },
			wantErr: []string{"ended without having implemented " + testSpec, "does not declare a completed implementation for " + testSpec},
		},
		{
			name: "receipt declaring the wrong status",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(`{"spec_code":"`+testSpec+`","status":"IN PROGRESS","tasks_done":4,"tests":"ok"}`, false))
				}()
			},
			wantErr: []string{"does not declare a completed implementation for " + testSpec},
		},
		{
			name:    "receipt declaring no completed task",
			drive:   func(fake *fakeClaude) { implementedSession(fake, testSpec, 0, testTests) },
			wantErr: []string{"does not declare a completed implementation for " + testSpec},
		},
		{
			name:    "receipt without a test summary",
			drive:   func(fake *fakeClaude) { implementedSession(fake, testSpec, 4, "  ") },
			wantErr: []string{"does not declare a completed implementation for " + testSpec},
		},
		{
			// An interrupted turn is the one case where the process can close
			// the turn with an error and still exit 0, so the turn — not the
			// exit code — is what decides.
			name: "an interrupted turn that exits 0 carries no implementation",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(implementReceiptLine(testSpec, 4, testTests), true))
				}()
			},
			wantErr: []string{"exited 0", "without implementing " + testSpec, "the turn never completed"},
		},
		{
			name:    "a turn that never ends",
			expired: true,
			wantErr: []string{"did not finish implementing " + testSpec, "within 1s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			if tc.drive != nil {
				tc.drive(fake)
			}
			req := implementRequest(command)
			if tc.expired {
				req.ProviderConfig["timeout_seconds"] = 1
			}
			provider := newSessionProvider(implementWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
			got, err := provider.Execute(context.Background(), req)
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
			assertContains(t, err.Error(), command, "execution error")
			for _, want := range tc.wantErr {
				assertContains(t, err.Error(), want, "execution error")
			}
			if starts, _, _ := fake.spawned(); starts != 1 {
				t.Fatalf("the session was started %d time(s), want 1", starts)
			}
		})
	}
}

// The two single-turn actions must stay distinguishable in a record: a shared
// phrase would make a failed plan and a failed implementation read the same.
func TestSingleTurnDiagnosticsNameTheirOwnAction(t *testing.T) {
	command := fakeCommand(t)
	cases := []struct {
		name    string
		dir     func(t *testing.T) string
		request func(string) execution.Request
		want    string
		absent  string
	}{
		{name: "planning", dir: workspaceWithSkill, request: testRequest, want: "did not finish planning " + testSpec, absent: "implementing"},
		{name: "implementing", dir: implementWorkspace, request: implementRequest, want: "did not finish implementing " + testSpec, absent: "planning"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			req := tc.request(command)
			req.ProviderConfig["timeout_seconds"] = 1
			provider := newSessionProvider(tc.dir(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
			_, err := provider.Execute(context.Background(), req)
			if err == nil {
				t.Fatal("expected an error")
			}
			assertContains(t, err.Error(), tc.want, "timeout error")
			if strings.Contains(err.Error(), tc.absent) {
				t.Fatalf("the %s diagnostic mentions the other action: %v", tc.name, err)
			}
		})
	}
}

// The agent's stream never enters the record, whatever the action: the receipt
// is the only thing that travels out of a successful run.
func TestSuccessfulImplementationNeverPersistsTheAgentOutput(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	fake.stderr = sentinel
	go func() {
		<-fake.started
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"` + sentinel + `"}]}}`)
		fake.emit(resultFrame(implementReceiptLine(testSpec, 3, testTests), false))
	}()

	provider := newSessionProvider(implementWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
	got, err := provider.Execute(context.Background(), implementRequest(command))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got.Payload), sentinel) {
		t.Fatalf("the payload carries what the agent printed: %s", got.Payload)
	}
}

// The timeout that bounds the run is the configured one, and the elapsed clock
// keeps measuring the work rather than the wall — the same guarantee planning
// has, asserted for the action that can take far longer.
func TestExecuteImplementUsesTheConfiguredTimeout(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	req := implementRequest(command)
	req.ProviderConfig["timeout_seconds"] = 1

	started := time.Now()
	provider := newSessionProvider(implementWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
	_, err := provider.Execute(context.Background(), req)
	if err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("the run took %s: the configured timeout did not bound it", elapsed)
	}
	assertContains(t, err.Error(), "within 1s", "timeout error")
}
