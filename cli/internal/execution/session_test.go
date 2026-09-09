package execution_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/arcipelago"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/codex"
)

type fakeNativeSession struct {
	snapshot execution.SessionSnapshot
	events   []execution.RunEvent
}

type fakeNativeStore struct {
	nextID   int
	sessions map[string]*fakeNativeSession
}

type fakeSessionProvider struct {
	store *fakeNativeStore
}

func newFakeSessionProvider(store *fakeNativeStore) *fakeSessionProvider {
	if store == nil {
		store = &fakeNativeStore{sessions: make(map[string]*fakeNativeSession)}
	}
	return &fakeSessionProvider{store: store}
}

func (p *fakeSessionProvider) ID() string { return "fake-native" }

func (p *fakeSessionProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return nil, nil
}

func (p *fakeSessionProvider) ValidateConfig(context.Context, map[string]any) error { return nil }

func (p *fakeSessionProvider) Execute(context.Context, execution.Request) (execution.Result, error) {
	return execution.Result{}, fmt.Errorf("the native-session fake executes no process action")
}

func (p *fakeSessionProvider) DiscoverSession(_ context.Context, req execution.SessionDiscoveryRequest) (execution.SessionDiscovery, error) {
	return execution.SessionDiscovery{
		Capabilities: []execution.SessionCapability{
			execution.SessionCapabilityResume,
			execution.SessionCapabilityTurnModel,
			execution.SessionCapabilityTurnOptions,
			execution.SessionCapabilitySkillDiscovery,
			execution.SessionCapabilitySkillInvocation,
			execution.SessionCapabilityInput,
			execution.SessionCapabilityApproval,
			execution.SessionCapabilitySteering,
			execution.SessionCapabilityInterrupt,
		},
		Models: []execution.ModelOption{{ID: "fake-model", Default: true}},
		Skills: []execution.SessionSkill{{Name: "fake-skill", Path: "/runtime/fake-skill/SKILL.md"}},
		Environment: execution.SessionEnvironment{
			WorkingDir:     req.WorkingDir,
			ProviderConfig: execution.CloneConfig(req.ProviderConfig),
			Location:       "local",
		},
	}, nil
}

func (p *fakeSessionProvider) CreateSession(_ context.Context, req execution.CreateSessionRequest) (execution.SessionSnapshot, error) {
	p.store.nextID++
	nativeID := fmt.Sprintf("native-%d", p.store.nextID)
	snapshot := execution.SessionSnapshot{
		Session: execution.SessionMetadata{
			ConversationID: req.ConversationID,
			ProviderID:     p.ID(),
			Environment:    req.Environment,
			Native:         execution.NativeSessionReference{Kind: "fake.thread", ID: nativeID},
		},
		Archive:          execution.ArchiveOpen,
		Connection:       execution.ConnectionConnected,
		ConnectionID:     "connection-create",
		Recovery:         execution.RecoveryResumable,
		Work:             execution.SessionIdle,
		PendingInputs:    []execution.PendingInput{},
		PendingApprovals: []execution.PendingApproval{},
	}
	p.store.sessions[nativeID] = &fakeNativeSession{snapshot: snapshot}
	return snapshot, nil
}

func (p *fakeSessionProvider) ResumeSession(_ context.Context, req execution.ResumeSessionRequest) (execution.SessionSnapshot, error) {
	session, err := p.session(req.Session)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	session.snapshot.Connection = execution.ConnectionConnected
	session.snapshot.ConnectionID = "connection-resume"
	return session.snapshot, nil
}

func (p *fakeSessionProvider) ReadSession(_ context.Context, req execution.SessionRequest) (execution.SessionSnapshot, error) {
	session, err := p.session(req.Session)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	return session.snapshot, nil
}

func (p *fakeSessionProvider) StartTurn(_ context.Context, req execution.StartTurnRequest) (execution.SessionTurnStarted, error) {
	session, err := p.session(req.Session)
	if err != nil {
		return execution.SessionTurnStarted{}, err
	}
	if session.snapshot.Connection != execution.ConnectionConnected || session.snapshot.Work != execution.SessionIdle {
		return execution.SessionTurnStarted{}, fmt.Errorf("session is not ready for a new turn")
	}
	turn := execution.SessionTurn{
		ID:          req.TurnID,
		NativeID:    "native-" + req.TurnID,
		ExecutionID: req.ExecutionID,
		State:       execution.TurnActive,
		Requested:   execution.TurnConfiguration{Model: req.Model, Options: req.Options},
		Applied:     execution.TurnConfiguration{Model: req.Model, Options: req.Options},
		StartedAt:   time.Unix(int64(len(session.events)+1), 0).UTC(),
	}
	session.snapshot.Work = execution.SessionTurnActive
	session.snapshot.CurrentTurn = &turn
	session.events = append(session.events, execution.RunEvent{
		ID:           int64(len(session.events) + 1),
		Kind:         "user_message",
		Text:         req.Message,
		TurnID:       req.TurnID,
		SubmissionID: req.SubmissionID,
	})
	return execution.SessionTurnStarted{
		Turn: turn,
		Delivery: execution.SessionDelivery{
			SubmissionID:     req.SubmissionID,
			TurnID:           req.TurnID,
			State:            execution.DeliveryConfirmed,
			NativeDeliveryID: "native-" + req.SubmissionID,
		},
	}, nil
}

func (p *fakeSessionProvider) completeTurn(sessionMetadata execution.SessionMetadata) {
	session, _ := p.session(sessionMetadata)
	completedAt := time.Unix(int64(len(session.events)+2), 0).UTC()
	session.snapshot.CurrentTurn.State = execution.TurnCompleted
	session.snapshot.CurrentTurn.CompletedAt = &completedAt
	session.snapshot.Work = execution.SessionIdle
}

func (p *fakeSessionProvider) StreamSessionEvents(_ context.Context, req execution.SessionRequest, afterID int64, sink func(execution.RunEvent) error) error {
	session, err := p.session(req.Session)
	if err != nil {
		return err
	}
	for _, event := range session.events {
		if event.ID > afterID {
			if err := sink(event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *fakeSessionProvider) SteerTurn(_ context.Context, req execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.confirm(req), nil
}

func (p *fakeSessionProvider) RespondSessionInput(_ context.Context, req execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.confirm(req), nil
}

func (p *fakeSessionProvider) RespondSessionApproval(_ context.Context, req execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.confirm(req), nil
}

func (p *fakeSessionProvider) InterruptTurn(_ context.Context, req execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	session, err := p.session(req.Session)
	if err != nil {
		return execution.SessionDelivery{}, err
	}
	if session.snapshot.CurrentTurn == nil || session.snapshot.CurrentTurn.ID != req.TurnID {
		return execution.SessionDelivery{}, fmt.Errorf("turn is not active")
	}
	completedAt := time.Unix(int64(len(session.events)+2), 0).UTC()
	session.snapshot.CurrentTurn.State = execution.TurnInterrupted
	session.snapshot.CurrentTurn.CompletedAt = &completedAt
	session.snapshot.Work = execution.SessionIdle
	return p.confirm(req), nil
}

func (p *fakeSessionProvider) ReleaseSession(_ context.Context, req execution.SessionRequest) error {
	session, err := p.session(req.Session)
	if err != nil {
		return err
	}
	session.snapshot.Connection = execution.ConnectionReleased
	session.snapshot.ConnectionID = ""
	return nil
}

func (p *fakeSessionProvider) session(metadata execution.SessionMetadata) (*fakeNativeSession, error) {
	session, ok := p.store.sessions[metadata.Native.ID]
	if !ok {
		return nil, fmt.Errorf("native session %q does not exist", metadata.Native.ID)
	}
	return session, nil
}

func (p *fakeSessionProvider) confirm(req execution.SessionCommandRequest) execution.SessionDelivery {
	return execution.SessionDelivery{SubmissionID: req.SubmissionID, TurnID: req.TurnID, State: execution.DeliveryConfirmed}
}

func TestNativeSessionContractCreatesTwoTurnsInterruptsAndResumes(t *testing.T) {
	ctx := context.Background()
	provider := newFakeSessionProvider(nil)
	sessions, supported := execution.SessionProviderFor(provider)
	if !supported {
		t.Fatal("the fake does not satisfy SessionProvider")
	}

	discovery, err := sessions.DiscoverSession(ctx, execution.SessionDiscoveryRequest{WorkingDir: "/workspace"})
	if err != nil {
		t.Fatalf("DiscoverSession failed: %v", err)
	}
	for _, capability := range []execution.SessionCapability{
		execution.SessionCapabilityResume,
		execution.SessionCapabilityTurnModel,
		execution.SessionCapabilityTurnOptions,
		execution.SessionCapabilitySkillDiscovery,
		execution.SessionCapabilitySkillInvocation,
		execution.SessionCapabilityInput,
		execution.SessionCapabilityApproval,
		execution.SessionCapabilitySteering,
		execution.SessionCapabilityInterrupt,
	} {
		if !execution.SupportsSessionCapability(discovery.Capabilities, capability) {
			t.Fatalf("discovery does not declare %q", capability)
		}
	}

	created, err := sessions.CreateSession(ctx, execution.CreateSessionRequest{
		ConversationID: "conversation-1",
		Environment: execution.SessionEnvironment{
			WorkingDir: "/workspace",
			Location:   "local",
		},
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if created.Session.Native.ID == created.Session.ConversationID {
		t.Fatal("the native reference was conflated with the conversation id")
	}

	first, err := sessions.StartTurn(ctx, execution.StartTurnRequest{
		Session:      created.Session,
		TurnID:       "turn-1",
		SubmissionID: "submission-1",
		Message:      "first",
		Model:        "fake-model",
		Options:      map[string]string{"effort": "low"},
		Skills:       []execution.SessionSkill{{Name: "fake-skill"}},
	})
	if err != nil {
		t.Fatalf("first StartTurn failed: %v", err)
	}
	if first.Delivery.State != execution.DeliveryConfirmed || first.Turn.Applied.Options["effort"] != "low" {
		t.Fatalf("first turn did not record delivery and applied settings: %#v", first)
	}
	provider.completeTurn(created.Session)

	second, err := sessions.StartTurn(ctx, execution.StartTurnRequest{
		Session:      created.Session,
		TurnID:       "turn-2",
		SubmissionID: "submission-2",
		ExecutionID:  "execution-7",
		Message:      "second",
		Model:        "fake-model",
		Options:      map[string]string{"effort": "high"},
	})
	if err != nil {
		t.Fatalf("second StartTurn failed: %v", err)
	}
	if second.Turn.ExecutionID != "execution-7" || second.Turn.ID == second.Turn.ExecutionID {
		t.Fatalf("turn and execution identities were conflated: %#v", second.Turn)
	}

	interrupted, err := sessions.InterruptTurn(ctx, execution.SessionCommandRequest{
		Session:      created.Session,
		TurnID:       second.Turn.ID,
		SubmissionID: "submission-interrupt",
	})
	if err != nil || interrupted.State != execution.DeliveryConfirmed {
		t.Fatalf("InterruptTurn = %#v, %v", interrupted, err)
	}
	current, err := sessions.ReadSession(ctx, execution.SessionRequest{Session: created.Session})
	if err != nil || current.Work != execution.SessionIdle || current.CurrentTurn.State != execution.TurnInterrupted {
		t.Fatalf("snapshot after interrupt = %#v, %v", current, err)
	}

	if err := sessions.ReleaseSession(ctx, execution.SessionRequest{Session: created.Session}); err != nil {
		t.Fatalf("ReleaseSession failed: %v", err)
	}
	restartedProvider := newFakeSessionProvider(provider.store)
	resumed, err := restartedProvider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: created.Session})
	if err != nil {
		t.Fatalf("ResumeSession after provider restart failed: %v", err)
	}
	if resumed.Session.Native.ID != created.Session.Native.ID || resumed.ConnectionID == "connection-create" {
		t.Fatalf("resume did not preserve the native session with a new connection: %#v", resumed)
	}

	var events []execution.RunEvent
	err = restartedProvider.StreamSessionEvents(ctx, execution.SessionRequest{Session: resumed.Session}, 0, func(event execution.RunEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil || len(events) != 2 {
		t.Fatalf("resumed event history has %d events (err=%v), want two", len(events), err)
	}
	if events[0].TurnID != "turn-1" || events[0].SubmissionID != "submission-1" || events[1].TurnID != "turn-2" {
		t.Fatalf("events lost turn/submission correlation: %#v", events)
	}
}

func TestIncompleteAdaptersDoNotExposeNativeSessions(t *testing.T) {
	providers := []execution.Provider{
		arcipelago.New(arcipelago.Options{}),
	}
	for _, provider := range providers {
		if _, supported := execution.SessionProviderFor(provider); supported {
			t.Fatalf("incomplete adapter %q exposes native session capabilities", provider.ID())
		}
	}
}

func TestCompletedLocalAdaptersExposeNativeSessions(t *testing.T) {
	if _, supported := execution.SessionProviderFor(codex.New(codex.Options{})); !supported {
		t.Fatal("codex does not expose native session capabilities")
	}
}

func TestDeliveryStateDistinguishesSafeRetryFromUncertainDelivery(t *testing.T) {
	if execution.DeliveryUnsent == execution.DeliveryUncertain || execution.DeliveryConfirmed == execution.DeliveryUncertain {
		t.Fatal("uncertain delivery must be distinguishable from both safe retry and confirmed delivery")
	}
}

var _ execution.Provider = (*fakeSessionProvider)(nil)
var _ execution.SessionProvider = (*fakeSessionProvider)(nil)
