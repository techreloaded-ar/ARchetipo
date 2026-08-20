package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/inmemory"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// TestMain redirects the user-level state directory for the whole package.
// NewServer opens the registry, and a successful creation records itself in
// it: without this, running the tests would write the temporary directories of
// this suite into the real registry of whoever runs them. Individual tests
// still replace srv.workspaces with their own, so this is the floor, not the
// isolation they rely on.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "archetipo-web-state-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv(workspace.EnvStateDir, dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// newWorkspacesTestServer builds a server whose known-workspace registry lives
// on a temporary directory, so no test ever touches the real state of the
// machine that runs it.
func newWorkspacesTestServer(t *testing.T, projectRoot string) (*Server, *workspace.Registry) {
	t.Helper()
	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	conn := inmemory.New(cfg)
	srv, err := NewServer(conn, cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	reg := &workspace.Registry{Dir: t.TempDir()}
	srv.workspaces = reg
	return srv, reg
}

// seedWorkspace creates a directory that is a workspace as far as the registry
// is concerned: it holds `.archetipo/config.yaml`.
func seedWorkspace(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.RelativePath), []byte("connector: file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func listWorkspaces(t *testing.T, srv *Server) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Workspaces []map[string]any `json:"workspaces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the list: %v", err)
	}
	return rec, out.Workspaces
}

func postWorkspaces(t *testing.T, srv *Server, payload any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

func deleteWorkspace(t *testing.T, srv *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/workspaces/"+id, nil))
	return rec
}

func TestListWorkspacesEmptyIsNotNull(t *testing.T) {
	srv, _ := newWorkspacesTestServer(t, t.TempDir())
	rec, items := listWorkspaces(t, srv)
	if len(items) != 0 {
		t.Fatalf("workspaces = %d, want 0", len(items))
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"workspaces":[]`)) {
		t.Fatalf("body = %s, want an empty array and not null", rec.Body.String())
	}
}

func TestListWorkspacesReportsNamePathAndLastOpened(t *testing.T) {
	root := t.TempDir()
	current := seedWorkspace(t, root, "corrente")
	other := seedWorkspace(t, root, "altro")
	srv, reg := newWorkspacesTestServer(t, current)

	start := time.Now().Add(-time.Second)
	if _, err := reg.Touch(current); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Touch(other); err != nil {
		t.Fatal(err)
	}

	_, items := listWorkspaces(t, srv)
	if len(items) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(items))
	}
	seen := map[string]map[string]any{}
	for _, item := range items {
		path, _ := item["path"].(string)
		if !filepath.IsAbs(path) {
			t.Fatalf("path = %q, want an absolute path", path)
		}
		if name, _ := item["name"].(string); name == "" {
			t.Fatalf("entry %v has an empty name", item)
		}
		raw, _ := item["lastOpenedAt"].(string)
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t.Fatalf("lastOpenedAt = %q, does not parse: %v", raw, err)
		}
		if at.Before(start) {
			t.Fatalf("lastOpenedAt = %v, want a moment after the test started (%v)", at, start)
		}
		if item["status"] != string(workspace.StatusReachable) || item["reachable"] != true {
			t.Fatalf("entry %v, want a reachable status", item)
		}
		seen[path] = item
	}
	if seen[filepath.Clean(current)]["current"] != true {
		t.Fatalf("the served workspace is not marked current: %v", seen[filepath.Clean(current)])
	}
	if seen[filepath.Clean(other)]["current"] != false {
		t.Fatalf("another workspace is marked current: %v", seen[filepath.Clean(other)])
	}
	if seen[filepath.Clean(current)]["name"] != "corrente" {
		t.Fatalf("name = %v, want %q", seen[filepath.Clean(current)]["name"], "corrente")
	}
}

func TestListWorkspacesKeepsUnreachableEntries(t *testing.T) {
	root := t.TempDir()
	gone := seedWorkspace(t, root, "sparito")
	stripped := seedWorkspace(t, root, "svuotato")
	srv, reg := newWorkspacesTestServer(t, t.TempDir())
	if _, err := reg.Touch(gone); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Touch(stripped); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(gone, filepath.Join(root, "sparito-rinominato")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(stripped, ".archetipo")); err != nil {
		t.Fatal(err)
	}

	_, items := listWorkspaces(t, srv)
	if len(items) != 2 {
		t.Fatalf("workspaces = %d, want 2: an unreachable entry must be reported, not dropped", len(items))
	}
	byPath := map[string]map[string]any{}
	for _, item := range items {
		byPath[item["path"].(string)] = item
	}
	if got := byPath[filepath.Clean(gone)]; got["reachable"] != false || got["status"] != string(workspace.StatusMissing) {
		t.Fatalf("renamed entry = %v, want reachable:false and status:missing", got)
	}
	if got := byPath[filepath.Clean(stripped)]; got["reachable"] != false || got["status"] != string(workspace.StatusNotWorkspace) {
		t.Fatalf("stripped entry = %v, want reachable:false and status:not_a_workspace", got)
	}
}

func TestAddWorkspaceAcceptsAnExistingOne(t *testing.T) {
	root := t.TempDir()
	ws := seedWorkspace(t, root, "esistente")
	srv, _ := newWorkspacesTestServer(t, t.TempDir())

	rec := postWorkspaces(t, srv, map[string]any{"path": ws})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspaces = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var entry map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["id"] == "" || entry["name"] != "esistente" || entry["path"] != filepath.Clean(ws) {
		t.Fatalf("entry = %v, want the added workspace", entry)
	}
	if _, err := time.Parse(time.RFC3339Nano, entry["lastOpenedAt"].(string)); err != nil {
		t.Fatalf("lastOpenedAt = %v, does not parse: %v", entry["lastOpenedAt"], err)
	}
	if entry["reachable"] != true {
		t.Fatalf("reachable = %v, want true", entry["reachable"])
	}

	if _, items := listWorkspaces(t, srv); len(items) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(items))
	}

	// Registering the same path twice updates the entry: the identity is
	// derived from the path, so there is nothing to duplicate.
	if again := postWorkspaces(t, srv, map[string]any{"path": ws}); again.Code != http.StatusCreated {
		t.Fatalf("second POST = %d, want 201: %s", again.Code, again.Body.String())
	}
	if _, items := listWorkspaces(t, srv); len(items) != 1 {
		t.Fatalf("workspaces after the second POST = %d, want 1", len(items))
	}
}

func TestAddWorkspaceRefusals(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "non-workspace")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		code string
	}{
		{"empty", "", workspace.CodeRegistryPathRequired},
		{"relative", "docs", workspace.CodeRegistryPathNotAbsolute},
		{"missing", filepath.Join(root, "non-esiste"), workspace.CodeRegistryPathMissing},
		{"not a workspace", plain, workspace.CodeRegistryNotAWorkspace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newWorkspacesTestServer(t, t.TempDir())
			rec := postWorkspaces(t, srv, map[string]any{"path": tc.path})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("POST = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Error  string `json:"error"`
				Code   string `json:"code"`
				Hint   string `json:"hint"`
				Fields []struct {
					Field   string `json:"field"`
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"fields"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error == "" || body.Code == "" || body.Hint == "" {
				t.Fatalf("body = %s, want error, code and hint", rec.Body.String())
			}
			if len(body.Fields) != 1 {
				t.Fatalf("fields = %v, want exactly one", body.Fields)
			}
			if body.Fields[0].Field != "path" {
				t.Fatalf("field = %q, want %q", body.Fields[0].Field, "path")
			}
			if body.Fields[0].Code != tc.code {
				t.Fatalf("code = %q, want %q", body.Fields[0].Code, tc.code)
			}
		})
	}
}

func TestRemoveWorkspaceLeavesFilesUntouched(t *testing.T) {
	ws := seedWorkspace(t, t.TempDir(), "da-dimenticare")
	srv, reg := newWorkspacesTestServer(t, t.TempDir())
	entry, err := reg.Touch(ws)
	if err != nil {
		t.Fatal(err)
	}
	before := listDirRecursive(t, ws)

	rec := deleteWorkspace(t, srv, entry.ID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("DELETE body = %q, want empty", rec.Body.String())
	}

	_, items := listWorkspaces(t, srv)
	for _, item := range items {
		if item["id"] == entry.ID {
			t.Fatal("the removed entry is still listed")
		}
	}
	if after := listDirRecursive(t, ws); !equalStrings(before, after) {
		t.Fatalf("the workspace tree changed: %v -> %v", before, after)
	}
	if _, err := os.ReadFile(filepath.Join(ws, config.RelativePath)); err != nil {
		t.Fatalf("the workspace config is no longer readable: %v", err)
	}
}

func TestRemoveUnknownWorkspace(t *testing.T) {
	srv, _ := newWorkspacesTestServer(t, t.TempDir())
	rec := deleteWorkspace(t, srv, "does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE unknown = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] == "" || body["code"] == nil {
		t.Fatalf("body = %s, want a code", rec.Body.String())
	}
}

func TestCreateWorkspaceRegistersIt(t *testing.T) {
	useRepoDataDir(t)
	srv, reg := newWorkspacesTestServer(t, t.TempDir())
	dir := filepath.Join(t.TempDir(), "creato")

	rec := postWorkspace(t, srv, map[string]any{
		"dir":       dir,
		"connector": "file",
		"tools":     []string{"pi"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspace = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	entries, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Path == filepath.Clean(dir) {
			found = true
		}
	}
	if !found {
		t.Fatalf("registry = %+v, want an entry for the created workspace %s", entries, dir)
	}

	_, items := listWorkspaces(t, srv)
	listed := false
	for _, item := range items {
		if item["path"] == filepath.Clean(dir) {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("the created workspace is not listed: %v", items)
	}
}

func TestCreateWorkspaceSurvivesAnUnavailableRegistry(t *testing.T) {
	useRepoDataDir(t)
	srv, _ := newWorkspacesTestServer(t, t.TempDir())
	srv.workspaces = nil
	dir := filepath.Join(t.TempDir(), "creato-senza-registro")

	rec := postWorkspace(t, srv, map[string]any{
		"dir":       dir,
		"connector": "file",
		"tools":     []string{"pi"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspace = %d, want 201 even without a registry: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if warning, _ := body["registryWarning"].(string); warning == "" {
		t.Fatalf("body = %s, want a non-empty registryWarning", rec.Body.String())
	}

	// The list stays a 200 with an empty array rather than a 500.
	rec, items := listWorkspaces(t, srv)
	if len(items) != 0 {
		t.Fatalf("workspaces = %d, want 0: %s", len(items), rec.Body.String())
	}
}

func TestRegisterWorkspaceRecordsTheServedRoot(t *testing.T) {
	root := seedWorkspace(t, t.TempDir(), "servito")
	srv, reg := newWorkspacesTestServer(t, root)

	if err := srv.RegisterWorkspace(); err != nil {
		t.Fatalf("RegisterWorkspace() error = %v", err)
	}
	entries, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != filepath.Clean(root) {
		t.Fatalf("registry = %+v, want the served root %s", entries, filepath.Clean(root))
	}

	srv.workspaces = nil
	if err := srv.RegisterWorkspace(); err == nil {
		t.Fatal("RegisterWorkspace() without a registry returned nil, want an error the caller can report")
	}

	empty, emptyReg := newWorkspacesTestServer(t, "")
	if err := empty.RegisterWorkspace(); err != nil {
		t.Fatalf("RegisterWorkspace() without a project root error = %v, want nil", err)
	}
	if entries, err := emptyReg.List(); err != nil || len(entries) != 0 {
		t.Fatalf("registry = %+v (err %v), want nothing written", entries, err)
	}
}
