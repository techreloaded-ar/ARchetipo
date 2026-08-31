package localrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperEnv = "GO_WANT_HELPER_PROCESS"

func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process")
	}
	args := helperArgs()
	if len(args) == 0 {
		os.Exit(2)
	}
	code, err := strconv.Atoi(args[0])
	if err != nil {
		os.Exit(2)
	}
	if len(args) > 1 {
		millis, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Duration(millis) * time.Millisecond)
	}
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "cwd="+cwd)
	fmt.Fprintln(os.Stderr, "helper-stderr")
	os.Exit(code)
}

func helperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

func runHelper(t *testing.T, dir string, exitCode int) (string, string, int, error) {
	t.Helper()
	t.Setenv(helperEnv, "1")
	return ExecRunner{}.Run(context.Background(), dir, os.Args[0], []string{"-test.run=^TestHelperProcess$", "--", strconv.Itoa(exitCode)})
}

func resolved(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestExecRunner(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode, err := runHelper(t, dir, 0)
	if err != nil || exitCode != 0 {
		t.Fatalf("Run() = exit %d, err %v", exitCode, err)
	}
	reported := strings.TrimPrefix(strings.TrimSpace(stdout), "cwd=")
	if resolved(t, reported) != resolved(t, dir) || strings.Contains(stdout, "helper-stderr") || strings.TrimSpace(stderr) != "helper-stderr" {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, exitCode, err = runHelper(t, dir, 7)
	if err != nil || exitCode != 7 || !strings.Contains(stdout, "cwd=") || !strings.Contains(stderr, "helper-stderr") {
		t.Fatalf("failed command = stdout %q, stderr %q, exit %d, err %v", stdout, stderr, exitCode, err)
	}

	missing := filepath.Join(dir, "missing")
	_, _, exitCode, err = ExecRunner{}.Run(context.Background(), dir, missing, nil)
	if err == nil || exitCode != -1 || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing command = exit %d, err %v", exitCode, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, exitCode, err = ExecRunner{}.Run(ctx, dir, os.Args[0], nil)
	if err == nil || exitCode != -1 {
		t.Fatalf("cancelled command = exit %d, err %v", exitCode, err)
	}
}

func TestExecRunnerReportsAProcessKilledByContextAsAnExit(t *testing.T) {
	t.Setenv(helperEnv, "1")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, exitCode, err := ExecRunner{}.Run(ctx, t.TempDir(), os.Args[0], []string{"-test.run=^TestHelperProcess$", "--", "0", "60000"})
	if ctx.Err() == nil || err != nil || exitCode == 0 {
		t.Fatalf("killed command = context %v, exit %d, err %v", ctx.Err(), exitCode, err)
	}
}

func TestDiagnosticSuffix(t *testing.T) {
	if got := DiagnosticSuffix(" \n"); !strings.Contains(got, "wrote nothing") {
		t.Fatal(got)
	}
	body := strings.Repeat("è", MaxCapturedOutput)
	if got := Truncate(body); !strings.HasSuffix(got, "...") || strings.ToValidUTF8(got, "") != got {
		t.Fatal(got)
	}
}
