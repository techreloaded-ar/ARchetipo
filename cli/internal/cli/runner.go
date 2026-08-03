package cli

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// streams bundles the I/O streams a sub-command operates on. cli.Execute
// passes the same streams down to every sub-command so tests can swap them.
type streams struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

// chdirFlag is the name of the global -C flag registered on the root command.
const chdirFlag = "chdir"

// loadConfigFor resolves the config for a command invocation. PersistentPreRunE
// has already chdir'd when -C was given, so cwd is always the requested start
// directory; an explicit -C additionally selects LoadExact, bypassing the
// nested-worktree guard, which is what keeps `archetipo -C <worktree> e2e run`
// operating on the worktree itself.
func loadConfigFor(cmd *cobra.Command) (config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return config.Config{}, iox.NewInternal("cwd unavailable", err)
	}
	load := config.Load
	if chdirRequested(cmd) {
		load = config.LoadExact
	}
	cfg, err := load(cwd)
	if err != nil {
		return config.Config{}, iox.NewInvalidInput(err.Error(), "fix .archetipo/config.yaml or remove it to fall back to defaults", err)
	}
	return cfg, nil
}

// chdirRequested reports whether the invocation carried an explicit -C. The
// flag is persistent on the root command, so every sub-command inherits it.
func chdirRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup(chdirFlag)
	return flag != nil && flag.Changed
}

// withConnector wires the standard plumbing (config load, connector build,
// JSON envelope on success or error) around a per-operation callback.
//
// The callback returns the kind tag and the data payload for the success
// envelope. On error, the failure is encoded to the error envelope on stderr
// and translated to an exit code by cli.Execute.
func withConnector(cmd *cobra.Command, s streams, kind string, fn func(ctx context.Context, c connector.Connector) (any, error)) error {
	cfg, err := loadConfigFor(cmd)
	if err != nil {
		return err
	}
	conn, err := connector.New(cfg)
	if err != nil {
		return iox.NewInvalidInput("connector unavailable", "check `connector:` in .archetipo/config.yaml", err)
	}
	data, err := fn(cmd.Context(), conn)
	if err != nil {
		return err
	}
	if err := iox.WriteOKWithWarnings(s.out, kind, data, cfg.ResolutionNotices); err != nil {
		return iox.NewInternal("encoding output", err)
	}
	return nil
}

// withConnectorCfg is like withConnector but also passes the loaded config to
// the callback. Used by operations that need config beyond the connector (e.g.
// the worktree workflow needs ProjectRoot and the worktree settings).
func withConnectorCfg(cmd *cobra.Command, s streams, kind string, fn func(ctx context.Context, cfg config.Config, c connector.Connector) (any, error)) error {
	cfg, err := loadConfigFor(cmd)
	if err != nil {
		return err
	}
	conn, err := connector.New(cfg)
	if err != nil {
		return iox.NewInvalidInput("connector unavailable", "check `connector:` in .archetipo/config.yaml", err)
	}
	data, err := fn(cmd.Context(), cfg, conn)
	if err != nil {
		return err
	}
	if err := iox.WriteOKWithWarnings(s.out, kind, data, cfg.ResolutionNotices); err != nil {
		return iox.NewInternal("encoding output", err)
	}
	return nil
}
