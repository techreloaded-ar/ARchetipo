package arcipelago

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func validConfig() map[string]any {
	return map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1"}
}

// TestConfigFieldsMatchAcceptedKeys is the guard against the most insidious
// defect of a declared form: a field the validator does not accept, or an
// accepted key nobody can fill in. Both directions are checked.
func TestConfigFieldsMatchAcceptedKeys(t *testing.T) {
	fields := New(Options{}).ConfigFields()
	declared := make(map[string]execution.ConfigField, len(fields))
	for _, field := range fields {
		if _, duplicate := declared[field.Name]; duplicate {
			t.Fatalf("field %q declared twice", field.Name)
		}
		if strings.TrimSpace(field.Label) == "" {
			t.Fatalf("field %q has no label", field.Name)
		}
		if field.Type != "text" && field.Type != "integer" {
			t.Fatalf("field %q has unsupported type %q", field.Name, field.Type)
		}
		declared[field.Name] = field
	}
	for name := range knownConfigKeys {
		if _, ok := declared[name]; !ok {
			t.Fatalf("accepted key %q is not declared as a configurable field", name)
		}
	}
	for name := range declared {
		if _, ok := knownConfigKeys[name]; !ok {
			t.Fatalf("declared field %q is not an accepted configuration key", name)
		}
	}
	// The credential never becomes a configuration field: the only key that may
	// mention a token is the *name* of the environment variable holding it.
	for name := range declared {
		if strings.Contains(name, "token") && name != "token_env" {
			t.Fatalf("field %q looks like a credential; secrets stay in the environment", name)
		}
	}
	required := map[string]any{}
	for name, field := range declared {
		if field.Required {
			required[name] = "https://hub.test"
		}
	}
	if len(required) == 0 {
		t.Fatal("no required field declared: a provider that needs a hub cannot be configured by defaults alone")
	}
	required["workspace_id"] = "ws-1"
	if err := New(Options{}).ValidateConfig(context.Background(), required); err != nil {
		t.Fatalf("a configuration filling only the required fields must validate: %v", err)
	}
}

func TestParseConfigAppliesDefaults(t *testing.T) {
	got, err := parseConfig(map[string]any{"base_url": "https://hub.test/", "workspace_id": "ws-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != "https://hub.test" {
		t.Fatalf("base_url was not normalized: %q", got.BaseURL)
	}
	if got.WorkspaceID != "ws-1" || got.TokenEnv != "ARCIPELAGO_TOKEN" {
		t.Fatalf("unexpected identity defaults: %#v", got)
	}
	if got.PollInterval != 5*time.Second || got.Timeout != 3600*time.Second {
		t.Fatalf("unexpected timing defaults: poll=%s timeout=%s", got.PollInterval, got.Timeout)
	}
}

func TestParseConfigRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   map[string]any
		field string
	}{
		{"base_url missing", map[string]any{"workspace_id": "ws-1"}, "base_url"},
		{"base_url empty", map[string]any{"base_url": "  ", "workspace_id": "ws-1"}, "base_url"},
		{"base_url not a string", map[string]any{"base_url": 42, "workspace_id": "ws-1"}, "base_url"},
		{"base_url not absolute", map[string]any{"base_url": "hub.test", "workspace_id": "ws-1"}, "base_url"},
		{"base_url wrong scheme", map[string]any{"base_url": "ftp://hub.test", "workspace_id": "ws-1"}, "base_url"},
		{"base_url without host", map[string]any{"base_url": "https:///path", "workspace_id": "ws-1"}, "base_url"},
		{"workspace_id missing", map[string]any{"base_url": "https://hub.test"}, "workspace_id"},
		{"workspace_id empty", map[string]any{"base_url": "https://hub.test", "workspace_id": ""}, "workspace_id"},
		{"workspace_id not a string", map[string]any{"base_url": "https://hub.test", "workspace_id": true}, "workspace_id"},
		{"workspace_id too long", map[string]any{"base_url": "https://hub.test", "workspace_id": strings.Repeat("w", 65)}, "workspace_id"},
		{"token_env empty", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "token_env": ""}, "token_env"},
		{"token_env lowercase", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "token_env": "Arcipelago_token"}, "token_env"},
		{"token_env with dash", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "token_env": "ARCIPELAGO-TOKEN"}, "token_env"},
		{"token_env starting with a digit", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "token_env": "1TOKEN"}, "token_env"},
		{"token_env not a string", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "token_env": 7}, "token_env"},
		{"poll_interval_seconds zero", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "poll_interval_seconds": 0}, "poll_interval_seconds"},
		{"poll_interval_seconds negative", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "poll_interval_seconds": -1}, "poll_interval_seconds"},
		{"poll_interval_seconds too large", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "poll_interval_seconds": 301}, "poll_interval_seconds"},
		{"poll_interval_seconds not numeric", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "poll_interval_seconds": "5"}, "poll_interval_seconds"},
		{"poll_interval_seconds fractional", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "poll_interval_seconds": 10.5}, "poll_interval_seconds"},
		{"timeout_seconds zero", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "timeout_seconds": 0}, "timeout_seconds"},
		{"timeout_seconds too large", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "timeout_seconds": 86401}, "timeout_seconds"},
		{"timeout_seconds below poll interval", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "poll_interval_seconds": 60, "timeout_seconds": 30}, "timeout_seconds"},
		{"unknown key", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "base_urls": "https://typo.test"}, "base_urls"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(tc.raw)
			var configErr *execution.ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("expected a configuration error, got %v", err)
			}
			if configErr.Field != tc.field {
				t.Fatalf("field = %q, want %q (%v)", configErr.Field, tc.field, err)
			}
		})
	}
}

// A provider config reaches the CLI both from YAML, which decodes integers as
// int, and from JSON, which decodes every number as float64.
func TestParseConfigAcceptsNumericFormsFromYAMLAndJSON(t *testing.T) {
	for _, value := range []any{10, int64(10), float64(10)} {
		raw := validConfig()
		raw["poll_interval_seconds"] = value
		got, err := parseConfig(raw)
		if err != nil {
			t.Fatalf("%T(%v): %v", value, value, err)
		}
		if got.PollInterval != 10*time.Second {
			t.Fatalf("%T(%v): poll interval = %s", value, value, got.PollInterval)
		}
	}
	raw := validConfig()
	raw["poll_interval_seconds"] = 10.5
	_, err := parseConfig(raw)
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "poll_interval_seconds" {
		t.Fatalf("fractional seconds accepted: %v", err)
	}
}

// Reading the environment during validation would make
// `execution provider set-default` unrunnable on a machine without the secret.
func TestValidateConfigDoesNotReadEnvironment(t *testing.T) {
	provider := New(Options{Getenv: func(string) string {
		t.Fatal("ValidateConfig must not read the environment")
		return ""
	}})
	if err := provider.ValidateConfig(context.Background(), validConfig()); err != nil {
		t.Fatal(err)
	}
}

func TestProviderIdentityAndCapabilities(t *testing.T) {
	provider := New(Options{})
	if provider.ID() != ProviderID || ProviderID != "arcipelago" {
		t.Fatalf("provider id = %q", provider.ID())
	}
	capabilities, err := provider.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []execution.Capability{execution.CapabilitySpecPlan, execution.CapabilitySpecImplement}
	if !slices.Equal(capabilities, want) {
		t.Fatalf("capabilities = %v, want %v", capabilities, want)
	}
}
