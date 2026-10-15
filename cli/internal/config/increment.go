package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IncrementWorkspacePath is runtime metadata, never a replacement for the
// project's tracked config. A sandbox keeps the same code/config baseline.
const IncrementWorkspacePath = ".archetipo/increment-workspace.json"

type IncrementWorkspace struct {
	ID         string `json:"id"`
	ParentRoot string `json:"parent_root"`
}

func isIncrementWorktreeRoot(dir string) bool {
	if !strings.HasPrefix(filepath.Base(dir), "inc-") {
		return false
	}
	parent := filepath.Dir(dir)
	if filepath.Base(parent) != "worktrees" {
		return false
	}
	parent = filepath.Dir(parent)
	if filepath.Base(parent) != "increments" {
		return false
	}
	return filepath.Base(filepath.Dir(parent)) == ".archetipo"
}

func applyIncrementWorkspace(c Config) (Config, error) {
	raw, err := os.ReadFile(filepath.Join(c.ProjectRoot, IncrementWorkspacePath))
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Config{}, err
	}
	var ref IncrementWorkspace
	if err := json.Unmarshal(raw, &ref); err != nil {
		return Config{}, fmt.Errorf("invalid increment workspace: %w", err)
	}
	if !strings.HasPrefix(ref.ID, "inc-") || filepath.Base(ref.ID) != ref.ID || strings.ContainsAny(ref.ID, "/\\") || !filepath.IsAbs(ref.ParentRoot) {
		return Config{}, fmt.Errorf("invalid increment workspace reference")
	}
	expected := filepath.Join(ref.ParentRoot, ".archetipo", "increments", "worktrees", ref.ID)
	if filepath.Clean(expected) != filepath.Clean(c.ProjectRoot) {
		return Config{}, fmt.Errorf("increment workspace root does not match its reference")
	}
	if _, err := os.Stat(filepath.Join(ref.ParentRoot, IncrementWorkspacePath)); !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("nested increment workspaces are not supported")
	}
	parent, err := LoadExact(ref.ParentRoot)
	if err != nil {
		return Config{}, err
	}
	parent.ProjectRoot = c.ProjectRoot
	for _, value := range []*string{&parent.Paths.PRD, &parent.Paths.Wiki, &parent.Paths.Mockups, &parent.Paths.TestResults} {
		local := *value
		if filepath.IsAbs(local) {
			var relErr error
			local, relErr = filepath.Rel(ref.ParentRoot, local)
			if relErr != nil {
				return Config{}, relErr
			}
		}
		local = filepath.Clean(local)
		if local == ".." || strings.HasPrefix(local, ".."+string(filepath.Separator)) {
			return Config{}, fmt.Errorf("increment artifact path escapes the workspace: %s", *value)
		}
		*value = local
	}
	parent.Connector = ConnectorFile
	parent.File.Backlog = ".archetipo/increment-local/backlog.yaml"
	parent.File.Planning = ".archetipo/increment-local/plans"
	parent.Worktree.Enabled = false
	parent.Git.AutoCommit = true
	parent.Increment = &ref
	return parent, nil
}
