package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/cli"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
)

// A real OS sandbox catches writes hidden in configuration loading and locks;
// a fake native provider cannot detect these boundaries.
func TestIncrementCLIInsideNativeWriteBoundary(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox-exec regression; portable workflow tests run separately")
	}
	sandboxExec, err := exec.LookPath("sandbox-exec")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	mustRun(t, "git", "init", "-b", "main")
	mustRun(t, "git", "config", "user.email", "tests@example.test")
	mustRun(t, "git", "config", "user.name", "Tests")
	if err := os.WriteFile("hello.txt", []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "git", "add", "hello.txt")
	mustRun(t, "git", "commit", "-m", "initial")
	if err := os.MkdirAll(".archetipo", 0o755); err != nil {
		t.Fatal(err)
	}
	// Just like RideAtlas, the configuration is not part of the product commit.
	for name, body := range map[string]string{config.RelativePath: "connector: file\n", ".archetipo/shared-runtime.md": "test runtime\n"} {
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	child := func(workdir, profile, input string, args ...string) []byte {
		t.Helper()
		encoded, _ := json.Marshal(args)
		cmd := exec.Command(sandboxExec, "-p", profile, os.Args[0], "-test.run=^TestIncrementSandboxCLIHelper$")
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(), "ARCHETIPO_TEST_SANDBOX_CLI_ARGS="+string(encoded))
		cmd.Stdin = bytes.NewBufferString(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("sandboxed CLI %v: %v\n%s", args, err, out)
		}
		return out
	}
	for _, legacy := range []bool{false, true} {
		name := "current"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			made := runCLI(t, `{"title":"Bicycle trips","request":"Add bicycle trips"}`, "increment", "create")
			if made.exit != 0 {
				t.Fatal(made.stderr.String())
			}
			var envelope struct {
				Data increment.Record `json:"data"`
			}
			if err := json.Unmarshal(made.stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			r := envelope.Data
			t.Cleanup(func() { mustRun(t, "git", "worktree", "remove", "--force", r.Workdir) })
			if legacy {
				if err := os.Remove(filepath.Join(r.Workdir, ".archetipo/increment-local/increment.json")); err != nil {
					t.Fatal(err)
				}
			}
			indexPath := filepath.Join(root, ".archetipo/increments", r.ID+".json")
			before, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			// All writes under the parent are denied except inside this increment.
			quote := func(s string) string { raw, _ := json.Marshal(s); return string(raw) }
			profile := `(version 1)(allow default)(deny file-write* (require-all (subpath ` + quote(root) + `) (require-not (subpath ` + quote(r.Workdir) + `))))`
			// Prove that the profile really refuses the parent write that caused the bug.
			probe := exec.Command(sandboxExec, "-p", profile, "/usr/bin/touch", filepath.Join(root, "forbidden-probe"))
			if err := probe.Run(); err == nil {
				t.Fatal("sandbox allowed writing the parent")
			}
			setup := child(r.Workdir, profile, "", "config", "show")
			var decoded struct {
				Data struct {
					ProjectRoot string `json:"project_root"`
					Connector   string `json:"connector"`
				} `json:"data"`
			}
			if err := json.Unmarshal(setup, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Data.ProjectRoot != r.Workdir || decoded.Data.Connector != "file" {
				t.Fatalf("wrong sandbox setup: %s", setup)
			}
			child(r.Workdir, profile, "", "increment", "show", r.ID)
			proposal := `{"intent":"Bicycle trips with dedicated information","stories":[{"code":"US-001","title":"Bicycle trips","epic":{"code":"EP-001","title":"Cycling"},"priority":"MEDIUM","points":3,"status":"TODO","scope":"MVP","body":"## User story\nAs a rider I want bicycle trips.\n## Demonstrates\nA bicycle trip.\n## Acceptance\n- [ ] Bicycle trips are available."}]}`
			child(r.Workdir, profile, proposal, "increment", "propose", r.ID)
			child(r.Workdir, profile, `{"revision":1}`, "increment", "approve", r.ID)
			child(r.Workdir, profile, "", "spec", "show", "US-001")
			shown := runCLI(t, "", "increment", "show", r.ID)
			if shown.exit != 0 {
				t.Fatal(shown.stderr.String())
			}
			if err := json.Unmarshal(shown.stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Status != increment.Developing || len(envelope.Data.Revisions) != 1 {
				t.Fatalf("parent did not observe local progress: %s", shown.stdout.String())
			}
			after, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("agent changed the parent index")
			}
		})
	}
}

func TestIncrementSandboxCLIHelper(t *testing.T) {
	raw := os.Getenv("ARCHETIPO_TEST_SANDBOX_CLI_ARGS")
	if raw == "" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatal(err)
	}
	os.Exit(cli.Execute(args, os.Stdin, os.Stdout, os.Stderr))
}
