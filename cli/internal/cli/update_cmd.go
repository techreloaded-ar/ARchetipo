package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

const npmPackageName = "@techreloaded/archetipo"

func newUpdateCmd(s streams) *cobra.Command {
	var check bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the archetipo CLI to the latest version via npm",
		Long: "Runs `npm i -g " + npmPackageName + "@latest` to update the global installation.\n" +
			"Use --check to only compare versions, or --dry-run to see the command without running it.\n" +
			"This only updates the CLI: skills already copied into projects or ~/.{tool}/skills/ stay at their current version. Re-run `archetipo init` to refresh them.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runUpdate(s, check, dryRun)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Compare installed version against the npm registry latest and exit")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the npm command that would run without executing it")
	return cmd
}

func runUpdate(s streams, check, dryRun bool) error {
	if check {
		npmPath, err := exec.LookPath("npm")
		if err != nil {
			return npmNotFoundError(err)
		}
		latest, err := fetchLatestVersion(npmPath, 4*time.Second)
		if err != nil {
			return iox.NewConnector(iox.CodeConnectorNetwork, "cannot reach npm registry", "check internet connection or use --dry-run", err)
		}
		if latest == version.Version {
			fmt.Fprintf(s.out, "archetipo %s is up to date.\n", version.Version)
			return nil
		}
		fmt.Fprintf(s.out, "Update available: %s → %s\nRun: archetipo update\n", version.Version, latest)
		return nil
	}

	cmdLine := []string{"npm", "i", "-g", npmPackageName + "@latest"}
	if dryRun {
		fmt.Fprintln(s.out, strings.Join(cmdLine, " "))
		return nil
	}

	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return npmNotFoundError(err)
	}

	c := exec.Command(npmPath, cmdLine[1:]...)
	c.Stdin = s.in
	c.Stdout = s.out
	c.Stderr = s.err
	c.Env = os.Environ()
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			ce := iox.NewInternal(fmt.Sprintf("npm exited with status %d", ee.ExitCode()), err)
			ce.Exit = ee.ExitCode()
			return ce
		}
		return iox.NewInternal("npm invocation failed", err)
	}
	return nil
}

func npmNotFoundError(err error) error {
	return iox.NewPrecondition(
		"npm not found in PATH",
		"install Node.js or update manually with `npm i -g "+npmPackageName+"@latest`",
		err,
	)
}

func fetchLatestVersion(npmPath string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, npmPath, "view", npmPackageName, "version").Output()
	if err != nil {
		return "", err
	}
	latest := strings.TrimSpace(string(output))
	if latest == "" {
		return "", fmt.Errorf("npm returned an empty version")
	}
	return latest, nil
}
