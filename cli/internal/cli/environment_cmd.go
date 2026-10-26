package cli

import (
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

func newEnvironmentCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "environment", Short: "Run the project's explicit reusable environment and verification recipe"}
	var assets []string
	var qualified bool
	publish := &cobra.Command{Use: "publish", Short: "Publish the common workspace recipe and explicit non-secret helpers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfigFor(cmd)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, verification.RecipePath))
		if err != nil {
			return err
		}
		if qualified {
			if err = validateCandidatePreview(cfg.ProjectRoot); err != nil {
				return err
			}
			q, verifyErr := verification.New(cfg.ProjectRoot, cfg.Paths.TestResults).VerifyQualification(assets)
			if verifyErr != nil {
				return verifyErr
			}
			result, publishErr := environmentrecipe.PublishQualified(cfg.ProjectRoot, assets, q.BundleDigest)
			if publishErr != nil {
				return publishErr
			}
			return iox.WriteOK(s.out, "environment_recipe", result)
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
	publish.Flags().BoolVar(&qualified, "qualified", false, "publish only the exact local candidate with retained setup qualification evidence")
	root.AddCommand(publish)
	root.AddCommand(newEnvironmentCapabilitiesCmd(s), newEnvironmentQualifyCmd(s))
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
	for _, operation := range []string{"show", "configure", "prepare", "check", "cleanup", "status", "evidence", "run"} {
		var spec string
		var input string
		var fresh bool
		var candidate bool
		cmd := &cobra.Command{Use: operation, Args: cobra.NoArgs}
		if operation == "configure" || operation == "prepare" || operation == "check" {
			cmd.Flags().BoolVar(&candidate, "candidate", false, "use the local candidate without reading or updating the shared bundle")
		}
		if operation == "prepare" {
			cmd.Flags().BoolVar(&fresh, "fresh", false, "execute preparation even if retained evidence is reusable")
		}
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
					if candidate {
						result, err = verification.ConfigureCandidate(cfg.ProjectRoot, raw)
					} else {
						result, err = verification.Configure(cfg.ProjectRoot, raw)
					}
				}
			case "evidence":
				result, err = verification.New(cfg.ProjectRoot, cfg.Paths.TestResults).Evidence(ctx)
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
				service := verification.New(cfg.ProjectRoot, cfg.Paths.TestResults)
				if candidate {
					service.ForCandidate()
				}
				result, err = service.Execute(ctx, operation, name, spec, fresh)
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

func newEnvironmentCapabilitiesCmd(s streams) *cobra.Command {
	var tools []string
	var input string
	cmd := &cobra.Command{Use: "capabilities", Short: "Observe this invoking runtime; never infer a project stack or daemon access", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfigFor(cmd)
		if err != nil {
			return err
		}
		var probes []verification.Command
		if input != "" {
			raw, readErr := readRawInput(s.in, input)
			if readErr != nil {
				return readErr
			}
			if err = json.Unmarshal(raw, &probes); err != nil {
				return iox.NewInvalidInput("expected an array of bounded capability probe commands", "", err)
			}
		}
		result, err := verification.New(cfg.ProjectRoot, cfg.Paths.TestResults).Inspect(cmd.Context(), tools, probes)
		if err != nil {
			return err
		}
		return iox.WriteOK(s.out, "environment_capabilities", result)
	}}
	cmd.Flags().StringArrayVar(&tools, "tool", nil, "explicit executable to locate; repeat after inspecting project requirements")
	cmd.Flags().StringVar(&input, "probe-file", "", "JSON array of explicit non-destructive commands, each bounded to 30 seconds; - for stdin")
	return cmd
}

func newEnvironmentQualifyCmd(s streams) *cobra.Command {
	var assets []string
	cmd := &cobra.Command{Use: "qualify", Short: "Test a local candidate twice and verify warm reuse before common publication", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfigFor(cmd)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err = validateCandidatePreview(cfg.ProjectRoot); err != nil {
			return err
		}
		lifetime, err := verification.OwnCandidateLifetime(cfg.ProjectRoot, s.err)
		if err != nil {
			return err
		}
		service := verification.New(cfg.ProjectRoot, cfg.Paths.TestResults).ForCandidate()
		if lifetime != nil {
			service = lifetime.Service(cfg.Paths.TestResults)
		}
		result, runErr := service.Qualify(ctx, assets)
		if lifetime != nil {
			var closeErr error
			if runErr == nil && result.Passed {
				closeErr = lifetime.Release()
			} else {
				closeErr = lifetime.Close()
			}
			if runErr == nil {
				runErr = closeErr
			}
		}
		if runErr != nil {
			return runErr
		}
		return iox.WriteOK(s.out, "environment_qualification", result)
	}}
	cmd.Flags().StringArrayVar(&assets, "asset", nil, "every explicit non-secret helper in the candidate bundle; repeat")
	return cmd
}

func validateCandidatePreview(root string) error {
	_, err := preview.LoadLocal(root)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
