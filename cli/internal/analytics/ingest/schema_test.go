package ingest

import (
	"testing"
)

func TestValidateEvent_Valid(t *testing.T) {
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"command_completed","timestamp":"2026-06-19T12:00:00Z","command":"spec.show","tool":"test-tool","tool_version":"1.0.0","os":"darwin","arch":"arm64","archetipo_version":"1.2.0","session_id":"sess-123","duration_ms":150,"success":true,"exit_code":0,"ci":false}`)
	evt := AnalyticsEvent{
		Schema:           "archetipo.analytics/v1",
		Event:            "command_completed",
		Timestamp:        "2026-06-19T12:00:00Z",
		Command:          "spec.show",
		Tool:             "test-tool",
		ToolVersion:      "1.0.0",
		OS:               "darwin",
		Arch:             "arm64",
		ArchetipoVersion: "1.2.0",
		SessionID:        "sess-123",
		DurationMs:       150,
		Success:          boolPtr(true),
		ExitCode:         0,
		CI:               false,
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateEvent_ValidMinimal(t *testing.T) {
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"cli.start"}`)
	evt := AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "cli.start",
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected no error for minimal event, got: %v", err)
	}
}

func TestValidateEvent_ForbiddenField(t *testing.T) {
	tests := []struct {
		name  string
		field string
		raw   string
	}{
		{"path", "path", `{"schema":"archetipo.analytics/v1","event":"x","path":"/home/user"}`},
		{"hostname", "hostname", `{"schema":"archetipo.analytics/v1","event":"x","hostname":"myhost"}`},
		{"username", "username", `{"schema":"archetipo.analytics/v1","event":"x","username":"admin"}`},
		{"email", "email", `{"schema":"archetipo.analytics/v1","event":"x","email":"a@b.com"}`},
		{"token", "token", `{"schema":"archetipo.analytics/v1","event":"x","token":"abc123"}`},
		{"password", "password", `{"schema":"archetipo.analytics/v1","event":"x","password":"secret"}`},
		{"secret", "secret", `{"schema":"archetipo.analytics/v1","event":"x","secret":"key"}`},
		{"key", "key", `{"schema":"archetipo.analytics/v1","event":"x","key":"val"}`},
		{"api_key", "api_key", `{"schema":"archetipo.analytics/v1","event":"x","api_key":"k"}`},
		{"credential", "credential", `{"schema":"archetipo.analytics/v1","event":"x","credential":"x"}`},
		{"cwd", "cwd", `{"schema":"archetipo.analytics/v1","event":"x","cwd":"/tmp"}`},
		{"env", "env", `{"schema":"archetipo.analytics/v1","event":"x","env":"prod"}`},
		{"home", "home", `{"schema":"archetipo.analytics/v1","event":"x","home":"/root"}`},
		{"ip", "ip", `{"schema":"archetipo.analytics/v1","event":"x","ip":"1.2.3.4"}`},
		{"machine_id", "machine_id", `{"schema":"archetipo.analytics/v1","event":"x","machine_id":"abc"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(tt.raw)
			evt := AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "x"}
			err := ValidateEvent(raw, evt)
			if err == nil {
				t.Fatalf("expected ValidationError for forbidden field %q", tt.field)
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if ve.Violation != ViolationForbiddenField {
				t.Fatalf("expected violation %q, got %q", ViolationForbiddenField, ve.Violation)
			}
		})
	}
}

func TestValidateEvent_UnknownField(t *testing.T) {
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"x","unknown_field":"value"}`)
	evt := AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "x"}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for unknown field")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationUnknownField {
		t.Fatalf("expected violation %q, got %q", ViolationUnknownField, ve.Violation)
	}
}

func TestValidateEvent_MissingSchema(t *testing.T) {
	raw := []byte(`{"event":"x"}`)
	evt := AnalyticsEvent{Event: "x"}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for missing schema")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationMissingSchema {
		t.Fatalf("expected violation %q, got %q", ViolationMissingSchema, ve.Violation)
	}
}

func TestValidateEvent_WrongSchema(t *testing.T) {
	raw := []byte(`{"schema":"wrong/v1","event":"x"}`)
	evt := AnalyticsEvent{Schema: "wrong/v1", Event: "x"}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for wrong schema")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationWrongSchema {
		t.Fatalf("expected violation %q, got %q", ViolationWrongSchema, ve.Violation)
	}
}

func TestValidateEvent_MissingEvent(t *testing.T) {
	raw := []byte(`{"schema":"archetipo.analytics/v1"}`)
	evt := AnalyticsEvent{Schema: "archetipo.analytics/v1"}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for missing event")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationMissingRequired {
		t.Fatalf("expected violation %q, got %q", ViolationMissingRequired, ve.Violation)
	}
}

func TestValidateEvent_NonObjectJSON(t *testing.T) {
	raw := []byte(`"just a string"`)
	evt := AnalyticsEvent{}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for non-object JSON")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationInvalidType {
		t.Fatalf("expected violation %q, got %q", ViolationInvalidType, ve.Violation)
	}
}

func TestValidateEvent_EmptyBody(t *testing.T) {
	raw := []byte(``)
	evt := AnalyticsEvent{}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for empty body")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationInvalidType {
		t.Fatalf("expected violation %q, got %q", ViolationInvalidType, ve.Violation)
	}
}

func TestValidateEvent_WithProperties(t *testing.T) {
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"x","properties":{"custom":"value","nested":{"a":1}}}`)
	evt := AnalyticsEvent{
		Schema:     "archetipo.analytics/v1",
		Event:      "x",
		Properties: map[string]any{"custom": "value", "nested": map[string]any{"a": float64(1)}},
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected no error for event with properties, got: %v", err)
	}
}

func TestValidateEvent_CommandCompletedRoundTrip(t *testing.T) {
	// A command_completed event from the client must be accepted by the server.
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"command_completed","timestamp":"2026-06-22T12:00:00Z","command":"spec.plan","archetipo_version":"1.0.0","session_id":"abc-def","os":"linux","arch":"amd64","connector":"file","success":false,"error_code":"E_INVALID_INPUT","exit_code":2,"duration_ms":1234,"ci":true,"anonymous_installation_id":"uuid-1234"}`)
	evt := AnalyticsEvent{
		Schema:                  "archetipo.analytics/v1",
		Event:                   "command_completed",
		Timestamp:               "2026-06-22T12:00:00Z",
		Command:                 "spec.plan",
		ArchetipoVersion:        "1.0.0",
		SessionID:               "abc-def",
		OS:                      "linux",
		Arch:                    "amd64",
		Connector:               "file",
		Success:                 boolPtr(false),
		ErrorCode:               "E_INVALID_INPUT",
		ExitCode:                2,
		DurationMs:              1234,
		CI:                      true,
		AnonymousInstallationID: "uuid-1234",
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected command_completed to be accepted, got: %v", err)
	}
}

func TestValidateEvent_VersionFieldRejected(t *testing.T) {
	// The old "version" field must be rejected as unknown.
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"command_completed","version":"1.0.0"}`)
	evt := AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "command_completed"}
	err := ValidateEvent(raw, evt)
	if err == nil {
		t.Fatal("expected ValidationError for 'version' field (now unknown)")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Violation != ViolationUnknownField {
		t.Fatalf("expected violation %q, got %q", ViolationUnknownField, ve.Violation)
	}
	if ve.Field != "version" {
		t.Fatalf("expected field=version, got %q", ve.Field)
	}
}

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{Field: "test_field", Violation: ViolationForbiddenField, Detail: "bad"}
	msg := ve.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestValidateEvent_WithArgs(t *testing.T) {
	// Events with a valid "args" field should pass validation.
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"command_completed","args":{"status":"TODO"}}`)
	evt := AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "command_completed",
		Args:   map[string]any{"status": "TODO"},
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected no error for event with args, got: %v", err)
	}
}

func TestValidateEvent_ArgsWithMultipleKeys(t *testing.T) {
	// An args map with multiple keys (flag names, positional) should validate.
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"command_completed","args":{"file":true,"status":"TODO","_0":"US-001"}}`)
	evt := AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "command_completed",
		Args: map[string]any{
			"file":   true,
			"status": "TODO",
			"_0":     "US-001",
		},
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected no error for event with multiple args, got: %v", err)
	}
}

func TestValidateEvent_ArgsWithToolSlice(t *testing.T) {
	// Args containing a []string (from StringSlice flags) should validate.
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"command_completed","args":{"tool":["claude","pi"]}}`)
	evt := AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "command_completed",
		Args:   map[string]any{"tool": []any{"claude", "pi"}},
	}
	if err := ValidateEvent(raw, evt); err != nil {
		t.Fatalf("expected no error for args with slice value, got: %v", err)
	}
}

func boolPtr(b bool) *bool { return &b }
