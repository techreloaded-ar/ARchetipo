package ingest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Server wraps an HTTP server that exposes the analytics ingest endpoint.
type Server struct {
	httpSrv  *http.Server
	store    *MemoryStore
	limiter  *TokenBucket
	handler  *IngestHandler
}

// ServerConfig holds all parameters needed to start the analytics server.
type ServerConfig struct {
	Addr       string
	RateLimit  RateLimitConfig
	StorageTTL time.Duration
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Addr:       "127.0.0.1:8080",
		RateLimit:  DefaultRateLimitConfig(),
		StorageTTL: 7 * 24 * time.Hour,
	}
}

// NewServer constructs a Server with the given config. The server is not
// started yet — call Run to begin listening.
func NewServer(cfg ServerConfig) *Server {
	store := NewMemoryStore(cfg.StorageTTL)
	limiter := NewTokenBucket(cfg.RateLimit)
	handler := NewIngestHandler(limiter, store)

	mux := http.NewServeMux()
	mux.Handle("/v1/events", handler)

	return &Server{
		httpSrv: &http.Server{
			Addr:              cfg.Addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		store:   store,
		limiter: limiter,
		handler: handler,
	}
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() string { return s.httpSrv.Addr }

// Store returns the underlying event store (for inspection in tests).
func (s *Server) Store() *MemoryStore { return s.store }

// Handler returns the underlying ingest handler (for testing with fixed origins).
func (s *Server) Handler() *IngestHandler { return s.handler }

// Run starts listening and blocks until ctx is done or the server errors.
// When ctx is cancelled the server is shut down with a 5s grace period.
// onReady is called (if non-nil) once the server is accepting connections.
func (s *Server) Run(ctx context.Context, onReady func(url string)) error {
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("analytics server listening on %s: %w", s.httpSrv.Addr, err)
	}
	s.httpSrv.Addr = ln.Addr().String()
	if onReady != nil {
		onReady("http://" + s.httpSrv.Addr)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		s.limiter.Close()
		s.store.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		s.limiter.Close()
		s.store.Close()
		return err
	}
}
