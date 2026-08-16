package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// setupResult is the `config show` payload: the connector's SetupInfo plus the
// workspace process Template. The Template does not depend on the connector, so
// it is assembled here instead of being threaded through all four connector
// implementations. The embedding inlines SetupInfo's JSON fields, so the
// envelope keeps every field it had and only gains `template`.
type setupResult struct {
	domain.SetupInfo
	Template config.TemplateConfig `json:"template"`
}

// newConfigCmd builds `archetipo config ...` as a command group. The only
// leaf today is `config show` (initialize_connector), but the group is in
// place to host future read/edit/validate sub-commands without breaking the
// surface again.
func newConfigCmd(s streams) *cobra.Command {
	root := &cobra.Command{
		Use:   "config",
		Short: "Configuration operations",
	}
	root.AddCommand(newConfigShowCmd(s))
	return root
}

// newConfigShowCmd implements `archetipo config show` -> initialize_connector.
//
// Output kind: "setup"
func newConfigShowCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Initialize the connector and emit metadata",
		Long:  "Authenticates (when applicable), detects repo/project, and prints connector metadata as JSON on stdout.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withConnectorCfg(cmd, s, "setup", func(ctx context.Context, cfg config.Config, c connector.Connector) (any, error) {
				info, err := c.InitializeConnector(ctx)
				if err != nil {
					return nil, err
				}
				return setupResult{SetupInfo: info, Template: cfg.Template}, nil
			})
		},
	}
}
