package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
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

// The init command must not carry a tool registry of its own: the viewer and
// the CLI have to offer exactly the same list, and the only way to guarantee
// that is to read it from the same place.
func TestInitToolRegistryComesFromWorkspacePackage(t *testing.T) {
	if !reflect.DeepEqual(allTools, workspace.Tools()) {
		t.Fatalf("allTools drifted from workspace.Tools():\n cli: %+v\n pkg: %+v", allTools, workspace.Tools())
	}
	if validToolKeysHint() != workspace.ToolKeysHint() {
		t.Fatalf("hint drifted: %q vs %q", validToolKeysHint(), workspace.ToolKeysHint())
	}
}
