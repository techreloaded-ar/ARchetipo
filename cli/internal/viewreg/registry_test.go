package viewreg

import (
	"errors"
	"io/fs"
	"net"
	"testing"
	"time"
)

func useTempRegistry(t *testing.T) {
	t.Helper()
	t.Setenv(EnvRunDir, t.TempDir())
}

func TestRegisterListRoundtrip(t *testing.T) {
	useTempRegistry(t)
	start := time.Now()
	if _, err := Register(Entry{PID: 111, Host: "127.0.0.1", Port: 8090, ProjectRoot: "/p/a", StartedAt: start}); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if _, err := Register(Entry{PID: 222, Host: "127.0.0.1", Port: 8081, ProjectRoot: "/p/b", StartedAt: start}); err != nil {
		t.Fatalf("register b: %v", err)
	}
	entries, err := List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	// Sorted by port.
	if entries[0].Port != 8081 || entries[1].Port != 8090 {
		t.Fatalf("unexpected order: %d, %d", entries[0].Port, entries[1].Port)
	}
	if entries[1].PID != 111 || entries[1].ProjectRoot != "/p/a" {
		t.Fatalf("roundtrip mismatch: %+v", entries[1])
	}
}

func TestRemoveIdempotent(t *testing.T) {
	useTempRegistry(t)
	if _, err := Register(Entry{PID: 1, Port: 9000}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := Remove(9000); err != nil {
		t.Fatalf("first remove: %v", err)
	}
	if err := Remove(9000); err != nil {
		t.Fatalf("second remove should be a no-op, got: %v", err)
	}
}

func TestReadNotExist(t *testing.T) {
	useTempRegistry(t)
	_, err := Read(1234)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist, got %v", err)
	}
}

func TestIsAliveAndPrune(t *testing.T) {
	useTempRegistry(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	live := Entry{PID: 1, Host: "127.0.0.1", Port: port}
	if !IsAlive(live) {
		t.Fatalf("expected live entry on port %d", port)
	}
	// A port nobody listens on: close the listener and probe it.
	_ = ln.Close()
	dead := Entry{PID: 2, Host: "127.0.0.1", Port: port}
	if IsAlive(dead) {
		t.Fatalf("expected dead entry after closing listener on port %d", port)
	}

	// Prune should drop the dead one and keep the live one.
	reopened, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relisten: %v", err)
	}
	defer reopened.Close()
	alivePort := reopened.Addr().(*net.TCPAddr).Port
	if _, err := Register(Entry{PID: 3, Host: "127.0.0.1", Port: alivePort}); err != nil {
		t.Fatalf("register alive: %v", err)
	}
	if _, err := Register(Entry{PID: 4, Host: "127.0.0.1", Port: port}); err != nil {
		t.Fatalf("register dead: %v", err)
	}
	all, _ := List()
	kept := Prune(all)
	if len(kept) != 1 || kept[0].Port != alivePort {
		t.Fatalf("prune result unexpected: %+v", kept)
	}
	// The dead pidfile must be gone from disk.
	if _, err := Read(port); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dead pidfile should be pruned, got %v", err)
	}
}

func TestStopNotExist(t *testing.T) {
	useTempRegistry(t)
	_, err := Stop(4321)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist, got %v", err)
	}
}

func TestSince(t *testing.T) {
	now := time.Unix(10000, 0)
	cases := []struct {
		delta time.Duration
		want  string
	}{
		{200 * time.Millisecond, "just now"},
		{5 * time.Second, "5s ago"},
		{90 * time.Second, "1m ago"},
		{2 * time.Hour, "2h0m ago"},
	}
	for _, c := range cases {
		got := Since(now.Add(-c.delta), now)
		if got != c.want {
			t.Errorf("Since(-%s) = %q, want %q", c.delta, got, c.want)
		}
	}
}
