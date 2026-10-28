package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
)

func newPreviewCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "preview", Short: "Configure and run the project's explicit browser preview"}
	root.AddCommand(&cobra.Command{Use: "_run", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return preview.Supervise(ctx, s.in, s.out, s.err)
	}})
	for _, operation := range []string{"show", "configure", "prepare", "run", "status", "stop"} {
		var input string
		var candidate bool
		cmd := &cobra.Command{Use: operation, Args: cobra.NoArgs}
		if operation == "configure" {
			cmd.Flags().StringVar(&input, "file", "-", "version 1 preview recipe JSON, or - for stdin")
			cmd.Flags().BoolVar(&candidate, "candidate", false, "configure only the local candidate without updating the common recipe")
		}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			if operation == "status" || operation == "stop" {
				manager := preview.NewManager()
				defer manager.Close()
				if operation == "stop" {
					if err = manager.StopEnvironment(cfg.ProjectRoot, "standalone"); err != nil {
						return err
					}
				}
				view, statusErr := manager.Status(cfg.ProjectRoot, "standalone")
				if statusErr != nil {
					return statusErr
				}
				return iox.WriteOK(s.out, "preview", view)
			}
			if operation == "run" || operation == "prepare" {
				ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
				defer stop()
				manager := preview.NewManager()
				defer manager.Close()
				var startErr error
				if operation == "prepare" {
					_, startErr = manager.Prepare(cfg.ProjectRoot, "standalone")
				} else {
					_, startErr = manager.Start(cfg.ProjectRoot, "standalone", "")
				}
				if startErr != nil {
					return startErr
				}
				announced := false
				tick := time.NewTicker(200 * time.Millisecond)
				defer tick.Stop()
				for {
					view, statusErr := manager.Status(cfg.ProjectRoot, "standalone")
					if statusErr != nil {
						return statusErr
					}
					if view.State == "error" {
						hint := "inspect the preview server output: " + view.Log
						switch view.ErrorCode {
						case iox.CodeConflict:
							return iox.NewConflict(view.Error, hint, nil)
						case iox.CodeInvalidInput:
							return iox.NewInvalidInput(view.Error, hint, nil)
						case iox.CodeInternal:
							return iox.NewInternal(view.Error, nil)
						default:
							return iox.NewPrecondition(view.Error, hint, fmt.Errorf("preview server failed"))
						}
					}
					if view.State == "stopped" {
						if operation == "prepare" || !announced {
							return iox.WriteOK(s.out, "preview", view)
						}
						return nil
					}
					if view.State == "ready" && !announced {
						if err = iox.WriteOK(s.out, "preview", view); err != nil {
							return err
						}
						announced = true
					}
					select {
					case <-ctx.Done():
						return nil
					case <-tick.C:
					}
				}
			}
			var recipe preview.Recipe
			if operation == "configure" {
				raw, readErr := readRawInput(s.in, input)
				if readErr != nil {
					return readErr
				}
				if candidate {
					recipe, err = preview.ConfigureCandidate(cfg.ProjectRoot, raw)
				} else {
					recipe, err = preview.Configure(cfg.ProjectRoot, raw)
				}
			} else {
				recipe, err = preview.Load(cfg.ProjectRoot)
			}
			if err != nil {
				return err
			}
			return iox.WriteOK(s.out, "preview_recipe", recipe)
		}
		root.AddCommand(cmd)
	}
	return root
}
