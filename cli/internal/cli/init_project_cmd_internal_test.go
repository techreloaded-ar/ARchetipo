package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Overwriting the config must leave the customizations recoverable, so the
// previous content has to land intact in the backup file.
func TestBackupExistingConfigPreservesContent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	original := "connector: jira\nwiki:\n  enabled: false\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	backupPath, err := backupExistingConfig(configPath)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	saved, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup not readable: %v", err)
	}
	if string(saved) != original {
		t.Fatalf("backup content differs:\n%s", saved)
	}
	if !strings.HasPrefix(filepath.Base(backupPath), "config.yaml.backup-") {
		t.Fatalf("unexpected backup name: %s", backupPath)
	}
}

// Two init runs in the same second must not overwrite each other's backup.
func TestUniqueBackupPathAvoidsCollisions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	first := uniqueBackupPath(configPath, now)
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := uniqueBackupPath(configPath, now)
	if second == first {
		t.Fatalf("collision not avoided: %s", second)
	}
	if !strings.HasSuffix(second, "-2") {
		t.Fatalf("unexpected collision suffix: %s", second)
	}
}

// The shipped template carries `wiki:` with a commented preamble, so the
// rewrite must land on the mapping's own `enabled:` key without disturbing the
// comments that explain it — and without matching an `enabled:` belonging to
// another section.
func TestSetWikiEnabledField(t *testing.T) {
	template := `connector: file

# Living Wiki, on by default.
wiki:
  enabled: true

worktree:
  enabled: false
  base: main
`
	out := setWikiEnabledField(template, false)
	if !strings.Contains(out, "wiki:\n  enabled: false\n") {
		t.Fatalf("wiki gate not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "worktree:\n  enabled: false\n  base: main") {
		t.Fatalf("worktree section must be untouched:\n%s", out)
	}
	if !strings.Contains(out, "# Living Wiki, on by default.") {
		t.Fatalf("comments must survive the rewrite:\n%s", out)
	}
}

func TestSetWikiEnabledFieldAppendsMissingSection(t *testing.T) {
	out := setWikiEnabledField("connector: file\n", false)
	if !strings.Contains(out, "wiki:\n  enabled: false\n") {
		t.Fatalf("missing section not appended:\n%s", out)
	}
	if !strings.HasPrefix(out, "connector: file\n") {
		t.Fatalf("existing content must be preserved:\n%s", out)
	}
}

// A `wiki:` mapping that exists but carries no `enabled:` must gain the key
// rather than be left at its default, which would silently keep the Wiki on.
func TestSetWikiEnabledFieldInsertsKeyIntoExistingSection(t *testing.T) {
	out := setWikiEnabledField("connector: file\nwiki:\n\nworktree:\n  enabled: false\n", false)
	if !strings.Contains(out, "wiki:\n  enabled: false\n") {
		t.Fatalf("key not inserted:\n%s", out)
	}
	if !strings.Contains(out, "worktree:\n  enabled: false\n") {
		t.Fatalf("worktree section lost:\n%s", out)
	}
}
