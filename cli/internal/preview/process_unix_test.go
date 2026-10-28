//go:build !windows

package preview

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A launcher can exit successfully (or crash) while its HTTP child keeps the
// inherited output handles and listening socket alive, as npm wrappers can do.
func TestSupervisorReapsServerAfterLauncherExit(t *testing.T) {
	testSupervisorStop(t, "launcher-exit")
}

func TestSupervisorReapsStubbornServerAfterOwnerEOF(t *testing.T) {
	testSupervisorStop(t, "owner-eof")
}

func testSupervisorStop(t *testing.T, mode string) {
	t.Helper()
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARCHETIPO_PREVIEW_PROCESS_TEST", root)
	t.Setenv("ARCHETIPO_PREVIEW_PROCESS_MODE", mode)
	request, _ := json.Marshal(commandRequest{Directory: root, Argv: []string{executable, "-test.run=^TestPreviewLauncherHelper$"}})
	input, control := io.Pipe()
	defer func() { _ = input.Close(); _ = control.Close() }()
	done := make(chan error, 1)
	go func() { done <- Supervise(context.Background(), input, io.Discard, io.Discard) }()
	if _, err = fmt.Fprintln(control, string(request)); err != nil {
		t.Fatal(err)
	}
	if mode == "owner-eof" {
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, err = os.Stat(filepath.Join(root, "server.address")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("server did not start")
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = control.Close()
	}
	select {
	case <-done: //nolint:errcheck // Launcher errors are expected; the listening socket is the oracle.
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not finish after launcher exit")
	}
	address, err := os.ReadFile(filepath.Join(root, "server.address"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", string(address), time.Second)
	if err == nil {
		_ = conn.Close()
		t.Fatal("launcher exited but its server is still listening")
	}
}

func TestPreviewLauncherHelper(t *testing.T) {
	root := os.Getenv("ARCHETIPO_PREVIEW_PROCESS_TEST")
	if root == "" {
		return
	}
	executable, _ := os.Executable()
	child := exec.Command(executable, "-test.run=^TestPreviewServerHelper$")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(root, "server.address")); err == nil {
			if os.Getenv("ARCHETIPO_PREVIEW_PROCESS_MODE") == "owner-eof" {
				_ = child.Wait()
			}
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(3)
}

func TestPreviewServerHelper(t *testing.T) {
	root := os.Getenv("ARCHETIPO_PREVIEW_PROCESS_TEST")
	if root == "" {
		return
	}
	if os.Getenv("ARCHETIPO_PREVIEW_PROCESS_MODE") == "owner-eof" {
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	defer func() { _ = listener.Close() }()
	if err = os.WriteFile(filepath.Join(root, "server.address"), []byte(listener.Addr().String()), 0o600); err != nil {
		os.Exit(3)
	}
	// Self-expiry bounds leakage even when running this regression unfixed.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.WriteString(os.Stdout, strings.Repeat("server running\n", 2))
		time.Sleep(20 * time.Millisecond)
	}
}
