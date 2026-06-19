package analytics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSendSuccess(t *testing.T) {
	var (
		gotBody        []byte
		gotContentType string
		gotUserAgent   string
		callCount      int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		gotBody, _ = io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		gotUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Settings{
		Enabled:   true,
		Endpoint:  srv.URL,
		Timeout:   5 * time.Second,
		UserAgent: "archetipo-analytics/dev",
	}, nil)

	success := true
	e := Event{
		Schema:                  "archetipo.analytics/v1",
		Event:                   "command_completed",
		Command:                 "spec show",
		Version:                 "1.0.0",
		OS:                      "darwin",
		Arch:                    "arm64",
		Connector:               "file",
		Success:                 &success,
		DurationMs:              42,
		AnonymousInstallationID: "anon-abc",
	}

	err := c.Send(context.Background(), e)
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}

	// Verify Content-Type
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	// Verify User-Agent
	if gotUserAgent != "archetipo-analytics/dev" {
		t.Errorf("User-Agent = %q, want archetipo-analytics/dev", gotUserAgent)
	}

	// Verify body is valid JSON with correct fields
	var m map[string]any
	if err := json.Unmarshal(gotBody, &m); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if m["schema"] != "archetipo.analytics/v1" {
		t.Errorf("schema = %v", m["schema"])
	}
	if m["event"] != "command_completed" {
		t.Errorf("event = %v", m["event"])
	}
	if m["command"] != "spec show" {
		t.Errorf("command = %v", m["command"])
	}

	// Verify no extra fields
	allowed := map[string]bool{
		"schema":                    true,
		"event":                     true,
		"command":                   true,
		"version":                   true,
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
			t.Errorf("extra field %q in body", key)
		}
	}
}

func TestClientSendDisabled(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer srv.Close()

	c := NewClient(Settings{
		Enabled:  false,
		Endpoint: srv.URL,
	}, nil)

	err := c.Send(context.Background(), Event{Schema: "archetipo.analytics/v1", Event: "command_completed"})
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}

	if callCount != 0 {
		t.Errorf("callCount = %d, want 0 (disabled)", callCount)
	}
}

func TestClientSendUnreachable(t *testing.T) {
	c := NewClient(Settings{
		Enabled:  true,
		Endpoint: "http://127.0.0.1:1", // nothing listening here
		Timeout:  100 * time.Millisecond,
	}, nil)

	err := c.Send(context.Background(), Event{Schema: "archetipo.analytics/v1", Event: "command_completed"})
	if err != nil {
		t.Fatalf("Send() returned error on unreachable endpoint: %v", err)
	}
}

func TestClientSendTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	c := NewClient(Settings{
		Enabled:  true,
		Endpoint: srv.URL,
		Timeout:  50 * time.Millisecond,
	}, nil)

	err := c.Send(context.Background(), Event{Schema: "archetipo.analytics/v1", Event: "command_completed"})
	if err != nil {
		t.Fatalf("Send() returned error on timeout: %v", err)
	}
}

func TestClientSendNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(Settings{
		Enabled:  true,
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
	}, nil)

	err := c.Send(context.Background(), Event{Schema: "archetipo.analytics/v1", Event: "command_completed"})
	if err != nil {
		t.Fatalf("Send() returned error on 500: %v", err)
	}
}

func TestClientSendBodyContentType(t *testing.T) {
	// Verify Content-Type is explicitly application/json.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "archetipo-analytics/") {
			t.Errorf("User-Agent = %q, want archetipo-analytics/*", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Settings{
		Enabled:  true,
		Endpoint: srv.URL,
	}, nil)

	_ = c.Send(context.Background(), Event{Schema: "archetipo.analytics/v1", Event: "command_completed"})
}
