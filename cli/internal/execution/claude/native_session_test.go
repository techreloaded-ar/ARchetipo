package claude

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

type claudeProcessSequence struct {
	mu        sync.Mutex
	processes []*fakeClaude
}

func (s *claudeProcessSequence) Start(ctx context.Context, dir, name string, args []string) (localrun.Process, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.processes) == 0 {
		return nil, context.Canceled
	}
	process := s.processes[0]
	s.processes = s.processes[1:]
	return process.Start(ctx, dir, name, args)
}

func TestNativeClaudeSessionResumesByIDWithPerTurnSettingsAndCorrelations(t *testing.T) {
	dir := t.TempDir()
	command := fakeCommand(t)
	firstProcess := newFakeClaude()
	resumeProbeProcess := newFakeClaude()
	secondProcess := newFakeClaude()
	firstProcess.silent = true
	resumeProbeProcess.silent = true
	secondProcess.silent = true
	processes := &claudeProcessSequence{processes: []*fakeClaude{firstProcess, resumeProbeProcess, secondProcess}}
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: processes, Now: time.Now})

	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{
		ConversationID: "conversation-native-claude",
		Environment:    execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": command, "permission_mode": "auto", "model": "sonnet", "effort": "low"}, Location: "local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.Native.ID == "" || created.Recovery != execution.RecoveryResumable {
		t.Fatalf("created session = %#v", created)
	}

	announceSessionOnFirstMessage(firstProcess, created.Session.Native.ID, "sonnet")
	started, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{
		Session: created.Session, TurnID: "turn-1", SubmissionID: "submission-1",
		Message: "ricorda la parola cedro", Model: "sonnet", Options: map[string]string{"effort": "low"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.Delivery.State != execution.DeliveryConfirmed || started.Turn.Applied.Model != "sonnet" || started.Turn.Applied.Options["effort"] != "low" {
		t.Fatalf("first turn = %#v", started)
	}
	assertArgumentsContainInOrder(t, firstProcess, "--session-id", created.Session.Native.ID, "--model", "sonnet", "--effort", "low")
	firstProcess.emit(userFrame("ricorda la parola cedro", true))
	firstProcess.emit(resultFrame("Memorizzato.", false))
	waitForNativeIdle(t, provider, created.Session)

	if err := provider.ReleaseSession(context.Background(), execution.SessionRequest{Session: created.Session}); err != nil {
		t.Fatal(err)
	}
	restarted := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: processes, Now: time.Now})
	resumed, err := restarted.ResumeSession(context.Background(), execution.ResumeSessionRequest{Session: created.Session})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var events []execution.RunEvent
	done := make(chan struct{})
	go func() {
		_ = restarted.StreamSessionEvents(ctx, execution.SessionRequest{Session: resumed.Session}, 2, func(event execution.RunEvent) error {
			events = append(events, event)
			if len(events) == 2 {
				cancel()
			}
			return nil
		})
		close(done)
	}()
	waitFor(t, func() bool {
		restartedSession := restarted.nativeSession(created.Session.Native.ID)
		restartedSession.mu.Lock()
		defer restartedSession.mu.Unlock()
		return len(restartedSession.subscribers) == 1
	})
	// Changing settings at the turn boundary releases the idle resume process
	// and opens the same native ID with the requested settings.
	announceSessionOnFirstMessage(secondProcess, created.Session.Native.ID, "opus")
	second, err := restarted.StartTurn(context.Background(), execution.StartTurnRequest{
		Session: resumed.Session, TurnID: "turn-2", SubmissionID: "submission-2",
		Message: "quale parola ricordi?", Model: "opus", Options: map[string]string{"effort": "high"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Turn.Applied.Model != "opus" || second.Turn.Applied.Options["effort"] != "high" {
		t.Fatalf("second turn = %#v", second)
	}
	assertArgumentsContainInOrder(t, secondProcess, "--resume", created.Session.Native.ID, "--model", "opus", "--effort", "high")
	secondProcess.emit(userFrame("quale parola ricordi?", true))
	secondProcess.emit(resultFrame("Cedro.", false))
	waitForNativeIdle(t, restarted, resumed.Session)

	<-done
	if len(events) != 2 || events[0].ID != 3 || events[0].TurnID != "turn-2" || events[0].SubmissionID != "submission-2" {
		t.Fatalf("resumed events = %#v", events)
	}
}

func TestNativeClaudeSessionBridgesApprovalAndInterruptWithoutDestroyingTheSession(t *testing.T) {
	dir := t.TempDir()
	command := fakeCommand(t)
	process := newFakeClaude()
	process.silent = true
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: &claudeProcessSequence{processes: []*fakeClaude{process}}, Now: time.Now})
	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{ConversationID: "approval-conversation", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": command}, Location: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	announceSessionOnFirstMessage(process, created.Session.Native.ID, "opus")
	_, err = provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-approval", SubmissionID: "submission-start", Message: "scrivi un file"})
	if err != nil {
		t.Fatal(err)
	}
	process.emit(`{"type":"control_request","request_id":"approval-1","request":{"subtype":"can_use_tool","tool_name":"Write","title":"Write marker","input":{"file_path":"marker.txt"}}}`)
	waitFor(t, func() bool {
		snapshot, _ := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
		return len(snapshot.PendingApprovals) == 1
	})
	process.emit(prefaceTextFrame)
	process.emit(discoveryInputFrame)
	waitFor(t, func() bool {
		snapshot, _ := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
		return len(snapshot.PendingInputs) == 1
	})
	inputDelivery, inputErr := provider.RespondSessionInput(context.Background(), execution.SessionCommandRequest{
		Session: created.Session, TurnID: "turn-approval", SubmissionID: "submission-input", InteractionID: "discovery",
		Answers: map[string][]string{"0": {"Meno errori"}, "1": {"Clienti"}},
	})
	if inputErr != nil || inputDelivery.State != execution.DeliveryConfirmed {
		t.Fatalf("input delivery = %#v, %v", inputDelivery, inputErr)
	}
	delivery, err := provider.RespondSessionApproval(context.Background(), execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-approval", SubmissionID: "submission-approval", InteractionID: "approval-1", OptionID: localrun.ApprovalAllow})
	if err != nil || delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("approval delivery = %#v, %v", delivery, err)
	}
	process.emit(prefaceTextFrame)
	process.emit(discoveryInputFrame)
	waitFor(t, func() bool {
		snapshot, _ := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
		return snapshot.Work == execution.SessionWaitingInput && len(snapshot.PendingInputs) == 1
	})
	delivery, err = provider.InterruptTurn(context.Background(), execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-approval", SubmissionID: "submission-interrupt"})
	if err != nil || delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("interrupt delivery = %#v, %v", delivery, err)
	}
	process.emit(resultFrame("Interrupted", true))
	waitForNativeIdle(t, provider, created.Session)
	snapshot, err := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.Recovery != execution.RecoveryResumable || snapshot.CurrentTurn.State != execution.TurnInterrupted || len(snapshot.PendingInputs) != 0 {
		t.Fatalf("snapshot after interrupt = %#v, %v", snapshot, err)
	}
	// An interrupt ends the turn, not the connection: the next turn is
	// written to the same process.
	_, err = provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-after-interrupt", SubmissionID: "submission-after-interrupt", Message: "continuiamo"})
	if err != nil {
		t.Fatal(err)
	}
	if starts, _, _ := process.spawned(); starts != 1 || !process.alive() {
		t.Fatalf("the turn after the interrupt did not reuse the process: starts=%d alive=%t", starts, process.alive())
	}
	process.emit(userFrame("continuiamo", true))
	process.emit(resultFrame("Continuiamo.", false))
	waitForNativeIdle(t, provider, created.Session)
	snapshot, err = provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.CurrentTurn.ID != "turn-after-interrupt" || snapshot.CurrentTurn.State != execution.TurnCompleted {
		t.Fatalf("snapshot after the second turn = %#v, %v", snapshot, err)
	}
}

func TestNativeClaudeUsesTheCatalogReportedBySystemInitInTheSameSession(t *testing.T) {
	dir := t.TempDir()
	process := newFakeClaude()
	process.silent = true
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: &claudeProcessSequence{processes: []*fakeClaude{process}}, Now: time.Now})
	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{ConversationID: "skill-conversation", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t)}, Location: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := provider.DiscoverSession(context.Background(), execution.SessionDiscoveryRequest{ProviderConfig: created.Session.Environment.ProviderConfig, WorkingDir: dir, Native: &created.Session.Native})
	if err != nil || before.SkillsKnown || len(before.Skills) != 0 {
		t.Fatalf("pre-init discovery must stay unknown: %#v, %v", before, err)
	}
	announceSessionOnFirstMessage(process, created.Session.Native.ID, "opus")
	_, err = provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "warmup", SubmissionID: "warmup-submission", Message: "ciao"})
	if err != nil {
		t.Fatal(err)
	}
	process.emit(userFrame("ciao", true))
	process.emit(resultFrame("Ciao.", false))
	waitForNativeIdle(t, provider, created.Session)
	discovery, err := provider.DiscoverSession(context.Background(), execution.SessionDiscoveryRequest{ProviderConfig: created.Session.Environment.ProviderConfig, WorkingDir: dir, Native: &created.Session.Native})
	if err != nil || !discovery.SkillsKnown || len(discovery.Skills) != 3 || discovery.Skills[0].Name != "claude-only" || discovery.Skills[2].Name != "shared" || discovery.Skills[1].Name != "plugin:fixture" || discovery.Skills[1].Namespace != "plugin" || discovery.Skills[1].Invocation != "/plugin:fixture" {
		t.Fatalf("runtime skill discovery = %#v, %v", discovery, err)
	}
	_, err = provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "skill-turn", SubmissionID: "skill-submission", Message: "esegui", Skills: []execution.SessionSkill{discovery.Skills[2]}})
	if err != nil {
		t.Fatal(err)
	}
	process.mu.Lock()
	last := string(process.sent[len(process.sent)-1])
	process.mu.Unlock()
	if !strings.Contains(last, "/shared\\n\\nesegui") {
		t.Fatalf("native skill invocation missing from %s", last)
	}
}

func announceSessionOnFirstMessage(process *fakeClaude, sessionID, model string) {
	process.onSend(func(line []byte) error {
		process.mu.Lock()
		process.sent = append(process.sent, append(json.RawMessage(nil), line...))
		process.mu.Unlock()
		process.emit(`{"type":"system","subtype":"init","session_id":"` + sessionID + `","model":"` + model + `","skills":["claude-only","shared","plugin:fixture","model-only"],"slash_commands":["claude-only","shared","plugin:fixture"]}`)
		return nil
	})
}

func waitForNativeIdle(t *testing.T, provider *Provider, session execution.SessionMetadata) {
	t.Helper()
	waitFor(t, func() bool {
		snapshot, _ := provider.ReadSession(context.Background(), execution.SessionRequest{Session: session})
		return snapshot.Work == execution.SessionIdle
	})
}

func assertArgumentsContainInOrder(t *testing.T, process *fakeClaude, expected ...string) {
	t.Helper()
	_, _, args := process.spawned()
	position := 0
	for _, argument := range args {
		if position < len(expected) && argument == expected[position] {
			position++
		}
	}
	if position != len(expected) {
		t.Fatalf("arguments %v do not contain %v in order", args, expected)
	}
	if strings.Contains(strings.Join(args, " "), "--continue") || strings.Contains(strings.Join(args, " "), "--no-session-persistence") {
		t.Fatalf("native arguments contain a forbidden generic/non-persistent flag: %v", args)
	}
}

// Discovery must use the connected conversation's process and workspace, not a
// second runtime, and initialize only once without publishing a user turn.
func TestNativeClaudeModelCatalogUsesTheConnectedRuntime(t *testing.T) {
	process := newFakeClaude()
	process.silent = true
	process.modelResponse = `{"models":[{"value":"workspace-model","displayName":"Workspace native name","supportsEffort":true,"supportedEffortLevels":["future-effort"]}]}`
	sequence := &claudeProcessSequence{processes: []*fakeClaude{process}}
	provider := New(Options{Starter: sequence, Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}})
	dir := t.TempDir()
	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{ConversationID: "catalog-session", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t)}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = provider.ReleaseSession(context.Background(), execution.SessionRequest{Session: created.Session})
	}()
	request := execution.SessionDiscoveryRequest{WorkingDir: dir, ProviderConfig: created.Session.Environment.ProviderConfig, Native: &created.Session.Native}
	first, err := provider.DiscoverSession(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Models) != 1 || first.Models[0].ID != "workspace-model" || first.Models[0].Label != "Workspace native name" || first.SkillsKnown {
		t.Fatalf("discovery = %#v", first)
	}
	first.Models[0].Label = "mutated"
	first.Models[0].Options[0].Choices[0].Value = "mutated"
	second, err := provider.DiscoverSession(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Models[0].Label != "Workspace native name" || second.Models[0].Options[0].Choices[0].Value != "future-effort" {
		t.Fatalf("catalog was mutated: %#v", second.Models)
	}
	frames := process.framesReceived()
	if len(frames) != 1 || !strings.Contains(string(frames[0]), `"subtype":"initialize"`) {
		t.Fatalf("unexpected work or reinitialization: %s", frames)
	}
	if !process.alive() || process.startedIn() != dir {
		t.Fatalf("discovery released or moved native runtime: alive=%t dir=%s", process.alive(), process.startedIn())
	}
	snapshot, err := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.CurrentTurn != nil {
		t.Fatalf("discovery started a turn: %#v, %v", snapshot, err)
	}
}

// The end of a turn is not the end of the connection. The process stays
// attached to the session, so an agent started in the background during the
// turn keeps running: what it produces between two turns enters the
// conversation's history, and the next turn with the same settings is written
// to the same process without starting another. What the process emits on its
// own in between — the turn it runs by itself on a task notification, and the
// results that close it — is history too, filed under the turn that ended last,
// and ends no turn of the person: the session stays idle through it.
func TestNativeClaudeKeepsTheProcessBetweenTurnsAndRecordsWhatArrivesInBetween(t *testing.T) {
	dir := t.TempDir()
	process := newFakeClaude()
	process.silent = true
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: &claudeProcessSequence{processes: []*fakeClaude{process}}, Now: time.Now})
	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{ConversationID: "background-conversation", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t)}, Location: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	announceSessionOnFirstMessage(process, created.Session.Native.ID, "opus")
	if _, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-1", SubmissionID: "submission-1", Message: "lancia un agente in background"}); err != nil {
		t.Fatal(err)
	}
	process.emit(userFrame("lancia un agente in background", true))
	process.emit(resultFrame("Lanciato.", false))
	waitForNativeIdle(t, provider, created.Session)
	if !process.alive() {
		t.Fatal("the end of the turn ended the process")
	}
	session := provider.nativeSession(created.Session.Native.ID)
	eventsBefore := len(session.snapshotEvents())

	// Between the two turns: the notification of the background agent, the
	// turn the harness runs by itself on it, and its own result.
	process.emit(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<task-notification>agent finished</task-notification>"}]}}`)
	process.emit(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"L'agente ha finito."}]}}`)
	process.emit(`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":"L'agente ha finito.","origin":{"kind":"task-notification"}}`)
	process.emit(`{"type":"result","subtype":"success","is_error":false,"num_turns":0,"result":"","origin":{"kind":"task-notification"}}`)
	waitFor(t, func() bool { return len(session.snapshotEvents()) == eventsBefore+4 })
	between := session.snapshotEvents()[eventsBefore:]
	for _, event := range between {
		if event.TurnID != "turn-1" || event.SubmissionID != "submission-1" {
			t.Fatalf("an event between two turns lost its turn: %#v", event)
		}
	}
	if between[0].Kind != localrun.KindUserMessage || between[1].Kind != localrun.KindText || between[2].Kind != localrun.KindTurnEnd || between[3].Kind != "result/task-notification" {
		t.Fatalf("the events between two turns = %q, %q, %q, %q", between[0].Kind, between[1].Kind, between[2].Kind, between[3].Kind)
	}
	snapshot, err := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.Work != execution.SessionIdle || snapshot.CurrentTurn.State != execution.TurnCompleted {
		t.Fatalf("snapshot between two turns = %#v, %v", snapshot, err)
	}

	second, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-2", SubmissionID: "submission-2", Message: "che cosa ha trovato?"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("second turn = %#v", second)
	}
	if starts, _, _ := process.spawned(); starts != 1 {
		t.Fatalf("the second turn started another process: starts=%d", starts)
	}
	if messages := process.messagesReceived(); len(messages) != 2 || messages[1] != "che cosa ha trovato?" {
		t.Fatalf("messages written to the process = %q", messages)
	}
	process.emit(userFrame("che cosa ha trovato?", true))
	process.emit(resultFrame("Tre bug.", false))
	waitForNativeIdle(t, provider, created.Session)
	snapshot, err = provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.CurrentTurn.ID != "turn-2" || snapshot.CurrentTurn.State != execution.TurnCompleted {
		t.Fatalf("snapshot after the second turn = %#v, %v", snapshot, err)
	}
	events := session.snapshotEvents()
	last := events[len(events)-1]
	if last.Kind != localrun.KindTurnEnd || last.TurnID != "turn-2" || last.FinalMessage != "Tre bug." {
		t.Fatalf("the end of the second turn = %#v", last)
	}
	if !process.alive() {
		t.Fatal("the end of the second turn ended the process")
	}

	// Releasing the session is what ends the process.
	if err := provider.ReleaseSession(context.Background(), execution.SessionRequest{Session: created.Session}); err != nil {
		t.Fatal(err)
	}
	if process.alive() {
		t.Fatal("releasing the session left the process running")
	}
}

// A process that left while the session was idle — it crashed, or somebody
// killed it — is not written to: the next turn resumes the native session on a
// fresh one, and a turn the process leaves in the middle of ends as
// interrupted, not as completed on the strength of the result before it.
func TestNativeClaudeReplacesAProcessThatLeftBetweenTurns(t *testing.T) {
	dir := t.TempDir()
	process := newFakeClaude()
	nextProcess := newFakeClaude()
	process.silent = true
	nextProcess.silent = true
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: &claudeProcessSequence{processes: []*fakeClaude{process, nextProcess}}, Now: time.Now})
	created, err := provider.CreateSession(context.Background(), execution.CreateSessionRequest{ConversationID: "crash-conversation", Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t)}, Location: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	announceSessionOnFirstMessage(process, created.Session.Native.ID, "opus")
	if _, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-1", SubmissionID: "submission-1", Message: "ciao"}); err != nil {
		t.Fatal(err)
	}
	process.emit(userFrame("ciao", true))
	process.emit(resultFrame("Ciao.", false))
	waitForNativeIdle(t, provider, created.Session)
	process.end()
	session := provider.nativeSession(created.Session.Native.ID)
	waitFor(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return session.client.sessionOver()
	})

	announceSessionOnFirstMessage(nextProcess, created.Session.Native.ID, "opus")
	if _, err := provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-2", SubmissionID: "submission-2", Message: "ci sei?"}); err != nil {
		t.Fatal(err)
	}
	assertArgumentsContainInOrder(t, nextProcess, "--resume", created.Session.Native.ID)
	nextProcess.emit(userFrame("ci sei?", true))
	nextProcess.end()
	waitForNativeIdle(t, provider, created.Session)
	snapshot, err := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.CurrentTurn.ID != "turn-2" || snapshot.CurrentTurn.State != execution.TurnInterrupted {
		t.Fatalf("snapshot after the process left = %#v, %v", snapshot, err)
	}
}

func (s *nativeSession) snapshotEvents() []execution.RunEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]execution.RunEvent(nil), s.events...)
}
