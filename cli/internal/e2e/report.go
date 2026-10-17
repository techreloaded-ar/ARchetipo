package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

var reportSpecPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)

type reportSource struct {
	Head       string `json:"git_head,omitempty"`
	DiffSHA256 string `json:"tracked_diff_sha256,omitempty"`
}

type functionalReport struct {
	Schema      string       `json:"schema"`
	Spec        string       `json:"spec"`
	ProjectRoot string       `json:"project_root"`
	Command     []string     `json:"command"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  time.Time    `json:"finished_at"`
	Passed      bool         `json:"passed"`
	ExitCode    int          `json:"exit_code"`
	RunError    string       `json:"run_error,omitempty"`
	OutputPath  string       `json:"output_path"`
	Source      reportSource `json:"source_at_start"`
}

// A source identifier supplements the test result; it is not an automatic
// freshness verdict. Untracked code and later changes still require review.
func captureReportSource(ctx context.Context, root string) reportSource {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	head := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	head.Dir = root
	value, err := head.Output()
	if err != nil {
		return reportSource{}
	}
	source := reportSource{Head: strings.TrimSpace(string(value))}
	diff := exec.CommandContext(ctx, "git", "diff", "--binary", "HEAD")
	diff.Dir = root
	hash := sha256.New()
	diff.Stdout = hash
	if err := diff.Run(); err == nil {
		source.DiffSHA256 = hex.EncodeToString(hash.Sum(nil))
	}
	return source
}

func saveFunctionalReport(opts RunOptions, args []string, started, finished time.Time, source reportSource, output string, runErr error) (string, string, error) {
	root, err := filepath.Abs(opts.ProjectRoot)
	if err != nil {
		return "", "", err
	}
	results := opts.TestResultsDir
	if results == "" {
		results = "docs/test-results"
	}
	if !filepath.IsAbs(results) {
		results = filepath.Join(root, results)
	}
	parent := filepath.Join(results, opts.Spec)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", "", err
	}
	dir, err := os.MkdirTemp(parent, "functional-"+started.Format("20060102T150405")+"-")
	if err != nil {
		return "", "", err
	}
	outputPath := filepath.Join(dir, "output.log")
	if err := os.WriteFile(outputPath, []byte(output), 0o600); err != nil {
		return "", "", err
	}
	report := functionalReport{
		Schema: "archetipo/e2e-report/v1", Spec: opts.Spec, ProjectRoot: root,
		Command: append([]string{"npx"}, args...), StartedAt: started, FinishedAt: finished,
		Passed: runErr == nil, OutputPath: outputPath, Source: source,
	}
	if runErr != nil {
		report.ExitCode, report.RunError = -1, runErr.Error()
		var exit interface{ ExitCode() int }
		if errors.As(runErr, &exit) {
			report.ExitCode = exit.ExitCode()
		}
	}
	reportPath := filepath.Join(dir, "report.json")
	if err := recordfile.WriteAtomic(dir, reportPath, ".report-", report); err != nil {
		return "", "", err
	}
	return reportPath, outputPath, nil
}
