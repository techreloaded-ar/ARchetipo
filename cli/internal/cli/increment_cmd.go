package cli

import (
	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func newIncrementCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "increment", Short: "Develop an isolated product increment"}
	for _, operation := range []string{"create", "show", "list", "propose", "approve", "ready", "feedback", "publish", "refresh"} {
		var file string
		cmd := &cobra.Command{Use: operation, Short: operation + " a product increment"}
		if operation == "create" || operation == "list" {
			cmd.Args = cobra.NoArgs
		} else {
			cmd.Use += " <id>"
			cmd.Args = cobra.ExactArgs(1)
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			if cfg.Increment != nil {
				// Standalone skills inside the sandbox address its owning increment,
				// while every ordinary spec command stays in the local file workspace.
				ref := cfg.Increment
				if operation == "create" {
					return iox.NewConflict("cannot create an increment inside another increment", "open the original workspace", nil)
				}
				if len(args) > 0 && args[0] != ref.ID {
					return iox.NewConflict("increment is outside this sandbox", "use its own increment id", nil)
				}
				cfg, err = config.LoadExactReadOnly(ref.ParentRoot)
				if err != nil {
					return err
				}
			}
			conn, err := connector.New(cfg)
			if err != nil {
				return err
			}
			service := increment.New(cfg, conn)
			var value any
			switch operation {
			case "list":
				value, err = service.List()
			case "show":
				value, err = service.Read(args[0])
			case "refresh":
				value, err = service.Refresh(cmd.Context(), args[0])
			default:
				var input struct {
					Title                string `json:"title"`
					Request              string `json:"request"`
					OriginConversationID string `json:"origin_conversation_id"`
					Revision             int    `json:"revision"`
					Feedback             string `json:"feedback"`
					HeadSHA              string `json:"head_sha"`
				}
				// Proposals and verification results retain their existing domain shapes.
				switch operation {
				case "propose":
					var proposal increment.Proposal
					if err = readStructuredInput(s.in, file, &proposal); err != nil {
						return err
					}
					value, err = service.Propose(cmd.Context(), args[0], proposal)
				case "ready":
					var result increment.Result
					if err = readStructuredInput(s.in, file, &result); err != nil {
						return err
					}
					value, err = service.Ready(cmd.Context(), args[0], result)
				default:
					if err = readStructuredInput(s.in, file, &input); err != nil {
						return err
					}
					switch operation {
					case "create":
						value, err = service.Create(cmd.Context(), input.Title, input.Request, input.OriginConversationID)
					case "approve":
						value, err = service.Approve(cmd.Context(), args[0], input.Revision)
					case "feedback":
						value, err = service.FeedbackFor(cmd.Context(), args[0], input.Feedback)
					case "publish":
						value, err = service.Publish(cmd.Context(), args[0], input.HeadSHA, input.Revision)
					}
				}
			}
			if err != nil {
				return err
			}
			return iox.WriteOK(s.out, "increment", value)
		}
		if operation != "list" && operation != "show" && operation != "refresh" {
			cmd.Flags().StringVar(&file, "file", "-", "JSON payload path, or - for stdin")
		}
		root.AddCommand(cmd)
	}
	return root
}
