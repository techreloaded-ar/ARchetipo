// Package ingest implements the analytics event ingestion endpoint
// (POST /v1/events) with strict schema validation, rate limiting, and
// IP-anonymised in-memory storage.
package ingest

import (
	"fmt"
	"time"
)

// AnalyticsEvent is the canonical shape of an archetipo.analytics/v1 event.
// Only fields listed here are accepted; all others are rejected either by
// JSON DisallowUnknownFields or by the denylist check in ValidateEvent.
type AnalyticsEvent struct {
	Schema                  string         `json:"schema"`
	Event                   string         `json:"event"`
	Timestamp               string         `json:"timestamp,omitempty"`
	Command                 string         `json:"command,omitempty"`
	Tool                    string         `json:"tool,omitempty"`
	ToolVersion             string         `json:"tool_version,omitempty"`
	OS                      string         `json:"os,omitempty"`
	Arch                    string         `json:"arch,omitempty"`
	ArchetipoVersion        string         `json:"archetipo_version,omitempty"`
	SessionID               string         `json:"session_id,omitempty"`
	DurationMs              int64          `json:"duration_ms,omitempty"`
	Success                 *bool          `json:"success,omitempty"`
	ErrorCode               string         `json:"error_code,omitempty"`
	ExitCode                int            `json:"exit_code,omitempty"`
	CI                      bool           `json:"ci,omitempty"`
	Connector               string         `json:"connector,omitempty"`
	AnonymousInstallationID string         `json:"anonymous_installation_id,omitempty"`
	SpecCode                string         `json:"spec_code,omitempty"`
	Args                    map[string]any `json:"args,omitempty"`
	Properties              map[string]any `json:"properties,omitempty"`
}

// StoredEvent is AnalyticsEvent without any origin-identifying fields.
// IP addresses are never persisted here; the rate limiter uses a hashed
// origin key internally and the storage layer never receives the raw IP.
type StoredEvent struct {
	Schema                  string         `json:"schema"`
	Event                   string         `json:"event"`
	Timestamp               string         `json:"timestamp,omitempty"`
	Command                 string         `json:"command,omitempty"`
	Tool                    string         `json:"tool,omitempty"`
	ToolVersion             string         `json:"tool_version,omitempty"`
	OS                      string         `json:"os,omitempty"`
	Arch                    string         `json:"arch,omitempty"`
	ArchetipoVersion        string         `json:"archetipo_version,omitempty"`
	SessionID               string         `json:"session_id,omitempty"`
	DurationMs              int64          `json:"duration_ms,omitempty"`
	Success                 *bool          `json:"success,omitempty"`
	ErrorCode               string         `json:"error_code,omitempty"`
	ExitCode                int            `json:"exit_code,omitempty"`
	CI                      bool           `json:"ci,omitempty"`
	Connector               string         `json:"connector,omitempty"`
	AnonymousInstallationID string         `json:"anonymous_installation_id,omitempty"`
	SpecCode                string         `json:"spec_code,omitempty"`
	Args                    map[string]any `json:"args,omitempty"`
	Properties              map[string]any `json:"properties,omitempty"`
	// ReceivedAt is set by the storage layer when the event is persisted.
	ReceivedAt time.Time `json:"received_at"`
}

// StoredEventFromAnalytics converts an AnalyticsEvent to a StoredEvent.
func StoredEventFromAnalytics(e AnalyticsEvent) StoredEvent {
	return StoredEvent{
		Schema:                  e.Schema,
		Event:                   e.Event,
		Timestamp:               e.Timestamp,
		Command:                 e.Command,
		Tool:                    e.Tool,
		ToolVersion:             e.ToolVersion,
		OS:                      e.OS,
		Arch:                    e.Arch,
		ArchetipoVersion:        e.ArchetipoVersion,
		SessionID:               e.SessionID,
		DurationMs:              e.DurationMs,
		Success:                 e.Success,
		ErrorCode:               e.ErrorCode,
		ExitCode:                e.ExitCode,
		CI:                      e.CI,
		Connector:               e.Connector,
		AnonymousInstallationID: e.AnonymousInstallationID,
		SpecCode:                e.SpecCode,
		Args:                    e.Args,
		Properties:              e.Properties,
		ReceivedAt:              time.Now(),
	}
}

// Violation describes why a field was rejected during validation.
type Violation string

const (
	ViolationMissingSchema   Violation = "missing_schema"
	ViolationWrongSchema     Violation = "wrong_schema"
	ViolationForbiddenField  Violation = "forbidden_field"
	ViolationUnknownField    Violation = "unknown_field"
	ViolationInvalidType     Violation = "invalid_type"
	ViolationMissingRequired Violation = "missing_required"
)

// ValidationError is returned by ValidateEvent when an event fails schema
// checks. It carries the offending field name and the reason.
type ValidationError struct {
	Field     string    `json:"field"`
	Violation Violation `json:"violation"`
	Detail    string    `json:"detail"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %q: %s — %s", e.Field, e.Violation, e.Detail)
}

// RateLimitConfig configures the token-bucket rate limiter.
type RateLimitConfig struct {
	// Rate is the sustained request rate (requests per window).
	Rate int
	// Window is the time window over which Rate is measured.
	Window time.Duration
	// Burst is the maximum burst size above the sustained rate.
	Burst int
}

// DefaultRateLimitConfig returns the default rate limiting configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Rate:   60,
		Window: 1 * time.Minute,
		Burst:  10,
	}
}

// RateLimiter decides whether a request from a given origin should be allowed.
type RateLimiter interface {
	// Allow returns true if the request should be accepted, false if rate
	// limited. origin is typically a hashed client IP.
	Allow(origin string) bool

	// Status returns the current rate limit state for an origin.
	Status(origin string) RateLimitStatus
}

// RateLimitStatus holds the current token bucket state for an origin.
type RateLimitStatus struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// EventStore persists analytics events.
type EventStore interface {
	// Store saves an event. It must not persist any origin-identifying data.
	Store(event AnalyticsEvent)
	// Events returns all stored events in insertion order.
	Events() []StoredEvent
	// Len returns the number of stored events.
	Len() int
}
