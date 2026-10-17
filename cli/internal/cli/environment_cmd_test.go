package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func TestEnvironmentRecipeCLIValidationAndAtomicConfigure(t *testing.T) {
	newProject(t)
	missing := runCLI(t, "", "execution", "environment", "show")
	if missing.exit != iox.ExitPreconditionMissing {
		t.Fatalf("missing recipe: %s", missing.stderr.String())
	}
	payload := `{"version":1,"inputs":[],"outputs":[],"prepare":[],"probe":[{"name":"node-ready","argv":["node","--version"],"timeout_seconds":5}],"checks":{}}`
	created := runCLI(t, payload, "execution", "environment", "configure")
	if created.exit != iox.ExitOK {
		t.Fatal(created.stderr.String())
	}
	before := runCLI(t, "", "execution", "environment", "show")
	bad := runCLI(t, `{"version":1,"unknown":"typo"}`, "execution", "environment", "configure")
	if bad.exit == iox.ExitOK {
		t.Fatal("invalid recipe accepted")
	}
	after := runCLI(t, "", "execution", "environment", "show")
	if before.stdout.String() != after.stdout.String() {
		t.Fatal("invalid configure changed the recipe")
	}
	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(created.stdout.Bytes(), &envelope); err != nil || envelope.Kind != "verification_result" {
		t.Fatal("unexpected envelope", err)
	}
	// Unknown check names and unsafe scopes fail before launching project code.
	if result := runCLI(t, "", "execution", "environment", "check", "missing", "--spec", "../outside"); result.exit == iox.ExitOK {
		t.Fatal("unsafe scope accepted")
	}
}

func TestEnvironmentConfigureRejectsInheritedDependencySymlink(t *testing.T) {
	newProject(t)
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "node_modules")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	payload := `{"version":1,"inputs":[],"outputs":["node_modules/client.js"],"prepare":[],"probe":[{"name":"ready","argv":["node","--version"],"timeout_seconds":1}],"checks":{}}`
	if result := runCLI(t, payload, "execution", "environment", "configure"); result.exit != iox.ExitInvalidInput {
		t.Fatalf("symlink accepted: %s", result.stderr.String())
	}
}
