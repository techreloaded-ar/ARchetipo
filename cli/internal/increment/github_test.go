package increment

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A retried publication must not post the details comment twice: the fake gh
// keeps the posted bodies in a file and serves them back on pr view.
func TestGitHubCommentIsPostedOncePerMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gh is a POSIX shell script")
	}
	dir := t.TempDir()
	posted := filepath.Join(dir, "posted")
	script := `#!/bin/sh
if [ "$2" = "view" ]; then
  if [ -s "` + posted + `" ]; then printf '{"comments":[{"body":"<!-- marker -->\\nold"}]}'; else printf '{"comments":[]}'; fi
elif [ "$2" = "comment" ]; then
  cat "$5" >> "` + posted + `"
fi
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for range 2 {
		if err := (GitHub{}).Comment(context.Background(), dir, 7, "<!-- marker -->\nIntento e verifiche"); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(posted)
	if err != nil || strings.Count(string(raw), "Intento e verifiche") != 1 {
		t.Fatalf("comment not posted exactly once: %q %v", raw, err)
	}
}
