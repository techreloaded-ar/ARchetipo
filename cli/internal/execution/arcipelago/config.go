package arcipelago

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/providerconfig"
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

var knownConfigKeys = map[string]any{
	"base_url":              true,
	"workspace_id":          true,
	"token_env":             true,
	"poll_interval_seconds": true,
	"timeout_seconds":       true,
}

// parseConnection validates the part of the configuration that says *which hub
// and with what credential*, leaving out the part that says *which workspace*.
//
// It exists for the one call that legitimately has no destination yet: asking
// the hub which workspaces this credential may use, which is how a setup finds
// the value of workspace_id in the first place. Requiring it there would make
// the question unaskable until its own answer was already known.
//
// parseConfig is written on top of this rather than beside it, so base_url and
// token_env cannot end up validated two ways; and ValidateConfig still goes
// through parseConfig, so `execution provider set-default` is no more
// permissive than it was.
func parseConnection(raw map[string]any) (settings, error) {
	if err := rejectUnknownKeys(raw); err != nil {
		return settings{}, err
	}
	baseURL, err := parseBaseURL(raw["base_url"])
	if err != nil {
		return settings{}, err
	}
	tokenEnv, err := parseTokenEnv(raw["token_env"])
	if err != nil {
		return settings{}, err
	}
	return settings{BaseURL: baseURL, TokenEnv: tokenEnv}, nil
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
	connection, err := parseConnection(raw)
	if err != nil {
		return settings{}, err
	}
	workspaceID, err := parseWorkspaceID(raw["workspace_id"])
	if err != nil {
		return settings{}, err
	}
	pollSeconds, err := providerconfig.ParseSeconds(raw, "poll_interval_seconds", defaultPollInterval, minPollInterval, maxPollInterval)
	if err != nil {
		return settings{}, err
	}
	timeoutSeconds, err := providerconfig.ParseSeconds(raw, "timeout_seconds", defaultTimeout, minTimeout, maxTimeout)
	if err != nil {
		return settings{}, err
	}
	if timeoutSeconds < pollSeconds {
		return settings{}, configErr("timeout_seconds", fmt.Sprintf("must be greater than or equal to poll_interval_seconds (%d)", pollSeconds))
	}
	return settings{
		BaseURL:      connection.BaseURL,
		WorkspaceID:  workspaceID,
		TokenEnv:     connection.TokenEnv,
		PollInterval: time.Duration(pollSeconds) * time.Second,
		Timeout:      time.Duration(timeoutSeconds) * time.Second,
	}, nil
}

// rejectUnknownKeys catches typos before any other check, and sorts the keys so
// the reported field is deterministic when more than one is unknown.
func rejectUnknownKeys(raw map[string]any) error {
	return providerconfig.RejectUnknownKeys(raw, knownConfigKeys, "arcipelago")
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
