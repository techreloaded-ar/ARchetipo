package wiki

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitCommandRunner is the narrow operation-local seam used to measure Git
// setup work deterministically. Production uses execGitCommandRunner; tests may
// inject a counter without changing command semantics.
type gitCommandRunner interface {
	Output(dir string, stdin io.Reader, args ...string) ([]byte, error)
}

type execGitCommandRunner struct{}

var newGitCommandRunner = func() gitCommandRunner { return execGitCommandRunner{} }

func (execGitCommandRunner) Output(dir string, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	return cmd.Output()
}

// evidenceSnapshot owns setup shared by one top-level Wiki operation. It does
// not cache per-file identities, directory membership, or page fingerprints.
type evidenceSnapshot struct {
	projectRoot string
	wikiRoot    string
	resolver    *evidencePathResolver
	runner      gitCommandRunner
	gitWorktree bool
	index       *gitIndex
	indexLoaded bool
	indexErr    error
}

func newEvidenceSnapshot(projectRoot, wikiRoot string) (*evidenceSnapshot, error) {
	return newEvidenceSnapshotWithRunner(projectRoot, wikiRoot, newGitCommandRunner())
}

func newEvidenceSnapshotWithRunner(projectRoot, wikiRoot string, runner gitCommandRunner) (*evidenceSnapshot, error) {
	resolver, err := newEvidencePathResolver(projectRoot)
	if err != nil {
		return nil, err
	}
	physicalWikiRoot, err := filepath.EvalSymlinks(wikiRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: resolving Wiki root: %v", ErrEvidenceUnreadable, err)
	}
	physicalWikiRoot, err = filepath.Abs(physicalWikiRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: resolving Wiki root: %v", ErrEvidenceUnreadable, err)
	}
	snapshot := &evidenceSnapshot{
		projectRoot: resolver.root,
		wikiRoot:    physicalWikiRoot,
		resolver:    resolver,
		runner:      runner,
	}
	out, probeErr := runner.Output(snapshot.projectRoot, nil, "rev-parse", "--is-inside-work-tree")
	snapshot.gitWorktree = probeErr == nil && strings.TrimSpace(string(out)) == "true"
	return snapshot, nil
}

func (snapshot *evidenceSnapshot) gitIndex() (*gitIndex, error) {
	if !snapshot.gitWorktree {
		return nil, nil
	}
	if !snapshot.indexLoaded {
		snapshot.indexLoaded = true
		snapshot.index, snapshot.indexErr = loadGitIndexWithRunner(snapshot.projectRoot, snapshot.runner)
	}
	return snapshot.index, snapshot.indexErr
}

func (snapshot *evidenceSnapshot) resolveAndAttest(sourcePath string) (resolvedEvidencePath, error) {
	resolved, err := snapshot.resolver.resolve(sourcePath)
	if err != nil {
		return resolvedEvidencePath{}, err
	}
	index, err := snapshot.gitIndex()
	if err != nil {
		return resolvedEvidencePath{}, err
	}
	if index != nil {
		if err := index.validatePathIdentity(resolved.Relative); err != nil {
			return resolvedEvidencePath{}, err
		}
	}
	return resolved, nil
}
