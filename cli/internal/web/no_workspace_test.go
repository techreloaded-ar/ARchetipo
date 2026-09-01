package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// homeServer builds the viewer as it is when the process was started outside
// any workspace: no session at all, and a known-workspaces registry on a
// temporary directory so no test writes into the real state of the machine.
func homeServer(t *testing.T) (*Server, *workspace.Registry) {
	t.Helper()
	srv, err := NewHomeServer(nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if srv.session() != nil {
		t.Fatal("NewHomeServer built a server that is already serving a workspace")
	}
	reg := &workspace.Registry{Dir: t.TempDir()}
	srv.workspaces = reg
	return srv, reg
}

// scopedRoute is one route that presupposes an open workspace: the pattern as
// registerRoutes registers it, and a concrete request that reaches it.
type scopedRoute struct {
	pattern string
	method  string
	path    string
	body    string
}

// workspaceScopedRoutes is the full list of routes that must refuse when no
// workspace is open. It is deliberately exhaustive rather than a sample:
// TestRouteClassificationIsExhaustive reads server.go and fails as soon as a
// route is registered without landing in this table or in homeRoutes.
var workspaceScopedRoutes = []scopedRoute{
	{"GET /api/board", http.MethodGet, "/api/board", ""},
	{"GET /api/board/stream", http.MethodGet, "/api/board/stream", ""},
	{"GET /api/metrics", http.MethodGet, "/api/metrics", ""},
	{"GET /api/spec/{code}", http.MethodGet, "/api/spec/US-001", ""},
	{"POST /api/spec", http.MethodPost, "/api/spec", "{}"},
	{"PUT /api/spec/{code}", http.MethodPut, "/api/spec/US-001", "{}"},
	{"DELETE /api/spec/{code}", http.MethodDelete, "/api/spec/US-001", ""},
	{"PUT /api/spec/{code}/plan", http.MethodPut, "/api/spec/US-001/plan", "{}"},
	{"POST /api/board/move", http.MethodPost, "/api/board/move", "{}"},
	{"GET /api/spec/{code}/transition-preview", http.MethodGet, "/api/spec/US-001/transition-preview?to=done", ""},
	{"GET /api/spec/{code}/diff", http.MethodGet, "/api/spec/US-001/diff", ""},
	{"GET /api/spec/{code}/review", http.MethodGet, "/api/spec/US-001/review", ""},
	{"PUT /api/spec/{code}/review", http.MethodPut, "/api/spec/US-001/review", "{}"},
	{"POST /api/spec/{code}/request-changes", http.MethodPost, "/api/spec/US-001/request-changes", "{}"},
	{"POST /api/spec/{code}/approve", http.MethodPost, "/api/spec/US-001/approve", "{}"},
	{"POST /api/spec/{code}/integrate", http.MethodPost, "/api/spec/US-001/integrate", "{}"},
	{"GET /api/prd", http.MethodGet, "/api/prd", ""},
	{"PUT /api/prd", http.MethodPut, "/api/prd", "{}"},
	{"GET /api/execution/providers", http.MethodGet, "/api/execution/providers", ""},
	{"PUT /api/execution/provider/default", http.MethodPut, "/api/execution/provider/default", "{}"},
	{"GET /api/execution/model-choice", http.MethodGet, "/api/execution/model-choice", ""},
	{"POST /api/spec/{code}/execution", http.MethodPost, "/api/spec/US-001/execution", "{}"},
	{"GET /api/execution/{id}", http.MethodGet, "/api/execution/EX-001", ""},
	{"GET /api/execution/{id}/run", http.MethodGet, "/api/execution/EX-001/run", ""},
	{"POST /api/execution/{id}/run/messages", http.MethodPost, "/api/execution/EX-001/run/messages", "{}"},
	{"POST /api/execution/{id}/run/approvals/{approvalId}", http.MethodPost, "/api/execution/EX-001/run/approvals/AP-001", "{}"},
	{"POST /api/execution/{id}/run/cancel", http.MethodPost, "/api/execution/EX-001/run/cancel", "{}"},
	{"GET /api/config", http.MethodGet, "/api/config", ""},
	{"PUT /api/config", http.MethodPut, "/api/config", "{}"},
	{"POST /api/config/test", http.MethodPost, "/api/config/test", "{}"},
	{"GET /api/workspace/actions", http.MethodGet, "/api/workspace/actions", ""},
	{"GET /api/workspace/status", http.MethodGet, "/api/workspace/status", ""},
	{"POST /api/workspace/execution", http.MethodPost, "/api/workspace/execution", "{}"},
	{"GET /api/workspace/runs", http.MethodGet, "/api/workspace/runs", ""},
	{"POST /api/workspace/conversations", http.MethodPost, "/api/workspace/conversations", "{}"},
	{"POST /api/workspace/conversations/{id}/messages", http.MethodPost, "/api/workspace/conversations/c-1/messages", "{}"},
	{"POST /api/workspace/conversations/{id}/proposal", http.MethodPost, "/api/workspace/conversations/c-1/proposal", "{}"},
	{"POST /api/workspace/conversations/{id}/approvals/{approvalId}", http.MethodPost, "/api/workspace/conversations/c-1/approvals/a-1", "{}"},
	{"DELETE /api/workspace/conversations/{id}", http.MethodDelete, "/api/workspace/conversations/c-1", ""},
	{"GET /api/workspace/conversations", http.MethodGet, "/api/workspace/conversations", ""},
	{"GET /api/workspace/conversations/{id}", http.MethodGet, "/api/workspace/conversations/c-1", ""},
	{"POST /api/workspace/conversations/{id}/resume", http.MethodPost, "/api/workspace/conversations/c-1/resume", "{}"},
	{"GET /api/mockups", http.MethodGet, "/api/mockups", ""},
}

// homeRoutes are the routes that must answer with no workspace open: the ones
// the home itself is made of.
var homeRoutes = []string{
	"GET /api/workspace/options",
	"POST /api/workspace",
	"GET /api/workspaces",
	"POST /api/workspaces",
	"DELETE /api/workspaces/{id}",
	"POST /api/workspaces/{id}/open",
}

// contentKeys are the keys a refusal must never carry. They are the payload
// keys of the refused routes themselves: their absence is what distinguishes
// "no workspace is open" from "here is your empty board", which is AC-5.
var contentKeys = []string{"columns", "specs", "providers", "stage"}

// callRoute issues one request against the mux. The deadline exists for the
// SSE route, which blocks until its request context is done: without it the
// route served by an open workspace would never return.
func callRoute(t *testing.T, srv *Server, r scopedRoute) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var req *http.Request
	if r.body == "" {
		req = httptest.NewRequest(r.method, r.path, nil)
	} else {
		req = httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

// TestHomeServerRefusesWorkspaceRoutes is AC-5: every route that presupposes a
// workspace declares that none is open, and answers with no content at all.
func TestHomeServerRefusesWorkspaceRoutes(t *testing.T) {
	srv, _ := homeServer(t)

	for _, route := range workspaceScopedRoutes {
		t.Run(route.pattern, func(t *testing.T) {
			rec := callRoute(t, srv, route)
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s = %d, want 409: %s", route.pattern, rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding the refusal of %s: %v (%s)", route.pattern, err, rec.Body.String())
			}
			if open, ok := body["workspaceOpen"].(bool); !ok || open {
				t.Errorf("%s: workspaceOpen = %v, want false", route.pattern, body["workspaceOpen"])
			}
			if body["code"] != iox.CodeConflict {
				t.Errorf("%s: code = %v, want %q", route.pattern, body["code"], iox.CodeConflict)
			}
			for _, key := range contentKeys {
				if _, present := body[key]; present {
					t.Errorf("%s: the refusal carries %q — it answered emptily instead of declaring that no workspace is open", route.pattern, key)
				}
			}
		})
	}
}

// TestHomeServerServesHomeRoutes is AC-1 and AC-6 at the route level: with no
// workspace open the home still lists, adds and forgets known workspaces.
func TestHomeServerServesHomeRoutes(t *testing.T) {
	srv, _ := homeServer(t)

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var list workspaceListView
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decoding the list: %v", err)
	}
	if list.Open {
		t.Errorf("open = true with no workspace open")
	}
	if list.CurrentPath != "" {
		t.Errorf("currentPath = %q, want empty with no workspace open", list.CurrentPath)
	}
	// AC-5: with no workspace open there is no name to show, and the server
	// says so instead of leaving the page to guess one.
	if list.CurrentName != "" {
		t.Errorf("currentName = %q, want empty with no workspace open (AC-5)", list.CurrentName)
	}

	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/options", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace/options = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	dir := seedWorkspace(t, t.TempDir(), "alpha")
	rec = postWorkspaces(t, srv, map[string]any{"path": dir})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/workspaces = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var added workspaceEntryView
	if err := json.Unmarshal(rec.Body.Bytes(), &added); err != nil {
		t.Fatalf("decoding the added entry: %v", err)
	}
	if added.ID == "" {
		t.Fatal("the added entry has no id")
	}
	if added.Current {
		t.Error("an entry is marked current while no workspace is open")
	}

	if rec := deleteWorkspace(t, srv, added.ID); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/workspaces/%s = %d, want 204: %s", added.ID, rec.Code, rec.Body.String())
	}
}

// TestOpenedWorkspaceRestoresRoutes is the server-side half of AC-2: the gate
// is a state, not a verdict. Once a workspace is opened from the home, the very
// routes that refused a moment ago serve again, on the same process.
func TestOpenedWorkspaceRestoresRoutes(t *testing.T) {
	srv, reg := homeServer(t)
	dir := realWorkspace(t, "alpha", "US-A01", true)

	entry, err := reg.Add(dir)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces/"+entry.ID+"/open", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/workspaces/%s/open = %d, want 200: %s", entry.ID, rec.Code, rec.Body.String())
	}
	if srv.session() == nil {
		t.Fatal("the workspace was reported open but the server serves no session")
	}

	for _, route := range workspaceScopedRoutes {
		t.Run(route.pattern, func(t *testing.T) {
			rec := callRoute(t, srv, route)
			if rec.Code != http.StatusConflict {
				return
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				return
			}
			if open, ok := body["workspaceOpen"].(bool); ok && !open {
				t.Errorf("%s still refuses with workspaceOpen:false after a workspace was opened: %s", route.pattern, rec.Body.String())
			}
		})
	}
}

// routeRegistration matches every explicit route registration in the package:
// the two classifying helpers and, deliberately, the raw mux calls too, so a
// route smuggled past the helpers is observable rather than invisible. The
// pattern is captured without assuming a method prefix, because Go 1.22
// ServeMux accepts method-less patterns and a regexp anchored on GET|POST|...
// would let "/api/newthing" through unseen.
//
// These tests read the package source on purpose: the mux exposes no way to
// enumerate what was registered on it, so the files are the only place where
// "a route was added" can be observed.
var routeRegistration = regexp.MustCompile(`(handleWorkspace|handleAlways|mux\.HandleFunc|mux\.Handle)\(\s*"([^"]*)"`)

// packageRouteRegistrations returns every route registration written as a
// literal anywhere in the non-test sources of this package.
func packageRouteRegistrations(t *testing.T) [][2]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	var found [][2]string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, m := range routeRegistration.FindAllStringSubmatch(string(src), -1) {
			found = append(found, [2]string{m[1], m[2]})
		}
	}
	if len(found) == 0 {
		t.Fatal("no route registration found in the package sources: this test must be updated with the code")
	}
	return found
}

// TestAPIRoutesGoThroughTheClassifyingHelpers is the other half of the guard:
// a route registered straight on the mux would carry no classification at all,
// and the exhaustiveness test below could not even see it as unclassified.
func TestAPIRoutesGoThroughTheClassifyingHelpers(t *testing.T) {
	for _, reg := range packageRouteRegistrations(t) {
		call, pattern := reg[0], reg[1]
		if !strings.Contains(pattern, "/api/") {
			continue
		}
		if call != "handleWorkspace" && call != "handleAlways" {
			t.Errorf("%q is registered with %s: every /api route must go through handleWorkspace or handleAlways, so it cannot answer emptily with no workspace open", pattern, call)
		}
	}
}

// TestRouteClassificationIsExhaustive is the guard against the forgotten route:
// a new /api/ route that nobody classified would otherwise answer emptily on
// the home instead of refusing, and no other test would notice.
func TestRouteClassificationIsExhaustive(t *testing.T) {
	classified := map[string]bool{}
	for _, route := range workspaceScopedRoutes {
		classified[route.pattern] = true
	}
	for _, pattern := range homeRoutes {
		classified[pattern] = true
	}

	var missing []string
	seen := map[string]bool{}
	for _, reg := range packageRouteRegistrations(t) {
		pattern := reg[1]
		if !strings.Contains(pattern, "/api/") {
			continue
		}
		seen[pattern] = true
		if !classified[pattern] {
			missing = append(missing, pattern)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("routes registered but not classified by this suite: %v\nadd each one to workspaceScopedRoutes or to homeRoutes", missing)
	}

	var stale []string
	for pattern := range classified {
		if !seen[pattern] {
			stale = append(stale, pattern)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("routes classified by this suite but no longer registered: %v", stale)
	}
}
