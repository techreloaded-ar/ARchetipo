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

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// The inception is the one action of this provider that is a conversation, and
// a conversation is exactly what a single-turn run cannot be: the agent ends a
// turn *in order to ask a question*, and the answer opens the next one. These
// tests drive the whole thing through the process seam, so what is asserted is
// the frames really exchanged with the process — never a double standing in for
// the dialogue itself.
//
// The run id is the one the request carries, so every assertion about the run
// goes through the collaborator the viewer would use.
const inceptionRunID = "exec-1"

// prdPath is what the agent claims it wrote. It is informative for this layer:
// confirming the document really exists happens one layer up, against the
// connector.
const prdPath = "docs/PRD.md"

// inceptionWorkspace is a working directory with the inception skill installed,
// which is the precondition executeInception checks before spawning.
func inceptionWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, inceptionSkillRelPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# archetipo-inception\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// inceptionRequest is the workspace-scoped request: it carries no spec, because
// the object of an inception is the workspace itself.
func inceptionRequest(command string) execution.Request {
	return execution.Request{
		ExecutionID:    inceptionRunID,
		Action:         execution.ActionInception,
		Capability:     execution.CapabilityWorkspaceInception,
		ProviderConfig: map[string]any{"command": command},
	}
}

func prdReceiptLine(path string) string {
	return fmt.Sprintf(`{"artifact":"prd","status":%q,"path":%q}`, execution.WrittenStatus, path)
}

// startInception dispatches the action on its own goroutine and hands back the
// channel its outcome will arrive on, so the test can act on the conversation
// while Execute is still inside it.
func startInception(provider *Provider, req execution.Request) (<-chan execution.Result, <-chan error) {
	results := make(chan execution.Result, 1)
	failures := make(chan error, 1)
	go func() {
		res, err := provider.Execute(context.Background(), req)
		results <- res
		failures <- err
	}()
	return results, failures
}

// collectEvents replays the history of a run through the collaborator and
// returns what it holds right now. The bounded context is what makes the replay
// return once the history is exhausted instead of following the run forever.
func collectEvents(provider *Provider, runID string, afterID int64) []execution.RunEvent {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var out []execution.RunEvent
	_ = provider.StreamRunEvents(ctx, execution.RunRequest{RunID: runID}, afterID, func(event execution.RunEvent) error {
		out = append(out, event)
		return nil
	})
	return out
}

func lastEventIDOf(provider *Provider, runID string) int64 {
	events := collectEvents(provider, runID, 0)
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].ID
}

func countEvents(events []execution.RunEvent, kind string) int {
	n := 0
	for _, event := range events {
		if event.Kind == kind {
			n++
		}
	}
	return n
}

// lingeringClaude is a process whose standard input can be closed without the
// process itself leaving. It is what a cancellation between two turns really
// meets: closing the input is a request, and the run is over only when the
// output ends. A fake that died on Close would make every "no terminal state
// was deduced from the command" assertion pass for the wrong reason.
type lingeringClaude struct {
	*fakeClaude
	once   sync.Once
	closed chan struct{}
}

var (
	_ localrun.Process = (*lingeringClaude)(nil)
	_ localrun.Starter = (*lingeringClaude)(nil)
)

func newLingeringClaude() *lingeringClaude {
	return &lingeringClaude{fakeClaude: newFakeClaude(), closed: make(chan struct{})}
}

func (l *lingeringClaude) Start(ctx context.Context, dir, name string, args []string) (localrun.Process, error) {
	if _, err := l.fakeClaude.Start(ctx, dir, name, args); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *lingeringClaude) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

// --- AC-2: the conversation stays open and the answer reaches the process ----

// AC-2, AC-3, AC-4 — one whole inception, from the agent's first question to
// the receipt that ends it. It is written as a single test because these are
// not independent facts: that the run survives the first turn is only
// meaningful if the answer then reaches the process and the receipt then closes
// the run, and splitting them would let each half pass while the conversation
// as a whole was broken.
func TestInceptionKeepsTheConversationOpenUntilTheReceipt(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	fake.stderr = "warning: " + sentinel
	provider := newSessionProvider(inceptionWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	results, failures := startInception(provider, inceptionRequest(command))

	// --- the agent asks its question and ends the turn on it -----------------
	const question = "Chi è la persona che userà questo prodotto?"
	<-fake.started
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":` + quoted(question) + `}]}}`)
	fake.emit(resultFrame(question, false))

	// The end of the turn is observable in the history, so the test acts on
	// what really happened rather than on a delay.
	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, inceptionRunID, 0), localrun.KindTurnEnd) == 1
	})
	select {
	case err := <-failures:
		t.Fatalf("Execute returned on the agent's question instead of waiting for the answer: %v", err)
	default:
	}
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: inceptionRunID})
	if err != nil {
		t.Fatalf("ReadRun failed while the conversation was open: %v", err)
	}
	if snapshot.State != execution.RunActive {
		t.Fatalf("the run reported %q after the first turn: a question is not the end of an inception", snapshot.State)
	}
	events := collectEvents(provider, inceptionRunID, 0)
	if countEvents(events, localrun.KindText) != 1 || events[0].Text != question {
		t.Fatalf("the agent's question is not in the history: %#v", events)
	}

	// --- the operator answers ------------------------------------------------
	after := lastEventIDOf(provider, inceptionRunID)
	const answer = "Un product owner che apre il workspace per la prima volta."
	if err := provider.SendRunMessage(context.Background(), execution.RunRequest{RunID: inceptionRunID}, answer); err != nil {
		t.Fatalf("the answer was refused after the turn ended: %v", err)
	}
	if got := fake.messagesReceived(); len(got) != 2 || got[1] != answer {
		t.Fatalf("the process received %v; want the prompt and then exactly the operator's answer", got)
	}
	// Nothing was written locally: a message becomes history when the process
	// re-emits it, never when it is sent.
	if pending := collectEvents(provider, inceptionRunID, after); len(pending) != 0 {
		t.Fatalf("the answer entered the history before the process re-emitted it: %#v", pending)
	}

	fake.emit(userFrame(answer, true))
	waitFor(t, func() bool { return len(collectEvents(provider, inceptionRunID, after)) == 1 })
	replayed := collectEvents(provider, inceptionRunID, after)[0]
	if replayed.Kind != localrun.KindUserMessage || replayed.Text != answer {
		t.Fatalf("the re-emitted answer = %#v", replayed)
	}

	// --- the second turn closes the conversation on the receipt --------------
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"reading ~/.claude/.credentials.json, token=` + sentinel + `"}]}}`)
	fake.emit(resultFrame(prdReceiptLine(prdPath), false))

	if err := <-failures; err != nil {
		t.Fatalf("the receipt did not close the inception: %v", err)
	}
	got := <-results

	// The answer appears once in the whole history and not twice: a client that
	// also wrote it locally would show the operator two copies of one moment.
	if n := countEvents(collectEvents(provider, inceptionRunID, 0), localrun.KindUserMessage); n != 1 {
		t.Fatalf("the operator's answer appears %d time(s) in the history", n)
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
	want := []string{"command", "duration_ms", "exit_code", "model", "prd_path", "result_summary", "turns"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("payload fields = %v, want %v", keys, want)
	}

	var payload struct {
		Command       string `json:"command"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		PRDPath       string `json:"prd_path"`
		Turns         int    `json:"turns"`
		DurationMS    int64  `json:"duration_ms"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != command || payload.ExitCode != 0 || payload.DurationMS != 1500 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.PRDPath != prdPath {
		t.Fatalf("prd_path = %q, want the path the receipt declared", payload.PRDPath)
	}
	if payload.Turns != 2 {
		t.Fatalf("turns = %d, want the two turns the conversation really took", payload.Turns)
	}
	var summary execution.PRDReceipt
	if err := json.Unmarshal([]byte(payload.ResultSummary), &summary); err != nil {
		t.Fatalf("result_summary is not the re-rendered receipt (%s): %v", payload.ResultSummary, err)
	}
	if summary != (execution.PRDReceipt{Artifact: "prd", Status: execution.WrittenStatus, Path: prdPath}) {
		t.Fatalf("result_summary = %#v", summary)
	}
	// The agent's stdout and stderr never enter the record, whatever it printed.
	if strings.Contains(string(got.Payload), sentinel) {
		t.Fatalf("the payload carried the agent output: %s", got.Payload)
	}
	if snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: inceptionRunID}); err != nil || snapshot.State != execution.RunClosed {
		t.Fatalf("the finished run reported %#v (err=%v), want the observed closed state", snapshot, err)
	}
}

// quoted renders a Go string as a JSON string, so a frame written by hand in a
// test cannot be broken by an apostrophe or an accent in the agent's words.
// acceptsPRD is the acceptor executeInception hands to converse: the loop is
// shared by every conversational action, so the tests that exercise it directly
// have to say which closing message ends the conversation they are describing.
func acceptsPRD(message string) bool {
	_, err := execution.AcceptPRDReceipt(message)
	return err == nil
}

func quoted(text string) string {
	payload, err := json.Marshal(text)
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// A turn that ends without a receipt is a question and never a success: an
// inception the operator simply stops answering must not close as done. This is
// the mirror of the planning rule and the reason the two actions do not share
// one flow.
func TestInceptionNeverSucceedsOnATurnWithoutAReceipt(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := newSessionProvider(inceptionWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)
	req := inceptionRequest(command)
	req.ProviderConfig["timeout_seconds"] = 1

	results, failures := startInception(provider, req)
	<-fake.started
	fake.emit(resultFrame("Ho finito, ho scritto tutto quello che serve.", false))

	err := <-failures
	if err == nil {
		t.Fatalf("a conversation without a receipt reported a success: %s", (<-results).Payload)
	}
	if payload := (<-results).Payload; payload != nil {
		t.Fatalf("a failed inception returned a payload: %s", payload)
	}
	assertContains(t, err.Error(), "did not finish the inception within 1s", "execution error")
}

// --- AC-4: the four ways an inception ends without a PRD --------------------

// Each of them must be told apart from the others, and none of them may read as
// a success: a turn the agent closed with an error, a process that left, the
// configured timeout, and a cancellation between two turns are four different
// things to fix.
func TestInceptionFailureModes(t *testing.T) {
	command := fakeCommand(t)
	cases := []struct {
		name    string
		skill   bool
		probe   []runOutcome
		timeout int
		drive   func(fake *fakeClaude)
		wantErr []string
		noEcho  bool
	}{
		{
			name:    "the inception skill is not installed",
			skill:   false,
			probe:   []runOutcome{probeOK},
			wantErr: []string{"inception skill is not installed", inceptionSkillRelPath, "archetipo init --tool claude"},
		},
		{
			name:  "the agent closes a turn with an error",
			skill: true,
			probe: []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				fake.stderr = "claude: the model refused to continue"
				go func() {
					<-fake.started
					fake.emit(resultFrame("Non posso continuare.", true))
				}()
			},
			wantErr: []string{
				"ended the inception on a turn that did not complete",
				"without having produced a PRD",
				"claude: the model refused to continue",
			},
		},
		{
			name:  "the process dies between two turns",
			skill: true,
			probe: []runOutcome{probeOK},
			drive: func(fake *fakeClaude) {
				fake.exitCode = 1
				fake.stderr = strings.Repeat("x", maxCapturedOutput*3) + sentinel
				go func() {
					<-fake.started
					fake.emit(resultFrame("E qual è il primo utente?", false))
					waitForTurnEnd(fake)
					fake.end()
				}()
			},
			wantErr: []string{"exited 1", "without having produced a PRD", "the inception ended without a receipt"},
			noEcho:  true,
		},
		{
			name:    "the conversation runs past the configured timeout",
			skill:   true,
			probe:   []runOutcome{probeOK},
			timeout: 1,
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"sto pensando"}]}}`)
				}()
			},
			wantErr: []string{"did not finish the inception within 1s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.skill {
				dir = inceptionWorkspace(t)
			}
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			if tc.drive != nil {
				tc.drive(fake)
			}
			req := inceptionRequest(command)
			if tc.timeout > 0 {
				req.ProviderConfig["timeout_seconds"] = tc.timeout
			}
			provider := newSessionProvider(dir, &fakeRunner{outcomes: tc.probe}, fake, nil)
			got, err := provider.Execute(context.Background(), req)
			if err == nil {
				t.Fatalf("expected an error, got payload %s", got.Payload)
			}
			if got.Payload != nil {
				t.Fatalf("a failed inception returned a payload: %s", got.Payload)
			}
			var remote *execution.RemoteError
			if errors.As(err, &remote) {
				t.Fatalf("a local run reported a remote unit of work: %v", err)
			}
			for _, want := range tc.wantErr {
				assertContains(t, err.Error(), want, "execution error")
			}
			if tc.noEcho {
				if strings.Contains(err.Error(), sentinel) {
					t.Fatalf("the tail of stderr beyond the limit was echoed: %v", err)
				}
				if len(err.Error()) > maxCapturedOutput*2 {
					t.Fatalf("the diagnostic is %d bytes long: the stream was not bounded", len(err.Error()))
				}
			}
			if !tc.skill {
				return
			}
			snapshot, readErr := provider.ReadRun(context.Background(), execution.RunRequest{RunID: inceptionRunID})
			if readErr != nil {
				t.Fatalf("ReadRun failed after the failure: %v", readErr)
			}
			if snapshot.State != execution.RunCrashed {
				t.Fatalf("state = %q, want the run reported as crashed", snapshot.State)
			}
			if strings.TrimSpace(snapshot.Error) == "" {
				t.Fatal("the run does not say why it ended")
			}
		})
	}
}

// waitForTurnEnd blocks until the client has consumed the frame that ends the
// turn, so a test that kills the process right after a turn cannot race the
// reader and swallow the outcome it just published.
func waitForTurnEnd(fake *fakeClaude) {
	for len(fake.lines) > 0 {
		time.Sleep(time.Millisecond)
	}
}

// AC-4 — a cancellation between two turns closes the input of the process, and
// that is all it does: no terminal state is deduced from the command, and the
// run ends only when the process's output really ends. The alternative — a run
// reported closed the instant the request left — is the lie the criterion
// forbids.
func TestInceptionCancelledBetweenTwoTurnsEndsOnlyWithTheProcess(t *testing.T) {
	command := fakeCommand(t)
	fake := newLingeringClaude()
	t.Cleanup(fake.end)
	provider := New(Options{
		Runner:     &fakeRunner{outcomes: []runOutcome{probeOK}},
		Starter:    fake,
		WorkingDir: func() (string, error) { return inceptionWorkspace(t), nil },
		Now:        fixedElapsedClock(1500 * time.Millisecond),
	})

	results, failures := startInception(provider, inceptionRequest(command))
	<-fake.started
	fake.emit(resultFrame("Quali sono le tre metriche di successo?", false))
	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, inceptionRunID, 0), localrun.KindTurnEnd) == 1
	})

	if err := provider.CancelRun(context.Background(), execution.RunRequest{RunID: inceptionRunID}); err != nil {
		t.Fatalf("the cancellation between two turns was refused: %v", err)
	}
	select {
	case <-fake.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("the cancellation never reached the input of the process")
	}

	// The command decided nothing: the run is still what it was observed to be,
	// and Execute is still inside the conversation.
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: inceptionRunID})
	if err != nil {
		t.Fatalf("ReadRun failed after the cancellation: %v", err)
	}
	if snapshot.State != execution.RunActive {
		t.Fatalf("the run reported %q on the command alone, before the process ended", snapshot.State)
	}
	select {
	case err := <-failures:
		t.Fatalf("Execute returned on the command instead of on the end of the process: %v", err)
	default:
	}

	// The process leaves, and only now is the run over — without a PRD and
	// saying so.
	fake.exitCode = 143
	fake.end()

	err = <-failures
	if err == nil {
		t.Fatalf("a cancelled inception reported a success: %s", (<-results).Payload)
	}
	if payload := (<-results).Payload; payload != nil {
		t.Fatalf("a cancelled inception returned a payload: %s", payload)
	}
	assertContains(t, err.Error(), "without having produced a PRD", "execution error")
	snapshot, err = provider.ReadRun(context.Background(), execution.RunRequest{RunID: inceptionRunID})
	if err != nil {
		t.Fatalf("ReadRun failed after the run: %v", err)
	}
	if snapshot.State != execution.RunCrashed || strings.TrimSpace(snapshot.Error) == "" {
		t.Fatalf("the cancelled run = %#v, want a crashed run that says why", snapshot)
	}
}

// The two actions are dispatched by the action alone, and the mode of the
// session is what separates them: a planning run that ends its turn without a
// receipt fails at once, and it must keep failing at once now that a
// conversation exists. The inception prompt must never be the one a planning
// run receives.
func TestInceptionAndPlanningAreDispatchedByTheActionAlone(t *testing.T) {
	command := fakeCommand(t)
	dir := inceptionWorkspace(t)
	installSkillIn(t, dir)

	// The planning action still fails on the first turn without a receipt.
	planFake := newFakeClaude()
	t.Cleanup(planFake.end)
	go func() {
		<-planFake.started
		planFake.emit(resultFrame("una domanda, non un piano", false))
	}()
	planReq := testRequest(command)
	planReq.ProviderConfig["timeout_seconds"] = 60
	if _, err := newSessionProvider(dir, &fakeRunner{outcomes: []runOutcome{probeOK}}, planFake, nil).Execute(context.Background(), planReq); err == nil {
		t.Fatal("a planning turn without a receipt was accepted: the conversational mode leaked into spec.plan")
	} else {
		assertContains(t, err.Error(), "ended without having produced a plan for "+testSpec, "planning error")
	}
	if got := planFake.messagesReceived(); len(got) != 1 || got[0] != buildPrompt(planReq) {
		t.Fatalf("the planning process received %v; want exactly the planning prompt", got)
	}

	// The inception action receives the inception prompt and holds the turn.
	inceptionFake := newFakeClaude()
	t.Cleanup(inceptionFake.end)
	req := inceptionRequest(command)
	provider := newSessionProvider(dir, &fakeRunner{outcomes: []runOutcome{probeOK}}, inceptionFake, nil)
	_, failures := startInception(provider, req)
	<-inceptionFake.started
	if got := inceptionFake.messagesReceived(); len(got) != 1 || got[0] != buildInceptionPrompt(req) {
		t.Fatalf("the inception process received %v; want exactly the inception prompt", got)
	}
	inceptionFake.emit(resultFrame(prdReceiptLine(prdPath), false))
	if err := <-failures; err != nil {
		t.Fatalf("the inception failed: %v", err)
	}
}

// A receipt published in the very instant the deadline fires is still a
// receipt. `select` picks at random among the cases that are ready, so the end
// of the run and the outcome of the last turn are genuinely concurrent: a
// deadline branch that did not drain the turns would throw the receipt away
// about half the time, close the record FAILED, and let the partial-PRD cleanup
// delete a document that was complete.
//
// The repetition is not a timing trick and nothing here sleeps: both cases are
// ready at every attempt, so a branch that drops the outcome is caught with
// certainty rather than with luck.
func TestConverseKeepsAReceiptPublishedAsTheDeadlineFires(t *testing.T) {
	for attempt := 0; attempt < 64; attempt++ {
		fake := newFakeClaude()
		client := newStreamSession(fake, localrun.NewSession(inceptionRunID, nil), true)
		client.publishTurn(TurnOutcome{Completed: true, Final: prdReceiptLine(prdPath)})

		runCtx, cancel := context.WithCancel(context.Background())
		cancel()

		final, turns, err := converse(runCtx, client, acceptsPRD)
		if err != nil {
			t.Fatalf("attempt %d: the receipt was lost to the deadline: %v", attempt, err)
		}
		receipt, err := execution.AcceptPRDReceipt(final)
		if err != nil || receipt.Path != prdPath || turns != 1 {
			t.Fatalf("attempt %d: final = %q (err=%v) after %d turns", attempt, final, err, turns)
		}
	}
}

// A deadline that fires with nothing published still stops the run, and says so
// with the sentinel the diagnostic is composed from.
func TestConverseStopsOnTheDeadlineWhenNoTurnWasPublished(t *testing.T) {
	fake := newFakeClaude()
	client := newStreamSession(fake, localrun.NewSession(inceptionRunID, nil), true)

	runCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, turns, err := converse(runCtx, client, acceptsPRD)
	if !errors.Is(err, errRunTerminated) {
		t.Fatalf("converse returned %v; want the run to be reported as stopped", err)
	}
	if turns != 0 {
		t.Fatalf("turns = %d on a conversation that never had one", turns)
	}
}

// A turn published without a receipt as the deadline fires is counted and does
// not rescue the run: draining is about not losing what was said, never about
// accepting less than a receipt.
func TestConverseCountsATurnWithoutAReceiptFoundOnTheDeadline(t *testing.T) {
	fake := newFakeClaude()
	client := newStreamSession(fake, localrun.NewSession(inceptionRunID, nil), true)
	client.publishTurn(TurnOutcome{Completed: true, Final: "e di che colore lo vuoi?"})

	runCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, turns, err := converse(runCtx, client, acceptsPRD)
	if err == nil {
		t.Fatal("a conversation stopped without a receipt reported a success")
	}
	if turns != 1 {
		t.Fatalf("turns = %d; want the turn that was drained to be counted", turns)
	}
}
