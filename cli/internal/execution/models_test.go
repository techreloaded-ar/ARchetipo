package execution

import (
	"context"
	"errors"
	"testing"
)

type modelListerProvider struct {
	testProvider
	models   []ModelOption
	err      error
	seen     map[string]any
	observed int
}

func (p *modelListerProvider) Models(_ context.Context, config map[string]any) ([]ModelOption, error) {
	p.observed++
	p.seen = config
	if config != nil {
		config["mutated"] = true
	}
	return p.models, p.err
}

func TestListModelsTreatsSilentProviderAsCatalogless(t *testing.T) {
	models, declared, err := ListModels(context.Background(), &testProvider{id: "plain"}, map[string]any{"model": "value"})
	if err != nil {
		t.Fatalf("a provider that declares no catalog must not fail, got %v", err)
	}
	if declared {
		t.Fatal("a provider that does not implement ModelLister must not be reported as declaring a catalog")
	}
	if len(models) != 0 {
		t.Fatalf("got %d models from a provider without a catalog, want 0", len(models))
	}
}

func TestListModelsReturnsTheDeclaredCatalog(t *testing.T) {
	provider := &modelListerProvider{
		testProvider: testProvider{id: "claude"},
		models: []ModelOption{
			{ID: "sonnet", Label: "Sonnet"},
			{ID: "opus", Default: true},
			{ID: "haiku"},
		},
	}
	models, declared, err := ListModels(context.Background(), provider, map[string]any{"command": "claude"})
	if err != nil {
		t.Fatalf("declared catalog reported %v", err)
	}
	if !declared {
		t.Fatal("a provider implementing ModelLister must be reported as declaring a catalog")
	}
	if provider.observed != 1 {
		t.Fatalf("catalog asked %d times, want 1", provider.observed)
	}
	want := []string{"sonnet", "opus", "haiku"}
	if len(models) != len(want) {
		t.Fatalf("got %d models, want %d: %#v", len(models), len(want), models)
	}
	for i, id := range want {
		if models[i].ID != id {
			t.Fatalf("model %d is %q, want %q (declaration order must be preserved)", i, models[i].ID, id)
		}
	}
	if models[0].Label != "Sonnet" {
		t.Fatalf("label lost: %#v", models[0])
	}
	if !models[1].Default {
		t.Fatalf("provider default lost: %#v", models[1])
	}
}

func TestListModelsPassesAClonedConfiguration(t *testing.T) {
	original := map[string]any{"command": "claude", "nested": map[string]any{"value": "before"}}
	provider := &modelListerProvider{testProvider: testProvider{id: "claude"}}
	if _, _, err := ListModels(context.Background(), provider, original); err != nil {
		t.Fatal(err)
	}
	if provider.seen["command"] != "claude" {
		t.Fatalf("configuration did not reach the provider: %#v", provider.seen)
	}
	if _, mutated := original["mutated"]; mutated {
		t.Fatal("the provider mutated the caller's configuration map")
	}
	provider.seen["nested"].(map[string]any)["value"] = "after"
	if original["nested"].(map[string]any)["value"] != "before" {
		t.Fatal("ListModels shared nested configuration values with the provider")
	}
}

func TestListModelsForwardsProviderErrorUnchanged(t *testing.T) {
	unreadable := errors.New(`the claude command "claude" was not found on this machine: exec: "claude": executable file not found in $PATH`)
	provider := &modelListerProvider{testProvider: testProvider{id: "claude"}, err: unreadable}
	models, declared, err := ListModels(context.Background(), provider, nil)
	if !errors.Is(err, unreadable) {
		t.Fatalf("provider diagnostic lost, got %v", err)
	}
	if err.Error() != unreadable.Error() {
		t.Fatalf("provider diagnostic reworded: %q", err.Error())
	}
	if !declared {
		t.Fatal("a catalog that failed must still be reported as declared, otherwise the reason has nowhere to be shown")
	}
	if len(models) != 0 {
		t.Fatalf("got %d models alongside an error, want 0", len(models))
	}
}

func TestListModelsNeverReturnsANilSliceOnSuccess(t *testing.T) {
	provider := &modelListerProvider{testProvider: testProvider{id: "claude"}, models: nil}
	models, declared, err := ListModels(context.Background(), provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !declared {
		t.Fatal("an empty catalog is still a declared catalog")
	}
	if models == nil {
		t.Fatal("a successful listing must return a non-nil slice")
	}
	if len(models) != 0 {
		t.Fatalf("got %d models, want 0", len(models))
	}
}

// optionCatalog is the shape US-048 is about: one model that declares an option
// with a closed set of choices, and one that declares none. Both cases exist in
// the real providers, and the panel renders them differently.
func optionCatalog() []ModelOption {
	return []ModelOption{
		{
			ID:    "sonnet",
			Label: "Sonnet",
			Options: []ModelOptionField{
				{
					Name:  "effort",
					Label: "Effort",
					Help:  "Left empty, no flag is passed.",
					Choices: []ModelOptionChoice{
						{Value: "low"},
						{Value: "medium", Default: true},
						{Value: "high"},
					},
				},
			},
		},
		{ID: "haiku", Label: "Haiku"},
	}
}

func TestListModelsCarriesTheOptionsDeclaredForEachModel(t *testing.T) {
	provider := &modelListerProvider{testProvider: testProvider{id: "claude"}, models: optionCatalog()}
	models, _, err := ListModels(context.Background(), provider, nil)
	if err != nil {
		t.Fatalf("declared catalog reported %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if len(models[0].Options) != 1 {
		t.Fatalf("model %q declares %d options, want 1", models[0].ID, len(models[0].Options))
	}
	option := models[0].Options[0]
	if option.Name != "effort" || option.Label != "Effort" {
		t.Fatalf("option identity lost: %#v", option)
	}
	if option.Help == "" {
		t.Fatal("the help text of the option was dropped")
	}
	wantChoices := []string{"low", "medium", "high"}
	if len(option.Choices) != len(wantChoices) {
		t.Fatalf("got %d choices, want %d: %#v", len(option.Choices), len(wantChoices), option.Choices)
	}
	for i, value := range wantChoices {
		if option.Choices[i].Value != value {
			t.Fatalf("choice %d is %q, want %q (declaration order must be preserved)", i, option.Choices[i].Value, value)
		}
	}
	defaults := 0
	for _, choice := range option.Choices {
		if choice.Default {
			defaults++
			if choice.Value != "medium" {
				t.Fatalf("the default marker moved to %q", choice.Value)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("%d choices are marked as default, want exactly 1", defaults)
	}
	if len(models[1].Options) != 0 {
		t.Fatalf("model %q must declare no option, got %#v", models[1].ID, models[1].Options)
	}
}

func TestListModelsDetachesTheOptionsItReturns(t *testing.T) {
	provider := &modelListerProvider{testProvider: testProvider{id: "claude"}, models: optionCatalog()}
	first, _, err := ListModels(context.Background(), provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	first[0].Options[0].Name = "mutated"
	first[0].Options[0].Choices[0].Value = "mutated"
	first[0].Options = nil

	second, _, err := ListModels(context.Background(), provider, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(second[0].Options) != 1 {
		t.Fatalf("mutating the result emptied the catalog: %#v", second[0])
	}
	if second[0].Options[0].Name != "effort" {
		t.Fatalf("option name is now %q, want %q", second[0].Options[0].Name, "effort")
	}
	if second[0].Options[0].Choices[0].Value != "low" {
		t.Fatalf("choice value is now %q, want %q", second[0].Options[0].Choices[0].Value, "low")
	}
}

func TestListModelsStillReportsACataloglessProviderWithOptionsInPlay(t *testing.T) {
	models, declared, err := ListModels(context.Background(), &testProvider{id: "plain"}, nil)
	if err != nil {
		t.Fatalf("a provider that declares no catalog must not fail, got %v", err)
	}
	if declared {
		t.Fatal("a provider that does not implement ModelLister must not be reported as declaring a catalog")
	}
	if len(models) != 0 {
		t.Fatalf("got %d models from a provider without a catalog, want 0", len(models))
	}
}
