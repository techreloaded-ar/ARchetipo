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
