package wiki

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"golang.org/x/text/unicode/norm"
)

func portableTestIndex(paths ...string) *gitIndex {
	index := &gitIndex{byPath: map[string][]gitIndexEntry{}}
	for _, path := range paths {
		index.byPath[path] = []gitIndexEntry{{Mode: "100644", OID: strings.Repeat("a", 40), Stage: 0, Path: path}}
	}
	return index
}

func TestGitIndexPortableCollisionIsScopedAndExact(t *testing.T) {
	index := portableTestIndex("README.md", "Readme.md", "docs/file.txt", "other/Case.txt", "other/case.txt")
	if _, err := index.entry("README.md"); !errors.Is(err, errPortablePathCollision) || !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("direct collision error=%v", err)
	}
	if entry, err := index.entry("docs/file.txt"); err != nil || entry == nil || entry.Path != "docs/file.txt" {
		t.Fatalf("unrelated collision affected exact entry: entry=%+v err=%v", entry, err)
	}
}

func TestGitIndexPortableAliasDoesNotDowngradeToUntracked(t *testing.T) {
	index := portableTestIndex("Docs/Evidence.txt")
	for _, path := range []string{"docs/Evidence.txt", "Docs/evidence.txt"} {
		if entry, err := index.entry(path); entry != nil || !errors.Is(err, ErrInvalidSourcePath) {
			t.Fatalf("entry(%q)=(%+v, %v), want noncanonical invalid path", path, entry, err)
		}
	}
}

func TestGitIndexPortableAncestorCollisionBlocksDescendant(t *testing.T) {
	index := portableTestIndex("Docs/a.txt", "docs/b.txt")
	if _, err := index.entry("Docs/a.txt"); !errors.Is(err, errPortablePathCollision) {
		t.Fatalf("ancestor collision error=%v", err)
	}
	if _, err := index.pathsWithin("Docs"); !errors.Is(err, errPortablePathCollision) {
		t.Fatalf("directory collision error=%v", err)
	}
}

func TestGitIndexPortableUnicodeCollision(t *testing.T) {
	composed := "Café.txt"
	decomposed := norm.NFD.String(composed)
	if composed == decomposed {
		t.Fatal("test requires distinct NFC/NFD spellings")
	}
	index := portableTestIndex(composed, decomposed)
	if _, err := index.entry(composed); !errors.Is(err, errPortablePathCollision) {
		t.Fatalf("Unicode collision error=%v", err)
	}
}

func TestGitIndexDirectoryRejectsIntersectingNonPortablePathOnly(t *testing.T) {
	index := portableTestIndex("docs/good.txt", "docs/bad:name.txt", "unrelated/bad:name.txt")
	if _, err := index.pathsWithin("docs"); !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("intersecting non-portable path error=%v", err)
	}
	clean := portableTestIndex("docs/good.txt", "unrelated/bad:name.txt")
	if paths, err := clean.pathsWithin("docs"); err != nil || len(paths) != 1 || paths[0] != "docs/good.txt" {
		t.Fatalf("unrelated non-portable path affected directory: paths=%v err=%v", paths, err)
	}
}

func TestGitIndexInvalidDescendantStillContributesPortableAncestorCollision(t *testing.T) {
	index := portableTestIndex("docs/good.txt", "Docs/bad:name.txt")
	if _, err := index.entry("docs/good.txt"); !errors.Is(err, errPortablePathCollision) || !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("direct evidence missed collision hidden by invalid descendant: %v", err)
	}
	if _, err := index.pathsWithin("docs"); !errors.Is(err, errPortablePathCollision) || !errors.Is(err, ErrEvidenceUnreadable) {
		t.Fatalf("directory evidence missed collision hidden by invalid descendant: %v", err)
	}
}

func TestGitIndexInvalidAndCollidingPathsOutsideScopeDoNotPoisonEvidence(t *testing.T) {
	index := portableTestIndex(
		"docs/good.txt",
		"Other/bad:name.txt",
		"other/worse:name.txt",
		`separate/bad\name.txt`,
	)
	entry, err := index.entry("docs/good.txt")
	if err != nil || entry == nil || entry.Path != "docs/good.txt" {
		t.Fatalf("unrelated invalid/colliding paths affected direct evidence: entry=%+v err=%v", entry, err)
	}
	paths, err := index.pathsWithin("docs")
	if err != nil || len(paths) != 1 || paths[0] != "docs/good.txt" {
		t.Fatalf("unrelated invalid/colliding paths affected directory evidence: paths=%v err=%v", paths, err)
	}
}

func TestTrackedRegularUsesExactGitIndexPathForCleanFilter(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki@example.com")
	git(t, project, "config", "user.name", "Wiki Test")
	if err := os.WriteFile(filepath.Join(project, ".gitattributes"), []byte("Exact.txt text eol=lf\nexact.txt -text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "Exact.txt"), []byte("line\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", ".gitattributes", "Exact.txt")
	git(t, project, "commit", "-qm", "baseline")

	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "Exact.txt"}}}}
	if _, err := evidenceFingerprint(project, root, page); err != nil {
		t.Fatalf("exact tracked path failed: %v", err)
	}
	if _, err := evidenceFingerprint(project, root, Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "exact.txt"}}}}); !errors.Is(err, ErrInvalidSourcePath) {
		t.Fatalf("wrong-case tracked path error=%v", err)
	}
}

func TestPortableCollisionValidationAndReconfirmFailClosed(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki@example.com")
	git(t, project, "config", "user.name", "Wiki Test")
	git(t, project, "config", "core.ignorecase", "false")
	if err := os.WriteFile(filepath.Join(project, "Case.txt"), []byte("evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, project, "add", "Case.txt")
	writeSimplePage(t, root, "overview", "Case.txt")
	if approved, err := Approve(project, root, []string{"overview"}); err != nil || approved != 1 {
		t.Fatalf("baseline approval count=%d err=%v", approved, err)
	}
	command := exec.Command("git", "hash-object", "Case.txt")
	command.Dir = project
	rawOID, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	oid := strings.TrimSpace(string(rawOID))
	git(t, project, "update-index", "--add", "--cacheinfo", "100644", oid, "case.txt")

	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_EVIDENCE_UNREADABLE") || hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("portable collision findings=%+v", report.Findings)
	}
	before, err := os.ReadFile(filepath.Join(root, "overview.md"))
	if err != nil {
		t.Fatal(err)
	}
	if count, err := Reconfirm(project, root, []string{"overview"}); err == nil || count != 0 {
		t.Fatalf("collision reconfirm count=%d err=%v", count, err)
	}
	after, err := os.ReadFile(filepath.Join(root, "overview.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed reconfirm mutated reviewed metadata")
	}
}

func TestPortableCollisionInCaseSensitiveWorktree(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("fixture requires a case-sensitive temporary filesystem")
	}
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, project, "init", "-q")
	git(t, project, "config", "user.email", "wiki@example.com")
	git(t, project, "config", "user.name", "Wiki Test")
	for _, name := range []string{"Case.txt", "case.txt"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, project, "add", "Case.txt", "case.txt")
	page := Page{Meta: domain.WikiPageMeta{Sources: []domain.WikiSource{{Path: "Case.txt"}}}}
	if _, err := evidenceFingerprint(project, root, page); !errors.Is(err, errPortablePathCollision) {
		t.Fatalf("case-sensitive worktree collision error=%v", err)
	}
}
