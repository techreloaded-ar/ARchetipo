package preview

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

func TestBrowserOriginRejectsInvalidOrCopiedRuntimeMetadata(t *testing.T) {
	root := t.TempDir()
	for _, value := range []any{
		map[string]any{"version": 1, "root": root, "port": 0},
		map[string]any{"version": 1, "root": root, "port": 65536},
		map[string]any{"version": 2, "root": root, "port": 3000},
		map[string]any{"version": 1, "root": "another-checkout", "port": 3000},
		"invalid metadata",
	} {
		path := filepath.Join(root, ".archetipo/environment/browser-origin.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if listener, _, err := listenBrowserOrigin(root); err == nil {
			_ = listener.Close()
			t.Fatalf("accepted unsafe runtime origin: %s", raw)
		}
	}
}

func freeRange(t *testing.T, size int) (int, int) {
	t.Helper()
	for first := 41000; first < 60000; first += size {
		ok := true
		for port := first; port < first+size && ok; port++ {
			l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				ok = false
				continue
			}
			_ = l.Close()
		}
		if ok {
			return first, first + size - 1
		}
	}
	t.Fatal("no free port range")
	return 0, 0
}

func originPort(t *testing.T, address string) int {
	t.Helper()
	_, raw, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(raw)
	return port
}

func TestBrowserOriginLocalDefaultsKeepLoopbackAndRetainedPort(t *testing.T) {
	t.Setenv(EnvBind, "")
	t.Setenv(EnvPorts, "")
	root := t.TempDir()
	listener, address, err := listenBrowserOrigin(root)
	if err != nil {
		t.Fatal(err)
	}
	if host := listener.Addr().(*net.TCPAddr).IP.String(); host != "127.0.0.1" || address != listener.Addr().String() {
		t.Fatalf("listener %s, browser address %s", listener.Addr(), address)
	}
	_ = listener.Close()
	again, second, err := listenBrowserOrigin(root)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if second != address {
		t.Fatalf("retained origin changed from %s to %s", address, second)
	}
}

func TestBrowserOriginOnEveryInterfaceIsOpenedOnHostLoopback(t *testing.T) {
	t.Setenv(EnvBind, "0.0.0.0")
	t.Setenv(EnvPorts, "")
	listener, address, err := listenBrowserOrigin(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if !listener.Addr().(*net.TCPAddr).IP.IsUnspecified() {
		t.Fatalf("listener bound to %s", listener.Addr())
	}
	if host, _, _ := net.SplitHostPort(address); host != "127.0.0.1" {
		t.Fatalf("browser address %s", address)
	}
}

func TestBrowserOriginRangeNeverHandsOutAnotherCheckoutsRetainedPort(t *testing.T) {
	first, last := freeRange(t, 2)
	t.Setenv(workspace.EnvStateDir, t.TempDir())
	t.Setenv(EnvBind, "127.0.0.1")
	t.Setenv(EnvPorts, fmt.Sprintf("%d-%d", first, last))
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()

	la, addressA, err := listenBrowserOrigin(a)
	if err != nil {
		t.Fatal(err)
	}
	portA := originPort(t, addressA)
	if portA < first || portA > last {
		t.Fatalf("port %d outside %d-%d", portA, first, last)
	}
	// A stopped checkout keeps its origin: its port is free but not available.
	_ = la.Close()
	lb, addressB, err := listenBrowserOrigin(b)
	if err != nil {
		t.Fatal(err)
	}
	if originPort(t, addressB) == portA {
		t.Fatalf("checkout b received the retained origin of checkout a (%d)", portA)
	}
	if _, _, err = listenBrowserOrigin(c); err == nil {
		t.Fatal("a third checkout received a port from an exhausted range")
	}
	_ = lb.Close()

	again, addressAgain, err := listenBrowserOrigin(a)
	if err != nil {
		t.Fatal(err)
	}
	_ = again.Close()
	if addressAgain != addressA {
		t.Fatalf("checkout a moved from %s to %s", addressA, addressAgain)
	}

	// Removing a checkout releases its port.
	if err = os.RemoveAll(a); err != nil {
		t.Fatal(err)
	}
	lc, addressC, err := listenBrowserOrigin(c)
	if err != nil {
		t.Fatalf("released port not reused: %v", err)
	}
	defer lc.Close()
	if originPort(t, addressC) != portA {
		t.Fatalf("checkout c got %s, want the released port %d", addressC, portA)
	}
}

func TestBrowserOriginRejectsRetainedPortOutsideRangeAndInvalidPolicy(t *testing.T) {
	t.Setenv(workspace.EnvStateDir, t.TempDir())
	t.Setenv(EnvBind, "")
	t.Setenv(EnvPorts, "")
	root := t.TempDir()
	listener, address, err := listenBrowserOrigin(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	port := originPort(t, address)
	other := port + 10
	if other > 65535 {
		other = port - 10
	}
	t.Setenv(EnvPorts, fmt.Sprintf("%d-%d", other, other))
	if l, _, err := listenBrowserOrigin(root); err == nil {
		_ = l.Close()
		t.Fatal("a retained origin outside the range was replaced or accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".archetipo/environment/browser-origin.json"))
	var kept map[string]any
	if json.Unmarshal(raw, &kept) != nil || int(kept["port"].(float64)) != port {
		t.Fatalf("retained origin rewritten: %s", raw)
	}

	for _, policy := range [][2]string{{"localhost", ""}, {"", "10"}, {"", "20-10"}, {"", "0-10"}, {"", "1-70000"}, {"", "1000-3000"}} {
		t.Setenv(EnvBind, policy[0])
		t.Setenv(EnvPorts, policy[1])
		if l, _, err := listenBrowserOrigin(t.TempDir()); err == nil {
			_ = l.Close()
			t.Fatalf("accepted bind %q ports %q", policy[0], policy[1])
		}
	}
}
