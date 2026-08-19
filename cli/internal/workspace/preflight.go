package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// Resolved is a request that has been checked and completed with the defaults
// the caller left out. Initialize works from this, never from raw Options.
type Resolved struct {
	Dir       string
	Connector string
	Tools     []Tool
	Paths     domain.ConfigPaths
	Worktree  domain.WorktreeConfig
	Template  template.Template
}

// Preflight validates a request and fills in the defaults, without writing
// anything into the destination. Every refusal names the input that caused it
// and carries a stable code, and all refusals of one request are reported
// together: fixing a form one field per round-trip is not a user experience.
func Preflight(opts Options) (Resolved, error) {
	invalid := &ValidationError{}
	var resolved Resolved

	resolved.Dir = checkDir(strings.TrimSpace(opts.Dir), invalid)
	resolved.Connector = checkConnector(strings.TrimSpace(opts.Connector), invalid)
	resolved.Tools = checkTools(opts.Tools, invalid)
	resolved.Paths = checkPaths(opts.Paths, invalid)
	resolved.Worktree = checkWorktree(opts.Worktree, invalid)
	resolved.Template = template.Default()

	if len(invalid.Fields) > 0 {
		return Resolved{}, invalid
	}
	return resolved, nil
}

// checkDir resolves and validates the destination. A relative path is refused
// rather than resolved against the process working directory: the viewer runs
// in some other workspace, so "docs" would land somewhere the user never named.
func checkDir(dir string, invalid *ValidationError) string {
	if dir == "" {
		invalid.add("dir", CodeDirRequired, "indicate the directory where the workspace must be created")
		return ""
	}
	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(dir, "~"), string(os.PathSeparator)))
		}
	}
	if !filepath.IsAbs(dir) {
		invalid.add("dir", CodeDirNotAbsolute, "indicate an absolute path: a relative one would be resolved against the viewer's directory, not yours")
		return ""
	}
	dir = filepath.Clean(dir)

	info, err := os.Stat(dir)
	switch {
	case err == nil && !info.IsDir():
		invalid.add("dir", CodeDirNotADirectory, "the path exists and is a file, not a directory")
		return ""
	case err == nil:
		if _, statErr := os.Stat(filepath.Join(dir, config.RelativePath)); statErr == nil {
			invalid.add("dir", CodeAlreadyInitialized, "the directory already contains an initialized workspace ("+config.RelativePath+")")
			return ""
		}
		if writeErr := probeWritable(dir); writeErr != nil {
			invalid.add("dir", CodeDirNotWritable, "the directory is not writable")
			return ""
		}
		return dir
	case errors.Is(err, os.ErrNotExist):
		ancestor := nearestExistingAncestor(dir)
		info, statErr := os.Stat(ancestor)
		if statErr != nil || !info.IsDir() {
			invalid.add("dir", CodeParentNotWritable, "the parent directory does not exist or is not a directory")
			return ""
		}
		if writeErr := probeWritable(ancestor); writeErr != nil {
			invalid.add("dir", CodeParentNotWritable, "the parent directory "+ancestor+" is not writable")
			return ""
		}
		return dir
	default:
		invalid.add("dir", CodeDirNotADirectory, "the path is not readable: "+err.Error())
		return ""
	}
}

// nearestExistingAncestor walks up from dir until it finds a path that exists.
// The filesystem root always does, so the walk terminates.
func nearestExistingAncestor(dir string) string {
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		if _, err := os.Stat(parent); err == nil {
			return parent
		}
		dir = parent
	}
}

// probeWritable answers the only question that matters — can we create a file
// here — by trying. Permission bits alone lie on more than one filesystem.
func probeWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".archetipo-write-probe-")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func checkConnector(connector string, invalid *ValidationError) string {
	if connector == "" || !IsConnector(connector) {
		invalid.add("connector", CodeConnectorUnknown, "choose one of: "+ConnectorsHint())
		return ""
	}
	return connector
}

func checkTools(keys []string, invalid *ValidationError) []Tool {
	nonEmpty := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.TrimSpace(k) != "" {
			nonEmpty = append(nonEmpty, k)
		}
	}
	if len(nonEmpty) == 0 {
		invalid.add("tools", CodeToolsRequired, "select at least one tool to install the skills for")
		return nil
	}
	resolved, err := ResolveTools(nonEmpty)
	if err != nil {
		var unknown *UnknownToolError
		if errors.As(err, &unknown) {
			invalid.add("tools", CodeToolUnknown, "unknown tool "+unknown.Key+"; valid: "+ToolKeysHint())
			return nil
		}
		invalid.add("tools", CodeToolUnknown, err.Error())
		return nil
	}
	return resolved
}

// checkPaths accepts an entirely empty block as "no preference" and fills in
// the defaults. A partially filled block is refused instead: a forgotten path
// would otherwise be indistinguishable from a deliberately empty one.
func checkPaths(paths domain.ConfigPaths, invalid *ValidationError) domain.ConfigPaths {
	entries := []struct {
		field string
		value *string
	}{
		{"paths.prd", &paths.PRD},
		{"paths.wiki", &paths.Wiki},
		{"paths.mockups", &paths.Mockups},
		{"paths.test_results", &paths.TestResults},
	}
	filled := 0
	for _, e := range entries {
		*e.value = strings.TrimSpace(*e.value)
		if *e.value != "" {
			filled++
		}
	}
	if filled == 0 {
		return config.Default().Paths
	}
	for _, e := range entries {
		if *e.value == "" {
			invalid.add(e.field, CodePathRequired, "the path cannot be empty")
			continue
		}
		if reason, ok := relativePathProblem(*e.value); !ok {
			invalid.add(e.field, CodePathNotRelative, reason)
		}
	}
	return paths
}

// checkWorktree validates the per-spec worktree settings. With the workflow
// off, empty values are the defaults and not an error: nothing reads them.
func checkWorktree(worktree domain.WorktreeConfig, invalid *ValidationError) domain.WorktreeConfig {
	defaults := config.Default().Worktree
	worktree.Base = strings.TrimSpace(worktree.Base)
	worktree.Dir = strings.TrimSpace(worktree.Dir)
	worktree.BranchPrefix = strings.TrimSpace(worktree.BranchPrefix)

	if !worktree.Enabled {
		if worktree.Base == "" {
			worktree.Base = defaults.Base
		}
		if worktree.Dir == "" {
			worktree.Dir = defaults.Dir
		}
		if worktree.BranchPrefix == "" {
			worktree.BranchPrefix = defaults.BranchPrefix
		}
		return worktree
	}

	if worktree.Base == "" {
		invalid.add("worktree.base", CodeWorktreeFieldNeeded, "indicate the base branch the worktrees fork from")
	}
	if worktree.BranchPrefix == "" {
		invalid.add("worktree.branch_prefix", CodeWorktreeFieldNeeded, "indicate the prefix of the per-spec branches")
	}
	if worktree.Dir == "" {
		invalid.add("worktree.dir", CodeWorktreeDirInvalid, "indicate the directory that holds the worktrees")
	} else if reason, ok := relativePathProblem(worktree.Dir); !ok {
		invalid.add("worktree.dir", CodeWorktreeDirInvalid, reason)
	}
	return worktree
}

// relativePathProblem reports whether a configured path stays inside the
// workspace. Absolute paths and `..` segments both escape it, which is how a
// workspace ends up writing where nobody asked it to.
func relativePathProblem(value string) (string, bool) {
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return "the path must be relative to the workspace, not absolute", false
	}
	cleaned := filepath.ToSlash(filepath.Clean(value))
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == ".." {
			return "the path cannot climb outside the workspace with `..`", false
		}
	}
	return "", true
}
