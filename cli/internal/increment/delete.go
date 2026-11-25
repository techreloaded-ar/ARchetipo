package increment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
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
	// RemovesBacklogSpec names the backlog spec the deletion takes away with the
	// increment; KeepsBacklogSpec names one the connector cannot delete.
	RemovesBacklogSpec string `json:"removes_backlog_spec,omitempty"`
	KeepsBacklogSpec   string `json:"keeps_backlog_spec,omitempty"`
}

type DeletionOptions struct {
	HeadSHA               string
	Confirm               bool
	PreserveConversations bool
	// AbandonEnvironment lets the deletion proceed when the registered cleanup
	// ran and failed. It is a second, explicit decision: the services and test
	// data that could not be removed stay on the computer and are named in the
	// receipt.
	AbandonEnvironment bool
}

// AbandonedEnvironment records what a deletion left behind because its cleanup
// failed and the person chose to remove the increment anyway.
type AbandonedEnvironment struct {
	Reason        string   `json:"reason"`
	EnvironmentID string   `json:"environment_id,omitempty"`
	Resources     []string `json:"resources,omitempty"`
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
	// The backlog spec of an increment closed without being integrated goes
	// with it. BacklogSpecSettled makes that step resumable like the others.
	BacklogSpecSettled   bool                  `json:"backlog_spec_settled,omitempty"`
	BacklogSpecRemoved   string                `json:"backlog_spec_removed,omitempty"`
	BacklogSpecKept      string                `json:"backlog_spec_kept,omitempty"`
	EnvironmentAbandoned *AbandonedEnvironment `json:"environment_abandoned,omitempty"`
}

// EnvironmentCleanupError reports a registered cleanup that ran and failed. The
// worktree is preserved; the same confirmed deletion may be repeated once the
// cause is fixed, or with AbandonEnvironment to leave the named resources behind.
type EnvironmentCleanupError struct {
	Reason        string
	ReportPath    string
	EnvironmentID string
	Resources     []string
	coded         *iox.CodedError
}

func (e *EnvironmentCleanupError) Error() string { return e.coded.Error() }
func (e *EnvironmentCleanupError) Unwrap() error { return e.coded }

// cleanupFailureReason names the step that failed in its own words: the line
// of its log that states a failure, else the first one that is not a CLI
// envelope. The complete log stays in the report the error names.
func cleanupFailureReason(cleanup verification.Result) string {
	for _, step := range cleanup.Steps {
		if step.Error == "" && step.ExitCode == 0 {
			continue
		}
		detail := step.Error
		if raw, err := os.ReadFile(step.LogPath); err == nil {
			first, failure := "", ""
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "{") {
					continue
				}
				if first == "" {
					first = line
				}
				lower := strings.ToLower(line)
				if strings.Contains(lower, "error") || strings.Contains(lower, "fail") || strings.Contains(lower, "fatal") || strings.Contains(lower, "denied") {
					failure = line
					break
				}
			}
			if failure != "" {
				detail = failure
			} else if first != "" {
				detail = first
			}
		}
		if runes := []rune(detail); len(runes) > 300 {
			detail = string(runes[:300]) + "…"
		}
		return step.Name + ": " + detail
	}
	return "the cleanup reported a failure without a failed step"
}

// registeredResourceNames lists the resources the environment journal still
// holds, read before cleanup rewrites it.
func registeredResourceNames(environment *verification.Service) []string {
	state, err := environment.Resources()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(state.Resources))
	for _, resource := range state.Resources {
		names = append(names, resource.Resource.Name)
	}
	return names
}

// withdrawBacklogSpec settles the backlog spec of an increment that is removed
// before being integrated: nothing would ever close it, so it goes with the
// increment. A delivered increment keeps its spec as the trace of what was
// merged, a spec the person already closed or removed is left alone, and a code
// that now names somebody else's story is never touched. With apply false it
// only reports what a deletion would do.
func (s *Service) withdrawBacklogSpec(ctx context.Context, r Record, apply bool) (removed, kept string, err error) {
	merged := r.PR != nil && r.PR.State == "MERGED"
	if r.Status == Delivered || merged || r.BacklogSpecCode == "" || s.Connector == nil {
		return "", "", nil
	}
	spec, found, err := s.ownBacklogSpec(ctx, r)
	deleter, canDelete := s.Connector.(connector.SpecDeleter)
	if !canDelete {
		if err == nil && (!found || spec.Status == domain.StatusDone) {
			return "", "", nil
		}
		return "", r.BacklogSpecCode, nil
	}
	if err != nil || !found || spec.Status == domain.StatusDone {
		return "", "", err
	}
	if apply {
		if _, err = deleter.DeleteSpec(ctx, spec.Code); err != nil {
			return "", "", err
		}
	}
	return spec.Code, "", nil
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
	// Commits already on the fork base (possibly fetched from origin ahead of
	// the local base branch) are not the increment's own work.
	countArgs := []string{"rev-list", "--count", plan.HeadSHA, "^refs/heads/" + r.BaseBranch}
	if r.BaseSHA != "" {
		countArgs = append(countArgs, "^"+r.BaseSHA)
	}
	count, err := git(ctx, s.Config.ProjectRoot, countArgs...)
	if err != nil {
		return plan, err
	}
	if plan.UnmergedCommits, err = strconv.Atoi(count); err != nil {
		return plan, err
	}
	// An unreadable backlog never hides the plan: the deletion itself reads it again.
	plan.RemovesBacklogSpec, plan.KeepsBacklogSpec, _ = s.withdrawBacklogSpec(ctx, r, false)
	return plan, nil
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
					environment := verification.New(r.Workdir, "").ForOwnedPreview().ForRemoval()
					registered := registeredResourceNames(environment)
					cleanup, cleanupErr := environment.Execute(ctx, "cleanup", "", "", false)
					if cleanupErr != nil {
						return r, cleanupErr
					}
					if !cleanup.Passed {
						// A failed cleanup is never a dead end: the person may fix
						// its cause and retry, or decide to leave the resources.
						reason := cleanupFailureReason(cleanup)
						if !options.AbandonEnvironment {
							return r, &EnvironmentCleanupError{
								Reason: reason, ReportPath: cleanup.ReportPath, EnvironmentID: cleanup.EnvironmentID, Resources: registered,
								coded: iox.NewPrecondition("environment cleanup failed: "+reason, "fix the cause and retry, or repeat the confirmed deletion with --abandon-environment to leave its services and test data in place; report: "+cleanup.ReportPath+"; worktree preserved", nil),
							}
						}
						result.EnvironmentAbandoned = &AbandonedEnvironment{Reason: reason, EnvironmentID: cleanup.EnvironmentID, Resources: registered}
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
		if !result.BacklogSpecSettled {
			if result.BacklogSpecRemoved, result.BacklogSpecKept, err = s.withdrawBacklogSpec(ctx, r, true); err != nil {
				return r, err
			}
			result.BacklogSpecSettled = true
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
