package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaVersion is the only accepted value for the "schema" field.
const SchemaVersion = "archetipo.analytics/v1"

// denylist contains field names that must never appear in an analytics event.
// These fields could leak secrets, PII, or system internals.
var denylist = map[string]bool{
	"path":        true,
	"cwd":         true,
	"hostname":    true,
	"username":    true,
	"user":        true,
	"email":       true,
	"token":       true,
	"password":    true,
	"secret":      true,
	"key":         true,
	"api_key":     true,
	"credential":  true,
	"credentials": true,
	"env":         true,
	"environment": true,
	"home":        true,
	"homedir":     true,
	"ip":          true,
	"ip_address":  true,
	"machine_id":  true,
	"device_id":   true,
	"mac":         true,
	"mac_address": true,
}

// allowlist is the set of field names that are permitted in an analytics event.
// Derived from the AnalyticsEvent struct JSON tags.
var allowlist = map[string]bool{
	"schema":            true,
	"event":             true,
	"tool":              true,
	"tool_version":      true,
	"os":                true,
	"arch":              true,
	"archetipo_version": true,
	"session_id":        true,
	"timestamp":         true,
	"duration_ms":       true,
	"success":           true,
	"error_code":        true,
	"connector":         true,
	"spec_code":         true,
	"properties":        true,
}

// ValidateEvent performs strict two-phase validation:
//  1. Denylist — rejects any field whose name is in the denylist.
//  2. Allowlist — rejects any field whose name is not in the allowlist.
//
// Additionally it checks that the required "schema" field equals
// SchemaVersion and that the required "event" field is non-empty.
//
// The raw JSON body must be provided so that the denylist check can
// inspect all keys before the struct-level allowlist takes over.
func ValidateEvent(rawBody []byte, event AnalyticsEvent) error {
	// Phase 1: denylist — scan raw JSON for forbidden keys.
	if err := checkDenylist(rawBody); err != nil {
		return err
	}

	// Phase 2a: required fields.
	if event.Schema == "" {
		return &ValidationError{
			Field:     "schema",
			Violation: ViolationMissingSchema,
			Detail:    "the 'schema' field is required",
		}
	}
	if event.Schema != SchemaVersion {
		return &ValidationError{
			Field:     "schema",
			Violation: ViolationWrongSchema,
			Detail:    fmt.Sprintf("expected %q, got %q", SchemaVersion, event.Schema),
		}
	}
	if event.Event == "" {
		return &ValidationError{
			Field:     "event",
			Violation: ViolationMissingRequired,
			Detail:    "the 'event' field is required",
		}
	}

	// Phase 2b: allowlist — check for unknown fields in the raw JSON.
	// DisallowUnknownFields in the decoder handles this at parse time,
	// but we also validate here so callers without decoder-level checks
	// still get a ValidationError.
	if err := checkAllowlist(rawBody); err != nil {
		return err
	}

	return nil
}

// checkDenylist scans the raw JSON object keys and returns a ValidationError
// for any key found in the denylist. Nested keys inside the "properties" map
// are not checked — the denylist applies only to top-level event fields.
func checkDenylist(raw []byte) error {
	keys, err := extractTopLevelKeys(raw)
	if err != nil {
		return &ValidationError{
			Field:     "(body)",
			Violation: ViolationInvalidType,
			Detail:    "unable to parse JSON for denylist check: " + err.Error(),
		}
	}
	for _, k := range keys {
		if denylist[strings.ToLower(k)] {
			return &ValidationError{
				Field:     k,
				Violation: ViolationForbiddenField,
				Detail:    fmt.Sprintf("field %q is forbidden for privacy/security reasons", k),
			}
		}
	}
	return nil
}

// checkAllowlist scans the raw JSON object keys and returns a ValidationError
// for any key not in the allowlist.
func checkAllowlist(raw []byte) error {
	keys, err := extractTopLevelKeys(raw)
	if err != nil {
		return &ValidationError{
			Field:     "(body)",
			Violation: ViolationInvalidType,
			Detail:    "unable to parse JSON for allowlist check: " + err.Error(),
		}
	}
	for _, k := range keys {
		if !allowlist[k] {
			return &ValidationError{
				Field:     k,
				Violation: ViolationUnknownField,
				Detail:    fmt.Sprintf("field %q is not recognized in schema %s", k, SchemaVersion),
			}
		}
	}
	return nil
}

// extractTopLevelKeys returns the top-level keys of a JSON object.
// Returns an error if raw is not a JSON object.
func extractTopLevelKeys(raw []byte) ([]string, error) {
	// Trim whitespace.
	s := strings.TrimSpace(string(raw))
	if len(s) == 0 || s[0] != '{' {
		return nil, fmt.Errorf("not a JSON object")
	}

	// Use a streaming decoder to read just the keys.
	dec := json.NewDecoder(strings.NewReader(s))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}

	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key")
		}
		keys = append(keys, key)
		// Skip the value.
		if err := skipValue(dec); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// skipValue reads and discards the next JSON value from the decoder.
func skipValue(dec *json.Decoder) error {
	var raw json.RawMessage
	return dec.Decode(&raw)
}
