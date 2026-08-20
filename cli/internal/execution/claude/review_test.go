package claude

import (
	"context"
	"encoding/json"
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

// Preparing a review is the third single-turn action of this provider, and what
// makes it its own action is not only its skill and its receipt: it is the one
// action whose success is defined by something *not* happening. These tests
// therefore assert as much on what the prompt forbids as on what it asks for.
// Everything they touch goes through the injectable seams, so the whole file
// runs on a machine that has no Claude Code on it.

// reviewWorkspace is a working directory with the review skill installed, which
// is the precondition executeReview checks before spawning.
func reviewWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, reviewSkillRelPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# archetipo-review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func reviewRequest(command string) execution.Request {
	return execution.Request{
		ExecutionID:    "exec-1",
		SpecCode:       testSpec,
		Action:         execution.ActionReview,
		Capability:     execution.CapabilitySpecReview,
		ProviderConfig: map[string]any{"command": command},
	}
}

func reviewReceiptLine(specCode string, criteria, blockers int) string {
	return fmt.Sprintf(`{"spec_code":%q,"status":%q,"criteria":%d,"blockers":%d}`, specCode, execution.ReviewStatus, criteria, blockers)
}

// reviewedSession drives the fake to the outcome of a successfully prepared
// review: the agent reads the increment, then closes the turn with the receipt
// as the message the run ends on.
func reviewedSession(fake *fakeClaude, specCode string, criteria, blockers int) {
	go func() {
		<-fake.started
		fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"preparo il dossier"}]}}`)
		fake.emit(resultFrame(reviewReceiptLine(specCode, criteria, blockers), false))
	}()
}

// --- buildReviewPrompt -----------------------------------------------------

func TestBuildReviewPromptIsDeterministicAndAsksForTheSharedReceipt(t *testing.T) {
	req := reviewRequest("claude")
	first := buildReviewPrompt(req)
	if second := buildReviewPrompt(req); first != second {
		t.Fatalf("prompt is not deterministic:\n%s\n---\n%s", first, second)
	}
	assertContains(t, first, testSpec, "prompt")
	assertContains(t, first, "/archetipo-review "+testSpec, "prompt")
	assertContains(t, first, "archetipo spec review-dossier "+testSpec, "prompt")
	assertContains(t, first, req.ExecutionID, "prompt")
	assertContains(t, first, `{"spec_code":"`+testSpec+`","status":"`+execution.ReviewStatus+`","criteria":<N>,"blockers":<M>}`, "prompt")
	// The three actions must not ask for the same thing.
	for _, other := range []string{"/archetipo-plan", "/archetipo-implement"} {
		if strings.Contains(first, other) {
			t.Fatalf("the review prompt invokes %s:\n%s", other, first)
		}
	}
}

// AC-2 — the prompt names the three transitions the agent must not perform. The
// forbidding is what turns a helpful agent into a harmless one, so removing any
// of the three names must fail here.
func TestBuildReviewPromptForbidsEveryTransitionByName(t *testing.T) {
	prompt := buildReviewPrompt(reviewRequest("claude"))
	for _, forbidden := range []string{
		"archetipo spec move",
		"archetipo spec integrate",
		"archetipo spec request-changes",
	} {
		assertContains(t, prompt, forbidden, "prompt")
	}
}

// --- the skill gate --------------------------------------------------------

// AC-1, AC-5 — a workspace without the review skill cannot run the action, and
// that must be said before anything is spawned.
func TestExecuteReviewRefusesAMissingSkillBeforeSpawning(t *testing.T) {
	command := fakeCommand(t)
	// The planning skill is installed and the review one is not, so a gate that
	// checked the wrong path would pass here.
	dir := workspaceWithSkill(t)
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	t.Cleanup(fake.end)

	_, err := newSessionProvider(dir, runner, fake, nil).Execute(context.Background(), reviewRequest(command))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"review skill is not installed", reviewSkillRelPath, "archetipo init --tool claude"} {
		assertContains(t, err.Error(), want, "execution error")
	}
	if starts, _, _ := fake.spawned(); starts != 0 {
		t.Fatalf("a missing skill started %d session(s)", starts)
	}
}

// --- Execute: success ------------------------------------------------------

// AC-1 — a valid receipt produces a payload carrying the counts a person reads
// before opening anything, and nothing the agent printed.
func TestExecuteReviewReturnsAPayloadBuiltFromTheReceipt(t *testing.T) {
	command := fakeCommand(t)
	dir := reviewWorkspace(t)
	runner := &fakeRunner{outcomes: []runOutcome{probeOK}}
	fake := newFakeClaude()
	fake.stderr = sentinel
	reviewedSession(fake, testSpec, 5, 2)
	req := reviewRequest(command)
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
	want := []string{"blockers", "command", "criteria", "duration_ms", "exit_code", "model", "result_summary"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("payload fields = %v, want %v", keys, want)
	}

	var payload struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		Criteria      int    `json:"criteria"`
		Blockers      int    `json:"blockers"`
		DurationMS    int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != command || payload.Model != "opus" || payload.ExitCode != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Criteria != 5 || payload.Blockers != 2 {
		t.Fatalf("criteria = %d, blockers = %d, want the counts declared by the receipt", payload.Criteria, payload.Blockers)
	}
	if payload.DurationMS != 1500 {
		t.Fatalf("duration_ms = %d", payload.DurationMS)
	}
	var summary execution.ReviewReceipt
	if err := json.Unmarshal([]byte(payload.ResultSummary), &summary); err != nil {
		t.Fatalf("result_summary is not the re-rendered receipt (%s): %v", payload.ResultSummary, err)
	}
	if summary != (execution.ReviewReceipt{SpecCode: testSpec, Status: execution.ReviewStatus, Criteria: 5, Blockers: 2}) {
		t.Fatalf("result_summary = %#v", summary)
	}
	if strings.Contains(string(got.Payload), sentinel) {
		t.Fatalf("the payload carries what the agent printed: %s", got.Payload)
	}

	// The instruction the process really received is the review one.
	if got := fake.messagesReceived(); len(got) != 1 || got[0] != buildReviewPrompt(req) {
		t.Fatalf("the process received %v; want exactly the review prompt", got)
	}
}

// An increment with nothing in its way is an ordinary outcome, not a weaker
// one: zero blockers must still be a success.
func TestExecuteReviewAcceptsADossierWithoutBlockers(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	reviewedSession(fake, testSpec, 4, 0)
	provider := newSessionProvider(reviewWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
	if _, err := provider.Execute(context.Background(), reviewRequest(command)); err != nil {
		t.Fatal(err)
	}
}

// --- Execute: failures -----------------------------------------------------

// AC-1, AC-2, AC-5 — every way a review run can end without having prepared the
// evidence produces an error that names the command and the dispatched spec,
// and none of them returns a payload. The DONE case is the one the whole story
// exists for: an agent that decided in the person's place is refused here,
// before the effect of the action is even looked at.
func TestExecuteReviewFailureModes(t *testing.T) {
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
			wantErr: []string{"exited 1", "without preparing the review of " + testSpec, "Please run /login"},
		},
		{
			name: "no receipt at all",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame("dossier ready, looks good to me", false))
				}()
			},
			wantErr: []string{"ended without having prepared the review of " + testSpec, "did not emit the expected JSON receipt line"},
		},
		{
			name: "the agent decided in the person's place and declared the spec closed",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(`{"spec_code":"`+testSpec+`","status":"DONE","criteria":5,"blockers":0}`, false))
				}()
			},
			wantErr: []string{"does not declare a prepared review dossier for " + testSpec},
		},
		{
			name:    "receipt for another spec",
			drive:   func(fake *fakeClaude) { reviewedSession(fake, "US-999", 4, 0) },
			wantErr: []string{"does not declare a prepared review dossier for " + testSpec},
		},
		{
			name:    "receipt declaring no examined criterion",
			drive:   func(fake *fakeClaude) { reviewedSession(fake, testSpec, 0, 0) },
			wantErr: []string{"does not declare a prepared review dossier for " + testSpec},
		},
		{
			name: "an interrupted turn that exits 0 carries no dossier",
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(resultFrame(reviewReceiptLine(testSpec, 4, 0), true))
				}()
			},
			wantErr: []string{"exited 0", "without preparing the review of " + testSpec, "the turn never completed"},
		},
		{
			name:    "a turn that never ends",
			expired: true,
			wantErr: []string{"did not finish preparing the review of " + testSpec, "within 1s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			if tc.drive != nil {
				tc.drive(fake)
			}
			req := reviewRequest(command)
			if tc.expired {
				req.ProviderConfig["timeout_seconds"] = 1
			}
			provider := newSessionProvider(reviewWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
			got, err := provider.Execute(context.Background(), req)
			if err == nil {
				t.Fatalf("expected an error, got payload %s", got.Payload)
			}
			if got.Payload != nil {
				t.Fatalf("a failed execution returned a payload: %s", got.Payload)
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
