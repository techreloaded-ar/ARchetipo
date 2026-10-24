package workspace

import (
	"os"
	"path/filepath"
)

// Retired managed methods are removed on upgrade. Personal skills and project
// artifacts are untouched. The names are owned by this product's distribution.
var retiredSkills = []string{"archetipo-autopilot", "archetipo-design", "archetipo-implement", "archetipo-review"}

func IsRetiredSkill(name string) bool {
	for _, retired := range retiredSkills {
		if name == retired {
			return true
		}
	}
	return false
}
func RemoveRetiredSkills(root string) error {
	for _, name := range retiredSkills {
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	return nil
}
