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
	ActionReview    ActionID = "review"
	ActionInception ActionID = "inception"
	ActionBacklog   ActionID = "backlog"
	// ActionSpecDraft is the assisted authoring of a single spec: the agent
	// interviews the person and proposes a spec, which somebody else then
	// confirms. It is deliberately not a variant of ActionBacklog — that action
	// generates a whole backlog and persists it, this one persists nothing.
	ActionSpecDraft ActionID = "spec-draft"

	CapabilitySpecPlan Capability = "spec.plan"
	// CapabilitySpecImplement says that a provider can execute the persisted
	// plan of a spec up to the point where that spec is ready for review. It is
	// deliberately distinct from CapabilitySpecPlan: writing a plan and carrying
	// it out are different pieces of work, and a provider that can do one is not
	// thereby able to do the other.
	CapabilitySpecImplement Capability = "spec.implement"
	// CapabilitySpecReview says that a provider can prepare the review evidence
	// of a spec that is already in review: read the acceptance criteria, the
	// diff, the tests and the documentation state, and leave a dossier behind.
	//
	// It contains no power to decide. Approving an increment or sending it back
	// is not an execution action at all: it is a human verdict, reached through
	// an entry point no provider can reach. A provider that declares this
	// capability is saying it can do the instruction, never that it can close
	// the spec.
	CapabilitySpecReview         Capability = "spec.review"
	CapabilityWorkspaceInception Capability = "workspace.inception"
	CapabilityWorkspaceBacklog   Capability = "workspace.backlog"
	// CapabilityWorkspaceSpecDraft says that a provider can interview a person
	// about one story and propose a spec conforming to the workspace's backlog:
	// title, epic, priority, points, scope, blockers and body.
	//
	// Writing that spec is not part of it, and the omission is the whole point.
	// The proposal is handed back inside the execution record and a person
	// confirms it through the ordinary spec creation route, so a provider that
	// declares this capability is saying it can draft, never that it can add
	// anything to the backlog.
	CapabilityWorkspaceSpecDraft Capability = "workspace.spec_draft"
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
	ExecutionID string     `json:"execution_id"`
	SpecCode    string     `json:"spec_code"`
	Action      ActionID   `json:"action"`
	Capability  Capability `json:"capability"`
	// WorkingDir is the directory the run was started in: the project root of
	// the workspace that asked for it, frozen at start. It travels on the
	// request and not on the provider because the provider is shared by every
	// workspace a long-lived process serves, while where a run has to execute
	// is a fact of the workspace that started it. Empty means the provider
	// falls back to its own default, so a caller that never sets it behaves
	// exactly as before.
	WorkingDir     string         `json:"working_dir,omitempty"`
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
	// ModelChoice is the model and options the run was *actually started with*,
	// not an override somebody typed. It is therefore populated even when
	// nobody chose anything, because "the one the workspace configures" is an
	// answer to which model ran and not an absence of one; Source is what tells
	// the two apart. It stays a pointer with omitempty so records written
	// before this field existed deserialize unchanged.
	ModelChoice *ModelChoice `json:"model_choice,omitempty"`
	// WorkingDir is the directory this run was *actually started in*, frozen at
	// start and never rewritten afterwards: opening another workspace while the
	// run is in flight cannot change where that run is executing, so the record
	// keeps saying where it really ran. omitempty keeps records written before
	// this field existed deserializable unchanged.
	WorkingDir string `json:"working_dir,omitempty"`
}

type ActionError struct{ Action ActionID }

func (e *ActionError) Error() string { return fmt.Sprintf("unsupported execution action %q", e.Action) }

func RequiredCapability(action ActionID) (Capability, error) {
	switch action {
	case ActionPlan:
		return CapabilitySpecPlan, nil
	case ActionImplement:
		return CapabilitySpecImplement, nil
	case ActionReview:
		return CapabilitySpecReview, nil
	case ActionInception:
		return CapabilityWorkspaceInception, nil
	case ActionBacklog:
		return CapabilityWorkspaceBacklog, nil
	case ActionSpecDraft:
		return CapabilityWorkspaceSpecDraft, nil
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
	case ActionPlan, ActionImplement, ActionReview:
		return ScopeSpec, nil
	case ActionInception, ActionBacklog, ActionSpecDraft:
		return ScopeWorkspace, nil
	default:
		return "", &ActionError{Action: action}
	}
}
