package localrun

// The neutral vocabulary of a run's history. A caller must not be able to tell
// where a run happened by looking at the kinds of its events, which is why the
// names live here and not in a provider: a constant repeated inside every
// provider is a constant that eventually disagrees with the others, and the
// direction of that disagreement is the expensive one — two local runs that
// render differently for the same moment of the same work.
//
// A provider translates its own protocol into these kinds and adds nothing to
// the list. Anything it cannot translate keeps its own name and travels whole
// in RunEvent.Raw, which is what keeps a translation from silently losing
// history the day an agent adds a message type.
const (
	KindUserMessage = "user_message"
	KindText        = "text"
	KindThinking    = "thinking"
	KindToolStart   = "tool_start"
	KindToolEnd     = "tool_end"
	KindToolError   = "tool_error"
	KindTurnEnd     = "turn_end"
	KindError       = "error"
)
