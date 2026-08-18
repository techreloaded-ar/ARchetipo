package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The real Runner is the one piece of this package that cannot be proved with a
// fake, because it is the wiring to os/exec itself. It is exercised with the
// standard Go sub-process pattern: the test binary re-executes itself in helper
// mode, so the fixture is cross-platform and needs neither a shell script nor a
// Codex installation.

const helperEnv = "GO_WANT_HELPER_PROCESS"

// TestHelperProcess is not a test: it is the program the Runner tests spawn. It
// exits immediately, before the testing framework can print anything, so the
// captured streams contain only what it wrote.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process: only meaningful when re-executed by a Runner test")
	}
	args := helperArgs()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "helper: missing exit code argument")
		os.Exit(2)
	}
	code, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper: bad exit code argument")
		os.Exit(2)
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper: "+err.Error())
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "cwd="+cwd)
	fmt.Fprintln(os.Stderr, "helper-stderr")
	os.Exit(code)
}

// helperArgs returns the arguments the parent test appended after the "--"
// separator, which is where the flag package stops parsing.
func helperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

// runHelper runs the test binary in helper mode through the real Runner.
func runHelper(t *testing.T, dir string, exitCode int) (string, string, int, error) {
	t.Helper()
	t.Setenv(helperEnv, "1")
	args := []string{"-test.run=^TestHelperProcess$", "--", strconv.Itoa(exitCode)}
	return execRunner{}.Run(context.Background(), dir, os.Args[0], args)
}

// resolved follows the symlinks of a temporary directory, because macOS reports
// /var as /private/var to the child process.
func resolved(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestRunnerCapturesTheStreamsSeparatelyAndHonoursTheDirectory(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode, err := runHelper(t, dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}
	reported := strings.TrimPrefix(strings.TrimSpace(stdout), "cwd=")
	if resolved(t, reported) != resolved(t, dir) {
		t.Fatalf("the command ran in %q, want the requested directory %q", reported, dir)
	}
	if strings.Contains(stdout, "helper-stderr") {
		t.Fatalf("stderr leaked into stdout: %q", stdout)
	}
	if strings.TrimSpace(stderr) != "helper-stderr" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunnerReportsANonZeroExitCodeAsAnOutcomeNotAnError(t *testing.T) {
	stdout, stderr, exitCode, err := runHelper(t, t.TempDir(), 7)
	if err != nil {
		t.Fatalf("a command that ran and failed reported an invocation error: %v", err)
	}
	if exitCode != 7 {
		t.Fatalf("exit code = %d, want 7", exitCode)
	}
	if !strings.Contains(stdout, "cwd=") || !strings.Contains(stderr, "helper-stderr") {
		t.Fatalf("streams of a failing command were dropped: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestRunnerReportsACommandThatCannotStart(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-an-executable")
	_, _, exitCode, err := execRunner{}.Run(context.Background(), t.TempDir(), missing, nil)
	if err == nil {
		t.Fatal("expected an error for a command that never started")
	}
	if exitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for a process that never ran", exitCode)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error does not name the command: %v", err)
	}
}

func TestRunnerReportsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	t.Setenv(helperEnv, "1")
	_, _, exitCode, err := execRunner{}.Run(ctx, t.TempDir(), os.Args[0], []string{"-test.run=^TestHelperProcess$", "--", "0"})
	if err == nil {
		t.Fatal("expected an error for a context cancelled before the start")
	}
	if exitCode != -1 {
		t.Fatalf("exit code = %d, want -1", exitCode)
	}
}
