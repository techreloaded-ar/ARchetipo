package execution

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ModelChoiceSourceWorkspace and ModelChoiceSourceRun are the two values Source
// takes.
const (
	// ModelChoiceSourceWorkspace marks a choice that was read from the saved
	// provider configuration.
	ModelChoiceSourceWorkspace = "workspace"
	// ModelChoiceSourceRun marks a choice that was made for one single run.
	ModelChoiceSourceRun = "run"
)

// ModelChoice is the model a run uses together with the options that model
// accepts.
//
// Source says *where the choice comes from*, not *what is configured*: a
// workspace choice and a run choice can name the very same model, and the
// difference the reader has to see is that one was inherited and the other was
// made for this run alone. It is therefore never derived by comparing the two
// models.
type ModelChoice struct {
	// Model is the catalog identifier the run uses. It is empty when neither
	// the configuration nor the catalog says which model applies.
	Model string `json:"model"`
	// Options are the values of the options the chosen model declares, keyed
	// by option name.
	Options map[string]string `json:"options,omitempty"`
	// Source is ModelChoiceSourceWorkspace or ModelChoiceSourceRun.
	Source string `json:"source"`
}

// ModelResolution is what a provider configuration says about the model a run
// would use, together with the state of the catalog behind it.
//
// It keeps apart the three situations a caller has to treat differently, with
// the same discipline as ListModels: no catalog at all (Declared false), a
// catalog that was declared but could not be obtained (Declared true with a
// non-empty Reason and no Models), and a catalog in hand (Declared true, empty
// Reason).
type ModelResolution struct {
	// Declared is false when the provider does not implement ModelLister.
	Declared bool
	// Models is the catalog, empty whenever it could not be obtained.
	Models []ModelOption
	// Reason is non-empty only when the catalog was declared but could not be
	// obtained; it is the provider's own diagnostic, verbatim.
	Reason string
	// Choice is the model and options the configuration resolves to, always
	// with Source ModelChoiceSourceWorkspace.
	Choice ModelChoice
}

// ModelChoiceUnavailableError says a per-run choice cannot be made at all,
// because the provider declares no catalog or its catalog could not be
// obtained. It is a distinct type from ConfigurationError so an HTTP caller can
// tell "choosing is not possible right now" (a state conflict) from "the choice
// is wrong" (invalid input) without reading any message text.
type ModelChoiceUnavailableError struct {
	Reason string
}

func (e *ModelChoiceUnavailableError) Error() string { return e.Reason }

// ResolveModelChoice reports which model and options a run started with this
// configuration would use.
//
// It returns no error on purpose: a catalog that cannot be obtained is a fact
// to report — the caller renders it as a reason — and not a failure of the
// question being asked. The effective model is the configured one when the
// configuration names it, otherwise the entry the catalog marks Default,
// otherwise nothing at all.
func ResolveModelChoice(ctx context.Context, provider Provider, config map[string]any) ModelResolution {
	models, declared, err := ListModels(ctx, provider, config)
	resolution := ModelResolution{Declared: declared, Models: models}
	if resolution.Models == nil {
		resolution.Models = []ModelOption{}
	}
	if err != nil {
		resolution.Models = []ModelOption{}
		resolution.Reason = err.Error()
	}

	model := configString(config, ModelFieldName)
	if model == "" {
		for _, candidate := range resolution.Models {
			if candidate.Default {
				model = candidate.ID
				break
			}
		}
	}

	resolution.Choice = ModelChoice{
		Model:   model,
		Options: configuredOptions(config, resolution.Models, model),
		Source:  ModelChoiceSourceWorkspace,
	}
	return resolution
}

// ApplyModelChoice merges a per-run choice onto a provider configuration and
// returns the configuration the run must use, plus the choice it represents.
//
// With no model and no options it is a read: the configuration comes back
// verbatim and the choice is the workspace one, so a run started without an
// explicit choice is byte-for-byte the run the workspace configures. With an
// override it prunes *every* option name the catalog declares before writing
// the ones the chosen model accepts, so options belonging to the previous model
// cannot survive into the new one.
func ApplyModelChoice(ctx context.Context, provider Provider, config map[string]any, model string, options map[string]string) (map[string]any, ModelChoice, error) {
	model = strings.TrimSpace(model)
	resolution := ResolveModelChoice(ctx, provider, config)
	if model == "" && len(options) == 0 {
		return CloneConfig(config), resolution.Choice, nil
	}

	if !resolution.Declared {
		return nil, ModelChoice{}, &ModelChoiceUnavailableError{
			Reason: "provider " + provider.ID() + " declares no model catalog",
		}
	}
	if resolution.Reason != "" {
		return nil, ModelChoice{}, &ModelChoiceUnavailableError{Reason: resolution.Reason}
	}

	chosen, ok := findModel(resolution.Models, model)
	if !ok {
		return nil, ModelChoice{}, &ConfigurationError{
			Field:  ModelFieldName,
			Reason: "must be one of " + strings.Join(modelIDs(resolution.Models), ", "),
		}
	}

	declaredByModel := make(map[string]ModelOptionField, len(chosen.Options))
	for _, option := range chosen.Options {
		declaredByModel[option.Name] = option
	}
	for _, name := range sortedKeys(options) {
		option, known := declaredByModel[name]
		if !known {
			return nil, ModelChoice{}, &ConfigurationError{
				Field:  name,
				Reason: "is not an option of model " + model,
			}
		}
		if !hasChoice(option.Choices, options[name]) {
			return nil, ModelChoice{}, &ConfigurationError{
				Field:  name,
				Reason: "must be one of " + strings.Join(choiceValues(option.Choices), ", "),
			}
		}
	}

	effective := CloneConfig(config)
	for name := range declaredOptionNames(resolution.Models) {
		delete(effective, name)
	}
	effective[ModelFieldName] = model
	choice := ModelChoice{Model: model, Source: ModelChoiceSourceRun}
	if len(options) > 0 {
		choice.Options = make(map[string]string, len(options))
		for name, value := range options {
			effective[name] = value
			choice.Options[name] = value
		}
	}
	return effective, choice, nil
}

// declaredOptionNames is the union of the option names of every entry of a
// catalog. It is what makes the pruning complete: the options to remove are not
// the ones of the previously configured model — which may itself have fallen
// out of the catalog — but every name the catalog can ever write.
func declaredOptionNames(models []ModelOption) map[string]struct{} {
	names := make(map[string]struct{})
	for _, model := range models {
		for _, option := range model.Options {
			names[option.Name] = struct{}{}
		}
	}
	return names
}

// configuredOptions reads from a configuration the values of the options the
// given model declares, and only those. A value stored as something other than
// a string is rendered as one, because the option vocabulary is textual.
func configuredOptions(config map[string]any, models []ModelOption, model string) map[string]string {
	chosen, ok := findModel(models, model)
	if !ok || len(chosen.Options) == 0 {
		return nil
	}
	options := make(map[string]string)
	for _, option := range chosen.Options {
		value, present := config[option.Name]
		if !present || value == nil {
			continue
		}
		options[option.Name] = valueToString(value)
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

func findModel(models []ModelOption, id string) (ModelOption, bool) {
	if id == "" {
		return ModelOption{}, false
	}
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return ModelOption{}, false
}

func modelIDs(models []ModelOption) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func choiceValues(choices []ModelOptionChoice) []string {
	values := make([]string, 0, len(choices))
	for _, choice := range choices {
		values = append(values, choice.Value)
	}
	return values
}

func hasChoice(choices []ModelOptionChoice, value string) bool {
	for _, choice := range choices {
		if choice.Value == value {
			return true
		}
	}
	return false
}

// sortedKeys puts the option names in a deterministic order, so the field a
// validation message names is the same on every run instead of following Go's
// map iteration.
func sortedKeys(options map[string]string) []string {
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func configString(config map[string]any, key string) string {
	value, ok := config[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func valueToString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}
