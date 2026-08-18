package arcipelago

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const (
	// sseEventRun is the name the hub gives to a run history frame.
	sseEventRun = "run_event"
	// sseEventEnd is the name of the terminal frame: the stream says why it is
	// ending instead of just closing the connection, so the consumer can tell a
	// finished run from a dropped socket.
	sseEventEnd = "end"

	// maxFrameBytes bounds a single SSE frame, on the same order of magnitude as
	// maxResponseBody. The stream itself is unbounded by design; one frame is
	// not, and buffering an unbounded line would be the one way this consumer
	// could be made to exhaust memory.
	maxFrameBytes = 1 << 20
)

// runEventFrame is the payload of a run_event frame.
type runEventFrame struct {
	ID    int64           `json:"id"`
	RunID string          `json:"runId"`
	Seq   int             `json:"seq"`
	TS    int64           `json:"ts"`
	Event json.RawMessage `json:"event"`
}

// endFrame is the payload of the terminal frame.
type endFrame struct {
	Reason string `json:"reason"`
}

// agentEvent is the part of the hub's AgentEvent this package interprets.
// Everything else survives in RunEvent.Raw: translation narrows what is
// rendered, never what is carried.
type agentEvent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ToolName string `json:"toolName"`
	IsError  bool   `json:"isError"`

	AssistantMessageEvent struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
	} `json:"assistantMessageEvent"`

	Message struct {
		StopReason   string `json:"stopReason"`
		ErrorMessage string `json:"errorMessage"`
	} `json:"message"`
}

// Neutral event kinds this provider produces.
const (
	kindUserMessage = "user_message"
	kindText        = "text"
	kindThinking    = "thinking"
	kindToolStart   = "tool_start"
	kindToolEnd     = "tool_end"
	kindToolError   = "tool_error"
	kindTurnEnd     = "turn_end"
)

// translateAgentEvent derives the rendered fields of a run event from the hub's
// agent event.
//
// The default branch is the point of the function: an event type this build has
// never seen still produces an event, carrying the raw type as its kind. A
// translation that dropped what it does not recognize would silently lose part
// of a run's history the day the hub adds a type.
func translateAgentEvent(raw json.RawMessage) (kind, text, tool string) {
	var event agentEvent
	if len(raw) == 0 || json.Unmarshal(raw, &event) != nil {
		return "", "", ""
	}
	switch event.Type {
	case "user_message":
		return kindUserMessage, event.Text, ""
	case "message_update":
		switch event.AssistantMessageEvent.Type {
		case "text_delta":
			return kindText, event.AssistantMessageEvent.Delta, ""
		case "thinking_delta":
			return kindThinking, event.AssistantMessageEvent.Delta, ""
		}
		return event.Type, "", ""
	case "tool_execution_start":
		return kindToolStart, "", event.ToolName
	case "tool_execution_end":
		if event.IsError {
			return kindToolError, "", event.ToolName
		}
		return kindToolEnd, "", event.ToolName
	case "message_end":
		if event.Message.StopReason == "error" || event.Message.StopReason == "aborted" {
			return kindTurnEnd, event.Message.ErrorMessage, ""
		}
		return kindTurnEnd, "", ""
	default:
		return event.Type, "", ""
	}
}

// StreamRunEvents consumes one connection to the run's event stream.
//
// It deliberately does not reconnect. Reconnection needs the high-water cursor,
// which belongs to whoever keeps the projection, so retrying here would either
// duplicate that state or resume from the wrong place. The caller reconnects by
// calling again with the cursor it holds.
func (p *Provider) StreamRunEvents(ctx context.Context, req execution.RunRequest, afterID int64, sink func(execution.RunEvent) error) error {
	cfg, err := parseConfig(req.ProviderConfig)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(p.getenv(cfg.TokenEnv))
	if token == "" {
		return fmt.Errorf("the ARcipelago application credential is not available: export it in the %s environment variable", cfg.TokenEnv)
	}
	endpoint := cfg.BaseURL + "/api/external/runs/" + url.PathEscape(req.RunID) + "/events"
	if afterID > 0 {
		endpoint += "?afterId=" + strconv.FormatInt(afterID, 10)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("building the arcipelago run event request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	// The stream client, never p.doer: see newStreamClient.
	resp, err := p.streamDoer.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("arcipelago run event stream for %s failed: %w", req.RunID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		return refusalFor(resp.StatusCode, req.RunID, classify(resp.StatusCode, payload, cfg))
	}
	return consumeRunStream(ctx, resp.Body, req.RunID, sink)
}

// consumeRunStream parses the SSE body and hands every translated event to
// sink. It is separate from the request so the parser is testable on a plain
// reader.
func consumeRunStream(ctx context.Context, body io.Reader, runID string, sink func(execution.RunEvent) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFrameBytes)

	var name string
	var data strings.Builder
	reset := func() {
		name = ""
		data.Reset()
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		switch {
		case line == "":
			// A blank line closes the frame. A frame without data carries nothing
			// to deliver and is dropped in silence, which is what the SSE spec
			// prescribes for a comment-only or field-only block.
			if data.Len() > 0 {
				done, err := deliverFrame(name, data.String(), runID, sink)
				if err != nil || done {
					return err
				}
			}
			reset()
		case strings.HasPrefix(line, ":"):
			// Comment line, the shape keep-alives take. Not part of any frame.
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		default:
			// `id:` and any other field: the cursor this consumer trusts is the one
			// inside the payload, so the transport-level id is not read here.
		}
	}
	if err := scanner.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("reading the arcipelago run event stream for %s: %w", runID, err)
	}
	// A trailing frame the server did not terminate with a blank line is still a
	// frame: dropping it would lose the last event of every stream that ends
	// exactly on one.
	if data.Len() > 0 {
		if _, err := deliverFrame(name, data.String(), runID, sink); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// deliverFrame interprets one complete frame. It reports done when the frame
// terminates the stream.
func deliverFrame(name, data, runID string, sink func(execution.RunEvent) error) (bool, error) {
	switch name {
	case sseEventEnd:
		var frame endFrame
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			return true, nil
		}
		if frame.Reason == "unauthorized" {
			return true, &execution.RunCommandError{Reason: execution.RunRefusedUnauthorized, RunID: runID}
		}
		return true, nil
	case sseEventRun, "":
		var frame runEventFrame
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			// A malformed frame is not worth ending a live stream over: the run
			// keeps producing history and the next frame is very likely readable.
			return false, nil
		}
		if name == "" && frame.ID == 0 {
			return false, nil
		}
		kind, text, tool := translateAgentEvent(frame.Event)
		event := execution.RunEvent{
			ID:   frame.ID,
			Seq:  frame.Seq,
			At:   time.UnixMilli(frame.TS).UTC(),
			Kind: kind,
			Text: text,
			Tool: tool,
			Raw:  frame.Event,
		}
		// The sink's error is propagated unchanged: it is how the consumer stops
		// the stream, and wrapping it would break the errors.Is it recognizes.
		if err := sink(event); err != nil {
			return true, err
		}
		return false, nil
	default:
		return false, nil
	}
}
