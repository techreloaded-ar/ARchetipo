package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

type spyStore struct {
	records              map[string]Execution
	creates, updates     int
	createErr, updateErr error
}

func (s *spyStore) Create(_ context.Context, e Execution) error {
	s.creates++
	if s.createErr != nil {
		return s.createErr
	}
	s.records[e.ID] = e
	return nil
}
func (s *spyStore) Update(_ context.Context, e Execution) error {
	s.updates++
	if s.updateErr != nil {
		return s.updateErr
	}
	s.records[e.ID] = e
	return nil
}
func (s *spyStore) Get(_ context.Context, id string) (Execution, error) {
	e, ok := s.records[id]
	if !ok {
		return Execution{}, &StoreError{Kind: StoreNotFound, ID: id}
	}
	return e, nil
}

func newTestService(t *testing.T, provider *testProvider, store *spyStore) *Service {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	service, err := NewService(registry, store, func() (string, error) { return "exec-001", nil }, func() time.Time { now = now.Add(time.Second); return now })
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func assertJSONSemanticEqual(t *testing.T, want, got json.RawMessage) {
	t.Helper()
	var wantValue, gotValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("decode expected JSON %q: %v", want, err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode actual JSON %q: %v", got, err)
	}
	if !reflect.DeepEqual(wantValue, gotValue) {
		t.Fatalf("JSON mismatch: want=%v got=%v", wantValue, gotValue)
	}
}

func TestServiceSuccess(t *testing.T) {
	wantPayload := json.RawMessage(`{"artifact":"plan-123"}`)
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: wantPayload}}
	store := &spyStore{records: map[string]Execution{}}
	providerConfig := map[string]any{"endpoint": "https://runner.test", "nested": map[string]any{"region": "eu"}}
	got, err := newTestService(t, p, store).Run(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", providerConfig)
	if err != nil {
		t.Fatal(err)
	}
	if store.creates != 1 || store.updates != 1 || p.calls != 1 || p.request.ExecutionID != got.ID {
		t.Fatalf("counts create=%d update=%d calls=%d", store.creates, store.updates, p.calls)
	}
	if got.Status != StatusSucceeded || got.Result == nil || got.Error != nil || got.SpecStatusBefore != domain.StatusTodo {
		t.Fatalf("outcome: %#v", got)
	}
	providerConfig["endpoint"] = "mutated"
	providerConfig["nested"].(map[string]any)["region"] = "mutated"
	if p.request.ProviderConfig["endpoint"] != "https://runner.test" || p.request.ProviderConfig["nested"].(map[string]any)["region"] != "eu" {
		t.Fatalf("provider config was not copied: %#v", p.request.ProviderConfig)
	}
	persisted, err := store.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != StatusSucceeded || persisted.Result == nil || persisted.Error != nil {
		t.Fatalf("persisted outcome: %#v", persisted)
	}
	assertJSONSemanticEqual(t, wantPayload, got.Result.Payload)
	assertJSONSemanticEqual(t, wantPayload, persisted.Result.Payload)
}

func TestServiceProviderFailure(t *testing.T) {
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, err: errors.New("boom")}
	store := &spyStore{records: map[string]Execution{}}
	got, err := newTestService(t, p, store).Run(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.Error == nil || got.Result != nil || p.calls != 1 || store.creates != 1 || store.updates != 1 {
		t.Fatalf("failure: %#v", got)
	}
}

// A failure that happens after remote work exists is the only trace left of
// that work, so the identifier must survive in a field a program can read and
// not only inside the message.
func TestServiceRecordsTheExternalIDOfAFailedRemoteDispatch(t *testing.T) {
	p := &testProvider{
		id:           "fake",
		capabilities: []Capability{CapabilitySpecPlan},
		err:          &RemoteError{ExternalID: "task-remote-1", Err: errors.New("timed out waiting for task-remote-1")},
	}
	store := &spyStore{records: map[string]Execution{}}
	got, err := newTestService(t, p, store).Run(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.Error == nil || got.Error.ExternalID != "task-remote-1" {
		t.Fatalf("failure did not keep the remote identifier: %#v", got.Error)
	}
	if got.Error.Message != "timed out waiting for task-remote-1" {
		t.Fatalf("the message was rewritten: %q", got.Error.Message)
	}
	persisted, err := store.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted.Error, got.Error) {
		t.Fatalf("persisted error drifted: %#v", persisted.Error)
	}
	// A local failure with nothing remote behind it must not invent one.
	plain := &testProvider{id: "plain", capabilities: []Capability{CapabilitySpecPlan}, err: errors.New("boom")}
	local, err := newTestService(t, plain, &spyStore{records: map[string]Execution{}}).Run(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	if local.Error == nil || local.Error.ExternalID != "" {
		t.Fatalf("a local failure carries an external id: %#v", local.Error)
	}
}

func TestServiceInvalidProviderResultIsPersistedFailure(t *testing.T) {
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`not-json`)}}
	store := &spyStore{records: map[string]Execution{}}
	got, err := newTestService(t, p, store).Run(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.Error == nil || got.Error.Code != "INVALID_PROVIDER_RESULT" || got.Result != nil || store.updates != 1 {
		t.Fatalf("invalid result outcome: %#v", got)
	}
}

func TestServiceCancellationAfterValidProviderResultIsPersistedFailureWithoutRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &testProvider{
		id:           "fake",
		capabilities: []Capability{CapabilitySpecPlan},
		execute: func(context.Context, Request) (Result, error) {
			cancel()
			return Result{Payload: json.RawMessage(`{"artifact":"plan-123"}`)}, nil
		},
	}
	store := &spyStore{records: map[string]Execution{}}
	got, err := newTestService(t, p, store).Run(ctx, domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || store.creates != 1 || store.updates != 1 {
		t.Fatalf("counts create=%d update=%d calls=%d", store.creates, store.updates, p.calls)
	}
	if got.Status != StatusFailed || got.Result != nil || got.Error == nil || got.Error.Code != "PROVIDER_ERROR" {
		t.Fatalf("cancelled outcome: %#v", got)
	}
	persisted, err := store.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != StatusFailed || persisted.Result != nil || !reflect.DeepEqual(persisted.Error, got.Error) {
		t.Fatalf("persisted cancelled outcome: %#v", persisted)
	}
}

func TestServiceMissingCapability(t *testing.T) {
	p := &testProvider{id: "fake", capabilities: []Capability{"other"}}
	store := &spyStore{records: map[string]Execution{}}
	_, err := newTestService(t, p, store).Run(context.Background(), domain.Spec{Code: "US-001"}, ActionPlan, "fake", nil)
	if err == nil || !strings.Contains(err.Error(), string(CapabilitySpecPlan)) || p.calls != 0 || store.creates != 0 || store.updates != 0 {
		t.Fatalf("unexpected: err=%v calls=%d create=%d", err, p.calls, store.creates)
	}
}

func TestServiceInfrastructureBranches(t *testing.T) {
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`{"ok":true}`)}}
	for _, tc := range []struct {
		name       string
		action     ActionID
		provider   string
		store      *spyStore
		wantCreate int
	}{
		{"unknown action", "unknown", "fake", &spyStore{records: map[string]Execution{}}, 0},
		{"unknown provider", ActionPlan, "missing", &spyStore{records: map[string]Execution{}}, 0},
		{"create failure", ActionPlan, "fake", &spyStore{records: map[string]Execution{}, createErr: errors.New("collision")}, 1},
		{"update failure", ActionPlan, "fake", &spyStore{records: map[string]Execution{}, updateErr: errors.New("disk")}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newTestService(t, p, tc.store).Run(context.Background(), domain.Spec{Code: "US-001"}, tc.action, tc.provider, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.store.creates != tc.wantCreate {
				t.Fatalf("creates %d", tc.store.creates)
			}
		})
	}
}

func TestServiceRejectsInvalidConfigBeforeEffects(t *testing.T) {
	p := &testProvider{
		id:           "fake",
		capabilities: []Capability{CapabilitySpecPlan},
		validate: func(_ context.Context, config map[string]any) error {
			config["endpoint"] = "provider-mutated-copy"
			return &ConfigurationError{Field: "endpoint", Reason: "must use https"}
		},
	}
	store := &spyStore{records: map[string]Execution{}}
	newIDCalls := 0
	registry := NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(registry, store, func() (string, error) { newIDCalls++; return "exec-001", nil }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"endpoint": "http://invalid"}
	_, err = service.Run(context.Background(), domain.Spec{Code: "US-001"}, ActionPlan, "fake", config)
	var configErr *ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "endpoint" || newIDCalls != 0 || store.creates != 0 || p.calls != 0 {
		t.Fatalf("err=%v ids=%d creates=%d calls=%d", err, newIDCalls, store.creates, p.calls)
	}
	if config["endpoint"] != "http://invalid" {
		t.Fatalf("provider mutated caller config: %#v", config)
	}
}

func newIdempotentTestFixture(t *testing.T) (*testProvider, *spyStore, *Service) {
	t.Helper()
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`{"artifact":"plan-123"}`)}}
	store := &spyStore{records: map[string]Execution{}}
	return p, store, newTestService(t, p, store)
}

func TestRunIdempotentReusesWithoutDispatch(t *testing.T) {
	p, store, service := newIdempotentTestFixture(t)
	spec := domain.Spec{Code: "US-001", Status: domain.StatusTodo}
	first, reused, err := service.RunIdempotent(context.Background(), spec, ActionPlan, "fake", nil, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if reused || first.Status != StatusSucceeded || first.RequestID != "r1" || p.calls != 1 {
		t.Fatalf("first run: reused=%t outcome=%#v calls=%d", reused, first, p.calls)
	}
	if first.ID != DeriveID("US-001", ActionPlan, "fake", "r1") {
		t.Fatalf("execution id is not derived from the request key: %q", first.ID)
	}
	second, reused, err := service.RunIdempotent(context.Background(), spec, ActionPlan, "fake", nil, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if !reused || second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) || p.calls != 1 || store.creates != 1 {
		t.Fatalf("second run: reused=%t outcome=%#v calls=%d creates=%d", reused, second, p.calls, store.creates)
	}
}

func TestRunIdempotentDistinctRequestIDsProduceDistinctExecutions(t *testing.T) {
	p, store, service := newIdempotentTestFixture(t)
	spec := domain.Spec{Code: "US-001", Status: domain.StatusTodo}
	first, _, err := service.RunIdempotent(context.Background(), spec, ActionPlan, "fake", nil, "r1")
	if err != nil {
		t.Fatal(err)
	}
	second, reused, err := service.RunIdempotent(context.Background(), spec, ActionPlan, "fake", nil, "r2")
	if err != nil {
		t.Fatal(err)
	}
	if reused || first.ID == second.ID || p.calls != 2 || store.creates != 2 {
		t.Fatalf("distinct keys collapsed: first=%q second=%q calls=%d creates=%d", first.ID, second.ID, p.calls, store.creates)
	}
}

// Reuse is unconditional: a record that failed is returned as it is, so
// retrying after a failure means using a new request key.
func TestRunIdempotentReusesFailedRecordWithoutDispatch(t *testing.T) {
	p, store, service := newIdempotentTestFixture(t)
	id := DeriveID("US-001", ActionPlan, "fake", "r1")
	store.records[id] = Execution{ID: id, SpecCode: "US-001", Action: ActionPlan, ProviderID: "fake", RequestID: "r1", Status: StatusFailed, Error: &ExecutionError{Code: "PROVIDER_ERROR", Message: "remote planning aborted"}}
	got, reused, err := service.RunIdempotent(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if !reused || got.Status != StatusFailed || got.Error == nil || got.Error.Message != "remote planning aborted" || p.calls != 0 || store.creates != 0 {
		t.Fatalf("failed record not reused: reused=%t outcome=%#v calls=%d creates=%d", reused, got, p.calls, store.creates)
	}
}

// raceStore models the concurrent-create window: the first Get misses, then a
// competing request wins Create, so the reread must return that record.
type raceStore struct {
	*spyStore
	winner Execution
	gets   int
}

func (s *raceStore) Get(ctx context.Context, id string) (Execution, error) {
	s.gets++
	if s.gets == 1 {
		return Execution{}, &StoreError{Kind: StoreNotFound, ID: id}
	}
	return s.winner, nil
}

func (s *raceStore) Create(_ context.Context, e Execution) error {
	s.creates++
	return &StoreError{Kind: StoreAlreadyExist, ID: e.ID}
}

func TestRunIdempotentTreatsAlreadyExistsAsReuse(t *testing.T) {
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`{"ok":true}`)}}
	registry := NewRegistry()
	if err := registry.Register(p); err != nil {
		t.Fatal(err)
	}
	id := DeriveID("US-001", ActionPlan, "fake", "r1")
	store := &raceStore{spyStore: &spyStore{records: map[string]Execution{}}, winner: Execution{ID: id, SpecCode: "US-001", RequestID: "r1", Status: StatusSucceeded}}
	service, err := NewService(registry, store, func() (string, error) { return "exec-001", nil }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, reused, err := service.RunIdempotent(context.Background(), domain.Spec{Code: "US-001"}, ActionPlan, "fake", nil, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if !reused || got.ID != id || got.Status != StatusSucceeded || store.creates != 1 {
		t.Fatalf("create race not treated as reuse: reused=%t outcome=%#v creates=%d", reused, got, store.creates)
	}
}

func TestRunIdempotentRejectsEmptyRequestID(t *testing.T) {
	for _, requestID := range []string{"", "   "} {
		p, store, service := newIdempotentTestFixture(t)
		_, reused, err := service.RunIdempotent(context.Background(), domain.Spec{Code: "US-001"}, ActionPlan, "fake", nil, requestID)
		if err == nil || reused || store.creates != 0 || p.calls != 0 {
			t.Fatalf("request id %q: err=%v reused=%t creates=%d calls=%d", requestID, err, reused, store.creates, p.calls)
		}
	}
}
