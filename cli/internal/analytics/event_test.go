package analytics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEventMarshalsOnlyAllowlistFields(t *testing.T) {
	success := true
	e := Event{
		Schema:                  "archetipo.analytics/v1",
		Event:                   "command_completed",
		Timestamp:               "2026-06-22T12:00:00Z",
		Command:                 "spec show",
		ArchetipoVersion:        "1.0.0",
		SessionID:               "sess-abc",
		OS:                      "darwin",
		Arch:                    "arm64",
		Connector:               "file",
		Success:                 &success,
		ErrorCode:               "",
		ExitCode:                0,
		DurationMs:              42,
		CI:                      false,
		AnonymousInstallationID: "anon-abc123",
	}

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Allowed fields from unified archetipo.analytics/v1 allowlist.
	allowed := map[string]bool{
		"schema":                    true,
		"event":                     true,
		"timestamp":                 true,
		"command":                   true,
		"tool":                      true,
		"tool_version":              true,
		"archetipo_version":         true,
		"session_id":                true,
		"os":                        true,
		"arch":                      true,
		"connector":                 true,
		"success":                   true,
		"error_code":                true,
		"exit_code":                 true,
		"duration_ms":               true,
		"ci":                        true,
		"anonymous_installation_id": true,
	}

	for key := range m {
		if !allowed[key] {
			t.Errorf("forbidden field %q found in JSON output", key)
		}
	}
}

func TestEventEmptyFieldsOmitted(t *testing.T) {
	e := Event{
		Schema:  "archetipo.analytics/v1",
		Event:   "command_completed",
		Command: "spec show",
	}

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(raw)

	// These empty/zero fields must NOT appear in the output.
	forbidden := []string{
		`"archetipo_version"`,
		`"os"`,
		`"arch"`,
		`"connector"`,
		`"success"`,
		`"error_code"`,
		`"exit_code"`,
		`"ci"`,
		`"duration_ms"`,
		`"anonymous_installation_id"`,
		`"session_id"`,
		`"timestamp"`,
	}

	for _, f := range forbidden {
		if strings.Contains(jsonStr, f) {
			t.Errorf("empty field %s should be omitted but appeared in: %s", f, jsonStr)
		}
	}
}

func TestEventCommandCompletedFixture(t *testing.T) {
	success := true
	e := Event{
		Schema:                  "archetipo.analytics/v1",
		Event:                   "command_completed",
		Timestamp:               "2026-06-22T12:00:00Z",
		Command:                 "spec start",
		ArchetipoVersion:        "2.3.1",
		OS:                      "linux",
		Arch:                    "amd64",
		Connector:               "github",
		SessionID:               "sess-xyz",
		Success:                 &success,
		ErrorCode:               "",
		ExitCode:                0,
		DurationMs:              1250,
		CI:                      true,
		AnonymousInstallationID: "anon-def456",
	}

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if m["schema"] != "archetipo.analytics/v1" {
		t.Errorf("schema = %v, want archetipo.analytics/v1", m["schema"])
	}
	if m["event"] != "command_completed" {
		t.Errorf("event = %v, want command_completed", m["event"])
	}
	if m["command"] != "spec start" {
		t.Errorf("command = %v, want spec start", m["command"])
	}
	if m["duration_ms"] != float64(1250) {
		t.Errorf("duration_ms = %v, want 1250", m["duration_ms"])
	}
	if m["ci"] != true {
		t.Errorf("ci = %v, want true", m["ci"])
	}
}

func TestEventNoForbiddenFields(t *testing.T) {
	e := Event{
		Schema:  "archetipo.analytics/v1",
		Event:   "command_completed",
		Command: "spec show",
	}

	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	jsonStr := string(raw)

	// US-001 denied fields must never appear.
	denied := []string{
		`"path"`,
		`"hostname"`,
		`"token"`,
		`"api_key"`,
		`"secret"`,
		`"properties"`,
		`"metadata"`,
		`"tags"`,
	}

	for _, f := range denied {
		if strings.Contains(jsonStr, f) {
			t.Errorf("denied field %s found in JSON output: %s", f, jsonStr)
		}
	}
}
