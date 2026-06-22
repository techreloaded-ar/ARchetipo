package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	// Concrete connectors register themselves via init().
	_ "github.com/techreloaded-ar/ARchetipo/cli/internal/connector/builtin"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/uuid"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

// AnalyticsSender is the interface the CLI uses to send analytics events.
// The default implementation is analytics.Client; tests inject mocks via
// AnalyticsClientFactory.
type AnalyticsSender interface {
	Send(ctx context.Context, e analytics.Event) error
}

// AnalyticsClientFactory is the injection point for tests. The default
// implementation creates an *analytics.Client when analytics is enabled
// and a config file is available; otherwise it returns nil.
var AnalyticsClientFactory func(cfg config.Config) AnalyticsSender

// Execute runs the archetipo CLI with the given args and returns the process
// exit code. Stdin/stdout/stderr are taken as parameters so tests can drive the
// CLI without touching the real OS streams.
//
// Exit codes follow the public CLI runtime contract:
//
//	0  ok
//	1  generic error
//	2  input/validation error
//	3  connector error (auth, network, gh)
//	4  precondition missing (e.g. backlog absent)
//
// On error, the JSON envelope is written to stderr exactly once: sub-commands
// return typed errors and Execute serializes them, so handlers don't have to.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	notifier := version.NewNotifier(version.NotifierConfig{
		PackageName: npmPackageName,
		UpdateCmd:   "archetipo update",
		CacheTTL:    24 * time.Hour,
		HTTPTimeout: 2 * time.Second,
	}, version.Version)
	notifier.Start()
	defer notifier.Print(stderr)

	// Session ID is generated once per invocation for event correlation.
	sessionID, _ := uuid.NewV4()
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Track the leaf command executed for analytics normalization.
	var leafCmd *cobra.Command
	start := time.Now()
	root := newRootCmd(stdin, stdout, stderr, &leafCmd)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		iox.WriteError(stderr, err)
	}
	exitCode := exitCodeFor(err)

	// Build analytics sender AFTER command execution, so consent changes
	// (e.g. analytics enable/disable) are reflected in the send decision.
	var sender AnalyticsSender
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		cfg, cfgErr := config.Load(cwd)
		if cfgErr == nil && cfg.Analytics.Consent != nil && *cfg.Analytics.Consent {
			if AnalyticsClientFactory != nil {
				sender = AnalyticsClientFactory(cfg)
			} else {
				sender = initAnalyticsSenderFromConfig(cfg)
			}
		}
	}

	if sender != nil {
		success := err == nil
		event := analytics.Event{
			Schema:                  analytics.DefaultSchema,
			Event:                   analytics.EventCommandCompleted,
			Timestamp:               timestamp,
			Command:                 normalizeCommand(leafCmd),
			ArchetipoVersion:        version.Version,
			SessionID:               sessionID,
			OS:                      runtime.GOOS,
			Arch:                    runtime.GOARCH,
			Success:                 &success,
			ExitCode:                exitCode,
			DurationMs:              durationMs,
			ErrorCode:               extractErrorCode(err),
			Connector:               resolveConnector(),
			CI:                      isCI(),
			AnonymousInstallationID: resolveAnonymousInstallationID(),
			Args:                    extractArgs(leafCmd),
		}
		// Fail-silent: never alter exit code or output.
		_ = sender.Send(context.Background(), event)
	}

	return exitCode
}

// normalizeCommand converts the Cobra command path (e.g. "archetipo spec list")
// into the dotted format used by analytics events (e.g. "spec.list").
// Returns empty string when cmd is nil (root-only invocation).
func normalizeCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	p := cmd.CommandPath()
	// CommandPath returns "archetipo spec list" — strip the root.
	parts := strings.Fields(p)
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], ".")
}

// extractErrorCode returns the stable error code from a *iox.CodedError,
// or "" for nil errors and untyped errors.
func extractErrorCode(err error) string {
	var ce *iox.CodedError
	if err != nil && errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// resolveConnector returns the connector name from the project config
// (best-effort). Returns "unknown" when the config is unavailable.
func resolveConnector() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return "unknown"
	}
	return cfg.Connector
}

// initAnalyticsSenderFromConfig builds the analytics sender from the project
// config. Returns nil when analytics is disabled.
func initAnalyticsSenderFromConfig(cfg config.Config) AnalyticsSender {
	if cfg.Analytics.Consent == nil || !*cfg.Analytics.Consent {
		return nil
	}
	s := analytics.Settings{
		Enabled:  true,
		Endpoint: cfg.Analytics.Endpoint,
	}
	return analytics.NewClient(s, nil)
}

// isCI reports whether the current environment is a CI system.
func isCI() bool {
	for _, v := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "JENKINS_HOME", "TRAVIS"} {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// resolveAnonymousInstallationID returns the anonymous installation ID from
// the project config, or empty string when unavailable.
func resolveAnonymousInstallationID() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return ""
	}
	return cfg.Analytics.AnonymousInstallationID
}

func newRootCmd(stdin io.Reader, stdout, stderr io.Writer, leafCmd **cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "archetipo",
		Short:         "ARchetipo connector CLI",
		Long:          "Deterministic CLI implementing the ARchetipo workflow operations (file and github).",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			if leafCmd != nil {
				*leafCmd = cmd
			}
		},
	}
	cmd.SetVersionTemplate(versionLine())
	cmd.SetContext(context.Background())

	s := streams{in: stdin, out: stdout, err: stderr}
	cmd.AddCommand(
		newAnalyticsCmd(s),
		newConfigCmd(s),
		newDoctorCmd(s),
		newInitProjectCmd(s),
		newUninstallCmd(s),
		newUpdateCmd(s),
		newPRDCmd(s),
		newSpecCmd(s),
		newMetricsCmd(s),
		newTaskCmd(s),
		newViewCmd(s),
		newVersionCmd(s),
		newAnalyticsCmd(s),
	)
	return cmd
}

// exitCodeFor maps a returned error to the documented exit code. Specific
// error types defined in internal/iox carry their own code; everything else is
// a generic error (1).
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	if coded, ok := err.(interface{ ExitCode() int }); ok {
		return coded.ExitCode()
	}
	return 1
}
