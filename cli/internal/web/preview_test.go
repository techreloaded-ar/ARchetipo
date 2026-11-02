package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/inmemory"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

func TestConversationPreviewUsesItsWorkingDirectory(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	server, err := NewServer(inmemory.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.session().previews.Close()
	workdir := t.TempDir()
	if _, err = preview.Configure(workdir, []byte(`{"version":1,"directory":".","url":"http://127.0.0.1:41987/checkout"}`)); err != nil {
		t.Fatal(err)
	}
	record := conversationlog.Record{ID: "conv-preview", WorkingDir: workdir}
	if err = server.session().conversationStore().Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/workspace/conversations/conv-preview/preview", nil)
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
	var view preview.View
	if err = json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Configured || view.Recipe.URL != "http://127.0.0.1:41987/checkout" {
		t.Fatalf("wrong working directory: %+v", view)
	}
}

func TestWorkspacePreviewNeedsNoProductConversation(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	server, err := NewServer(inmemory.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.session().previews.Close()
	runPreviewGit(t, cfg.ProjectRoot, "init", "-b", "main")
	response := httptest.NewRecorder()
	server.mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/workspace/preview", nil))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var view environmentPreviewView
	if err = json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Context.WorkingDirectory != cfg.ProjectRoot || view.Context.Branch != "main" || view.Context.ConversationID != "" || view.Context.Worktree {
		t.Fatalf("wrong target: %+v", view.Context)
	}
	if view.State != "unconfigured" {
		t.Fatalf("unexpected state: %+v", view)
	}
	// Metadata reflects the checkout's actual branch, never a fixed main label.
	runPreviewGit(t, cfg.ProjectRoot, "symbolic-ref", "HEAD", "refs/heads/develop")
	response = httptest.NewRecorder()
	server.mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/workspace/preview", nil))
	if !strings.Contains(response.Body.String(), `"branch":"develop"`) {
		t.Fatal("stale branch metadata", response.Body.String())
	}
}

func TestLivePreviewDoesNotDisplayOldOperationErrorAsCurrent(t *testing.T) {
	root := t.TempDir()
	if err := saveEnvironmentOperation(root, environmentOperation{State: "error", Error: "old setup failure", ConversationID: "conv-env"}); err != nil {
		t.Fatal(err)
	}
	view := conversationPreviewView(root, preview.View{State: "ready", Preparation: &verification.Result{Passed: true, Stable: true}})
	if view.State != "ready" || view.Environment.State != "ready" || view.Environment.Error != "" || view.Environment.PreviousError != "old setup failure" || view.Environment.ConversationID != "conv-env" {
		t.Fatalf("stale error or lost history: %+v", view)
	}
}

func TestPreviewContextFollowsWorktreeAndDetachedHead(t *testing.T) {
	root := t.TempDir()
	runPreviewGit(t, root, "init", "-b", "main")
	runPreviewGit(t, root, "-c", "user.name=Tests", "-c", "user.email=tests@example.test", "commit", "--allow-empty", "-m", "baseline")
	worktree := filepath.Join(t.TempDir(), "feature")
	runPreviewGit(t, root, "worktree", "add", "-b", "feature/preview", worktree)
	info := checkoutPreviewContext(context.Background(), worktree)
	if !info.Worktree || info.Branch != "feature/preview" || info.WorkingDirectory != worktree {
		t.Fatalf("wrong worktree identity: %+v", info)
	}
	runPreviewGit(t, worktree, "checkout", "--detach")
	info = checkoutPreviewContext(context.Background(), worktree)
	if info.Branch != "" || info.Commit == "" || !strings.Contains(info.Label, "HEAD scollegato") {
		t.Fatalf("detached head presented as branch: %+v", info)
	}
}

func runPreviewGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git: %s %v", out, err)
	}
}

func TestPreviewPreparationRejectsCrossOriginCommands(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	server, err := NewServer(inmemory.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.session().previews.Close()
	for _, suffix := range []string{"/preview", "/preview/prepare", "/preview/configure"} {
		request := httptest.NewRequest(http.MethodPost, "/api/workspace/conversations/missing"+suffix, nil)
		request.Header.Set("Origin", "http://127.0.0.1:9999")
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", suffix, recorder.Code, recorder.Body.String())
		}
	}
}
func TestWorkspacePreviewRejectsCrossOriginCommands(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	server, err := NewServer(inmemory.New(cfg), cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.session().previews.Close()
	for _, method := range []string{"POST", "DELETE"} {
		request := httptest.NewRequest(method, "/api/workspace/preview", nil)
		request.Header.Set("Origin", "http://127.0.0.1:9999")
		response := httptest.NewRecorder()
		server.mux.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatal(method, response.Code, response.Body.String())
		}
	}
}
