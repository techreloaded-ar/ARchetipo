package ingest

import (
	"sync"
	"time"
)

// MemoryStore implements EventStore with an in-memory buffer protected by
// a read-write mutex. Events older than TTL are cleaned up by a background
// goroutine. No IP raw data is ever stored here.
type MemoryStore struct {
	mu       sync.RWMutex
	events   []StoredEvent
	ttl      time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewMemoryStore creates a MemoryStore with the given TTL and starts the
// cleanup goroutine. Call Close when the server shuts down.
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	ms := &MemoryStore{
		events: make([]StoredEvent, 0),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go ms.cleanupLoop()
	return ms
}

// Store implements EventStore. It converts the AnalyticsEvent to a StoredEvent
// (which carries no IP data) and appends it to the in-memory buffer.
func (ms *MemoryStore) Store(event AnalyticsEvent) {
	se := StoredEventFromAnalytics(event)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.events = append(ms.events, se)
}

// Events implements EventStore. Returns a snapshot of stored events in
// insertion order. The returned slice is a copy.
func (ms *MemoryStore) Events() []StoredEvent {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	out := make([]StoredEvent, len(ms.events))
	copy(out, ms.events)
	return out
}

// Len implements EventStore. Returns the number of currently stored events.
func (ms *MemoryStore) Len() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.events)
}

// cleanupLoop runs the TTL cleanup on a periodic tick (every ttl/10 or at
// least every minute).
func (ms *MemoryStore) cleanupLoop() {
	interval := ms.ttl / 10
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.cleanup()
		case <-ms.stopCh:
			return
		}
	}
}

// cleanup removes events older than TTL.
func (ms *MemoryStore) cleanup() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	cutoff := time.Now().Add(-ms.ttl)
	n := 0
	for _, e := range ms.events {
		if e.ReceivedAt.After(cutoff) {
			ms.events[n] = e
			n++
		}
	}
	ms.events = ms.events[:n]
}

// Close stops the background cleanup goroutine. Safe to call multiple times.
func (ms *MemoryStore) Close() {
	ms.stopOnce.Do(func() {
		close(ms.stopCh)
	})
}
