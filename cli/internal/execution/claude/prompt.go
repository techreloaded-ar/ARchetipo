package claude

import (
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// plannedStatus is the spec status the receipt must declare. It is an alias of
// the shared execution constant so that the prompt and the acceptance gate can
// never ask for one word and accept another.
const plannedStatus = execution.PlannedStatus

// reviewStatus is the same fact for the implementation receipt: the status the
// prompt asks for and the status the shared gate accepts are one constant, so
// they cannot drift into asking for one word and accepting another.
const reviewStatus = execution.ReviewStatus

const localOpening = "Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed."

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
// `--permission-prompt-tool stdio` is what makes the permission policy mean
// what it says. Without it `--print` has nobody to ask, so a tool call the
// policy will not grant on its own is refused where it stands: the agent is
// told no by a wall, and closes the turn asking a person for something nothing
// in the run could have given it. With it, the question travels out as a
// `can_use_tool` control request and the person following the run answers it.
// The value is a sentinel and not an MCP tool name — the process reads it as
// "the host on the other end of this stream decides". Verified against Claude
// Code 2.1.246.
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
		"--permission-prompt-tool", permissionPromptHost,
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Effort != "" {
		args = append(args, "--effort", cfg.Effort)
	}
	return args
}

func buildNativeSessionArgs(cfg settings, sessionID string, resume bool) []string {
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--replay-user-messages",
		"--permission-mode", cfg.PermissionMode,
		"--permission-prompt-tool", permissionPromptHost,
	}
	if resume {
		args = append(args, "--resume", sessionID)
	} else {
		args = append(args, "--session-id", sessionID)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Effort != "" {
		args = append(args, "--effort", cfg.Effort)
	}
	return args
}

// buildConversationPrompt renders the single instruction that opens a free
// conversation about the workspace.
//
// It is pure and deterministic like every other prompt here, and it takes only
// the vocabulary of the process — no spec, no artifact and no receipt, because
// a conversation has none. It ends on nobody closing it rather than on a
// closing message, which is why it asks for no receipt line — a receipt would
// end a conversation that is meant to stay open.
//
// It authorizes reading, authorizes *proposing* an action, and keeps forbidding
// acting. The two coexist because proposing is not acting: naming what would be
// started starts nothing, and the guarantee that a conversation never becomes
// an action of the process is unchanged and still the one relied upon — it
// lives one layer up and is structural, since no execution record is ever
// written for a conversation, so there is nothing for it to appear as. The
// prohibition here is a courtesy towards the agent; a prompt the model talked
// itself past would leave the guarantee intact.
//
// The action ids come from the caller and are never written here: the process
// is not knowledge of a provider, and a literal id in this package would be a
// second declaration of a vocabulary that has exactly one. With no actions
// declared the whole proposal block is omitted — a list the agent cannot read
// is a list it would invent.
// resumed is the transcript of a past conversation this one takes up again, and
// is empty for a conversation that takes up nothing — in which case the prompt
// is byte for byte the one a conversation has always been given. When it is
// present it is fenced and announced as context rather than pasted in as if
// somebody had just said it: a transcript is full of sentences that were
// instructions once, addressed to another session, and an agent that read them
// as its own would act on requests that have already been answered.
//
// Everything but the opening sentence is the shared declaration in
// execution.ConversationPrompt: the vocabulary of a conversation has one
// contract, and a copy of it here would be a second one, free to drift. What
// stays local is the one sentence only this provider can write — it holds the
// agent on the person's own working directory.
func buildConversationPrompt(actions []execution.ConversationAction, resumed string) string {
	return execution.ConversationPrompt(conversationOpening, actions, resumed)
}

// conversationOpening says where a conversation held by this provider is
// standing: the very directory the person has open, because the process runs on
// their machine.
const conversationOpening = "Work in the current working directory: it is the ARchetipo workspace a person has open in front of them, with the archetipo CLI and the ARchetipo skills installed."

// conversationContextFence is the shared fence, named here too because this
// package draws prompts and reads them back.
const conversationContextFence = execution.ConversationContextFence
