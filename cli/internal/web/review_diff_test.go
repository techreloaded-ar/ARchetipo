package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// gitRepoWithTwoCommits builds a repository whose main branch carries two
// commits and returns the SHA of the first one. It is the shape the fork base
// has to tell apart: the increment of a spec started at the first commit is the
// second one, not the whole distance from the base branch.
func gitRepoWithTwoCommits(t *testing.T, root string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "archetipo-test@example.com")
	run("config", "user.name", "ARchetipo Test")
	write("a.txt", "uno\n")
	run("add", ".")
	run("commit", "-q", "-m", "primo")
	first := run("rev-parse", "HEAD")
	write("b.txt", "due\n")
	run("add", ".")
	run("commit", "-q", "-m", "secondo")
	return first
}

func getDiff(t *testing.T, srv *Server, code string) diffView {
	t.Helper()
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/spec/"+code+"/diff", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET diff: %d %s", w.Code, w.Body.String())
	}
	var out diffView
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Senza branch il diff parte dal fork_base che `spec start` ha registrato. La
// base configurata resta l'ultima spiaggia: su un branch di lavoro lontano da
// main mostrerebbe tutto il divario invece dell'incremento della spec.
func TestGetDiffWithoutBranchUsesTheRecordedForkBase(t *testing.T) {
	srv, cfg := newFileServer(t)
	first := gitRepoWithTwoCommits(t, cfg.ProjectRoot)
	ctx := context.Background()
	if _, err := srv.session().conn.SaveInitialBacklog(ctx, []domain.Spec{
		{Code: "US-001", Title: "Con base", Status: domain.StatusReview, ForkBase: first},
		{Code: "US-002", Title: "Senza base", Status: domain.StatusReview},
	}); err != nil {
		t.Fatal(err)
	}

	withBase := getDiff(t, srv, "US-001")
	if withBase.Base != first {
		t.Errorf("base = %q, want il fork base %q", withBase.Base, first)
	}
	if withBase.BaseFallback {
		t.Error("un fork base valido non è un fallback")
	}
	if len(withBase.Files) != 1 || withBase.Files[0].NewPath != "b.txt" {
		t.Fatalf("atteso il solo secondo commit nel diff, got %+v", withBase.Files)
	}

	// Una spec senza fork base registrato — quelle già in REVIEW prima di questa
	// modifica — continua a vedere il diff dalla base configurata.
	withoutBase := getDiff(t, srv, "US-002")
	if withoutBase.Base != cfg.Worktree.Base {
		t.Errorf("base = %q, want la base configurata %q", withoutBase.Base, cfg.Worktree.Base)
	}
	if len(withoutBase.Files) != 0 {
		t.Errorf("dalla base configurata il working tree è pulito, got %+v", withoutBase.Files)
	}
}

// Uno SHA sparito dopo un rebase non lascia la tab senza diff: si ripiega sulla
// base configurata e lo dice, perché il chip del viewer mostri la verità.
func TestGetDiffFallsBackWhenTheForkBaseIsGone(t *testing.T) {
	srv, cfg := newFileServer(t)
	gitRepoWithTwoCommits(t, cfg.ProjectRoot)
	ctx := context.Background()
	if _, err := srv.session().conn.SaveInitialBacklog(ctx, []domain.Spec{
		{Code: "US-001", Title: "Base morta", Status: domain.StatusReview, ForkBase: "0000000000000000000000000000000000000000"},
	}); err != nil {
		t.Fatal(err)
	}
	got := getDiff(t, srv, "US-001")
	if got.Base != cfg.Worktree.Base {
		t.Errorf("base = %q, want il ripiegamento su %q", got.Base, cfg.Worktree.Base)
	}
	if !got.BaseFallback {
		t.Error("il ripiegamento non è stato dichiarato")
	}
}
