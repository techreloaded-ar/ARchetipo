package claude

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
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
	Input     json.RawMessage `json:"input"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    *imageSource    `json:"source"`
}

// imageSource is the image of an `image` block as the message format carries
// it: base64, with its media type.
type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
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
		s.append(localrun.KindTurnEnd, "", "", raw, s.FinalMessage())
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
			// A structured question is something the agent said to the
			// person, not a tool it ran: it enters the history as readable
			// text, where it stays above the answer once the card is gone.
			if questions, ok := parseAskUserQuestion(block.Name, block.Input); ok {
				s.append(localrun.KindQuestion, execution.QuestionText(questions), "", raw)
				continue
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
//
// One user frame is not an operator's message at all: the opening instruction
// the caller composed, which the process replays like every other. It is
// dropped here, so a conversation opens on an empty transcript and the first
// thing a reader sees is what a person really wrote.
func (s *streamSession) projectUser(f frame, raw json.RawMessage) {
	if f.IsSynthetic {
		// A user frame the harness wrote to itself — «[Image: original
		// 2280x984, displayed at 2000x863…]» beside a screenshot it resized —
		// is its own working, not something the person said: shown as a
		// message it would read as theirs.
		s.append(kindOf(f)+"/synthetic", "", "", raw)
		return
	}
	blocks, ok := decodeBlocks(f.Message)
	if !ok || len(blocks) == 0 {
		s.append(kindOf(f), "", "", raw)
		return
	}
	// A frame that carries images is not kept raw: the base64 of a screenshot
	// is the one thing that must not enter the journal. The images reach the
	// user_message event by reference, through what expectAttachments kept.
	//
	// A tool result that carries images — the agent read a screenshot — is
	// the other way the bytes would enter: there the frame is kept, with the
	// base64 stripped out of it, and the images are stored beside the
	// conversation and attached to the tool_end by reference.
	images := false
	for _, block := range blocks {
		if block.Type == "image" {
			images = true
			raw = nil
		}
		if block.Type == "tool_result" && len(toolResultImages(block.Content)) > 0 {
			raw = stripImageData(raw)
		}
	}
	said := false
	for _, block := range blocks {
		switch block.Type {
		case "image":
			continue
		case "tool_result":
			kind := localrun.KindToolEnd
			if block.IsError {
				kind = localrun.KindToolError
			}
			s.appendWith(execution.RunEvent{Kind: kind, Text: toolResultText(block.Content), Tool: s.toolNameOf(block.ToolUseID), Attachments: execution.AttachmentReferences(s.storeToolResultImages(block.Content, f.ToolUseResult.File.FilePath)), Raw: raw})
		case "text":
			// The opening instruction comes back like any other user frame, and
			// it is the one that is not history: it is what the caller asked
			// for, not what the person said. Only the instruction is dropped —
			// a message replayed together with it keeps its place in the
			// history — and everything the person writes afterwards enters it
			// exactly as before.
			text := block.Text
			if rest, echo := s.openingEchoOf(text); echo {
				if rest == "" {
					continue
				}
				text = rest
			}
			// Two more user frames are not the person's either. The
			// instruction a caller composed is shown as what it asked for,
			// not as the prompt it took to ask it; the body of a skill the
			// harness expanded is the working of the agent, not a message,
			// and it is filed with the tool that loaded it.
			attachments := s.attachmentsOf(text)
			if display, echo := s.instructionEchoOf(text); echo {
				s.appendUserMessage(display, attachments, raw)
				said = true
				continue
			}
			if name, ok := skillPreambleOf(text); ok {
				s.append(localrun.KindInstruction, name, "Skill", raw)
				continue
			}
			s.appendUserMessage(text, attachments, raw)
			said = true
		default:
			s.append(kindOf(f), "", "", raw)
		}
	}
	// Images without words: the frame had no text block, so the event that
	// shows them is appended here, with the attachments written beside an
	// empty text.
	if images && !said {
		s.appendUserMessage("", s.attachmentsOf(""), nil)
	}
}

// appendUserMessage appends what the person said, with the images they
// attached to it.
func (s *streamSession) appendUserMessage(text string, attachments []execution.Attachment, raw json.RawMessage) {
	s.appendWith(execution.RunEvent{Kind: localrun.KindUserMessage, Text: text, Attachments: execution.AttachmentReferences(attachments), Raw: raw})
}

func (s *streamSession) toolNameOf(toolUseID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tools[toolUseID]
}

func (s *streamSession) append(kind, text, tool string, raw json.RawMessage, final ...string) {
	finalMessage := ""
	if len(final) > 0 {
		finalMessage = final[0]
	}
	s.appendWith(execution.RunEvent{FinalMessage: finalMessage, Kind: kind, Text: text, Tool: tool, Raw: raw})
}

// appendWith stamps the event with the turn it belongs to and hands it on.
func (s *streamSession) appendWith(event execution.RunEvent) {
	s.mu.Lock()
	event.Seq = s.seq
	event.TurnID = s.turnID
	event.SubmissionID = s.submissionID
	appendEvent := s.appendEvent
	s.mu.Unlock()
	event.Raw = localrun.RawOf(event.Raw)
	appendEvent(event)
}

// Claude Code descrive il contenuto di un messaggio in due forme: la lista di
// blocchi, e — quando l'operatore ha scritto un comando con la barra — una
// stringa sola. Le due dicono la stessa cosa, quindi da qui escono uguali: un
// blocco di testo. Prima la seconda non si decodificava, e un messaggio scritto
// come `/skill ...` non entrava nella storia — restava un evento senza testo,
// e la riga «in consegna» del compositore non trovava mai la sua e restava
// appesa in coda, sotto la risposta che le era arrivata dopo.
func decodeBlocks(message json.RawMessage) ([]contentBlock, bool) {
	if len(message) == 0 {
		return nil, false
	}
	var decoded streamMessage
	if json.Unmarshal(message, &decoded) == nil {
		return decoded.Content, true
	}
	var plain struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(message, &plain) != nil || plain.Content == "" {
		return nil, false
	}
	return []contentBlock{{Type: "text", Text: commandText(plain.Content)}}, true
}

// Un comando con la barra arriva avvolto nei tag con cui l'harness lo descrive
// a se stesso. Quello che la persona ha scritto sono il nome e gli argomenti,
// e quello entra nella storia: il resto è impalcatura, e leggerlo nel thread
// sarebbe leggere il protocollo invece della conversazione.
func commandText(content string) string {
	name := taggedValue(content, "command-name")
	if name == "" {
		return content
	}
	args := taggedValue(content, "command-args")
	if args == "" {
		return name
	}
	return name + " " + args
}

func taggedValue(content, tag string) string {
	opening, closing := "<"+tag+">", "</"+tag+">"
	from := strings.Index(content, opening)
	if from == -1 {
		return ""
	}
	rest := content[from+len(opening):]
	to := strings.Index(rest, closing)
	if to == -1 {
		return ""
	}
	return strings.TrimSpace(rest[:to])
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

// toolResultImages are the base64 image blocks of a tool result, when its
// content is a list of blocks.
func toolResultImages(content json.RawMessage) []imageSource {
	var blocks []contentBlock
	if len(content) == 0 || json.Unmarshal(content, &blocks) != nil {
		return nil
	}
	var images []imageSource
	for _, block := range blocks {
		if block.Type == "image" && block.Source != nil && block.Source.Data != "" {
			images = append(images, *block.Source)
		}
	}
	return images
}

// storeToolResultImages writes the images of a tool result under the session's
// attachments directory and hands back their references. Without a directory
// nothing is stored; an image that cannot be decoded or written is left out,
// because the text of the result is still history worth keeping. The stored
// file is named by its content; the attachment keeps the name of the file the
// tool read, when the harness says it, because that is the name a person
// recognises a mockup by.
func (s *streamSession) storeToolResultImages(content json.RawMessage, filePath string) []execution.Attachment {
	if s.attachmentsDir == "" {
		return nil
	}
	images := toolResultImages(content)
	var attachments []execution.Attachment
	for _, image := range images {
		data, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil {
			continue
		}
		attachment, err := execution.StoreAttachment(s.attachmentsDir, image.MediaType, data)
		if err != nil {
			continue
		}
		if len(images) == 1 && strings.TrimSpace(filePath) != "" {
			attachment.Name = filepath.Base(filePath)
		}
		attachments = append(attachments, attachment)
	}
	return attachments
}

// stripImageData returns the frame without the base64 of the images its tool
// results carry: everything else — ids, text, media types — stays, so the raw
// payload still says what arrived without holding the one thing that must not
// enter the journal. Claude Code carries the bytes twice, in the result's
// blocks and again in `tool_use_result.file.base64`, so both copies go. A
// frame that cannot be read back is dropped whole.
func stripImageData(raw json.RawMessage) json.RawMessage {
	var frame map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &frame) != nil {
		return nil
	}
	if result, ok := frame["tool_use_result"].(map[string]any); ok {
		if file, ok := result["file"].(map[string]any); ok {
			delete(file, "base64")
		}
	}
	message, _ := frame["message"].(map[string]any)
	blocks, _ := message["content"].([]any)
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		inner, _ := block["content"].([]any)
		for _, ib := range inner {
			image, _ := ib.(map[string]any)
			if image["type"] != "image" {
				continue
			}
			if source, ok := image["source"].(map[string]any); ok {
				delete(source, "data")
			}
		}
	}
	out, err := json.Marshal(frame)
	if err != nil {
		return nil
	}
	return out
}
