package execution

import (
	"context"
	"errors"
	"testing"
)

type availabilityProvider struct {
	testProvider
	err      error
	seen     map[string]any
	observed int
}

func (p *availabilityProvider) Available(_ context.Context, config map[string]any) error {
	p.observed++
	p.seen = config
	if config != nil {
		config["mutated"] = true
	}
	return p.err
}

func TestCheckAvailabilityTreatsSilentProviderAsAvailable(t *testing.T) {
	if err := CheckAvailability(context.Background(), &testProvider{id: "plain"}, map[string]any{"key": "value"}); err != nil {
		t.Fatalf("a provider that declares no availability must be available, got %v", err)
	}
}

func TestCheckAvailabilityReportsAvailableProvider(t *testing.T) {
	provider := &availabilityProvider{testProvider: testProvider{id: "codex"}}
	if err := CheckAvailability(context.Background(), provider, map[string]any{"binary": "codex"}); err != nil {
		t.Fatalf("available provider reported %v", err)
	}
	if provider.observed != 1 {
		t.Fatalf("provider probed %d times, want 1", provider.observed)
	}
	if provider.seen["binary"] != "codex" {
		t.Fatalf("configuration did not reach the provider: %#v", provider.seen)
	}
}

func TestCheckAvailabilityForwardsProviderErrorUnchanged(t *testing.T) {
	missing := errors.New("codex is not installed on this machine")
	provider := &availabilityProvider{testProvider: testProvider{id: "codex"}, err: missing}
	err := CheckAvailability(context.Background(), provider, nil)
	if !errors.Is(err, missing) {
		t.Fatalf("provider diagnostic lost, got %v", err)
	}
	if err.Error() != missing.Error() {
		t.Fatalf("provider diagnostic reworded: %q", err.Error())
	}
}

func TestCheckAvailabilityPassesAClonedConfiguration(t *testing.T) {
	original := map[string]any{"binary": "codex", "nested": map[string]any{"value": "before"}}
	provider := &availabilityProvider{testProvider: testProvider{id: "codex"}}
	if err := CheckAvailability(context.Background(), provider, original); err != nil {
		t.Fatal(err)
	}
	if _, mutated := original["mutated"]; mutated {
		t.Fatal("the provider mutated the caller's configuration map")
	}
	provider.seen["nested"].(map[string]any)["value"] = "after"
	if original["nested"].(map[string]any)["value"] != "before" {
		t.Fatal("CheckAvailability shared nested configuration values with the provider")
	}
}
