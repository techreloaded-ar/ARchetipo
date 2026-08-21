package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
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
func (s *spyStore) ListBySpec(_ context.Context, specCode string) ([]Execution, error) {
	out := []Execution{}
	for _, e := range s.records {
		if e.SpecCode == strings.TrimSpace(specCode) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func newTestService(t *testing.T, provider *testProvider, store *spyStore) *Service {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	service, err := NewService(registry, store, func() (string, error) { return "exec-001", nil }, func() time.Time { now = now.Add(time.Second); return now }, t.TempDir())
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
	service, err := NewService(registry, store, func() (string, error) { newIDCalls++; return "exec-001", nil }, time.Now, t.TempDir())
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
	service, err := NewService(registry, store, func() (string, error) { return "exec-001", nil }, time.Now, t.TempDir())
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

// blockingProvider stays inside Execute until the test releases it. It is what
// makes "the record exists before the provider is contacted" observable without
// a single sleep: the order is imposed, not hoped for.
type blockingProvider struct {
	id           string
	capabilities []Capability
	release      chan struct{}
	entered      chan struct{}
	result       Result
	err          error
	calls        atomic.Int32
}

func newBlockingProvider(id string) *blockingProvider {
	return &blockingProvider{
		id:           id,
		capabilities: []Capability{CapabilitySpecPlan},
		release:      make(chan struct{}),
		entered:      make(chan struct{}, 1),
		result:       Result{Payload: json.RawMessage(`{"artifact":"plan-123"}`)},
	}
}

func (p *blockingProvider) ID() string { return p.id }
func (p *blockingProvider) Capabilities(context.Context) ([]Capability, error) {
	return p.capabilities, nil
}
func (p *blockingProvider) ValidateConfig(context.Context, map[string]any) error { return nil }
func (p *blockingProvider) Execute(ctx context.Context, _ Request) (Result, error) {
	p.calls.Add(1)
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	return p.result, p.err
}

func newBlockingService(t *testing.T, provider *blockingProvider, store Store) *Service {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	service, err := NewService(registry, store, func() (string, error) { return "exec-start-001", nil }, func() time.Time { now = now.Add(time.Second); return now }, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// AC-1: the record the UI receives exists, and is RUNNING, before the provider
// is contacted at all.
func TestStartPersistsRunningRecordBeforeDispatch(t *testing.T) {
	p := newBlockingProvider("fake")
	store := &spyStore{records: map[string]Execution{}}
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if continuation == nil {
		t.Fatal("Start returned no continuation")
	}
	if p.calls.Load() != 0 {
		t.Fatalf("the provider was contacted before Start returned: calls=%d", p.calls.Load())
	}
	if started.Status != StatusRunning || started.CompletedAt != nil || started.SpecStatusBefore != domain.StatusTodo || started.SpecCode != "US-001" || started.ProviderID != "fake" {
		t.Fatalf("started record: %#v", started)
	}
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatalf("the record was not persisted by Start: %v", err)
	}
	if persisted.Status != StatusRunning || persisted.CompletedAt != nil || store.creates != 1 || store.updates != 0 {
		t.Fatalf("persisted record: %#v creates=%d updates=%d", persisted, store.creates, store.updates)
	}
	close(p.release)
}

func TestStartContinuationClosesTheRecordOnSuccess(t *testing.T) {
	p := newBlockingProvider("fake")
	store := &spyStore{records: map[string]Execution{}}
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	close(p.release)
	outcome, err := continuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ID != started.ID || outcome.Status != StatusSucceeded || outcome.Result == nil || outcome.CompletedAt == nil {
		t.Fatalf("continuation outcome: %#v", outcome)
	}
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, outcome) {
		t.Fatalf("persisted record drifted from the returned one: %#v", persisted)
	}
}

// AC-3/AC-4: the caller's verdict is part of the terminal write, not a second
// write after it.
//
// An asynchronous caller is read while it works, so a record closed as
// SUCCEEDED and demoted a moment later is a success somebody can read in
// between. The store is the witness: it must be updated exactly once, and the
// single state it ever receives must already be the verified one.
func TestStartConfirmsTheOutcomeBeforeClosingTheRecord(t *testing.T) {
	p := newBlockingProvider("fake")
	store := &spyStore{records: map[string]Execution{}}
	var sawStatus ExecutionStatus
	confirm := func(_ context.Context, outcome *Execution) {
		// What the confirmation is handed is the success it has to judge, and the
		// store has not seen it yet.
		sawStatus = outcome.Status
		if store.updates != 0 {
			t.Errorf("the record was closed before the verdict: updates=%d", store.updates)
		}
		outcome.Status = StatusFailed
		outcome.Result = nil
		outcome.Error = &ExecutionError{Code: "UNCONFIRMED_EFFECT", Message: "the connector does not confirm it"}
	}
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, confirm)
	if err != nil {
		t.Fatal(err)
	}
	close(p.release)
	outcome, err := continuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sawStatus != StatusSucceeded {
		t.Fatalf("the confirmation judged %s instead of the provider's claim", sawStatus)
	}
	if outcome.Status != StatusFailed || outcome.Error == nil || outcome.Error.Code != "UNCONFIRMED_EFFECT" || outcome.Result != nil {
		t.Fatalf("the returned outcome is not the verdict: %#v", outcome)
	}
	if store.updates != 1 {
		t.Fatalf("the verdict cost a second write: updates=%d", store.updates)
	}
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != StatusFailed || persisted.Error == nil || persisted.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("the store saw an unverified state: %#v", persisted)
	}
	// A verdict that approves changes nothing: the same single write carries the
	// success through.
	approving := newBlockingProvider("fake")
	approvingStore := &spyStore{records: map[string]Execution{}}
	_, approvingContinuation, err := newBlockingService(t, approving, approvingStore).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, func(context.Context, *Execution) {})
	if err != nil {
		t.Fatal(err)
	}
	close(approving.release)
	approved, err := approvingContinuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != StatusSucceeded || approvingStore.updates != 1 {
		t.Fatalf("an approved outcome: %#v updates=%d", approved, approvingStore.updates)
	}
}

// The confirmation only ever judges a success: a dispatch that already failed
// has nothing for it to disown, and calling it would let a verdict resurrect a
// record the provider already closed.
func TestStartDoesNotConfirmAFailedDispatch(t *testing.T) {
	p := newBlockingProvider("fake")
	p.err = errors.New("boom")
	store := &spyStore{records: map[string]Execution{}}
	called := false
	_, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, func(_ context.Context, outcome *Execution) {
		called = outcome.Status == StatusSucceeded
	})
	if err != nil {
		t.Fatal(err)
	}
	close(p.release)
	outcome, err := continuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("the confirmation was handed a failure to judge")
	}
	if outcome.Status != StatusFailed || outcome.Error == nil || outcome.Error.Code != "PROVIDER_ERROR" {
		t.Fatalf("the provider failure was rewritten: %#v", outcome)
	}
}

// AC-4: a dispatch that fails closes the record with a readable reason, and a
// failure that happened after remote work existed keeps that identifier.
func TestStartContinuationClosesTheRecordOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name           string
		err            error
		wantExternalID string
	}{
		{"local failure", errors.New("boom"), ""},
		{"remote failure", &RemoteError{ExternalID: "task-remote-9", Err: errors.New("remote planning aborted")}, "task-remote-9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newBlockingProvider("fake")
			p.err = tc.err
			close(p.release)
			store := &spyStore{records: map[string]Execution{}}
			_, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := continuation(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Status != StatusFailed || outcome.Result != nil || outcome.Error == nil || outcome.Error.Code != "PROVIDER_ERROR" {
				t.Fatalf("failed outcome: %#v", outcome)
			}
			if outcome.Error.Message == "" || outcome.Error.ExternalID != tc.wantExternalID {
				t.Fatalf("failure reason: %#v", outcome.Error)
			}
		})
	}
}

// Nothing is created when the request cannot even be dispatched: a caller that
// gets an error from Start has no record to close.
func TestStartCreatesNoRecordWhenItFails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		action   ActionID
		provider string
		validate func(context.Context, map[string]any) error
	}{
		{"unknown action", "unknown", "fake", nil},
		{"unknown provider", ActionPlan, "missing", nil},
		{"rejected configuration", ActionPlan, "fake", func(context.Context, map[string]any) error {
			return &ConfigurationError{Field: "endpoint", Reason: "must use https"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, validate: tc.validate, result: Result{Payload: json.RawMessage(`{"ok":true}`)}}
			store := &spyStore{records: map[string]Execution{}}
			started, continuation, err := newTestService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001"}, tc.action, tc.provider, nil, nil)
			if err == nil {
				t.Fatal("expected Start to fail")
			}
			if continuation != nil || started.ID != "" {
				t.Fatalf("a failed Start handed back work to do: %#v", started)
			}
			if store.creates != 0 || p.calls != 0 {
				t.Fatalf("creates=%d calls=%d", store.creates, p.calls)
			}
		})
	}
}

// A shutdown cancels the dispatch context, and the record must still be closed:
// otherwise it would stay RUNNING for ever with nobody left to close it.
func TestStartContinuationClosesTheRecordOnACancelledContext(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := newBlockingProvider("fake")
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := continuation(ctx)
	if err != nil {
		t.Fatalf("the continuation refused to close the record: %v", err)
	}
	if outcome.Status != StatusFailed || outcome.Error == nil || outcome.CompletedAt == nil {
		t.Fatalf("interrupted outcome: %#v", outcome)
	}
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != StatusFailed {
		t.Fatalf("the interrupted record stayed %s on disk", persisted.Status)
	}
	close(p.release)
}

// --- Workspace-scoped executions (US-040) -----------------------------------

// The scope of an action is derived from the action itself and is total: every
// known action answers, and an invented one is an *ActionError rather than a
// silent default.
func TestActionScopeIsDerivedFromTheAction(t *testing.T) {
	for _, tc := range []struct {
		action ActionID
		want   Scope
	}{
		{ActionPlan, ScopeSpec},
		{ActionInception, ScopeWorkspace},
	} {
		got, err := ActionScope(tc.action)
		if err != nil || got != tc.want {
			t.Fatalf("ActionScope(%q) = %q, %v; want %q", tc.action, got, err, tc.want)
		}
	}
	for _, action := range []ActionID{"", "unknown", "workspace.inception"} {
		got, err := ActionScope(action)
		var actionErr *ActionError
		if !errors.As(err, &actionErr) || actionErr.Action != action || got != "" {
			t.Fatalf("ActionScope(%q) = %q, %v; want an *ActionError", action, got, err)
		}
	}
}

func TestRequiredCapabilityOfInception(t *testing.T) {
	got, err := RequiredCapability(ActionInception)
	if err != nil || got != CapabilityWorkspaceInception {
		t.Fatalf("RequiredCapability(inception) = %q, %v; want %q", got, err, CapabilityWorkspaceInception)
	}
}

func newInceptionProvider() *blockingProvider {
	p := newBlockingProvider("fake")
	p.capabilities = []Capability{CapabilityWorkspaceInception}
	p.result = Result{Payload: json.RawMessage(`{"artifact":"prd","status":"WRITTEN"}`)}
	return p
}

// AC-1: an execution whose object is the workspace goes through the same
// pipeline as a spec-scoped one and lands as a record with an empty spec_code.
func TestStartWorkspacePersistsRunningRecordWithoutSpecCode(t *testing.T) {
	p := newInceptionProvider()
	store := &spyStore{records: map[string]Execution{}}
	started, continuation, err := newBlockingService(t, p, store).StartWorkspace(context.Background(), ActionInception, "fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if continuation == nil {
		t.Fatal("StartWorkspace returned no continuation")
	}
	if p.calls.Load() != 0 {
		t.Fatalf("the provider was contacted before StartWorkspace returned: calls=%d", p.calls.Load())
	}
	if started.SpecCode != "" || started.Action != ActionInception || started.Capability != CapabilityWorkspaceInception {
		t.Fatalf("started record: %#v", started)
	}
	if started.Status != StatusRunning || started.CompletedAt != nil || started.ProviderID != "fake" {
		t.Fatalf("started record: %#v", started)
	}
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatalf("the record was not persisted by StartWorkspace: %v", err)
	}
	if persisted.Status != StatusRunning || persisted.SpecCode != "" || store.creates != 1 || store.updates != 0 {
		t.Fatalf("persisted record: %#v creates=%d updates=%d", persisted, store.creates, store.updates)
	}

	close(p.release)
	outcome, err := continuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ID != started.ID || outcome.Status != StatusSucceeded || outcome.Result == nil || outcome.CompletedAt == nil {
		t.Fatalf("continuation outcome: %#v", outcome)
	}
	assertJSONSemanticEqual(t, json.RawMessage(`{"artifact":"prd","status":"WRITTEN"}`), outcome.Result.Payload)
	if p.calls.Load() != 1 || store.updates != 1 {
		t.Fatalf("calls=%d updates=%d", p.calls.Load(), store.updates)
	}
	closed, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(closed, outcome) {
		t.Fatalf("persisted record drifted from the returned one: %#v", closed)
	}
}

// What the provider is handed carries the workspace scope too: no spec code,
// the inception action, and the capability it declared.
func TestStartWorkspaceDispatchesARequestWithoutSpecCode(t *testing.T) {
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilityWorkspaceInception}, result: Result{Payload: json.RawMessage(`{"artifact":"prd","status":"WRITTEN"}`)}}
	store := &spyStore{records: map[string]Execution{}}
	started, continuation, err := newTestService(t, p, store).StartWorkspace(context.Background(), ActionInception, "fake", map[string]any{"model": "sonnet"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := continuation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 {
		t.Fatalf("calls=%d", p.calls)
	}
	if p.request.SpecCode != "" || p.request.Action != ActionInception || p.request.Capability != CapabilityWorkspaceInception || p.request.ExecutionID != started.ID {
		t.Fatalf("provider request: %#v", p.request)
	}
	if p.request.ProviderConfig["model"] != "sonnet" {
		t.Fatalf("the provider configuration did not reach the request: %#v", p.request.ProviderConfig)
	}
}

// A provider that does not declare workspace.inception leaves nothing behind:
// the refusal happens before any record is written to disk.
func TestStartWorkspaceWithoutCapabilityCreatesNoRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	p := newBlockingProvider("fake")
	p.capabilities = []Capability{CapabilitySpecPlan}
	started, continuation, err := newBlockingService(t, p, store).StartWorkspace(context.Background(), ActionInception, "fake", nil, nil)
	var capabilityErr *CapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Fatalf("want a *CapabilityError, got %v", err)
	}
	if capabilityErr.ProviderID != "fake" || capabilityErr.Capability != CapabilityWorkspaceInception {
		t.Fatalf("capability error: %#v", capabilityErr)
	}
	if continuation != nil || started.ID != "" {
		t.Fatalf("a refused StartWorkspace handed back work to do: %#v", started)
	}
	if p.calls.Load() != 0 {
		t.Fatalf("the provider was dispatched anyway: calls=%d", p.calls.Load())
	}
	entries, err := os.ReadDir(filepath.Join(root, ".archetipo", "executions"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused start left %d record(s) on disk", len(entries))
	}
}

// The object of an action is not negotiable in either direction: a
// workspace-scoped action cannot be started against a spec, and a spec-scoped
// one cannot be started without it.
func TestStartAndStartWorkspaceRejectTheWrongActionObject(t *testing.T) {
	t.Run("inception through Start", func(t *testing.T) {
		p := newInceptionProvider()
		close(p.release)
		store := &spyStore{records: map[string]Execution{}}
		started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionInception, "fake", nil, nil)
		if err == nil {
			t.Fatal("a workspace-scoped action was accepted with a spec on it")
		}
		if !strings.Contains(err.Error(), string(ActionInception)) {
			t.Fatalf("the refusal does not name the action: %v", err)
		}
		if continuation != nil || started.ID != "" || store.creates != 0 || p.calls.Load() != 0 {
			t.Fatalf("started=%#v creates=%d calls=%d", started, store.creates, p.calls.Load())
		}
	})
	t.Run("plan through StartWorkspace", func(t *testing.T) {
		p := newBlockingProvider("fake")
		close(p.release)
		store := &spyStore{records: map[string]Execution{}}
		started, continuation, err := newBlockingService(t, p, store).StartWorkspace(context.Background(), ActionPlan, "fake", nil, nil)
		if err == nil {
			t.Fatal("a spec-scoped action was accepted without a spec")
		}
		if continuation != nil || started.ID != "" || store.creates != 0 || p.calls.Load() != 0 {
			t.Fatalf("started=%#v creates=%d calls=%d", started, store.creates, p.calls.Load())
		}
	})
	t.Run("unknown action through StartWorkspace", func(t *testing.T) {
		p := newInceptionProvider()
		close(p.release)
		store := &spyStore{records: map[string]Execution{}}
		_, continuation, err := newBlockingService(t, p, store).StartWorkspace(context.Background(), "unknown", "fake", nil, nil)
		var actionErr *ActionError
		if !errors.As(err, &actionErr) || continuation != nil || store.creates != 0 {
			t.Fatalf("err=%v creates=%d", err, store.creates)
		}
	})
}

// Preflight is the phase a caller runs before producing any effect of its own,
// so its central promise is that a refusal costs nothing: no record is created,
// and the caller is free to leave the spec exactly where it was.
func TestPreflightRefusesWithoutCreatingAnyRecord(t *testing.T) {
	cases := []struct {
		name       string
		action     ActionID
		providerID string
		provider   *testProvider
		assertErr  func(*testing.T, error)
	}{
		{
			name:       "a provider that cannot implement",
			action:     ActionImplement,
			providerID: "fake",
			provider:   &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}},
			assertErr: func(t *testing.T, err error) {
				var capErr *CapabilityError
				if !errors.As(err, &capErr) {
					t.Fatalf("error = %T (%v), want *CapabilityError", err, err)
				}
				if capErr.Capability != CapabilitySpecImplement {
					t.Fatalf("capability = %q, want %q", capErr.Capability, CapabilitySpecImplement)
				}
			},
		},
		{
			name:       "an unknown provider",
			action:     ActionImplement,
			providerID: "ghost",
			provider:   &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecImplement}},
			assertErr: func(t *testing.T, err error) {
				var regErr *RegistryError
				if !errors.As(err, &regErr) {
					t.Fatalf("error = %T (%v), want *RegistryError", err, err)
				}
			},
		},
		{
			name:       "a configuration the provider refuses",
			action:     ActionImplement,
			providerID: "fake",
			provider: &testProvider{
				id:           "fake",
				capabilities: []Capability{CapabilitySpecImplement},
				validate: func(context.Context, map[string]any) error {
					return &ConfigurationError{Field: "model", Reason: "missing"}
				},
			},
			assertErr: func(t *testing.T, err error) {
				var cfgErr *ConfigurationError
				if !errors.As(err, &cfgErr) {
					t.Fatalf("error = %T (%v), want *ConfigurationError", err, err)
				}
			},
		},
		{
			name:       "an unknown action",
			action:     ActionID("deploy"),
			providerID: "fake",
			provider:   &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecImplement}},
			assertErr: func(t *testing.T, err error) {
				var actionErr *ActionError
				if !errors.As(err, &actionErr) {
					t.Fatalf("error = %T (%v), want *ActionError", err, err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &spyStore{records: map[string]Execution{}}
			err := newTestService(t, tc.provider, store).Preflight(context.Background(), tc.action, tc.providerID, map[string]any{"model": "m"})
			if err == nil {
				t.Fatal("Preflight accepted the dispatch, want a refusal")
			}
			tc.assertErr(t, err)
			if store.creates != 0 || store.updates != 0 {
				t.Fatalf("store writes create=%d update=%d, want none", store.creates, store.updates)
			}
		})
	}
}

func TestPreflightAcceptsAProviderDeclaringTheCapability(t *testing.T) {
	provider := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan, CapabilitySpecImplement}}
	store := &spyStore{records: map[string]Execution{}}
	config := map[string]any{"model": "m"}
	if err := newTestService(t, provider, store).Preflight(context.Background(), ActionImplement, "fake", config); err != nil {
		t.Fatal(err)
	}
	if store.creates != 0 {
		t.Fatalf("Preflight created %d record(s), want none", store.creates)
	}
	// The configuration is validated on a copy, exactly as a dispatch does.
	config["model"] = "mutated"
	if provider.validated["model"] != "m" {
		t.Fatalf("validated config = %#v, want the caller's map to be untouched", provider.validated)
	}
}

// AC-3: the model a run uses has to be readable while that run is still open.
// The assertion is made *before* the continuation on purpose: it is exactly the
// moment somebody opens the detail of a run in progress, and a choice written
// only on the terminal write would be invisible right then.
func TestStartRecordsModelChoiceBeforeDispatch(t *testing.T) {
	p := newBlockingProvider("fake")
	store := &spyStore{records: map[string]Execution{}}
	choice := ModelChoice{Model: "m1", Options: map[string]string{"opt": "b"}, Source: ModelChoiceSourceRun}
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil, WithModelChoice(choice))
	if err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 0 {
		t.Fatalf("the provider was contacted before Start returned: calls=%d", p.calls.Load())
	}
	running, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatalf("the record was not persisted by Start: %v", err)
	}
	if running.Status != StatusRunning {
		t.Fatalf("record status = %q, want %q", running.Status, StatusRunning)
	}
	if running.ModelChoice == nil {
		t.Fatal("a RUNNING record carries no model choice, want the one the run started with")
	}
	if !reflect.DeepEqual(*running.ModelChoice, choice) {
		t.Fatalf("model choice on the RUNNING record = %#v, want %#v", *running.ModelChoice, choice)
	}

	close(p.release)
	outcome, err := continuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != StatusSucceeded {
		t.Fatalf("outcome status = %q, want %q", outcome.Status, StatusSucceeded)
	}
	closed, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != StatusSucceeded || closed.CompletedAt == nil {
		t.Fatalf("closed record: %#v", closed)
	}
	if closed.ModelChoice == nil || !reflect.DeepEqual(*closed.ModelChoice, choice) {
		t.Fatalf("model choice after the terminal write = %#v, want %#v", closed.ModelChoice, choice)
	}
}

// A run started without any choice must be byte-for-byte the record it was
// before this field existed: not an empty model_choice, no model_choice at all.
func TestStartWithoutModelChoiceLeavesRecordUnchanged(t *testing.T) {
	p := newBlockingProvider("fake")
	store := &spyStore{records: map[string]Execution{}}
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	close(p.release)
	if _, err := continuation(context.Background()); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ModelChoice != nil {
		t.Fatalf("model choice = %#v, want nil for a run started without one", persisted.ModelChoice)
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, present := fields["model_choice"]; present {
		t.Fatalf("serialized record carries a model_choice key: %s", encoded)
	}
}

// The caller keeps its own map; the record must not be a window onto it. A
// caller that mutates after Start cannot rewrite the history of a run that has
// already started.
func TestWithModelChoiceCopiesOptions(t *testing.T) {
	p := newBlockingProvider("fake")
	store := &spyStore{records: map[string]Execution{}}
	options := map[string]string{"opt": "b"}
	started, continuation, err := newBlockingService(t, p, store).Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil, WithModelChoice(ModelChoice{Model: "m1", Options: options, Source: ModelChoiceSourceRun}))
	if err != nil {
		t.Fatal(err)
	}
	options["opt"] = "mutated"
	options["added"] = "later"

	running, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"opt": "b"}
	if running.ModelChoice == nil || !reflect.DeepEqual(running.ModelChoice.Options, want) {
		t.Fatalf("options on the RUNNING record = %#v, want %#v", running.ModelChoice, want)
	}

	close(p.release)
	if _, err := continuation(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.ModelChoice == nil || !reflect.DeepEqual(closed.ModelChoice.Options, want) {
		t.Fatalf("options after the terminal write = %#v, want %#v", closed.ModelChoice, want)
	}
}

// The workspace-scoped variant records the choice through the same path: an
// inception run started with a model of its own is as readable as a spec one.
func TestStartWorkspaceRecordsModelChoice(t *testing.T) {
	p := newInceptionProvider()
	store := &spyStore{records: map[string]Execution{}}
	choice := ModelChoice{Model: "m1", Options: map[string]string{"opt": "b"}, Source: ModelChoiceSourceRun}
	started, continuation, err := newBlockingService(t, p, store).StartWorkspace(context.Background(), ActionInception, "fake", nil, nil, WithModelChoice(choice))
	if err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 0 {
		t.Fatalf("the provider was contacted before StartWorkspace returned: calls=%d", p.calls.Load())
	}
	running, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatalf("the record was not persisted by StartWorkspace: %v", err)
	}
	if running.SpecCode != "" {
		t.Fatalf("spec code = %q, want empty for a workspace-scoped run", running.SpecCode)
	}
	if running.Status != StatusRunning {
		t.Fatalf("record status = %q, want %q", running.Status, StatusRunning)
	}
	if running.ModelChoice == nil || !reflect.DeepEqual(*running.ModelChoice, choice) {
		t.Fatalf("model choice on the RUNNING record = %#v, want %#v", running.ModelChoice, choice)
	}

	close(p.release)
	if _, err := continuation(context.Background()); err != nil {
		t.Fatal(err)
	}
	closed, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != StatusSucceeded {
		t.Fatalf("closed record status = %q, want %q", closed.Status, StatusSucceeded)
	}
	if closed.ModelChoice == nil || !reflect.DeepEqual(*closed.ModelChoice, choice) {
		t.Fatalf("model choice after the terminal write = %#v, want %#v", closed.ModelChoice, choice)
	}
}

// newRootedTestService builds a service on an explicit working root, so a test
// can name the directory it expects the run to be started in instead of
// discovering it after the fact.
func newRootedTestService(t *testing.T, provider *testProvider, store *spyStore, workingRoot string) *Service {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(registry, store, func() (string, error) { return "exec-root-001", nil }, time.Now, workingRoot)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// AC-1: the run executes in the project root of the workspace the service
// serves, not in the working directory of the process that happens to host it.
func TestStartStampsTheWorkspaceRootOnRequestAndRecord(t *testing.T) {
	root := t.TempDir()
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`{"ok":true}`)}}
	store := &spyStore{records: map[string]Execution{}}
	service := newRootedTestService(t, p, store, root)

	started, continuation, err := service.Start(context.Background(), domain.Spec{Code: "US-001", Status: domain.StatusTodo}, ActionPlan, "fake", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if started.WorkingDir != root {
		t.Fatalf("the RUNNING record does not carry the workspace root: got %q want %q", started.WorkingDir, root)
	}
	closed, err := continuation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.request.WorkingDir != root {
		t.Fatalf("the provider was asked to run in %q, want %q", p.request.WorkingDir, root)
	}
	if closed.WorkingDir != root {
		t.Fatalf("the closed record lost the working directory: %#v", closed)
	}
	// AC-3 (contract): the value the closed record carries is the one stamped at
	// start, read back from the store after the continuation ran.
	persisted, err := store.Get(context.Background(), started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.WorkingDir != root {
		t.Fatalf("the persisted record does not carry the working directory: %#v", persisted)
	}
}

// The synchronous path the CLI uses stamps exactly the same root as the
// viewer's Start path, because both go through the one code path.
func TestRunAndRunIdempotentStampTheSameWorkspaceRoot(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*Service, domain.Spec) (Execution, error)
	}{
		{"Run", func(s *Service, spec domain.Spec) (Execution, error) {
			return s.Run(context.Background(), spec, ActionPlan, "fake", nil)
		}},
		{"RunIdempotent", func(s *Service, spec domain.Spec) (Execution, error) {
			out, _, err := s.RunIdempotent(context.Background(), spec, ActionPlan, "fake", nil, "r1")
			return out, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`{"ok":true}`)}}
			store := &spyStore{records: map[string]Execution{}}
			outcome, err := tc.run(newRootedTestService(t, p, store, root), domain.Spec{Code: "US-001"})
			if err != nil {
				t.Fatal(err)
			}
			if p.request.WorkingDir != root || outcome.WorkingDir != root {
				t.Fatalf("%s did not stamp the workspace root: request=%q record=%q want %q", tc.name, p.request.WorkingDir, outcome.WorkingDir, root)
			}
		})
	}
}

// A service that does not know where it executes must not be constructible:
// that is the defect this spec closes, expressed as a compile-time-adjacent
// guarantee rather than a convention.
func TestNewServiceRejectsAnEmptyWorkingRoot(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&testProvider{id: "fake"}); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"", "   "} {
		service, err := NewService(registry, &spyStore{records: map[string]Execution{}}, RandomID, time.Now, root)
		if err == nil || service != nil {
			t.Fatalf("working root %q: err=%v service=%v", root, err, service)
		}
	}
}

// The root reaches the provider absolute and clean, whatever shape the caller
// wrote it in.
func TestStartNormalisesTheWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	p := &testProvider{id: "fake", capabilities: []Capability{CapabilitySpecPlan}, result: Result{Payload: json.RawMessage(`{"ok":true}`)}}
	store := &spyStore{records: map[string]Execution{}}
	service := newRootedTestService(t, p, store, filepath.Join(root, "sub", ".."))

	if _, err := service.Run(context.Background(), domain.Spec{Code: "US-001"}, ActionPlan, "fake", nil); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p.request.WorkingDir) || p.request.WorkingDir != root {
		t.Fatalf("the working root was not normalised: got %q want %q", p.request.WorkingDir, root)
	}
}
