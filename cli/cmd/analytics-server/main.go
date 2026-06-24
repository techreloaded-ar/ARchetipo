// Command archetipo-analytics is the standalone analytics ingest server.
//
// It exposes POST /v1/events (same schema as `archetipo analytics serve`)
// backed by a persistent SQLite database, plus GET /healthz for platform
// healthchecks. It is intended to run on Fly.io (or any container host)
// with the database file on a persistent volume.
//
// Unlike the `archetipo` CLI, this binary does not include the connector,
// config loader, or skills plumbing — only the ingest server + SQLite store.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics/ingest"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics/ingest/sqlitestore"
)

func main() {
	addr := flag.String("addr", envOr("ADDR", "0.0.0.0:8080"),
		"indirizzo di ascolto (default 0.0.0.0:8080; env ADDR)")
	dbPath := flag.String("db-path", envOr("DB_PATH", "/data/analytics.db"),
		"percorso del file SQLite (default /data/analytics.db; env DB_PATH)")
	rateLimit := flag.Int("rate-limit", 60,
		"numero massimo di richieste per finestra temporale")
	rateWindow := flag.Duration("rate-window", time.Minute,
		"finestra temporale per il rate limiting (es. 1m, 30s)")
	storageTTL := flag.Duration("storage-ttl", 168*time.Hour,
		"TTL degli eventi in storage (es. 168h = 7 giorni)")
	flag.Parse()

	// Validate the storage TTL.
	if *storageTTL <= 0 {
		fmt.Fprintln(os.Stderr, "storage-ttl must be positive")
		os.Exit(2)
	}

	// Open the SQLite store (persistent across restarts).
	store, err := sqlitestore.New(*dbPath, *storageTTL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening sqlite store at %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer store.Close()

	cfg := ingest.ServerConfig{
		Addr: *addr,
		RateLimit: ingest.RateLimitConfig{
			Rate:   *rateLimit,
			Window: *rateWindow,
			Burst:  10,
		},
		StorageTTL: *storageTTL,
	}
	srv := ingest.NewServer(cfg, store)

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(os.Stdout, "archetipo-analytics\n")
	fmt.Fprintf(os.Stdout, "Listening on %s (POST /v1/events, GET /healthz)\n", cfg.Addr)
	fmt.Fprintf(os.Stdout, "Database: %s\n", *dbPath)
	fmt.Fprintf(os.Stdout, "Rate limit: %d req/%s, burst 10\n", cfg.RateLimit.Rate, cfg.RateLimit.Window)
	fmt.Fprintf(os.Stdout, "Storage TTL: %s\n", cfg.StorageTTL)

	if err := srv.Run(ctx, func(url string) {
		fmt.Fprintf(os.Stdout, "Ready on %s\n", url)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// envOr returns the value of the environment variable named by key, or
// fallback if it is empty/unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
