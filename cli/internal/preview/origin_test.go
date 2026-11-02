package preview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserOriginRejectsInvalidOrCopiedRuntimeMetadata(t *testing.T) {
	root := t.TempDir()
	for _, value := range []any{
		map[string]any{"version": 1, "root": root, "port": 0},
		map[string]any{"version": 1, "root": root, "port": 65536},
		map[string]any{"version": 2, "root": root, "port": 3000},
		map[string]any{"version": 1, "root": "another-checkout", "port": 3000},
		"invalid metadata",
	} {
		path := filepath.Join(root, ".archetipo/environment/browser-origin.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if listener, err := listenBrowserOrigin(root); err == nil {
			_ = listener.Close()
			t.Fatalf("accepted unsafe runtime origin: %s", raw)
		}
	}
}
