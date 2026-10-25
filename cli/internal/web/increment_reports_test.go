package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertIncrementReportScope(t *testing.T, srv *Server, id, workdir string) {
	t.Helper()
	rel := filepath.Join(srv.session().cfg.Paths.TestResults, id)
	for _, root := range []string{workdir, srv.session().cfg.ProjectRoot} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel, "README.md"), []byte(root), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	base := "/docs/test-results/" + id + "/"
	response := doJSON(t, srv, http.MethodGet, base+"README.md", nil)
	if response.Code != http.StatusOK || response.Body.String() != workdir {
		t.Fatalf("report did not come from the increment: %d %s", response.Code, response.Body.String())
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain") || response.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("report content is not inert")
	}
	for _, path := range []string{"missing.md", "%2e%2e%2fREADME.md", "%2Fetc%2Fpasswd", "./", "../../config.yaml"} {
		if got := doJSON(t, srv, http.MethodGet, base+path, nil); got.Code == http.StatusOK {
			t.Fatalf("report route accepted %q", path)
		}
	}
	t.Run("report symlinks cannot escape", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "private.txt")
		if err := os.WriteFile(outside, []byte("PRIVATE"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(workdir, rel, "escape.txt")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if got := doJSON(t, srv, http.MethodGet, base+"escape.txt", nil); got.Code != http.StatusNotFound {
			t.Fatalf("symlink escaped report root: %d", got.Code)
		}
	})
}
