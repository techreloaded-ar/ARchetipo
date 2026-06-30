package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
)

func TestGetConfigMissing(t *testing.T) {
	srv, cfg := newFileServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	srv.mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var got configView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Exists {
		t.Fatal("expected exists=false for missing config")
	}
	if got.Path != filepath.Join(cfg.ProjectRoot, config.RelativePath) {
		t.Fatalf("path = %q, want %q", got.Path, filepath.Join(cfg.ProjectRoot, config.RelativePath))
	}
	if !strings.Contains(got.Raw, "connector: file") {
		t.Fatalf("expected rendered default YAML, got:\n%s", got.Raw)
	}
}

func TestGetConfigPresent(t *testing.T) {
	srv, cfg := newFileServer(t)
	mustWriteConfig(t, cfg.ProjectRoot, "connector: github\n")

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	srv.mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var got configView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Exists {
		t.Fatal("expected exists=true")
	}
	if got.Config.Connector != config.ConnectorGitHub {
		t.Fatalf("connector = %q, want %q", got.Config.Connector, config.ConnectorGitHub)
	}
	if got.Raw != "connector: github\n" {
		t.Fatalf("raw = %q", got.Raw)
	}
}

func TestSaveConfigStructuredCreatesBackup(t *testing.T) {
	srv, cfg := newFileServer(t)
	mustWriteConfig(t, cfg.ProjectRoot, "connector: file\n")

	payload := saveConfigReq{Config: func() *config.Config {
		c := config.Default()
		c.ProjectRoot = cfg.ProjectRoot
		c.Paths.PRD = "docs/PRD-2.md"
		c.E2E.RecordDemoVideo = true
		return &c
	}()}
	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	srv.mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var got configView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.RestartRequired {
		t.Fatal("expected restart_required=true after config change")
	}
	if got.BackupPath == "" {
		t.Fatal("expected backup_path on overwrite")
	}
	backup, err := os.ReadFile(got.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "connector: file\n" {
		t.Fatalf("backup mismatch: %q", string(backup))
	}
	raw, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, config.RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"prd: docs/PRD-2.md", "record_demo_video: true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config missing %q:\n%s", want, text)
		}
	}
}

func TestSaveConfigRejectsInvalidRaw(t *testing.T) {
	srv, cfg := newFileServer(t)
	mustWriteConfig(t, cfg.ProjectRoot, "connector: file\n")

	body := []byte(`{"raw":"connector: [\n"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	srv.mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, config.RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "connector: file\n" {
		t.Fatalf("config changed after failed save: %q", string(raw))
	}
}

func TestTestConfigFileConnector(t *testing.T) {
	srv, _ := newFileServer(t)
	body := []byte(`{"raw":"connector: file\n"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/config/test", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	srv.mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var got configTestResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatal("expected ok=true")
	}
	if got.Info == nil || got.Info.Connector != config.ConnectorFile {
		t.Fatalf("unexpected connector info: %+v", got.Info)
	}
}

func mustWriteConfig(t *testing.T, root, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.RelativePath), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}
