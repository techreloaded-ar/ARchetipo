package arcipelago

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	// sourceARchetipo is the external namespace ARchetipo owns on the hub. It is
	// part of the (workspaceId, source, externalId) identity triple.
	sourceARchetipo = "archetipo"

	pathTasks       = "/api/external/tasks"
	pathByReference = "/api/external/tasks/by-reference"

	// maxErrorBody bounds how much of a remote error body is echoed back.
	maxErrorBody = 512

	// maxResponseBody bounds how much of a remote response is buffered at all.
	// The envelopes this client reads are a handful of fields; anything past
	// this size is a malfunction, not a payload to decode.
	maxResponseBody = 1 << 20
)

// Remote task statuses. Only the last three are terminal.
const (
	statusCompleted = "completed"
	statusFailed    = "failed"
	statusCancelled = "cancelled"
)

// createTaskRequest mirrors the only fields the external namespace accepts
// (packages/hub/src/api/app.ts:391-409). cwdHint, skills, assigneeAgentId and
// targetRunnerId are deliberately absent, and sending them would be useless
// rather than rejected: readBody parses the body by reading the known keys one
// by one (packages/hub/src/api/app.ts:116-135), so an unknown key is dropped in
// silence and never reaches the task. Either way an external task cannot steer
// itself to a runner or a directory.
//
// Metadata carries no omitempty on purpose: it must always travel, and always
// as an object.
type createTaskRequest struct {
	WorkspaceID string         `json:"workspaceId"`
	Source      string         `json:"source"`
	ExternalID  string         `json:"externalId"`
	Title       string         `json:"title"`
	Prompt      string         `json:"prompt"`
	Metadata    map[string]any `json:"metadata"`
}

type remoteTask struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	ResultSummary string `json:"resultSummary"`
	// RunID names the run the hub assigned to this task, and is empty until it
	// assigns one. It is the only bridge from an execution record to the
	// interactive run behind it.
	RunID string `json:"runId"`
}

type taskEnvelope struct {
	Task remoteTask `json:"task"`
}

// errorResponse discriminates the two distinct causes the hub encodes with the
// same 409 status.
type errorResponse struct {
	Error string `json:"error"`
}

// do performs one authenticated call against the hub. It returns the HTTP
// status alongside the error so the caller can discriminate causes. The token
// is never included in a returned message.
func (p *Provider) do(ctx context.Context, cfg settings, token, method, path string, body, out any) (int, error) {
	status, _, err := p.doWithBody(ctx, cfg, token, method, path, body, out)
	return status, err
}

// doWithBody is do with the raw response echoed back to the caller. It exists
// because a refusal the hub encodes inside the body — the two distinct causes
// it both answers with 409 — cannot be classified from the status alone, and
// re-deriving it from the text of the error classify built would be branching
// on a message this package writes itself.
func (p *Provider) doWithBody(ctx context.Context, cfg settings, token, method, path string, body, out any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("encoding arcipelago request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.BaseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("building arcipelago request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.doer.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("arcipelago request to %s failed: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// The read error is not discarded and the body is bounded: a connection that
	// drops mid-response must stay diagnosable as the network failure it is,
	// instead of surfacing as an unexplained JSON decoding error, and an
	// unbounded body must not be buffered whole.
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading the arcipelago response from %s failed: %w", path, err)
	}
	if len(payload) > maxResponseBody {
		return resp.StatusCode, nil, fmt.Errorf("the arcipelago response from %s exceeds %d bytes", path, maxResponseBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, payload, classify(resp.StatusCode, payload, cfg)
	}
	if out == nil || len(payload) == 0 {
		return resp.StatusCode, payload, nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return resp.StatusCode, payload, fmt.Errorf("decoding arcipelago response from %s: %w", path, err)
	}
	return resp.StatusCode, payload, nil
}

// externalIdentityConflictError is the one 409 cause the caller acts on rather
// than only reporting: it can enrich the message with the id of the task that
// already holds the reference. It is a type so that decision is not taken by
// matching the text of an error this package builds itself.
type externalIdentityConflictError struct{ existingTaskID string }

func (e *externalIdentityConflictError) Error() string {
	message := "arcipelago already has a task with this external identity but a different request payload (HTTP 409 external_task_conflict): the same --request-id was used for a different assignment, so pass a new --request-id"
	if e.existingTaskID != "" {
		message += "; the existing remote task is " + e.existingTaskID
	}
	return message
}

// classify turns a non-2xx response into a stable, readable message. The error
// surface of the external namespace is narrow and is mirrored exactly: there is
// deliberately no 403 branch, because applicationAuth answers 401 when the
// bearer does not resolve (packages/hub/src/application-auth.ts:32) and
// per-workspace authorization is expressed as 404.
func classify(status int, payload []byte, cfg settings) error {
	body := truncate(payload)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("arcipelago rejected the application credential (HTTP 401): check the token exported in %s", cfg.TokenEnv)
	case http.StatusNotFound:
		return fmt.Errorf("arcipelago workspace %q or task is not visible to this credential (HTTP 404): check workspace_id and that the credential is granted the workspace", cfg.WorkspaceID)
	case http.StatusConflict:
		var decoded errorResponse
		if err := json.Unmarshal(payload, &decoded); err == nil {
			switch decoded.Error {
			case "external_task_conflict":
				return &externalIdentityConflictError{}
			case "workspace_archived":
				return fmt.Errorf("arcipelago workspace %q is archived (HTTP 409 workspace_archived): unarchive it or configure another workspace_id", cfg.WorkspaceID)
			}
		}
		return fmt.Errorf("arcipelago request failed (HTTP 409): %s", body)
	case http.StatusBadRequest:
		return fmt.Errorf("arcipelago rejected the request (HTTP 400): %s", body)
	default:
		return fmt.Errorf("arcipelago request failed (HTTP %d): %s", status, body)
	}
}

// truncate bounds an echoed body without splitting a rune: cutting on a byte
// boundary inside a multi-byte character would emit U+FFFD in the middle of a
// message an operator is meant to read.
func truncate(payload []byte) string {
	body := strings.TrimSpace(string(payload))
	if body == "" {
		return "empty response body"
	}
	if len(body) <= maxErrorBody {
		return body
	}
	cut := maxErrorBody
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + "..."
}

// byReferenceQuery builds the recovery route that finds a remote task from the
// external reference ARchetipo already holds
// (packages/hub/src/api/app.ts:341-360).
func byReferenceQuery(cfg settings, externalID string) string {
	values := url.Values{}
	values.Set("workspaceId", cfg.WorkspaceID)
	values.Set("source", sourceARchetipo)
	values.Set("externalId", externalID)
	return pathByReference + "?" + values.Encode()
}
