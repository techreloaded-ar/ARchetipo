package codex

import (
	"context"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// The catalog is what the configuration form offers, so every entry must carry
// an identifier a person can pick and read. The default is checked by count and
// not by name: which model a provider defaults to is the vendor's business and
// may change, but "exactly one entry is the default" is what the UI relies on.
func TestModelCatalogHoldsItsInvariants(t *testing.T) {
	models, err := (&Provider{}).Models(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("listing models with an empty configuration failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("the declared catalog is empty: the configuration form would have nothing to offer")
	}
	seen := make(map[string]bool, len(models))
	defaults := 0
	for _, model := range models {
		if model.ID == "" || strings.TrimSpace(model.ID) != model.ID {
			t.Fatalf("model identifier %q is empty or carries surrounding blanks", model.ID)
		}
		if seen[model.ID] {
			t.Fatalf("model identifier %q is declared twice", model.ID)
		}
		seen[model.ID] = true
		if strings.TrimSpace(model.Label) == "" {
			t.Fatalf("model %q has no label to read", model.ID)
		}
		if model.Default {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("the catalog marks %d entries as the provider default, want exactly 1", defaults)
	}
}

// The placeholder of the model field is an example of what to type, so it must
// name a model the catalog actually offers. Without this test the example and
// the catalog could drift apart without anything noticing.
func TestModelFieldPlaceholderBelongsToTheCatalog(t *testing.T) {
	provider := &Provider{}
	placeholder := ""
	found := false
	for _, field := range provider.ConfigFields() {
		if field.Name == execution.ModelFieldName {
			placeholder = field.Placeholder
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no configuration field is named %q", execution.ModelFieldName)
	}
	models, err := provider.Models(context.Background(), nil)
	if err != nil {
		t.Fatalf("listing models failed: %v", err)
	}
	for _, model := range models {
		if model.ID == placeholder {
			return
		}
	}
	t.Fatalf("the %q field suggests %q, which the catalog does not offer", execution.ModelFieldName, placeholder)
}

// A configuration that cannot be read is a reason the catalog is not
// obtainable, and the reason has to reach the caller as an error rather than as
// a panic or an empty list.
func TestModelListingReportsAnUnreadableConfiguration(t *testing.T) {
	models, err := (&Provider{}).Models(context.Background(), map[string]any{execution.ModelFieldName: true})
	if err == nil {
		t.Fatal("a non-string model was accepted: the caller would have no reason to show")
	}
	if models != nil {
		t.Fatalf("a failed listing still returned %d models", len(models))
	}
}

// A caller that sorts or trims the slice it got must not be able to corrupt the
// catalog every later caller reads.
func TestModelCatalogIsDetachedFromTheCaller(t *testing.T) {
	provider := &Provider{}
	first, err := provider.Models(context.Background(), nil)
	if err != nil {
		t.Fatalf("listing models failed: %v", err)
	}
	original := make([]execution.ModelOption, len(first))
	copy(original, first)
	for i := range first {
		first[i] = execution.ModelOption{ID: "mutated", Label: "mutated", Default: true}
	}
	second, err := provider.Models(context.Background(), nil)
	if err != nil {
		t.Fatalf("listing models a second time failed: %v", err)
	}
	if len(second) != len(original) {
		t.Fatalf("the catalog now holds %d entries, want %d", len(second), len(original))
	}
	for i, model := range second {
		if model != original[i] {
			t.Fatalf("entry %d is now %+v, want %+v", i, model, original[i])
		}
	}
}
