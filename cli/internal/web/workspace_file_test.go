package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
)

// An image the agent wrote into the workspace is served by its path relative
// to the project root, and nothing else is: a path that leaves the root, a
// symlink that leaves it, a file that is not an image and an absolute path all
// answer 404.
func TestWorkspaceFileRouteServesImagesInsideTheRootOnly(t *testing.T) {
	srv, cfg, _ := newRunServer(t, nil, false)
	root := cfg.ProjectRoot
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "shot.png"), []byte("PNGBYTES"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "notes.txt"), []byte("testo"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs", "link.png")); err != nil {
		t.Fatal(err)
	}

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/file?path="+url.QueryEscape(path), nil))
		return rec
	}

	rec := get("docs/shot.png")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "PNGBYTES" {
		t.Fatalf("served = %d %s %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if rec := get("./docs/shot.png"); rec.Code != http.StatusOK {
		t.Fatalf("./ prefix = %d", rec.Code)
	}

	refused := []string{
		"../" + filepath.Base(filepath.Dir(outside)) + "/secret.png",
		"docs/../../" + filepath.Base(filepath.Dir(outside)) + "/secret.png",
		"docs/link.png",
		"docs/notes.txt",
		"docs",
		"docs/missing.png",
		outside,
		"",
	}
	for _, path := range refused {
		if rec := get(path); rec.Code != http.StatusNotFound {
			t.Fatalf("%q = %d, want 404: %s", path, rec.Code, rec.Body.String())
		}
	}

	// Named with a conversation, the path is the one the agent of that
	// conversation would write: relative to its working directory — a
	// worktree — and confined to it, so the same `docs/shot.png` is the
	// worktree's and never the root's.
	worktree := filepath.Join(root, ".archetipo", "worktrees", "US-1")
	if err := os.MkdirAll(filepath.Join(worktree, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "docs", "shot.png"), []byte("WORKTREE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := srv.session().conversationStore().Save(context.Background(), conversationlog.Record{Version: conversationlog.CurrentVersion, ID: "conv-wt", WorkingDir: worktree}); err != nil {
		t.Fatal(err)
	}
	scoped := func(path, conversation string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/file?path="+url.QueryEscape(path)+"&conversation="+url.QueryEscape(conversation), nil))
		return rec
	}
	if rec := scoped("docs/shot.png", "conv-wt"); rec.Code != http.StatusOK || rec.Body.String() != "WORKTREE" {
		t.Fatalf("in the worktree = %d %q", rec.Code, rec.Body.String())
	}
	for path, conversation := range map[string]string{
		"../../../docs/shot.png": "conv-wt",
		"docs/shot.png":          "conv-unknown",
	} {
		if rec := scoped(path, conversation); rec.Code != http.StatusNotFound {
			t.Fatalf("%q in %q = %d, want 404: %s", path, conversation, rec.Code, rec.Body.String())
		}
	}
}
