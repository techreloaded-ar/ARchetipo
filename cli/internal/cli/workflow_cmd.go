package cli

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workflowrun"
	"io"
)

func newWorkflowCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "workflow", Short: "Persist and resume skill execution checkpoints"}
	for _, op := range []string{"start", "show", "list", "checkpoint"} {
		var file string
		cmd := &cobra.Command{Use: op, Args: cobra.NoArgs}
		if op == "show" || op == "checkpoint" {
			cmd.Use += " <id>"
			cmd.Args = cobra.ExactArgs(1)
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigFor(cmd)
			if err != nil {
				return err
			}
			store := workflowrun.New(cfg.ProjectRoot)
			var value any
			switch op {
			case "list":
				value, err = store.List()
			case "show":
				value, err = store.Read(args[0])
			case "start":
				var input workflowrun.Start
				if err = readWorkflowInput(s.in, file, &input); err == nil {
					value, err = store.Start(cmd.Context(), input)
				}
			case "checkpoint":
				var input workflowrun.Checkpoint
				if err = readWorkflowInput(s.in, file, &input); err == nil {
					value, err = store.Update(cmd.Context(), args[0], input)
				}
			}
			if err != nil {
				return err
			}
			return iox.WriteOKWithWarnings(s.out, "workflow_run", value, cfg.ResolutionNotices)
		}
		if op == "start" || op == "checkpoint" {
			cmd.Flags().StringVar(&file, "file", "-", "JSON input file, or - for stdin")
		}
		root.AddCommand(cmd)
	}
	return root
}

func readWorkflowInput(in io.Reader, file string, value any) error {
	raw, err := readRawInput(in, file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return iox.NewInvalidInput("invalid workflow JSON", "supply a JSON checkpoint object", err)
	}
	return nil
}
