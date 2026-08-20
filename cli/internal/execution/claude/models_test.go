package claude

import (
	"context"
	"reflect"
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
		if !reflect.DeepEqual(model, original[i]) {
			t.Fatalf("entry %d is now %+v, want %+v", i, model, original[i])
		}
	}
}

// --- model options ---------------------------------------------------------

// The option is what the panel draws under the model, so the catalog must
// declare it on the models that expose it — and only on those. The levels are
// asserted literally because they are the set `claude --help` documents: the
// list a person picks from is what this package claims the CLI accepts.
func TestCatalogDeclaresTheEffortOptionOnTheModelsThatExposeIt(t *testing.T) {
	models, err := (&Provider{}).Models(context.Background(), nil)
	if err != nil {
		t.Fatalf("listing models failed: %v", err)
	}
	byID := map[string]execution.ModelOption{}
	for _, model := range models {
		byID[model.ID] = model
	}
	wantLevels := []string{"low", "medium", "high", "xhigh", "max"}
	for _, id := range []string{"opus", "sonnet"} {
		model, ok := byID[id]
		if !ok {
			t.Fatalf("the catalog no longer offers %q", id)
		}
		if len(model.Options) != 1 {
			t.Fatalf("model %q declares %d options, want 1: %#v", id, len(model.Options), model.Options)
		}
		option := model.Options[0]
		if option.Name != "effort" {
			t.Fatalf("model %q declares option %q, want %q", id, option.Name, "effort")
		}
		if strings.TrimSpace(option.Label) == "" {
			t.Fatalf("the option of model %q has no label to read", id)
		}
		if strings.TrimSpace(option.Help) == "" {
			t.Fatalf("the option of model %q does not say what leaving it unset does", id)
		}
		if len(option.Choices) != len(wantLevels) {
			t.Fatalf("option %q offers %d choices, want %d: %#v", option.Name, len(option.Choices), len(wantLevels), option.Choices)
		}
		defaults := 0
		for i, level := range wantLevels {
			if option.Choices[i].Value != level {
				t.Fatalf("choice %d of %q is %q, want %q (declaration order must be preserved)", i, option.Name, option.Choices[i].Value, level)
			}
			if strings.TrimSpace(option.Choices[i].Label) == "" {
				t.Fatalf("choice %q has no label to read", level)
			}
			if option.Choices[i].Default {
				defaults++
			}
		}
		if defaults != 1 {
			t.Fatalf("option %q marks %d choices as the provider default, want exactly 1", option.Name, defaults)
		}
	}
	haiku, ok := byID["haiku"]
	if !ok {
		t.Fatal("the catalog no longer offers \"haiku\"")
	}
	if len(haiku.Options) != 0 {
		t.Fatalf("model %q declares %#v, want no option at all", haiku.ID, haiku.Options)
	}
}

// Every level the catalog offers has to be a level the parser accepts:
// offering a value the configuration then rejects would make the panel produce
// an error out of its own list.
func TestEveryOfferedEffortLevelIsAccepted(t *testing.T) {
	for _, choice := range effortOption.Choices {
		cfg, err := parseConfig(map[string]any{"effort": choice.Value})
		if err != nil {
			t.Fatalf("the catalog offers %q but the configuration rejects it: %v", choice.Value, err)
		}
		if cfg.Effort != choice.Value {
			t.Fatalf("effort = %q, want %q", cfg.Effort, choice.Value)
		}
	}
}
