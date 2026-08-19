package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Result describes the workspace that was created.
type Result struct {
	Dir       string                `json:"dir"`
	Connector string                `json:"connector"`
	Tools     []string              `json:"tools"`
	Paths     domain.ConfigPaths    `json:"paths"`
	Worktree  domain.WorktreeConfig `json:"worktree"`
	Template  TemplateIdentity      `json:"template"`
}

// Initialize creates a workspace in opts.Dir, or leaves the destination
// exactly as it found it.
//
// It never writes into the destination directly. The whole workspace is built
// in a temporary directory first and only then moved in, one file at a time,
// recording every path it creates. On any failure — a refused request, a
// cancelled context, a collision discovered mid-move — those recorded paths
// are removed and nothing else is touched. Files and directories that were
// already there are never in the ledger, so a rollback cannot delete them.
//
// Moving file by file rather than renaming whole directories is what makes
// that possible: a destination may legitimately already contain .claude/ or
// .github/, and renaming onto them would either fail or destroy what is there.
func Initialize(ctx context.Context, opts Options) (Result, error) {
	resolved, err := Preflight(opts)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, cancelled(err)
	}

	dataDir, err := DiscoverDataDir()
	if err != nil {
		return Result{}, err
	}
	skillsDir := filepath.Join(dataDir, "skills")
	if _, statErr := os.Stat(skillsDir); statErr != nil {
		return Result{}, iox.NewPrecondition(
			"skills directory not found",
			"set ARCHETIPO_DATA_DIR to the package root, or reinstall the CLI via `npm i -g @techreloaded/archetipo`",
			statErr,
		)
	}
	runtimeDir, err := RuntimeAssetsDir(dataDir)
	if err != nil {
		return Result{}, err
	}

	staging, err := os.MkdirTemp("", "archetipo-init-")
	if err != nil {
		return Result{}, iox.NewInternal("cannot create the staging directory", err)
	}
	// Always: after a successful commit the staging directory is empty, so the
	// removal is a no-op; after a failure it is the only thing left to clean.
	defer func() { _ = os.RemoveAll(staging) }()

	if err := stage(ctx, staging, skillsDir, runtimeDir, resolved); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, cancelled(err)
	}

	ledger := &commitLedger{}
	if err := commit(ctx, staging, resolved.Dir, ledger); err != nil {
		ledger.rollback()
		return Result{}, err
	}

	toolKeys := make([]string, 0, len(resolved.Tools))
	for _, t := range resolved.Tools {
		toolKeys = append(toolKeys, t.Key)
	}
	return Result{
		Dir:       resolved.Dir,
		Connector: resolved.Connector,
		Tools:     toolKeys,
		Paths:     resolved.Paths,
		Worktree:  resolved.Worktree,
		Template: TemplateIdentity{
			ID:      resolved.Template.ID,
			Version: resolved.Template.Version,
			Label:   resolved.Template.Label,
		},
	}, nil
}

// stage builds the complete workspace inside dir. Nothing here can leave a
// trace in the destination: a failure at this point only costs a temporary
// directory the caller already removes.
func stage(ctx context.Context, dir, skillsDir, runtimeDir string, resolved Resolved) error {
	archetipoDir := filepath.Join(dir, ".archetipo")
	if err := os.MkdirAll(archetipoDir, 0o755); err != nil {
		return iox.NewInternal("cannot stage .archetipo/", err)
	}

	body, err := os.ReadFile(filepath.Join(runtimeDir, "config.yaml"))
	if err != nil {
		return iox.NewInternal("read config template", err)
	}
	rendered := RenderConfig(string(body), RenderInput{
		Connector: resolved.Connector,
		Paths:     resolved.Paths,
		Worktree:  resolved.Worktree,
		// The Living Wiki gate stays off, as it is for `archetipo init`
		// without --wiki: the skill is installed either way and stays usable
		// on demand.
		Wiki:     false,
		Template: resolved.Template,
	})
	if err := os.WriteFile(filepath.Join(archetipoDir, "config.yaml"), []byte(rendered), 0o644); err != nil {
		return iox.NewInternal("cannot stage config.yaml", err)
	}

	sharedSrc := filepath.Join(runtimeDir, "shared-runtime.md")
	if _, statErr := os.Stat(sharedSrc); statErr == nil {
		if err := CopyFile(sharedSrc, filepath.Join(archetipoDir, "shared-runtime.md")); err != nil {
			return iox.NewInternal("cannot stage shared-runtime.md", err)
		}
	}

	for _, tool := range resolved.Tools {
		if err := ctx.Err(); err != nil {
			return cancelled(err)
		}
		target := filepath.Join(dir, filepath.FromSlash(tool.SkillsDir))
		if err := os.MkdirAll(target, 0o755); err != nil {
			return iox.NewInternal("cannot stage "+tool.SkillsDir, err)
		}
		for _, skill := range resolved.Template.Skills {
			src := filepath.Join(skillsDir, skill)
			if _, statErr := os.Stat(src); statErr != nil {
				return iox.NewPrecondition("skill missing in package: "+skill, "reinstall the CLI", statErr)
			}
			if err := CopyTree(src, filepath.Join(target, skill)); err != nil {
				return iox.NewInternal("cannot stage skill "+skill, err)
			}
		}
	}
	return nil
}

// commitLedger records exactly what the commit created, so a rollback can undo
// that and nothing else.
type commitLedger struct {
	files []string
	dirs  []string
}

func (l *commitLedger) rollback() {
	for i := len(l.files) - 1; i >= 0; i-- {
		_ = os.Remove(l.files[i])
	}
	for i := len(l.dirs) - 1; i >= 0; i-- {
		// os.Remove, never RemoveAll: a directory that meanwhile holds
		// something else is not ours to destroy, and an empty one is all we
		// ever created.
		_ = os.Remove(l.dirs[i])
	}
}

// commit moves the staged workspace into dest. `.archetipo` goes first, then
// the tool directories in lexical order, so the sequence is deterministic and
// a mid-way failure is reproducible.
func commit(ctx context.Context, staging, dest string, ledger *commitLedger) error {
	if err := ensureDir(dest, ledger); err != nil {
		return err
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return iox.NewInternal("cannot read the staging directory", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == ".archetipo" {
			return true
		}
		if names[j] == ".archetipo" {
			return false
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		if err := commitTree(ctx, filepath.Join(staging, name), filepath.Join(dest, name), ledger); err != nil {
			return err
		}
	}
	return nil
}

func commitTree(ctx context.Context, src, dst string, ledger *commitLedger) error {
	info, err := os.Lstat(src)
	if err != nil {
		return iox.NewInternal("cannot read "+src, err)
	}
	if !info.IsDir() {
		return commitFile(ctx, src, dst, ledger)
	}
	if err := ensureDir(dst, ledger); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return iox.NewInternal("cannot read "+src, err)
	}
	for _, e := range entries {
		if err := commitTree(ctx, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()), ledger); err != nil {
			return err
		}
	}
	return nil
}

func commitFile(ctx context.Context, src, dst string, ledger *commitLedger) error {
	if err := ctx.Err(); err != nil {
		return cancelled(err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return &ValidationError{Fields: []FieldError{{
			Field:   "dir",
			Code:    CodePathOccupied,
			Message: "the destination already contains " + dst,
		}}}
	} else if !errors.Is(err, os.ErrNotExist) {
		return iox.NewInternal("cannot inspect "+dst, err)
	}

	if err := os.Rename(src, dst); err != nil {
		// The staging directory lives in the system temp area, which may be on
		// a different filesystem than the destination: on macOS that is the
		// normal case, not an exotic one. Fall back to copy + remove.
		if !isCrossDeviceRename(err) {
			return iox.NewInternal("cannot move "+dst, err)
		}
		if copyErr := CopyFile(src, dst); copyErr != nil {
			return iox.NewInternal("cannot copy "+dst, copyErr)
		}
		_ = os.Remove(src)
	}
	ledger.files = append(ledger.files, dst)
	return nil
}

// ensureDir creates dir when missing and records it, so the rollback removes
// only the directories the commit itself brought into existence.
func ensureDir(dir string, ledger *commitLedger) error {
	info, err := os.Stat(dir)
	switch {
	case err == nil && info.IsDir():
		return nil
	case err == nil:
		return &ValidationError{Fields: []FieldError{{
			Field:   "dir",
			Code:    CodePathOccupied,
			Message: dir + " exists and is not a directory",
		}}}
	case !errors.Is(err, os.ErrNotExist):
		return iox.NewInternal("cannot inspect "+dir, err)
	}
	parent := filepath.Dir(dir)
	if parent != dir {
		if err := ensureDir(parent, ledger); err != nil {
			return err
		}
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return iox.NewInternal("cannot create "+dir, err)
	}
	ledger.dirs = append(ledger.dirs, dir)
	return nil
}

// isCrossDeviceRename reports the one rename failure that is not a failure at
// all: the staging directory and the destination live on different volumes.
func isCrossDeviceRename(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// cancelled turns an interruption into a refusal the caller can render, rather
// than an internal failure: nothing went wrong, the operator changed their mind.
func cancelled(err error) error {
	return &ValidationError{Fields: []FieldError{{
		Field:   "dir",
		Code:    CodeCancelled,
		Message: "the initialization was interrupted before completing: " + err.Error(),
	}}}
}
