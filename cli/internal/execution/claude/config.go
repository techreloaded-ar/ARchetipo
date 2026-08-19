package claude

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const (
	// ProviderID is the identifier this provider is registered and configured
	// under.
	ProviderID = "claude"

	defaultCommand = "claude"
	defaultTimeout = 3600

	minTimeout = 1
	maxTimeout = 86400
)

// permissionModes are the local permission policies Claude Code accepts on
// `--permission-mode`. The list is closed on purpose: a value outside it would
// be handed straight to the process and refused there, with a diagnostic that
// points at the CLI instead of at the configuration field.
//
// Verified against Claude Code 2.1.235.
var permissionModes = []string{"acceptEdits", "auto", "bypassPermissions", "manual", "dontAsk", "plan"}

// defaultPermissionMode hands the local policy decision to Claude itself, which
// is what lets the planning skill persist the plan without a prompt no one is
// there to answer. It is the behaviour this provider has always had.
const defaultPermissionMode = "auto"

// settings is the parsed, non-secret provider configuration. Claude
// authenticates by itself, so no credential — and no path to its session
// material — is ever part of this struct.
type settings struct {
	Command        string
	Model          string
	PermissionMode string
	Timeout        time.Duration
}

var knownConfigKeys = map[string]struct{}{
	"command":         {},
	"model":           {},
	"permission_mode": {},
	"timeout_seconds": {},
}

// ConfigFields declares the non-secret settings this provider accepts, so a
// caller that does not know Claude — the viewer's configuration form — can
// offer them without hard-coding this package's keys. The names are exactly the
// keys parseConfig accepts, and none of them carries a credential: Claude owns
// its own authentication and ARchetipo never touches that material.
func (p *Provider) ConfigFields() []execution.ConfigField {
	return []execution.ConfigField{
		{
			Name:        "command",
			Label:       "Claude command",
			Type:        "text",
			Help:        "Name of the Claude Code executable to look up on PATH, or an absolute path to it. Defaults to " + defaultCommand + ".",
			Placeholder: defaultCommand,
		},
		{
			Name:        "model",
			Label:       "Model",
			Type:        "text",
			Help:        "Model Claude is asked to use. Left empty, no model flag is passed and Claude picks its own default.",
			Placeholder: "opus",
		},
		{
			Name:  "permission_mode",
			Label: "Permission mode",
			// The session flags themselves are deliberately not configurable:
			// the dialogue rests on the streaming protocol, and an argument
			// that can only break it is not a choice worth offering. The local
			// permission policy is the one decision that stays meaningful.
			Type:        "text",
			Help:        "Local permission policy Claude runs the session with, one of " + strings.Join(permissionModes, ", ") + ". Defaults to " + defaultPermissionMode + ".",
			Placeholder: defaultPermissionMode,
		},
		{
			Name:        "timeout_seconds",
			Label:       "Timeout (seconds)",
			Type:        "integer",
			Help:        fmt.Sprintf("How long the local Claude process may run, between %d and %d. Defaults to %d.", minTimeout, maxTimeout, defaultTimeout),
			Placeholder: fmt.Sprintf("%d", defaultTimeout),
		},
	}
}

var _ execution.ConfigDescriber = (*Provider)(nil)

func configErr(field, reason string) error {
	return &execution.ConfigurationError{Field: field, Reason: reason}
}

// parseConfig validates the shape of the provider configuration and applies the
// documented defaults. Every rejection names the exact offending key, so the
// CLI can render it as execution.default_provider.config.<field>.
//
// It reads no environment variable and never looks the command up on PATH:
// `execution provider set-default` must stay runnable on a machine that does
// not have Claude Code installed.
func parseConfig(raw map[string]any) (settings, error) {
	if err := rejectUnknownKeys(raw); err != nil {
		return settings{}, err
	}
	command, err := parseCommand(raw["command"])
	if err != nil {
		return settings{}, err
	}
	model, err := parseModel(raw["model"])
	if err != nil {
		return settings{}, err
	}
	permissionMode, err := parsePermissionMode(raw["permission_mode"])
	if err != nil {
		return settings{}, err
	}
	timeoutSeconds, err := parseSeconds(raw, "timeout_seconds", defaultTimeout, minTimeout, maxTimeout)
	if err != nil {
		return settings{}, err
	}
	return settings{
		Command:        command,
		Model:          model,
		PermissionMode: permissionMode,
		Timeout:        time.Duration(timeoutSeconds) * time.Second,
	}, nil
}

// rejectUnknownKeys catches typos before any other check, and sorts the keys so
// the reported field is deterministic when more than one is unknown.
func rejectUnknownKeys(raw map[string]any) error {
	unknown := make([]string, 0, len(raw))
	for key := range raw {
		if _, ok := knownConfigKeys[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return configErr(unknown[0], "is not a recognized claude provider configuration key")
}

// parseCommand accepts a bare executable name to resolve on PATH, or an
// absolute path to the binary. A relative path with separators is refused
// because it would silently depend on the working directory.
func parseCommand(value any) (string, error) {
	if value == nil {
		return defaultCommand, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr("command", "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", configErr("command", "must not be empty")
	}
	if strings.ContainsRune(text, filepath.Separator) || strings.ContainsRune(text, '/') {
		if !filepath.IsAbs(text) {
			return "", configErr("command", "must be an executable name or an absolute path")
		}
	}
	return text, nil
}

func parseModel(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr("model", "must be a string")
	}
	return strings.TrimSpace(text), nil
}

// parsePermissionMode accepts one of the policies Claude Code understands. A
// key that is present must carry one of them: a value outside the set would be
// passed straight to the process and refused there, with a diagnostic that
// points at the CLI instead of at the configuration field.
func parsePermissionMode(value any) (string, error) {
	if value == nil {
		return defaultPermissionMode, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr("permission_mode", "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", configErr("permission_mode", "must not be empty")
	}
	for _, mode := range permissionModes {
		if text == mode {
			return text, nil
		}
	}
	return "", configErr("permission_mode", "must be one of "+strings.Join(permissionModes, ", "))
}

// parseSeconds accepts the numeric forms a provider config can arrive in: YAML
// decodes integers as int, JSON decodes every number as float64, and an int64
// can reach here through a programmatic caller.
func parseSeconds(raw map[string]any, field string, fallback, minimum, maximum int) (int, error) {
	value, present := raw[field]
	if !present || value == nil {
		return fallback, nil
	}
	var seconds int
	switch typed := value.(type) {
	case int:
		seconds = typed
	case int64:
		seconds = int(typed)
	case float64:
		if typed != float64(int(typed)) {
			return 0, configErr(field, "must be a whole number of seconds")
		}
		seconds = int(typed)
	default:
		return 0, configErr(field, "must be an integer number of seconds")
	}
	if seconds < minimum || seconds > maximum {
		return 0, configErr(field, fmt.Sprintf("must be between %d and %d", minimum, maximum))
	}
	return seconds, nil
}
