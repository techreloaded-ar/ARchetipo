package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

type testProvider struct {
	id           string
	capabilities []Capability
	result       Result
	err          error
	execute      func(context.Context, Request) (Result, error)
	validate     func(context.Context, map[string]any) error
	validated    map[string]any
	calls        int
	request      Request
}

func (p *testProvider) ID() string { return p.id }
func (p *testProvider) Capabilities(context.Context) ([]Capability, error) {
	return p.capabilities, nil
}
func (p *testProvider) ValidateConfig(ctx context.Context, config map[string]any) error {
	p.validated = CloneConfig(config)
	if p.validate != nil {
		return p.validate(ctx, config)
	}
	return nil
}
func (p *testProvider) Execute(ctx context.Context, request Request) (Result, error) {
	p.calls++
	p.request = request
	if p.execute != nil {
		return p.execute(ctx, request)
	}
	return p.result, p.err
}

func TestRequiredCapability(t *testing.T) {
	got, err := RequiredCapability(ActionPlan)
	if err != nil || got != CapabilitySpecPlan {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, action := range []ActionID{"", "unknown"} {
		if _, err := RequiredCapability(action); err == nil {
			t.Fatalf("expected %q to fail", action)
		}
	}
}

func TestRegistry(t *testing.T) {
	one, two := NewRegistry(), NewRegistry()
	p := &testProvider{id: "fake"}
	if err := one.Register(p); err != nil {
		t.Fatal(err)
	}
	if got, err := one.Resolve("fake"); err != nil || got != p {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := two.Resolve("fake"); err == nil {
		t.Fatal("registry state leaked")
	}
	if err := one.Register(p); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := one.Register(&testProvider{}); err == nil {
		t.Fatal("empty id accepted")
	}
	if Supports([]Capability{"spec.plna", "Plan", CapabilitySpecPlan, CapabilitySpecPlan}, CapabilitySpecPlan) != true {
		t.Fatal("required capability not found")
	}
	if Supports([]Capability{"spec.plna", "Plan"}, CapabilitySpecPlan) {
		t.Fatal("typo accepted")
	}
}

type describingProvider struct {
	testProvider
	fields []ConfigField
}

func (p *describingProvider) ConfigFields() []ConfigField { return p.fields }

func TestRegistryListKeepsRegistrationOrder(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"zeta", "alpha", "mid"} {
		if err := registry.Register(&testProvider{id: id}); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"zeta", "alpha", "mid"}
	got := registry.List()
	if len(got) != len(want) {
		t.Fatalf("expected %d providers, got %d", len(want), len(got))
	}
	for i, provider := range got {
		if provider.ID() != want[i] {
			t.Fatalf("provider %d: expected %q, got %q", i, want[i], provider.ID())
		}
	}
	got[0] = &testProvider{id: "tampered"}
	if again := registry.List(); again[0].ID() != "zeta" {
		t.Fatalf("returned slice is not detached from the registry: %q", again[0].ID())
	}
	if (*Registry)(nil).List() != nil {
		t.Fatal("nil registry must list nothing")
	}
}

func TestDescribeConfig(t *testing.T) {
	if fields := DescribeConfig(&testProvider{id: "plain"}); fields == nil || len(fields) != 0 {
		t.Fatalf("a provider without fields must describe an empty, non-nil list, got %#v", fields)
	}
	declared := []ConfigField{{Name: "endpoint", Label: "Endpoint", Type: "text", Required: true}}
	describer := &describingProvider{testProvider: testProvider{id: "rich"}, fields: declared}
	fields := DescribeConfig(describer)
	if len(fields) != 1 || fields[0].Name != "endpoint" {
		t.Fatalf("declared fields lost: %#v", fields)
	}
	fields[0].Name = "tampered"
	if declared[0].Name != "endpoint" {
		t.Fatal("DescribeConfig did not detach the returned slice")
	}
}

func TestConfigurationErrorAndCloneConfig(t *testing.T) {
	cause := context.Canceled
	err := &ConfigurationError{Field: "endpoint", Reason: "is required", Err: cause}
	if !strings.Contains(err.Error(), "endpoint") || !errors.Is(err, cause) {
		t.Fatalf("configuration error lost field or cause: %v", err)
	}
	original := map[string]any{"nested": map[string]any{"value": "before"}, "list": []any{map[string]any{"value": "before"}}}
	cloned := CloneConfig(original)
	cloned["nested"].(map[string]any)["value"] = "after"
	cloned["list"].([]any)[0].(map[string]any)["value"] = "after"
	if original["nested"].(map[string]any)["value"] != "before" || original["list"].([]any)[0].(map[string]any)["value"] != "before" {
		t.Fatal("CloneConfig did not isolate nested values")
	}
}

func TestExecutionJSON(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	cases := []Execution{
		{ID: "one", SpecCode: "US-001", Status: StatusSucceeded, SpecStatusBefore: domain.StatusTodo, Result: &Result{Payload: json.RawMessage(`{"artifact":"plan"}`), ExternalID: "remote"}, CreatedAt: now, CompletedAt: &now},
		{ID: "two", SpecCode: "US-001", Status: StatusFailed, SpecStatusBefore: domain.StatusTodo, Error: &ExecutionError{Code: "PROVIDER_ERROR", Message: "boom"}, CreatedAt: now, CompletedAt: &now},
	}
	for _, want := range cases {
		body, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got Execution
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != want.Status || got.SpecStatusBefore != domain.StatusTodo {
			t.Fatalf("round trip mismatch: %#v", got)
		}
		if want.Status == StatusSucceeded && (got.Result == nil || got.Error != nil || string(got.Result.Payload) != `{"artifact":"plan"}`) {
			t.Fatalf("success XOR mismatch: %#v", got)
		}
		if want.Status == StatusFailed && (got.Error == nil || got.Result != nil || got.Error.Message != "boom") {
			t.Fatalf("failure XOR mismatch: %#v", got)
		}
	}
}
