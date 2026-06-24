package sqlitestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics/ingest"
)

// newTempStore creates a SQLiteStore backed by a temp file, scoped to the
// test, with a 1-hour TTL. The db file (and -wal/-shm sidecars) are removed
// on cleanup.
func newTempStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "analytics.db")
	s, err := New(path, time.Hour)
	if err != nil {
		t.Fatalf("New SQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func validEvent() ingest.AnalyticsEvent {
	return ingest.AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "test.event",
		Tool:   "test-tool",
		OS:     "linux",
		Arch:   "amd64",
	}
}

func TestSQLiteStore_StoreAndRetrieve(t *testing.T) {
	s := newTempStore(t)

	s.Store(validEvent())

	if s.Len() != 1 {
		t.Fatalf("expected 1 event, got %d", s.Len())
	}
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "test.event" {
		t.Fatalf("expected event 'test.event', got %q", events[0].Event)
	}
	if events[0].Tool != "test-tool" {
		t.Fatalf("expected tool 'test-tool', got %q", events[0].Tool)
	}
	if events[0].OS != "linux" {
		t.Fatalf("expected os 'linux', got %q", events[0].OS)
	}
}

func TestSQLiteStore_InsertionOrderPreserved(t *testing.T) {
	s := newTempStore(t)

	for i := 0; i < 5; i++ {
		s.Store(ingest.AnalyticsEvent{
			Schema: "archetipo.analytics/v1",
			Event:  "test.event",
		})
		time.Sleep(5 * time.Millisecond) // distinct ReceivedAt
	}

	events := s.Events()
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].ReceivedAt.Before(events[i-1].ReceivedAt) {
			t.Fatalf("event %d ReceivedAt before event %d: insertion order not preserved", i, i-1)
		}
	}
}

func TestSQLiteStore_TTLCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analytics.db")
	// Short TTL so cleanup kicks in quickly.
	s, err := New(path, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	s.Store(ingest.AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "old"})
	if s.Len() != 1 {
		t.Fatalf("expected 1 event, got %d", s.Len())
	}

	// Wait for TTL to expire, then trigger cleanup manually.
	time.Sleep(150 * time.Millisecond)
	s.cleanup()

	if s.Len() != 0 {
		t.Fatalf("expected 0 events after TTL, got %d", s.Len())
	}
}

func TestSQLiteStore_TTLKeepsRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analytics.db")
	s, err := New(path, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	s.Store(ingest.AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "old"})
	time.Sleep(150 * time.Millisecond)
	s.Store(ingest.AnalyticsEvent{Schema: "archetipo.analytics/v1", Event: "recent"})
	// Wait so "old" expires but "recent" is still fresh.
	time.Sleep(200 * time.Millisecond)
	s.cleanup()

	if s.Len() != 1 {
		t.Fatalf("expected 1 event (recent kept), got %d", s.Len())
	}
	events := s.Events()
	if events[0].Event != "recent" {
		t.Fatalf("expected 'recent' event, got %q", events[0].Event)
	}
}

// TestSQLiteStore_PersistenceSurvivesCloseReopen is the key test: data must
// survive a close + reopen cycle (simulating a redeploy / restart on Fly).
func TestSQLiteStore_PersistenceSurvivesCloseReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analytics.db")

	// First run: store 3 events.
	s1, err := New(path, time.Hour)
	if err != nil {
		t.Fatalf("New s1: %v", err)
	}
	for i := 0; i < 3; i++ {
		s1.Store(ingest.AnalyticsEvent{
			Schema: "archetipo.analytics/v1",
			Event:  "persist.test",
		})
	}
	if s1.Len() != 3 {
		t.Fatalf("expected 3 events before close, got %d", s1.Len())
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close s1: %v", err)
	}

	// Second run: reopen the same file and verify events survived.
	s2, err := New(path, time.Hour)
	if err != nil {
		t.Fatalf("New s2: %v", err)
	}
	defer s2.Close()

	if s2.Len() != 3 {
		t.Fatalf("expected 3 events after reopen, got %d — persistence broken", s2.Len())
	}
	events := s2.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events from Events(), got %d", len(events))
	}
	for _, e := range events {
		if e.Event != "persist.test" {
			t.Fatalf("unexpected event %q", e.Event)
		}
	}
}

func TestSQLiteStore_StoredEventNoIP(t *testing.T) {
	s := newTempStore(t)
	s.Store(validEvent())

	events := s.Events()
	if len(events) != 1 {
		t.Fatal("expected 1 stored event")
	}
	data, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, forbidden := range []string{"ip", "ip_address", "addr", "origin", "remote"} {
		if _, ok := m[forbidden]; ok {
			t.Fatalf("stored event contains forbidden key %q", forbidden)
		}
	}
}

func TestSQLiteStore_Concurrency(t *testing.T) {
	s := newTempStore(t)

	var wg sync.WaitGroup
	numWriters := 10
	numEventsPerWriter := 100

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numEventsPerWriter; j++ {
				s.Store(ingest.AnalyticsEvent{
					Schema: "archetipo.analytics/v1",
					Event:  "test.event",
				})
			}
		}()
	}

	// Concurrent readers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = s.Events()
				_ = s.Len()
			}
		}()
	}
	wg.Wait()

	expected := numWriters * numEventsPerWriter
	if s.Len() != expected {
		t.Fatalf("expected %d events, got %d", expected, s.Len())
	}
}

func TestSQLiteStore_EventsReturnsCopy(t *testing.T) {
	s := newTempStore(t)
	s.Store(validEvent())

	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	events[0].Event = "modified"

	// Re-fetch: original event must be unchanged.
	again := s.Events()
	if again[0].Event != "test.event" {
		t.Fatal("Events() should return a copy, not a reference to internal state")
	}
}

func TestSQLiteStore_NestedArgsPropertiesRoundtrip(t *testing.T) {
	s := newTempStore(t)
	evt := ingest.AnalyticsEvent{
		Schema: "archetipo.analytics/v1",
		Event:  "args.test",
		Args: map[string]any{
			"spec_code": "US-001",
			"count":     42,
		},
		Properties: map[string]any{
			"duration_bucket": "fast",
		},
	}
	s.Store(evt)

	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Args["spec_code"] != "US-001" {
		t.Fatalf("args roundtrip failed: %v", events[0].Args)
	}
	if events[0].Properties["duration_bucket"] != "fast" {
		t.Fatalf("properties roundtrip failed: %v", events[0].Properties)
	}
}

func TestSQLiteStore_SuccessPointerRoundtrip(t *testing.T) {
	s := newTempStore(t)
	t.Run("true", func(t *testing.T) {
		s.Store(ingest.AnalyticsEvent{
			Schema:  "archetipo.analytics/v1",
			Event:   "ok",
			Success: boolPtr(true),
		})
	})
	t.Run("false", func(t *testing.T) {
		s.Store(ingest.AnalyticsEvent{
			Schema:  "archetipo.analytics/v1",
			Event:   "fail",
			Success: boolPtr(false),
		})
	})
	t.Run("nil", func(t *testing.T) {
		s.Store(ingest.AnalyticsEvent{
			Schema: "archetipo.analytics/v1",
			Event:  "unknown",
		})
	})

	events := s.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Success == nil || *events[0].Success != true {
		t.Fatalf("expected success=true, got %v", events[0].Success)
	}
	if events[1].Success == nil || *events[1].Success != false {
		t.Fatalf("expected success=false, got %v", events[1].Success)
	}
	if events[2].Success != nil {
		t.Fatalf("expected success=nil, got %v", events[2].Success)
	}
}

func TestSQLiteStore_NewCreatesDbFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "analytics.db")
	// The driver creates the file; the parent dir must exist though.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := New(path, time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	s.Store(validEvent())
	if s.Len() != 1 {
		t.Fatalf("expected 1 event, got %d", s.Len())
	}
}

func boolPtr(b bool) *bool { return &b }
