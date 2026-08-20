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
var models = []execution.ModelOption{
	{ID: "opus", Label: "opus"},
	{ID: "sonnet", Label: "sonnet", Default: true},
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
	out := make([]execution.ModelOption, len(models))
	copy(out, models)
	return out, nil
}

var _ execution.ModelLister = (*Provider)(nil)
