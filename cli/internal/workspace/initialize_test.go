package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// repoDataDir points the initialization at the source repository, which is the
// layout DiscoverDataDir supports through its .archetipo/ fallback. The real
// skills and the real packaged config are used: nothing about the assets is
// simulated.
func repoDataDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills")); err != nil {
		t.Skipf("skipped: the repository skills directory is not available (%v)", err)
	}
	t.Setenv("ARCHETIPO_DATA_DIR", root)
	return root
}

// isolateTempDir makes the staging directory land somewhere the test owns, so
// "no staging left behind" is an assertion instead of a hope.
func isolateTempDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	return tmp
}

func TestInitializeCreatesACompleteWorkspace(t *testing.T) {
	repoDataDir(t)
	dest := filepath.Join(t.TempDir(), "nuovo")

	paths := domain.ConfigPaths{
		PRD:         "docs/prodotto.md",
		Wiki:        "docs/kb/",
		Mockups:     "docs/mock/",
		TestResults: "docs/esiti/",
	}
	worktree := domain.WorktreeConfig{Enabled: true, Base: "develop", Dir: ".archetipo/wt", BranchPrefix: "us/"}

	result, err := Initialize(context.Background(), Options{
		Dir:       dest,
		Connector: "file",
		Tools:     []string{"pi", "claude"},
		Paths:     paths,
		Worktree:  worktree,
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if result.Dir != dest {
		t.Fatalf("result dir = %q, want %q", result.Dir, dest)
	}
	if !reflect.DeepEqual(result.Tools, []string{"pi", "claude"}) {
		t.Fatalf("result tools = %v", result.Tools)
	}
	tpl := template.Default()
	if result.Template.ID != tpl.ID || result.Template.Version != tpl.Version {
		t.Fatalf("result template = %+v, want %s %s", result.Template, tpl.ID, tpl.Version)
	}

	cfgPath := filepath.Join(dest, ".archetipo", "config.yaml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config.yaml was not created: %v", err)
	}
	cfg, err := config.ValidateRaw(dest, raw)
	if err != nil {
		t.Fatalf("the created config does not parse: %v", err)
	}
	if cfg.Paths != paths {
		t.Fatalf("persisted paths = %+v, want %+v", cfg.Paths, paths)
	}
	if cfg.Worktree != worktree {
		t.Fatalf("persisted worktree = %+v, want %+v", cfg.Worktree, worktree)
	}
	if cfg.Template.ID != tpl.ID || cfg.Template.Version != tpl.Version {
		t.Fatalf("persisted template = %+v, want the built-in Archetype", cfg.Template)
	}
	if _, err := os.Stat(filepath.Join(dest, ".archetipo", "shared-runtime.md")); err != nil {
		t.Fatalf("shared-runtime.md was not created: %v", err)
	}
	for _, tool := range []string{".pi/skills", ".claude/skills"} {
		for _, skill := range tpl.Skills {
			skillPath := filepath.Join(dest, filepath.FromSlash(tool), skill, "SKILL.md")
			if _, err := os.Stat(skillPath); err != nil {
				t.Fatalf("skill %s missing under %s: %v", skill, tool, err)
			}
		}
	}
}

func TestInitializeWritesNothingWhenTheContextIsAlreadyCancelled(t *testing.T) {
	repoDataDir(t)
	dest := filepath.Join(t.TempDir(), "nuovo")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Initialize(ctx, Options{Dir: dest, Connector: "file", Tools: []string{"pi"}}); err == nil {
		t.Fatal("Initialize succeeded on a cancelled context")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("the destination exists after a cancelled initialization: %v", err)
	}
}

func TestInitializeLeavesNoPartialWorkspaceWhenTheCommitFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped: the collision is built on POSIX rename semantics")
	}
	repoDataDir(t)
	tmpRoot := isolateTempDir(t)

	dest := mkdir(t, filepath.Join(t.TempDir(), "occupata"))
	withFile(t, dest, "README.md", "contenuto da non perdere")
	// `.pi` is a file, so the commit cannot create `.pi/skills/` under it. The
	// failure happens after `.archetipo/` has already been moved in, which is
	// exactly the half-done state the rollback must undo.
	withFile(t, dest, ".pi", "sono un file, non una directory")
	before := listRecursive(t, dest)

	_, err := Initialize(context.Background(), Options{Dir: dest, Connector: "file", Tools: []string{"pi"}})
	if err == nil {
		t.Fatal("Initialize succeeded despite the collision")
	}
	// The failure must be the collision on .pi and not something earlier:
	// otherwise the rollback below would be asserting on work never done.
	if !strings.Contains(err.Error(), ".pi") {
		t.Fatalf("expected the collision on .pi to be the failure, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, ".archetipo", "config.yaml")); statErr == nil {
		t.Fatal("config.yaml survived the rollback")
	}

	after := listRecursive(t, dest)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the failed initialization left something behind:\nbefore %v\nafter  %v", before, after)
	}
	kept, readErr := os.ReadFile(filepath.Join(dest, "README.md"))
	if readErr != nil || string(kept) != "contenuto da non perdere" {
		t.Fatalf("the rollback damaged a pre-existing file: %q, %v", string(kept), readErr)
	}
	assertNoStagingLeft(t, tmpRoot)
}

func TestInitializeRefusesAnAlreadyInitializedDestination(t *testing.T) {
	repoDataDir(t)
	dest := mkdir(t, filepath.Join(t.TempDir(), "gia-iniziato"))
	mkdir(t, filepath.Join(dest, ".archetipo"))
	withFile(t, filepath.Join(dest, ".archetipo"), "config.yaml", "connector: file\n")
	before := listRecursive(t, dest)

	if _, err := Initialize(context.Background(), Options{Dir: dest, Connector: "file", Tools: []string{"pi"}}); err == nil {
		t.Fatal("Initialize accepted an already initialized destination")
	}
	if after := listRecursive(t, dest); !reflect.DeepEqual(before, after) {
		t.Fatalf("the refusal wrote into the destination:\nbefore %v\nafter  %v", before, after)
	}
}

// assertNoStagingLeft proves the temporary build area is gone. It is a separate
// guarantee from the destination being clean: a leftover staging directory is
// a partial workspace too, just in a place nobody looks at.
func assertNoStagingLeft(t *testing.T, tmpRoot string) {
	t.Helper()
	entries, err := os.ReadDir(tmpRoot)
	if err != nil {
		t.Fatalf("reading the temp root: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "archetipo-init-") {
			t.Fatalf("staging directory left behind: %s", e.Name())
		}
	}
}
