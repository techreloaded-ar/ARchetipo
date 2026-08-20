package execution

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// choiceCatalog is the smallest catalog that exercises the four rules of the
// merge: one model that declares an option with a closed set of choices, and
// one that declares none and is the provider's own default. Pruning only has
// something to prove when the two models disagree about which keys belong in
// the configuration.
func choiceCatalog() []ModelOption {
	return []ModelOption{
		{
			ID:    "m1",
			Label: "Model One",
			Options: []ModelOptionField{
				{
					Name:  "opt",
					Label: "Option",
					Choices: []ModelOptionChoice{
						{Value: "a"},
						{Value: "b", Default: true},
					},
				},
			},
		},
		{ID: "m2", Label: "Model Two", Default: true},
	}
}

func catalogProvider() *modelListerProvider {
	return &modelListerProvider{testProvider: testProvider{id: "stub"}, models: choiceCatalog()}
}

func TestResolveModelChoiceInheritsConfiguredModel(t *testing.T) {
	resolution := ResolveModelChoice(context.Background(), catalogProvider(), map[string]any{"model": "m1", "opt": "a"})

	if !resolution.Declared {
		t.Fatal("a provider implementing ModelLister must be reported as declaring a catalog")
	}
	if resolution.Reason != "" {
		t.Fatalf("an obtainable catalog must carry no reason, got %q", resolution.Reason)
	}
	if resolution.Choice.Model != "m1" {
		t.Fatalf("resolved model is %q, want the configured %q", resolution.Choice.Model, "m1")
	}
	if !reflect.DeepEqual(resolution.Choice.Options, map[string]string{"opt": "a"}) {
		t.Fatalf("resolved options are %#v, want the configured {opt: a}", resolution.Choice.Options)
	}
	if resolution.Choice.Source != ModelChoiceSourceWorkspace {
		t.Fatalf("source is %q, want %q: nothing was chosen for this run", resolution.Choice.Source, ModelChoiceSourceWorkspace)
	}
}

func TestResolveModelChoiceFallsBackToCatalogDefault(t *testing.T) {
	resolution := ResolveModelChoice(context.Background(), catalogProvider(), map[string]any{})

	if resolution.Choice.Model != "m2" {
		t.Fatalf("resolved model is %q, want the catalog entry marked Default (%q)", resolution.Choice.Model, "m2")
	}
	if len(resolution.Choice.Options) != 0 {
		t.Fatalf("a model declaring no option must resolve to no options, got %#v", resolution.Choice.Options)
	}
}

func TestResolveModelChoiceReportsUnobtainableCatalog(t *testing.T) {
	unreadable := errors.New(`the command "stub" was not found on this machine`)
	provider := &modelListerProvider{testProvider: testProvider{id: "stub"}, err: unreadable}

	resolution := ResolveModelChoice(context.Background(), provider, map[string]any{"model": "m1"})

	if !resolution.Declared {
		t.Fatal("a catalog that failed must still be reported as declared, otherwise the reason has nowhere to be shown")
	}
	if len(resolution.Models) != 0 {
		t.Fatalf("got %d models alongside a failure, want 0", len(resolution.Models))
	}
	if resolution.Reason != unreadable.Error() {
		t.Fatalf("reason is %q, want the provider diagnostic verbatim (%q)", resolution.Reason, unreadable.Error())
	}
	if resolution.Choice.Model != "m1" {
		t.Fatalf("the configured model must still be reported when the catalog fails, got %q", resolution.Choice.Model)
	}
}

func TestResolveModelChoiceOnProviderWithoutCatalog(t *testing.T) {
	resolution := ResolveModelChoice(context.Background(), &testProvider{id: "plain"}, map[string]any{"model": "whatever"})

	if resolution.Declared {
		t.Fatal("a provider that does not implement ModelLister must not be reported as declaring a catalog")
	}
	if resolution.Reason != "" {
		t.Fatalf("no catalog at all is not a failure to obtain one, got reason %q", resolution.Reason)
	}
	if resolution.Choice.Model != "whatever" {
		t.Fatalf("the configured model is what the run uses, got %q", resolution.Choice.Model)
	}
}

func TestApplyModelChoiceWithoutOverrideKeepsConfigVerbatim(t *testing.T) {
	original := map[string]any{"model": "m1", "opt": "a", "command": "x"}
	want := map[string]any{"model": "m1", "opt": "a", "command": "x"}

	effective, choice, err := ApplyModelChoice(context.Background(), catalogProvider(), original, "", nil)
	if err != nil {
		t.Fatalf("a run started without a choice must not fail: %v", err)
	}
	if !reflect.DeepEqual(effective, want) {
		t.Fatalf("effective configuration is %#v, want the workspace one verbatim %#v", effective, want)
	}
	if choice.Source != ModelChoiceSourceWorkspace {
		t.Fatalf("source is %q, want %q", choice.Source, ModelChoiceSourceWorkspace)
	}
	if choice.Model != "m1" {
		t.Fatalf("choice model is %q, want the configured %q", choice.Model, "m1")
	}
	if !reflect.DeepEqual(original, want) {
		t.Fatalf("the caller's configuration was mutated: %#v", original)
	}
}

func TestApplyModelChoicePrunesOptionsOfOtherModels(t *testing.T) {
	original := map[string]any{"model": "m1", "opt": "a", "command": "x"}

	effective, choice, err := ApplyModelChoice(context.Background(), catalogProvider(), original, "m2", nil)
	if err != nil {
		t.Fatalf("choosing a catalog model must not fail: %v", err)
	}
	want := map[string]any{"model": "m2", "command": "x"}
	if !reflect.DeepEqual(effective, want) {
		t.Fatalf("effective configuration is %#v, want %#v: the option of the previous model must not survive", effective, want)
	}
	if choice.Source != ModelChoiceSourceRun {
		t.Fatalf("source is %q, want %q", choice.Source, ModelChoiceSourceRun)
	}
	if _, mutated := original["opt"]; !mutated {
		t.Fatalf("the caller's configuration lost its option: %#v", original)
	}
	if original["model"] != "m1" {
		t.Fatalf("the caller's configuration was overwritten: %#v", original)
	}
}

func TestApplyModelChoiceSetsRequestedOptions(t *testing.T) {
	original := map[string]any{"model": "m2", "command": "x"}

	effective, choice, err := ApplyModelChoice(context.Background(), catalogProvider(), original, "m1", map[string]string{"opt": "b"})
	if err != nil {
		t.Fatalf("choosing a declared option value must not fail: %v", err)
	}
	want := map[string]any{"model": "m1", "opt": "b", "command": "x"}
	if !reflect.DeepEqual(effective, want) {
		t.Fatalf("effective configuration is %#v, want %#v", effective, want)
	}
	if choice.Model != "m1" {
		t.Fatalf("choice model is %q, want %q", choice.Model, "m1")
	}
	if choice.Source != ModelChoiceSourceRun {
		t.Fatalf("source is %q, want %q", choice.Source, ModelChoiceSourceRun)
	}
	if !reflect.DeepEqual(choice.Options, map[string]string{"opt": "b"}) {
		t.Fatalf("choice options are %#v, want {opt: b}", choice.Options)
	}
}

func TestApplyModelChoiceRejectsUnknownModel(t *testing.T) {
	_, _, err := ApplyModelChoice(context.Background(), catalogProvider(), map[string]any{"model": "m1"}, "nope", nil)

	var configErr *ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("got %#v, want a *ConfigurationError", err)
	}
	if configErr.Field != ModelFieldName {
		t.Fatalf("the rejection names field %q, want %q", configErr.Field, ModelFieldName)
	}
}

func TestApplyModelChoiceRejectsUndeclaredOption(t *testing.T) {
	_, _, err := ApplyModelChoice(context.Background(), catalogProvider(), map[string]any{}, "m2", map[string]string{"opt": "a"})

	var configErr *ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("got %#v, want a *ConfigurationError", err)
	}
	if configErr.Field != "opt" {
		t.Fatalf("the rejection names field %q, want %q", configErr.Field, "opt")
	}
}

func TestApplyModelChoiceRejectsUnknownOptionValue(t *testing.T) {
	_, _, err := ApplyModelChoice(context.Background(), catalogProvider(), map[string]any{}, "m1", map[string]string{"opt": "z"})

	var configErr *ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("got %#v, want a *ConfigurationError", err)
	}
	if configErr.Field != "opt" {
		t.Fatalf("the rejection names field %q, want %q", configErr.Field, "opt")
	}
}

func TestApplyModelChoiceRefusesWhenCatalogUnavailable(t *testing.T) {
	t.Run("provider declaring no catalog", func(t *testing.T) {
		_, _, err := ApplyModelChoice(context.Background(), &testProvider{id: "plain"}, map[string]any{}, "m1", nil)

		var unavailable *ModelChoiceUnavailableError
		if !errors.As(err, &unavailable) {
			t.Fatalf("got %#v, want a *ModelChoiceUnavailableError", err)
		}
		if unavailable.Reason == "" {
			t.Fatal("the refusal must carry a reason a reader can act on")
		}
	})

	t.Run("catalog declared but not obtainable", func(t *testing.T) {
		unreadable := errors.New(`the command "stub" was not found on this machine`)
		provider := &modelListerProvider{testProvider: testProvider{id: "stub"}, err: unreadable}

		_, _, err := ApplyModelChoice(context.Background(), provider, map[string]any{}, "m1", nil)

		var unavailable *ModelChoiceUnavailableError
		if !errors.As(err, &unavailable) {
			t.Fatalf("got %#v, want a *ModelChoiceUnavailableError", err)
		}
		if unavailable.Reason != unreadable.Error() {
			t.Fatalf("reason is %q, want the provider diagnostic (%q)", unavailable.Reason, unreadable.Error())
		}
	})
}
