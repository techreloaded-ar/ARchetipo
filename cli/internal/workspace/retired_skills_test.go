package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeRemovesOnlyRetiredManagedMethods(t *testing.T) {
	root := t.TempDir()
	for _, name := range append(append([]string{}, retiredSkills...), "personal-check", "archetipo-wiki") {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("keep personal content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveRetiredSkills(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range retiredSkills {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("retired method survived", name, err)
		}
	}
	for _, name := range []string{"personal-check", "archetipo-wiki"} {
		body, err := os.ReadFile(filepath.Join(root, name, "SKILL.md"))
		if err != nil || string(body) != "keep personal content" {
			t.Fatal("upgrade touched unrelated content", name, err)
		}
	}
}
