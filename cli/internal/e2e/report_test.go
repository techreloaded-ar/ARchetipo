package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testExitError struct{}

func (testExitError) Error() string { return "test process exited 7" }
func (testExitError) ExitCode() int { return 7 }

func TestFunctionalEvidencePreservesFullOutputAndFailures(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "pass", true: "fail"}[failed], func(t *testing.T) {
			root := t.TempDir()
			writePackageJSON(t, root, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
			output := "FIRST LINE\n" + strings.Repeat("test output\n", 3000) + "LAST LINE\n"
			runner := &fakeRunner{output: output}
			if failed {
				runner.err = testExitError{}
			}
			result, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: root, Spec: "US-001", TestResultsDir: "custom/results", Grep: "bike", Runner: runner})
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed == failed || !strings.HasPrefix(result.ReportPath, filepath.Join(root, "custom", "results", "US-001")) {
				t.Fatalf("unexpected result: %+v", result)
			}
			saved, err := os.ReadFile(result.OutputPath)
			if err != nil || string(saved) != output {
				t.Fatalf("complete output was not retained: %v", err)
			}
			var report functionalReport
			raw, err := os.ReadFile(result.ReportPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			wantExit := 0
			if failed {
				wantExit = 7
			}
			if report.Passed == failed || report.ExitCode != wantExit || report.OutputPath != result.OutputPath || report.StartedAt.After(report.FinishedAt) {
				t.Fatalf("inaccurate evidence: %+v", report)
			}
			if strings.Join(report.Command, " ") != "npx playwright test --reporter=list --grep bike" {
				t.Fatalf("command omitted or changed: %v", report.Command)
			}
			second, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: root, Spec: "US-001", TestResultsDir: "custom/results", Runner: runner})
			if err != nil || second.ReportPath == result.ReportPath {
				t.Fatalf("retry overwrote evidence: %+v, %v", second, err)
			}
		})
	}
}

func TestFunctionalEvidenceUnknownExitIsNotReportedAsSuccess(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
	result, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: root, Spec: "US-001", Runner: &fakeRunner{err: context.DeadlineExceeded, output: "timed out"}})
	if err != nil {
		t.Fatal(err)
	}
	var report functionalReport
	raw, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.ExitCode != -1 || report.RunError == "" {
		t.Fatalf("timeout disguised as success: %+v", report)
	}
}

func TestFunctionalEvidenceRejectsUnsafeScopeBeforeRunning(t *testing.T) {
	for _, spec := range []string{"../outside", `US-001\child`, "/tmp/US-001", "..", "US-001/../US-002"} {
		runner := &fakeRunner{}
		_, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: t.TempDir(), Spec: spec, Runner: runner})
		if err == nil || !strings.Contains(err.Error(), "E_INVALID_INPUT") || len(runner.calls) != 0 {
			t.Fatalf("unsafe scope %q accepted: %v, %v", spec, err, runner.calls)
		}
	}
}

func TestFunctionalEvidencePersistenceFailureDoesNotReturnSuccess(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"devDependencies":{"@playwright/test":"1.0.0"}}`)
	if err := os.WriteFile(filepath.Join(root, "results"), []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := RunFunctional(context.Background(), RunOptions{ProjectRoot: root, Spec: "US-001", TestResultsDir: "results", Runner: &fakeRunner{}})
	if err == nil || !strings.Contains(err.Error(), "E_INTERNAL") {
		t.Fatalf("missing artifact ignored: %v", err)
	}
}
