package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

func validOptions(dir string) Options {
	return Options{
		Dir:       dir,
		Connector: "file",
		Tools:     []string{"pi"},
	}
}

func TestAvailableReportsTheAcceptedChoices(t *testing.T) {
	choices := Available()
	if len(choices.Connectors) != len(Connectors()) {
		t.Fatalf("connectors = %d, want %d", len(choices.Connectors), len(Connectors()))
	}
	if len(choices.Tools) != len(Tools()) {
		t.Fatalf("tools = %d, want %d", len(choices.Tools), len(Tools()))
	}
	if choices.Paths != config.Default().Paths {
		t.Fatalf("default paths = %+v, want %+v", choices.Paths, config.Default().Paths)
	}
	if choices.Worktree != config.Default().Worktree {
		t.Fatalf("default worktree = %+v, want %+v", choices.Worktree, config.Default().Worktree)
	}
	if choices.Template.ID != template.DefaultID || choices.Template.Version == "" {
		t.Fatalf("template identity = %+v, want the built-in Archetype", choices.Template)
	}
	// Everything offered must be accepted: that is the whole contract.
	for _, tool := range choices.Tools {
		if _, err := ResolveTools([]string{tool.ID}); err != nil {
			t.Fatalf("offered tool %q is not accepted: %v", tool.ID, err)
		}
	}
	for _, conn := range choices.Connectors {
		if !IsConnector(conn.ID) {
			t.Fatalf("offered connector %q is not accepted", conn.ID)
		}
	}
}

func TestPreflightAcceptsAValidRequest(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{
		"destinazione inesistente":  filepath.Join(root, "nuovo"),
		"destinazione vuota":        mkdir(t, filepath.Join(root, "vuota")),
		"destinazione non iniziata": withFile(t, mkdir(t, filepath.Join(root, "esistente")), "README.md", "ciao"),
	}
	for name, dir := range cases {
		t.Run(name, func(t *testing.T) {
			resolved, err := Preflight(validOptions(dir))
			if err != nil {
				t.Fatalf("Preflight refused a valid request: %v", err)
			}
			if resolved.Dir != filepath.Clean(dir) {
				t.Fatalf("resolved dir = %q, want %q", resolved.Dir, dir)
			}
			if resolved.Paths != config.Default().Paths {
				t.Fatalf("paths = %+v, want the defaults", resolved.Paths)
			}
			if resolved.Worktree != config.Default().Worktree {
				t.Fatalf("worktree = %+v, want the defaults", resolved.Worktree)
			}
			if resolved.Template.ID != template.DefaultID {
				t.Fatalf("template = %q, want the built-in one", resolved.Template.ID)
			}
		})
	}
}

func TestPreflightRefusalsNameTheField(t *testing.T) {
	root := t.TempDir()
	initialized := mkdir(t, filepath.Join(root, "gia-iniziato"))
	if err := os.MkdirAll(filepath.Join(initialized, ".archetipo"), 0o755); err != nil {
		t.Fatal(err)
	}
	withFile(t, filepath.Join(initialized, ".archetipo"), "config.yaml", "connector: file\n")
	aFile := withFile(t, root, "un-file", "non sono una directory")

	cases := []struct {
		name  string
		opts  Options
		field string
		code  string
	}{
		{"destinazione assente", Options{Connector: "file", Tools: []string{"pi"}}, "dir", CodeDirRequired},
		{"destinazione relativa", validOptions("relativa/qui"), "dir", CodeDirNotAbsolute},
		{"destinazione che è un file", validOptions(filepath.Join(aFile, "un-file")), "dir", CodeDirNotADirectory},
		{"destinazione già inizializzata", validOptions(initialized), "dir", CodeAlreadyInitialized},
		{"connector sconosciuto", Options{Dir: filepath.Join(root, "x"), Connector: "nope", Tools: []string{"pi"}}, "connector", CodeConnectorUnknown},
		{"nessun tool", Options{Dir: filepath.Join(root, "x"), Connector: "file"}, "tools", CodeToolsRequired},
		{"tool sconosciuto", Options{Dir: filepath.Join(root, "x"), Connector: "file", Tools: []string{"nope"}}, "tools", CodeToolUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefusal(t, tc.opts, tc.field, tc.code)
		})
	}
}

func TestPreflightRefusesADirectoryItCannotWriteTo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipped: running as root, where a 0o500 directory is still writable")
	}
	dir := mkdir(t, filepath.Join(t.TempDir(), "bloccata"))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	assertRefusal(t, validOptions(dir), "dir", CodeDirNotWritable)
}

func TestPreflightRefusesPathsThatEscapeTheWorkspace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nuovo")
	opts := validOptions(dir)
	opts.Paths = domain.ConfigPaths{
		PRD:         "/etc/PRD.md",
		Wiki:        "../fuori/",
		Mockups:     "docs/mock/",
		TestResults: "",
	}
	err := refusalOf(t, opts)
	byField := map[string]string{}
	for _, f := range err.Fields {
		byField[f.Field] = f.Code
	}
	want := map[string]string{
		"paths.prd":          CodePathNotRelative,
		"paths.wiki":         CodePathNotRelative,
		"paths.test_results": CodePathRequired,
	}
	for field, code := range want {
		if byField[field] != code {
			t.Fatalf("field %s = %q, want %q (all: %+v)", field, byField[field], code, err.Fields)
		}
	}
	if _, unexpected := byField["paths.mockups"]; unexpected {
		t.Fatalf("a valid path was refused: %+v", err.Fields)
	}
}

func TestPreflightWorktreeRulesDependOnTheGate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nuovo")

	off := validOptions(dir)
	off.Worktree = domain.WorktreeConfig{Enabled: false}
	resolved, err := Preflight(off)
	if err != nil {
		t.Fatalf("an empty worktree block with the workflow off must be accepted: %v", err)
	}
	if resolved.Worktree != config.Default().Worktree {
		t.Fatalf("worktree = %+v, want the defaults", resolved.Worktree)
	}

	on := validOptions(dir)
	on.Worktree = domain.WorktreeConfig{Enabled: true, Dir: "/assoluta"}
	failure := refusalOf(t, on)
	got := map[string]string{}
	for _, f := range failure.Fields {
		got[f.Field] = f.Code
	}
	want := map[string]string{
		"worktree.base":          CodeWorktreeFieldNeeded,
		"worktree.branch_prefix": CodeWorktreeFieldNeeded,
		"worktree.dir":           CodeWorktreeDirInvalid,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("worktree refusals = %+v, want %+v", got, want)
	}
}

func TestPreflightReportsEveryInvalidFieldAtOnce(t *testing.T) {
	err := refusalOf(t, Options{Dir: "relativa", Connector: "nope"})
	if len(err.Fields) < 3 {
		t.Fatalf("expected the three invalid inputs at once, got %+v", err.Fields)
	}
}

func TestPreflightWritesNothingIntoTheDestination(t *testing.T) {
	dir := mkdir(t, filepath.Join(t.TempDir(), "intatta"))
	withFile(t, dir, "README.md", "contenuto")
	before := listRecursive(t, dir)

	opts := validOptions(dir)
	opts.Connector = "nope"
	if _, err := Preflight(opts); err == nil {
		t.Fatal("Preflight accepted an unknown connector")
	}
	// A valid request is also a read-only operation: only Initialize writes.
	if _, err := Preflight(validOptions(dir)); err != nil {
		t.Fatalf("Preflight refused a valid request: %v", err)
	}
	if after := listRecursive(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("Preflight touched the destination:\nbefore %v\nafter  %v", before, after)
	}
}

func assertRefusal(t *testing.T, opts Options, field, code string) {
	t.Helper()
	err := refusalOf(t, opts)
	for _, f := range err.Fields {
		if f.Field == field && f.Code == code {
			if strings.TrimSpace(f.Message) == "" {
				t.Fatalf("refusal %s/%s carries no message", field, code)
			}
			return
		}
	}
	t.Fatalf("no refusal on %s with code %s; got %+v", field, code, err.Fields)
}

func refusalOf(t *testing.T, opts Options) *ValidationError {
	t.Helper()
	_, err := Preflight(opts)
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want *ValidationError", err)
	}
	return invalid
}

func mkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func withFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// listRecursive returns the sorted relative paths under dir, which is how the
// tests state "nothing was written" without trusting an implementation detail.
func listRecursive(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if rel != "." {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	sort.Strings(out)
	return out
}
