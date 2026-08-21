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

// openWorkspaceFixture builds the situation the story describes: a viewer
// serving A, and B known but not open. The connector is the real file one and
// both workspaces are real directories, because the oracle of every assertion
// below is the backlog that gets served.
type openWorkspaceFixture struct {
	srv  *Server
	reg  *workspace.Registry
	a, b string
	idA  string
	idB  string
}

func newOpenWorkspaceFixture(t *testing.T) openWorkspaceFixture {
	t.Helper()
	a := realWorkspace(t, "alpha", "US-A01", true)
	b := realWorkspace(t, "beta", "US-B01", false)
	srv := serverOn(t, a)
	reg := &workspace.Registry{Dir: t.TempDir()}
	srv.workspaces = reg
	// B is recorded first so that A is the most recently opened one: the test on
	// the last access has then something to observe when B moves to the head.
	for _, root := range []string{b, a} {
		if _, err := reg.Touch(root); err != nil {
			t.Fatal(err)
		}
	}
	return openWorkspaceFixture{srv: srv, reg: reg, a: a, b: b, idA: workspace.EntryID(a), idB: workspace.EntryID(b)}
}

func openWorkspace(t *testing.T, srv *Server, id string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/"+id+"/open", nil))
	return rec
}

func workspaceActions(t *testing.T, srv *Server) workspaceActionsView {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/actions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace/actions = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var view workspaceActionsView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the actions: %v", err)
	}
	return view
}

// lastOpenedOf reads one entry's last access from the list route.
func lastOpenedOf(t *testing.T, srv *Server, id string) (time.Time, int) {
	t.Helper()
	_, entries := listWorkspaces(t, srv)
	for i, e := range entries {
		if e["id"] == id {
			at, err := time.Parse(time.RFC3339Nano, e["lastOpenedAt"].(string))
			if err != nil {
				t.Fatalf("parsing lastOpenedAt: %v", err)
			}
			return at, i
		}
	}
	t.Fatalf("no entry %s in the list", id)
	return time.Time{}, -1
}

// TestOpenWorkspaceSwitchesBoardConfigAndActions is AC-1, AC-2 and AC-3 stated
// on the HTTP contract, in one process that was never restarted.
func TestOpenWorkspaceSwitchesBoardConfigAndActions(t *testing.T) {
	f := newOpenWorkspaceFixture(t)

	if !workspaceActions(t, f.srv).HasPRD {
		t.Fatal("the fixture is wrong: A must have a PRD before the switch")
	}

	rec := openWorkspace(t, f.srv, f.idB)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST open = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var opened map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &opened); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	if opened["current"] != true {
		t.Errorf("current = %v, want true", opened["current"])
	}
	if opened["path"] != f.b {
		t.Errorf("path = %v, want %s", opened["path"], f.b)
	}

	codes := boardCodes(t, f.srv)
	if !codes["US-B01"] || codes["US-A01"] {
		t.Errorf("board after the switch = %v, want only the codes of B", codes)
	}
	if got := specStatus(t, f.srv, "US-A01"); got != http.StatusNotFound {
		t.Errorf("GET /api/spec/US-A01 = %d, want 404", got)
	}
	if got := configPath(t, f.srv); !strings.HasPrefix(got, f.b) {
		t.Errorf("config path = %q, want it under %q", got, f.b)
	}
	if workspaceActions(t, f.srv).HasPRD {
		t.Error("the actions still report a PRD: they were computed on the workspace that was left")
	}
}

// TestOpenWorkspaceUpdatesLastOpened is AC-5, asserted both through the API and
// on the registry file, because the value must survive the process.
func TestOpenWorkspaceUpdatesLastOpened(t *testing.T) {
	f := newOpenWorkspaceFixture(t)
	before, position := lastOpenedOf(t, f.srv, f.idB)
	if position == 0 {
		t.Fatal("the fixture is wrong: B must not already be the most recent entry")
	}

	if rec := openWorkspace(t, f.srv, f.idB); rec.Code != http.StatusOK {
		t.Fatalf("POST open = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	after, position := lastOpenedOf(t, f.srv, f.idB)
	if !after.After(before) {
		t.Errorf("lastOpenedAt = %s, want it after %s", after, before)
	}
	if position != 0 {
		t.Errorf("B is at position %d in the list, want it first", position)
	}

	raw, err := os.ReadFile(filepath.Join(f.reg.Dir, "workspaces.json"))
	if err != nil {
		t.Fatalf("reading the registry file: %v", err)
	}
	var onDisk struct {
		Workspaces []struct {
			ID           string    `json:"id"`
			LastOpenedAt time.Time `json:"lastOpenedAt"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("decoding the registry file: %v", err)
	}
	found := false
	for _, e := range onDisk.Workspaces {
		if e.ID == f.idB {
			found = true
			if !e.LastOpenedAt.After(before) {
				t.Errorf("on disk lastOpenedAt = %s, want it after %s", e.LastOpenedAt, before)
			}
		}
	}
	if !found {
		t.Errorf("no entry %s in %s", f.idB, filepath.Join(f.reg.Dir, "workspaces.json"))
	}
}

func TestOpenWorkspaceUnknownID(t *testing.T) {
	f := newOpenWorkspaceFixture(t)
	rec := openWorkspace(t, f.srv, "0123456789abcdef")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST open on an unknown id = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if codes := boardCodes(t, f.srv); !codes["US-A01"] {
		t.Errorf("board = %v, want A still served", codes)
	}
}

// TestOpenWorkspaceUnreachableKeepsPreviousActive is AC-4: the refusal names its
// reason, and the workspace that was already open stays not merely readable but
// writable.
func TestOpenWorkspaceUnreachableKeepsPreviousActive(t *testing.T) {
	f := newOpenWorkspaceFixture(t)
	if err := os.RemoveAll(filepath.Join(f.b, ".archetipo")); err != nil {
		t.Fatal(err)
	}

	rec := openWorkspace(t, f.srv, f.idB)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("POST open on an unreachable workspace = %d, want 4xx: %s", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decoding the error: %v", err)
	}
	if !strings.Contains(errBody.Error, "not a workspace") {
		t.Errorf("error = %q, want it to name the probed reason", errBody.Error)
	}

	if codes := boardCodes(t, f.srv); !codes["US-A01"] {
		t.Errorf("board = %v, want A still served", codes)
	}
	if got := configPath(t, f.srv); !strings.HasPrefix(got, f.a) {
		t.Errorf("config path = %q, want it still under %q", got, f.a)
	}

	// Readable is not enough: the workspace that was left open must still accept
	// a write.
	payload, err := json.Marshal(validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	write := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/spec", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	f.srv.mux.ServeHTTP(write, req)
	if write.Code != http.StatusCreated && write.Code != http.StatusOK {
		t.Fatalf("POST /api/spec on the workspace still open = %d, want a success: %s", write.Code, write.Body.String())
	}
}

// TestOpenWorkspaceDoesNotTouchRegistryOnFailure keeps the registry honest: a
// workspace that was not opened was not accessed.
func TestOpenWorkspaceDoesNotTouchRegistryOnFailure(t *testing.T) {
	f := newOpenWorkspaceFixture(t)
	before, _ := lastOpenedOf(t, f.srv, f.idB)
	if err := os.RemoveAll(filepath.Join(f.b, ".archetipo")); err != nil {
		t.Fatal(err)
	}

	if rec := openWorkspace(t, f.srv, f.idB); rec.Code < 400 {
		t.Fatalf("POST open = %d, want a refusal: %s", rec.Code, rec.Body.String())
	}

	after, _ := lastOpenedOf(t, f.srv, f.idB)
	if !after.Equal(before) {
		t.Errorf("lastOpenedAt = %s after a failed open, want it unchanged at %s", after, before)
	}
}

// listWorkspacesView reads the whole answer of GET /api/workspaces, not just
// the entries: the identity of the open workspace lives on the envelope, and
// listWorkspaces above deliberately decodes only "workspaces".
func listWorkspacesView(t *testing.T, srv *Server) workspaceListView {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var view workspaceListView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the list view: %v", err)
	}
	return view
}

// TestListWorkspacesNamesTheOpenWorkspace is AC-1: the server names the
// workspace it is serving, and gives its full path, without the page having to
// find the entry marked current in the list.
func TestListWorkspacesNamesTheOpenWorkspace(t *testing.T) {
	root := t.TempDir()
	current := seedWorkspace(t, root, "corrente")
	srv, reg := newWorkspacesTestServer(t, current)
	if _, err := reg.Touch(current); err != nil {
		t.Fatal(err)
	}

	view := listWorkspacesView(t, srv)
	if !view.Open {
		t.Fatalf("open = false, want true: a workspace is being served")
	}
	if view.CurrentName != "corrente" {
		t.Errorf("currentName = %q, want %q", view.CurrentName, "corrente")
	}
	if want := filepath.Clean(current); view.CurrentPath != want {
		t.Errorf("currentPath = %q, want %q", view.CurrentPath, want)
	}
	if !filepath.IsAbs(view.CurrentPath) {
		t.Errorf("currentPath = %q, want an absolute path", view.CurrentPath)
	}
}

// TestListWorkspacesNamesTheOpenWorkspaceWithoutARegistryEntry is AC-1 on the
// degraded case: the name of the workspace being served cannot depend on the
// readability of a list that is about the *other* workspaces. With no entry,
// and with no registry at all, the name falls back to the base of the path —
// which is exactly what Touch would have recorded.
func TestListWorkspacesNamesTheOpenWorkspaceWithoutARegistryEntry(t *testing.T) {
	root := t.TempDir()
	current := seedWorkspace(t, root, "corrente")
	srv, _ := newWorkspacesTestServer(t, current)

	want := filepath.Base(filepath.Clean(current))
	if want == "" {
		t.Fatal("the fixture is wrong: the base name must not be empty")
	}

	view := listWorkspacesView(t, srv)
	if view.CurrentName != want {
		t.Errorf("currentName with an empty registry = %q, want %q", view.CurrentName, want)
	}

	srv.workspaces = nil
	view = listWorkspacesView(t, srv)
	if view.CurrentName != want {
		t.Errorf("currentName with no registry at all = %q, want %q", view.CurrentName, want)
	}
	if !view.Open {
		t.Errorf("open = false with no registry, want true: a workspace is still being served")
	}
}

// TestOpenWorkspaceChangesTheDeclaredIdentity is AC-3, and AC-4 with it: the
// declared identity changes on the process that is already running, not at a
// restart, and it is the list of known workspaces that opened the other one.
func TestOpenWorkspaceChangesTheDeclaredIdentity(t *testing.T) {
	f := newOpenWorkspaceFixture(t)

	before := listWorkspacesView(t, f.srv)
	if want := filepath.Base(filepath.Clean(f.a)); before.CurrentName != want {
		t.Fatalf("the fixture is wrong: currentName = %q, want %q", before.CurrentName, want)
	}

	rec := openWorkspace(t, f.srv, f.idB)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST open = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var opened map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &opened); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	wantName := filepath.Base(filepath.Clean(f.b))
	// The open response already carries name and path: that is what lets the
	// indicator update without asking a second question.
	if opened["name"] != wantName {
		t.Errorf("open response name = %v, want %q", opened["name"], wantName)
	}
	if opened["path"] != f.b {
		t.Errorf("open response path = %v, want %q", opened["path"], f.b)
	}

	after := listWorkspacesView(t, f.srv)
	if after.CurrentName != wantName {
		t.Errorf("currentName after the switch = %q, want %q", after.CurrentName, wantName)
	}
	if want := filepath.Clean(f.b); after.CurrentPath != want {
		t.Errorf("currentPath after the switch = %q, want %q", after.CurrentPath, want)
	}

	current := map[string]bool{}
	for _, entry := range after.Workspaces {
		current[entry.Name] = entry.Current
	}
	if !current[wantName] {
		t.Errorf("the entry of %q is not current after the switch", wantName)
	}
	if leftName := filepath.Base(filepath.Clean(f.a)); current[leftName] {
		t.Errorf("the entry of %q is still current after the switch", leftName)
	}
}
