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
	nextProcess := newFakeClaude()
	process.silent = true
	nextProcess.silent = true
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: &claudeProcessSequence{processes: []*fakeClaude{process, nextProcess}}, Now: time.Now})
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
	delivery, err := provider.RespondSessionApproval(context.Background(), execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-approval", SubmissionID: "submission-approval", InteractionID: "approval-1", OptionID: localrun.ApprovalAllow})
	if err != nil || delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("approval delivery = %#v, %v", delivery, err)
	}
	delivery, err = provider.InterruptTurn(context.Background(), execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-approval", SubmissionID: "submission-interrupt"})
	if err != nil || delivery.State != execution.DeliveryConfirmed {
		t.Fatalf("interrupt delivery = %#v, %v", delivery, err)
	}
	process.emit(resultFrame("Interrupted", true))
	waitForNativeIdle(t, provider, created.Session)
	snapshot, err := provider.ReadSession(context.Background(), execution.SessionRequest{Session: created.Session})
	if err != nil || snapshot.Recovery != execution.RecoveryResumable || snapshot.CurrentTurn.State != execution.TurnInterrupted {
		t.Fatalf("snapshot after interrupt = %#v, %v", snapshot, err)
	}
	announceSessionOnFirstMessage(nextProcess, created.Session.Native.ID, "opus")
	_, err = provider.StartTurn(context.Background(), execution.StartTurnRequest{Session: created.Session, TurnID: "turn-after-interrupt", SubmissionID: "submission-after-interrupt", Message: "continuiamo"})
	if err != nil {
		t.Fatal(err)
	}
	assertArgumentsContainInOrder(t, nextProcess, "--resume", created.Session.Native.ID)
	nextProcess.emit(userFrame("continuiamo", true))
	nextProcess.emit(resultFrame("Continuiamo.", false))
	waitForNativeIdle(t, provider, created.Session)
}

func announceSessionOnFirstMessage(process *fakeClaude, sessionID, model string) {
	process.onSend(func(line []byte) error {
		process.mu.Lock()
		process.sent = append(process.sent, append(json.RawMessage(nil), line...))
		process.mu.Unlock()
		process.emit(`{"type":"system","subtype":"init","session_id":"` + sessionID + `","model":"` + model + `"}`)
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
