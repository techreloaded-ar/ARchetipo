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
var models = []execution.ModelOption{
	{ID: "gpt-5-codex", Label: "gpt-5-codex", Default: true},
	{ID: "gpt-5", Label: "gpt-5"},
}

// Models declares the catalog the `model` configuration field accepts. An
// unreadable configuration is a legitimate reason for the catalog not to be
// obtainable, so the parse error is returned as it is: it names the offending
// field, which is exactly what the reader has to see.
func (p *Provider) Models(_ context.Context, raw map[string]any) ([]execution.ModelOption, error) {
	if _, err := parseConfig(raw); err != nil {
		return nil, err
	}
	out := make([]execution.ModelOption, len(models))
	copy(out, models)
	return out, nil
}

var _ execution.ModelLister = (*Provider)(nil)
