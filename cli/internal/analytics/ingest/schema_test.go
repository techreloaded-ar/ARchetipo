package ingest

import (
	"testing"
)

func TestValidateEvent_Valid(t *testing.T) {
	raw := []byte(`{"schema":"archetipo.analytics/v1","event":"cli.invocation","tool":"test-tool","tool_version":"1.0.0","os":"darwin","arch":"arm64","archetipo_version":"1.2.0","session_id":"sess-123","timestamp":"2026-06-19T12:00:00Z","duration_ms":150,"success":true}`)
	evt := AnalyticsEvent{
		Schema:           "archetipo.analytics/v1",
		Event:            "cli.invocation",
		Tool:             "test-tool",
		ToolVersion:      "1.0.0",
		OS:               "darwin",
		Arch:             "arm64",
		ArchetipoVersion: "1.2.0",
		SessionID:        "sess-123",
		Timestamp:        "2026-06-19T12:00:00Z",
		DurationMs:       150,
		Success:          boolPtr(true),
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

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{Field: "test_field", Violation: ViolationForbiddenField, Detail: "bad"}
	msg := ve.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func boolPtr(b bool) *bool { return &b }
