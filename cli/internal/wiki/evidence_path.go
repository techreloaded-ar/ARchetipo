package wiki

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EvidencePathError identifies a source path and, when available, the
// project-relative component that could not be inspected safely. It never
// includes a resolved path outside the project root.
type EvidencePathError struct {
	Source    string
	Component string
	Err       error
}

func (err *EvidencePathError) Error() string {
	if err.Component != "" && err.Component != err.Source {
		return fmt.Sprintf("evidence source %q at %q: %v", err.Source, err.Component, err.Err)
	}
	return fmt.Sprintf("evidence source %q: %v", err.Source, err.Err)
}

func (err *EvidencePathError) Unwrap() error { return err.Err }

type resolvedEvidencePath struct {
	Relative string
	Path     string
	Info     fs.FileInfo
	Exists   bool
}

type evidencePathResolver struct {
	root         string
	platformRoot platformEvidenceRoot
}

func newEvidencePathResolver(projectRoot string) (*evidencePathResolver, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return nil, &EvidencePathError{Source: projectRoot, Err: ErrInvalidSourcePath}
	}
	absolute, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, &EvidencePathError{Source: projectRoot, Err: errors.Join(ErrEvidenceUnreadable, err)}
	}
	// The configured root may itself be a symlink or Windows junction. Resolve
	// that trusted alias once, then reject redirection below the physical anchor.
	physical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, &EvidencePathError{Source: projectRoot, Err: errors.Join(ErrEvidenceUnreadable, err)}
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return nil, &EvidencePathError{Source: projectRoot, Err: errors.Join(ErrEvidenceUnreadable, err)}
	}
	info, err := os.Lstat(physical)
	if err != nil {
		return nil, &EvidencePathError{Source: projectRoot, Err: errors.Join(ErrEvidenceUnreadable, err)}
	}
	if !info.IsDir() {
		return nil, &EvidencePathError{Source: projectRoot, Err: errors.Join(ErrInvalidSourcePath, errors.New("project root is not a directory"))}
	}
	platformRoot, err := newPlatformEvidenceRoot(physical)
	if err != nil {
		return nil, &EvidencePathError{Source: projectRoot, Err: err}
	}
	return &evidencePathResolver{
		root:         filepath.Clean(physical),
		platformRoot: platformRoot,
	}, nil
}

func (resolver *evidencePathResolver) normalize(sourcePath string) (string, error) {
	normalized, err := normalizePortableEvidencePath(sourcePath)
	if err != nil {
		return "", &EvidencePathError{Source: sourcePath, Err: err}
	}
	return normalized, nil
}

func hasWindowsVolumePrefix(path string) bool {
	return len(path) >= 2 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':'
}

func (resolver *evidencePathResolver) directoryEntries(path string) ([]fs.DirEntry, error) {
	// Phase A shares resolver/root identity only. Directory membership is read
	// afresh so changes during an operation are never hidden across pages.
	return os.ReadDir(path)
}

// attestComponent requires the authored component to match the exact spelling
// returned by the parent directory. Portable-equivalent sibling names are
// ambiguous on at least one supported host and therefore fail closed.
func (resolver *evidencePathResolver) attestComponent(parent, component string) (bool, error) {
	entries, err := resolver.directoryEntries(parent)
	if err != nil {
		return false, errors.Join(ErrEvidenceUnreadable, err)
	}
	key, err := portableComponentKey(component)
	if err != nil {
		return false, err
	}
	exact := false
	equivalent := make([]string, 0, 1)
	for _, entry := range entries {
		entryKey, keyErr := portableComponentKey(entry.Name())
		if keyErr != nil {
			// An unrelated non-portable sibling does not poison the directory.
			continue
		}
		if entryKey == key {
			equivalent = append(equivalent, entry.Name())
			if entry.Name() == component {
				exact = true
			}
		}
	}
	if len(equivalent) > 1 {
		return false, errors.Join(ErrEvidenceUnreadable, errPortablePathCollision)
	}
	if exact {
		return true, nil
	}
	if len(equivalent) != 0 {
		return false, errors.Join(ErrInvalidSourcePath, errNonCanonicalPath)
	}

	// Windows 8.3 names and other filesystem aliases need not appear in a
	// directory listing. If such an alias opens, reject it instead of treating
	// it as a missing untracked source.
	candidate := filepath.Join(parent, filepath.FromSlash(component))
	if _, statErr := os.Lstat(candidate); statErr == nil {
		return false, errors.Join(ErrInvalidSourcePath, errNonCanonicalPath)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return false, errors.Join(ErrEvidenceUnreadable, statErr)
	}
	return false, nil
}

// resolve walks and attests every existing component without following a
// terminal link. Intermediate symlinks, reparses, mount crossings, spelling
// aliases, and portable-equivalent sibling collisions fail closed.
//
// These checks reduce accidental traversal and deterministic local redirection
// attacks, but they do not claim a hostile-filesystem sandbox against a process
// concurrently replacing filesystem entries.
func (resolver *evidencePathResolver) resolve(sourcePath string) (resolvedEvidencePath, error) {
	relative, err := resolver.normalize(sourcePath)
	if err != nil {
		return resolvedEvidencePath{}, err
	}
	candidate := resolver.root
	components := []string{}
	if relative != "." {
		components = strings.Split(relative, "/")
	}
	for index, component := range components {
		componentRel := strings.Join(components[:index+1], "/")
		exists, attestErr := resolver.attestComponent(candidate, component)
		if attestErr != nil {
			return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Component: componentRel, Err: attestErr}
		}
		candidate = filepath.Join(candidate, filepath.FromSlash(component))
		if !exists {
			return resolvedEvidencePath{Relative: relative, Path: filepath.Join(resolver.root, filepath.FromSlash(relative))}, nil
		}
		info, statErr := os.Lstat(candidate)
		if statErr != nil {
			return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Component: componentRel, Err: errors.Join(ErrEvidenceUnreadable, statErr)}
		}
		terminal := index == len(components)-1
		if platformErr := validatePlatformEvidenceComponent(resolver.platformRoot, candidate, component, terminal); platformErr != nil {
			return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Component: componentRel, Err: platformErr}
		}
		if info.Mode()&os.ModeSymlink != 0 && !terminal {
			return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Component: componentRel, Err: ErrUnsafeSourcePath}
		}
		if !terminal && !info.IsDir() {
			return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Component: componentRel, Err: errors.Join(ErrEvidenceUnreadable, errors.New("intermediate component is not a directory"))}
		}
		if terminal {
			return resolvedEvidencePath{Relative: relative, Path: candidate, Info: info, Exists: true}, nil
		}
	}

	info, err := os.Lstat(resolver.root)
	if err != nil {
		return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Err: errors.Join(ErrEvidenceUnreadable, err)}
	}
	return resolvedEvidencePath{Relative: relative, Path: resolver.root, Info: info, Exists: true}, nil
}
