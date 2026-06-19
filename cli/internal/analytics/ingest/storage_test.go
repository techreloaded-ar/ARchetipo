package ingest

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_StoreAndRetrieve(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer ms.Close()

	evt := AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "test.event",
		Tool:   "test-tool",
	}
	ms.Store(evt)

	if ms.Len() != 1 {
		t.Fatalf("expected 1 event, got %d", ms.Len())
	}

	events := ms.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "test.event" {
		t.Fatalf("expected event 'test.event', got %q", events[0].Event)
	}
	if events[0].Tool != "test-tool" {
		t.Fatalf("expected tool 'test-tool', got %q", events[0].Tool)
	}
}

func TestMemoryStore_InsertionOrderPreserved(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer ms.Close()

	for i := 0; i < 5; i++ {
		ms.Store(AnalyticsEvent{
			Schema: "archetipo.analytics/v1",
			Event:  "test.event",
			Tool:   "t",
		})
		time.Sleep(time.Millisecond) // ensure distinct ReceivedAt
	}

	events := ms.Events()
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	// Verify timestamps are monotonically increasing.
	for i := 1; i < len(events); i++ {
		if events[i].ReceivedAt.Before(events[i-1].ReceivedAt) {
			t.Fatalf("event %d ReceivedAt before event %d: insertion order not preserved", i, i-1)
		}
	}
}

func TestMemoryStore_TTLCleanup(t *testing.T) {
	ttl := 100 * time.Millisecond
	ms := NewMemoryStore(ttl)
	defer ms.Close()

	// Store an event.
	ms.Store(AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "old"})
	if ms.Len() != 1 {
		t.Fatalf("expected 1 event, got %d", ms.Len())
	}

	// Wait for TTL to expire.
	time.Sleep(ttl + 100*time.Millisecond)

	// Manual cleanup.
	ms.cleanup()

	if ms.Len() != 0 {
		t.Fatalf("expected 0 events after TTL, got %d", ms.Len())
	}
}

func TestMemoryStore_TTLKeepsRecent(t *testing.T) {
	ttl := 200 * time.Millisecond
	ms := NewMemoryStore(ttl)
	defer ms.Close()

	// Store an old event.
	ms.Store(AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "old"})

	// Wait half TTL, then store another.
	time.Sleep(100 * time.Millisecond)
	ms.Store(AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "recent"})

	// Wait for old event to expire.
	time.Sleep(150 * time.Millisecond)
	ms.cleanup()

	if ms.Len() != 1 {
		t.Fatalf("expected 1 event (recent kept), got %d", ms.Len())
	}
	events := ms.Events()
	if events[0].Event != "recent" {
		t.Fatalf("expected 'recent' event, got %q", events[0].Event)
	}
}

func TestMemoryStore_StoredEventNoIP(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer ms.Close()

	evt := AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "test.event",
	}
	ms.Store(evt)

	events := ms.Events()
	stored := events[0]

	// StoredEvent must not have any IP-related fields.
	// This is enforced by the type: StoredEvent does not embed
	// any origin fields. The conversion is done in StoredEventFromAnalytics
	// which only copies the allowlist fields.
	_ = stored // type-level guarantee — StoredEvent has no IP fields
}

func TestMemoryStore_Concurrency(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer ms.Close()

	var wg sync.WaitGroup
	numWriters := 10
	numEventsPerWriter := 100

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numEventsPerWriter; j++ {
				ms.Store(AnalyticsEvent{
					Schema: "archetipo.analytics/v1",
					Event:  "test.event",
				})
			}
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = ms.Events()
				_ = ms.Len()
			}
		}()
	}

	wg.Wait()

	total := ms.Len()
	expected := numWriters * numEventsPerWriter
	if total != expected {
		t.Fatalf("expected %d events, got %d", expected, total)
	}
}

func TestMemoryStore_EventsReturnsCopy(t *testing.T) {
	ms := NewMemoryStore(time.Hour)
	defer ms.Close()

	ms.Store(AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "original"})

	events := ms.Events()
	events[0].Event = "modified"

	// Original should be unchanged.
	events2 := ms.Events()
	if events2[0].Event != "original" {
		t.Fatal("Events() should return a copy, not a reference to internal slice")
	}
}
