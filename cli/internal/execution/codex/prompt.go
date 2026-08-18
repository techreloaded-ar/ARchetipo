package codex

import (
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// plannedStatus is the spec status the receipt must declare. It is an alias of
// the shared execution constant so that the prompt and the acceptance gate can
// never ask for one word and accept another.
const plannedStatus = execution.PlannedStatus

// buildPrompt renders the single instruction given to the local Codex process.
//
// It is a pure, deterministic function of the fields of req: no timestamp, no
// random value, no environment or filesystem lookup. That is what makes the
// invocation reproducible and testable without a machine that has Codex on it,
// and it is also what keeps the prompt free of anything that could carry local
// or authentication material into the agent's context.
//
// The wording mirrors the arcipelago prompt on purpose. The two providers ask
// for the same work and accept the same receipt, so a divergence in the request
// would mean one provider asking for something the shared acceptance gate does
// not recognize.
func buildPrompt(req execution.Request) string {
	return strings.Join([]string{
		"Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed.",
		"Plan the spec " + req.SpecCode + " by invoking the ARchetipo planning skill:",
		"",
		"/archetipo-plan " + req.SpecCode,
		"",
		"Persist the plan through the configured connector, exactly as the skill prescribes. Do not paste the plan into your final message.",
		"Close your run with a single JSON receipt line and nothing after it:",
		"",
		`{"spec_code":"` + req.SpecCode + `","status":"` + plannedStatus + `","tasks":<N>}`,
		"",
		"<N> is the number of tasks of the plan you actually persisted. Emit the receipt only after the plan is persisted and the spec is " + plannedStatus + ".",
	}, "\n")
}

// buildArgs renders the full argument list of the Codex invocation. It is the
// single place where the flags live, so changing how Codex is called never
// means touching Execute.
//
// The defaults are `exec -s workspace-write --skip-git-repo-check`: a
// non-interactive run whose sandbox may write inside the workspace — planning
// has to persist a plan — and that does not refuse to start when the workspace
// is not a git checkout. They are verified against codex-cli 0.147.0, whose
// `exec` rejects the interactive CLI's --full-auto outright. `exec_args` stays
// the escape hatch for a Codex release that spells them differently.
//
// An `exec_args` value replaces the intermediate default flags only. The `exec`
// subcommand stays first and the prompt stays last, because those two are not
// tuning: they are what makes the invocation a Codex run of this prompt at all.
func buildArgs(cfg settings, prompt string) []string {
	args := []string{"exec"}
	if len(cfg.ExecArgs) > 0 {
		args = append(args, cfg.ExecArgs...)
	} else {
		args = append(args, defaultExecArgs...)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	return append(args, prompt)
}
