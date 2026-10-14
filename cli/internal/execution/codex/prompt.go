package codex

import "github.com/techreloaded-ar/ARchetipo/cli/internal/execution"

const localOpening = "Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed."

// plannedStatus is the spec status the receipt must declare. It is an alias of
// the shared execution constant so that the prompt and the acceptance gate can
// never ask for one word and accept another.
const plannedStatus = execution.PlannedStatus

// reviewStatus is the same fact for the implementation receipt: the status the
// prompt asks for and the status the shared gate accepts are one constant, so
// they cannot drift into asking for one word and accepting another.
const reviewStatus = execution.ReviewStatus

// buildArgs renders the full argument list of the Codex invocation. It is the
// single place where the invocation lives, so changing how Codex is started
// never means touching Execute.
//
// The session runs through `codex app-server --listen stdio://`: a JSON-RPC
// server on standard input and output, which is the only surface of codex-cli
// 0.147.0 that keeps a conversation alive — `codex exec` runs to completion and
// reads nothing after its prompt, and `codex proto` no longer exists. Every
// tunable of the session (working directory, sandbox, model, approvals) travels
// inside the protocol rather than on the command line, which is why this
// function takes no settings.
func buildArgs() []string {
	return []string{"app-server", "--listen", "stdio://"}
}
