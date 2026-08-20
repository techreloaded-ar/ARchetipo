package claude

import (
	"context"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// models are the model aliases Claude Code accepts on `--model`. The list is
// closed on purpose and declared here: this package states what it knows,
// instead of interrogating the installed CLI, so the catalog is the same
// wherever ARchetipo runs and asking for it never costs a process.
//
// Verified against Claude Code 2.1.236. The aliases are a property of that
// CLI version; the Default marker is a best-effort hint about the model Claude
// Code picks when none is passed, which a person's own settings or plan can
// override, so it is a hint and not a verified fact about their machine.
//
// A declared list can fall behind the vendor. An identifier this list has not
// caught up with is never rejected: a value already configured outside the
// catalog stays selected and is saved unchanged.
// effortLevels are the values Claude Code accepts on `--effort`. The set is
// closed on purpose and declared here for the same reason the model aliases
// are: this package states what it knows instead of interrogating the installed
// CLI.
//
// Verified against Claude Code 2.1.236, whose `claude --help` documents
// `--effort <level>` with exactly these levels. The Default marker is a
// best-effort hint about the level Claude Code applies when no flag is passed,
// which a person's own settings or plan can override, so it is a hint and not a
// verified fact about their machine.
var effortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// defaultEffortLevel is the level the marker points at. It is only what the
// list reads as the provider's own default; leaving the option unset passes no
// flag at all.
const defaultEffortLevel = "medium"

// effortOption is the option the models that expose a reasoning budget declare.
// Its Name is a plain key of the provider configuration, so it must not collide
// with the keys of ConfigFields — and it deliberately is not one of them: an
// option of a model is not a setting of the provider, and declaring it in both
// places would draw it twice in the form.
var effortOption = execution.ModelOptionField{
	Name:    "effort",
	Label:   "Effort",
	Help:    "How much reasoning Claude spends on the run. Left empty, no effort flag is passed and Claude applies its own level.",
	Choices: effortChoices(),
}

func effortChoices() []execution.ModelOptionChoice {
	choices := make([]execution.ModelOptionChoice, 0, len(effortLevels))
	for _, level := range effortLevels {
		choices = append(choices, execution.ModelOptionChoice{
			Value:   level,
			Label:   level,
			Default: level == defaultEffortLevel,
		})
	}
	return choices
}

var models = []execution.ModelOption{
	{ID: "opus", Label: "opus", Options: []execution.ModelOptionField{effortOption}},
	{ID: "sonnet", Label: "sonnet", Default: true, Options: []execution.ModelOptionField{effortOption}},
	// haiku exposes no effort control, so it declares no option at all: it is
	// the case the panel renders with an explicit sentence instead of an empty
	// section.
	{ID: "haiku", Label: "haiku"},
}

// Models declares the catalog the model configuration field accepts.
//
// It validates the configuration first and forwards a parse error unchanged: a
// configuration that cannot be read is a legitimate reason for the catalog not
// to be obtainable, and the caller shows that reason as it is. On success the
// returned slice is detached, so a caller that mutates it cannot corrupt the
// catalog of this package.
func (p *Provider) Models(_ context.Context, raw map[string]any) ([]execution.ModelOption, error) {
	if _, err := parseConfig(raw); err != nil {
		return nil, err
	}
	return execution.CloneModels(models), nil
}

var _ execution.ModelLister = (*Provider)(nil)
