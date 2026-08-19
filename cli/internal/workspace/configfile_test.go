package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// The shipped template carries `wiki:` with a commented preamble, so the
// rewrite must land on the mapping's own `enabled:` key without disturbing the
// comments that explain it — and without matching an `enabled:` belonging to
// another section.
func TestSetWikiEnabledField(t *testing.T) {
	template := `connector: file

# Living Wiki, off by default.
wiki:
  enabled: false

worktree:
  enabled: false
  base: main
`
	out := setWikiEnabledField(template, true)
	if !strings.Contains(out, "wiki:\n  enabled: true\n") {
		t.Fatalf("wiki gate not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "worktree:\n  enabled: false\n  base: main") {
		t.Fatalf("worktree section must be untouched:\n%s", out)
	}
	if !strings.Contains(out, "# Living Wiki, off by default.") {
		t.Fatalf("comments must survive the rewrite:\n%s", out)
	}
}

func TestSetWikiEnabledFieldAppendsMissingSection(t *testing.T) {
	out := setWikiEnabledField("connector: file\n", true)
	if !strings.Contains(out, "wiki:\n  enabled: true\n") {
		t.Fatalf("missing section not appended:\n%s", out)
	}
	if !strings.HasPrefix(out, "connector: file\n") {
		t.Fatalf("existing content must be preserved:\n%s", out)
	}
}

// A `wiki:` mapping that exists but carries no `enabled:` must gain the key
// rather than be left at its default, which would silently keep the Wiki off.
func TestSetWikiEnabledFieldInsertsKeyIntoExistingSection(t *testing.T) {
	out := setWikiEnabledField("connector: file\nwiki:\n\nworktree:\n  enabled: false\n", true)
	if !strings.Contains(out, "wiki:\n  enabled: true\n") {
		t.Fatalf("key not inserted:\n%s", out)
	}
	if !strings.Contains(out, "worktree:\n  enabled: false\n") {
		t.Fatalf("worktree section lost:\n%s", out)
	}
}

// setTemplateFields rewrites YAML textually, so both of its branches — patching
// an existing block and appending a missing one — are exercised here. The
// append branch is what an older packaged runtime asset hits, and it has no
// coverage from the CLI-level tests, which always run against the current asset.
func TestSetTemplateFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "patches an existing block and keeps its comment",
			body: "connector: file\n\n# the process\ntemplate:\n  id: old\n  version: \"0.0.1\"\n\nfile:\n  backlog: b\n",
			want: "connector: file\n\n# the process\ntemplate:\n  id: fabbrica\n  version: \"2.0.0\"\n\nfile:\n  backlog: b\n",
		},
		{
			name: "appends the whole block when the key is absent",
			body: "connector: file\n",
			want: "connector: file\n\ntemplate:\n  id: fabbrica\n  version: \"2.0.0\"\n",
		},
		{
			name: "completes a block that declares only the id",
			body: "template:\n  id: old\n\nfile:\n  backlog: b\n",
			want: "template:\n  id: fabbrica\n  version: \"2.0.0\"\n\nfile:\n  backlog: b\n",
		},
		{
			name: "leaves unrelated children of the block untouched",
			body: "template:\n  id: old\n  note: keep me\n  version: \"0.0.1\"\n",
			want: "template:\n  id: fabbrica\n  note: keep me\n  version: \"2.0.0\"\n",
		},
		{
			name: "does not confuse a nested template key with the top-level one",
			body: "other:\n  template:\n    id: nested\n",
			want: "other:\n  template:\n    id: nested\n\ntemplate:\n  id: fabbrica\n  version: \"2.0.0\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := setTemplateFields(tc.body, "fabbrica", "2.0.0"); got != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// The packaged asset must already carry the block, so a normal init patches it
// instead of appending: appending would move the template away from the comment
// that explains it.
func TestSetTemplateFieldsIsIdempotent(t *testing.T) {
	body := "connector: file\ntemplate:\n  id: a\n  version: \"1\"\n"
	once := setTemplateFields(body, "fabbrica", "2.0.0")
	if twice := setTemplateFields(once, "fabbrica", "2.0.0"); twice != once {
		t.Fatalf("not idempotent:\nfirst:\n%q\nsecond:\n%q", once, twice)
	}
}

// The whole point of rendering onto the packaged template instead of
// re-marshalling a struct is that the file keeps explaining itself.
func TestRenderConfigWritesChosenValuesAndKeepsComments(t *testing.T) {
	body := packagedConfigForTest(t)
	in := RenderInput{
		Connector: "github",
		Paths: domain.ConfigPaths{
			PRD:         "docs/prodotto.md",
			Wiki:        "docs/kb/",
			Mockups:     "docs/mock/",
			TestResults: "docs/esiti/",
		},
		Worktree: domain.WorktreeConfig{
			Enabled:      true,
			Base:         "develop",
			Dir:          ".archetipo/wt",
			BranchPrefix: "us/",
		},
		Wiki:     false,
		Template: template.Default(),
	}
	out := RenderConfig(body, in)

	cfg, err := config.ValidateRaw(t.TempDir(), []byte(out))
	if err != nil {
		t.Fatalf("rendered config does not parse: %v\n%s", err, out)
	}
	if cfg.Connector != "github" {
		t.Fatalf("connector = %q, want github", cfg.Connector)
	}
	if cfg.Paths != in.Paths {
		t.Fatalf("paths = %+v, want %+v", cfg.Paths, in.Paths)
	}
	if cfg.Worktree != in.Worktree {
		t.Fatalf("worktree = %+v, want %+v", cfg.Worktree, in.Worktree)
	}
	if cfg.Wiki.Enabled {
		t.Fatal("wiki.enabled = true, want false")
	}
	if cfg.Template.ID != template.DefaultID || cfg.Template.Version != template.Default().Version {
		t.Fatalf("template = %+v, want the built-in one", cfg.Template)
	}
	if !strings.Contains(out, "# Shared paths — used by every connector.") {
		t.Fatal("the rendered config lost the comments of the packaged template")
	}
}

func TestRenderConfigIsIdempotent(t *testing.T) {
	body := packagedConfigForTest(t)
	in := RenderInput{
		Connector: "file",
		Paths:     config.Default().Paths,
		Worktree:  config.Default().Worktree,
		Wiki:      true,
		Template:  template.Default(),
	}
	once := RenderConfig(body, in)
	twice := RenderConfig(once, in)
	if once != twice {
		t.Fatalf("RenderConfig is not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestSetMappingFieldsAppendsAnAbsentSection(t *testing.T) {
	out := setMappingFields("connector: file\n", "worktree", []mappingField{
		{Key: "enabled", Value: "true", Always: true},
		{Key: "base", Value: "main"},
	})
	if !strings.Contains(out, "worktree:\n  enabled: true\n  base: main\n") {
		t.Fatalf("appended section is missing or malformed:\n%s", out)
	}
}

func TestSetMappingFieldsInsertsAMissingKey(t *testing.T) {
	out := setMappingFields("paths:\n  prd: docs/PRD.md\n\nconnector: file\n", "paths", []mappingField{
		{Key: "prd", Value: "docs/altro.md"},
		{Key: "mockups", Value: "docs/mock/"},
	})
	if !strings.Contains(out, "  prd: docs/altro.md") {
		t.Fatalf("existing key was not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "  mockups: docs/mock/") {
		t.Fatalf("missing key was not inserted:\n%s", out)
	}
	if !strings.Contains(out, "connector: file") {
		t.Fatalf("the insertion swallowed the rest of the document:\n%s", out)
	}
}

// packagedConfigForTest reads the config.yaml the CLI actually ships, so the
// rendering is exercised against the real asset and not a hand-made stub.
func packagedConfigForTest(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatalf("reading the packaged config template: %v", err)
	}
	return string(body)
}
