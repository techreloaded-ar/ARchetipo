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

const (
	ActionPlan         ActionID        = "plan"
	CapabilitySpecPlan Capability      = "spec.plan"
	StatusRunning      ExecutionStatus = "RUNNING"
	StatusSucceeded    ExecutionStatus = "SUCCEEDED"
	StatusFailed       ExecutionStatus = "FAILED"
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
}

type Execution struct {
	ID               string          `json:"id"`
	SpecCode         string          `json:"spec_code"`
	Action           ActionID        `json:"action"`
	Capability       Capability      `json:"capability"`
	ProviderID       string          `json:"provider_id"`
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
	default:
		return "", &ActionError{Action: action}
	}
}
