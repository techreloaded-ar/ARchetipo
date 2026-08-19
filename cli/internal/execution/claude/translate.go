package claude

import (
	"encoding/json"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// isNoise names the frames this provider deliberately drops.
//
// They are enumerated one by one on purpose. The rule everywhere else is that
// an unrecognized frame still becomes an event — a translation that discards
// what it does not know silently loses part of a run's history the day Claude
// Code adds a frame. These two are not unknown: they are known to carry no
// history at all. `rate_limit_event` is a billing counter, and the
// `thinking_tokens` system frame is an incremental counter re-sent while the
// agent thinks, so both would flood a history without adding a moment to it.
func isNoise(f frame) bool {
	if f.Type == "rate_limit_event" {
		return true
	}
	return f.Type == frameSystem && f.Subtype == "thinking_tokens"
}

// contentBlock is the part of a message block this package interprets. As
// everywhere else, what is not read here still travels whole in the event's
// raw payload.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type streamMessage struct {
	Content []contentBlock `json:"content"`
}

// project translates one frame and, when it carries history, appends it to the
// session.
//
// The default branch is the point of the function: a frame this build has never
// seen still produces an event, carrying the frame's own type as its kind and
// the line untouched in Raw. A translation that dropped what it does not
// recognize would silently lose part of a run's history the day Claude Code
// adds a frame type.
func (s *streamSession) project(f frame, raw json.RawMessage) {
	if isNoise(f) {
		return
	}
	if f.Type == frameSystem && f.Subtype == subtypeInit {
		// The announcement that the session is open is protocol, not history.
		// It says that the process is ready — which `consume` acts on, and which
		// is the one thing it is good for — and nothing about what the agent
		// did. Codex draws the same line for `turn/started`, and it has to be
		// drawn identically here: a frame only Claude emits, appearing in every
		// run, would make the two local providers render differently for the
		// same moment of the same work, which is exactly what a neutral history
		// exists to prevent.
		return
	}
	switch f.Type {
	case frameAssistant:
		s.projectAssistant(f, raw)
	case frameUser:
		s.projectUser(f, raw)
	case frameResult:
		// A result frame is the process's own statement that the turn is over.
		// Only a result that reports no error can carry a plan: an interrupted
		// turn also ends with a result, and reports `error_during_execution`.
		s.mu.Lock()
		s.completed = !f.IsError
		s.finalMessage = f.Result
		s.mu.Unlock()
		// The end of the turn is claimed here, in the same breath as the result
		// that ended it, and only closed once the outcome has been published and
		// the event appended. Claiming first is what makes the whole sequence one
		// decision: a message sent while it is still running opens the next turn
		// on its own wait, and the close below can then only fall on the turn it
		// belongs to. Publishing before closing is deliberate too — a caller woken
		// by TurnDone already finds what the turn ended with instead of having to
		// race for it.
		done := s.claimTurnEnd()
		// FinalMessage, not the raw result, because a turn that ends with an empty
		// result still ended on the agent's last words.
		s.publishTurn(TurnOutcome{Completed: !f.IsError, Final: s.FinalMessage()})
		s.append(localrun.KindTurnEnd, "", "", raw)
		s.mu.Lock()
		// The next turn starts on a fresh seq, and the tools of the turn that
		// just ended can no longer be referenced by anything.
		s.seq++
		s.tools = make(map[string]string)
		s.mu.Unlock()
		if done != nil {
			close(done)
		}
	default:
		s.append(kindOf(f), "", "", raw)
	}
}

// kindOf names a frame that has no neutral translation. The subtype is part of
// the name when there is one, because `system` alone says almost nothing about
// what arrived.
func kindOf(f frame) string {
	if strings.TrimSpace(f.Subtype) != "" {
		return f.Type + "/" + f.Subtype
	}
	return f.Type
}

// projectAssistant renders what the agent produced, block by block and in
// order, because the order of the blocks is the order in which the agent
// worked.
func (s *streamSession) projectAssistant(f frame, raw json.RawMessage) {
	blocks, ok := decodeBlocks(f.Message)
	if !ok || len(blocks) == 0 {
		s.append(kindOf(f), "", "", raw)
		return
	}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			s.mu.Lock()
			s.lastAssistantText = block.Text
			s.mu.Unlock()
			s.append(localrun.KindText, block.Text, "", raw)
		case "thinking":
			s.append(localrun.KindThinking, thinkingText(block), "", raw)
		case "tool_use":
			if block.ID != "" {
				s.mu.Lock()
				s.tools[block.ID] = block.Name
				s.mu.Unlock()
			}
			s.append(localrun.KindToolStart, "", block.Name, raw)
		default:
			s.append(kindOf(f), "", "", raw)
		}
	}
}

// projectUser renders what arrives on the user side of the conversation: the
// results of the tools the agent ran, and the operator's own message coming
// back out. The re-emission is the only way a message enters the history — it
// is never written when it is sent.
func (s *streamSession) projectUser(f frame, raw json.RawMessage) {
	blocks, ok := decodeBlocks(f.Message)
	if !ok || len(blocks) == 0 {
		s.append(kindOf(f), "", "", raw)
		return
	}
	for _, block := range blocks {
		switch block.Type {
		case "tool_result":
			kind := localrun.KindToolEnd
			if block.IsError {
				kind = localrun.KindToolError
			}
			s.append(kind, toolResultText(block.Content), s.toolNameOf(block.ToolUseID), raw)
		case "text":
			s.append(localrun.KindUserMessage, block.Text, "", raw)
		default:
			s.append(kindOf(f), "", "", raw)
		}
	}
}

func (s *streamSession) toolNameOf(toolUseID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tools[toolUseID]
}

func (s *streamSession) append(kind, text, tool string, raw json.RawMessage) {
	s.mu.Lock()
	seq := s.seq
	s.mu.Unlock()
	s.session.Append(execution.RunEvent{
		Seq:  seq,
		Kind: kind,
		Text: text,
		Tool: tool,
		Raw:  localrun.RawOf(raw),
	})
}

func decodeBlocks(message json.RawMessage) ([]contentBlock, bool) {
	if len(message) == 0 {
		return nil, false
	}
	var decoded streamMessage
	if json.Unmarshal(message, &decoded) != nil {
		return nil, false
	}
	return decoded.Content, true
}

func thinkingText(block contentBlock) string {
	if strings.TrimSpace(block.Thinking) != "" {
		return block.Thinking
	}
	return block.Text
}

// toolResultText renders the content of a tool result, which the protocol sends
// either as a plain string or as an array of blocks. Both shapes were observed
// on the same build, so both are read here rather than one being assumed.
func toolResultText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var blocks []contentBlock
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
