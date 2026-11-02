package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func TestConversationReportsConfineReferencedArtifacts(t *testing.T) {
	srv, cfg := newFileServer(t)
	root := cfg.ProjectRoot
	report := filepath.Join(root, ".archetipo/environment/technical-summary.json")
	if err := os.MkdirAll(filepath.Dir(report), 0700); err != nil {
		t.Fatal(err)
	}
	content := `<script>alert("never execute")</script>`
	if err := os.WriteFile(report, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	record := conversationlog.Record{ID: "conv-reports", WorkingDir: root, Events: []execution.RunEvent{{Kind: "text", Text: "[Report](" + report + ") [Missing](.archetipo/environment/technical-session.md) [Secret](.env) [Outside](/etc/passwd)"}}}
	if err := srv.session().conversationStore().Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	base := "/api/workspace/conversations/conv-reports/reports"
	list := doJSON(t, srv, http.MethodGet, base, nil)
	var links []conversationReportLink
	if err := json.Unmarshal(list.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || !links[0].Available || links[1].Available {
		t.Fatalf("unexpected reports: %s", list.Body.String())
	}
	if strings.Contains(links[0].URL, root) {
		t.Fatal("URL leaks checkout path")
	}
	got := doJSON(t, srv, http.MethodGet, links[0].URL, nil)
	if got.Code != 200 || got.Body.String() != content || !strings.HasPrefix(got.Header().Get("Content-Type"), "text/plain") || !strings.Contains(got.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("unsafe report: %d %s", got.Code, got.Body.String())
	}
	for _, target := range []string{links[1].URL, base + "/unknown", "/api/workspace/conversations/conv-other/reports/unknown"} {
		got := doJSON(t, srv, http.MethodGet, target, nil)
		if got.Code != 404 || !strings.Contains(got.Body.String(), "Report non più disponibile") {
			t.Fatalf("missing message: %d %s", got.Code, got.Body.String())
		}
	}
	if err := os.Remove(report); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, ".env")
	if err := os.WriteFile(secret, []byte("PRIVATE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, report); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := doJSON(t, srv, http.MethodGet, links[0].URL, nil); got.Code != 404 || strings.Contains(got.Body.String(), "PRIVATE") {
		t.Fatal("report symlink escaped scope")
	}
	// Neither transcript metadata nor a familiar basename permits a different checkout.
	record.WorkingDir = filepath.Join(root, "removed-worktree")
	if err := srv.session().conversationStore().Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if got := doJSON(t, srv, http.MethodGet, links[0].URL, nil); got.Code != 404 {
		t.Fatal("deleted worktree fell back to main")
	}
}

func TestConversationReportPathPolicy(t *testing.T) {
	root := t.TempDir()
	for _, ref := range []string{".env", ".archetipo/config.yaml", ".archetipo/environment/runtime/package.json", ".archetipo/environment/state.json", "../outside.json", "file://other/etc/passwd", "https://example.com/report.json", "docs/test-results/../../.env", "docs/test-results/a.png"} {
		if got := conversationReportPath(root, "docs/test-results", ref); got != "" {
			t.Errorf("accepted %q: %s", ref, got)
		}
	}
	for _, ref := range []string{"docs/test-results/inc-1/environment/report.json", ".archetipo/environment/technical-summary.json"} {
		if got := conversationReportPath(root, "docs/test-results", ref); got == "" {
			t.Errorf("refused report %q", ref)
		}
	}
	abs := filepath.Join(root, ".archetipo/environment/technical-summary.json")
	uriPath := filepath.ToSlash(abs)
	if filepath.VolumeName(abs) != "" {
		uriPath = "/" + uriPath
	}
	for _, ref := range []string{abs, (&url.URL{Scheme: "file", Path: uriPath}).String()} {
		if got := conversationReportPath(root, "docs/test-results", ref); got != ".archetipo"+string(filepath.Separator)+"environment"+string(filepath.Separator)+"technical-summary.json" {
			t.Errorf("refused absolute local report %q: %q", ref, got)
		}
	}
}
