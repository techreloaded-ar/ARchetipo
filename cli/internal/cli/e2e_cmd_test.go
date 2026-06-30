package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func TestE2EDetect_EmptyProject(t *testing.T) {
	newProject(t)
	r := runCLI(t, "", "e2e", "detect")
	if r.exit != iox.ExitOK {
		t.Fatalf("exit = %d, stderr=%s", r.exit, r.stderr.String())
	}
	var env struct {
		Schema string `json:"schema"`
		Kind   string `json:"kind"`
		Data   struct {
			Framework string `json:"framework"`
			Installed bool   `json:"installed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.stdout.Bytes(), &env); err != nil {
		t.Fatalf("bad envelope: %v\n%s", err, r.stdout.String())
	}
	if env.Schema != iox.Schema || env.Kind != "e2e_detection" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if env.Data.Framework != "" || env.Data.Installed {
		t.Fatalf("empty project should report no framework, got %+v", env.Data)
	}
}

func TestE2EEnsure_NoPackageJSON_Precondition(t *testing.T) {
	newProject(t)
	r := runCLI(t, "", "e2e", "ensure")
	if r.exit != iox.ExitPreconditionMissing {
		t.Fatalf("exit = %d, want %d; stderr=%s", r.exit, iox.ExitPreconditionMissing, r.stderr.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(r.stderr.Bytes(), &env); err != nil {
		t.Fatalf("bad error envelope: %v\n%s", err, r.stderr.String())
	}
	if env.Error.Code != iox.CodePreconditionMissing {
		t.Fatalf("code = %q, want %q", env.Error.Code, iox.CodePreconditionMissing)
	}
}
