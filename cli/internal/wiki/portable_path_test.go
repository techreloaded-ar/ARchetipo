package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"golang.org/x/text/unicode/norm"
)

func TestPortablePathGoldenVectors(t *testing.T) {
	valid := map[string]string{
		".":                        ".",
		"./":                       ".",
		`.\`:                       ".",
		`docs\architecture\map.md`: "docs/architecture/map.md",
		"docs/name with space.md":  "docs/name with space.md",
		"docs/interior.dot/name~1": "docs/interior.dot/name~1",
		"資料/Überblick.md":          "資料/Überblick.md",
		"./docs/item.md":           "docs/item.md",
		"docs//item.md":            "docs/item.md",
		"docs/./item.md":           "docs/item.md",
		"docs/item.md/":            "docs/item.md",
	}
	for input, expected := range valid {
		t.Run("valid_"+strings.ReplaceAll(input, "/", "_"), func(t *testing.T) {
			actual, err := normalizePortableEvidencePath(input)
			if err != nil || actual != expected {
				t.Fatalf("normalizePortableEvidencePath(%q)=(%q, %v), want (%q, nil)", input, actual, err, expected)
			}
		})
	}

	invalid := []string{
		"", " ", "\x00", "control\x1f.txt", "delete\x7f.txt",
		"/absolute", `\rooted`, `C:relative`, `C:\absolute`,
		`\\server\share\item`, `\\?\C:\item`, `\\.\NUL`, `\??\C:\item`,
		"name.", "name ",
		"stream:secret", `bad<name`, `bad>name`, `bad"name`, `bad|name`, `bad?name`, `bad*name`,
		"CON", "con.txt", "PRN.md", "aux", "NUL.json", "CLOCK$", "CONIN$", "CONOUT$.txt",
		"COM1", "com9.log", "LPT1", "lpt9.md", "COM¹.txt", "com²", "LPT³.log",
	}
	for _, input := range invalid {
		t.Run("invalid_"+strings.ReplaceAll(input, "/", "_"), func(t *testing.T) {
			if _, err := normalizePortableEvidencePath(input); !errors.Is(err, ErrInvalidSourcePath) {
				t.Fatalf("normalizePortableEvidencePath(%q) error=%v, want ErrInvalidSourcePath", input, err)
			}
		})
	}

	for _, input := range []string{"..", "../outside", `..\outside`, "nested/../../outside", `nested\..\..\outside`} {
		t.Run("unsafe_"+strings.ReplaceAll(input, "/", "_"), func(t *testing.T) {
			if _, err := normalizePortableEvidencePath(input); !errors.Is(err, ErrUnsafeSourcePath) {
				t.Fatalf("normalizePortableEvidencePath(%q) error=%v, want ErrUnsafeSourcePath", input, err)
			}
		})
	}
}

func TestPortableComponentCollisionKeyUsesNFCAndUnicodeCaseFold(t *testing.T) {
	composed := "Café"
	decomposed := norm.NFD.String(composed)
	keys := []string{"CAFÉ", "café", decomposed}
	var expected string
	for _, value := range keys {
		key, err := portableComponentKey(value)
		if err != nil {
			t.Fatal(err)
		}
		if expected == "" {
			expected = key
		} else if key != expected {
			t.Fatalf("portable key for %q=%q, want %q", value, key, expected)
		}
	}
	literal, err := portableComponentKey("name~1")
	if err != nil || literal == "" {
		t.Fatalf("literal ~1 component key=(%q, %v)", literal, err)
	}
}

func TestCanonicalEvidenceComponentRequiresStoredSpelling(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "ExactName.txt"), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("ExactName.txt"); err != nil {
		t.Fatalf("exact spelling failed: %v", err)
	}
	if _, err := resolver.resolve("exactname.txt"); !errors.Is(err, ErrInvalidSourcePath) {
		t.Fatalf("wrong-case spelling error=%v, want ErrInvalidSourcePath", err)
	}
}

func TestCanonicalEvidenceComponentRejectsUnicodeEquivalentAlias(t *testing.T) {
	project := t.TempDir()
	requested := "Café.txt"
	if err := os.WriteFile(filepath.Join(project, requested), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(project)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ReadDir entries=%v err=%v", entries, err)
	}
	stored := entries[0].Name()
	alias := norm.NFD.String(stored)
	if alias == stored {
		alias = norm.NFC.String(stored)
	}
	if alias == stored {
		t.Skip("filesystem spelling has no distinct NFC/NFD alias")
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve(stored); err != nil {
		t.Fatalf("stored spelling failed: %v", err)
	}
	if _, err := resolver.resolve(alias); !errors.Is(err, ErrInvalidSourcePath) {
		t.Fatalf("Unicode-equivalent alias error=%v, want ErrInvalidSourcePath", err)
	}
}

func TestEvidencePathAllowsTrustedProjectRootAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows trusted junction coverage is native-platform specific")
	}
	physical := t.TempDir()
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "project-alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	resolver, err := newEvidencePathResolver(alias)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("."); err != nil {
		t.Fatalf("trusted project root alias failed: %v", err)
	}
}

func TestEvidencePathAllowsHardLinks(t *testing.T) {
	project := t.TempDir()
	original := filepath.Join(project, "original.txt")
	linked := filepath.Join(project, "linked.txt")
	if err := os.WriteFile(original, []byte("hard-link evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Fatalf("creating hard-link fixture: %v", err)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.resolve("linked.txt")
	if err != nil {
		t.Fatalf("hard link rejected: %v", err)
	}
	content, err := readRegularEvidence(resolved.Path)
	if err != nil {
		t.Fatalf("reading hard-link evidence: %v", err)
	}
	if string(content) != "hard-link evidence" {
		t.Fatalf("hard-link content=%q", content)
	}
}

func TestPortableInvalidSourceBatchApprovalWritesNothing(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "good.txt"), []byte("good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "a-good", "good.txt")
	writeSimplePage(t, root, "b-invalid", "CON.txt")
	goodPath := filepath.Join(root, "a-good.md")
	before, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if approved, err := Approve(project, root, []string{"a-good", "b-invalid"}); approved != 0 || !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("invalid batch approval count=%d err=%v", approved, err)
	}
	after, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid batch approval mutated an earlier valid page")
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_INVALID_SOURCE") || hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("invalid portable source findings=%+v", report.Findings)
	}
}

func TestCanonicalWrongCaseReviewFailsClosed(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, "docs", "wiki")
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "Exact.txt"), []byte("evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSimplePage(t, root, "overview", "Exact.txt")
	if approved, err := Approve(project, root, []string{"overview"}); approved != 1 || err != nil {
		t.Fatalf("baseline approval count=%d err=%v", approved, err)
	}
	pages, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pages[0].Meta.Sources[0].Path = "exact.txt"
	pages[0].Meta.Review.ContentHash = pageContentHash(pages[0])
	raw, err := renderPage(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, pages[0].Path), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Validate(project, root)
	if report.OK || !hasFinding(report, "WIKI_INVALID_SOURCE") || hasFinding(report, "WIKI_EVIDENCE_CHANGED") {
		t.Fatalf("wrong-case review findings=%+v", report.Findings)
	}
	if pages[0].Meta.Status != domain.WikiStatusReviewed {
		t.Fatalf("test setup lost reviewed status: %s", pages[0].Meta.Status)
	}
}
