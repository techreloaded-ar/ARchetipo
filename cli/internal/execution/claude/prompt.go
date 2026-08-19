package claude

import (
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// plannedStatus is the spec status the receipt must declare. It is an alias of
// the shared execution constant so that the prompt and the acceptance gate can
// never ask for one word and accept another.
const plannedStatus = execution.PlannedStatus

// buildPrompt renders the single instruction given to the local Claude process.
//
// It is a pure, deterministic function of the fields of req: no timestamp, no
// random value, no environment or filesystem lookup. That is what makes the
// invocation reproducible and testable without a machine that has Claude Code
// on it, and it is also what keeps the prompt free of anything that could carry
// local or authentication material into the agent's context.
//
// The wording mirrors the arcipelago and codex prompts on purpose. The three
// providers ask for the same work and accept the same receipt, so a divergence
// in the request would mean one provider asking for something the shared
// acceptance gate does not recognize.
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

// buildArgs renders the full argument list of the Claude invocation. It is the
// single place where the flags live, so changing how Claude is called never
// means touching Execute.
//
// Every flag here is load-bearing, which is why none of them is configurable.
// The dialogue rests on the streaming protocol: `--input-format stream-json`
// is what lets a message reach the agent while it works, `--output-format
// stream-json` with `--verbose` is what makes its work observable frame by
// frame, and `--replay-user-messages` is what sends an operator's message back
// out so it can enter the history — the shared rule is that a message becomes
// history when the process re-emits it, never when it is sent, and without this
// flag Claude would simply never re-emit it. `--no-session-persistence` keeps a
// managed run from leaving a resumable session on disk. Verified against Claude
// Code 2.1.235.
//
// The prompt is deliberately absent: it travels inside the protocol as the
// first user frame, not as an argument, because a live session is opened before
// it is told what to do.
func buildArgs(cfg settings) []string {
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--replay-user-messages",
		"--no-session-persistence",
		"--permission-mode", cfg.PermissionMode,
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	return args
}
