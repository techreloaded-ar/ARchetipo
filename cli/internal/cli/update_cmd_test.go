package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func TestRunUpdateCheckRequiresNPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := runUpdate(streams{}, true, false)
	var codedError *iox.CodedError
	if !errors.As(err, &codedError) {
		t.Fatalf("error = %T, want *iox.CodedError", err)
	}
	if codedError.Code != iox.CodePreconditionMissing || codedError.Exit != iox.ExitPreconditionMissing {
		t.Fatalf("error = %#v, want npm precondition", codedError)
	}
	if !strings.Contains(codedError.Message, "npm") {
		t.Fatalf("message = %q, want npm", codedError.Message)
	}
}

func TestRunUpdateDryRunPrintsNPMCommandWithoutRequiringNPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var stdout bytes.Buffer

	if err := runUpdate(streams{out: &stdout}, false, true); err != nil {
		t.Fatal(err)
	}
	want := "npm i -g " + npmPackageName + "@latest\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}
