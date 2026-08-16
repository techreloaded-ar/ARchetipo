package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	// Concrete connectors register themselves via init().
	_ "github.com/techreloaded-ar/ARchetipo/cli/internal/connector/builtin"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/arcipelago"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

// Execute runs the archetipo CLI with the given args and returns the process
// exit code. Stdin/stdout/stderr are taken as parameters so tests can drive the
// CLI without touching the real OS streams.
//
// Exit codes follow the public CLI runtime contract:
//
//	0  ok
//	1  generic error
//	2  input/validation error
//	3  connector error (auth, network, gh)
//	4  precondition missing (e.g. backlog absent)
//
// On error, the JSON envelope is written to stderr exactly once: sub-commands
// return typed errors and Execute serializes them, so handlers don't have to.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	notifier := version.NewNotifier(version.NotifierConfig{
		PackageName: npmPackageName,
		UpdateCmd:   "archetipo update",
		CacheTTL:    24 * time.Hour,
		HTTPTimeout: 2 * time.Second,
	}, version.Version)
	notifier.Start()
	defer notifier.Print(stderr)

	root := newRootCmd(stdin, stdout, stderr)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		iox.WriteError(stderr, err)
		return exitCodeFor(err)
	}
	return 0
}

// defaultExecutionRegistry is the registry the real CLI runs with. Registering
// a single static provider cannot fail — the registry is fresh and the id is a
// non-empty constant — so the error is deliberately discarded rather than
// turned into an unreachable failure path.
func defaultExecutionRegistry() *execution.Registry {
	registry := execution.NewRegistry()
	_ = registry.Register(arcipelago.New(arcipelago.Options{}))
	return registry
}

func newRootCmd(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	return newRootCmdWithExecution(stdin, stdout, stderr, executionDependencies{
		registry: defaultExecutionRegistry(),
		newID:    execution.RandomID,
		now:      time.Now,
		storeFactory: func(projectRoot string) (execution.Store, error) {
			return execution.NewFileStore(projectRoot)
		},
	})
}

func newRootCmdWithExecution(stdin io.Reader, stdout, stderr io.Writer, deps executionDependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "archetipo",
		Short:         "ARchetipo connector CLI",
		Long:          "Deterministic CLI implementing the ARchetipo workflow operations (file and github).",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}
	cmd.SetVersionTemplate(versionLine())
	cmd.SetContext(context.Background())

	// -C works like `git -C`: a real chdir before the sub-command runs, so every
	// relative path (--file, git invocations, the connector) behaves exactly as
	// if the command had been launched from there. It is also the explicit
	// override for the nested-worktree guard in config.Load (see loadConfigFor):
	// an implicit cwd is not trusted, an explicit root is.
	cmd.PersistentFlags().StringP(chdirFlag, "C", "", "run as if started from <dir> (resolution root, nested-worktree guard disabled)")
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		dir, err := c.Flags().GetString(chdirFlag)
		if err != nil {
			return iox.NewInternal("reading --"+chdirFlag, err)
		}
		if dir == "" {
			return nil
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return iox.NewInvalidInput("invalid --"+chdirFlag+" directory: "+dir, "pass an existing directory", err)
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return iox.NewInvalidInput("--"+chdirFlag+" directory not found: "+abs, "pass an existing directory", err)
		}
		if err := os.Chdir(abs); err != nil {
			return iox.NewInvalidInput("cannot enter --"+chdirFlag+" directory: "+abs, "pass an existing directory", err)
		}
		return nil
	}

	s := streams{in: stdin, out: stdout, err: stderr}
	cmd.AddCommand(
		newConfigCmd(s),
		newDoctorCmd(s),
		newInitProjectCmd(s),
		newUninstallCmd(s),
		newUpdateCmd(s),
		newPRDCmd(s),
		newSpecCmd(s),
		newE2ECmd(s),
		newExecutionCmd(s, deps),
		newMetricsCmd(s),
		newTaskCmd(s),
		newValidateCmd(s),
		newViewCmd(s),
		newWikiCmd(s),
		newVersionCmd(s),
	)
	return cmd
}

// exitCodeFor maps a returned error to the documented exit code. Specific
// error types defined in internal/iox carry their own code; everything else is
// a generic error (1).
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	if coded, ok := err.(interface{ ExitCode() int }); ok {
		return coded.ExitCode()
	}
	return 1
}
