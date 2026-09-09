package codex

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

type sequenceStarter struct {
	mu        sync.Mutex
	processes []*fakeCodex
}

func (s *sequenceStarter) Start(context.Context, string, string, []string) (localrun.Process, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	process := s.processes[0]
	s.processes = s.processes[1:]
	return process, nil
}

func TestNativeCodexSessionUsesPersistentThreadAcrossTurnsAndResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dir := t.TempDir()
	skillPath := dir + "/.agents/skills/fixture/SKILL.md"
	discoveryFake, firstFake, resumedFake := newFakeCodex(), newFakeCodex(), newFakeCodex()
	discoveryFake.skillPath, firstFake.skillPath, resumedFake.skillPath = skillPath, skillPath, skillPath
	provider := New(Options{
		Runner:     &fakeRunner{outcomes: []runOutcome{probeOK}},
		Starter:    &sequenceStarter{processes: []*fakeCodex{discoveryFake, firstFake, resumedFake}},
		WorkingDir: func() (string, error) { return dir, nil },
	})
	config := map[string]any{"command": fakeCommand(t), "sandbox": "workspace-write"}

	discovery, err := provider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{ProviderConfig: config, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Models) != 1 || discovery.Models[0].ID != "gpt-native" || len(discovery.Skills) != 1 || discovery.Skills[0].Path != skillPath {
		t.Fatalf("runtime discovery = %#v", discovery)
	}
	for _, capability := range []execution.SessionCapability{execution.SessionCapabilityResume, execution.SessionCapabilityInput, execution.SessionCapabilityApproval, execution.SessionCapabilitySkillInvocation} {
		if !execution.SupportsSessionCapability(discovery.Capabilities, capability) {
			t.Fatalf("runtime discovery omitted %s", capability)
		}
	}

	created, err := provider.CreateSession(ctx, execution.CreateSessionRequest{ConversationID: "conv-1", Environment: discovery.Environment})
	if err != nil {
		t.Fatal(err)
	}
	if created.Session.Native.ID != "thread-1" || created.Session.Native.Kind != "codex.thread" {
		t.Fatalf("created session = %#v", created)
	}
	threadParams := firstFake.paramsOf(methodThreadStart)
	if threadParams["ephemeral"] != false || threadParams["approvalPolicy"] != "untrusted" || threadParams["cwd"] != dir {
		t.Fatalf("thread/start params = %#v", threadParams)
	}

	first := startNativeCodexTurn(t, ctx, provider, created.Session, firstFake, "turn-core-1", "submission-1", "primo", "low", discovery.Skills)
	if first.Turn.NativeID != "turn-1" || first.Turn.Applied.Model != "gpt-native" || first.Turn.Applied.Options["effort"] != "low" {
		t.Fatalf("first turn = %#v", first)
	}
	firstFake.emit("item/started", `{"item":{"type":"userMessage","content":[{"type":"text","text":"primo"}]}}`)
	firstFake.completeTurn()
	waitForCodexSession(t, provider, created.Session, func(snapshot execution.SessionSnapshot) bool {
		return snapshot.Work == execution.SessionIdle && snapshot.CurrentTurn.State == execution.TurnCompleted
	})

	second := startNativeCodexTurn(t, ctx, provider, created.Session, firstFake, "turn-core-2", "submission-2", "secondo", "high", nil)
	if second.Turn.NativeID != "turn-2" {
		t.Fatalf("second turn = %#v", second)
	}
	methods := firstFake.methodsCalled()
	if countMethod(methods, methodThreadStart) != 1 || countMethod(methods, methodTurnStart) != 2 {
		t.Fatalf("same connection methods = %v", methods)
	}

	firstFake.request(41, "item/commandExecution/requestApproval", `{"threadId":"thread-1","turnId":"turn-2","itemId":"item-1","reason":"scrivere il file"}`)
	waitForCodexSession(t, provider, created.Session, func(snapshot execution.SessionSnapshot) bool {
		return snapshot.Work == execution.SessionWaitingApproval
	})
	approval, err := provider.RespondSessionApproval(ctx, execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-core-2", SubmissionID: "approval-1", InteractionID: "41", OptionID: localrun.ApprovalAllow})
	if err != nil || approval.State != execution.DeliveryConfirmed {
		t.Fatalf("approval = %#v, %v", approval, err)
	}
	assertFakeResponse(t, firstFake, "41", `{"decision":"accept"}`)

	firstFake.request(42, "item/tool/requestUserInput", `{"threadId":"thread-1","turnId":"turn-2","questions":[{"id":"choice","question":"A o B?"}]}`)
	waitForCodexSession(t, provider, created.Session, func(snapshot execution.SessionSnapshot) bool { return snapshot.Work == execution.SessionWaitingInput })
	inputPayload := json.RawMessage(`{"answers":{"choice":{"answers":["B"]}}}`)
	input, err := provider.RespondSessionInput(ctx, execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-core-2", SubmissionID: "input-1", InteractionID: "42", Payload: inputPayload})
	if err != nil || input.State != execution.DeliveryConfirmed {
		t.Fatalf("input = %#v, %v", input, err)
	}
	assertFakeResponse(t, firstFake, "42", string(inputPayload))

	interrupted, err := provider.InterruptTurn(ctx, execution.SessionCommandRequest{Session: created.Session, TurnID: "turn-core-2", SubmissionID: "interrupt-1"})
	if err != nil || interrupted.State != execution.DeliveryConfirmed {
		t.Fatalf("interrupt = %#v, %v", interrupted, err)
	}
	firstFake.interruptTurn()
	waitForCodexSession(t, provider, created.Session, func(snapshot execution.SessionSnapshot) bool {
		return snapshot.Work == execution.SessionIdle && snapshot.CurrentTurn.State == execution.TurnInterrupted
	})

	if err := provider.ReleaseSession(ctx, execution.SessionRequest{Session: created.Session}); err != nil {
		t.Fatal(err)
	}
	resumed, err := provider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: created.Session})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Session.Native.ID != created.Session.Native.ID || countMethod(resumedFake.methodsCalled(), methodThreadResume) != 1 || countMethod(resumedFake.methodsCalled(), methodThreadStart) != 0 {
		t.Fatalf("resume created another thread: snapshot=%#v methods=%v", resumed, resumedFake.methodsCalled())
	}

	var events []execution.RunEvent
	eventCtx, stopEvents := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- provider.StreamSessionEvents(eventCtx, execution.SessionRequest{Session: created.Session}, 0, func(event execution.RunEvent) error {
			events = append(events, event)
			if len(events) == 3 {
				stopEvents()
			}
			return nil
		})
	}()
	<-done
	if len(events) != 3 || events[0].TurnID != "turn-core-1" || events[0].SubmissionID != "submission-1" || events[1].TurnID != "turn-core-1" || events[2].TurnID != "turn-core-2" {
		t.Fatalf("correlated events = %#v", events)
	}
}

func TestNativeCodexSessionDoesNotTreatProcessDeathAsTurnCompletion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fake := newFakeCodex()
	provider := New(Options{Runner: &fakeRunner{outcomes: []runOutcome{probeOK}}, Starter: fake, WorkingDir: func() (string, error) { return dir, nil }})
	environment := execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: map[string]any{"command": fakeCommand(t)}, Location: "local"}
	created, err := provider.CreateSession(ctx, execution.CreateSessionRequest{ConversationID: "conv-death", Environment: environment})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.StartTurn(ctx, execution.StartTurnRequest{Session: created.Session, TurnID: "turn-death", SubmissionID: "submission-death", Message: "work"}); err != nil {
		t.Fatal(err)
	}
	<-fake.turnStarted
	fake.end()
	waitForCodexSession(t, provider, created.Session, func(snapshot execution.SessionSnapshot) bool {
		return snapshot.CurrentTurn != nil && snapshot.CurrentTurn.State == execution.TurnFailed
	})
}

func startNativeCodexTurn(t *testing.T, ctx context.Context, provider *Provider, session execution.SessionMetadata, fake *fakeCodex, turnID, submissionID, message, effort string, skills []execution.SessionSkill) execution.SessionTurnStarted {
	t.Helper()
	started, err := provider.StartTurn(ctx, execution.StartTurnRequest{Session: session, TurnID: turnID, SubmissionID: submissionID, Message: message, Model: "gpt-native", Options: map[string]string{"effort": effort}, Skills: skills})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.turnStarted:
	case <-ctx.Done():
		t.Fatal("turn/start was not observed")
	}
	return started
}

func waitForCodexSession(t *testing.T, provider *Provider, session execution.SessionMetadata, ready func(execution.SessionSnapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := provider.ReadSession(context.Background(), execution.SessionRequest{Session: session})
		if err == nil && ready(snapshot) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("codex session did not reach the expected state")
}

func countMethod(methods []string, wanted string) int {
	count := 0
	for _, method := range methods {
		if method == wanted {
			count++
		}
	}
	return count
}

func assertFakeResponse(t *testing.T, fake *fakeCodex, id, wanted string) {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, message := range fake.requests {
		if string(message.ID) == id && message.Method == "" && string(message.Result) == wanted {
			return
		}
	}
	t.Fatalf("fake did not receive response id=%s result=%s", id, wanted)
}
