package codex

import (
	"context"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// models are the identifiers `thread/start` accepts as its `model` parameter,
// verified against codex-cli 0.147.0. The list is closed on purpose and is
// declared by this package rather than discovered by interrogating the CLI:
// asking the binary would make the configuration form depend on a process that
// may not be installed, and Codex exposes no enumeration of its models anyway.
// A declared list can fall behind the vendor, so an identifier outside this
// catalog is never rejected: a value already configured outside it stays
// selected and is saved unchanged.
// reasoningEfforts are the values this package offers for the reasoning budget
// of a thread. `codex app-server generate-json-schema` shows `thread/start`
// with a free-keyed `config` field and a `ReasoningEffort` type that is just a
// non-empty string, so the protocol declares no enumeration: the set is closed
// by choice of this package, exactly as the model catalog is, and the Default
// marker is a best-effort hint about the level Codex applies on its own, which
// a person's `~/.codex/config.toml` can override.
var reasoningEfforts = []string{"minimal", "low", "medium", "high"}

// defaultReasoningEffort is the level the marker points at. Leaving the option
// unset passes no override at all.
const defaultReasoningEffort = "medium"

// reasoningEffortOption is the option every model of this catalog declares. Its
// Name is a plain key of the provider configuration, in the same flat namespace
// as command, model, sandbox and timeout_seconds, so it must not collide with
// them — and it deliberately is not one of ConfigFields: an option of a model
// is not a setting of the provider, and declaring it in both places would draw
// it twice in the form.
var reasoningEffortOption = execution.ModelOptionField{
	Name:    "reasoning_effort",
	Label:   "Reasoning effort",
	Help:    "How much reasoning Codex spends on the run. Left empty, no override is sent and Codex applies its own setting.",
	Choices: reasoningEffortChoices(),
}

func reasoningEffortChoices() []execution.ModelOptionChoice {
	choices := make([]execution.ModelOptionChoice, 0, len(reasoningEfforts))
	for _, effort := range reasoningEfforts {
		choices = append(choices, execution.ModelOptionChoice{
			Value:   effort,
			Label:   effort,
			Default: effort == defaultReasoningEffort,
		})
	}
	return choices
}

var models = []execution.ModelOption{
	{ID: "gpt-5-codex", Label: "gpt-5-codex", Default: true, Options: []execution.ModelOptionField{reasoningEffortOption}},
	{ID: "gpt-5", Label: "gpt-5", Options: []execution.ModelOptionField{reasoningEffortOption}},
}

// Models declares the catalog the `model` configuration field accepts. An
// unreadable configuration is a legitimate reason for the catalog not to be
// obtainable, so the parse error is returned as it is: it names the offending
// field, which is exactly what the reader has to see.
func (p *Provider) Models(_ context.Context, raw map[string]any) ([]execution.ModelOption, error) {
	if _, err := parseConfig(raw); err != nil {
		return nil, err
	}
	return execution.CloneModels(models), nil
}

var _ execution.ModelLister = (*Provider)(nil)
