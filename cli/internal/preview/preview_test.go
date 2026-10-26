package preview

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecipeRejectsEscapesAndExternalTargets(t *testing.T) {
	root := t.TempDir()
	for _, r := range []Recipe{
		{Version: 1, Directory: "../escape", URL: "http://127.0.0.1:3000"},
		{Version: 1, Directory: ".", URL: "http://example.org"},
		{Version: 1, Directory: ".", URL: "http://user:password@localhost:3000"},
		{Version: 1, Directory: ".", URL: "https://localhost:3000"},
		{Version: 1, Directory: ".", URL: "http://localhost.evil.test:3000"},
		{Version: 1, Directory: ".", URL: "http://127.0.0.1:{port}"},
		{Version: 1, Directory: ".", URL: "http://127.0.0.1:3000/{port}", Argv: []string{"server", "{port}"}},
		{Version: 1, Directory: ".", URL: "http://127.0.0.1:1/{port}", Argv: []string{"server", "{port}"}},
		{Version: 1, Directory: ".", URL: "http://127.0.0.1:3000", Argv: []string{"server", "{port}"}},
		{Version: 1, Directory: ".", URL: "http://127.0.0.1:3000", Simulated: []string{" "}},
		{Version: 1, Directory: ".", URL: "http://127.0.0.1:3000", Simulated: []string{"google", "google"}},
	} {
		if _, err := r.Validate(root); err == nil {
			t.Fatalf("accepted unsafe recipe: %+v", r)
		}
	}
	raw := []byte(`{"version":1,"directory":".","argv":["npm","run","dev"],"url":"http://localhost:3000/path"}`)
	if _, err := Configure(root, raw); err != nil {
		t.Fatal(err)
	}
	r, err := Load(root)
	if err != nil || r.URL != "http://localhost:3000/path" {
		t.Fatalf("round trip: %+v %v", r, err)
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err == nil {
		r.Directory = "outside"
		if _, err = r.Validate(root); err == nil {
			t.Fatal("accepted escaping symlink")
		}
	}
	if _, err = Configure(root, []byte(`{"version":1,"directory":".","url":"http://localhost","unknown":true}`)); err == nil {
		t.Fatal("accepted unknown fields")
	}
}

func TestPortableRecipeResolvesOnlyAtStartup(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"version":1,"directory":".","argv":["server","--port={port}"],"url":"http://127.0.0.1:{port}/page","prepare_environment":true}`)
	if _, err := Configure(root, raw); err != nil {
		t.Fatal(err)
	}
	recipe, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, owned, err := recipe.resolvePort()
	if err != nil || !owned || strings.Contains(resolved.URL, "{port}") || !resolved.PrepareEnvironment {
		t.Fatalf("unresolved recipe: %+v %v", resolved, err)
	}
	u, err := url.Parse(resolved.URL)
	if err != nil || u.Port() == "" || resolved.Argv[1] != "--port="+u.Port() || u.Path != "/page" {
		t.Fatalf("mismatched launch arguments: %+v %v", resolved, err)
	}
	persisted, err := Load(root)
	if err != nil || persisted.URL != recipe.URL || recipe.Argv[1] != "--port={port}" {
		t.Fatal("startup mutated reusable recipe", err)
	}
}

func TestPreparationFailureNeverStartsPreview(t *testing.T) {
	root := t.TempDir()
	if _, err := Configure(root, []byte(`{"version":1,"directory":".","argv":["must-not-run","{port}"],"url":"http://127.0.0.1:{port}","prepare_environment":true}`)); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Close()
	view, err := manager.Start(root, "missing-environment", "")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for view.State != "error" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		view, _ = manager.Status(root, "missing-environment")
	}
	if view.State != "error" || !strings.Contains(view.Error, "no project environment recipe") || view.Log != "" {
		t.Fatalf("did not stop at missing setup: %+v", view)
	}
}

func TestBorrowedServerProxyAndOwnership(t *testing.T) {
	var upstreamPath string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><body><button data-testid="save">Save</button></body></html>`)
	}))
	defer app.Close()
	root := t.TempDir()
	raw, _ := json.Marshal(Recipe{Version: 1, Directory: ".", URL: app.URL + "/nested"})
	if _, err := Configure(root, raw); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	defer m.Close()
	view, err := m.Start(root, "conversation-a", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for view.State == "starting" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		view, _ = m.Status(root, "conversation-a")
	}
	if view.State != "ready" || !view.Reused {
		t.Fatalf("not ready/reused: %+v", view)
	}
	response, err := http.Get(view.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if upstreamPath != "/nested" || !strings.Contains(string(body), "/__archetipo_picker.js?token="+view.Token) {
		t.Fatalf("missing page or picker: %s %s", upstreamPath, body)
	}
	response, err = http.Get(strings.TrimSuffix(view.URL, "/nested") + "/__archetipo_picker.js?token=" + view.Token)
	if err != nil {
		t.Fatal(err)
	}
	script, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !strings.Contains(string(script), "http://127.0.0.1:8080") || !strings.Contains(string(script), "e.source!==parent") {
		t.Fatal("picker channel is not bound to parent")
	}
	another := NewManager()
	defer another.Close()
	if _, err = another.Start(root, "conversation-a", "http://127.0.0.1:8080"); err == nil {
		t.Fatal("a second viewer took ownership")
	}
	if err := another.StopOwned(root); err != nil {
		t.Fatal(err)
	}
	if err := m.StopOwned(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if status, err := m.Status(root, "conversation-a"); err != nil || status.State != "ready" {
		t.Fatalf("stopped an unrelated preview: %+v %v", status, err)
	}
	if err := m.StopOwned(root); err != nil {
		t.Fatal(err)
	}
	if status, err := m.Status(root, "conversation-a"); err != nil || status.State != "stopped" {
		t.Fatalf("owned preview still active: %+v %v", status, err)
	}
	if !ready(context.Background(), app.URL) {
		t.Fatal("stopping borrowed preview killed the app")
	}
}

func TestUnconfiguredAndNoServer(t *testing.T) {
	root := t.TempDir()
	m := NewManager()
	defer m.Close()
	view, err := m.Status(root, "a")
	if err != nil || view.State != "unconfigured" {
		t.Fatalf("%+v %v", view, err)
	}
	// A just-closed httptest listener provides a known unavailable local address.
	app := httptest.NewServer(http.NotFoundHandler())
	url := app.URL
	app.Close()
	raw, _ := json.Marshal(Recipe{Version: 1, Directory: ".", URL: url})
	if _, err = Configure(root, raw); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(root, "a", "http://localhost"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		view, _ = m.Status(root, "a")
		if view.State == "error" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if view.State != "error" || view.Error == "" {
		t.Fatalf("failure not reported: %+v", view)
	}
}

func TestProxyUpgradesAndClosesBorrowedWebSocket(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/socket" {
			w.WriteHeader(200)
			return
		}
		conn, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = buffer.Flush()
		_, _ = io.Copy(conn, buffer)
	}))
	defer app.Close()
	root := t.TempDir()
	raw, _ := json.Marshal(Recipe{Version: 1, Directory: ".", URL: app.URL})
	if _, err := Configure(root, raw); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Close()
	view, err := manager.Start(root, "ws", "http://localhost")
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().Add(3 * time.Second)
	for view.State == "starting" && time.Now().Before(end) {
		time.Sleep(10 * time.Millisecond)
		view, _ = manager.Status(root, "ws")
	}
	parsed, _ := url.Parse(view.URL)
	conn, err := net.Dial("tcp", parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = fmt.Fprintf(conn, "GET /socket HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n", parsed.Host)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 101 {
		t.Fatalf("upgrade: %d", response.StatusCode)
	}
	_, _ = io.WriteString(conn, "ping\n")
	line, err := reader.ReadString('\n')
	if err != nil || line != "ping\n" {
		t.Fatalf("websocket tunnel: %q %v", line, err)
	}
	if err := manager.Stop("ws"); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadByte(); err == nil {
		t.Fatal("WebSocket outlived preview stop")
	}
	if !ready(context.Background(), app.URL) {
		t.Fatal("borrowed app was terminated")
	}
}

func TestPendingResourcesRemainVisibleWithoutPreviewRecipe(t *testing.T) {
	root := t.TempDir()
	command := verification.Command{Name: "resource-command", Argv: []string{"unused-adapter"}, TimeoutSeconds: 1}
	resources := verification.ResourceState{Version: 1, Owner: "owned-fixture", Ports: map[string]string{"DB": "12345"}, Resources: []verification.ResourceRecord{{Resource: verification.Resource{Name: "db", Start: command, Probe: command, Stop: command, Stopped: command}, State: "starting"}}}
	raw, err := json.Marshal(resources)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".archetipo", "environment")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "resources.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Close()
	view, err := manager.Status(root, "after-restart")
	if err != nil || view.State != "error" || view.Resources == nil || view.Resources.Owner != "owned-fixture" || view.Error == "" {
		t.Fatalf("lost recovery state: %+v %v", view, err)
	}
}
