package increment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", iox.NewConflict("git "+strings.Join(args, " ")+": "+strings.TrimSpace(string(out)), "inspect the increment working directory and retry", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) Create(ctx context.Context, title, request, origin string) (Record, error) {
	return s.locked(ctx, func() (record Record, err error) {
		title, request = strings.TrimSpace(title), strings.TrimSpace(request)
		if title == "" || request == "" {
			return record, iox.NewInvalidInput("title and request are required", "describe the intended increment", nil)
		}
		if s.Config.Increment != nil {
			return record, iox.NewConflict("cannot create an increment inside another increment", "open the original workspace", nil)
		}
		const base = "main"
		sha, err := git(ctx, s.Config.ProjectRoot, "rev-parse", "--verify", "refs/heads/"+base+"^{commit}")
		if err != nil {
			return record, iox.NewPrecondition("increment base branch main is unavailable", "create or fetch the local main branch before starting an increment", err)
		}
		var entropy [8]byte
		if _, err = rand.Read(entropy[:]); err != nil {
			return record, err
		}
		id := "inc-" + hex.EncodeToString(entropy[:])
		record = Record{ID: id, Title: title, Request: request, OriginConversationID: origin, Status: Discovery, CreatedAt: time.Now().UTC(), BaseBranch: base, BaseSHA: sha, Branch: "archetipo/increments/" + id, Workdir: filepath.Join(s.dir(), "worktrees", id), Revisions: []Revision{}, Feedback: []string{}}
		if err = os.MkdirAll(filepath.Dir(record.Workdir), 0o700); err != nil {
			return Record{}, err
		}
		if _, err = git(ctx, s.Config.ProjectRoot, "worktree", "add", "-b", record.Branch, record.Workdir, sha); err != nil {
			return Record{}, err
		}
		defer func() {
			if err != nil {
				clean := context.WithoutCancel(ctx)
				_, _ = git(clean, s.Config.ProjectRoot, "worktree", "remove", "--force", record.Workdir)
				_, _ = git(clean, s.Config.ProjectRoot, "branch", "-D", record.Branch)
			}
		}()
		if err = s.excludeRuntime(ctx); err != nil {
			return Record{}, err
		}
		marker := filepath.Join(record.Workdir, config.IncrementWorkspacePath)
		if err = recordfile.WriteAtomic(filepath.Dir(marker), marker, ".increment-workspace-", config.IncrementWorkspace{ID: id, ParentRoot: s.Config.ProjectRoot}); err != nil {
			return Record{}, err
		}
		if err = s.installRuntime(record.Workdir); err != nil {
			return Record{}, err
		}
		cfg, err := config.LoadExact(record.Workdir)
		if err != nil {
			return Record{}, err
		}
		if err = filefs.New(cfg).EnsureEmptyBacklog(); err != nil {
			return Record{}, err
		}
		if err = s.save(record); err == nil {
			// The parent keeps only the discovery index. All later changes live
			// in the sandbox, which is the native agent’s writable boundary.
			path, _ := s.path(record.ID)
			err = recordfile.WriteAtomic(s.dir(), path, ".increment-", record)
		}
		return record, err
	})
}

// RemoveUnused rolls back a failed native-session open. It can never remove
// an increment that has begun work, acquired a conversation, or changed code.
func (s *Service) RemoveUnused(ctx context.Context, id string) error {
	_, err := s.lockedWorkspaceRecord(ctx, id, func() (Record, error) {
		r, readErr := s.Read(id)
		if readErr != nil {
			return r, readErr
		}
		if r.ConversationID != "" || len(r.Revisions) != 0 {
			return r, iox.NewConflict("increment has already started", "preserve its work and resume it", nil)
		}
		if cleanErr := s.requireClean(ctx, r); cleanErr != nil {
			return r, cleanErr
		}
		head, headErr := git(ctx, r.Workdir, "rev-parse", "HEAD")
		if headErr != nil {
			return r, headErr
		}
		if head != r.BaseSHA {
			return r, iox.NewConflict("increment has new commits", "preserve its work", nil)
		}
		if _, removeErr := git(ctx, s.Config.ProjectRoot, "worktree", "remove", "--force", r.Workdir); removeErr != nil {
			return r, removeErr
		}
		if _, removeErr := git(ctx, s.Config.ProjectRoot, "branch", "-d", r.Branch); removeErr != nil {
			return r, removeErr
		}
		recordPath, _ := s.path(r.ID)
		return r, os.Remove(recordPath)
	})
	return err
}

func (s *Service) excludeRuntime(ctx context.Context) error {
	common, err := git(ctx, s.Config.ProjectRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(s.Config.ProjectRoot, common)
	}
	path := filepath.Join(common, "info", "exclude")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	patterns := []string{"/.archetipo/increments/", "/.archetipo/increment-archives/", "/.archetipo/increment-local/", "/.archetipo/increment-workspace.json", "/.archetipo/conversations/", "/.archetipo/executions/", "/.archetipo/workflow-runs/", "/.archetipo/environment/", "/.archetipo/tmp/", "/.archetipo/shared-runtime.md"}
	for _, tool := range workspace.Tools() {
		patterns = append(patterns, "/"+tool.SkillsDir+"/archetipo-*/")
	}
	text := string(raw)
	for _, pattern := range patterns {
		if !strings.Contains("\n"+text+"\n", "\n"+pattern+"\n") {
			text += "\n" + pattern + "\n"
		}
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func (s *Service) installRuntime(target string) error {
	// Copy only workflow assets, never environment files or credentials.
	// A project recipe declares commands and variable names. Never copy its
	// resource checkpoint: every checkout prepares and owns its own environment.
	sourceRecipe := filepath.Join(s.Config.ProjectRoot, ".archetipo", "environment.json")
	targetRecipe := filepath.Join(target, ".archetipo", "environment.json")
	if _, err := os.Stat(sourceRecipe); err == nil {
		if _, err = os.Stat(targetRecipe); os.IsNotExist(err) {
			if err = workspace.CopyFile(sourceRecipe, targetRecipe); err != nil {
				return err
			}
		}
	}
	// Versioned recipes arrive with main. Also inherit a portable workspace
	// recipe before it is committed, but never a fixed endpoint from another run.
	sourcePreview := filepath.Join(s.Config.ProjectRoot, preview.RecipePath)
	targetPreview := filepath.Join(target, preview.RecipePath)
	if _, err := os.Stat(targetPreview); os.IsNotExist(err) {
		if recipe, loadErr := preview.Load(s.Config.ProjectRoot); loadErr == nil && strings.Contains(recipe.URL, "{port}") {
			if err = workspace.CopyFile(sourcePreview, targetPreview); err != nil {
				return err
			}
		}
	}
	dataDir, _ := workspace.DiscoverDataDir()
	runtimeSource := filepath.Join(s.Config.ProjectRoot, ".archetipo", "shared-runtime.md")
	if _, err := os.Stat(runtimeSource); os.IsNotExist(err) && dataDir != "" {
		if assets, assetErr := workspace.RuntimeAssetsDir(dataDir); assetErr == nil {
			runtimeSource = filepath.Join(assets, "shared-runtime.md")
		}
	}
	dst := filepath.Join(target, ".archetipo", "shared-runtime.md")
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err = workspace.CopyFile(runtimeSource, dst); err != nil {
			return iox.NewPrecondition("workflow runtime is unavailable", "update the workspace installation before starting an increment", err)
		}
	}
	for _, tool := range workspace.Tools() {
		source := filepath.Join(s.Config.ProjectRoot, tool.SkillsDir)
		_, installedErr := os.Stat(source)
		selected := s.Config.Execution.DefaultProvider != nil && s.Config.Execution.DefaultProvider.ID == tool.Key
		if installedErr != nil && !selected {
			continue
		}
		for _, name := range append([]string{"archetipo-increment"}, "archetipo-spec", "archetipo-plan", "archetipo-implement", "archetipo-review", "archetipo-autopilot", "archetipo-wiki") {
			from := filepath.Join(source, name)
			if _, err := os.Stat(from); os.IsNotExist(err) && dataDir != "" {
				from = filepath.Join(dataDir, "skills", name)
			}
			if _, err := os.Stat(from); err != nil {
				if selected {
					return fmt.Errorf("install %s: %w", name, err)
				}
				continue
			}
			dest := filepath.Join(target, tool.SkillsDir, name)
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				if err = workspace.CopyTree(from, dest); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
