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
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// Generating a backlog is the second conversational action of this provider,
// and these tests drive it through the same seam the inception ones use: the
// process is a double, so what is asserted is the frames really exchanged with
// it, and no machine that has Claude Code on it is needed to prove any of it.
//
// The run id is the one the request carries, so every assertion about the run
// goes through the collaborator the viewer would use.
const backlogRunID = "exec-1"

// The counts the agent claims it persisted. They are informative for this
// layer: confirming that the epics and the specs really exist happens one layer
// up, against the connector.
const (
	backlogEpics = 3
	backlogSpecs = 11
)

// backlogWorkspace is a working directory with the spec skill installed, which
// is the precondition executeBacklog checks before spawning.
func backlogWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, backlogSkillRelPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# archetipo-spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// backlogRequest is the workspace-scoped request: it carries no spec, because
// the object of a backlog generation is the workspace itself — the specs are
// its outcome, not its input.
func backlogRequest(command string) execution.Request {
	return execution.Request{
		ExecutionID:    backlogRunID,
		Action:         execution.ActionBacklog,
		Capability:     execution.CapabilityWorkspaceBacklog,
		ProviderConfig: map[string]any{"command": command},
	}
}

func backlogReceiptLine(epics, specs int) string {
	return fmt.Sprintf(`{"artifact":"backlog","status":%q,"epics":%d,"specs":%d}`, execution.WrittenStatus, epics, specs)
}

// startBacklog dispatches the action on its own goroutine and hands back the
// channels its outcome will arrive on, so the test can act on the conversation
// while Execute is still inside it.
func startBacklog(provider *Provider, req execution.Request) (<-chan execution.Result, <-chan error) {
	results := make(chan execution.Result, 1)
	failures := make(chan error, 1)
	go func() {
		res, err := provider.Execute(context.Background(), req)
		results <- res
		failures <- err
	}()
	return results, failures
}

// --- the prompt -------------------------------------------------------------

// The prompt is what makes the run reproducible: it is a pure function of the
// request, it names the skill that does the work, and it asks for exactly the
// receipt the shared acceptance gate recognizes. A prompt asking for anything
// else would produce a run this provider could never accept.
func TestBuildBacklogPromptAsksForTheSkillAndTheSharedReceipt(t *testing.T) {
	req := backlogRequest("claude")
	first := buildBacklogPrompt(req)
	if second := buildBacklogPrompt(req); first != second {
		t.Fatalf("the backlog prompt is not deterministic:\n%s\n---\n%s", first, second)
	}
	assertContains(t, first, "/archetipo-spec", "backlog prompt")
	assertContains(t, first, "archetipo spec add", "backlog prompt")
	assertContains(t, first, `{"artifact":"backlog","status":"`+execution.WrittenStatus+`","epics":<N>,"specs":<M>}`, "backlog prompt")
	assertContains(t, first, "a single question per message", "backlog prompt")

	// The receipt shape asked for is the one the shared gate accepts: the two
	// must never be able to drift.
	if _, err := execution.AcceptBacklogReceipt(backlogReceiptLine(1, 1)); err != nil {
		t.Fatalf("the receipt the prompt asks for is not the one the gate accepts: %v", err)
	}
	// A backlog run must never be asked to plan a spec or to write a PRD.
	for _, forbidden := range []string{"/archetipo-plan", "/archetipo-inception", "archetipo prd write"} {
		if strings.Contains(first, forbidden) {
			t.Fatalf("the backlog prompt asks for %q:\n%s", forbidden, first)
		}
	}
}

// --- the guard --------------------------------------------------------------

// A workspace without the spec skill cannot run the conversation at all, and
// the refusal must come before anything is spawned: a process started here
// would burn the whole timeout to fail on an unknown command. The oracle is the
// double never being asked for a process.
func TestBacklogRefusesBeforeSpawningWhenTheSkillIsMissing(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := newSessionProvider(t.TempDir(), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	got, err := provider.Execute(context.Background(), backlogRequest(command))
	if err == nil {
		t.Fatalf("a workspace without the spec skill was accepted: %s", got.Payload)
	}
	if got.Payload != nil {
		t.Fatalf("a failed backlog returned a payload: %s", got.Payload)
	}
	if starts, _, _ := fake.spawned(); starts != 0 {
		t.Fatalf("the process was started %d time(s) before the missing skill was noticed", starts)
	}
	assertContains(t, err.Error(), "spec skill is not installed", "execution error")
	assertContains(t, err.Error(), backlogSkillRelPath, "execution error")
	assertContains(t, err.Error(), "archetipo init --tool claude", "execution error")
}

// --- AC-2: the conversation ends on the receipt and on nothing else ----------

// AC-2 — one whole backlog generation, from the agent's question to the receipt
// that ends it. It is one test because these are not independent facts: that
// the run survives a turn without a receipt is only meaningful if the receipt
// then closes it, and splitting them would let each half pass while the
// conversation as a whole was broken.
func TestBacklogKeepsTheConversationOpenUntilTheReceipt(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	fake.stderr = "warning: " + sentinel
	provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	req := backlogRequest(command)
	results, failures := startBacklog(provider, req)

	// --- the instruction really delivered to the process --------------------
	<-fake.started
	if got := fake.messagesReceived(); len(got) != 1 || got[0] != buildBacklogPrompt(req) {
		t.Fatalf("the process received %v; want exactly the backlog prompt", got)
	}

	// --- the agent asks its question and ends the turn on it ----------------
	const question = "Il backlog deve coprire anche l'onboarding, o solo il flusso principale?"
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":` + quoted(question) + `}]}}`)
	fake.emit(resultFrame(question, false))

	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, backlogRunID, 0), localrun.KindTurnEnd) == 1
	})
	select {
	case err := <-failures:
		t.Fatalf("Execute returned on the agent's question instead of waiting for the answer: %v", err)
	default:
	}
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: backlogRunID})
	if err != nil {
		t.Fatalf("ReadRun failed while the conversation was open: %v", err)
	}
	if snapshot.State != execution.RunActive {
		t.Fatalf("the run reported %q after the first turn: a question is not the end of a backlog generation", snapshot.State)
	}

	// --- the second turn closes the conversation on the receipt -------------
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"scrivo le storie, token=` + sentinel + `"}]}}`)
	fake.emit(resultFrame("Fatto.\n"+backlogReceiptLine(backlogEpics, backlogSpecs), false))

	if err := <-failures; err != nil {
		t.Fatalf("the receipt did not close the backlog generation: %v", err)
	}
	got := <-results

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got.Payload, &fields); err != nil {
		t.Fatalf("payload is not valid JSON (%s): %v", got.Payload, err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"backlog_epics", "backlog_specs", "command", "duration_ms", "exit_code", "model", "result_summary", "turns"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("payload fields = %v, want %v", keys, want)
	}

	var payload struct {
		Command       string `json:"command"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		BacklogEpics  int    `json:"backlog_epics"`
		BacklogSpecs  int    `json:"backlog_specs"`
		Turns         int    `json:"turns"`
		DurationMS    int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != command || payload.ExitCode != 0 || payload.DurationMS != 1500 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.BacklogEpics != backlogEpics || payload.BacklogSpecs != backlogSpecs {
		t.Fatalf("the payload reports %d epic(s) and %d spec(s), want the counts the receipt declared", payload.BacklogEpics, payload.BacklogSpecs)
	}
	if payload.Turns != 2 {
		t.Fatalf("turns = %d, want the two turns the conversation really took", payload.Turns)
	}

	// --- result_summary is the receipt alone, re-rendered -------------------
	//
	// Not a slice of the output: what the agent printed around the line — here
	// a word before it and a secret in an earlier message — must not be able to
	// travel with it into the record.
	rendered, err := json.Marshal(execution.BacklogReceipt{
		Artifact: "backlog",
		Status:   execution.WrittenStatus,
		Epics:    backlogEpics,
		Specs:    backlogSpecs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.ResultSummary != string(rendered) {
		t.Fatalf("result_summary = %q, want the re-rendered receipt %q", payload.ResultSummary, rendered)
	}
	if strings.Contains(payload.ResultSummary, "Fatto.") {
		t.Fatalf("result_summary carried what the agent printed around the receipt: %q", payload.ResultSummary)
	}
	if strings.Contains(string(got.Payload), sentinel) {
		t.Fatalf("the payload carried the agent output: %s", got.Payload)
	}
	if snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: backlogRunID}); err != nil || snapshot.State != execution.RunClosed {
		t.Fatalf("the finished run reported %#v (err=%v), want the observed closed state", snapshot, err)
	}
}

// A receipt published in the very instant the process leaves is still a
// receipt: the buffered outcome is drained before the death of the process is
// treated as the end of the run. Without the draining rule a backlog that was
// really persisted would be reported FAILED, and the partial-backlog cleanup
// would then discard it.
func TestBacklogSucceedsWhenTheReceiptAndTheEndOfTheProcessCoincide(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	results, failures := startBacklog(provider, backlogRequest(command))
	<-fake.started
	fake.emit(resultFrame(backlogReceiptLine(backlogEpics, backlogSpecs), false))
	fake.end()

	if err := <-failures; err != nil {
		t.Fatalf("a receipt published as the process left was thrown away: %v", err)
	}
	var payload struct {
		BacklogEpics int `json:"backlog_epics"`
		BacklogSpecs int `json:"backlog_specs"`
	}
	if err := json.Unmarshal((<-results).Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.BacklogEpics != backlogEpics || payload.BacklogSpecs != backlogSpecs {
		t.Fatalf("payload = %#v, want the counts the receipt declared", payload)
	}
}

// --- AC-4: the ways a backlog generation ends without a backlog -------------

// Each of them must be told apart from the others, none of them may read as a
// success, and every one of them names the backlog: a diagnostic that talked
// about the PRD or about planning would send whoever reads it to the wrong
// place.
func TestBacklogFailureModes(t *testing.T) {
	command := fakeCommand(t)
	cases := []struct {
		name    string
		timeout int
		drive   func(fake *fakeClaude)
		wantErr []string
	}{
		{
			name: "the agent closes a turn with an error",
			drive: func(fake *fakeClaude) {
				fake.stderr = "claude: the model refused to continue"
				go func() {
					<-fake.started
					fake.emit(resultFrame("Non posso continuare.", true))
				}()
			},
			wantErr: []string{
				"ended the backlog generation on a turn that did not complete",
				"without having produced a backlog",
				"claude: the model refused to continue",
			},
		},
		{
			name: "the process leaves between two turns",
			drive: func(fake *fakeClaude) {
				fake.exitCode = 1
				go func() {
					<-fake.started
					fake.emit(resultFrame("Quante epiche vuoi?", false))
					waitForTurnEnd(fake)
					fake.end()
				}()
			},
			wantErr: []string{"exited 1", "without having produced a backlog", "the backlog generation ended without a receipt"},
		},
		{
			name:    "the conversation runs past the configured timeout",
			timeout: 1,
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"sto leggendo il PRD"}]}}`)
				}()
			},
			wantErr: []string{"did not finish the backlog within 1s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			if tc.drive != nil {
				tc.drive(fake)
			}
			req := backlogRequest(command)
			if tc.timeout > 0 {
				req.ProviderConfig["timeout_seconds"] = tc.timeout
			}
			provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

			got, err := provider.Execute(context.Background(), req)
			if err == nil {
				t.Fatalf("expected an error, got payload %s", got.Payload)
			}
			if got.Payload != nil {
				t.Fatalf("a failed backlog returned a payload: %s", got.Payload)
			}
			var remote *execution.RemoteError
			if errors.As(err, &remote) {
				t.Fatalf("a local run reported a remote unit of work: %v", err)
			}
			for _, want := range tc.wantErr {
				assertContains(t, err.Error(), want, "execution error")
			}
			for _, forbidden := range []string{"PRD", "planning"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("the backlog diagnostic talks about %q: %v", forbidden, err)
				}
			}
			snapshot, readErr := provider.ReadRun(context.Background(), execution.RunRequest{RunID: backlogRunID})
			if readErr != nil {
				t.Fatalf("ReadRun failed after the failure: %v", readErr)
			}
			if snapshot.State != execution.RunCrashed || strings.TrimSpace(snapshot.Error) == "" {
				t.Fatalf("the failed run = %#v, want a crashed run that says why", snapshot)
			}
		})
	}
}

// AC-4 — a run stopped between two turns is the fourth cause, and it is not the
// timeout: the input of the process is closed, and the run ends only when the
// process's output really ends. Until then Execute is still inside the
// conversation.
func TestBacklogStoppedBetweenTwoTurnsEndsOnlyWithTheProcess(t *testing.T) {
	command := fakeCommand(t)
	fake := newLingeringClaude()
	t.Cleanup(fake.end)
	provider := New(Options{
		Runner:     &fakeRunner{outcomes: []runOutcome{probeOK}},
		Starter:    fake,
		WorkingDir: func() (string, error) { return backlogWorkspace(t), nil },
		Now:        fixedElapsedClock(1500 * time.Millisecond),
	})

	results, failures := startBacklog(provider, backlogRequest(command))
	<-fake.started
	fake.emit(resultFrame("Vuoi un'epica dedicata alle notifiche?", false))
	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, backlogRunID, 0), localrun.KindTurnEnd) == 1
	})

	if err := provider.CancelRun(context.Background(), execution.RunRequest{RunID: backlogRunID}); err != nil {
		t.Fatalf("the cancellation between two turns was refused: %v", err)
	}
	select {
	case <-fake.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("the cancellation never reached the input of the process")
	}
	select {
	case err := <-failures:
		t.Fatalf("Execute returned on the command instead of on the end of the process: %v", err)
	default:
	}

	fake.exitCode = 143
	fake.end()

	err := <-failures
	if err == nil {
		t.Fatalf("a stopped backlog generation reported a success: %s", (<-results).Payload)
	}
	if payload := (<-results).Payload; payload != nil {
		t.Fatalf("a stopped backlog generation returned a payload: %s", payload)
	}
	assertContains(t, err.Error(), "without having produced a backlog", "execution error")
}
