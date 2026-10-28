package increment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

type DeletionPlan struct {
	Record          Record `json:"increment"`
	HeadSHA         string `json:"head_sha"`
	Dirty           bool   `json:"dirty"`
	UnmergedCommits int    `json:"unmerged_commits"`
}

type DeletionOptions struct {
	HeadSHA               string
	Confirm               bool
	PreserveConversations bool
}

type DeletionResult struct {
	Record                 Record   `json:"increment"`
	HeadSHA                string   `json:"head_sha"`
	ArchivePath            string   `json:"archive_path"` // Legacy receipts only; new deletions retain commits, never environment files.
	BackupRef              string   `json:"backup_ref"`
	WorktreeRemoved        bool     `json:"worktree_removed"`
	BranchRemoved          bool     `json:"branch_removed"`
	Deleted                bool     `json:"deleted"`
	ConversationIDs        []string `json:"conversation_ids"`
	ConversationsPreserved bool     `json:"conversations_preserved"`
}

// DeletionPlan verifies actual Git ownership before offering a destructive action.
// A path in a conversation or a forged increment record never authorizes removal.
func (s *Service) DeletionPlan(ctx context.Context, id string) (DeletionPlan, error) {
	r, err := s.Read(id)
	if err != nil {
		return DeletionPlan{}, err
	}
	plan := DeletionPlan{Record: r}
	top, err := git(ctx, r.Workdir, "rev-parse", "--show-toplevel")
	if err != nil {
		return plan, err
	}
	actual, err := filepath.EvalSymlinks(r.Workdir)
	if err != nil {
		return plan, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(s.Config.ProjectRoot)
	if err != nil {
		return plan, err
	}
	if actual != filepath.Join(canonicalRoot, ".archetipo", "increments", "worktrees", id) {
		return plan, iox.NewConflict("increment path traverses a symlink", "preserve the worktree and repair its location", nil)
	}
	canonicalTop, err := filepath.EvalSymlinks(top)
	if err != nil || actual != canonicalTop {
		return plan, iox.NewConflict("increment directory is not its own worktree", "preserve its files and repair the Git registration", err)
	}
	parentCommon, err := git(ctx, s.Config.ProjectRoot, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return plan, err
	}
	common, err := git(ctx, r.Workdir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return plan, err
	}
	parentCommon, err = filepath.EvalSymlinks(parentCommon)
	if err != nil {
		return plan, err
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil || common != parentCommon {
		return plan, iox.NewConflict("increment belongs to another repository", "preserve the worktree", err)
	}
	branch, err := git(ctx, r.Workdir, "symbolic-ref", "--short", "HEAD")
	if err != nil || branch != r.Branch {
		return plan, iox.NewConflict("increment branch changed", "restore the expected increment branch before removing it", err)
	}
	plan.HeadSHA, err = git(ctx, r.Workdir, "rev-parse", "HEAD")
	if err != nil {
		return plan, err
	}
	status, err := git(ctx, r.Workdir, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return plan, err
	}
	plan.Dirty = status != ""
	count, err := git(ctx, s.Config.ProjectRoot, "rev-list", "--count", plan.HeadSHA, "^refs/heads/"+r.BaseBranch)
	if err != nil {
		return plan, err
	}
	plan.UnmergedCommits, err = strconv.Atoi(count)
	return plan, err
}

// Delete is explicit, local and recoverable. The caller must stop native work
// first; the same ownership locks refuse a live runtime held by any viewer.
// A durable receipt makes removal resumable after worktree removal or a crash.
func (s *Service) Delete(ctx context.Context, id string, options DeletionOptions) (DeletionResult, error) {
	expectedHead, confirmed := options.HeadSHA, options.Confirm
	var result DeletionResult
	if !confirmed || expectedHead == "" {
		return result, iox.NewInvalidInput("increment deletion needs explicit confirmation and head_sha", "inspect increment deletion-plan before confirming deletion", nil)
	}
	_, err := s.locked(ctx, func() (Record, error) {
		if _, err := s.path(id); err != nil {
			return Record{}, err
		}
		dir, directoryErr := s.archiveDirectory(id)
		if directoryErr != nil {
			return Record{}, directoryErr
		}
		receipt := filepath.Join(dir, "deletion.json")
		if previous, readErr := s.DeletionReceipt(id); readErr == nil {
			result = previous
			if result.HeadSHA != expectedHead || result.ConversationsPreserved != options.PreserveConversations {
				return Record{}, iox.NewConflict("deletion receipt does not match confirmation", "inspect the existing deletion receipt", nil)
			}
			if result.Deleted {
				return result.Record, nil
			}
			if head, refErr := git(ctx, s.Config.ProjectRoot, "rev-parse", "--verify", result.BackupRef); refErr != nil || head != expectedHead {
				return Record{}, iox.NewPrecondition("recovery commit unavailable", "restore the Git recovery ref before continuing deletion", refErr)
			}
		} else if !os.IsNotExist(readErr) {
			return Record{}, readErr
		}
		r, err := s.Read(id)
		if err != nil {
			if result.WorktreeRemoved && result.BackupRef != "" && errors.Is(err, os.ErrNotExist) {
				r = result.Record
			} else {
				return r, err
			}
		}
		result.Record = r
		result.ConversationsPreserved = options.PreserveConversations
		store, err := conversationlog.NewFileStore(s.Config.ProjectRoot)
		if err != nil {
			return r, err
		}
		conversations, err := store.List(ctx)
		if err != nil {
			return r, err
		}
		for _, conv := range conversations {
			if conv.IncrementID != id && filepath.Clean(conv.WorkingDir) != filepath.Clean(r.Workdir) {
				continue
			}
			for _, lockID := range []string{conv.ID + "-runtime", conv.ID} {
				lock, lockErr := store.TryLock(ctx, lockID)
				if lockErr != nil {
					return r, iox.NewConflict("a conversation still owns this worktree", "stop and release its native session before deleting the increment", lockErr)
				}
				defer func() { _ = lock.Unlock() }()
			}
			if conv.Native() && conv.Work != "" && conv.Work != execution.SessionIdle {
				return r, iox.NewConflict("the native turn has not stopped", "stop the conversation before deleting its increment", nil)
			}
			if !conv.Native() && conv.FinalState == "ACTIVE" {
				return r, iox.NewConflict("a legacy conversation is still active", "close it before removing the worktree", nil)
			}
			if !containsID(result.ConversationIDs, conv.ID) {
				result.ConversationIDs = append(result.ConversationIDs, conv.ID)
			}
		}
		save := func() error { return recordfile.WriteAtomic(dir, receipt, ".deletion-", result) }
		if !result.WorktreeRemoved {
			// A saved recovery ref permits resuming after checkout removal.
			// A live checkout is always inspected again.
			if _, statErr := os.Lstat(r.Workdir); os.IsNotExist(statErr) && result.BackupRef != "" {
				result.WorktreeRemoved = true
			} else {
				plan, planErr := s.DeletionPlan(ctx, id)
				if planErr != nil {
					return r, planErr
				}
				if plan.HeadSHA != expectedHead {
					return r, iox.NewConflict("increment changed since deletion was confirmed", "inspect its current state and confirm again", nil)
				}
				localStore, storeErr := conversationlog.NewFileStore(r.Workdir)
				if storeErr != nil {
					return r, storeErr
				}
				mutationLock, lockErr := localStore.TryLock(ctx, "increment-mutations")
				if lockErr != nil {
					return r, iox.NewConflict("increment mutation is still running", "wait for it to finish", lockErr)
				}
				defer func() { _ = mutationLock.Unlock() }()
				// A released native session may still have a preview owned by another
				// viewer. Every preview lock must be free, including the standalone CLI.
				for _, previewID := range append([]string{"standalone", "environment", "command"}, result.ConversationIDs...) {
					previewLock, lockErr := localStore.TryLock(ctx, "preview-"+previewID)
					if lockErr != nil {
						return r, iox.NewConflict("a preview is still running", "stop the preview in its owning viewer or terminal before deleting this increment", lockErr)
					}
					defer func() { _ = previewLock.Unlock() }()
				}
				if err = preview.VerifyStopped(r.Workdir); err != nil {
					return r, err
				}
				_, stateErr := os.Stat(filepath.Join(r.Workdir, ".archetipo", "environment", "state.json"))
				_, resourceErr := os.Stat(filepath.Join(r.Workdir, ".archetipo", "environment", "resources.json"))
				if stateErr == nil || resourceErr == nil {
					// The deletion locks above already hold preview-environment,
					// excluding every preview while registered cleanup runs.
					cleanup, cleanupErr := verification.New(r.Workdir, "").ForOwnedPreview().Execute(ctx, "cleanup", "", "", false)
					if cleanupErr != nil {
						return r, cleanupErr
					}
					if !cleanup.Passed {
						return r, iox.NewPrecondition("environment cleanup failed", "inspect "+cleanup.ReportPath+"; worktree preserved", nil)
					}
				} else if !os.IsNotExist(stateErr) {
					return r, stateErr
				} else if !os.IsNotExist(resourceErr) {
					return r, resourceErr
				}
				for _, lockID := range []string{"environment-preparation", "verification-environment", "verification-command", "environment-lifetime"} {
					lock, lockErr := localStore.TryLock(ctx, lockID)
					if lockErr != nil {
						return r, iox.NewConflict("environment operation is still running", "wait before deleting its checkout", lockErr)
					}
					defer func() { _ = lock.Unlock() }()
				}
				if raw, stateErr := os.ReadFile(filepath.Join(r.Workdir, ".archetipo", "environment", "state.json")); stateErr == nil {
					var state struct {
						PreparationID string `json:"preparation_id"`
					}
					if err = json.Unmarshal(raw, &state); err != nil {
						return r, err
					}
					if state.PreparationID != "" {
						return r, iox.NewConflict("environment restarted during cleanup", "retry cleanup before removing the worktree", nil)
					}
				} else if !os.IsNotExist(stateErr) {
					return r, stateErr
				}

				if err = os.MkdirAll(dir, 0o700); err != nil {
					return r, err
				}
				result.HeadSHA = plan.HeadSHA
				result.BackupRef = "refs/archetipo/deleted/" + id
				currentPlan, verifyErr := s.DeletionPlan(ctx, id)
				if verifyErr != nil {
					return r, verifyErr
				}
				if currentPlan.HeadSHA != expectedHead {
					return r, iox.NewConflict("increment changed during cleanup", "inspect its newer commits and confirm again; worktree preserved", nil)
				}
				if _, err = git(ctx, s.Config.ProjectRoot, "update-ref", result.BackupRef, plan.HeadSHA); err != nil {
					return r, err
				}
				if err = save(); err != nil {
					return r, err
				}
				// Retain the latest record outside the directory about to be removed.
				parentPath, _ := s.path(id)
				if err = recordfile.WriteAtomic(s.dir(), parentPath, ".increment-", r); err != nil {
					return r, err
				}
				if _, err = git(ctx, s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir); err != nil {
					return r, err
				}
				result.WorktreeRemoved = true
			}
			if err = save(); err != nil {
				return r, err
			}
		}
		if !result.BranchRemoved {
			current, refErr := git(ctx, s.Config.ProjectRoot, "rev-parse", "--verify", "refs/heads/"+r.Branch)
			if refErr == nil {
				if current != expectedHead {
					return r, iox.NewConflict("branch advanced after confirmation", "preserve its newer commits before removing it; recovery ref: "+result.BackupRef, nil)
				}
				if _, err = git(ctx, s.Config.ProjectRoot, "branch", "-D", r.Branch); err != nil {
					return r, err
				}
			} else {
				refs, listErr := git(ctx, s.Config.ProjectRoot, "for-each-ref", "--format=%(objectname)", "refs/heads/"+r.Branch)
				if listErr != nil {
					return r, listErr
				}
				if refs != "" {
					return r, refErr
				}
			}
			result.BranchRemoved = true
			if err = save(); err != nil {
				return r, err
			}
		}
		for _, conv := range conversations {
			if conv.IncrementID == id || filepath.Clean(conv.WorkingDir) == filepath.Clean(r.Workdir) {
				if options.PreserveConversations {
					conv.Archive = execution.ArchiveArchived
					conv.Connection = execution.ConnectionReleased
					err = store.Save(ctx, conv)
				} else {
					err = store.Delete(ctx, conv.ID)
				}
				if err != nil {
					return r, err
				}
			}
		}
		parentPath, _ := s.path(id)
		if err = os.Remove(parentPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return r, err
		}
		// Finish legacy interrupted deletions without retaining their environment archive.
		if result.ArchivePath != "" {
			if err = os.Remove(result.ArchivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return r, err
			}
			result.ArchivePath = ""
		}
		result.Deleted = true
		return r, save()
	})
	if err != nil && result.BackupRef != "" {
		return result, iox.NewConflict("increment deletion incomplete: "+err.Error(), fmt.Sprintf("recovery ref: %s; retry the same confirmed deletion", result.BackupRef), err)
	}
	return result, err
}

func containsID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

// DeletionReceipt retains the result and recovery location after the live index
// and worktree are gone. Archived conversations use it without recreating work.
func (s *Service) DeletionReceipt(id string) (DeletionResult, error) {
	var result DeletionResult
	if _, err := s.path(id); err != nil {
		return result, err
	}
	dir, directoryErr := s.archiveDirectory(id)
	if directoryErr != nil {
		return result, directoryErr
	}
	raw, err := os.ReadFile(filepath.Join(dir, "deletion.json"))
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	if result.Record.ID != id || result.Record.Workdir != filepath.Join(s.dir(), "worktrees", id) || result.Record.Branch != "archetipo/increments/"+id || (result.ArchivePath != "" && result.ArchivePath != filepath.Join(dir, "worktree.tar.gz")) || result.BackupRef != "refs/archetipo/deleted/"+id {
		return result, iox.NewConflict("invalid deletion receipt", "preserve its recovery files", nil)
	}
	return result, nil
}

// Refuse archive paths redirected outside the owning checkout. No directory is
// created by a receipt read, and a damaged recovery location never authorizes
// removing live work.
func (s *Service) archiveDirectory(id string) (string, error) {
	current := s.Config.ProjectRoot
	for _, part := range []string{".archetipo", "increment-archives", id} {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", iox.NewConflict("invalid increment recovery directory", "use a local directory owned by this workspace", nil)
		}
	}
	return current, nil
}
