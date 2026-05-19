package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
)

// newInitCmd implements `archetipo init` -> initialize_connector.
//
// Output kind: "setup"
func newInitCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the connector and emit metadata",
		Long:  "Authenticates (when applicable), detects repo/project, and prints connector metadata as JSON on stdout.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withConnector(cmd, s, "setup", func(ctx context.Context, c connector.Connector) (any, error) {
				return c.InitializeConnector(ctx)
			})
		},
	}
}
