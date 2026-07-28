package wiki

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

type operationCommandCounter struct {
	delegate        gitCommandRunner
	repositoryProbe int
	indexLoad       int
	cleanHash       int
	untrackedList   int
}

func (counter *operationCommandCounter) Output(dir string, stdin io.Reader, args ...string) ([]byte, error) {
	switch {
	case equalGitArgs(args, "rev-parse", "--is-inside-work-tree"):
		counter.repositoryProbe++
	case equalGitArgs(args, "ls-files", "--stage", "-z"):
		counter.indexLoad++
	case len(args) >= 2 && args[0] == "hash-object" && args[1] == "--stdin":
		counter.cleanHash++
	case len(args) >= 3 && args[0] == "ls-files" && args[1] == "-z" && args[2] == "--others":
		counter.untrackedList++
	}
	return counter.delegate.Output(dir, stdin, args...)
}

func equalGitArgs(got []string, want ...string) bool {
	return strings.Join(got, "\x00") == strings.Join(want, "\x00")
}

type operationFixture struct {
	project string
	root    string
}

func newOperationFixture(tb testing.TB, reviewed bool) operationFixture {
	tb.Helper()
	project := tb.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		tb.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "evidence"), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "shared.txt"), []byte("shared\n"), 0o644); err != nil {
		tb.Fatal(err)
	}
	writeSimplePageTB(tb, root, "direct", "evidence/shared.txt")
	writeSimplePageTB(tb, root, "directory", "evidence")
	gitTB(tb, project, "init", "-q")
	gitTB(tb, project, "config", "user.email", "wiki-test@example.test")
	gitTB(tb, project, "config", "user.name", "Wiki Test")
	gitTB(tb, project, "add", "evidence/shared.txt")
	gitTB(tb, project, "commit", "-qm", "evidence baseline")
	if reviewed {
		if approved, err := Approve(project, root, []string{"direct", "directory"}); err != nil || approved != 2 {
			tb.Fatalf("approve fixture: approved=%d err=%v", approved, err)
		}
	}
	return operationFixture{project: project, root: root}
}

func writeSimplePageTB(tb testing.TB, root, id, source string) {
	tb.Helper()
	page := Page{ID: id, Path: id + ".md", Meta: testPageMeta(id, source), Body: "# " + id + "\n"}
	raw, err := renderPage(page)
	if err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, page.Path), raw, 0o644); err != nil {
		tb.Fatal(err)
	}
}

func testPageMeta(id, source string) domain.WikiPageMeta {
	return domain.WikiPageMeta{
		Type:        "guide",
		Title:       id,
		Description: id + " description",
		Status:      domain.WikiStatusGenerated,
		Sources:     []domain.WikiSource{{Path: source}},
	}
}

func gitTB(tb testing.TB, dir string, args ...string) {
	tb.Helper()
	if out, err := (execGitCommandRunner{}).Output(dir, nil, args...); err != nil {
		tb.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, out)
	}
}

func withOperationCounter(t *testing.T) *operationCommandCounter {
	t.Helper()
	counter := &operationCommandCounter{delegate: execGitCommandRunner{}}
	previous := newGitCommandRunner
	newGitCommandRunner = func() gitCommandRunner { return counter }
	t.Cleanup(func() { newGitCommandRunner = previous })
	return counter
}

func assertOperationSetupCounts(t *testing.T, counter *operationCommandCounter) {
	t.Helper()
	if counter.repositoryProbe != 1 {
		t.Fatalf("repository probes=%d want=1", counter.repositoryProbe)
	}
	if counter.indexLoad > 1 {
		t.Fatalf("Git index loads=%d want<=1", counter.indexLoad)
	}
}

func TestEvidenceResolverDoesNotCacheDirectoryMembership(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	before, err := resolver.resolve("evidence/created.txt")
	if err != nil || before.Exists {
		t.Fatalf("initial missing resolve: exists=%v err=%v", before.Exists, err)
	}
	if err := os.WriteFile(filepath.Join(project, "evidence", "created.txt"), []byte("created\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := resolver.resolve("evidence/created.txt")
	if err != nil || !after.Exists {
		t.Fatalf("second resolve reused stale membership: exists=%v err=%v", after.Exists, err)
	}
}

// TestEvidenceOperationLegacySetupBaseline reproduces the pre-Phase-A setup
// call graph: every page evidence pass owned its own resolver, repository probe,
// and index load. It intentionally exercises current fingerprint semantics with
// independent snapshots rather than preserving a second production evaluator.
func TestEvidenceOperationLegacySetupBaseline(t *testing.T) {
	tests := []struct {
		name   string
		passes int
	}{
		{name: "Validate", passes: 1},
		{name: "Status", passes: 2},
		{name: "Approve", passes: 2},
		{name: "Reconfirm", passes: 3},
		{name: "Catalog", passes: 1},
		{name: "SearchStatus", passes: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOperationFixture(t, true)
			pages, err := Load(fixture.root)
			if err != nil {
				t.Fatal(err)
			}
			counter := &operationCommandCounter{delegate: execGitCommandRunner{}}
			for pass := 0; pass < test.passes; pass++ {
				for _, page := range pages {
					snapshot, err := newEvidenceSnapshotWithRunner(fixture.project, fixture.root, counter)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := evidenceFingerprintWithSnapshot(snapshot, page); err != nil {
						t.Fatal(err)
					}
				}
			}
			wantSetup := test.passes * len(pages)
			if counter.repositoryProbe != wantSetup || counter.indexLoad != wantSetup {
				t.Fatalf("legacy setup probes=%d indexes=%d want=%d", counter.repositoryProbe, counter.indexLoad, wantSetup)
			}
			if counter.cleanHash != test.passes*2 || counter.untrackedList != test.passes {
				t.Fatalf("legacy evidence hashes=%d listings=%d want hashes=%d listings=%d", counter.cleanHash, counter.untrackedList, test.passes*2, test.passes)
			}
			t.Logf("legacy commands: repository_probe=%d index_load=%d clean_hash=%d untracked_list=%d", counter.repositoryProbe, counter.indexLoad, counter.cleanHash, counter.untrackedList)
		})
	}
}

func TestEvidenceOperationCommandCount(t *testing.T) {
	tests := []struct {
		name          string
		reviewed      bool
		cleanHashes   int
		untrackedList int
		run           func(operationFixture) error
	}{
		{name: "Validate", reviewed: true, cleanHashes: 2, untrackedList: 1, run: func(f operationFixture) error {
			report := Validate(f.project, f.root)
			if !report.OK {
				return ErrValidationFailed
			}
			return nil
		}},
		{name: "Status", reviewed: true, cleanHashes: 4, untrackedList: 2, run: func(f operationFixture) error { _, _, _, err := Status(f.project, f.root); return err }},
		{name: "Approve", cleanHashes: 4, untrackedList: 2, run: func(f operationFixture) error {
			_, err := Approve(f.project, f.root, []string{"direct", "directory"})
			return err
		}},
		{name: "Reconfirm", reviewed: true, cleanHashes: 6, untrackedList: 3, run: func(f operationFixture) error {
			if err := os.WriteFile(filepath.Join(f.project, "evidence", "shared.txt"), []byte("changed\n"), 0o644); err != nil {
				return err
			}
			_, err := Reconfirm(f.project, f.root, []string{"direct", "directory"})
			return err
		}},
		{name: "Catalog", reviewed: true, cleanHashes: 2, untrackedList: 1, run: func(f operationFixture) error { _, err := Catalog(f.project, f.root); return err }},
		{name: "SearchStatus", reviewed: true, cleanHashes: 2, untrackedList: 1, run: func(f operationFixture) error { _, err := Search(f.project, f.root, "", "", "reviewed"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOperationFixture(t, test.reviewed)
			counter := withOperationCounter(t)
			if err := test.run(fixture); err != nil {
				t.Fatal(err)
			}
			assertOperationSetupCounts(t, counter)
			if counter.indexLoad != 1 {
				t.Fatalf("Git index loads=%d want=1", counter.indexLoad)
			}
			if counter.cleanHash != test.cleanHashes || counter.untrackedList != test.untrackedList {
				t.Fatalf("evidence commands: hashes=%d listings=%d want hashes=%d listings=%d", counter.cleanHash, counter.untrackedList, test.cleanHashes, test.untrackedList)
			}
			t.Logf("commands: repository_probe=%d index_load=%d clean_hash=%d untracked_list=%d", counter.repositoryProbe, counter.indexLoad, counter.cleanHash, counter.untrackedList)
		})
	}
}

func BenchmarkWikiEvidence(b *testing.B) {
	fixture := newOperationFixture(b, true)
	pages, err := Load(fixture.root)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := evidenceFingerprint(fixture.project, fixture.root, pages[0]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWikiValidate(b *testing.B) {
	fixture := newOperationFixture(b, true)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if report := Validate(fixture.project, fixture.root); !report.OK {
			b.Fatal(report.Findings)
		}
	}
}

func BenchmarkWikiStatus(b *testing.B) {
	fixture := newOperationFixture(b, true)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, _, _, err := Status(fixture.project, fixture.root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWikiApprove(b *testing.B) {
	fixture := newOperationFixture(b, true)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := Approve(fixture.project, fixture.root, []string{"direct", "directory"}); err != nil {
			b.Fatal(err)
		}
	}
}
