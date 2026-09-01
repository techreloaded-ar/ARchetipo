package workspace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// seedTemporaryArtifacts creates a .archetipo/tmp/ tree containing one entry per
// name; names ending in "/" become directories with a file inside, so that a
// non-recursive removal would be caught by the assertions.
func seedTemporaryArtifacts(t *testing.T, names ...string) string {
	t.Helper()
	projectRoot := t.TempDir()
	temporaryRoot := filepath.Join(projectRoot, ".archetipo", "tmp")
	if err := os.MkdirAll(temporaryRoot, 0o755); err != nil {
		t.Fatalf("seeding temporary root: %v", err)
	}
	for _, name := range names {
		path := filepath.Join(temporaryRoot, strings.TrimSuffix(name, "/"))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("seeding %s: %v", name, err)
			}
			if err := os.WriteFile(filepath.Join(path, "part-01.md"), []byte("staged"), 0o644); err != nil {
				t.Fatalf("seeding %s content: %v", name, err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	return projectRoot
}

func remainingTemporaryArtifacts(t *testing.T, projectRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(projectRoot, ".archetipo", "tmp"))
	if err != nil {
		t.Fatalf("reading temporary root: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func TestRemoveSpecTemporaryArtifacts(t *testing.T) {
	cases := []struct {
		name     string
		seeded   []string
		specCode string
		want     []string
	}{
		{
			name: "removes the spec's staging directory and every payload it assembled",
			seeded: []string{
				"plan-US-001/",
				"payload-US-001-plan.json",
				"payload-US-001-feedback.json",
			},
			specCode: "US-001",
			want:     []string{},
		},
		{
			name: "leaves batch staging directories alone: they are not spec-scoped",
			seeded: []string{
				"plan-US-001/",
				"specs-US-001-US-015/",
				"payload-US-001-US-015.json",
			},
			specCode: "US-001",
			want:     []string{"payload-US-001-US-015.json", "specs-US-001-US-015"},
		},
		{
			name: "leaves another spec's artifacts untouched",
			seeded: []string{
				"plan-US-001/",
				"plan-US-002/",
				"payload-US-002-plan.json",
			},
			specCode: "US-001",
			want:     []string{"payload-US-002-plan.json", "plan-US-002"},
		},
		{
			name: "does not match a longer code that merely shares the prefix",
			seeded: []string{
				"plan-US-0011/",
				"payload-US-0011-plan.json",
			},
			specCode: "US-001",
			want:     []string{"payload-US-0011-plan.json", "plan-US-0011"},
		},
		{
			name:     "ignores non-payload files that happen to share the prefix",
			seeded:   []string{"payload-US-001-plan.json.bak"},
			specCode: "US-001",
			want:     []string{"payload-US-001-plan.json.bak"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := seedTemporaryArtifacts(t, tc.seeded...)
			if err := RemoveSpecTemporaryArtifacts(projectRoot, tc.specCode); err != nil {
				t.Fatalf("RemoveSpecTemporaryArtifacts: unexpected error %v", err)
			}
			got := remainingTemporaryArtifacts(t, projectRoot)
			if len(got) != len(tc.want) {
				t.Fatalf("remaining artifacts = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("remaining artifacts = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestRemoveSpecTemporaryArtifactsWithoutTemporaryRoot(t *testing.T) {
	// A project that never staged anything must not be an error: the sweep runs
	// after every transition to DONE, including on projects that use no skill.
	if err := RemoveSpecTemporaryArtifacts(t.TempDir(), "US-001"); err != nil {
		t.Fatalf("expected a missing tmp/ directory to be a no-op, got %v", err)
	}
}

func TestRemoveSpecTemporaryArtifactsRejectsCodesThatEscapeTheRoot(t *testing.T) {
	for _, specCode := range []string{"", "   ", "../../etc", "US-001/../..", `..\..\win`} {
		t.Run(specCode, func(t *testing.T) {
			projectRoot := seedTemporaryArtifacts(t, "plan-US-001/")
			if err := RemoveSpecTemporaryArtifacts(projectRoot, specCode); err == nil {
				t.Fatalf("expected spec code %q to be refused", specCode)
			}
			if got := remainingTemporaryArtifacts(t, projectRoot); len(got) != 1 {
				t.Fatalf("refused sweep must remove nothing, remaining = %v", got)
			}
		})
	}
}
