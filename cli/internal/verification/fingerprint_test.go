package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/services"
)

func TestAgentEvidenceRemainsCurrentInViewerProcess(t *testing.T) {
	s, _, recipe := fixture(t)
	name := "runner"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	tool := filepath.Join(s.Root, "runtime", name)
	mustWrite(t, tool, "local test tool")
	if err := os.Chmod(tool, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range recipe.Prepare {
		recipe.Prepare[i].Argv = []string{"runtime/" + name}
	}
	for i := range recipe.Probe {
		recipe.Probe[i].Argv = []string{"runtime/" + name}
	}
	check := recipe.Checks["focused"]
	check.Argv = []string{"runtime/" + name}
	recipe.Checks["focused"] = check
	writeRecipe(t, s.Root, recipe)
	t.Chdir(s.Root)
	if report := execute(t, s, "check"); !report.Passed || !report.Stable {
		t.Fatalf("agent check failed: %+v", report)
	}
	// A viewer serves worktrees while its cwd is the parent workspace, and the
	// agent shell adds session-specific shim entries to the inherited PATH.
	t.Chdir(t.TempDir())
	t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+t.TempDir())
	viewer := New(s.Root, s.Results)
	if _, err := viewer.Evidence(context.Background()); err != nil {
		t.Fatalf("viewer rejected the agent's original evidence: %v", err)
	}
	observations, err := viewer.ObserveChecks(context.Background())
	if err != nil || len(observations) != 1 || !observations[0].Current {
		t.Fatalf("viewer mislabeled original check: %+v %v", observations, err)
	}
	if err := os.WriteFile(tool, []byte("updated local tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.Evidence(context.Background()); err == nil {
		t.Fatal("changed project tool did not invalidate retained evidence")
	}
}

func TestFingerprintIgnoresUnrelatedHarnessPathEntries(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	recipe := Recipe{Version: 1, Probe: []Command{{Name: "ready", Argv: []string{executable}, TimeoutSeconds: 10}}}
	before, err := fingerprint(root, recipe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeDeclared, err := fingerprint(root, Recipe{EnvKeys: []string{"PATH"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+t.TempDir())
	after, err := fingerprint(root, recipe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("an unrelated harness PATH entry invalidated the same resolved toolchain")
	}
	afterDeclared, err := fingerprint(root, Recipe{EnvKeys: []string{"PATH"}}, nil, nil)
	if err != nil || beforeDeclared == afterDeclared {
		t.Fatalf("explicit PATH input did not invalidate: %v", err)
	}
}

func TestFingerprintTracksResolvedExecutable(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	name := "fixture-tool"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	for _, dir := range []string{first, second} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	recipe := Recipe{Version: 1, Probe: []Command{{Name: "ready", Argv: []string{name}, TimeoutSeconds: 10}}}
	t.Setenv("PATH", first)
	before, err := fingerprint(root, recipe, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", second)
	after, err := fingerprint(root, recipe, nil, nil)
	if err != nil || before == after {
		t.Fatalf("a different resolved toolchain did not invalidate: %v", err)
	}
}

// A recipe with declared services names `archetipo` in the lifecycle commands it
// derives. Reinstalling ARchetipo rewrites that launcher: the retained reports
// of a candidate must survive it, while the declaration itself stays an input.
func TestFingerprintIgnoresTheArchetipoLauncherOfDeclaredServices(t *testing.T) {
	launcherDirectory := t.TempDir()
	launcherName := "archetipo"
	if os.PathSeparator == '\\' {
		launcherName += ".cmd"
	}
	launcher := filepath.Join(launcherDirectory, launcherName)
	if err := os.WriteFile(launcher, []byte("launcher of the installed version"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", launcherDirectory)
	declaredService := services.Spec{
		Image: "postgres:17-alpine", Port: 5432, Data: "/var/lib/postgresql/data",
		Env:    map[string]string{"POSTGRES_USER": "user", "POSTGRES_PASSWORD": "password", "POSTGRES_DB": "app"},
		Expose: map[string]string{"DATABASE_URL": "postgresql://user:password@{host}:{port}/app"},
	}
	recipeWithService := func(spec services.Spec) Recipe {
		recipe := Recipe{Version: 1, Services: map[string]services.Spec{"db": spec}}
		if err := recipe.deriveServices(); err != nil {
			t.Fatal(err)
		}
		return recipe
	}
	root := t.TempDir()
	beforeUpdate, err := fingerprint(root, recipeWithService(declaredService), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(launcher, []byte("launcher rewritten by an update of ARchetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	afterUpdate, err := fingerprint(root, recipeWithService(declaredService), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if beforeUpdate != afterUpdate {
		t.Fatal("updating ARchetipo invalidated evidence whose declared inputs never changed")
	}
	declaredService.Image = "postgres:16-alpine"
	otherDeclaration, err := fingerprint(root, recipeWithService(declaredService), nil, nil)
	if err != nil || otherDeclaration == afterUpdate {
		t.Fatalf("a changed service declaration did not invalidate: %v", err)
	}
}

func TestRecipeRejectsPatternsInsteadOfHashingMissingLiteralPaths(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"inputs":["*.py"],"probe":[{"name":"ready","argv":["tool"],"timeout_seconds":10}]}`,
		`{"version":1,"probe":[{"name":"ready","argv":["tool"],"timeout_seconds":10}],"checks":{"tests":{"name":"tests","argv":["tool"],"timeout_seconds":10,"inputs":["tests/**"]}}}`,
	} {
		if _, err := parseRecipe(t.TempDir(), []byte(raw)); err == nil {
			t.Fatal("unsupported input pattern accepted as a literal missing file")
		}
	}
}
