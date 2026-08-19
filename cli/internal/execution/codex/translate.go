package codex

import (
	"encoding/json"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// threadItem is the part of a Codex thread item this package interprets.
// Everything else survives untouched in RunEvent.Raw: translation narrows what
// is rendered, never what is carried.
type threadItem struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Status  string `json:"status"`
	Tool    string `json:"tool"`
	Server  string `json:"server"`
	Query   string `json:"query"`
	Command any    `json:"command"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	ExitCode *int   `json:"exitCode"`
	Error    string `json:"error"`
}

// itemNotification is the shape shared by every notification that carries a
// thread item.
type itemNotification struct {
	Item threadItem `json:"item"`
}

// deltaNotification is the shape of every streaming fragment.
type deltaNotification struct {
	Delta string `json:"delta"`
	Text  string `json:"text"`
}

// project translates one notification and, when it carries history, appends it
// to the session.
//
// The default branch is the point of the function: a notification this build
// has never seen still produces an event, carrying the method as its kind and
// the payload untouched in Raw. A translation that dropped what it does not
// recognize would silently lose part of a run's history the day Codex adds a
// notification.
func (a *appServer) project(method string, params json.RawMessage) {
	if _, noise := noiseNotifications[method]; noise {
		return
	}
	switch method {
	case "turn/started":
		a.mu.Lock()
		a.seq++
		a.agent.Reset()
		a.mu.Unlock()
		return
	case "turn/completed":
		a.mu.Lock()
		a.completed = true
		a.mu.Unlock()
		a.append(localrun.KindTurnEnd, "", "", params)
		a.endTurn()
		return
	case "item/agentMessage/delta":
		delta := decodeDelta(params)
		a.mu.Lock()
		a.agent.WriteString(delta)
		a.mu.Unlock()
		a.append(localrun.KindText, delta, "", params)
		return
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		a.append(localrun.KindThinking, decodeDelta(params), "", params)
		return
	case "item/started":
		kind, text, tool, carries := startedItem(params)
		if carries {
			a.append(kind, text, tool, params)
		}
		return
	case "item/completed":
		kind, text, tool, carries := completedItem(a, params)
		if carries {
			a.append(kind, text, tool, params)
		}
		return
	case "error":
		a.append(localrun.KindError, decodeErrorText(params), "", params)
		return
	default:
		a.append(method, "", "", params)
	}
}

func (a *appServer) append(kind, text, tool string, params json.RawMessage) {
	a.mu.Lock()
	seq := a.seq
	a.mu.Unlock()
	a.session.Append(execution.RunEvent{
		Seq:  seq,
		Kind: kind,
		Text: text,
		Tool: tool,
		Raw:  localrun.RawOf(params),
	})
}

func decodeDelta(params json.RawMessage) string {
	var delta deltaNotification
	if json.Unmarshal(params, &delta) != nil {
		return ""
	}
	if delta.Delta != "" {
		return delta.Delta
	}
	return delta.Text
}

func decodeErrorText(params json.RawMessage) string {
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(params, &payload) != nil {
		return ""
	}
	if strings.TrimSpace(payload.Message) != "" {
		return payload.Message
	}
	return payload.Error
}

// startedItem translates the opening of a thread item.
//
// A user message is rendered here and not on completion, so the operator's line
// appears in the history exactly once. An agent message is rendered by its
// deltas, and reasoning by its own delta notifications, so neither carries
// history at this point.
func startedItem(params json.RawMessage) (kind, text, tool string, carries bool) {
	var notification itemNotification
	if json.Unmarshal(params, &notification) != nil {
		return "", "", "", false
	}
	item := notification.Item
	switch item.Type {
	case "userMessage":
		return localrun.KindUserMessage, joinContent(item.Content, item.Text), "", true
	case "agentMessage", "reasoning", "plan":
		return "", "", "", false
	default:
		return localrun.KindToolStart, toolText(item), toolName(item), true
	}
}

// completedItem translates the closing of a thread item. It is also where the
// agent's finished message is captured, because that text is what carries the
// plan receipt.
func completedItem(a *appServer, params json.RawMessage) (kind, text, tool string, carries bool) {
	var notification itemNotification
	if json.Unmarshal(params, &notification) != nil {
		return "", "", "", false
	}
	item := notification.Item
	switch item.Type {
	case "agentMessage":
		a.mu.Lock()
		a.lastFull = item.Text
		a.mu.Unlock()
		return "", "", "", false
	case "userMessage", "reasoning", "plan":
		return "", "", "", false
	default:
		if failed(item.Status, item.Error, item.ExitCode) {
			return localrun.KindToolError, toolText(item), toolName(item), true
		}
		return localrun.KindToolEnd, toolText(item), toolName(item), true
	}
}

func failed(status, errText string, exitCode *int) bool {
	if strings.TrimSpace(errText) != "" {
		return true
	}
	if exitCode != nil && *exitCode != 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "aborted", "rejected":
		return true
	}
	return false
}

// toolName names the tool an item belongs to, preferring what the item declares
// over the shape of the item itself.
func toolName(item threadItem) string {
	if name := strings.TrimSpace(item.Tool); name != "" {
		if server := strings.TrimSpace(item.Server); server != "" {
			return server + "." + name
		}
		return name
	}
	return item.Type
}

func toolText(item threadItem) string {
	if query := strings.TrimSpace(item.Query); query != "" {
		return query
	}
	return commandText(item.Command)
}

// commandText renders a command the way Codex sends it: a string, or the
// argument vector of the process it ran.
func commandText(command any) string {
	switch typed := command.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			if text, ok := part.(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func joinContent(content []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}, fallback string) string {
	parts := make([]string, 0, len(content))
	for _, entry := range content {
		if strings.TrimSpace(entry.Text) != "" {
			parts = append(parts, entry.Text)
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, "\n")
}
