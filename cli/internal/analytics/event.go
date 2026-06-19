// Package analytics implements the client-side telemetry sender
// for archetipo CLI commands. It builds events with exactly the
// fields permitted by the US-001 allowlist and sends them via HTTP
// with opt-out, timeout, and network resilience built in.
package analytics

const (
	// DefaultSchema is the analytics event schema version.
	DefaultSchema = "archetipo.analytics/v1"
	// EventCommandCompleted is emitted after every CLI command execution.
	EventCommandCompleted = "command_completed"
)

// Event is a client-side analytics event whose shape is restricted
// to the US-001 allowlist. No generic or untyped fields are permitted
// so that the caller cannot accidentally leak arbitrary runtime data.
type Event struct {
	Schema                  string `json:"schema,omitempty"`
	Event                   string `json:"event,omitempty"`
	Command                 string `json:"command,omitempty"`
	Version                 string `json:"version,omitempty"`
	OS                      string `json:"os,omitempty"`
	Arch                    string `json:"arch,omitempty"`
	Connector               string `json:"connector,omitempty"`
	Success                 *bool  `json:"success,omitempty"`
	ErrorCode               string `json:"error_code,omitempty"`
	ExitCode                int    `json:"exit_code,omitempty"`
	DurationMs              int64  `json:"duration_ms,omitempty"`
	CI                      bool   `json:"ci,omitempty"`
	AnonymousInstallationID string `json:"anonymous_installation_id,omitempty"`
}
