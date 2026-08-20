package claude

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// Proposing a spec is the third conversational action of this provider, and
// these tests drive it through the same seam the inception and backlog ones
// use: the process is a double, so what is asserted is the frames really
// exchanged with it, and no machine that has Claude Code on it is needed to
// prove any of it.
const specDraftRunID = "exec-1"

// specDraftRequest is the workspace-scoped request: it carries no spec, because
// the object of a proposal is the workspace — the spec is what is being
// proposed for it, and it does not exist yet.
func specDraftRequest(command string) execution.Request {
	return execution.Request{
		ExecutionID:    specDraftRunID,
		Action:         execution.ActionSpecDraft,
		Capability:     execution.CapabilityWorkspaceSpecDraft,
		ProviderConfig: map[string]any{"command": command},
	}
}

// proposedSpec is the proposal the fake agent closes on. The body is genuinely
// multi-line, because that is the one shape the single-line receipt could
// plausibly damage on the way to the record.
func proposedSpec() execution.SpecDraftReceipt {
	return execution.SpecDraftReceipt{
		Artifact: "spec_draft",
		Status:   execution.ProposedStatus,
		Title:    "Esportare il backlog in CSV",
		EpicCode: "EP-005",
		Priority: "MEDIUM",
		Points:   3,
		Scope:    "MVP",
		BlockedBy: []string{
			"US-001",
		},
		Body: strings.Join([]string{
			"**User Story**",
			"Come analista, voglio esportare il backlog in CSV.",
			"",
			"**Criteri di accettazione**",
			"- [ ] AC-1 — L'esportazione produce un file `backlog.csv`.",
		}, "\n"),
	}
}

func specDraftReceiptLine(t *testing.T, receipt execution.SpecDraftReceipt) string {
	t.Helper()
	line, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("encoding the fixture receipt: %v", err)
	}
	return string(line)
}

// startSpecDraft dispatches the action on its own goroutine and hands back the
// channels its outcome will arrive on, so the test can act on the conversation
// while Execute is still inside it.
func startSpecDraft(provider *Provider, req execution.Request) (<-chan execution.Result, <-chan error) {
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
// receipt the shared acceptance gate recognizes.
//
// It is also the only prompt of this package that must forbid a persistence,
// and that prohibition is asserted here rather than trusted, because it is the
// first of the three guards that keep an unconfirmed spec out of the backlog.
func TestBuildSpecDraftPromptForbidsWritingAndAsksForTheSharedReceipt(t *testing.T) {
	req := specDraftRequest("claude")
	first := buildSpecDraftPrompt(req)
	if second := buildSpecDraftPrompt(req); first != second {
		t.Fatalf("the spec draft prompt is not deterministic:\n%s\n---\n%s", first, second)
	}
	assertContains(t, first, "/archetipo-spec", "spec draft prompt")
	assertContains(t, first, "Do NOT persist anything", "spec draft prompt")
	assertContains(t, first, "must not run `archetipo spec add`", "spec draft prompt")
	assertContains(t, first, "a single question per message", "spec draft prompt")
	assertContains(t, first, `"status":"`+execution.ProposedStatus+`"`, "spec draft prompt")
	assertContains(t, first, `"epic_code"`, "spec draft prompt")
	assertContains(t, first, `"body"`, "spec draft prompt")

	// The receipt shape asked for is the one the shared gate accepts: the two
	// must never be able to drift.
	if _, err := execution.AcceptSpecDraftReceipt(specDraftReceiptLine(t, proposedSpec())); err != nil {
		t.Fatalf("the receipt the prompt asks for is not the one the gate accepts: %v", err)
	}
	// A proposal must never be asked to plan, to write a PRD, or to generate a
	// whole backlog.
	for _, forbidden := range []string{"/archetipo-plan", "/archetipo-inception", "archetipo prd write"} {
		if strings.Contains(first, forbidden) {
			t.Fatalf("the spec draft prompt asks for %q:\n%s", forbidden, first)
		}
	}
}

// --- the guard --------------------------------------------------------------

// A workspace without the spec skill cannot run the conversation at all, and
// the refusal must come before anything is spawned: a process started here
// would burn the whole timeout to fail on an unknown command.
func TestSpecDraftRefusesBeforeSpawningWhenTheSkillIsMissing(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := newSessionProvider(t.TempDir(), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	got, err := provider.Execute(context.Background(), specDraftRequest(command))
	if err == nil {
		t.Fatalf("a workspace without the spec skill was accepted: %s", got.Payload)
	}
	if got.Payload != nil {
		t.Fatalf("a failed proposal returned a payload: %s", got.Payload)
	}
	if starts, _, _ := fake.spawned(); starts != 0 {
		t.Fatalf("the process was started %d time(s) before the missing skill was noticed", starts)
	}
	assertContains(t, err.Error(), "spec skill is not installed", "execution error")
	assertContains(t, err.Error(), backlogSkillRelPath, "execution error")
	assertContains(t, err.Error(), "archetipo init --tool claude", "execution error")
}

// --- AC-2 and AC-3: the conversation, the answer, and the proposal ----------

// One whole proposal, from the agent's question to the receipt that ends it.
//
// It is one test because these are not independent facts: that the run survives
// a turn without a receipt only matters if the operator's answer then reaches
// the process and the receipt closes the conversation on a proposal that is
// complete. Splitting them would let each half pass while the conversation as a
// whole was broken.
func TestSpecDraftKeepsTheConversationOpenUntilTheProposal(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	fake.stderr = "warning: " + sentinel
	provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	req := specDraftRequest(command)
	results, failures := startSpecDraft(provider, req)

	// --- the instruction really delivered to the process --------------------
	<-fake.started
	if got := fake.messagesReceived(); len(got) != 1 || got[0] != buildSpecDraftPrompt(req) {
		t.Fatalf("the process received %v; want exactly the spec draft prompt", got)
	}

	// --- the agent asks its question and ends the turn on it ----------------
	const question = "L'esportazione riguarda tutto il backlog o solo le spec di un'epica?"
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":` + quoted(question) + `}]}}`)
	fake.emit(resultFrame(question, false))

	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, specDraftRunID, 0), localrun.KindTurnEnd) == 1
	})
	select {
	case err := <-failures:
		t.Fatalf("Execute returned on the agent's question instead of waiting for the answer: %v", err)
	default:
	}
	snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: specDraftRunID})
	if err != nil {
		t.Fatalf("ReadRun failed while the conversation was open: %v", err)
	}
	if snapshot.State != execution.RunActive {
		t.Fatalf("the run reported %q after the first turn: a question is not the end of a proposal", snapshot.State)
	}

	// --- the operator answers ------------------------------------------------
	after := lastEventIDOf(provider, specDraftRunID)
	const answer = "Tutto il backlog, senza filtri."
	if err := provider.SendRunMessage(context.Background(), execution.RunRequest{RunID: specDraftRunID}, answer); err != nil {
		t.Fatalf("the answer was refused after the turn ended: %v", err)
	}
	if got := fake.messagesReceived(); len(got) != 2 || got[1] != answer {
		t.Fatalf("the process received %v; want the prompt and then exactly the operator's answer", got)
	}
	// Nothing was written locally: a message becomes history when the process
	// re-emits it, never when it is sent.
	if pending := collectEvents(provider, specDraftRunID, after); len(pending) != 0 {
		t.Fatalf("the answer entered the history before the process re-emitted it: %#v", pending)
	}
	fake.emit(userFrame(answer, true))
	waitFor(t, func() bool { return len(collectEvents(provider, specDraftRunID, after)) == 1 })

	// --- the second turn closes the conversation on the receipt -------------
	want := proposedSpec()
	fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"ecco la proposta, token=` + sentinel + `"}]}}`)
	fake.emit(resultFrame("Ecco la spec che propongo.\n"+specDraftReceiptLine(t, want), false))

	if err := <-failures; err != nil {
		t.Fatalf("the receipt did not close the proposal: %v", err)
	}
	got := <-results

	// The answer appears once in the whole history and not twice.
	if n := countEvents(collectEvents(provider, specDraftRunID, 0), localrun.KindUserMessage); n != 1 {
		t.Fatalf("the operator's answer appears %d time(s) in the history", n)
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
	wantKeys := []string{"command", "duration_ms", "exit_code", "model", "result_summary", "spec_draft", "turns"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("payload fields = %v, want %v", keys, wantKeys)
	}

	var payload struct {
		Command       string           `json:"command"`
		ExitCode      int              `json:"exit_code"`
		ResultSummary string           `json:"result_summary"`
		SpecDraft     specDraftPayload `json:"spec_draft"`
		Turns         int              `json:"turns"`
		DurationMS    int64            `json:"duration_ms"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != command || payload.ExitCode != 0 || payload.DurationMS != 1500 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Turns != 2 {
		t.Fatalf("turns = %d, want the two turns the conversation really took", payload.Turns)
	}

	// --- the proposal is carried whole -------------------------------------
	//
	// It is the only outcome of this action: nothing was written anywhere, so a
	// field dropped here is a field the person reviewing the proposal will
	// never see.
	wantDraft := specDraftPayload{
		Title:     want.Title,
		EpicCode:  want.EpicCode,
		Priority:  want.Priority,
		Points:    want.Points,
		Scope:     want.Scope,
		BlockedBy: want.BlockedBy,
		Body:      want.Body,
	}
	if !reflect.DeepEqual(payload.SpecDraft, wantDraft) {
		t.Fatalf("spec_draft = %#v, want %#v", payload.SpecDraft, wantDraft)
	}
	if !strings.Contains(payload.SpecDraft.Body, "\n") {
		t.Fatalf("the markdown body lost its line breaks: %q", payload.SpecDraft.Body)
	}

	// --- result_summary is the receipt alone, re-rendered -------------------
	rendered, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ResultSummary != string(rendered) {
		t.Fatalf("result_summary = %q, want the re-rendered receipt %q", payload.ResultSummary, rendered)
	}
	if strings.Contains(payload.ResultSummary, "Ecco la spec che propongo.") {
		t.Fatalf("result_summary carried what the agent printed around the receipt: %q", payload.ResultSummary)
	}
	if strings.Contains(string(got.Payload), sentinel) {
		t.Fatalf("the payload carried the agent output: %s", got.Payload)
	}
	if snapshot, err := provider.ReadRun(context.Background(), execution.RunRequest{RunID: specDraftRunID}); err != nil || snapshot.State != execution.RunClosed {
		t.Fatalf("the finished run reported %#v (err=%v), want the observed closed state", snapshot, err)
	}
}

// A proposal without blockers must reach the record as an empty list and never
// as a null: the form that renders it joins the values, and a null would make a
// well-formed proposal unreadable for a reason that has nothing to do with it.
func TestSpecDraftCarriesAnEmptyBlockedByAsAList(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	receipt := proposedSpec()
	receipt.BlockedBy = nil
	results, failures := startSpecDraft(provider, specDraftRequest(command))
	<-fake.started
	fake.emit(resultFrame(specDraftReceiptLine(t, receipt), false))

	if err := <-failures; err != nil {
		t.Fatalf("the receipt did not close the proposal: %v", err)
	}
	if !strings.Contains(string((<-results).Payload), `"blocked_by":[]`) {
		t.Fatal("blocked_by was not rendered as an empty list")
	}
}

// A receipt published in the very instant the process leaves is still a
// receipt: the buffered outcome is drained before the death of the process is
// treated as the end of the run.
func TestSpecDraftSucceedsWhenTheReceiptAndTheEndOfTheProcessCoincide(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	results, failures := startSpecDraft(provider, specDraftRequest(command))
	<-fake.started
	fake.emit(resultFrame(specDraftReceiptLine(t, proposedSpec()), false))
	fake.end()

	if err := <-failures; err != nil {
		t.Fatalf("a receipt published as the process left was thrown away: %v", err)
	}
	var payload struct {
		SpecDraft specDraftPayload `json:"spec_draft"`
	}
	if err := json.Unmarshal((<-results).Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SpecDraft.Title != proposedSpec().Title {
		t.Fatalf("payload = %#v, want the proposal the receipt declared", payload)
	}
}

// A conversation that closes on a receipt for another artifact has not proposed
// anything: the run fails and produces no payload.
func TestSpecDraftRefusesAReceiptForAnotherArtifact(t *testing.T) {
	command := fakeCommand(t)
	fake := newFakeClaude()
	t.Cleanup(fake.end)
	fake.exitCode = 0
	provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

	results, failures := startSpecDraft(provider, specDraftRequest(command))
	<-fake.started
	fake.emit(resultFrame(`{"artifact":"backlog","status":"WRITTEN","epics":2,"specs":3}`, false))
	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, specDraftRunID, 0), localrun.KindTurnEnd) == 1
	})
	fake.end()

	err := <-failures
	if err == nil {
		t.Fatalf("a receipt for another artifact closed the proposal: %s", (<-results).Payload)
	}
	if payload := (<-results).Payload; payload != nil {
		t.Fatalf("a failed proposal returned a payload: %s", payload)
	}
	assertContains(t, err.Error(), "without having proposed a spec", "execution error")
}

// --- the ways a proposal ends without a proposal ----------------------------

// Each of them must be told apart from the others, none may read as a success,
// and every one names the proposal: a diagnostic that talked about the backlog,
// the PRD or the planning would send whoever reads it to the wrong place.
func TestSpecDraftFailureModes(t *testing.T) {
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
				"ended the spec proposal on a turn that did not complete",
				"without having proposed a spec",
				"claude: the model refused to continue",
			},
		},
		{
			name: "the process leaves between two turns",
			drive: func(fake *fakeClaude) {
				fake.exitCode = 1
				go func() {
					<-fake.started
					fake.emit(resultFrame("Sotto quale epica la metto?", false))
					waitForTurnEnd(fake)
					fake.end()
				}()
			},
			wantErr: []string{"exited 1", "without having proposed a spec", "the conversation ended without a receipt"},
		},
		{
			name:    "the conversation runs past the configured timeout",
			timeout: 1,
			drive: func(fake *fakeClaude) {
				go func() {
					<-fake.started
					fake.emit(`{"type":"assistant","message":{"content":[{"type":"text","text":"sto leggendo il backlog"}]}}`)
				}()
			},
			wantErr: []string{"did not finish the spec proposal within 1s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClaude()
			t.Cleanup(fake.end)
			if tc.drive != nil {
				tc.drive(fake)
			}
			req := specDraftRequest(command)
			if tc.timeout > 0 {
				req.ProviderConfig["timeout_seconds"] = tc.timeout
			}
			provider := newSessionProvider(backlogWorkspace(t), &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil)

			got, err := provider.Execute(context.Background(), req)
			if err == nil {
				t.Fatalf("expected an error, got payload %s", got.Payload)
			}
			if got.Payload != nil {
				t.Fatalf("a failed proposal returned a payload: %s", got.Payload)
			}
			var remote *execution.RemoteError
			if errors.As(err, &remote) {
				t.Fatalf("a local run reported a remote unit of work: %v", err)
			}
			for _, want := range tc.wantErr {
				assertContains(t, err.Error(), want, "execution error")
			}
			for _, forbidden := range []string{"PRD", "planning", "backlog generation"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("the spec draft diagnostic talks about %q: %v", forbidden, err)
				}
			}
			snapshot, readErr := provider.ReadRun(context.Background(), execution.RunRequest{RunID: specDraftRunID})
			if readErr != nil {
				t.Fatalf("ReadRun failed after the failure: %v", readErr)
			}
			if snapshot.State != execution.RunCrashed || strings.TrimSpace(snapshot.Error) == "" {
				t.Fatalf("the failed run = %#v, want a crashed run that says why", snapshot)
			}
		})
	}
}

// A run stopped between two turns is the fourth cause, and it is not the
// timeout: the input of the process is closed, and the run ends only when the
// process's output really ends. It is the cancellation a person performs by
// closing the form, so it must never read as a success.
func TestSpecDraftStoppedBetweenTwoTurnsEndsOnlyWithTheProcess(t *testing.T) {
	command := fakeCommand(t)
	fake := newLingeringClaude()
	t.Cleanup(fake.end)
	provider := New(Options{
		Runner:     &fakeRunner{outcomes: []runOutcome{probeOK}},
		Starter:    fake,
		WorkingDir: func() (string, error) { return backlogWorkspace(t), nil },
		Now:        fixedElapsedClock(1500 * time.Millisecond),
	})

	results, failures := startSpecDraft(provider, specDraftRequest(command))
	<-fake.started
	fake.emit(resultFrame("Quanti punti le assegno?", false))
	waitFor(t, func() bool {
		return countEvents(collectEvents(provider, specDraftRunID, 0), localrun.KindTurnEnd) == 1
	})

	if err := provider.CancelRun(context.Background(), execution.RunRequest{RunID: specDraftRunID}); err != nil {
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
		t.Fatalf("a stopped proposal reported a success: %s", (<-results).Payload)
	}
	if payload := (<-results).Payload; payload != nil {
		t.Fatalf("a stopped proposal returned a payload: %s", payload)
	}
	assertContains(t, err.Error(), "without having proposed a spec", "execution error")
}
