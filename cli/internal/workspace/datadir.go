package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// DiscoverDataDir returns the directory containing skills/ and runtime/.
// Resolution order:
//  1. ARCHETIPO_DATA_DIR env var (set by the npm shim)
//  2. directory of the running binary, looking for skills/ alongside or in parent
//  3. the repo layout when running from source (skills/ + .archetipo/ at repo root)
func DiscoverDataDir() (string, error) {
	if env := strings.TrimSpace(os.Getenv("ARCHETIPO_DATA_DIR")); env != "" {
		if _, err := os.Stat(filepath.Join(env, "skills")); err == nil {
			return env, nil
		}
	}
	exe, err := os.Executable()
	if err == nil {
		resolved, _ := filepath.EvalSymlinks(exe)
		if resolved != "" {
			exe = resolved
		}
		for _, base := range []string{filepath.Dir(exe), filepath.Dir(filepath.Dir(exe))} {
			if _, err := os.Stat(filepath.Join(base, "skills")); err == nil {
				return base, nil
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if _, statErr := os.Stat(filepath.Join(cwd, "skills")); statErr == nil {
			return repoFallbackDataDir(cwd), nil
		}
	}
	return "", iox.NewPrecondition(
		"could not locate ARchetipo data directory",
		"set ARCHETIPO_DATA_DIR or reinstall via `npm i -g @techreloaded/archetipo`",
		nil,
	)
}

// repoFallbackDataDir maps the repo layout (skills/ + .archetipo/) onto the
// expected runtime layout by treating .archetipo/ as runtime/.
func repoFallbackDataDir(repoRoot string) string {
	// Callers use dataDir/skills and dataDir/runtime, while the repo has
	// skills/ and .archetipo/. We return the repo root and let the runtime
	// asset lookup fall back to .archetipo/.
	return repoRoot
}

// RuntimeAssetsDir returns the directory of the packaged runtime assets
// (config.yaml, shared-runtime.md) inside dataDir. It mirrors the two layouts
// DiscoverDataDir can return: runtime/ in the npm package, .archetipo/ in the
// source repository.
func RuntimeAssetsDir(dataDir string) (string, error) {
	for _, dir := range []string{filepath.Join(dataDir, "runtime"), filepath.Join(dataDir, ".archetipo")} {
		if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
			return dir, nil
		}
	}
	return "", iox.NewPrecondition(
		"runtime assets not found",
		"package may be incomplete; reinstall the CLI",
		nil,
	)
}
