package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics/ingest"
)

func newAnalyticsCmd(s streams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analytics",
		Short: "Gestisci il server di ingest telemetria",
		Long:  "Sottocomandi per avviare e gestire il server di ingest per eventi telemetrici archetipo.analytics/v1.",
	}
	cmd.AddCommand(newAnalyticsServeCmd(s))
	return cmd
}

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
			cfg := ingest.ServerConfig{
				Addr: addr,
				RateLimit: ingest.RateLimitConfig{
					Rate:   rateLimit,
					Window: rateWindow,
					Burst:  10,
				},
				StorageTTL: storageTTL,
			}

			srv := ingest.NewServer(cfg)

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
