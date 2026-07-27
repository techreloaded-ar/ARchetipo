package wiki

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
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
	root                 string
	embeddedRepositories map[string]bool
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
	return &evidencePathResolver{
		root:                 filepath.Clean(physical),
		embeddedRepositories: map[string]bool{},
	}, nil
}

func (resolver *evidencePathResolver) normalize(sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" || strings.IndexByte(sourcePath, 0) >= 0 {
		return "", &EvidencePathError{Source: sourcePath, Err: ErrInvalidSourcePath}
	}

	// Normalize both separators regardless of the host so a path authored on
	// one supported OS cannot become an escape when consumed on another.
	portable := strings.ReplaceAll(sourcePath, `\`, "/")
	if pathpkg.IsAbs(portable) || strings.HasPrefix(portable, "//") || hasWindowsVolumePrefix(portable) || filepath.IsAbs(sourcePath) || filepath.VolumeName(sourcePath) != "" {
		return "", &EvidencePathError{Source: sourcePath, Err: ErrInvalidSourcePath}
	}
	clean := pathpkg.Clean(portable)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", &EvidencePathError{Source: sourcePath, Err: ErrUnsafeSourcePath}
	}
	return clean, nil
}

func hasWindowsVolumePrefix(path string) bool {
	return len(path) >= 2 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':'
}

// resolve walks every existing component with Lstat. Terminal symlinks are
// returned as links and may be fingerprinted with Readlink; intermediate
// symlinks and platform path-redirection objects are rejected.
//
// These component checks reduce accidental traversal and deterministic local
// redirection attacks, but they do not eliminate TOCTOU races with a process
// concurrently replacing filesystem entries. A race-free hostile-filesystem
// sandbox would require descriptor-relative, platform-specific APIs.
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
		candidate = filepath.Join(candidate, filepath.FromSlash(component))
		componentRel := strings.Join(components[:index+1], "/")
		info, statErr := os.Lstat(candidate)
		if errors.Is(statErr, fs.ErrNotExist) {
			return resolvedEvidencePath{Relative: relative, Path: filepath.Join(resolver.root, filepath.FromSlash(relative))}, nil
		}
		if statErr != nil {
			return resolvedEvidencePath{}, &EvidencePathError{Source: sourcePath, Component: componentRel, Err: errors.Join(ErrEvidenceUnreadable, statErr)}
		}
		terminal := index == len(components)-1
		if platformErr := validatePlatformEvidenceComponent(candidate, terminal); platformErr != nil {
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
