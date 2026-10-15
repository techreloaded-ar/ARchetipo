package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

func TestDocumentationReadsConfiguredWikiWithAutomaticMaintenanceOff(t *testing.T) {
	srv, cfg := newFileServer(t)
	root := filepath.Join(cfg.ProjectRoot, "knowledge")
	srv.session().cfg.Paths.Wiki = root
	if cfg.Wiki.Enabled {
		t.Fatal("fixture must have automatic maintenance disabled")
	}
	if _, err := wiki.Init(root); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: overview\ntitle: Panoramica\ndescription: Un progetto\nstatus: generated\n---\n# Panoramica\n\nContenuto ricercabile.\n"
	pagePath := filepath.Join(root, "overview.md")
	if err := os.WriteFile(pagePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wiki.Approve(cfg.ProjectRoot, root, []string{"overview"}); err != nil {
		t.Fatal(err)
	}
	pages, states, _, err := wiki.Status(cfg.ProjectRoot, root)
	if err != nil {
		t.Fatal(err)
	}
	w := documentationRequest(srv, "/api/documentation?q=RICERCABILE")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var result struct {
		Collections []documentationCollection `json:"collections"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Collections) != 1 || !result.Collections[0].Available || len(result.Collections[0].Items) != 1 {
		t.Fatalf("collection: %+v", result)
	}
	item := result.Collections[0].Items[0]
	if item.Body != "" || item.State != states[0] || item.StateLabel != "Revisionata" {
		t.Fatalf("index does not agree with CLI status: %+v", item)
	}
	w = documentationRequest(srv, "/api/documentation/wiki/page?id=overview")
	var detail documentationPage
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || !reflect.DeepEqual(detail.Page, pages[0]) || detail.State != states[0] {
		t.Fatalf("page differs from shared Wiki read: %s", w.Body.String())
	}
	before, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/documentation?q=absent", "/api/documentation/wiki/page?id=../../README", "/api/documentation/wiki/page?id=/overview", "/api/documentation/unknown/page?id=overview"} {
		w = documentationRequest(srv, route)
		if strings.Contains(route, "q=absent") {
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) {
				t.Fatalf("empty search: %s", w.Body.String())
			}
		} else if w.Code != http.StatusNotFound {
			t.Fatalf("invalid selection %s: %d", route, w.Code)
		}
	}
	after, err := os.ReadFile(pagePath)
	if err != nil || string(before) != string(after) {
		t.Fatal("reading changed Wiki bytes")
	}
}

func TestDocumentationDistinguishesAbsentEmptyAndUnreadableWiki(t *testing.T) {
	srv, cfg := newFileServer(t)
	root := filepath.Join(cfg.ProjectRoot, cfg.Paths.Wiki)
	w := documentationRequest(srv, "/api/documentation")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"available":false`) {
		t.Fatalf("absent: %s", w.Body.String())
	}
	if _, err := wiki.Init(root); err != nil {
		t.Fatal(err)
	}
	w = documentationRequest(srv, "/api/documentation")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"available":true,"items":[]`) {
		t.Fatalf("empty: %s", w.Body.String())
	}
	if err := os.WriteFile(filepath.Join(root, "broken.md"), []byte("not a Wiki page"), 0o600); err != nil {
		t.Fatal(err)
	}
	w = documentationRequest(srv, "/api/documentation")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"code":"E_CONFLICT"`) {
		t.Fatalf("unreadable: %s", w.Body.String())
	}
}

func TestDocumentationDoesNotReadSymlinkOutsideWiki(t *testing.T) {
	srv, cfg := newFileServer(t)
	root := filepath.Join(cfg.ProjectRoot, cfg.Paths.Wiki)
	if _, err := wiki.Init(root); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "private.md")
	if err := os.WriteFile(external, []byte("---\ntitle: PRIVATE_SENTINEL\n---\nPrivate body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "escape.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	w := documentationRequest(srv, "/api/documentation")
	if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "PRIVATE_SENTINEL") {
		t.Fatalf("escaped Wiki root: %d %s", w.Code, w.Body.String())
	}
}

func documentationRequest(srv *Server, route string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
	return w
}
