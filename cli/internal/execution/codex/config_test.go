package codex

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func TestConfigAppliesDocumentedDefaults(t *testing.T) {
	got, err := parseConfig(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "codex" {
		t.Fatalf("command = %q, want %q", got.Command, "codex")
	}
	if got.Model != "" {
		t.Fatalf("model = %q, want empty so that no model flag is emitted", got.Model)
	}
	if len(got.ExecArgs) != 0 {
		t.Fatalf("exec_args = %#v, want empty", got.ExecArgs)
	}
	if got.Timeout != 3600*time.Second {
		t.Fatalf("timeout = %s, want 3600s", got.Timeout)
	}
}

func TestConfigAppliesFullOverride(t *testing.T) {
	got, err := parseConfig(map[string]any{
		"command":         "/opt/homebrew/bin/codex",
		"model":           "gpt-5-codex",
		"exec_args":       "--full-auto --skip-git-repo-check",
		"timeout_seconds": 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "/opt/homebrew/bin/codex" {
		t.Fatalf("command = %q", got.Command)
	}
	if got.Model != "gpt-5-codex" {
		t.Fatalf("model = %q", got.Model)
	}
	if want := []string{"--full-auto", "--skip-git-repo-check"}; !reflect.DeepEqual(got.ExecArgs, want) {
		t.Fatalf("exec_args = %#v, want %#v", got.ExecArgs, want)
	}
	if got.Timeout != 120*time.Second {
		t.Fatalf("timeout = %s, want 120s", got.Timeout)
	}
}

func TestConfigRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   map[string]any
		field string
	}{
		{"command not a string", map[string]any{"command": 42}, "command"},
		{"command empty", map[string]any{"command": "   "}, "command"},
		{"command relative path", map[string]any{"command": "./bin/codex"}, "command"},
		{"model not a string", map[string]any{"model": true}, "model"},
		{"exec_args not a string", map[string]any{"exec_args": []string{"--full-auto"}}, "exec_args"},
		{"exec_args empty", map[string]any{"exec_args": ""}, "exec_args"},
		{"exec_args blank", map[string]any{"exec_args": "   "}, "exec_args"},
		{"timeout_seconds not an integer", map[string]any{"timeout_seconds": "3600"}, "timeout_seconds"},
		{"timeout_seconds fractional", map[string]any{"timeout_seconds": 10.5}, "timeout_seconds"},
		{"timeout_seconds below range", map[string]any{"timeout_seconds": 0}, "timeout_seconds"},
		{"timeout_seconds above range", map[string]any{"timeout_seconds": 86401}, "timeout_seconds"},
		{"unknown key", map[string]any{"comand": "codex"}, "comand"},
		// With several unknown keys the reported field must be stable across
		// runs, or the CLI would point at a different typo every time.
		{"several unknown keys", map[string]any{"zulu": 1, "alpha": 1, "mike": 1}, "alpha"},
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
func TestConfigAcceptsNumericFormsFromYAMLAndJSON(t *testing.T) {
	for _, value := range []any{60, int64(60), float64(60)} {
		got, err := parseConfig(map[string]any{"timeout_seconds": value})
		if err != nil {
			t.Fatalf("%T(%v): %v", value, value, err)
		}
		if got.Timeout != 60*time.Second {
			t.Fatalf("%T(%v): timeout = %s", value, value, got.Timeout)
		}
	}
	_, err := parseConfig(map[string]any{"timeout_seconds": 60.5})
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "timeout_seconds" {
		t.Fatalf("fractional seconds accepted: %v", err)
	}
}

// The declared form and the validator must agree, and none of the fields may be
// a place to put a credential: Codex authenticates by itself and ARchetipo
// never stores authentication material.
func TestConfigFieldsDeclareNoSecretAndMatchAcceptedKeys(t *testing.T) {
	fields := (&Provider{}).ConfigFields()
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Label) == "" {
			t.Fatalf("field %q has no label", field.Name)
		}
		if strings.TrimSpace(field.Help) == "" {
			t.Fatalf("field %q has no help text", field.Name)
		}
		if field.Type != "text" && field.Type != "integer" {
			t.Fatalf("field %q has unsupported type %q", field.Name, field.Type)
		}
		if field.Required {
			t.Fatalf("field %q is required, but every codex setting has a default", field.Name)
		}
		for _, forbidden := range []string{"token", "secret", "password", "api_key", "auth", "session"} {
			if strings.Contains(strings.ToLower(field.Name), forbidden) {
				t.Fatalf("field %q looks like a credential; codex owns its own authentication", field.Name)
			}
		}
		if _, ok := knownConfigKeys[field.Name]; !ok {
			t.Fatalf("declared field %q is not an accepted configuration key", field.Name)
		}
		names = append(names, field.Name)
	}
	sort.Strings(names)
	want := []string{"command", "exec_args", "model", "timeout_seconds"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("declared fields = %v, want %v", names, want)
	}
	for name := range knownConfigKeys {
		found := false
		for _, declared := range names {
			if declared == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("accepted key %q is not declared as a configurable field", name)
		}
	}
}
