package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/validation"
)

// newValidateCmd builds `archetipo validate <artifact>`.
//
// `prd` validates the persisted PRD on disk and reports failures as an
// E_VALIDATION error envelope. `spec` and `plan` validate a structured payload
// before persistence and report failures as a normal validation_result with
// ok:false (exit 0) so skills can repair the payload without mutating state.
func newValidateCmd(s streams) *cobra.Command {
	root := &cobra.Command{
		Use:   "validate",
		Short: "Validate an artifact (PRD, spec, plan)",
		Long:  "Run deterministic validation on an artifact and return structured findings.",
	}
	root.AddCommand(newValidatePRDCmd(s), newValidateSpecCmd(s), newValidatePlanCmd(s))
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

// newValidateSpecCmd builds `archetipo validate spec`.
func newValidateSpecCmd(s streams) *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Validate a spec add payload before persistence",
		Long: `Run structural validation on a spec add payload (YAML or JSON).

Validation failure is a normal result, not a process error: a validation_result
envelope is written to stdout with data.ok:false and data.findings. Repair every
error-severity finding and rerun before calling 'archetipo spec add'. Warnings are
quality feedback and do not block persistence.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" {
				return errInvalidUsage("missing input file", "pass --file path/to/specs.yaml or --file -")
			}
			var payload specsPayload
			if err := readStructuredInput(s.in, filePath, &payload); err != nil {
				return err
			}
			result := validation.ValidateSpecs(filePath, payload.Specs)
			return iox.WriteOK(s.out, "validation_result", result)
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "path to a YAML or JSON specs payload, or - for stdin")
	return cmd
}

// newValidatePlanCmd builds `archetipo validate plan US-XXX`.
func newValidatePlanCmd(s streams) *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "plan US-XXX",
		Short: "Validate a plan payload before persistence",
		Long: `Run structural validation on a plan payload (YAML or JSON) for a single spec.

Validation failure is a normal result, not a process error: a validation_result
envelope is written to stdout with data.ok:false and data.findings. Repair every
error-severity finding and rerun before persisting the plan.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := strings.TrimSpace(args[0])
			if ref == "" {
				return errInvalidUsage("missing spec code", "pass US-XXX as positional argument")
			}
			if filePath == "" {
				return errInvalidUsage("missing input file", "pass --file path/to/plan.yaml or --file -")
			}
			var input domain.PlanInput
			if err := readStructuredInput(s.in, filePath, &input); err != nil {
				return err
			}
			result := validation.ValidatePlan(filePath, ref, input)
			return iox.WriteOK(s.out, "validation_result", result)
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "path to a YAML or JSON plan payload, or - for stdin")
	return cmd
}
