package wiki

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func fingerprintGitlink(manifest *evidenceManifest, rel string, entry *gitIndexEntry, resolved resolvedEvidencePath) error {
	block := func(reason string, cause error) error {
		if cause != nil {
			return fmt.Errorf("%w: %s: %s: %v", ErrSubmoduleEvidence, rel, reason, cause)
		}
		return fmt.Errorf("%w: %s: %s", ErrSubmoduleEvidence, rel, reason)
	}
	if !resolved.Exists {
		return block("submodule worktree is missing or uninitialized", nil)
	}
	if resolved.Info == nil || !resolved.Info.IsDir() {
		return block("gitlink is not an ordinary directory", nil)
	}

	top, err := gitEmbeddedOutput(resolved.Path, "rev-parse", "--show-toplevel")
	if err != nil || !samePhysicalPath(strings.TrimSpace(string(top)), resolved.Path) {
		return block("submodule worktree is missing or uninitialized", err)
	}
	headRaw, err := gitEmbeddedOutput(resolved.Path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return block("submodule HEAD is unavailable", err)
	}
	head := strings.ToLower(strings.TrimSpace(string(headRaw)))
	indexOID := strings.ToLower(strings.TrimSpace(entry.OID))
	if head == "" || head != indexOID {
		return block("checked-out HEAD does not match the stage-0 gitlink", nil)
	}
	status, err := gitEmbeddedOutput(
		resolved.Path,
		"status",
		"--porcelain=v2",
		"-z",
		"--untracked-files=all",
		"--ignore-submodules=none",
	)
	if err != nil {
		return block("submodule status is unavailable", err)
	}
	if len(status) != 0 {
		return block("submodule has tracked, untracked, conflicted, or nested changes", nil)
	}

	manifest.record("gitlink", rel, indexOID, head)
	return nil
}

func samePhysicalPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftPhysical, leftErr := filepath.EvalSymlinks(left)
	rightPhysical, rightErr := filepath.EvalSymlinks(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftAbsolute, leftErr := filepath.Abs(leftPhysical)
	rightAbsolute, rightErr := filepath.Abs(rightPhysical)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftAbsolute = filepath.Clean(leftAbsolute)
	rightAbsolute = filepath.Clean(rightAbsolute)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftAbsolute, rightAbsolute)
	}
	return leftAbsolute == rightAbsolute
}
