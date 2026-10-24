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
	var confirmed bool
	var archiveConversations bool
	var expectedHead string
	deletionPlan := &cobra.Command{Use: "deletion-plan <id>", Short: "Inspect the worktree and unintegrated work before deleting an increment", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigFor(cmd)
		if err != nil {
			return err
		}
		if cfg.Increment != nil {
			return iox.NewPrecondition("inspect deletion from the parent workspace", "do not remove the current working directory", nil)
		}
		conn, err := connector.New(cfg)
		if err != nil {
			return err
		}
		result, err := increment.New(cfg, conn).DeletionPlan(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return iox.WriteOK(s.out, "increment_deletion_plan", result)
	}}
	deletion := &cobra.Command{Use: "delete <id>", Short: "Retain committed work in a Git ref and remove an increment, its worktree and branch", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigFor(cmd)
		if err != nil {
			return err
		}
		if cfg.Increment != nil {
			return iox.NewPrecondition("delete from the parent workspace", "do not remove the current working directory", nil)
		}
		conn, err := connector.New(cfg)
		if err != nil {
			return err
		}
		result, err := increment.New(cfg, conn).Delete(cmd.Context(), args[0], increment.DeletionOptions{HeadSHA: expectedHead, Confirm: confirmed, PreserveConversations: archiveConversations})
		if err != nil {
			return err
		}
		return iox.WriteOK(s.out, "increment_deletion", result)
	}}
	deletion.Flags().BoolVar(&archiveConversations, "archive-conversations", false, "retain linked conversations as readable archives after removing their environment")
	deletion.Flags().BoolVar(&confirmed, "confirm", false, "explicitly remove this increment and its local files, retaining only committed work in Git")
	deletion.Flags().StringVar(&expectedHead, "head", "", "head_sha returned by increment deletion-plan")
	root.AddCommand(deletionPlan, deletion)
	for _, operation := range []string{"create", "show", "list", "propose", "approve", "ready", "feedback", "publish", "refresh", "learn", "conclude"} {
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
					Observation          string   `json:"observation"`
					Decision             string   `json:"decision"`
					DevelopmentMode      string   `json:"development_mode"`
					Title                string   `json:"title"`
					Request              string   `json:"request"`
					OriginConversationID string   `json:"origin_conversation_id"`
					Revision             int      `json:"revision"`
					Feedback             string   `json:"feedback"`
					Kind                 string   `json:"kind"`
					StoryCodes           []string `json:"story_codes" yaml:"story_codes"`
					HeadSHA              string   `json:"head_sha"`
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
					case "learn", "conclude":
						value, err = service.Learn(cmd.Context(), args[0], input.Observation, input.Decision, operation == "conclude")
					case "create":
						value, err = service.CreateWithMode(cmd.Context(), input.Title, input.Request, input.OriginConversationID, input.DevelopmentMode)
					case "approve":
						value, err = service.Approve(cmd.Context(), args[0], input.Revision)
					case "feedback":
						value, err = service.RequestChanges(cmd.Context(), args[0], increment.FeedbackRequest{Feedback: input.Feedback, Kind: input.Kind, StoryCodes: input.StoryCodes})
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
