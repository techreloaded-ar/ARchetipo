package arcipelago

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const (
	defaultTokenEnv     = "ARCIPELAGO_TOKEN"
	defaultPollInterval = 5
	defaultTimeout      = 3600

	maxWorkspaceIDLength = 64
	minPollInterval      = 1
	maxPollInterval      = 300
	minTimeout           = 1
	maxTimeout           = 86400
)

// settings is the parsed, non-secret provider configuration. The credential is
// never part of it: only the name of the environment variable that holds it.
type settings struct {
	BaseURL      string
	WorkspaceID  string
	TokenEnv     string
	PollInterval time.Duration
	Timeout      time.Duration
}

var knownConfigKeys = map[string]struct{}{
	"base_url":              {},
	"workspace_id":          {},
	"token_env":             {},
	"poll_interval_seconds": {},
	"timeout_seconds":       {},
}

// ConfigFields declares the non-secret settings this provider accepts, so a
// caller that does not know ARcipelago — the viewer's configuration form — can
// offer them without hard-coding this package's keys. The names are exactly the
// keys parseConfig accepts, and none of them carries the credential: the token
// stays in the environment and only the *name* of its variable is configured.
func (p *Provider) ConfigFields() []execution.ConfigField {
	return []execution.ConfigField{
		{
			Name:        "base_url",
			Label:       "Hub base URL",
			Type:        "text",
			Help:        "Absolute http or https URL of the ARcipelago hub.",
			Placeholder: "https://arcipelago.example",
			Required:    true,
		},
		{
			Name:        "workspace_id",
			Label:       "Workspace id",
			Type:        "text",
			Help:        fmt.Sprintf("Identifier of the ARcipelago workspace, at most %d characters.", maxWorkspaceIDLength),
			Placeholder: "my-workspace",
			Required:    true,
		},
		{
			Name:        "token_env",
			Label:       "Token environment variable",
			Type:        "text",
			Help:        "Name of the environment variable holding the ARcipelago token. The token itself is never stored in the configuration. Defaults to " + defaultTokenEnv + ".",
			Placeholder: defaultTokenEnv,
		},
		{
			Name:        "poll_interval_seconds",
			Label:       "Poll interval (seconds)",
			Type:        "integer",
			Help:        fmt.Sprintf("How often the run is polled, between %d and %d. Defaults to %d.", minPollInterval, maxPollInterval, defaultPollInterval),
			Placeholder: fmt.Sprintf("%d", defaultPollInterval),
		},
		{
			Name:        "timeout_seconds",
			Label:       "Timeout (seconds)",
			Type:        "integer",
			Help:        fmt.Sprintf("How long the run may take, between %d and %d and never below the poll interval. Defaults to %d.", minTimeout, maxTimeout, defaultTimeout),
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
func parseConfig(raw map[string]any) (settings, error) {
	if err := rejectUnknownKeys(raw); err != nil {
		return settings{}, err
	}
	baseURL, err := parseBaseURL(raw["base_url"])
	if err != nil {
		return settings{}, err
	}
	workspaceID, err := parseWorkspaceID(raw["workspace_id"])
	if err != nil {
		return settings{}, err
	}
	tokenEnv, err := parseTokenEnv(raw["token_env"])
	if err != nil {
		return settings{}, err
	}
	pollSeconds, err := parseSeconds(raw, "poll_interval_seconds", defaultPollInterval, minPollInterval, maxPollInterval)
	if err != nil {
		return settings{}, err
	}
	timeoutSeconds, err := parseSeconds(raw, "timeout_seconds", defaultTimeout, minTimeout, maxTimeout)
	if err != nil {
		return settings{}, err
	}
	if timeoutSeconds < pollSeconds {
		return settings{}, configErr("timeout_seconds", fmt.Sprintf("must be greater than or equal to poll_interval_seconds (%d)", pollSeconds))
	}
	return settings{
		BaseURL:      baseURL,
		WorkspaceID:  workspaceID,
		TokenEnv:     tokenEnv,
		PollInterval: time.Duration(pollSeconds) * time.Second,
		Timeout:      time.Duration(timeoutSeconds) * time.Second,
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
	return configErr(unknown[0], "is not a recognized arcipelago provider configuration key")
}

func requiredString(value any, field string) (string, error) {
	if value == nil {
		return "", configErr(field, "is required")
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr(field, "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", configErr(field, "is required")
	}
	return text, nil
}

func parseBaseURL(value any) (string, error) {
	text, err := requiredString(value, "base_url")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return "", &execution.ConfigurationError{Field: "base_url", Reason: "must be an absolute http or https URL", Err: err}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", configErr("base_url", "must be an absolute http or https URL")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", configErr("base_url", "must include a host")
	}
	return strings.TrimRight(text, "/"), nil
}

func parseWorkspaceID(value any) (string, error) {
	text, err := requiredString(value, "workspace_id")
	if err != nil {
		return "", err
	}
	if len(text) > maxWorkspaceIDLength {
		return "", configErr("workspace_id", fmt.Sprintf("must be at most %d characters", maxWorkspaceIDLength))
	}
	return text, nil
}

func parseTokenEnv(value any) (string, error) {
	if value == nil {
		return defaultTokenEnv, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", configErr("token_env", "must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", configErr("token_env", "must not be empty")
	}
	if text[0] >= '0' && text[0] <= '9' {
		return "", configErr("token_env", "must not start with a digit")
	}
	for _, r := range text {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return "", configErr("token_env", "must contain only upper-case letters, digits and underscores")
	}
	return text, nil
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
