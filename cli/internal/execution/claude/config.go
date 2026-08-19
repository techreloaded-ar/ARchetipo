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

// defaultPrintArgs are the intermediate flags buildArgs emits when print_args
// is not configured. They live here, in one place, because the print_args help
// text quotes them: a default described in prose next to a default written in
// code is a pair that drifts.
//
// Verified against Claude Code 2.1.234: `--no-session-persistence` is accepted
// only together with `--print`, which is exactly the mode this provider runs
// in, and keeps a managed planning run from leaving a resumable session behind.
// `--permission-mode auto` hands the local policy decision to Claude itself,
// which is what lets the planning skill persist the plan without a prompt no
// one is there to answer.
var defaultPrintArgs = []string{"--no-session-persistence", "--permission-mode", "auto"}

// settings is the parsed, non-secret provider configuration. Claude
// authenticates by itself, so no credential — and no path to its session
// material — is ever part of this struct.
type settings struct {
	Command   string
	Model     string
	PrintArgs []string
	Timeout   time.Duration
}

var knownConfigKeys = map[string]struct{}{
	"command":         {},
	"model":           {},
	"print_args":      {},
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
			Name:  "print_args",
			Label: "Print arguments",
			Type:  "text",
			// The wording says "replace" because buildArgs replaces: whatever is
			// set here stands in for the default flags rather than joining them.
			// Reading it as "append" is the expensive mistake — dropping the
			// permission mode leaves Claude waiting for an approval nobody can
			// give, so the run burns its whole timeout — which is why the
			// default is spelled out here.
			Help:        "Space-separated arguments that replace the default Claude print-mode flags (" + strings.Join(defaultPrintArgs, " ") + "). Left empty, those defaults are used. Use it to apply a different local permission policy.",
			Placeholder: strings.Join(defaultPrintArgs, " "),
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
	printArgs, err := parsePrintArgs(raw["print_args"])
	if err != nil {
		return settings{}, err
	}
	timeoutSeconds, err := parseSeconds(raw, "timeout_seconds", defaultTimeout, minTimeout, maxTimeout)
	if err != nil {
		return settings{}, err
	}
	return settings{
		Command:   command,
		Model:     model,
		PrintArgs: printArgs,
		Timeout:   time.Duration(timeoutSeconds) * time.Second,
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

// parsePrintArgs takes the arguments as a single space-separated string, the
// shape a configuration form can offer, and splits it into the slice the
// invocation needs. A key that is present must carry something: an empty string
// is a mistake, while omitting the key is the documented default.
func parsePrintArgs(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, configErr("print_args", "must be a string")
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, configErr("print_args", "must not be empty")
	}
	return fields, nil
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
