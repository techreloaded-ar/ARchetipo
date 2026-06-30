package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/validation"
)

// newValidateCmd builds `archetipo validate <phase>`.
func newValidateCmd(s streams) *cobra.Command {
	root := &cobra.Command{
		Use:   "validate",
		Short: "Validate an artifact (PRD, backlog, etc.)",
		Long:  "Run deterministic, marker-based validation on an artifact phase and return structured findings.",
	}
	root.AddCommand(newValidatePRDCmd(s))
	return root
}

// newValidatePRDCmd builds `archetipo validate prd`.
func newValidatePRDCmd(s streams) *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "prd",
		Short: "Validate the PRD against PRD structural rules",
		Long: `Run structural validation on the PRD artifact.

The validator checks:
  - PRD is not empty
  - No unresolved {{PLACEHOLDER}} tokens
  - All required section markers are present and have meaningful content

On success, a validation_result envelope is written to stdout.
On failure, an E_VALIDATION error envelope with structured findings is
written to stderr. Use error.details.findings to correct the PRD and retry.`,
		Args: cobra.NoArgs,
		RunE: runValidatePRD(s, &filePath),
	}
	cmd.Flags().StringVar(&filePath, "file", "", "path to the PRD markdown, or - for stdin (default: docs/PRD.md)")
	return cmd
}

func runValidatePRD(s streams, filePath *string) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return iox.NewInternal("cwd unavailable", err)
		}
		cfg, err := config.Load(cwd)
		if err != nil {
			return iox.NewInvalidInput(err.Error(), "fix the file or remove it to fall back to defaults", err)
		}

		var markdown string
		target := cfg.AbsPath(cfg.Paths.PRD)

		if *filePath != "" {
			if *filePath == "-" {
				body, readErr := readRawInput(s.in, "-")
				if readErr != nil {
					return readErr
				}
				markdown = string(body)
				target = "stdin"
			} else {
				data, readErr := os.ReadFile(*filePath)
				if readErr != nil {
					return iox.NewPrecondition(
						"PRD file not found: "+*filePath,
						"run archetipo-inception or archetipo prd write first",
						readErr,
					)
				}
				markdown = string(data)
				target = *filePath
			}
		} else {
			data, readErr := os.ReadFile(target)
			if readErr != nil {
				return iox.NewPrecondition(
					"PRD file not found at "+target,
					"run archetipo-inception or archetipo prd write first",
					readErr,
				)
			}
			markdown = string(data)
		}

		result := validation.ValidatePRD(target, markdown)

		if !result.OK {
			return iox.NewValidation(
				"prd validation failed",
				"fix the listed PRD findings and rerun validation",
				domain.ValidationErrorDetails{
					Artifact: result.Artifact,
					Target:   result.Target,
					Findings: result.Findings,
				},
			)
		}

		return iox.WriteOK(s.out, "validation_result", result)
	}
}
