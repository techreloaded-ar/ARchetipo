package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// temporaryArtifactsDirName is the single root under .archetipo/ where every
// skill stages the files it assembles payloads from. Keeping them all in one
// place is what makes this sweep possible: nothing temporary is written
// outside it, so a leftover is always both visible and reachable from here.
const temporaryArtifactsDirName = "tmp"

// RemoveSpecTemporaryArtifacts deletes the temporary artifacts a single spec
// accumulates under .archetipo/tmp/: the `plan-<CODE>/` staging directory
// written by the planning skill, and the `payload-<CODE>-*.json` files
// assembled by planning and by review's request-changes.
//
// Batch staging directories (`specs-<range>/`) are deliberately left alone:
// they belong to a backlog bootstrap or extension, which happens before any
// spec is implemented, so they are not part of one spec's lifecycle.
//
// Callers treat a failure as non-fatal. By the time this runs the spec has
// already been integrated or transitioned, and a file that could not be
// removed must never turn a completed transition into a failed command.
func RemoveSpecTemporaryArtifacts(projectRoot, specCode string) error {
	code := strings.TrimSpace(specCode)
	// The code arrives from the command line and is about to become part of a
	// path, so refuse anything that could escape the temporary root instead of
	// building a path out of it.
	if code == "" || strings.ContainsAny(code, `/\`) || strings.Contains(code, "..") {
		return fmt.Errorf("refusing to sweep temporary artifacts for invalid spec code %q", specCode)
	}

	temporaryRoot := filepath.Join(projectRoot, ".archetipo", temporaryArtifactsDirName)
	entries, err := os.ReadDir(temporaryRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	stagingDirName := "plan-" + code
	// Matched by exact name, never by prefix: a batch payload for a range that
	// starts at this code (`payload-US-001-US-015.json`) shares the prefix
	// `payload-US-001-` but belongs to a backlog bootstrap, not to this spec.
	// A skill that introduces another spec-scoped payload must be added here.
	specPayloadNames := map[string]bool{
		"payload-" + code + "-plan.json":     true,
		"payload-" + code + "-feedback.json": true,
	}

	var notRemoved []string
	for _, entry := range entries {
		name := entry.Name()
		isStagingDir := entry.IsDir() && name == stagingDirName
		isPayload := !entry.IsDir() && specPayloadNames[name]
		if !isStagingDir && !isPayload {
			continue
		}
		if err := os.RemoveAll(filepath.Join(temporaryRoot, name)); err != nil {
			notRemoved = append(notRemoved, name)
		}
	}

	if len(notRemoved) > 0 {
		return fmt.Errorf("could not remove temporary artifacts under %s: %s",
			temporaryRoot, strings.Join(notRemoved, ", "))
	}
	return nil
}
