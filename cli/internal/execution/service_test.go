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
		return Execution{}, errors.New("missing")
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
