package preview

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// The isolated runtime publishes the preview through a gateway that forwards
// a fixed range of host-loopback ports to the container. Unset, the preview
// keeps its local behaviour: a loopback listener on any free port.
const (
	EnvBind  = "ARCHETIPO_PREVIEW_BIND"
	EnvPorts = "ARCHETIPO_PREVIEW_PORTS"
)

const maxPortRange = 1024

type originPolicy struct {
	bind        string
	first, last int
}

func (p originPolicy) ranged() bool { return p.first > 0 }

func (p originPolicy) contains(port int) bool {
	return !p.ranged() || (port >= p.first && port <= p.last)
}

// browserHost is the host the person's browser uses. A listener on every
// interface is reached through the gateway on host loopback.
func (p originPolicy) browserHost() string {
	if ip := net.ParseIP(p.bind); ip != nil && ip.IsUnspecified() {
		return "127.0.0.1"
	}
	return p.bind
}

func loadOriginPolicy() (originPolicy, error) {
	policy := originPolicy{bind: "127.0.0.1"}
	if raw := strings.TrimSpace(os.Getenv(EnvBind)); raw != "" {
		if net.ParseIP(raw) == nil {
			return policy, iox.NewPrecondition(EnvBind+" non è un indirizzo IP", "imposta un indirizzo come 127.0.0.1 o 0.0.0.0", nil)
		}
		policy.bind = raw
	}
	raw := strings.TrimSpace(os.Getenv(EnvPorts))
	if raw == "" {
		return policy, nil
	}
	first, last, ok := strings.Cut(raw, "-")
	a, errA := strconv.Atoi(strings.TrimSpace(first))
	b, errB := strconv.Atoi(strings.TrimSpace(last))
	if !ok || errA != nil || errB != nil || a < 1 || b > 65535 || a > b || b-a+1 > maxPortRange {
		return policy, iox.NewPrecondition(EnvPorts+" non è un intervallo di porte valido", fmt.Sprintf("usa la forma 47810-47829, al massimo %d porte", maxPortRange), nil)
	}
	policy.first, policy.last = a, b
	return policy, nil
}

type browserOrigin struct {
	Version int    `json:"version"`
	Root    string `json:"root"`
	Port    int    `json:"port"`
}

func originPath(root string) string {
	return filepath.Join(root, ".archetipo/environment/browser-origin.json")
}

func readOrigin(root string) (browserOrigin, bool, error) {
	var origin browserOrigin
	raw, err := os.ReadFile(originPath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return origin, false, nil
	}
	if err != nil {
		return origin, false, err
	}
	if err = json.Unmarshal(raw, &origin); err != nil || origin.Version != 1 || origin.Root != root || origin.Port < 1 || origin.Port > 65535 {
		return origin, true, iox.NewPrecondition("indirizzo browser della preview non valido", "conserva i dati locali e verifica i metadati browser-origin.json del worktree", err)
	}
	return origin, true, nil
}

// The browser's local storage belongs to an origin, not to the upstream app.
// Keep the checkout's proxy port across stops, revisions and viewer restarts.
// Never fall back to a new origin if that port is occupied: that would make
// retained browser data appear lost. This is local runtime state, not a recipe.
// It returns the listener and the host:port the browser must open.
func listenBrowserOrigin(root string) (net.Listener, string, error) {
	policy, err := loadOriginPolicy()
	if err != nil {
		return nil, "", err
	}
	origin, retained, err := readOrigin(root)
	if err != nil {
		return nil, "", err
	}
	if retained && !policy.contains(origin.Port) {
		return nil, "", iox.NewConflict("indirizzo browser della preview fuori dall’intervallo consentito", fmt.Sprintf("la porta %d conservata non è tra %d e %d; rimuovi browser-origin.json accettando la perdita dei dati del browser", origin.Port, policy.first, policy.last), nil)
	}
	retainedPort := 0
	if retained {
		retainedPort = origin.Port
	}
	var listener net.Listener
	switch {
	case policy.ranged():
		if listener, err = reserveRangedOrigin(root, policy, retainedPort); err != nil {
			return nil, "", err
		}
	case retained:
		if listener, err = listenRetained(policy, retainedPort); err != nil {
			return nil, "", err
		}
	default:
		if listener, err = net.Listen("tcp", net.JoinHostPort(policy.bind, "0")); err != nil {
			return nil, "", iox.NewConflict("indirizzo browser della preview non disponibile", "verifica "+EnvBind+" e riprova", err)
		}
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if !retained {
		origin = browserOrigin{Version: 1, Root: root, Port: port}
		path := originPath(root)
		if err = recordfile.WriteAtomic(filepath.Dir(path), path, ".browser-origin-", origin); err != nil {
			_ = listener.Close()
			return nil, "", err
		}
	}
	return listener, net.JoinHostPort(policy.browserHost(), strconv.Itoa(port)), nil
}

func listenRetained(policy originPolicy, port int) (net.Listener, error) {
	address := net.JoinHostPort(policy.bind, strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, iox.NewConflict("indirizzo browser della preview non disponibile", fmt.Sprintf("libera %s e riprova; l’indirizzo viene conservato per mantenere i dati del browser", address), err)
	}
	return listener, nil
}

// A short range is shared by every checkout of the environment, so a free port
// may still be the retained origin of a stopped checkout. Reservations live in
// the user-level state directory and are released only when their checkout no
// longer retains that port.
const portReservations = "preview-ports.json"

type reservationFile struct {
	Version int               `json:"version"`
	Ports   map[string]string `json:"ports"`
}

func reserveRangedOrigin(root string, policy originPolicy, retainedPort int) (net.Listener, error) {
	dir, err := workspace.StateDir()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	unlock, err := lockReservations(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	path := filepath.Join(dir, portReservations)
	file := reservationFile{Version: 1, Ports: map[string]string{}}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		if json.Unmarshal(raw, &file) != nil || file.Version != 1 || file.Ports == nil {
			return nil, iox.NewPrecondition("prenotazioni delle porte della preview non valide", "verifica o rimuovi "+path, nil)
		}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return nil, readErr
	}
	for key, owner := range file.Ports {
		if owner == root || !retains(owner, key) {
			delete(file.Ports, key)
		}
	}
	if retainedPort > 0 {
		key := strconv.Itoa(retainedPort)
		if owner, taken := file.Ports[key]; taken {
			return nil, iox.NewConflict("indirizzo browser della preview assegnato a un altro checkout", fmt.Sprintf("la porta %d è conservata anche da %s; rimuovi browser-origin.json di uno dei due accettando la perdita dei dati del browser", retainedPort, owner), nil)
		}
		listener, err := listenRetained(policy, retainedPort)
		if err != nil {
			return nil, err
		}
		file.Ports[key] = root
		if err = recordfile.WriteAtomic(dir, path, ".preview-ports-", file); err != nil {
			_ = listener.Close()
			return nil, err
		}
		return listener, nil
	}
	for port := policy.first; port <= policy.last; port++ {
		key := strconv.Itoa(port)
		if _, taken := file.Ports[key]; taken {
			continue
		}
		listener, listenErr := net.Listen("tcp", net.JoinHostPort(policy.bind, key))
		if listenErr != nil {
			continue
		}
		file.Ports[key] = root
		if err = recordfile.WriteAtomic(dir, path, ".preview-ports-", file); err != nil {
			_ = listener.Close()
			return nil, err
		}
		return listener, nil
	}
	return nil, iox.NewConflict("nessuna porta libera per la preview", fmt.Sprintf("tutte le porte tra %d e %d sono assegnate ad altri checkout; ferma o rimuovi un incremento e riprova", policy.first, policy.last), nil)
}

func retains(root, port string) bool {
	origin, retained, err := readOrigin(root)
	return err == nil && retained && strconv.Itoa(origin.Port) == port
}

// Viewer and CLI may start previews from different processes. A directory is
// created atomically; a holder that died leaves it behind, so an old lock is
// taken over after a bounded wait.
func lockReservations(dir string) (func(), error) {
	path := filepath.Join(dir, ".preview-ports.lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := os.Mkdir(path, 0o700); err == nil {
			return func() { _ = os.Remove(path) }, nil
		} else if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > 30*time.Second {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, iox.NewConflict("prenotazioni delle porte della preview occupate", "riprova tra qualche secondo", nil)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
