package increment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func TestDeleteIncrementRetainsOnlyCommitsAndPreservesNativeSession(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete-chat", true: "archive-chat"}[preserve], func(t *testing.T) {
			ctx := context.Background()
			s := fixture(t)
			r := create(t, s)
			for name, body := range map[string]string{"hello.txt": "committed change", ".gitignore": "private.env\n"} {
				if err := os.WriteFile(filepath.Join(r.Workdir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runGit(t, r.Workdir, "add", "hello.txt", ".gitignore")
			runGit(t, r.Workdir, "commit", "-m", "feat: increment change")
			for name, body := range map[string]string{"hello.txt": "dirty change", "new.txt": "untracked work", "private.env": "local synthetic settings"} {
				if err := os.WriteFile(filepath.Join(r.Workdir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			outside := filepath.Join(t.TempDir(), "outside.txt")
			if err := os.WriteFile(outside, []byte("unrelated"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(r.Workdir, "external-link")); err != nil {
				t.Fatal(err)
			}
			store, err := conversationlog.NewFileStore(s.Config.ProjectRoot)
			if err != nil {
				t.Fatal(err)
			}
			record := conversationlog.Record{Version: 2, ID: "conv-removal", IncrementID: r.ID, WorkingDir: r.Workdir, Work: execution.SessionIdle, Session: &execution.SessionMetadata{Native: execution.NativeSessionReference{ID: "keep-native-session"}}}
			if err = store.Save(ctx, record); err != nil {
				t.Fatal(err)
			}
			unrelated := conversationlog.Record{ID: "conv-unrelated", WorkingDir: s.Config.ProjectRoot, FinalState: "CLOSED"}
			if err = store.Save(ctx, unrelated); err != nil {
				t.Fatal(err)
			}
			plan, err := s.DeletionPlan(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Dirty || plan.UnmergedCommits != 1 {
				t.Fatalf("missing warnings: %+v", plan)
			}
			options := DeletionOptions{HeadSHA: plan.HeadSHA, Confirm: true, PreserveConversations: preserve}
			result, err := s.Delete(ctx, r.ID, options)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Deleted || !result.WorktreeRemoved || !result.BranchRemoved {
				t.Fatalf("incomplete deletion: %+v", result)
			}
			if _, err = os.Stat(r.Workdir); !os.IsNotExist(err) {
				t.Fatal("worktree retained", err)
			}
			if strings.Contains(runGit(t, s.Config.ProjectRoot, "worktree", "list", "--porcelain"), r.Workdir) {
				t.Fatal("Git registration retained")
			}
			if got := runGit(t, s.Config.ProjectRoot, "for-each-ref", "--format=%(refname)", "refs/heads/"+r.Branch); got != "" {
				t.Fatal("branch retained", got)
			}
			if runGit(t, s.Config.ProjectRoot, "rev-parse", result.BackupRef) != plan.HeadSHA {
				t.Fatal("commit recovery ref missing")
			}
			if result.ArchivePath != "" {
				t.Fatal("retained an environment archive", result.ArchivePath)
			}
			entries, err := os.ReadDir(filepath.Join(s.Config.ProjectRoot, ".archetipo", "increment-archives", r.ID))
			if err != nil || len(entries) != 1 || entries[0].Name() != "deletion.json" {
				t.Fatalf("expected only deletion receipt, got %v: %v", entries, err)
			}
			if got := runGit(t, s.Config.ProjectRoot, "show", result.BackupRef+":hello.txt"); got != "committed change" {
				t.Fatal("lost committed work", got)
			}
			if got, err := os.ReadFile(outside); err != nil || string(got) != "unrelated" {
				t.Fatal("changed external resource", err)
			}
			got, err := store.GetMetadata(ctx, record.ID)
			if preserve {
				if err != nil || got.Archive != execution.ArchiveArchived || got.Session.Native.ID != "keep-native-session" {
					t.Fatalf("lost archived native context: %+v %v", got, err)
				}
			} else if err == nil {
				t.Fatal("conversation retained")
			}
			if _, err = store.GetMetadata(ctx, unrelated.ID); err != nil {
				t.Fatal("removed unrelated conversation", err)
			}
			records, err := s.List()
			if err != nil || len(records) != 0 {
				t.Fatalf("increment still indexed: %+v %v", records, err)
			}
			again, err := s.Delete(ctx, r.ID, options)
			if err != nil || !again.Deleted || again.ArchivePath != result.ArchivePath {
				t.Fatal("retry not idempotent", err)
			}
		})
	}
}

func TestDeleteIncrementRefusesUnconfirmedChangedOrOwnedWork(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []DeletionOptions{{HeadSHA: plan.HeadSHA}, {Confirm: true}, {Confirm: true, HeadSHA: "outdated"}} {
		if _, err = s.Delete(ctx, r.ID, options); err == nil {
			t.Fatal("removed work without valid confirmation")
		}
	}
	store, err := conversationlog.NewFileStore(s.Config.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(ctx, conversationlog.Record{ID: "conv-owned", IncrementID: r.ID, WorkingDir: r.Workdir, FinalState: "CLOSED"}); err != nil {
		t.Fatal(err)
	}
	lock, err := store.TryLock(ctx, "conv-owned-runtime")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	_, err = s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA})
	var coded *iox.CodedError
	if !errors.As(err, &coded) || coded.Code != iox.CodeConflict {
		t.Fatalf("ignored live owner: %v", err)
	}
	if _, err = os.Stat(r.Workdir); err != nil {
		t.Fatal("worktree lost", err)
	}
}

func TestDeleteIncrementResumesAfterWorktreeRemoval(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "ref-only", true: "legacy-archive"}[legacy], func(t *testing.T) {
			ctx := context.Background()
			s := fixture(t)
			r := create(t, s)
			plan, err := s.DeletionPlan(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(s.Config.ProjectRoot, ".archetipo", "increment-archives", r.ID)
			if err = os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			receipt := DeletionResult{Record: r, HeadSHA: plan.HeadSHA, BackupRef: "refs/archetipo/deleted/" + r.ID}
			archive := filepath.Join(dir, "worktree.tar.gz")
			if legacy {
				receipt.ArchivePath = archive
				if err = os.WriteFile(archive, []byte("legacy environment archive"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runGit(t, s.Config.ProjectRoot, "update-ref", receipt.BackupRef, receipt.HeadSHA)
			raw, _ := json.Marshal(receipt)
			if err = os.WriteFile(filepath.Join(dir, "deletion.json"), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			runGit(t, s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir)
			// An interrupted deletion must not remove its branch without recovery.
			runGit(t, s.Config.ProjectRoot, "update-ref", "-d", receipt.BackupRef)
			if _, err = s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}); err == nil {
				t.Fatal("continued without a recovery ref")
			}
			if got := runGit(t, s.Config.ProjectRoot, "rev-parse", r.Branch); got != plan.HeadSHA {
				t.Fatal("removed branch without recovery", got)
			}
			runGit(t, s.Config.ProjectRoot, "update-ref", receipt.BackupRef, receipt.HeadSHA)
			result, err := s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA})
			if err != nil || !result.Deleted {
				t.Fatalf("could not resume: %+v %v", result, err)
			}
			if _, err = os.Stat(archive); !os.IsNotExist(err) || result.ArchivePath != "" {
				t.Fatal("retained environment archive", err)
			}
		})
	}
}

func TestDeleteIncrementPreservesWorkWhenCleanupCannotRun(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(r.Workdir, ".archetipo", "environment")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"checks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}); err == nil {
		t.Fatal("removed an environment without its cleanup recipe")
	}
	if _, err = os.Stat(filepath.Join(r.Workdir, "hello.txt")); err != nil {
		t.Fatal("lost working files", err)
	}
}

func TestDeletionRefusesRedirectedWorktreeAndRecoveryDirectory(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	archives := filepath.Join(s.Config.ProjectRoot, ".archetipo", "increment-archives")
	if err = os.Symlink(t.TempDir(), archives); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}); err == nil {
		t.Fatal("followed a redirected recovery directory")
	}
	if err = os.Remove(archives); err != nil {
		t.Fatal(err)
	}
	moved := r.Workdir + "-moved"
	if err = os.Rename(r.Workdir, moved); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(r.Workdir); _ = os.Rename(moved, r.Workdir) }()
	if err = os.Symlink(moved, r.Workdir); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeletionPlan(ctx, r.ID); err == nil {
		t.Fatal("accepted redirected worktree")
	}
}

func TestDeleteIncrementRefusesPreviewOwnedByAnotherViewer(t *testing.T) {
	ctx := context.Background()
	s := fixture(t)
	r := create(t, s)
	plan, err := s.DeletionPlan(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := conversationlog.NewFileStore(s.Config.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = parent.Save(ctx, conversationlog.Record{ID: "conv-preview-owner", IncrementID: r.ID, WorkingDir: r.Workdir, FinalState: "CLOSED"}); err != nil {
		t.Fatal(err)
	}
	local, err := conversationlog.NewFileStore(r.Workdir)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := local.TryLock(ctx, "preview-conv-preview-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	if _, err = s.Delete(ctx, r.ID, DeletionOptions{Confirm: true, HeadSHA: plan.HeadSHA}); err == nil {
		t.Fatal("removed worktree under another viewer's preview")
	}
	if _, err = os.Stat(r.Workdir); err != nil {
		t.Fatal("lost worktree", err)
	}
}
