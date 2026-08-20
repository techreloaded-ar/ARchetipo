package claude

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
	if got.Command != "claude" {
		t.Fatalf("command = %q, want %q", got.Command, "claude")
	}
	if got.Model != "" {
		t.Fatalf("model = %q, want empty so that no model flag is emitted", got.Model)
	}
	if got.PermissionMode != defaultPermissionMode {
		t.Fatalf("permission_mode = %q, want %q", got.PermissionMode, defaultPermissionMode)
	}
	if got.Timeout != 3600*time.Second {
		t.Fatalf("timeout = %s, want 3600s", got.Timeout)
	}
}

func TestConfigAppliesFullOverride(t *testing.T) {
	got, err := parseConfig(map[string]any{
		"command":         "/opt/homebrew/bin/claude",
		"model":           "opus",
		"permission_mode": "plan",
		"timeout_seconds": 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "/opt/homebrew/bin/claude" {
		t.Fatalf("command = %q", got.Command)
	}
	if got.Model != "opus" {
		t.Fatalf("model = %q", got.Model)
	}
	if got.PermissionMode != "plan" {
		t.Fatalf("permission_mode = %q, want %q", got.PermissionMode, "plan")
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
		{"command relative path", map[string]any{"command": "./bin/claude"}, "command"},
		{"model not a string", map[string]any{"model": true}, "model"},
		{"permission_mode not a string", map[string]any{"permission_mode": []string{"auto"}}, "permission_mode"},
		{"permission_mode empty", map[string]any{"permission_mode": ""}, "permission_mode"},
		{"permission_mode blank", map[string]any{"permission_mode": "   "}, "permission_mode"},
		// A mode Claude does not accept must be refused here, where the field
		// can be named, rather than by the process, which would blame the CLI.
		{"permission_mode outside the set", map[string]any{"permission_mode": "yolo"}, "permission_mode"},
		// print_args was removed with the streaming session: the session flags
		// are what the dialogue rests on, so they are no longer negotiable.
		{"the removed print_args key", map[string]any{"print_args": "--permission-mode plan"}, "print_args"},
		{"timeout_seconds not an integer", map[string]any{"timeout_seconds": "3600"}, "timeout_seconds"},
		{"timeout_seconds fractional", map[string]any{"timeout_seconds": 10.5}, "timeout_seconds"},
		{"timeout_seconds below range", map[string]any{"timeout_seconds": 0}, "timeout_seconds"},
		{"timeout_seconds above range", map[string]any{"timeout_seconds": 86401}, "timeout_seconds"},
		{"unknown key", map[string]any{"comand": "claude"}, "comand"},
		// A key that belongs to the sibling local provider is a plausible
		// mistake, and it must be reported as unknown rather than silently
		// ignored.
		{"a codex key", map[string]any{"exec_args": "-s workspace-write"}, "exec_args"},
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
// a place to put a credential: Claude authenticates by itself and ARchetipo
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
			t.Fatalf("field %q is required, but every claude setting has a default", field.Name)
		}
		for _, forbidden := range []string{"token", "secret", "password", "api_key", "credential", "auth", "session"} {
			if strings.Contains(strings.ToLower(field.Name), forbidden) {
				t.Fatalf("field %q looks like a credential; claude owns its own authentication", field.Name)
			}
		}
		if _, ok := knownConfigKeys[field.Name]; !ok {
			t.Fatalf("declared field %q is not an accepted configuration key", field.Name)
		}
		names = append(names, field.Name)
	}
	sort.Strings(names)
	want := []string{"command", "model", "permission_mode", "timeout_seconds"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("declared fields = %v, want %v", names, want)
	}
	// Every accepted key must be declared somewhere a person can see it —
	// either as a configuration field of the provider, or as an option of a
	// model. A model option lives in the same flat namespace as the fields but
	// is deliberately not one of them: it belongs to the model that declares
	// it, and declaring it in both places would draw it twice in the form.
	for name := range knownConfigKeys {
		found := false
		for _, declared := range names {
			if declared == name {
				found = true
				break
			}
		}
		for _, option := range declaredModelOptionNames() {
			if option == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("accepted key %q is declared neither as a configurable field nor as a model option", name)
		}
	}
	// The two namespaces are flat and shared, so a collision would make one
	// declaration shadow the other.
	for _, option := range declaredModelOptionNames() {
		for _, declared := range names {
			if option == declared {
				t.Fatalf("model option %q collides with the configuration field of the same name", option)
			}
		}
	}
}

// declaredModelOptionNames is every option name the catalog declares, without
// duplicates.
func declaredModelOptionNames() []string {
	seen := map[string]struct{}{}
	names := []string{}
	for _, model := range models {
		for _, option := range model.Options {
			if _, ok := seen[option.Name]; ok {
				continue
			}
			seen[option.Name] = struct{}{}
			names = append(names, option.Name)
		}
	}
	sort.Strings(names)
	return names
}

// The permission_mode help quotes the modes the parser actually accepts and the
// default buildArgs actually emits. Prose and code drifted apart once in the
// sibling provider, so the pair is pinned by a test rather than by discipline.
func TestPermissionModeHelpQuotesTheRealModes(t *testing.T) {
	for _, field := range (&Provider{}).ConfigFields() {
		if field.Name != "permission_mode" {
			continue
		}
		for _, mode := range permissionModes {
			if !strings.Contains(field.Help, mode) {
				t.Fatalf("the permission_mode help does not quote the accepted mode %q: %s", mode, field.Help)
			}
		}
		if !strings.Contains(field.Help, defaultPermissionMode) {
			t.Fatalf("the permission_mode help does not quote the default %q: %s", defaultPermissionMode, field.Help)
		}
		return
	}
	t.Fatal("permission_mode is not a declared field")
}

// --- effort ----------------------------------------------------------------

// An absent effort is "not set", not a default: no flag is passed at all and
// Claude applies its own level.
func TestEffortIsUnsetWhenAbsent(t *testing.T) {
	cfg, err := parseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("an empty configuration failed: %v", err)
	}
	if cfg.Effort != "" {
		t.Fatalf("effort = %q, want the empty string when the key is absent", cfg.Effort)
	}
}

func TestEffortAcceptsEveryDeclaredLevel(t *testing.T) {
	for _, level := range effortLevels {
		cfg, err := parseConfig(map[string]any{"effort": level})
		if err != nil {
			t.Fatalf("level %q was rejected: %v", level, err)
		}
		if cfg.Effort != level {
			t.Fatalf("effort = %q, want %q", cfg.Effort, level)
		}
	}
}

// The rejection has to name the option, because that name is what the panel
// highlights and what the CLI renders as the offending field.
func TestEffortRejectionNamesTheOption(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"a level outside the declared set", "turbo"},
		{"an empty string", ""},
		{"blanks only", "   "},
		{"a value that is not a string", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfig(map[string]any{"effort": tc.value})
			if err == nil {
				t.Fatalf("value %#v was accepted", tc.value)
			}
			var configErr *execution.ConfigurationError
			if !errors.As(err, &configErr) {
				t.Fatalf("error is %T, want *execution.ConfigurationError: %v", err, err)
			}
			if configErr.Field != "effort" {
				t.Fatalf("the rejection names field %q, want %q", configErr.Field, "effort")
			}
		})
	}
}

// The help of the option quotes nothing, but the message of the rejection has
// to list the levels a person can pick, or the error leaves them guessing.
func TestEffortRejectionListsTheAcceptedLevels(t *testing.T) {
	_, err := parseConfig(map[string]any{"effort": "turbo"})
	if err == nil {
		t.Fatal("an unknown level was accepted")
	}
	for _, level := range effortLevels {
		if !strings.Contains(err.Error(), level) {
			t.Fatalf("the rejection does not quote the accepted level %q: %s", level, err.Error())
		}
	}
}
