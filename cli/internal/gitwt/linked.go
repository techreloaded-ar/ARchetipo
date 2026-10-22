package gitwt

import (
	"context"
	"path/filepath"
)

// IsLinkedWorktree identifies an actual secondary checkout of this repository,
// rather than treating an arbitrary conversation directory as disposable work.
func IsLinkedWorktree(ctx context.Context, root, directory string) bool {
	root, rootErr := filepath.EvalSymlinks(root)
	directory, dirErr := filepath.EvalSymlinks(directory)
	if rootErr != nil || dirErr != nil || root == directory {
		return false
	}
	top, err := runGit(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || top != directory {
		return false
	}
	parentCommon, err := runGit(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false
	}
	common, err := runGit(ctx, directory, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false
	}
	parentCommon, parentErr := filepath.EvalSymlinks(parentCommon)
	common, err = filepath.EvalSymlinks(common)
	return err == nil && parentErr == nil && common == parentCommon
}
