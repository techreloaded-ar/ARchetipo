package ingest

import (
	"sync"
	"time"
)

// Clock abstracts time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// realClock uses the system clock.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// TokenBucket implements RateLimiter using the token bucket algorithm.
// Each origin (hashed IP) gets its own bucket. Idle origin buckets are
// cleaned up periodically by a background goroutine.
type TokenBucket struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	cfg      RateLimitConfig
	clock    Clock
	stopCh   chan struct{}
	stopOnce sync.Once
}

// bucket holds the state for a single origin's token bucket.
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewTokenBucket creates a TokenBucket with the given config and starts the
// cleanup goroutine. Call Close to stop cleanup when the server shuts down.
func NewTokenBucket(cfg RateLimitConfig) *TokenBucket {
	return newTokenBucket(cfg, realClock{})
}

// newTokenBucket is the internal constructor that accepts an injectable clock.
func newTokenBucket(cfg RateLimitConfig, clock Clock) *TokenBucket {
	tb := &TokenBucket{
		buckets: make(map[string]*bucket),
		cfg:     cfg,
		clock:   clock,
		stopCh:  make(chan struct{}),
	}
	go tb.cleanupLoop()
	return tb
}

// Allow implements RateLimiter. It returns true if the request from origin
// should be allowed, and updates the token bucket state.
func (tb *TokenBucket) Allow(origin string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.buckets[origin]
	if !ok {
		// New origin: start with a full bucket (burst capacity).
		b = &bucket{
			tokens:     float64(tb.cfg.Burst),
			lastRefill: tb.clock.Now(),
		}
		tb.buckets[origin] = b
	}

	tb.refill(b)

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Status implements RateLimiter. Returns the current rate limit state for
// an origin without consuming a token.
func (tb *TokenBucket) Status(origin string) RateLimitStatus {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.buckets[origin]
	if !ok {
		return RateLimitStatus{
			Limit:     tb.cfg.Rate,
			Remaining: tb.cfg.Burst,
			Reset:     tb.clock.Now().Add(tb.cfg.Window),
		}
	}

	tb.refill(b)

	resetAt := b.lastRefill.Add(tb.cfg.Window)
	remaining := int(b.tokens)
	if remaining > tb.cfg.Rate {
		remaining = tb.cfg.Rate
	}
	return RateLimitStatus{
		Limit:     tb.cfg.Rate,
		Remaining: remaining,
		Reset:     resetAt,
	}
}

// refill adds tokens based on elapsed time since last refill.
// Must be called under mu.
func (tb *TokenBucket) refill(b *bucket) {
	now := tb.clock.Now()
	elapsed := now.Sub(b.lastRefill)
	if elapsed <= 0 {
		return
	}

	// Tokens to add: (elapsed / window) * rate
	rate := float64(tb.cfg.Rate) / tb.cfg.Window.Seconds()
	added := elapsed.Seconds() * rate
	b.tokens += added

	// Cap at burst capacity.
	if b.tokens > float64(tb.cfg.Burst) {
		b.tokens = float64(tb.cfg.Burst)
	}
	b.lastRefill = now
}

// cleanupLoop periodically removes buckets that haven't been accessed
// for longer than 2 * window. Runs every window duration.
func (tb *TokenBucket) cleanupLoop() {
	ticker := time.NewTicker(tb.cfg.Window)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tb.cleanup()
		case <-tb.stopCh:
			return
		}
	}
}

// cleanup removes idle buckets.
func (tb *TokenBucket) cleanup() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	cutoff := tb.clock.Now().Add(-2 * tb.cfg.Window)
	for origin, b := range tb.buckets {
		if b.lastRefill.Before(cutoff) {
			delete(tb.buckets, origin)
		}
	}
}

// Close stops the background cleanup goroutine. Safe to call multiple times.
func (tb *TokenBucket) Close() {
	tb.stopOnce.Do(func() {
		close(tb.stopCh)
	})
}
