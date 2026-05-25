package filefs_test

import (
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/conformance"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
)

func TestFilefsConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) connector.Connector {
		t.Helper()
		dir := t.TempDir()
		cfg := config.Default()
		cfg.ProjectRoot = dir
		// Use absolute paths so writes land inside the temp dir.
		cfg.File.Backlog = filepath.Join(dir, ".archetipo", "backlog.yaml")
		cfg.File.Planning = filepath.Join(dir, ".archetipo", "plans")
		cfg.Paths.PRD = filepath.Join(dir, "PRD.md")
		return filefs.New(cfg)
	})
}
