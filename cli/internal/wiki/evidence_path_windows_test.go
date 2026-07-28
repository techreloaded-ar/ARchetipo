//go:build windows

package wiki

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsReparseClassification(t *testing.T) {
	const (
		unknownNameSurrogate = uint32(0xA000001D)
		unknownReparse       = uint32(0x8000001E)
	)
	tests := []struct {
		name       string
		attributes uint32
		tag        uint32
		terminal   bool
		want       error
	}{
		{name: "ordinary intermediate", terminal: false},
		{name: "ordinary terminal", terminal: true},
		{name: "terminal symlink", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: windows.IO_REPARSE_TAG_SYMLINK, terminal: true},
		{name: "intermediate symlink", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: windows.IO_REPARSE_TAG_SYMLINK, terminal: false, want: ErrUnsafeSourcePath},
		{name: "terminal junction or mount point", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: windows.IO_REPARSE_TAG_MOUNT_POINT, terminal: true, want: ErrUnsafeSourcePath},
		{name: "terminal unknown name surrogate", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: unknownNameSurrogate, terminal: true, want: ErrUnsafeSourcePath},
		{name: "terminal unsupported reparse", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: unknownReparse, terminal: true, want: ErrUnsupportedEvidenceEntry},
		{name: "intermediate unsupported reparse", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: unknownReparse, terminal: false, want: ErrUnsafeSourcePath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyWindowsReparse(test.attributes, test.tag, test.terminal)
			if !errors.Is(err, test.want) || test.want == nil && err != nil {
				t.Fatalf("classifyWindowsReparse() error=%v want=%v", err, test.want)
			}
		})
	}
}

func TestWindowsEvidencePathCanonicalCasingAndADS(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "ExactName.txt"), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("ExactName.txt"); err != nil {
		t.Fatalf("exact path failed: %v", err)
	}
	if _, err := resolver.resolve("exactname.txt"); !errors.Is(err, ErrInvalidSourcePath) {
		t.Fatalf("wrong-case path error=%v", err)
	}
	// NTFS alternate data streams are real native aliases. The resolver must
	// reject the stream spelling lexically before it can be opened as evidence.
	if err := os.WriteFile(filepath.Join(project, "ExactName.txt:secret"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("creating mandatory ADS fixture: %v", err)
	}
	for _, source := range []string{"ExactName.txt:secret", "CON.txt", "LPT¹.log", "trailing.", "trailing "} {
		if _, err := resolver.resolve(source); !errors.Is(err, ErrInvalidSourcePath) {
			t.Fatalf("portable Windows alias %q error=%v", source, err)
		}
	}
}

func TestWindowsGitIndexPortableCollision(t *testing.T) {
	index := portableTestIndex("Case.txt", "case.txt")
	if _, err := index.entry("Case.txt"); !errors.Is(err, errPortablePathCollision) {
		t.Fatalf("manufactured index collision error=%v", err)
	}
}

func TestWindowsEvidencePathSupportsLongCanonicalPaths(t *testing.T) {
	project := t.TempDir()
	parts := make([]string, 10)
	for index := range parts {
		parts[index] = strings.Repeat(string(rune('a'+index)), 30)
	}
	directory := filepath.Join(append([]string{project}, parts...)...)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("creating long path: %v", err)
	}
	file := filepath.Join(directory, "evidence.txt")
	if err := os.WriteFile(file, []byte("long path evidence"), 0o644); err != nil {
		t.Fatalf("writing long path: %v", err)
	}
	relative, err := filepath.Rel(project, file)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve(filepath.ToSlash(relative)); err != nil {
		t.Fatalf("resolving long path: %v", err)
	}
}

func TestWindowsEvidencePathRejectsJunction(t *testing.T) {
	project := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "sentinel.txt"), []byte("must not be reached"), 0o644); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(project, "junction")
	output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, target).CombinedOutput()
	if err != nil {
		t.Fatalf("creating mandatory junction fixture: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if err := os.Remove(junction); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("removing junction without traversing it: %v", err)
		}
	})
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve("junction"); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("terminal junction error=%v", err)
	}
	if _, err := resolver.resolve("junction/sentinel.txt"); !errors.Is(err, ErrUnsafeSourcePath) {
		t.Fatalf("junction traversal error=%v", err)
	}
}

func TestWindowsEvidencePathRejectsAvailableShortNameAlias(t *testing.T) {
	project := t.TempDir()
	longName := "EvidenceDirectoryWithLongName"
	path := filepath.Join(project, longName)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("cmd", "/c", "for", "%I", "in", "(\""+path+"\")", "do", "@echo", "%~sI").CombinedOutput()
	if err != nil {
		t.Fatalf("8.3 capability probe failed unexpectedly: %v: %s", err, output)
	}
	shortPath := strings.TrimSpace(string(output))
	if !strings.Contains(filepath.Base(shortPath), "~") {
		t.Skip("8.3 short names are disabled on this volume")
	}
	alias, err := filepath.Rel(project, shortPath)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := newEvidencePathResolver(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve(filepath.ToSlash(alias)); !errors.Is(err, ErrInvalidSourcePath) {
		t.Fatalf("8.3 alias error=%v", err)
	}
}
