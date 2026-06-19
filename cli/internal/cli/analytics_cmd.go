package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// analyticsStatus is the JSON envelope data for `archetipo analytics status`.
type analyticsStatus struct {
	Enabled                        bool   `json:"enabled"`
	Source                         string `json:"source"`
	Endpoint                       string `json:"endpoint"`
	AnonymousInstallationIDPresent bool   `json:"anonymous_installation_id_present"`
}

func newAnalyticsCmd(s streams) *cobra.Command {
	root := &cobra.Command{
		Use:   "analytics",
		Short: "Manage telemetry consent",
		Long:  "Enable, disable, or check telemetry consent. All changes are scoped to the current project's .archetipo/config.yaml.",
	}
	root.AddCommand(
		newAnalyticsStatusCmd(s),
		newAnalyticsEnableCmd(s),
		newAnalyticsDisableCmd(s),
	)
	return root
}

func newAnalyticsStatusCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show telemetry consent status",
		Long:  "Reads the project config and returns whether telemetry is enabled, where the consent was set (project_config or default), the telemetry endpoint channel name, and whether an anonymous installation ID exists.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return iox.NewInternal("cwd unavailable", err)
			}
			cfg, err := config.Load(cwd)
			if err != nil {
				return iox.NewInvalidInput(err.Error(), "fix the file or remove it to fall back to defaults", err)
			}
			hasConsent, err := cfg.HasAnalyticsConsent()
			if err != nil {
				return iox.NewInternal("checking analytics consent", err)
			}
			source := "default"
			if hasConsent {
				source = "project_config"
			}
			idPresent := cfg.Analytics.AnonymousInstallationID != ""
			st := analyticsStatus{
				Enabled:                        cfg.Analytics.Consent,
				Source:                         source,
				Endpoint:                       config.AnalyticsEndpoint,
				AnonymousInstallationIDPresent: idPresent,
			}
			return iox.WriteOK(s.out, "analytics_status", st)
		},
	}
}

func newAnalyticsEnableCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable telemetry consent for this project",
		Long:  "Sets analytics.consent: true in the project's .archetipo/config.yaml. Idempotent: running it again when already enabled is a no-op.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return setConsent(s, true)
		},
	}
}

func newAnalyticsDisableCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable telemetry consent for this project",
		Long:  "Sets analytics.consent: false in the project's .archetipo/config.yaml. Idempotent: running it again when already disabled is a no-op.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return setConsent(s, false)
		},
	}
}

// setConsent loads the project config, checks whether the desired consent
// value is already explicitly set (idempotent no-op), and otherwise writes
// the consent key to .archetipo/config.yaml.
func setConsent(s streams, consent bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return iox.NewInternal("cwd unavailable", err)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return iox.NewInvalidInput(err.Error(), "fix the file or remove it to fall back to defaults", err)
	}
	hasConsent, err := cfg.HasAnalyticsConsent()
	if err != nil {
		return iox.NewInternal("checking analytics consent", err)
	}
	// Idempotent no-op: the key already exists with the desired value.
	if hasConsent && cfg.Analytics.Consent == consent {
		msg := "analytics already enabled"
		if !consent {
			msg = "analytics already disabled"
		}
		return iox.WriteOK(s.out, "write_result", map[string]any{"ok": true, "message": msg})
	}
	if err := cfg.SetAnalyticsConsent(consent); err != nil {
		return iox.NewInternal("writing analytics consent", err)
	}
	msg := "analytics enabled"
	if !consent {
		msg = "analytics disabled"
	}
	return iox.WriteOK(s.out, "write_result", map[string]any{"ok": true, "message": msg})
}
