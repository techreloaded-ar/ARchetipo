package execution

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

type ActionID string
type Capability string
type ExecutionStatus string

// An action is what the caller asks for; a capability is what a provider
// declares it can do. The two are kept distinct on purpose: the caller never
// names a capability, and a provider never names an action.
const (
	ActionPlan      ActionID = "plan"
	ActionImplement ActionID = "implement"
	ActionInception ActionID = "inception"
	ActionBacklog   ActionID = "backlog"

	CapabilitySpecPlan Capability = "spec.plan"
	// CapabilitySpecImplement says that a provider can execute the persisted
	// plan of a spec up to the point where that spec is ready for review. It is
	// deliberately distinct from CapabilitySpecPlan: writing a plan and carrying
	// it out are different pieces of work, and a provider that can do one is not
	// thereby able to do the other.
	CapabilitySpecImplement      Capability = "spec.implement"
	CapabilityWorkspaceInception Capability = "workspace.inception"
	CapabilityWorkspaceBacklog   Capability = "workspace.backlog"
	// CapabilityRunDialog says that a provider exposes a run one can follow and
	// command while it works: read its history, send it a message, cancel it. It
	// is not a statement about the work the provider can do — a provider can
	// plan a spec without offering any conversation, and the two capabilities
	// are therefore independent.
	//
	// It is never declared by hand in a provider's Capabilities: see
	// DeclaredCapabilities, which derives it from the interface the provider
	// actually implements.
	CapabilityRunDialog Capability      = "run.dialog"
	StatusRunning       ExecutionStatus = "RUNNING"
	StatusSucceeded     ExecutionStatus = "SUCCEEDED"
	StatusFailed        ExecutionStatus = "FAILED"
)

type Request struct {
	ExecutionID    string         `json:"execution_id"`
	SpecCode       string         `json:"spec_code"`
	Action         ActionID       `json:"action"`
	Capability     Capability     `json:"capability"`
	ProviderConfig map[string]any `json:"provider_config,omitempty"`
}

type Result struct {
	Payload    json.RawMessage `json:"payload"`
	ExternalID string          `json:"external_id,omitempty"`
}

type ExecutionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// ExternalID names the remote unit of work when the failure happened after
	// that work already existed. Without it the identifier of a task that is
	// still alive on the remote system would live only inside prose, so a
	// program could not follow it.
	ExternalID string `json:"external_id,omitempty"`
}

// RemoteError is the error a provider returns when the dispatch failed after a
// remote unit of work had already been created. It carries the identifier of
// that work so the failed record can name it in a structured field and not only
// in the message.
type RemoteError struct {
	ExternalID string
	Err        error
}

func (e *RemoteError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("remote work %q failed", e.ExternalID)
	}
	return e.Err.Error()
}

func (e *RemoteError) Unwrap() error { return e.Err }

type Execution struct {
	ID               string          `json:"id"`
	SpecCode         string          `json:"spec_code"`
	Action           ActionID        `json:"action"`
	Capability       Capability      `json:"capability"`
	ProviderID       string          `json:"provider_id"`
	RequestID        string          `json:"request_id,omitempty"`
	SpecStatusBefore domain.Status   `json:"spec_status_before"`
	Status           ExecutionStatus `json:"status"`
	Result           *Result         `json:"result,omitempty"`
	Error            *ExecutionError `json:"error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

type ActionError struct{ Action ActionID }

func (e *ActionError) Error() string { return fmt.Sprintf("unsupported execution action %q", e.Action) }

func RequiredCapability(action ActionID) (Capability, error) {
	switch action {
	case ActionPlan:
		return CapabilitySpecPlan, nil
	case ActionImplement:
		return CapabilitySpecImplement, nil
	case ActionInception:
		return CapabilityWorkspaceInception, nil
	case ActionBacklog:
		return CapabilityWorkspaceBacklog, nil
	default:
		return "", &ActionError{Action: action}
	}
}

// Scope says what an action acts upon. It is derived from the action and never
// stored on the execution record: the action determines the scope totally, and
// a stored field that can be derived is a field that can contradict what it is
// derived from.
type Scope string

const (
	ScopeSpec      Scope = "spec"
	ScopeWorkspace Scope = "workspace"
)

// ActionScope reports whether the object of an action is a single spec or the
// whole workspace.
func ActionScope(action ActionID) (Scope, error) {
	switch action {
	case ActionPlan, ActionImplement:
		return ScopeSpec, nil
	case ActionInception, ActionBacklog:
		return ScopeWorkspace, nil
	default:
		return "", &ActionError{Action: action}
	}
}
