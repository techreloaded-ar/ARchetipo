package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

type persistentFakeNativeStore struct {
	mu       sync.Mutex
	next     int
	sessions map[string]*persistentFakeNativeSession
	starts   int
}

type persistentFakeNativeSession struct {
	snapshot execution.SessionSnapshot
	events   []execution.RunEvent
	notify   chan struct{}
}

type persistentFakeNativeProvider struct {
	id        string
	store     *persistentFakeNativeStore
	failStart bool
}

func newPersistentFakeNativeProvider(id string, store *persistentFakeNativeStore) *persistentFakeNativeProvider {
	if store == nil {
		store = &persistentFakeNativeStore{sessions: map[string]*persistentFakeNativeSession{}}
	}
	return &persistentFakeNativeProvider{id: id, store: store}
}

func (p *persistentFakeNativeProvider) ID() string { return p.id }
func (p *persistentFakeNativeProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{}, nil
}
func (p *persistentFakeNativeProvider) ValidateConfig(context.Context, map[string]any) error {
	return nil
}
func (p *persistentFakeNativeProvider) Execute(context.Context, execution.Request) (execution.Result, error) {
	return execution.Result{}, nil
}
func (p *persistentFakeNativeProvider) DiscoverSession(_ context.Context, request execution.SessionDiscoveryRequest) (execution.SessionDiscovery, error) {
	return execution.SessionDiscovery{Capabilities: []execution.SessionCapability{execution.SessionCapabilityResume, execution.SessionCapabilityInterrupt, execution.SessionCapabilitySteering, execution.SessionCapabilityTurnModel, execution.SessionCapabilityTurnOptions},
		Models:      []execution.ModelOption{{ID: "fake-large", Default: true, Options: []execution.ModelOptionField{{Name: "effort", Choices: []execution.ModelOptionChoice{{Value: "high"}, {Value: "low"}}}}}},
		Environment: execution.SessionEnvironment{WorkingDir: request.WorkingDir, ProviderConfig: execution.CloneConfig(request.ProviderConfig), Location: "local"}}, nil
}
func (p *persistentFakeNativeProvider) CreateSession(_ context.Context, request execution.CreateSessionRequest) (execution.SessionSnapshot, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	p.store.next++
	snapshot := execution.SessionSnapshot{Session: execution.SessionMetadata{ConversationID: request.ConversationID, ProviderID: p.id,
		Environment: request.Environment, Native: execution.NativeSessionReference{Kind: "fake.thread", ID: fmt.Sprintf("native-%d", p.store.next)}},
		Archive: execution.ArchiveOpen, Connection: execution.ConnectionConnected, Recovery: execution.RecoveryResumable,
		Work: execution.SessionIdle, PendingInputs: []execution.PendingInput{}, PendingApprovals: []execution.PendingApproval{}}
	p.store.sessions[snapshot.Session.Native.ID] = &persistentFakeNativeSession{snapshot: snapshot, notify: make(chan struct{}, 1)}
	return snapshot, nil
}
func (p *persistentFakeNativeProvider) ResumeSession(_ context.Context, request execution.ResumeSessionRequest) (execution.SessionSnapshot, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	session := p.store.sessions[request.Session.Native.ID]
	if session == nil {
		return execution.SessionSnapshot{}, fmt.Errorf("missing native session")
	}
	session.snapshot.Connection = execution.ConnectionConnected
	return session.snapshot, nil
}
func (p *persistentFakeNativeProvider) ReadSession(_ context.Context, request execution.SessionRequest) (execution.SessionSnapshot, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	session := p.store.sessions[request.Session.Native.ID]
	if session == nil {
		return execution.SessionSnapshot{}, fmt.Errorf("missing native session")
	}
	return session.snapshot, nil
}
func (p *persistentFakeNativeProvider) StartTurn(_ context.Context, request execution.StartTurnRequest) (execution.SessionTurnStarted, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	session := p.store.sessions[request.Session.Native.ID]
	if session.snapshot.Work != execution.SessionIdle {
		return execution.SessionTurnStarted{}, fmt.Errorf("turn already active")
	}
	p.store.starts++
	if p.failStart {
		return execution.SessionTurnStarted{}, fmt.Errorf("connection lost while submitting")
	}
	turn := execution.SessionTurn{ID: request.TurnID, State: execution.TurnActive, Requested: execution.TurnConfiguration{Model: request.Model, Options: request.Options},
		Applied: execution.TurnConfiguration{Model: request.Model, Options: request.Options}, StartedAt: time.Now().UTC()}
	session.snapshot.Work, session.snapshot.CurrentTurn = execution.SessionTurnActive, &turn
	session.events = append(session.events, execution.RunEvent{ID: int64(len(session.events) + 1), Kind: "user_message", Text: request.Message,
		TurnID: request.TurnID, SubmissionID: request.SubmissionID, At: time.Now().UTC()})
	select {
	case session.notify <- struct{}{}:
	default:
	}
	return execution.SessionTurnStarted{Turn: turn, Delivery: execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID,
		State: execution.DeliveryConfirmed, NativeDeliveryID: "native-" + request.SubmissionID}}, nil
}
func (p *persistentFakeNativeProvider) StreamSessionEvents(ctx context.Context, request execution.SessionRequest, afterID int64, sink func(execution.RunEvent) error) error {
	p.store.mu.Lock()
	session := p.store.sessions[request.Session.Native.ID]
	notify := session.notify
	events := append([]execution.RunEvent(nil), session.events...)
	p.store.mu.Unlock()
	if len(events) == 0 || events[len(events)-1].ID <= afterID {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
		p.store.mu.Lock()
		events = append([]execution.RunEvent(nil), session.events...)
		p.store.mu.Unlock()
	}
	for _, event := range events {
		if event.ID > afterID {
			if err := sink(event); err != nil {
				return err
			}
		}
	}
	return nil
}
func (p *persistentFakeNativeProvider) SteerTurn(_ context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryConfirmed}, nil
}
func (p *persistentFakeNativeProvider) RespondSessionInput(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.SteerTurn(ctx, request)
}
func (p *persistentFakeNativeProvider) RespondSessionApproval(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.SteerTurn(ctx, request)
}
func (p *persistentFakeNativeProvider) InterruptTurn(_ context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	session := p.store.sessions[request.Session.Native.ID]
	completed := time.Now().UTC()
	session.snapshot.CurrentTurn.State = execution.TurnInterrupted
	session.snapshot.CurrentTurn.CompletedAt = &completed
	session.snapshot.Work = execution.SessionIdle
	return execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryConfirmed}, nil
}
func (p *persistentFakeNativeProvider) ReleaseSession(_ context.Context, request execution.SessionRequest) error {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	session := p.store.sessions[request.Session.Native.ID]
	session.snapshot.Connection = execution.ConnectionReleased
	return nil
}

func TestNativeConversationPersistsWithoutReadsResumesSameIDAndSteersActiveTurn(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, cfg, conn := newRunServer(t, provider, true)
	opened := openConversationOK(t, srv)
	id := opened.Conversation.ID

	var responses sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		responses.Add(1)
		go func() {
			defer responses.Done()
			w := doJSON(t, srv, "POST", conversationsRoute+"/"+id+"/messages", map[string]any{"message": "only once"})
			statuses <- w.Code
		}()
	}
	responses.Wait()
	close(statuses)
	accepted := 0
	for status := range statuses {
		if status == 202 {
			accepted++
		}
	}
	if accepted != 2 || provider.store.starts != 1 {
		t.Fatalf("accepted=%d starts=%d, want two deliveries in one turn", accepted, provider.store.starts)
	}

	provider.store.mu.Lock()
	var nativeSession *persistentFakeNativeSession
	for _, candidate := range provider.store.sessions {
		nativeSession = candidate
	}
	for len(nativeSession.events) < 2105 {
		nativeSession.events = append(nativeSession.events, execution.RunEvent{ID: int64(len(nativeSession.events) + 1), Kind: "text", Text: "persisted", At: time.Now().UTC()})
	}
	select {
	case nativeSession.notify <- struct{}{}:
	default:
	}
	provider.store.mu.Unlock()

	store, _ := conversationlog.NewFileStore(cfg.ProjectRoot)
	deadline := time.Now().Add(3 * time.Second)
	for {
		page, err := store.ReadEvents(context.Background(), id, 0, 0)
		if err == nil && len(page.Events) == 2105 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("events were not persisted without GET: %d, %v", len(page.Events), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cursor := int64(0)
	total := 0
	for {
		status, page, body := readConversation(t, srv, id, cursor)
		if status != 200 {
			t.Fatalf("page after %d = %d: %s", cursor, status, body)
		}
		for _, event := range page.Events {
			if event.ID != cursor+1 {
				t.Fatalf("pagination gap after %d: got %d", cursor, event.ID)
			}
			cursor = event.ID
			total++
		}
		if !page.HasMore {
			break
		}
	}
	if total != 2105 || cursor != 2105 {
		t.Fatalf("paginated history = %d events through %d", total, cursor)
	}

	if w := doJSON(t, srv, "POST", conversationsRoute+"/"+id+"/interrupt", map[string]any{}); w.Code != 202 {
		t.Fatalf("interrupt = %d: %s", w.Code, w.Body.String())
	}
	closeConversation(t, srv, id)
	restartedProvider := newPersistentFakeNativeProvider("native-fake", provider.store)
	otherProvider := newPersistentFakeNativeProvider("other-native", nil)
	registry := execution.NewRegistry()
	if err := registry.Register(restartedProvider); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(otherProvider); err != nil {
		t.Fatal(err)
	}
	if _, err := config.UpdateDefaultProvider(cfg.ProjectRoot, config.DefaultProviderConfig{ID: otherProvider.ID(), Config: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	w := doJSON(t, restarted, "POST", conversationsRoute+"/"+id+"/messages", map[string]any{"message": "same session"})
	if w.Code != 202 {
		t.Fatalf("message after restart = %d: %s", w.Code, w.Body.String())
	}
	resumed := decodeConversation(t, w.Body.String())
	if resumed.Conversation.ID != id {
		t.Fatalf("resume changed conversation id from %s to %s", id, resumed.Conversation.ID)
	}
	if restartedProvider.store.starts != 2 || otherProvider.store.starts != 0 {
		t.Fatalf("existing session moved with the default: original starts=%d other starts=%d", restartedProvider.store.starts, otherProvider.store.starts)
	}
	closeConversation(t, restarted, id)
}

func TestNativeConversationKeepsAnUncertainDeliveryAfterSubmissionFailure(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	provider.failStart = true
	srv, cfg, _ := newRunServer(t, provider, true)
	id := openConversationOK(t, srv).Conversation.ID
	first := doJSON(t, srv, "POST", conversationsRoute+"/"+id+"/messages", map[string]any{"message": "may have arrived"})
	if first.Code == 202 {
		t.Fatalf("a failed submission was reported successful: %s", first.Body.String())
	}
	record, err := srv.session().conversationStore().GetMetadata(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Deliveries) != 1 || record.Deliveries[0].State != execution.DeliveryUncertain {
		t.Fatalf("delivery after crash boundary = %#v", record.Deliveries)
	}
	starts := provider.store.starts
	second := doJSON(t, srv, "POST", conversationsRoute+"/"+id+"/messages", map[string]any{"message": "automatic retry"})
	if second.Code != 409 || provider.store.starts != starts {
		t.Fatalf("uncertain command was retried: status=%d starts=%d body=%s", second.Code, provider.store.starts, second.Body.String())
	}
	if _, err := conversationlog.NewFileStore(cfg.ProjectRoot); err != nil {
		t.Fatal(err)
	}
}

func TestNativeConversationLifecycleThroughHTTPWithoutBrowser(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	httpServer := httptest.NewServer(srv.mux)
	defer httpServer.Close()

	status, body := nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("open = %d: %s", status, body)
	}
	opened := decodeConversation(t, string(body))
	id := opened.Conversation.ID
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodGet, conversationsRoute+"/"+id+"/model-choice", nil)
	var modelChoice nativeConversationModelChoiceView
	if status != http.StatusOK || json.Unmarshal(body, &modelChoice) != nil || modelChoice.ProviderID != "native-fake" || modelChoice.ModelSource != "session" || modelChoice.Environment.Location != "local" || len(modelChoice.Models) != 1 {
		t.Fatalf("session model choice = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPut, conversationsRoute+"/"+id+"/next-turn",
		map[string]any{"model": "not-in-session-catalog", "model_options": map[string]string{"effort": "high"}})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid next turn = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPut, conversationsRoute+"/"+id+"/next-turn",
		map[string]any{"model": "fake-large", "model_options": map[string]string{"effort": "high"}})
	if status != http.StatusOK {
		t.Fatalf("next turn = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "over real HTTP"})
	if status != http.StatusAccepted {
		t.Fatalf("message = %d: %s", status, body)
	}
	provider.store.mu.Lock()
	var native *persistentFakeNativeSession
	for _, candidate := range provider.store.sessions {
		native = candidate
	}
	native.snapshot.Work = execution.SessionWaitingInput
	native.snapshot.PendingInputs = []execution.PendingInput{{ID: "input-1", Payload: json.RawMessage(`{"question":"A o B?"}`)}}
	provider.store.mu.Unlock()
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/inputs/input-1",
		map[string]any{"payload": map[string]any{"answers": map[string]any{"choice": "B"}}})
	if status != http.StatusAccepted {
		t.Fatalf("input response = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/interrupt", map[string]any{})
	if status != http.StatusAccepted {
		t.Fatalf("interrupt = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodDelete, conversationsRoute+"/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("release = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/resume", map[string]any{"message": "same id"})
	if status != http.StatusCreated || decodeConversation(t, string(body)).Conversation.ID != id {
		t.Fatalf("native resume = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/archive", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("archive = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodDelete, conversationsRoute+"/"+id+"/record", nil)
	if status != http.StatusOK {
		t.Fatalf("delete metadata = %d: %s", status, body)
	}
	var deleted map[string]any
	if err := json.Unmarshal(body, &deleted); err != nil || deleted["native_session_preserved"] != true {
		t.Fatalf("delete semantics are not explicit: %s (%v)", body, err)
	}
}

func nativeHTTPRequest(t *testing.T, baseURL, method, path string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}

var _ execution.Provider = (*persistentFakeNativeProvider)(nil)
var _ execution.SessionProvider = (*persistentFakeNativeProvider)(nil)
