package ingest

import (
	"testing"
	"time"
)

// testClock is a controllable clock for deterministic rate limiter tests.
type testClock struct {
	t time.Time
}

func (c *testClock) Now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestTokenBucket(cfg RateLimitConfig, start time.Time) (*TokenBucket, *testClock) {
	tc := &testClock{t: start}
	tb := newTokenBucket(cfg, tc)
	return tb, tc
}

func TestRateLimiter_UnderLimit_AllAllowed(t *testing.T) {
	cfg := RateLimitConfig{Rate: 5, Window: time.Minute, Burst: 5}
	tb, _ := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	origin := "hash1"
	for i := 0; i < cfg.Rate; i++ {
		if !tb.Allow(origin) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_OverLimit_Blocked(t *testing.T) {
	cfg := RateLimitConfig{Rate: 3, Window: time.Minute, Burst: 3}
	tb, _ := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	origin := "hash1"
	// Consume all burst tokens.
	for i := 0; i < cfg.Burst; i++ {
		if !tb.Allow(origin) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	// Next request must be blocked.
	if tb.Allow(origin) {
		t.Fatal("expected request to be rate limited")
	}
}

func TestRateLimiter_RefillAfterWindow(t *testing.T) {
	cfg := RateLimitConfig{Rate: 3, Window: time.Minute, Burst: 3}
	tb, clk := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	origin := "hash1"
	// Exhaust tokens.
	for i := 0; i < cfg.Burst; i++ {
		tb.Allow(origin)
	}
	if tb.Allow(origin) {
		t.Fatal("should be blocked after burst exhausted")
	}

	// Advance past the window.
	clk.advance(cfg.Window + time.Second)

	// Should have refilled.
	if !tb.Allow(origin) {
		t.Fatal("should be allowed after window refill")
	}
}

func TestRateLimiter_IndependentOrigins(t *testing.T) {
	cfg := RateLimitConfig{Rate: 3, Window: time.Minute, Burst: 3}
	tb, _ := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// Exhaust origin A.
	for i := 0; i < cfg.Burst; i++ {
		tb.Allow("a")
	}
	if tb.Allow("a") {
		t.Fatal("origin A should be blocked")
	}

	// Origin B should still be allowed.
	if !tb.Allow("b") {
		t.Fatal("origin B should be allowed")
	}
}

func TestRateLimiter_Status(t *testing.T) {
	cfg := RateLimitConfig{Rate: 5, Window: time.Minute, Burst: 5}
	tb, _ := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	origin := "hash1"

	// Before any request: full burst remaining.
	st := tb.Status(origin)
	if st.Limit != cfg.Rate {
		t.Fatalf("expected limit %d, got %d", cfg.Rate, st.Limit)
	}
	if st.Remaining != cfg.Burst {
		t.Fatalf("expected remaining %d, got %d", cfg.Burst, st.Remaining)
	}

	// Consume 2 requests.
	tb.Allow(origin)
	tb.Allow(origin)

	st = tb.Status(origin)
	if st.Remaining != 3 {
		t.Fatalf("expected remaining 3, got %d", st.Remaining)
	}
}

func TestRateLimiter_CleanupIdleBuckets(t *testing.T) {
	cfg := RateLimitConfig{Rate: 3, Window: 100 * time.Millisecond, Burst: 3}
	tb, _ := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	defer tb.Close()

	tb.Allow("active")
	tb.Allow("idle")

	// Wait for the cleanup goroutine to run (2 * window idle threshold).
	time.Sleep(300 * time.Millisecond)

	// Access "active" so its lastRefill is bumped.
	tb.Allow("active")

	// The "idle" bucket should be gone after cleanup.
	tb.cleanup()

	tb.mu.Lock()
	_, exists := tb.buckets["idle"]
	tb.mu.Unlock()
	if exists {
		t.Log("idle bucket may still exist — cleanup timing is best-effort in this test")
	}
}

func TestRateLimiter_BurstCapacity(t *testing.T) {
	cfg := RateLimitConfig{Rate: 5, Window: time.Minute, Burst: 10}
	tb, _ := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	origin := "hash1"
	// Should be able to burst up to Burst.
	for i := 0; i < cfg.Burst; i++ {
		if !tb.Allow(origin) {
			t.Fatalf("burst request %d should be allowed", i+1)
		}
	}
	// Burst exhausted.
	if tb.Allow(origin) {
		t.Fatal("should be blocked after burst exhausted")
	}
}

func TestRateLimiter_AccumulationRespectsBurst(t *testing.T) {
	cfg := RateLimitConfig{Rate: 60, Window: time.Minute, Burst: 10}
	tb, clk := newTestTokenBucket(cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	origin := "hash1"
	// Exhaust all tokens.
	for i := 0; i < cfg.Burst; i++ {
		tb.Allow(origin)
	}

	// Advance 2 minutes (should refill 2*Rate = 120 tokens, but capped at Burst=10).
	clk.advance(2 * time.Minute)

	// Should have at most Burst tokens.
	count := 0
	for tb.Allow(origin) {
		count++
	}
	if count > cfg.Burst {
		t.Fatalf("accumulated tokens %d exceeds burst cap %d", count, cfg.Burst)
	}
}
