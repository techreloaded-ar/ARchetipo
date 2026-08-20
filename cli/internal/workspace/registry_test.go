package workspace

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
)

func TestStateDirHonoursEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvStateDir, dir)
	got, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir() error = %v", err)
	}
	if got != dir {
		t.Fatalf("StateDir() = %q, want %q", got, dir)
	}

	t.Setenv(EnvStateDir, "")
	got, err = StateDir()
	if err != nil {
		t.Fatalf("StateDir() without override error = %v", err)
	}
	if got == "" {
		t.Fatal("StateDir() without override returned an empty path")
	}
	if filepath.Base(got) != "archetipo" {
		t.Fatalf("StateDir() = %q, want a path ending in archetipo", got)
	}
}

func TestEntryIDIsStableAndPathDerived(t *testing.T) {
	first := EntryID("/a/b")
	if first != EntryID("/a/b") {
		t.Fatal("EntryID is not stable across calls on the same path")
	}
	if first != EntryID("/a/b/") {
		t.Fatalf("EntryID(%q) = %q, want the same id as %q", "/a/b/", EntryID("/a/b/"), "/a/b")
	}
	if first == EntryID("/a/c") {
		t.Fatal("EntryID collided on two different paths")
	}
	if len(first) != 16 {
		t.Fatalf("len(EntryID) = %d, want 16", len(first))
	}
	if strings.Trim(first, "0123456789abcdef") != "" {
		t.Fatalf("EntryID = %q, want lowercase hexadecimal", first)
	}
}

func TestListMissingRegistryIsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "assente")
	r := &Registry{Dir: dir}
	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() on a missing registry error = %v, want nil", err)
	}
	if len(entries) != 0 {
		t.Fatalf("List() = %d entries, want 0", len(entries))
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Fatal("reading the registry created the state directory; it must appear on the first write only")
	}
}

// writeRegistryFile puts raw bytes where the registry expects its document, to
// reproduce a file this process did not write.
func writeRegistryFile(t *testing.T, r *Registry, content string) {
	t.Helper()
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(r.Path(), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestListCorruptRegistryIsEmpty(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	writeRegistryFile(t, r, "not json at all")

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() on a corrupt registry error = %v, want nil", err)
	}
	if len(entries) != 0 {
		t.Fatalf("List() = %d entries, want 0", len(entries))
	}
}

func TestListUnknownSchemaIsEmpty(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	writeRegistryFile(t, r, `{"schema":999,"workspaces":[{"id":"x","name":"n","path":"/p"}]}`)

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() on an unknown schema error = %v, want nil", err)
	}
	if len(entries) != 0 {
		t.Fatalf("List() = %d entries, want 0", len(entries))
	}
}

func TestSaveRecreatesCorruptRegistry(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	writeRegistryFile(t, r, "not json at all")

	if err := r.save([]Entry{{ID: "a", Name: "n", Path: "/p", LastOpenedAt: time.Now().UTC()}}); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "a" || entries[0].Path != "/p" || entries[0].Name != "n" {
		t.Fatalf("List() = %+v, want exactly the saved entry", entries)
	}

	b, err := os.ReadFile(r.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var doc registryFile
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("the rewritten registry is not valid JSON: %v", err)
	}
	if doc.Schema != registrySchema {
		t.Fatalf("schema = %d, want %d", doc.Schema, registrySchema)
	}
}

func TestSaveIsAtomicAndLeavesNoTemporary(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	if err := r.save([]Entry{{ID: "a", Name: "n", Path: "/p", LastOpenedAt: time.Now().UTC()}}); err != nil {
		t.Fatalf("save() error = %v", err)
	}
	names, err := os.ReadDir(r.Dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(names) != 1 || names[0].Name() != registryFileName {
		got := make([]string, 0, len(names))
		for _, n := range names {
			got = append(got, n.Name())
		}
		t.Fatalf("state directory = %v, want only %s", got, registryFileName)
	}
}

func TestListSortsByLastOpenedDesc(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	base := time.Now().UTC().Add(-time.Hour)
	if err := r.save([]Entry{
		{ID: "a", Name: "a", Path: "/a", LastOpenedAt: base},
		{ID: "b", Name: "b", Path: "/b", LastOpenedAt: base.Add(time.Minute)},
		{ID: "c", Name: "c", Path: "/c", LastOpenedAt: base.Add(2 * time.Minute)},
	}); err != nil {
		t.Fatalf("save() error = %v", err)
	}

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []string{"c", "b", "a"}
	if len(entries) != len(want) {
		t.Fatalf("List() = %d entries, want %d", len(entries), len(want))
	}
	for i, id := range want {
		if entries[i].ID != id {
			t.Fatalf("List()[%d].ID = %q, want %q (order is most recently opened first)", i, entries[i].ID, id)
		}
	}
}

// makeWorkspace builds a directory that looks like a real workspace — an
// `.archetipo/config.yaml` plus one ordinary file — so that Probe has
// something to answer about and the inventory has something to compare.
func makeWorkspace(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".archetipo"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.RelativePath), []byte("connector: file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile readme: %v", err)
	}
	return dir
}

// inventory is the recursive listing of a tree, used to assert that an
// operation on the registry left the workspace on disk byte-for-byte alone.
func inventory(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", root, err)
	}
	sort.Strings(paths)
	return paths
}

func assertSameInventory(t *testing.T, before, after []string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("inventory changed: %v -> %v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("inventory changed at %d: %q -> %q", i, before[i], after[i])
		}
	}
}

func TestTouchRegistersOutsideTheWorkspace(t *testing.T) {
	state := t.TempDir()
	ws := makeWorkspace(t, t.TempDir(), "alfa")
	before := inventory(t, ws)

	r := &Registry{Dir: state}
	entry, err := r.Touch(ws)
	if err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	if entry.Name != "alfa" {
		t.Fatalf("entry.Name = %q, want %q", entry.Name, "alfa")
	}
	if entry.Path != filepath.Clean(ws) {
		t.Fatalf("entry.Path = %q, want %q", entry.Path, filepath.Clean(ws))
	}
	if entry.LastOpenedAt.IsZero() {
		t.Fatal("entry.LastOpenedAt is zero, want the moment of registration")
	}
	if _, err := os.Stat(filepath.Join(state, registryFileName)); err != nil {
		t.Fatalf("the registry file was not written in the state directory: %v", err)
	}
	assertSameInventory(t, before, inventory(t, ws))
}

func TestTouchIsIdempotentAndAdvancesLastOpened(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	ws := makeWorkspace(t, t.TempDir(), "beta")

	first, err := r.Touch(ws)
	if err != nil {
		t.Fatalf("first Touch() error = %v", err)
	}
	second, err := r.Touch(ws + string(os.PathSeparator))
	if err != nil {
		t.Fatalf("second Touch() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("ID changed on a trailing separator: %q -> %q", first.ID, second.ID)
	}
	if second.LastOpenedAt.Before(first.LastOpenedAt) {
		t.Fatalf("LastOpenedAt went backwards: %v -> %v", first.LastOpenedAt, second.LastOpenedAt)
	}
	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() = %d entries, want 1 after two Touch on the same path", len(entries))
	}
}

func TestAddRefusesInvalidPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "un-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	plain := filepath.Join(root, "non-workspace")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cases := []struct {
		name string
		path string
		code string
	}{
		{"empty", "", CodeRegistryPathRequired},
		{"relative", "docs", CodeRegistryPathNotAbsolute},
		{"missing", filepath.Join(root, "non-esiste"), CodeRegistryPathMissing},
		{"file", file, CodeRegistryPathNotADirectory},
		{"not a workspace", plain, CodeRegistryNotAWorkspace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Registry{Dir: t.TempDir()}
			_, err := r.Add(tc.path)
			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Add(%q) error = %v, want *ValidationError", tc.path, err)
			}
			if len(invalid.Fields) != 1 {
				t.Fatalf("fields = %+v, want exactly one", invalid.Fields)
			}
			if invalid.Fields[0].Field != "path" {
				t.Fatalf("field = %q, want %q", invalid.Fields[0].Field, "path")
			}
			if invalid.Fields[0].Code != tc.code {
				t.Fatalf("code = %q, want %q", invalid.Fields[0].Code, tc.code)
			}
		})
	}
}

func TestAddAcceptsAnExistingWorkspace(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	ws := makeWorkspace(t, t.TempDir(), "gamma")

	entry, err := r.Add(ws)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != entry.ID || entries[0].Path != filepath.Clean(ws) {
		t.Fatalf("List() = %+v, want the added workspace", entries)
	}
}

func TestProbeReportsUnreachableInsteadOfDropping(t *testing.T) {
	root := t.TempDir()
	r := &Registry{Dir: t.TempDir()}
	first := makeWorkspace(t, root, "uno")
	second := makeWorkspace(t, root, "due")
	if _, err := r.Touch(first); err != nil {
		t.Fatalf("Touch(first) error = %v", err)
	}
	if _, err := r.Touch(second); err != nil {
		t.Fatalf("Touch(second) error = %v", err)
	}

	if err := os.Rename(first, filepath.Join(root, "uno-rinominato")); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List() = %d entries, want 2: a renamed directory must not make an entry disappear", len(entries))
	}
	byPath := map[string]Entry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if got := Probe(byPath[filepath.Clean(first)]); got != StatusMissing {
		t.Fatalf("Probe(renamed) = %q, want %q", got, StatusMissing)
	}
	if got := Probe(byPath[filepath.Clean(second)]); got != StatusReachable {
		t.Fatalf("Probe(intact) = %q, want %q", got, StatusReachable)
	}

	file := filepath.Join(root, "un-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := Probe(Entry{Path: file}); got != StatusNotDirectory {
		t.Fatalf("Probe(file) = %q, want %q", got, StatusNotDirectory)
	}

	plain := filepath.Join(root, "senza-archetipo")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if got := Probe(Entry{Path: plain}); got != StatusNotWorkspace {
		t.Fatalf("Probe(plain dir) = %q, want %q", got, StatusNotWorkspace)
	}
}

func TestProbeReportsUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("0o000 does not refuse a read on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits")
	}
	dir := makeWorkspace(t, t.TempDir(), "chiuso")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if got := Probe(Entry{Path: dir}); got != StatusNotReadable {
		t.Fatalf("Probe(unreadable) = %q, want %q", got, StatusNotReadable)
	}
}

func TestRemoveLeavesTheWorkspaceUntouched(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	ws := makeWorkspace(t, t.TempDir(), "delta")
	entry, err := r.Touch(ws)
	if err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	before := inventory(t, ws)
	configBefore, err := os.ReadFile(filepath.Join(ws, config.RelativePath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if err := r.Remove(entry.ID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	entries, err := r.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, e := range entries {
		if e.ID == entry.ID {
			t.Fatal("the removed entry is still listed")
		}
	}
	assertSameInventory(t, before, inventory(t, ws))
	configAfter, err := os.ReadFile(filepath.Join(ws, config.RelativePath))
	if err != nil {
		t.Fatalf("the workspace config is no longer readable after Remove: %v", err)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatal("the workspace config changed after Remove")
	}
}

func TestRemoveUnknownID(t *testing.T) {
	r := &Registry{Dir: t.TempDir()}
	ws := makeWorkspace(t, t.TempDir(), "epsilon")
	if _, err := r.Touch(ws); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

	err := r.Remove("00000000")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("Remove(unknown) error = %v, want ErrEntryNotFound", err)
	}
	entries, listErr := r.List()
	if listErr != nil {
		t.Fatalf("List() error = %v", listErr)
	}
	if len(entries) != 1 {
		t.Fatalf("List() = %d entries, want 1 unchanged", len(entries))
	}
}
