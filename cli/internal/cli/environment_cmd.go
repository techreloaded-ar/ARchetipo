package cli

import (
	"github.com/spf13/cobra"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func newEnvironmentCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "environment", Short: "Run the project's explicit reusable environment and verification recipe"}
	var assets []string
	publish := &cobra.Command{Use: "publish", Short: "Publish the common workspace recipe and explicit non-secret helpers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfigFor(cmd)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, verification.RecipePath))
		if err != nil {
			return err
		}
		if _, err = verification.Configure(cfg.ProjectRoot, raw); err != nil {
			return err
		}
		if raw, err = os.ReadFile(filepath.Join(cfg.ProjectRoot, preview.RecipePath)); err == nil {
			if _, err = preview.Configure(cfg.ProjectRoot, raw); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		result, err := environmentrecipe.Publish(cfg.ProjectRoot, assets)
		if err != nil {
			return err
		}
		return iox.WriteOK(s.out, "environment_recipe", result)
	}}
	publish.Flags().StringArrayVar(&assets, "asset", nil, "explicit non-secret helper under scripts/archetipo/; repeat for each file")
	root.AddCommand(publish)
	root.AddCommand(&cobra.Command{Use: "_run", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		code, err := verification.Supervise(cmd.Context(), s.in, s.out, s.err)
		if err == nil {
			return nil
		}
		if code < 1 {
			code = 1
		}
		return &iox.CodedError{Code: iox.CodeInternal, Message: "verification command ended unsuccessfully", Exit: code, Cause: err}
	}})
	root.AddCommand(&cobra.Command{Use: "_guard", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return verification.Guard(cmd.Context(), s.in, s.out)
	}})
	for _, operation := range []string{"show", "configure", "prepare", "check", "cleanup", "status", "run"} {
		var spec string
		var input string
		var fresh bool
		cmd := &cobra.Command{Use: operation, Args: cobra.NoArgs}
		if operation == "configure" {
			cmd.Flags().StringVar(&input, "file", "-", "version 1 project recipe JSON, or - for stdin")
		}
		if operation == "check" {
			cmd.Use += " <name>"
			cmd.Args = cobra.ExactArgs(1)
			cmd.Flags().StringVar(&spec, "spec", "", "write this check's complete evidence under the configured spec test-results directory")
			cmd.Flags().BoolVar(&fresh, "fresh", false, "run the check even when identical declared inputs already passed")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			var result any
			switch operation {
			case "configure":
				var raw []byte
				raw, err = readRawInput(s.in, input)
				if err == nil {
					result, err = verification.Configure(cfg.ProjectRoot, raw)
				}
			case "status":
				result, err = verification.New(cfg.ProjectRoot, cfg.Paths.TestResults).Resources()
			case "run":
				lifetime, ownErr := verification.OwnLifetime(cfg.ProjectRoot, s.err)
				if ownErr != nil {
					return ownErr
				}
				if lifetime == nil {
					return iox.NewPrecondition("environment run requires managed resources", "configure resources with start/probe/stop/stopped adapters", nil)
				}
				preparation, prepErr := lifetime.Service(cfg.Paths.TestResults).Execute(ctx, "prepare", "", "", false)
				if prepErr != nil || !preparation.Passed || !preparation.Stable {
					closeErr := lifetime.Close()
					if prepErr != nil {
						return prepErr
					}
					if closeErr != nil {
						return closeErr
					}
					return iox.WriteOK(s.out, "verification_result", preparation)
				}
				if err = iox.WriteOK(s.out, "verification_result", preparation); err != nil {
					_ = lifetime.Close()
					return err
				}
				select {
				case <-ctx.Done():
				case <-lifetime.Done():
				}
				return lifetime.Close()
			case "show":
				result, err = verification.Load(cfg.ProjectRoot)
			default:
				name := ""
				if len(args) != 0 {
					name = args[0]
				}
				result, err = verification.New(cfg.ProjectRoot, cfg.Paths.TestResults).Execute(ctx, operation, name, spec, fresh)
			}
			if err != nil {
				return err
			}
			return iox.WriteOKWithWarnings(s.out, "verification_result", result, cfg.ResolutionNotices)
		}
		root.AddCommand(cmd)
	}
	return root
}
