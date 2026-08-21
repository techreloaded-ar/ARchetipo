package cli

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/viewreg"
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

// makeWorkspace turns dir into a workspace by writing the minimal
// .archetipo/config.yaml that config.FindRoot looks for.
func makeWorkspace(t *testing.T, dir string) {
	t.Helper()
	cfgPath := filepath.Join(dir, config.RelativePath)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("connector: file\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestResolveViewTargetOutsideWorkspace(t *testing.T) {
	// A bare temporary directory has no .archetipo/config.yaml anywhere up the
	// tree that the test controls, so the command must choose the home.
	root, home, err := resolveViewTarget(t.TempDir())
	if err != nil {
		t.Fatalf("resolveViewTarget: %v", err)
	}
	if !home {
		t.Fatalf("want home=true outside a workspace, got false (root %q)", root)
	}
	if root != "" {
		t.Fatalf("want no root outside a workspace, got %q", root)
	}
}

func TestResolveViewTargetInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	makeWorkspace(t, ws)
	deep := filepath.Join(ws, "docs", "specs", "nested")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, cwd := range []string{ws, deep} {
		root, home, err := resolveViewTarget(cwd)
		if err != nil {
			t.Fatalf("resolveViewTarget(%s): %v", cwd, err)
		}
		if home {
			t.Fatalf("want home=false from %s, got true", cwd)
		}
		if root != ws {
			t.Fatalf("want root %q from %s, got %q", ws, cwd, root)
		}
	}
}

func TestViewListRendersMissingProjectRoot(t *testing.T) {
	t.Setenv(viewreg.EnvRunDir, t.TempDir())
	// The listing prunes viewers that no longer answer, so the entry needs a
	// real listener behind its port to survive as far as the rendering.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if _, err := viewreg.Register(viewreg.Entry{
		PID:         os.Getpid(),
		Host:        "127.0.0.1",
		Port:        port,
		ProjectRoot: "",
		StartedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	var out, errBuf bytes.Buffer
	cmd := newViewListCmd(streams{in: strings.NewReader(""), out: &out, err: &errBuf})
	cmd.SetArgs(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("view list: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "(no workspace)") {
		t.Fatalf("want the listing to name the missing workspace, got:\n%s", got)
	}
	// The port line must not carry an all-blank PROJECT column.
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if !strings.HasPrefix(line, strconv.Itoa(port)) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			t.Fatalf("viewer line has an empty PROJECT column: %q", line)
		}
	}
}
