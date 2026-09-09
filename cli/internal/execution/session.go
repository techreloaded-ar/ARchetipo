package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ValidateTurnConfiguration checks a next-turn choice against the catalog
// discovered in the actual native session environment.
func ValidateTurnConfiguration(models []ModelOption, configuration TurnConfiguration) error {
	model, found := findModel(models, strings.TrimSpace(configuration.Model))
	if !found {
		return fmt.Errorf("model must be one of %s", strings.Join(modelIDs(models), ", "))
	}
	declared := make(map[string]ModelOptionField, len(model.Options))
	for _, option := range model.Options {
		declared[option.Name] = option
	}
	for _, name := range sortedKeys(configuration.Options) {
		value := configuration.Options[name]
		option, ok := declared[name]
		if !ok {
			return fmt.Errorf("%s is not an option of model %s", name, model.ID)
		}
		if !hasChoice(option.Choices, value) {
			return fmt.Errorf("%s must be one of %s", name, strings.Join(choiceValues(option.Choices), ", "))
		}
	}
	return nil
}

// SessionCapability is an optional behaviour of a native harness session.
// Creation, event streaming and release are the baseline SessionProvider
// contract; everything else is advertised only after discovery in the actual
// provider environment.
type SessionCapability string

const (
	SessionCapabilityResume          SessionCapability = "session.resume"
	SessionCapabilityTurnModel       SessionCapability = "turn.model"
	SessionCapabilityTurnOptions     SessionCapability = "turn.options"
	SessionCapabilitySkillDiscovery  SessionCapability = "skill.discovery"
	SessionCapabilitySkillInvocation SessionCapability = "skill.invoke"
	SessionCapabilityInput           SessionCapability = "turn.input"
	SessionCapabilityApproval        SessionCapability = "turn.approval"
	SessionCapabilitySteering        SessionCapability = "turn.steering"
	SessionCapabilityInterrupt       SessionCapability = "turn.interrupt"
)

// SessionEnvironment freezes where and how a conversation runs. ProviderConfig
// contains non-secret values only; credentials remain in the environment named
// by that configuration.
type SessionEnvironment struct {
	WorkingDir     string         `json:"working_dir"`
	ProviderConfig map[string]any `json:"provider_config,omitempty"`
	Location       string         `json:"location"` // for example local or remote
}

// NativeSessionReference is the durable provider-owned identity. ID is not an
// ARchetipo conversation or execution id. Attributes hold additional
// non-secret provider references such as a remote task or runner id.
type NativeSessionReference struct {
	Kind       string            `json:"kind"`
	ID         string            `json:"id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// SessionMetadata is the durable identity of a conversation. One conversation
// owns one native session and may be linked to any number of executions.
type SessionMetadata struct {
	ConversationID string                 `json:"conversation_id"`
	ProviderID     string                 `json:"provider_id"`
	Environment    SessionEnvironment     `json:"environment"`
	Native         NativeSessionReference `json:"native"`
}

// ArchiveState is deliberately independent from runtime and turn state.
type ArchiveState string

const (
	ArchiveOpen     ArchiveState = "OPEN"
	ArchiveArchived ArchiveState = "ARCHIVED"
)

// ConnectionState describes only the current transport/runtime attachment.
type ConnectionState string

const (
	ConnectionConnected    ConnectionState = "CONNECTED"
	ConnectionDisconnected ConnectionState = "DISCONNECTED"
	ConnectionReleased     ConnectionState = "RELEASED"
)

// RecoveryState says whether the durable native reference can be resumed. A
// disconnected session is recoverable only when this value is RESUMABLE.
type RecoveryState string

const (
	RecoveryResumable     RecoveryState = "RESUMABLE"
	RecoveryUnrecoverable RecoveryState = "UNRECOVERABLE"
)

// SessionWorkState describes what the harness is doing without conflating it
// with archive or connection state.
type SessionWorkState string

const (
	SessionIdle            SessionWorkState = "IDLE"
	SessionTurnActive      SessionWorkState = "TURN_ACTIVE"
	SessionWaitingInput    SessionWorkState = "WAITING_INPUT"
	SessionWaitingApproval SessionWorkState = "WAITING_APPROVAL"
)

type TurnState string

const (
	TurnActive          TurnState = "ACTIVE"
	TurnWaitingInput    TurnState = "WAITING_INPUT"
	TurnWaitingApproval TurnState = "WAITING_APPROVAL"
	TurnCompleted       TurnState = "COMPLETED"
	TurnInterrupted     TurnState = "INTERRUPTED"
	TurnFailed          TurnState = "FAILED"
)

// TurnConfiguration keeps the requested and provider-applied values separate.
// Options include effort when the selected model exposes it.
type TurnConfiguration struct {
	Model   string            `json:"model,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

// SessionSkill is one skill reported by the actual harness in this session's
// environment. Path is the native invocation reference when the harness has
// one; it is not inferred by scanning another provider's directories.
type SessionSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
}

// SessionTurn has a core id, an optional provider id and an optional execution
// link. The execution link does not own the turn or the session: several turns
// may name the same execution and ordinary turns name none.
type SessionTurn struct {
	ID          string            `json:"id"`
	NativeID    string            `json:"native_id,omitempty"`
	ExecutionID string            `json:"execution_id,omitempty"`
	State       TurnState         `json:"state"`
	Requested   TurnConfiguration `json:"requested"`
	Applied     TurnConfiguration `json:"applied"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// DeliveryState makes the crash boundary explicit. UNSENT is safe to retry;
// UNCERTAIN must be reconciled with the native session and is never retried
// automatically because the original submission may already have had effects.
type DeliveryState string

const (
	DeliveryUnsent    DeliveryState = "UNSENT"
	DeliveryConfirmed DeliveryState = "CONFIRMED"
	DeliveryUncertain DeliveryState = "UNCERTAIN"
)

type SessionDelivery struct {
	SubmissionID     string        `json:"submission_id"`
	TurnID           string        `json:"turn_id"`
	State            DeliveryState `json:"state"`
	NativeDeliveryID string        `json:"native_delivery_id,omitempty"`
}

// PendingInput is a provider request that needs structured user input. Payload
// stays provider-neutral and preserves the native questions without inventing
// a common question language a harness may not support.
type PendingInput struct {
	ID        string          `json:"id"`
	Title     string          `json:"title,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type SessionSnapshot struct {
	Session      SessionMetadata  `json:"session"`
	Archive      ArchiveState     `json:"archive"`
	Connection   ConnectionState  `json:"connection"`
	ConnectionID string           `json:"connection_id,omitempty"`
	Recovery     RecoveryState    `json:"recovery"`
	Work         SessionWorkState `json:"work"`
	// CurrentTurn is the active turn or the most recently observed terminal
	// turn. Work is the authority on whether it is active.
	CurrentTurn      *SessionTurn      `json:"current_turn,omitempty"`
	PendingInputs    []PendingInput    `json:"pending_inputs"`
	PendingApprovals []PendingApproval `json:"pending_approvals"`
}

type SessionDiscoveryRequest struct {
	ProviderConfig map[string]any          `json:"provider_config,omitempty"`
	WorkingDir     string                  `json:"working_dir"`
	Native         *NativeSessionReference `json:"native,omitempty"`
}

type SessionDiscovery struct {
	Capabilities []SessionCapability `json:"capabilities"`
	Models       []ModelOption       `json:"models"`
	Skills       []SessionSkill      `json:"skills"`
	Environment  SessionEnvironment  `json:"environment"`
}

type CreateSessionRequest struct {
	ConversationID string             `json:"conversation_id"`
	Environment    SessionEnvironment `json:"environment"`
}

type ResumeSessionRequest struct {
	Session SessionMetadata `json:"session"`
}

type SessionRequest struct {
	Session SessionMetadata `json:"session"`
}

type StartTurnRequest struct {
	Session      SessionMetadata   `json:"session"`
	TurnID       string            `json:"turn_id"`
	SubmissionID string            `json:"submission_id"`
	ExecutionID  string            `json:"execution_id,omitempty"`
	Message      string            `json:"message"`
	Model        string            `json:"model,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	// Skills are references selected from DiscoverSession. A provider validates
	// them against the current native catalog before invocation.
	Skills []SessionSkill `json:"skills,omitempty"`
}

// SessionCommandRequest correlates every command sent during an active turn.
// SteerTurn uses Message, approval responses use InteractionID and OptionID,
// and structured input responses use InteractionID and Payload.
type SessionCommandRequest struct {
	Session       SessionMetadata `json:"session"`
	TurnID        string          `json:"turn_id"`
	SubmissionID  string          `json:"submission_id"`
	Message       string          `json:"message,omitempty"`
	InteractionID string          `json:"interaction_id,omitempty"`
	OptionID      string          `json:"option_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type SessionTurnStarted struct {
	Turn     SessionTurn     `json:"turn"`
	Delivery SessionDelivery `json:"delivery"`
}

// SessionProvider is the provider-neutral boundary for durable native harness
// sessions. It owns no process rules and creates no execution records. Command
// methods return their SessionDelivery even with a non-nil error: after a
// transport failure its state says whether an explicit retry is safe or the
// native session must first be reconciled.
type SessionProvider interface {
	DiscoverSession(context.Context, SessionDiscoveryRequest) (SessionDiscovery, error)
	CreateSession(context.Context, CreateSessionRequest) (SessionSnapshot, error)
	ResumeSession(context.Context, ResumeSessionRequest) (SessionSnapshot, error)
	ReadSession(context.Context, SessionRequest) (SessionSnapshot, error)
	StartTurn(context.Context, StartTurnRequest) (SessionTurnStarted, error)
	StreamSessionEvents(context.Context, SessionRequest, int64, func(RunEvent) error) error
	SteerTurn(context.Context, SessionCommandRequest) (SessionDelivery, error)
	RespondSessionInput(context.Context, SessionCommandRequest) (SessionDelivery, error)
	RespondSessionApproval(context.Context, SessionCommandRequest) (SessionDelivery, error)
	InterruptTurn(context.Context, SessionCommandRequest) (SessionDelivery, error)
	ReleaseSession(context.Context, SessionRequest) error
}

func SessionProviderFor(provider Provider) (SessionProvider, bool) {
	if provider == nil {
		return nil, false
	}
	sessions, ok := provider.(SessionProvider)
	return sessions, ok
}

func NormalizeSessionCapabilities(capabilities []SessionCapability) []SessionCapability {
	seen := make(map[SessionCapability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability != "" {
			seen[capability] = struct{}{}
		}
	}
	out := make([]SessionCapability, 0, len(seen))
	for capability := range seen {
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func SupportsSessionCapability(capabilities []SessionCapability, required SessionCapability) bool {
	capabilities = NormalizeSessionCapabilities(capabilities)
	index := sort.Search(len(capabilities), func(i int) bool { return capabilities[i] >= required })
	return index < len(capabilities) && capabilities[index] == required
}
