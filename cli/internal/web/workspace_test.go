package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// useRepoDataDir points the initialization at the source repository, so the
// real skills and the real packaged config template are installed.
func useRepoDataDir(t *testing.T) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "skills")); statErr != nil {
		t.Skipf("skipped: the repository skills directory is not available (%v)", statErr)
	}
	t.Setenv("ARCHETIPO_DATA_DIR", root)
}

func getWorkspaceOptions(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/options", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace/options = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the options: %v", err)
	}
	return out
}

func postWorkspace(t *testing.T, srv *Server, payload any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workspace", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

func TestWorkspaceOptionsExposeTheAcceptedChoices(t *testing.T) {
	srv, _ := newTestServer(t)
	body := getWorkspaceOptions(t, srv)

	connectors, _ := body["connectors"].([]any)
	if len(connectors) != len(workspace.Connectors()) {
		t.Fatalf("connectors = %v, want %d entries", body["connectors"], len(workspace.Connectors()))
	}
	toolEntries, _ := body["tools"].([]any)
	if len(toolEntries) != len(workspace.Tools()) {
		t.Fatalf("tools = %v, want %d entries", body["tools"], len(workspace.Tools()))
	}
	// The Archetype is reported as one identity, never as a list: with a single
	// installed process there is nothing to choose.
	tpl, ok := body["template"].(map[string]any)
	if !ok {
		t.Fatalf("template = %v, want a single object", body["template"])
	}
	if tpl["id"] != template.DefaultID {
		t.Fatalf("template id = %v, want %q", tpl["id"], template.DefaultID)
	}
	if _, hasPaths := body["paths"]; !hasPaths {
		t.Fatal("the options carry no default paths")
	}
	if _, hasWorktree := body["worktree"]; !hasWorktree {
		t.Fatal("the options carry no default worktree settings")
	}
}

// Every tool the options offer must be one the creation route accepts:
// "offered" and "accepted" being the same list is the whole contract.
func TestWorkspaceCreationAcceptsEveryOfferedTool(t *testing.T) {
	useRepoDataDir(t)
	srv, _ := newTestServer(t)
	options := getWorkspaceOptions(t, srv)
	root := t.TempDir()

	for _, entry := range options["tools"].([]any) {
		tool := entry.(map[string]any)["id"].(string)
		rec := postWorkspace(t, srv, map[string]any{
			"dir":       filepath.Join(root, tool),
			"connector": "file",
			"tools":     []string{tool},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("creating with the offered tool %q = %d: %s", tool, rec.Code, rec.Body.String())
		}
	}
}

func TestWorkspaceCreationWritesTheChosenParameters(t *testing.T) {
	useRepoDataDir(t)
	srv, _ := newTestServer(t)
	dest := filepath.Join(t.TempDir(), "nuovo")

	rec := postWorkspace(t, srv, map[string]any{
		"dir":       dest,
		"connector": "file",
		"tools":     []string{"pi", "claude"},
		"paths": map[string]string{
			"prd":          "docs/prodotto.md",
			"wiki":         "docs/kb/",
			"mockups":      "docs/mock/",
			"test_results": "docs/esiti/",
		},
		"worktree": map[string]any{
			"enabled":       true,
			"base":          "develop",
			"dir":           ".archetipo/wt",
			"branch_prefix": "us/",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspace = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var view struct {
		Dir      string   `json:"dir"`
		Tools    []string `json:"tools"`
		Hint     string   `json:"hint"`
		Template struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"template"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	if view.Dir != dest {
		t.Fatalf("response dir = %q, want %q", view.Dir, dest)
	}
	if view.Template.ID != template.DefaultID || view.Template.Version != template.Default().Version {
		t.Fatalf("response template = %+v, want the built-in Archetype", view.Template)
	}
	if view.Hint == "" {
		t.Fatal("the response does not say how to open the new workspace")
	}
	raw, err := os.ReadFile(filepath.Join(dest, ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml was not created: %v", err)
	}
	for _, want := range []string{"docs/prodotto.md", "docs/kb/", "develop", "us/", template.DefaultID} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("the created config does not carry %q:\n%s", want, raw)
		}
	}
}

func TestWorkspaceCreationFillsInTheDefaultPaths(t *testing.T) {
	useRepoDataDir(t)
	srv, _ := newTestServer(t)
	dest := filepath.Join(t.TempDir(), "predefiniti")

	rec := postWorkspace(t, srv, map[string]any{
		"dir":       dest,
		"connector": "file",
		"tools":     []string{"pi"},
		"paths":     map[string]string{},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspace = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(dest, ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("prd: docs/PRD.md")) {
		t.Fatalf("the default paths were not applied:\n%s", raw)
	}
}

func TestWorkspaceCreationRefusalsNameTheFieldAndWriteNothing(t *testing.T) {
	useRepoDataDir(t)
	srv, _ := newTestServer(t)
	root := t.TempDir()

	initialized := filepath.Join(root, "gia-iniziato")
	if err := os.MkdirAll(filepath.Join(initialized, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(initialized, ".archetipo", "config.yaml"), []byte("connector: file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		payload map[string]any
		field   string
		code    string
		dir     string
	}{
		{
			name:    "connector sconosciuto",
			payload: map[string]any{"dir": filepath.Join(root, "a"), "connector": "nope", "tools": []string{"pi"}},
			field:   "connector",
			code:    workspace.CodeConnectorUnknown,
		},
		{
			name:    "nessun tool",
			payload: map[string]any{"dir": filepath.Join(root, "b"), "connector": "file", "tools": []string{}},
			field:   "tools",
			code:    workspace.CodeToolsRequired,
		},
		{
			name:    "tool sconosciuto",
			payload: map[string]any{"dir": filepath.Join(root, "c"), "connector": "file", "tools": []string{"nope"}},
			field:   "tools",
			code:    workspace.CodeToolUnknown,
		},
		{
			name:    "destinazione relativa",
			payload: map[string]any{"dir": "relativa", "connector": "file", "tools": []string{"pi"}},
			field:   "dir",
			code:    workspace.CodeDirNotAbsolute,
		},
		{
			name:    "destinazione già inizializzata",
			payload: map[string]any{"dir": initialized, "connector": "file", "tools": []string{"pi"}},
			field:   "dir",
			code:    workspace.CodeAlreadyInitialized,
			dir:     initialized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var before []string
			if tc.dir != "" {
				before = listDirRecursive(t, tc.dir)
			}
			rec := postWorkspace(t, srv, tc.payload)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Fields []fieldError `json:"fields"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding the refusal: %v", err)
			}
			found := false
			for _, f := range body.Fields {
				if f.Field == tc.field && f.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("no refusal on %s/%s; got %+v", tc.field, tc.code, body.Fields)
			}
			if tc.dir != "" {
				if after := listDirRecursive(t, tc.dir); !equalStrings(before, after) {
					t.Fatalf("the refusal wrote into the destination:\nbefore %v\nafter  %v", before, after)
				}
			}
		})
	}
}

// The Archetype is not a parameter. A client that sends one is told so rather
// than having its choice silently dropped.
func TestWorkspaceCreationRefusesAnArchetypeChoice(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := postWorkspace(t, srv, map[string]any{
		"dir":       filepath.Join(t.TempDir(), "x"),
		"connector": "file",
		"tools":     []string{"pi"},
		"template":  "un-altro-archetipo",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func listDirRecursive(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if rel != "." {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
