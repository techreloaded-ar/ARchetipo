package cli

import (
	"strings"
	"testing"
)

// The shipped template carries `wiki:` with a commented preamble, so the
// rewrite must land on the mapping's own `enabled:` key without disturbing the
// comments that explain it — and without matching an `enabled:` belonging to
// another section.
func TestSetWikiEnabledField(t *testing.T) {
	template := `connector: file

# Living Wiki, on by default.
wiki:
  enabled: true

worktree:
  enabled: false
  base: main
`
	out := setWikiEnabledField(template, false)
	if !strings.Contains(out, "wiki:\n  enabled: false\n") {
		t.Fatalf("wiki gate not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "worktree:\n  enabled: false\n  base: main") {
		t.Fatalf("worktree section must be untouched:\n%s", out)
	}
	if !strings.Contains(out, "# Living Wiki, on by default.") {
		t.Fatalf("comments must survive the rewrite:\n%s", out)
	}
}

func TestSetWikiEnabledFieldAppendsMissingSection(t *testing.T) {
	out := setWikiEnabledField("connector: file\n", false)
	if !strings.Contains(out, "wiki:\n  enabled: false\n") {
		t.Fatalf("missing section not appended:\n%s", out)
	}
	if !strings.HasPrefix(out, "connector: file\n") {
		t.Fatalf("existing content must be preserved:\n%s", out)
	}
}

// A `wiki:` mapping that exists but carries no `enabled:` must gain the key
// rather than be left at its default, which would silently keep the Wiki on.
func TestSetWikiEnabledFieldInsertsKeyIntoExistingSection(t *testing.T) {
	out := setWikiEnabledField("connector: file\nwiki:\n\nworktree:\n  enabled: false\n", false)
	if !strings.Contains(out, "wiki:\n  enabled: false\n") {
		t.Fatalf("key not inserted:\n%s", out)
	}
	if !strings.Contains(out, "worktree:\n  enabled: false\n") {
		t.Fatalf("worktree section lost:\n%s", out)
	}
}
