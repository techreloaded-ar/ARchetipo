package analytics

import (
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

func TestSettingsDefaults(t *testing.T) {
	var s Settings
	s.ApplyDefaults()

	if s.Enabled != false {
		t.Errorf("Enabled default = %v, want false", s.Enabled)
	}
	if s.Timeout != DefaultTimeout {
		t.Errorf("Timeout default = %v, want %v", s.Timeout, DefaultTimeout)
	}
	if got, want := DefaultTimeout, 2*time.Second; got != want {
		t.Errorf("DefaultTimeout constant = %v, want %v", got, want)
	}
	if s.Endpoint != DefaultEndpoint {
		t.Errorf("Endpoint default = %q, want %q", s.Endpoint, DefaultEndpoint)
	}
	if !strings.Contains(s.UserAgent, version.Version) {
		t.Errorf("UserAgent default = %q, want to contain version %q", s.UserAgent, version.Version)
	}
	if !strings.HasPrefix(s.UserAgent, "archetipo-analytics/") {
		t.Errorf("UserAgent default = %q, want prefix 'archetipo-analytics/'", s.UserAgent)
	}
}

func TestNewClientCustomSettings(t *testing.T) {
	s := Settings{
		Enabled:   true,
		Endpoint:  "https://example.com/v1/events",
		Timeout:   5 * time.Second,
		UserAgent: "custom/1.0",
	}
	c := NewClient(s, nil)

	if !c.settings.Enabled {
		t.Error("Enabled = false, want true")
	}
	if c.settings.Endpoint != "https://example.com/v1/events" {
		t.Errorf("Endpoint = %q, want 'https://example.com/v1/events'", c.settings.Endpoint)
	}
	if c.settings.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.settings.Timeout)
	}
	if c.settings.UserAgent != "custom/1.0" {
		t.Errorf("UserAgent = %q, want 'custom/1.0'", c.settings.UserAgent)
	}
}

func TestNewClientEmptySettings(t *testing.T) {
	c := NewClient(Settings{}, nil)

	if c.settings.Enabled != DefaultEnabled {
		t.Errorf("Enabled = %v, want %v", c.settings.Enabled, DefaultEnabled)
	}
	if c.settings.Endpoint != DefaultEndpoint {
		t.Errorf("Endpoint = %q, want %q", c.settings.Endpoint, DefaultEndpoint)
	}
	if c.settings.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.settings.Timeout, DefaultTimeout)
	}
	expectedUA := DefaultUserAgent()
	if c.settings.UserAgent != expectedUA {
		t.Errorf("UserAgent = %q, want %q", c.settings.UserAgent, expectedUA)
	}
}
