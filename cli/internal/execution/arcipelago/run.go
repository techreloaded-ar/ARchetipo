package arcipelago

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// Provider satisfies the optional collaboration contract in full. The assertion
// is what keeps the seven methods from drifting apart from the interface: a
// missing one stops the build here rather than at a type assertion that quietly
// returns false at runtime.
var _ execution.RunCollaborator = (*Provider)(nil)

// refusalFor turns a remote status into a typed refusal, or leaves the cause
// untouched when the status is a fault rather than a decision.
//
// On the external namespace of the hub a 404 means only "not visible to this
// credential" — never "gone". Per-workspace authorization is expressed as 404
// there (packages/hub/src/application-auth.ts), which is why it classifies as a
// refusal the caller can render and not as a failure to retry.
//
// A 5xx is deliberately not a refusal: nothing was decided, so retrying can
// still change the outcome, and a caller that treated it as a decision would
// stop following a run that is perfectly alive.
func refusalFor(status int, runID string, cause error) error {
	return refusalForBody(status, nil, runID, cause)
}

// refusalForBody is refusalFor with the response body in hand.
//
// It exists for one distinction the status code cannot carry: the hub encodes
// both "the run is over" and "nothing is attached to execute this" as 409, and
// separates them only in the `error` field of the body. Collapsing them would
// hide the one refusal of the five that is transient — a runner that is offline
// now can be back in a moment, while a closed run never reopens.
func refusalForBody(status int, payload []byte, runID string, cause error) error {
	switch status {
	case http.StatusUnauthorized:
		return &execution.RunCommandError{Reason: execution.RunRefusedUnauthorized, RunID: runID, Err: cause}
	case http.StatusNotFound:
		return &execution.RunCommandError{Reason: execution.RunRefusedNotFound, RunID: runID, Err: cause}
	case http.StatusConflict:
		reason := execution.RunRefusedNotActive
		var decoded errorResponse
		if len(payload) > 0 && json.Unmarshal(payload, &decoded) == nil && decoded.Error == "runner_offline" {
			reason = execution.RunRefusedRunnerOffline
		}
		return &execution.RunCommandError{Reason: reason, RunID: runID, Err: cause}
	default:
		return cause
	}
}

// ResolveRun finds the remote run backing an execution record.
//
// It looks for the remote task id in the record first — a succeeded record
// carries it in Result.ExternalID, a record that failed after the task existed
// carries it in Error.ExternalID — and falls back to the external reference
// otherwise. The fallback is what makes a still-RUNNING execution reachable:
// its record has no remote id yet, but the task itself carries the execution id
// as its externalId, so the identity triple finds it.
//
// An empty run id with a nil error is a real answer: the task exists and the
// hub has not assigned it a run yet.
func (p *Provider) ResolveRun(ctx context.Context, exec execution.Execution, providerConfig map[string]any) (string, error) {
	cfg, err := parseConfig(providerConfig)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(p.getenv(cfg.TokenEnv))
	if token == "" {
		return "", fmt.Errorf("the ARcipelago application credential is not available: export it in the %s environment variable", cfg.TokenEnv)
	}
	var envelope taskEnvelope
	status, err := p.do(ctx, cfg, token, http.MethodGet, remoteTaskPath(cfg, exec), nil, &envelope)
	if err != nil {
		return "", refusalFor(status, "", err)
	}
	return strings.TrimSpace(envelope.Task.RunID), nil
}

// remoteTaskPath picks the route that identifies the remote task of a record.
func remoteTaskPath(cfg settings, exec execution.Execution) string {
	if taskID := remoteTaskID(exec); taskID != "" {
		return pathTasks + "/" + url.PathEscape(taskID)
	}
	return byReferenceQuery(cfg, exec.ID)
}

func remoteTaskID(exec execution.Execution) string {
	if exec.Result != nil {
		if id := strings.TrimSpace(exec.Result.ExternalID); id != "" {
			return id
		}
	}
	if exec.Error != nil {
		if id := strings.TrimSpace(exec.Error.ExternalID); id != "" {
			return id
		}
	}
	return ""
}

// runEnvelope mirrors the run the hub exposes on its external namespace.
type runEnvelope struct {
	Run struct {
		ID        string `json:"id"`
		RunnerID  string `json:"runnerId"`
		TaskID    string `json:"taskId"`
		State     string `json:"state"`
		CreatedAt int64  `json:"createdAt"`
		ClosedAt  int64  `json:"closedAt"`
		Error     string `json:"error"`
	} `json:"run"`
}

// approvalsEnvelope mirrors the pending approvals of a run.
type approvalsEnvelope struct {
	Approvals []struct {
		ID        string `json:"id"`
		RunID     string `json:"runId"`
		RunnerID  string `json:"runnerId"`
		CreatedAt int64  `json:"createdAt"`
		Request   struct {
			ToolName string          `json:"toolName"`
			Title    string          `json:"title"`
			Args     json.RawMessage `json:"args"`
			Options  []struct {
				OptionID string `json:"optionId"`
				Name     string `json:"name"`
				Kind     string `json:"kind"`
			} `json:"options"`
		} `json:"request"`
	} `json:"approvals"`
}

// runPath builds the base route of one run.
func runPath(runID string) string {
	return "/api/external/runs/" + url.PathEscape(runID)
}

// prepare resolves the settings and the credential every run call needs.
func (p *Provider) prepare(req execution.RunRequest) (settings, string, error) {
	cfg, err := parseConfig(req.ProviderConfig)
	if err != nil {
		return settings{}, "", err
	}
	token := strings.TrimSpace(p.getenv(cfg.TokenEnv))
	if token == "" {
		return settings{}, "", fmt.Errorf("the ARcipelago application credential is not available: export it in the %s environment variable", cfg.TokenEnv)
	}
	return cfg, token, nil
}

// translateRunState maps the hub's vocabulary onto the neutral one.
//
// An unrecognized state deliberately becomes RunCrashed and never RunActive:
// reading "active" into a state this build does not understand would let the
// UI offer commands on a run that may well be over. The original spelling is
// preserved in Error, so the operator still sees what the hub actually said.
func translateRunState(state string) (execution.RunState, string) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "active":
		return execution.RunActive, ""
	case "closed":
		return execution.RunClosed, ""
	case "crashed":
		return execution.RunCrashed, ""
	default:
		return execution.RunCrashed, fmt.Sprintf("unknown remote run state %q", state)
	}
}

// ReadRun reports the current state of the run as the hub sees it. It is the
// only authority on that state: nothing in this package ever derives one.
func (p *Provider) ReadRun(ctx context.Context, req execution.RunRequest) (execution.RunSnapshot, error) {
	cfg, token, err := p.prepare(req)
	if err != nil {
		return execution.RunSnapshot{}, err
	}
	var envelope runEnvelope
	status, payload, err := p.doWithBody(ctx, cfg, token, http.MethodGet, runPath(req.RunID), nil, &envelope)
	if err != nil {
		return execution.RunSnapshot{}, refusalForBody(status, payload, req.RunID, err)
	}
	state, note := translateRunState(envelope.Run.State)
	snapshot := execution.RunSnapshot{RunID: req.RunID, State: state, Error: strings.TrimSpace(envelope.Run.Error)}
	if snapshot.Error == "" {
		snapshot.Error = note
	}
	if id := strings.TrimSpace(envelope.Run.ID); id != "" {
		snapshot.RunID = id
	}
	if envelope.Run.ClosedAt > 0 {
		closedAt := time.UnixMilli(envelope.Run.ClosedAt).UTC()
		snapshot.ClosedAt = &closedAt
	}
	return snapshot, nil
}

// ReadRunApprovals lists the decisions the run is waiting on. It always returns
// a non-nil slice, so a caller that serializes the result emits an empty list
// rather than a null.
func (p *Provider) ReadRunApprovals(ctx context.Context, req execution.RunRequest) ([]execution.PendingApproval, error) {
	cfg, token, err := p.prepare(req)
	if err != nil {
		return nil, err
	}
	var envelope approvalsEnvelope
	status, payload, err := p.doWithBody(ctx, cfg, token, http.MethodGet, runPath(req.RunID)+"/approvals", nil, &envelope)
	if err != nil {
		return nil, refusalForBody(status, payload, req.RunID, err)
	}
	out := make([]execution.PendingApproval, 0, len(envelope.Approvals))
	for _, approval := range envelope.Approvals {
		options := make([]execution.ApprovalOption, 0, len(approval.Request.Options))
		for _, option := range approval.Request.Options {
			options = append(options, execution.ApprovalOption{ID: option.OptionID, Label: option.Name, Kind: option.Kind})
		}
		pending := execution.PendingApproval{
			ID:       approval.ID,
			ToolName: approval.Request.ToolName,
			Title:    approval.Request.Title,
			Args:     approval.Request.Args,
			Options:  options,
		}
		if approval.CreatedAt > 0 {
			pending.CreatedAt = time.UnixMilli(approval.CreatedAt).UTC()
		}
		out = append(out, pending)
	}
	return out, nil
}

// SendRunMessage delivers an operator message to the run. A 202 means the hub
// accepted the delivery, never that the message is already part of the history:
// it becomes history when the hub republishes it on the event stream.
func (p *Provider) SendRunMessage(ctx context.Context, req execution.RunRequest, message string) error {
	// An empty message is refused before any call: it is the caller's mistake,
	// not a decision of the hub, and sending it would spend a round trip to be
	// told so.
	text := strings.TrimSpace(message)
	if text == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedUnsupported,
			RunID:  req.RunID,
			Err:    fmt.Errorf("the message is empty"),
		}
	}
	cfg, token, err := p.prepare(req)
	if err != nil {
		return err
	}
	body := struct {
		Message string `json:"message"`
	}{Message: text}
	status, payload, err := p.doWithBody(ctx, cfg, token, http.MethodPost, runPath(req.RunID)+"/messages", body, nil)
	if err != nil {
		return refusalForBody(status, payload, req.RunID, err)
	}
	return nil
}

// RespondRunApproval answers one pending approval with one of the options the
// hub declared for it.
func (p *Provider) RespondRunApproval(ctx context.Context, req execution.RunRequest, approvalID, optionID string) error {
	approval := strings.TrimSpace(approvalID)
	option := strings.TrimSpace(optionID)
	if approval == "" || option == "" {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedUnsupported,
			RunID:  req.RunID,
			Err:    fmt.Errorf("both the approval and the option must be named"),
		}
	}
	cfg, token, err := p.prepare(req)
	if err != nil {
		return err
	}
	body := struct {
		OptionID string `json:"optionId"`
	}{OptionID: option}
	path := runPath(req.RunID) + "/approvals/" + url.PathEscape(approval) + "/respond"
	status, payload, err := p.doWithBody(ctx, cfg, token, http.MethodPost, path, body, nil)
	if err != nil {
		return refusalForBody(status, payload, req.RunID, err)
	}
	return nil
}

// CancelRun asks the runner to close the session.
//
// A 202 means the command was delivered to the runner, never that the run is
// over. The terminal state is a fact the runner reports and must be read back
// with ReadRun; deriving it here would show a closed run the moment the request
// left, which is exactly the lie the acceptance criterion forbids.
func (p *Provider) CancelRun(ctx context.Context, req execution.RunRequest) error {
	cfg, token, err := p.prepare(req)
	if err != nil {
		return err
	}
	status, payload, err := p.doWithBody(ctx, cfg, token, http.MethodPost, runPath(req.RunID)+"/cancel", nil, nil)
	if err != nil {
		return refusalForBody(status, payload, req.RunID, err)
	}
	return nil
}
