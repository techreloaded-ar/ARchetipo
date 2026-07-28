package wiki

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (resolver *evidencePathResolver) embeddedRepositoryBoundary(sourcePath string) (string, error) {
	relative, err := resolver.normalize(sourcePath)
	if err != nil {
		return "", err
	}
	if relative == "." {
		return "", nil
	}

	components := strings.Split(relative, "/")
	for count := 1; count <= len(components); count++ {
		boundary := strings.Join(components[:count], "/")
		candidate := filepath.Join(resolver.root, filepath.FromSlash(boundary))
		info, statErr := os.Lstat(candidate)
		if errors.Is(statErr, fs.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", &EvidencePathError{Source: sourcePath, Component: boundary, Err: errors.Join(ErrEvidenceUnreadable, statErr)}
		}
		if !info.IsDir() {
			break
		}

		suspected, suspectErr := suspectedEmbeddedRepository(candidate)
		if suspectErr != nil {
			return "", &EvidencePathError{Source: sourcePath, Component: boundary, Err: errors.Join(ErrEvidenceUnreadable, suspectErr)}
		}
		embedded := false
		if suspected {
			embedded, err = confirmedEmbeddedRepository(candidate)
			if err != nil {
				return "", &EvidencePathError{Source: sourcePath, Component: boundary, Err: errors.Join(ErrEvidenceUnreadable, err)}
			}
		}
		if embedded {
			return boundary, nil
		}
	}
	return "", nil
}

func suspectedEmbeddedRepository(candidate string) (bool, error) {
	if _, err := os.Lstat(filepath.Join(candidate, ".git")); err == nil {
		return true, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	head, headErr := os.Lstat(filepath.Join(candidate, "HEAD"))
	if headErr != nil {
		if errors.Is(headErr, fs.ErrNotExist) {
			return false, nil
		}
		return false, headErr
	}
	if !head.Mode().IsRegular() && head.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}

	// Bare repository layouts are not limited to loose refs. Reftable and
	// future layouts still carry HEAD plus repository metadata. Suspicion is
	// deliberately broad; Git confirmation below is authoritative and closed.
	for _, marker := range []string{"objects", "refs", "packed-refs", "reftable", "config", "commondir"} {
		if _, err := os.Lstat(filepath.Join(candidate, marker)); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func confirmedEmbeddedRepository(candidate string) (bool, error) {
	bare, err := gitEmbeddedOutput(candidate, "rev-parse", "--is-bare-repository")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(bare)) == "true" {
		gitDir, gitDirErr := gitEmbeddedOutput(candidate, "rev-parse", "--absolute-git-dir")
		if gitDirErr != nil {
			return false, gitDirErr
		}
		return samePhysicalPath(strings.TrimSpace(string(gitDir)), candidate), nil
	}

	top, err := gitEmbeddedOutput(candidate, "rev-parse", "--show-toplevel")
	if err != nil {
		return false, err
	}
	return samePhysicalPath(strings.TrimSpace(string(top)), candidate), nil
}

func gitEmbeddedOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return out, nil
}
