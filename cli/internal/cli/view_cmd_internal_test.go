package cli

import (
	"net"
	"strconv"
	"testing"
)

func TestFindFreePortSkipsBusy(t *testing.T) {
	// Occupy an arbitrary free port, then ask findFreePort to start there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	got, err := findFreePort("127.0.0.1", busy, 64)
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	if got == busy {
		t.Fatalf("expected a port different from the busy one %d", busy)
	}
	// The returned port must actually be bindable.
	check, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(got)))
	if err != nil {
		t.Fatalf("returned port %d not bindable: %v", got, err)
	}
	_ = check.Close()
}

func TestFindFreePortExhausted(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	// maxTries=1 starting on the busy port leaves no room to fall back.
	if _, err := findFreePort("127.0.0.1", busy, 1); err == nil {
		t.Fatalf("expected error when the only candidate port is busy")
	}
}
