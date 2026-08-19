package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// execRunner is the real Runner: it spawns the command with os/exec and reports
// what came out of it.
type execRunner struct{}

var _ Runner = execRunner{}

// Run executes one command to completion in dir and returns its two streams
// separately, so a diagnostic can quote stderr without the noise of whatever
// the agent printed on stdout.
//
// It keeps apart the two failures that a single error value would blur: a
// process that never started (bad path, no permission, cancelled context) comes
// back as a non-nil error with exit code -1, while a process that ran and
// failed comes back with its own exit code and no error, because that is an
// outcome to classify rather than a breakdown of the invocation.
func (execRunner) Run(ctx context.Context, dir string, name string, args []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.String(), stderr.String(), exitErr.ExitCode(), nil
	}
	return stdout.String(), stderr.String(), -1, fmt.Errorf("running %s: %w", name, err)
}
