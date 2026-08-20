package codex

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
	ProviderID = "codex"

	defaultCommand = "codex"
	defaultTimeout = 3600

	minTimeout = 1
	maxTimeout = 86400
)

// sandboxModes are the sandbox policies `thread/start` accepts, verified
// against codex-cli 0.147.0. They live here, in one place, because the help
// text of the `sandbox` field quotes them: a set described in prose next to a
// set written in code is a pair that drifts.
var sandboxModes = []string{"read-only", "workspace-write", "danger-full-access"}

// defaultSandbox is what planning needs: the agent has to write the plan into
// the workspace, so a read-only session would produce runs that always fail.
const defaultSandbox = "workspace-write"

// settings is the parsed, non-secret provider configuration. Codex
// authenticates by itself, so no credential — and no path to its session
// material — is ever part of this struct.
type settings struct {
	Command         string
	Model           string
	ReasoningEffort string
	Sandbox         string
	Timeout         time.Duration
}

var knownConfigKeys = map[string]struct{}{
	"command":          {},
	"model":            {},
	"reasoning_effort": {},
	"sandbox":          {},
	"timeout_seconds":  {},
}

// ConfigFields declares the non-secret settings this provider accepts, so a
// caller that does not know Codex — the viewer's configuration form — can offer
// them without hard-coding this package's keys. The names are exactly the keys
// parseConfig accepts, and none of them carries a credential: Codex owns its
// own authentication and ARchetipo never touches that material.
func (p *Provider) ConfigFields() []execution.ConfigField {
	return []execution.ConfigField{
		{
			Name:        "command",
			Label:       "Codex command",
			Type:        "text",
			Help:        "Name of the Codex executable to look up on PATH, or an absolute path to it. Defaults to " + defaultCommand + ".",
			Placeholder: defaultCommand,
		},
		{
			Name:        "model",
			Label:       "Model",
			Type:        "text",
			Help:        "Model Codex is asked to use. Left empty, no model flag is passed and Codex picks its own default.",
			Placeholder: "gpt-5-codex",
		},
		{
			Name:  "sandbox",
			Label: "Sandbox",
			Type:  "text",
			// The consequence is spelled out because it is the expensive
			// mistake: planning has to persist a plan, so a read-only session
			// produces runs that always fail.
			Help:        "Sandbox policy of the Codex session, one of " + strings.Join(sandboxModes, ", ") + ". Defaults to " + defaultSandbox + ", which is what lets the agent write the plan into the workspace.",
			Placeholder: defaultSandbox,
		},
		{
			Name:        "timeout_seconds",
			Label:       "Timeout (seconds)",
			Type:        "integer",
			Help:        fmt.Sprintf("How long the local Codex process may run, between %d and %d. Defaults to %d.", minTimeout, maxTimeout, defaultTimeout),
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
// not have Codex installed.
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
	reasoningEffort, err := parseReasoningEffort(raw["reasoning_effort"])
	if err != nil {
		return settings{}, err
	}
	sandbox, err := parseSandbox(raw["sandbox"])
	if err != nil {
		return settings{}, err
	}
	timeoutSeconds, err := parseSeconds(raw, "timeout_seconds", defaultTimeout, minTimeout, maxTimeout)
	if err != nil {
		return settings{}, err
	}
	return settings{
		Command:         command,
		Model:           model,
		ReasoningEffort: reasoningEffort,
		Sandbox:         sandbox,
		Timeout:         time.Duration(timeoutSeconds) * time.Second,
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
	return configErr(unknown[0], "is not a recognized codex provider configuration key")
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

// parseReasoningEffort accepts one of the levels this package offers for the
// reasoning budget. Unlike sandbox it has no default: an absent key means "not
// set", and then no override is sent at all, so Codex applies its own setting.
// A key that is present must carry one of the declared levels — a value outside
// the set would travel to the thread and be refused there, with a diagnostic
// that points at the protocol instead of at the option.
func parseReasoningEffort(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr("reasoning_effort", "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", configErr("reasoning_effort", "must not be empty")
	}
	for _, effort := range reasoningEfforts {
		if text == effort {
			return text, nil
		}
	}
	return "", configErr("reasoning_effort", "must be one of "+strings.Join(reasoningEfforts, ", "))
}

// parseSandbox accepts one of the policies the Codex session understands. A key
// that is present must carry one of them: a value outside the set would be
// passed straight to the process and refused there, with a diagnostic that
// points at the protocol instead of at the configuration field.
func parseSandbox(value any) (string, error) {
	if value == nil {
		return defaultSandbox, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr("sandbox", "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", configErr("sandbox", "must not be empty")
	}
	for _, mode := range sandboxModes {
		if text == mode {
			return text, nil
		}
	}
	return "", configErr("sandbox", "must be one of "+strings.Join(sandboxModes, ", "))
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
