package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

func TestCheckRuntimeAssetsWithoutLiveWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dir        string
		configName string
	}{
		{name: "source repository", dir: ".archetipo", configName: workspace.ConfigTemplateName},
		{name: "npm package", dir: "runtime", configName: "config.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runtimeDir := filepath.Join(root, tc.dir)
			if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{tc.configName, "shared-runtime.md"} {
				if err := os.WriteFile(filepath.Join(runtimeDir, name), []byte("product asset\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			check := checkRuntimeAssets(root)
			if !check.ok || check.detail != runtimeDir {
				t.Fatalf("product assets should pass without a live workspace: %+v", check)
			}
			if err := os.Remove(filepath.Join(runtimeDir, tc.configName)); err != nil {
				t.Fatal(err)
			}
			if check := checkRuntimeAssets(root); check.ok {
				t.Fatal("missing product template should fail the diagnostic")
			}
		})
	}
}
