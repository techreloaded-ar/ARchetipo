package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

func cloneRequest(t *testing.T, srv *Server, url string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	body := `{"url":` + strings.TrimSpace(string(mustJSON(t, url))) + `}`
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/clone", strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneAvailable(t *testing.T, srv *Server) bool {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	var out struct {
		CloneAvailable bool `json:"cloneAvailable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.CloneAvailable
}

func firstFieldCode(t *testing.T, out map[string]any) string {
	t.Helper()
	fields, _ := out["fields"].([]any)
	if len(fields) != 1 {
		t.Fatalf("expected one field error: %v", out)
	}
	field := fields[0].(map[string]any)
	if field["field"] != "url" {
		t.Fatalf("error not on the url field: %v", field)
	}
	return field["code"].(string)
}

func TestCloneIsOfferedOnlyWhereTheViewerDeclaresADirectory(t *testing.T) {
	t.Setenv(workspace.EnvCloneDir, "")
	srv, _ := newWorkspacesTestServer(t, t.TempDir())
	if cloneAvailable(t, srv) {
		t.Fatal("clone offered without a clone directory")
	}
	rec, out := cloneRequest(t, srv, "https://github.com/owner/repo")
	if rec.Code != http.StatusBadRequest || firstFieldCode(t, out) != workspace.CodeCloneNotOffered {
		t.Fatalf("clone without a directory = %d %v", rec.Code, out)
	}

	dir := t.TempDir()
	t.Setenv(workspace.EnvCloneDir, dir)
	if !cloneAvailable(t, srv) {
		t.Fatal("clone not offered with a clone directory")
	}
	rec, out = cloneRequest(t, srv, "https://token@github.com/owner/repo")
	if rec.Code != http.StatusBadRequest || firstFieldCode(t, out) != workspace.CodeCloneURLInvalid {
		t.Fatalf("credential URL = %d %v", rec.Code, out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a refused clone wrote %d entries", len(entries))
	}
}
