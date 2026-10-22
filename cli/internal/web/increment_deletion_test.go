package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func deletionFixture(t *testing.T) (*Server, conversationView) {
	t.Helper()
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	server, cfg, _ := newRunServer(t, provider, true)
	incrementGit(t, cfg.ProjectRoot, "init", "-b", "main")
	incrementGit(t, cfg.ProjectRoot, "config", "user.email", "tests@example.test")
	incrementGit(t, cfg.ProjectRoot, "config", "user.name", "Tests")
	if err := os.WriteFile(filepath.Join(cfg.ProjectRoot, ".archetipo", "shared-runtime.md"), []byte("runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ProjectRoot, "hello.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	incrementGit(t, cfg.ProjectRoot, "add", "-A")
	incrementGit(t, cfg.ProjectRoot, "commit", "-m", "initial")
	response := doJSON(t, server, http.MethodPost, "/api/increments", map[string]any{"request": "A removable increment"})
	if response.Code != 201 {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var view conversationView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		server.session().nativeFollowers.closeAll()
		_, _ = gitForIncrementCleanup(cfg.ProjectRoot, view.Increment.Workdir)
	})
	finishIncrementTurn(t, server, provider, view)
	return server, view
}

func TestIncrementConversationRemovalRequiresExplicitChoice(t *testing.T) {
	for _, action := range []string{"delete", "archive"} {
		t.Run(action, func(t *testing.T) {
			server, view := deletionFixture(t)
			id := view.Conversation.ID
			endpoint := conversationsRoute + "/" + id
			planResponse := doJSON(t, server, http.MethodGet, endpoint+"/deletion-plan?action="+action, nil)
			var plan conversationDeletionPlan
			if err := json.Unmarshal(planResponse.Body.Bytes(), &plan); err != nil {
				t.Fatal(err)
			}
			if !plan.RequiresChoice || plan.Increment == nil || len(plan.Choices) != 2 {
				t.Fatalf("no choice: %s", planResponse.Body.String())
			}
			method, path := http.MethodDelete, endpoint+"/record"
			if action == "archive" {
				method, path = http.MethodPost, endpoint+"/archive"
			}
			response := doJSON(t, server, method, path, nil)
			if response.Code != http.StatusConflict {
				t.Fatalf("implicit worktree choice accepted: %d %s", response.Code, response.Body.String())
			}
			response = doJSON(t, server, method, path+"?worktree=keep", nil)
			if response.Code != http.StatusOK {
				t.Fatalf("keep failed: %d %s", response.Code, response.Body.String())
			}
			if _, err := os.Stat(view.Increment.Workdir); err != nil {
				t.Fatal("keep removed worktree", err)
			}
			_, err := server.session().conversationStore().GetMetadata(context.Background(), id)
			if action == "archive" && err != nil {
				t.Fatal("lost archive", err)
			}
			if action == "delete" && err == nil {
				t.Fatal("chat not deleted")
			}
		})
	}
}

func TestIncrementEnvironmentRemovalFromChatArchiveAndButton(t *testing.T) {
	for _, action := range []string{"delete", "archive", "button"} {
		t.Run(action, func(t *testing.T) {
			server, view := deletionFixture(t)
			id := view.Conversation.ID
			root := server.session().cfg.ProjectRoot
			service := server.incrementService(server.session())
			plan, err := service.DeletionPlan(context.Background(), view.Increment.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(view.Increment.Workdir, "untracked.txt"), []byte("preserve this"), 0o600); err != nil {
				t.Fatal(err)
			}
			endpoint := conversationsRoute + "/" + id
			method, path := http.MethodDelete, endpoint+"/record?worktree=delete&head="+plan.HeadSHA
			if action == "archive" {
				method, path = http.MethodPost, endpoint+"/archive?worktree=delete&head="+plan.HeadSHA
			}
			if action == "button" {
				path = "/api/increments/" + view.Increment.ID + "?confirm=true&head=" + plan.HeadSHA
			}
			response := doJSON(t, server, method, path, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("remove failed: %d %s", response.Code, response.Body.String())
			}
			if _, err = os.Stat(view.Increment.Workdir); !os.IsNotExist(err) {
				t.Fatal("environment retained", err)
			}
			if refs := incrementGit(t, root, "for-each-ref", "--format=%(refname)", "refs/heads/"+view.Increment.Branch); refs != "" {
				t.Fatal("branch retained", refs)
			}
			receipt, err := service.DeletionReceipt(view.Increment.ID)
			if err != nil || !receipt.Deleted {
				t.Fatal("missing deletion evidence", err)
			}
			if receipt.ArchivePath != "" {
				t.Fatal("retained environment archive", receipt.ArchivePath)
			}
			if head := incrementGit(t, root, "rev-parse", receipt.BackupRef); head != plan.HeadSHA {
				t.Fatal("missing recovery commit", head)
			}
			record, err := server.session().conversationStore().GetMetadata(context.Background(), id)
			if action == "archive" {
				if err != nil || record.Archive != execution.ArchiveArchived || record.Session.Native.ID != view.Session.Session.Native.ID {
					t.Fatalf("lost native archive: %+v %v", record, err)
				}
				read := doJSON(t, server, http.MethodGet, endpoint, nil)
				var archived conversationView
				if err = json.Unmarshal(read.Body.Bytes(), &archived); err != nil {
					t.Fatal(err)
				}
				if archived.Increment == nil || archived.Increment.StatusLabel != "Ambiente eliminato" || archived.Increment.DeleteLabel != "" {
					t.Fatalf("archive not readable: %s", read.Body.String())
				}
			} else if err == nil {
				t.Fatal("chat retained")
			}
		})
	}
}
