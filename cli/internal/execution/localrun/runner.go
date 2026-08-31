package localrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// ExecRunner runs a command to completion and keeps stdout and stderr separate.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, name string, args []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), stderr.String(), exitErr.ExitCode(), nil
		}
		return stdout.String(), stderr.String(), -1, fmt.Errorf("running %s: %w", name, err)
	}
	return stdout.String(), stderr.String(), 0, nil
}

// DiagnosticSuffix appends a bounded stderr diagnostic to an error message.
func DiagnosticSuffix(stderr string) string {
	body := strings.TrimSpace(stderr)
	if body == "" {
		return " (it wrote nothing on standard error)"
	}
	return ": " + Truncate(body)
}

// Truncate bounds an echoed stream and cuts on a rune boundary.
func Truncate(body string) string {
	if len(body) <= MaxCapturedOutput {
		return body
	}
	cut := MaxCapturedOutput
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + "..."
}
