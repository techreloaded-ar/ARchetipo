package cli

import (
	"github.com/spf13/cobra"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
	"os"
	"os/signal"
	"syscall"
)

func newEnvironmentCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "environment", Short: "Run the project's explicit reusable environment and verification recipe"}
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
	for _, operation := range []string{"show", "configure", "prepare", "check", "cleanup"} {
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
