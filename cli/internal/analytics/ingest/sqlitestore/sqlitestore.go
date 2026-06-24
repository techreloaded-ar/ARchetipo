// Package sqlitestore implements the ingest.EventStore interface backed by
// a persistent SQLite database. It is imported only by the analytics-server
// binary (cmd/analytics-server) so that the CLI binary (archetipo) stays
// lean and free of the modernc.org/sqlite dependency.
//
// The driver is modernc.org/sqlite (pure Go, no CGO), which keeps the
// cross-compilation matrix (CGO_ENABLED=0) of GoReleaser intact.
package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite" // register the "sqlite" driver

	"github.com/techreloaded-ar/ARchetipo/cli/internal/analytics/ingest"
)

// SQLiteStore implements ingest.EventStore backed by a SQLite database.
// Events survive process restarts and redeploys as long as the database
// file lives on a persistent volume.
type SQLiteStore struct {
	db       *sql.DB
	ttl      time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// dsn builds the SQLite connection string with pragmas tuned for a
// single-writer/multi-reader ingest workload:
//   - journal_mode(WAL): readers don't block the writer, durability ok.
//   - busy_timeout(5000): avoid SQLITE_BUSY under concurrent inserts.
//   - synchronous(NORMAL): safe with WAL, faster than FULL.
func dsn(path string) string {
	return fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", path)
}

// New opens (or creates) the SQLite database at path, initialises the schema,
// and starts the TTL cleanup goroutine. The caller must call Close to release
// the database connection and stop cleanup.
func New(path string, ttl time.Duration) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}
	// SQLite serialises writes; a small pool is enough and avoids spinlock.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{
		db:     db,
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	go s.cleanupLoop()
	return s, nil
}

// initSchema creates the events table and indices if they don't exist.
// args and properties are stored as TEXT (JSON-encoded); the auto-increment
// id preserves insertion order (Events() ORDER BY id ASC).
func (s *SQLiteStore) initSchema() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS events (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    schema                   TEXT    NOT NULL,
    event                    TEXT    NOT NULL,
    timestamp                TEXT,
    command                  TEXT,
    tool                     TEXT,
    tool_version             TEXT,
    os                       TEXT,
    arch                     TEXT,
    archetipo_version        TEXT,
    session_id               TEXT,
    duration_ms              INTEGER,
    success                  INTEGER,
    error_code               TEXT,
    exit_code                INTEGER,
    ci                       INTEGER,
    connector                TEXT,
    anonymous_installation_id TEXT,
    spec_code                TEXT,
    args                     TEXT,
    properties               TEXT,
    received_at              TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_received_at ON events(received_at);
`
	_, err := s.db.Exec(ddl)
	if err != nil {
		return fmt.Errorf("initialising schema: %w", err)
	}
	return nil
}

// Store implements ingest.EventStore. It converts the AnalyticsEvent to a
// StoredEvent (which carries no origin/IP data) and inserts it. The
// received_at column is set server-side to the current UTC time as RFC3339.
func (s *SQLiteStore) Store(event ingest.AnalyticsEvent) {
	se := ingest.StoredEventFromAnalytics(event)
	// Override ReceivedAt with a stable UTC RFC3339 string for indexing.
	receivedAt := se.ReceivedAt.UTC().Format(time.RFC3339Nano)

	argsJSON, _ := json.Marshal(se.Args)
	propsJSON, _ := json.Marshal(se.Properties)

	const q = `INSERT INTO events (
        schema, event, timestamp, command, tool, tool_version, os, arch,
        archetipo_version, session_id, duration_ms, success, error_code,
        exit_code, ci, connector, anonymous_installation_id, spec_code,
        args, properties, received_at
    ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

	var successVal any
	if se.Success != nil {
		successVal = boolToInt(*se.Success)
	}
	_, _ = s.db.Exec(q,
		se.Schema, se.Event, se.Timestamp, se.Command, se.Tool, se.ToolVersion,
		se.OS, se.Arch, se.ArchetipoVersion, se.SessionID, se.DurationMs,
		successVal, se.ErrorCode, se.ExitCode, boolToInt(se.CI), se.Connector,
		se.AnonymousInstallationID, se.SpecCode, string(argsJSON),
		string(propsJSON), receivedAt,
	)
}

// Events implements ingest.EventStore. Returns all non-expired stored events
// in insertion order (ORDER BY id ASC). Expired events are filtered out
// in-memory as a safety net on top of the background cleanup goroutine.
func (s *SQLiteStore) Events() []ingest.StoredEvent {
	cutoff := time.Now().Add(-s.ttl).UTC().Format(time.RFC3339Nano)
	const q = `SELECT
        schema, event, timestamp, command, tool, tool_version, os, arch,
        archetipo_version, session_id, duration_ms, success, error_code,
        exit_code, ci, connector, anonymous_installation_id, spec_code,
        args, properties, received_at
    FROM events WHERE received_at > ? ORDER BY id ASC`

	rows, err := s.db.Query(q, cutoff)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []ingest.StoredEvent
	for rows.Next() {
		se, err := scanEvent(rows)
		if err != nil {
			continue
		}
		out = append(out, se)
	}
	return out
}

// Len implements ingest.EventStore. Returns the count of non-expired events.
func (s *SQLiteStore) Len() int {
	cutoff := time.Now().Add(-s.ttl).UTC().Format(time.RFC3339Nano)
	const q = `SELECT COUNT(*) FROM events WHERE received_at > ?`
	var n int
	_ = s.db.QueryRow(q, cutoff).Scan(&n)
	return n
}

// Close stops the cleanup goroutine and closes the database connection.
// Safe to call multiple times.
func (s *SQLiteStore) Close() error {
	s.stopOnce.Do(func() { close(s.stopCh) })
	return s.db.Close()
}

// cleanupLoop periodically deletes events older than TTL. The interval is
// ttl/10 (capped at a minimum of 1 minute) — same pattern as MemoryStore.
func (s *SQLiteStore) cleanupLoop() {
	interval := s.ttl / 10
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

// cleanup deletes expired events. It uses a short timeout context so a
// slow delete never blocks shutdown.
func (s *SQLiteStore) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cutoff := time.Now().Add(-s.ttl).UTC().Format(time.RFC3339Nano)
	const q = `DELETE FROM events WHERE received_at < ?`
	_, _ = s.db.ExecContext(ctx, q, cutoff)
}

// scanEvent reads a single row from a *sql.Rows into a StoredEvent.
func scanEvent(rows *sql.Rows) (ingest.StoredEvent, error) {
	var se ingest.StoredEvent
	var (
		timestamp   sql.NullString
		command     sql.NullString
		tool        sql.NullString
		toolVersion sql.NullString
		osVal       sql.NullString
		arch        sql.NullString
		archVer     sql.NullString
		sessionID   sql.NullString
		successVal  sql.NullInt64
		errorCode   sql.NullString
		connector   sql.NullString
		anonID      sql.NullString
		specCode    sql.NullString
		argsJSON    sql.NullString
		propsJSON   sql.NullString
		receivedAt  sql.NullString
	)
	if err := rows.Scan(
		&se.Schema, &se.Event, &timestamp, &command, &tool, &toolVersion,
		&osVal, &arch, &archVer, &sessionID, &se.DurationMs, &successVal,
		&errorCode, &se.ExitCode, &se.CI, &connector, &anonID, &specCode,
		&argsJSON, &propsJSON, &receivedAt,
	); err != nil {
		return ingest.StoredEvent{}, err
	}
	se.Timestamp = timestamp.String
	se.Command = command.String
	se.Tool = tool.String
	se.ToolVersion = toolVersion.String
	se.OS = osVal.String
	se.Arch = arch.String
	se.ArchetipoVersion = archVer.String
	se.SessionID = sessionID.String
	if successVal.Valid {
		b := successVal.Int64 != 0
		se.Success = &b
	}
	se.ErrorCode = errorCode.String
	se.Connector = connector.String
	se.AnonymousInstallationID = anonID.String
	se.SpecCode = specCode.String
	if argsJSON.Valid && argsJSON.String != "" {
		_ = json.Unmarshal([]byte(argsJSON.String), &se.Args)
	}
	if propsJSON.Valid && propsJSON.String != "" {
		_ = json.Unmarshal([]byte(propsJSON.String), &se.Properties)
	}
	if receivedAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, receivedAt.String); err == nil {
			se.ReceivedAt = t
		} else {
			se.ReceivedAt = time.Now()
		}
	} else {
		se.ReceivedAt = time.Now()
	}
	return se, nil
}

// boolToInt maps a bool to the INTEGER storage used by SQLite (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
