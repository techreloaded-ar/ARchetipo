// Package viewreg is a small on-disk registry of running `archetipo view`
// servers. Each viewer writes a pidfile (one JSON file per port) into a
// shared user-level directory when it starts listening, and removes it on
// exit. This lets `archetipo view list` enumerate the running viewers and
// `archetipo view stop` terminate them, across projects and OSes.
package viewreg

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnvRunDir overrides the registry directory. Primarily used by tests to
// avoid touching the real user cache directory.
const EnvRunDir = "ARCHETIPO_RUN_DIR"

// Entry is the content of a single pidfile.
type Entry struct {
	PID         int       `json:"pid"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	ProjectRoot string    `json:"projectRoot"`
	StartedAt   time.Time `json:"startedAt"`
}

// Dir returns the registry directory, creating it if necessary. It mirrors the
// version notifier's cache location with a dedicated `run/` subfolder:
//
//	macOS   ~/Library/Caches/archetipo/run/
//	Linux   $XDG_CACHE_HOME/archetipo/run/  (or ~/.cache/archetipo/run/)
//	Windows %LocalAppData%\archetipo\run\
func Dir() (string, error) {
	dir, err := baseDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func baseDir() (string, error) {
	if override := os.Getenv(EnvRunDir); override != "" {
		return override, nil
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Caches", "archetipo", "run"), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "archetipo", "run"), nil
}

func pidfilePath(dir string, port int) string {
	return filepath.Join(dir, strconv.Itoa(port)+".json")
}

// Register writes the pidfile for e and returns its path.
func Register(e Entry) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	path := pidfilePath(dir, e.Port)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Remove deletes the pidfile for port. It is idempotent: a missing file is not
// an error.
func Remove(port int) error {
	dir, err := baseDir()
	if err != nil {
		return err
	}
	if err := os.Remove(pidfilePath(dir, port)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Read returns the entry for port. The error wraps os.ErrNotExist when no
// pidfile exists, so callers can detect it with errors.Is.
func Read(port int) (Entry, error) {
	dir, err := baseDir()
	if err != nil {
		return Entry{}, err
	}
	return readFile(pidfilePath(dir, port))
}

func readFile(path string) (Entry, error) {
	var e Entry
	b, err := os.ReadFile(path)
	if err != nil {
		return e, err
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return e, err
	}
	return e, nil
}

// List returns all registered entries, sorted by port. Corrupt or unreadable
// pidfiles are skipped rather than failing the whole listing.
func List() ([]Entry, error) {
	dir, err := baseDir()
	if err != nil {
		return nil, err
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		e, err := readFile(name)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Port < entries[j].Port })
	return entries, nil
}

// IsAlive reports whether a viewer for e still answers on its port. This is a
// portable liveness check (a simple TCP dial to the loopback interface): it
// avoids Signal(0), which is unsupported on Windows, and sidesteps PID reuse.
func IsAlive(e Entry) bool {
	host := e.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(e.Port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Prune removes pidfiles whose viewer is no longer alive and returns the
// entries that survived.
func Prune(entries []Entry) []Entry {
	alive := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if IsAlive(e) {
			alive = append(alive, e)
			continue
		}
		_ = Remove(e.Port)
	}
	return alive
}

// Stop terminates the viewer registered on port and removes its pidfile. It
// returns the entry it acted on. The error wraps os.ErrNotExist when no viewer
// is registered on that port.
func Stop(port int) (Entry, error) {
	e, err := Read(port)
	if err != nil {
		return Entry{}, err
	}
	if err := terminate(e.PID); err != nil {
		// Best effort: still drop the pidfile so the registry stays coherent
		// (e.g. the process is already gone).
		_ = Remove(port)
		return e, fmt.Errorf("terminating pid %d: %w", e.PID, err)
	}
	if err := Remove(port); err != nil {
		return e, err
	}
	return e, nil
}

// Since renders a compact human-readable age like "2m" or "1h3m".
func Since(t, now time.Time) string {
	d := now.Sub(t)
	if d < time.Second {
		return "just now"
	}
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return strings.TrimSpace(fmt.Sprintf("%ds ago", int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		return fmt.Sprintf("%dh%dm ago", h, m)
	}
}
