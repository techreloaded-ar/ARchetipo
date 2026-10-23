package environmentrecipe

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s %v", args, raw, err)
	}
}
func write(t *testing.T, root, path, body string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
func TestPublicationFromIncrementReachesExistingAndFutureWorktrees(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.name", "Tests")
	git(t, root, "config", "user.email", "tests@example.test")
	write(t, root, "hello.txt", "initial")
	git(t, root, "add", "hello.txt")
	git(t, root, "commit", "-m", "initial")
	first, second, third := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second"), filepath.Join(t.TempDir(), "third")
	git(t, root, "worktree", "add", "-b", "first", first)
	git(t, root, "worktree", "add", "-b", "second", second)
	write(t, first, Environment, `{"version":1}`)
	write(t, first, Preview, `{"version":1,"url":"http://127.0.0.1:{port}/","argv":["server","{port}"],"prepare_environment":true}`)
	write(t, first, "scripts/archetipo/run.sh", "#!/bin/sh\nexit 0\n")
	write(t, first, ".archetipo/environment/state.json", "private checkpoint")
	if _, err := Publish(first, []string{"scripts/archetipo/run.sh"}); err != nil {
		t.Fatal(err)
	}
	git(t, root, "worktree", "add", "-b", "third", third)
	for _, target := range []string{root, second, third} {
		if err := Sync(target); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(target, "scripts/archetipo/run.sh"))
		if err != nil || string(raw) != "#!/bin/sh\nexit 0\n" {
			t.Fatalf("helper unavailable: %s %v", raw, err)
		}
		if _, err = os.Stat(filepath.Join(target, ".archetipo/environment/state.json")); !os.IsNotExist(err) {
			t.Fatal("copied another environment's checkpoint")
		}
	}
	// A common change updates previously inherited helpers, but preserves an
	// unrelated local edit and reports a conflict before changing any file.
	write(t, first, "scripts/archetipo/run.sh", "#!/bin/sh\nexit 1\n")
	if _, err := Publish(first, []string{"scripts/archetipo/run.sh"}); err != nil {
		t.Fatal(err)
	}
	if err := Sync(second); err != nil {
		t.Fatal(err)
	}
	write(t, third, "scripts/archetipo/run.sh", "local work")
	if err := Sync(third); err == nil {
		t.Fatal("overwrote local helper changes")
	}
	raw, _ := os.ReadFile(filepath.Join(third, "scripts/archetipo/run.sh"))
	if string(raw) != "local work" {
		t.Fatal("lost local work")
	}
}
func TestPublicationNeverCopiesUnlistedFilesAndRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	write(t, root, Environment, `{"version":1}`)
	write(t, root, ".env", "secret")
	r, err := Publish(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 1 {
		t.Fatal("included undeclared files")
	}
	for _, asset := range []string{".env", "../secret", "/tmp/secret", "scripts/archetipo/../../secret"} {
		if _, err = Publish(root, []string{asset}); err == nil {
			t.Fatalf("accepted %s", asset)
		}
	}
	if err = os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(root, "scripts/archetipo")); err != nil {
		t.Fatal(err)
	}
	if _, err = Publish(root, []string{"scripts/archetipo/secret"}); err == nil {
		t.Fatal("followed symlink")
	}
}

func TestNestedWorkspaceDoesNotPublishIntoEnclosingRepository(t *testing.T) {
	outer := t.TempDir()
	git(t, outer, "init", "-b", "main")
	nested := filepath.Join(outer, "nested")
	write(t, nested, Environment, `{"version":1}`)
	if _, err := Publish(nested, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outer, Path)); !os.IsNotExist(err) {
		t.Fatal("published into enclosing repository")
	}
	if _, err := os.Stat(filepath.Join(nested, Path)); err != nil {
		t.Fatal(err)
	}
}
func TestConfigureUpdatesTheSingleCommonReference(t *testing.T) {
	root := t.TempDir()
	write(t, root, Environment, `{"version":1}`)
	if _, err := Publish(root, nil); err != nil {
		t.Fatal(err)
	}
	updated := []byte(`{"version":1,"checks":{}}`)
	if err := Update(root, Environment, updated); err != nil {
		t.Fatal(err)
	}
	raw, err := Read(root, Environment)
	if err != nil || string(raw) != string(updated) {
		t.Fatalf("configure did not take effect: %s %v", raw, err)
	}
	if err = Update(root, Preview, []byte(`{"version":1,"url":"http://127.0.0.1:3000/"}`)); err == nil {
		t.Fatal("published a fixed endpoint")
	}
}
