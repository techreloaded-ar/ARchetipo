package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/e2e"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// newE2ECmd builds `archetipo e2e ...`: deterministic helpers for end-to-end
// test setup. Today only Playwright is supported.
func newE2ECmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "e2e", Short: "End-to-end testing helpers"}
	root.AddCommand(newE2EDetectCmd(s), newE2EEnsureCmd(s), newE2ERunCmd(s), newE2EDemoCmd(s))
	return root
}

// projectRoot loads the config from cwd and returns its ProjectRoot. config.Load
// falls back to a default rooted at cwd when no .archetipo/config.yaml exists,
// so this never fails just because the project is not initialized.
func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", iox.NewInternal("cwd unavailable", err)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return "", iox.NewInvalidInput(err.Error(), "fix .archetipo/config.yaml or remove it to fall back to defaults", err)
	}
	return cfg.ProjectRoot, nil
}

func newE2EDetectCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Report the e2e framework state of the project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return err
			}
			det, err := e2e.Detect(root)
			if err != nil {
				return iox.NewInternal("detecting e2e framework", err)
			}
			return iox.WriteOK(s.out, "e2e_detection", det)
		},
	}
}

func newE2EEnsureCmd(s streams) *cobra.Command {
	var withDeps bool
	var browser string
	cmd := &cobra.Command{
		Use:   "ensure",
		Short: "Idempotently bootstrap Playwright (non-interactive, single browser)",
		Long: "Installs @playwright/test when missing, writes a minimal config when absent " +
			"(never overwriting an existing one) and installs a single browser. " +
			"Non-interactive and idempotent: safe to run repeatedly.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return err
			}
			res, err := e2e.Ensure(cmd.Context(), e2e.EnsureOptions{
				ProjectRoot: root,
				Browser:     browser,
				WithDeps:    withDeps,
			})
			if err != nil {
				return err
			}
			return iox.WriteOK(s.out, "e2e_ensure", res)
		},
	}
	cmd.Flags().StringVar(&browser, "browser", e2e.DefaultBrowser, "browser to install")
	cmd.Flags().BoolVar(&withDeps, "with-deps", false, "also install OS-level browser dependencies (may require sudo)")
	return cmd
}

func newE2ERunCmd(s streams) *cobra.Command {
	var grep string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Playwright functional suite headless (no video)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := projectRoot()
			if err != nil {
				return err
			}
			res, err := e2e.RunFunctional(cmd.Context(), e2e.RunOptions{ProjectRoot: root, Grep: grep})
			if err != nil {
				return err
			}
			return iox.WriteOK(s.out, "e2e_run", res)
		},
	}
	cmd.Flags().StringVar(&grep, "grep", "", "only run tests matching this pattern")
	return cmd
}

func newE2EDemoCmd(s streams) *cobra.Command {
	var spec, grep string
	cmd := &cobra.Command{
		Use:   "demo --spec US-XXX --grep <demo>",
		Short: "Record a watchable demo video for one scenario",
		Long: "Runs a single demo test with deterministic recording (video on, slow motion, " +
			"fixed viewport) and collects the video under <test_results>/<spec>/. The recording " +
			"settings are injected via an ephemeral config, so the demo test file stays a plain scenario.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return iox.NewInternal("cwd unavailable", err)
			}
			cfg, err := config.Load(cwd)
			if err != nil {
				return iox.NewInvalidInput(err.Error(), "fix .archetipo/config.yaml or remove it to fall back to defaults", err)
			}
			res, err := e2e.RecordDemo(cmd.Context(), e2e.DemoOptions{
				ProjectRoot:    cfg.ProjectRoot,
				Spec:           spec,
				Grep:           grep,
				TestResultsDir: cfg.Paths.TestResults,
			})
			if err != nil {
				return err
			}
			return iox.WriteOK(s.out, "e2e_demo", res)
		},
	}
	cmd.Flags().StringVar(&spec, "spec", "", "spec code (US-XXX); used as the artifact subfolder")
	cmd.Flags().StringVar(&grep, "grep", "", "pattern selecting the single demo test to record")
	return cmd
}
