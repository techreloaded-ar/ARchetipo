package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics/ingest"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// analyticsStatus is the JSON envelope data for `archetipo analytics status`.
type analyticsStatus struct {
	Enabled                        bool   `json:"enabled"`
	Source                         string `json:"source"`
	EndpointConfigured             bool   `json:"endpoint_configured"`
	EndpointHost                   string `json:"endpoint_host"`
	AnonymousInstallationIDPresent bool   `json:"anonymous_installation_id_present"`
}

func newAnalyticsCmd(s streams) *cobra.Command {
	root := &cobra.Command{
		Use:   "analytics",
		Short: "Gestisci telemetria: consenso e server di ingest",
		Long:  "Sottocomandi per gestire il consenso telemetria (status, enable, disable) e avviare il server di ingest per eventi archetipo.analytics/v1 (serve).",
	}
	root.AddCommand(
		newAnalyticsStatusCmd(s),
		newAnalyticsEnableCmd(s),
		newAnalyticsDisableCmd(s),
		newAnalyticsServeCmd(s),
	)
	return root
}

// ---- Consent management (US-003, US-005) ----

func newAnalyticsStatusCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Mostra lo stato del consenso telemetria",
		Long:  "Legge la configurazione di progetto e indica se la telemetria è abilitata, da dove proviene il consenso (project_config o default), l'endpoint di telemetria e se esiste un ID di installazione anonimo.",
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
			consent := false
			if cfg.Analytics.Consent != nil {
				consent = *cfg.Analytics.Consent
			}
			endpointConfigured := false
			endpointHost := "none (local noop)"
			if cfg.Analytics.Endpoint != "" {
				if u, parseErr := url.Parse(cfg.Analytics.Endpoint); parseErr == nil && u.Host != "" {
					endpointConfigured = true
					endpointHost = u.Host
				} else if !strings.HasPrefix(cfg.Analytics.Endpoint, "http") {
					// Non-URL endpoint (e.g. host:port), show redacted.
					endpointConfigured = true
					endpointHost = redactEndpoint(cfg.Analytics.Endpoint)
				}
			}
			st := analyticsStatus{
				Enabled:                        consent,
				Source:                         source,
				EndpointConfigured:             endpointConfigured,
				EndpointHost:                   endpointHost,
				AnonymousInstallationIDPresent: idPresent,
			}
			return iox.WriteOK(s.out, "analytics_status", st)
		},
	}
}

func newAnalyticsEnableCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Abilita il consenso telemetria per questo progetto",
		Long:  "Imposta analytics.consent: true nel file .archetipo/config.yaml del progetto. Idempotente: eseguirlo di nuovo quando già abilitato non ha effetto.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return setConsent(s, true)
		},
	}
}

func newAnalyticsDisableCmd(s streams) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disabilita il consenso telemetria per questo progetto",
		Long:  "Imposta analytics.consent: false nel file .archetipo/config.yaml del progetto. Idempotente: eseguirlo di nuovo quando già disabilitato non ha effetto.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return setConsent(s, false)
		},
	}
}

// setConsent loads the project config, checks whether the desired consent
// value is already explicitly set (idempotent no-op), and otherwise writes
// the consent key to .archetipo/config.yaml. When enabling, it also generates
// an anonymous installation ID if one does not already exist.
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
	if hasConsent && cfg.Analytics.Consent != nil && *cfg.Analytics.Consent == consent {
		msg := "analytics already enabled"
		if !consent {
			msg = "analytics already disabled"
		}
		return iox.WriteOK(s.out, "write_result", map[string]any{"ok": true, "message": msg})
	}
	if err := cfg.SetAnalyticsConsent(consent); err != nil {
		return iox.NewInternal("writing analytics consent", err)
	}
	// Generate anonymous installation ID on first enable.
	if consent && cfg.Analytics.AnonymousInstallationID == "" {
		cfg.Analytics.Consent = &consent // SetAnalyticsConsent already wrote consent, but cfg is stale
		if _, idErr := cfg.EnsureAnonymousInstallationID(); idErr != nil {
			return iox.NewInternal("generating anonymous installation id", idErr)
		}
	}
	msg := "analytics enabled"
	if !consent {
		msg = "analytics disabled"
	}
	return iox.WriteOK(s.out, "write_result", map[string]any{"ok": true, "message": msg})
}

// redactEndpoint returns a privacy-safe version of the endpoint string:
// only the host is shown (no scheme, path, or query).
func redactEndpoint(raw string) string {
	// Try parsing as URL first.
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	// Fallback: strip scheme prefix if present.
	s := raw
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// ---- Server ingest (US-006) ----

func newAnalyticsServeCmd(s streams) *cobra.Command {
	var (
		addr       string
		rateLimit  int
		rateWindow time.Duration
		storageTTL time.Duration
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Avvia il server di ingest analytics (POST /v1/events)",
		Long: "Avvia un server HTTP che espone l'endpoint POST /v1/events per la raccolta " +
			"di eventi telemetrici nel formato archetipo.analytics/v1. Il server implementa " +
			"rate limiting, validazione strict dello schema e storage in-memory anonimizzato.\n\n" +
			"Il server non richiede autenticazione. La protezione anti-abuso si basa su rate " +
			"limiting e validazione schema. Gli IP dei client non sono mai persistiti.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := ingest.NewMemoryStore(storageTTL)
			cfg := ingest.ServerConfig{
				Addr: addr,
				RateLimit: ingest.RateLimitConfig{
					Rate:   rateLimit,
					Window: rateWindow,
					Burst:  10,
				},
				StorageTTL: storageTTL,
			}

			srv := ingest.NewServer(cfg, store)

			ctx, cancel := signal.NotifyContext(context.Background(),
				os.Interrupt, syscall.SIGTERM)
			defer cancel()

			fmt.Fprintf(s.out, "Avvio server analytics su %s\n", cfg.Addr)
			fmt.Fprintf(s.out, "Endpoint: POST /v1/events\n")
			fmt.Fprintf(s.out, "Rate limit: %d richieste/%s, burst 10\n", cfg.RateLimit.Rate, cfg.RateLimit.Window)
			fmt.Fprintf(s.out, "Storage TTL: %s\n", cfg.StorageTTL)

			return srv.Run(ctx, func(url string) {
				fmt.Fprintf(s.out, "In ascolto su %s\n", url)
			})
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080",
		"Indirizzo di ascolto (es. 127.0.0.1:8080)")
	cmd.Flags().IntVar(&rateLimit, "rate-limit", 60,
		"Numero massimo di richieste per finestra temporale")
	cmd.Flags().DurationVar(&rateWindow, "rate-window", 1*time.Minute,
		"Finestra temporale per il rate limiting (es. 1m, 30s)")
	cmd.Flags().DurationVar(&storageTTL, "storage-ttl", 168*time.Hour,
		"TTL degli eventi in storage (es. 168h = 7 giorni)")

	return cmd
}
