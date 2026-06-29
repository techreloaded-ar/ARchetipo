// Package e2e contains the deterministic plumbing behind the `archetipo e2e`
// commands. It keeps the framework-specific knowledge (today: Playwright)
// isolated behind a small internal seam so a second framework can be added
// later as a new type + switch, without changing the skills that call the CLI.
//
// The package deliberately avoids the full connector-style abstraction
// (registry + conformance + config selector): with a single supported
// framework that machinery would be dead code.
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// FrameworkPlaywright is the only e2e framework supported today.
const FrameworkPlaywright = "playwright"

// DefaultBrowser is installed by Ensure unless overridden. A single browser
// keeps the bootstrap fast and avoids downloading firefox + webkit when only
// chromium is needed.
const DefaultBrowser = "chromium"

// playwrightConfigNames are the file names recognized as a Playwright config.
var playwrightConfigNames = []string{
	"playwright.config.ts",
	"playwright.config.js",
	"playwright.config.mjs",
	"playwright.config.cjs",
	"playwright.config.mts",
	"playwright.config.cts",
}

// Runner executes an external command in a working directory. It is abstracted
// so tests can stub command execution without touching npm/npx.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (output string, err error)
}

// osRunner runs commands via os/exec with a non-interactive environment so the
// bootstrap never blocks waiting on stdin (the #1 cause of "stuck" runs).
type osRunner struct{}

func (osRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = nil // never read from stdin: no interactive prompts
	cmd.Env = append(os.Environ(), "CI=1", "npm_config_yes=true")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Detection reports the current e2e state of a project.
type Detection struct {
	Framework  string `json:"framework"`             // "playwright" or "" when unknown
	Installed  bool   `json:"installed"`             // @playwright/test present in package.json
	ConfigPath string `json:"config_path,omitempty"` // relative path of the config file, if any
}

// EnsureOptions configures Ensure. Runner defaults to the OS runner.
type EnsureOptions struct {
	ProjectRoot string
	Browser     string
	WithDeps    bool
	Runner      Runner
}

// EnsureResult is the JSON payload returned by `archetipo e2e ensure`.
type EnsureResult struct {
	Framework  string   `json:"framework"`
	Action     string   `json:"action"` // "already-present" | "installed"
	Installed  bool     `json:"installed"`
	ConfigPath string   `json:"config_path"`
	Browser    string   `json:"browser"`
	Steps      []string `json:"steps"`
}

// Detect inspects projectRoot and reports whether Playwright is set up. It only
// reads the filesystem; it never runs commands.
func Detect(projectRoot string) (Detection, error) {
	pkg := filepath.Join(projectRoot, "package.json")
	raw, err := os.ReadFile(pkg)
	if err != nil {
		if os.IsNotExist(err) {
			return Detection{}, nil
		}
		return Detection{}, err
	}
	installed := strings.Contains(string(raw), "@playwright/test")
	cfg := ""
	for _, name := range playwrightConfigNames {
		if _, err := os.Stat(filepath.Join(projectRoot, name)); err == nil {
			cfg = name
			break
		}
	}
	det := Detection{Installed: installed, ConfigPath: cfg}
	if installed || cfg != "" {
		det.Framework = FrameworkPlaywright
	}
	return det, nil
}

// Ensure idempotently bootstraps the Playwright e2e stack: installs
// @playwright/test (only if missing), writes a minimal config (only if absent,
// never overwriting an existing one) and installs the configured browser
// (idempotent — Playwright skips browsers already present). It is
// non-interactive and never downloads more than the configured browser.
func Ensure(ctx context.Context, opts EnsureOptions) (EnsureResult, error) {
	root := opts.ProjectRoot
	if root == "" {
		return EnsureResult{}, iox.NewInvalidInput("project root is empty", "run inside a project directory", nil)
	}
	browser := opts.Browser
	if browser == "" {
		browser = DefaultBrowser
	}
	run := opts.Runner
	if run == nil {
		run = osRunner{}
	}

	if _, err := os.Stat(filepath.Join(root, "package.json")); err != nil {
		if os.IsNotExist(err) {
			return EnsureResult{}, iox.NewPrecondition(
				"no package.json found in the project root",
				"e2e ensure expects a Node.js project; run `npm init -y` first or set up the project", nil)
		}
		return EnsureResult{}, iox.NewInternal("reading package.json", err)
	}

	det, err := Detect(root)
	if err != nil {
		return EnsureResult{}, iox.NewInternal("detecting e2e framework", err)
	}
	if det.Framework != "" && det.Framework != FrameworkPlaywright {
		return EnsureResult{}, iox.NewPrecondition(
			fmt.Sprintf("unsupported e2e framework %q", det.Framework),
			"archetipo e2e supports Playwright only; configure the other framework manually", nil)
	}

	res := EnsureResult{Framework: FrameworkPlaywright, Browser: browser, Action: "already-present"}

	// 1. Install @playwright/test only when missing.
	if !det.Installed {
		if out, err := run.Run(ctx, root, "npm", "install", "--save-dev", "@playwright/test"); err != nil {
			return EnsureResult{}, iox.NewConnector("", "npm install @playwright/test failed", firstLine(out), err)
		}
		res.Action = "installed"
		res.Steps = append(res.Steps, "installed @playwright/test")
	}
	res.Installed = true

	// 2. Write a minimal config only when none exists. Never overwrite.
	cfgPath := det.ConfigPath
	if cfgPath == "" {
		cfgPath = "playwright.config.ts"
		if err := os.WriteFile(filepath.Join(root, cfgPath), []byte(minimalPlaywrightConfig), 0o644); err != nil {
			return EnsureResult{}, iox.NewInternal("writing "+cfgPath, err)
		}
		res.Steps = append(res.Steps, "wrote "+cfgPath)
	}
	res.ConfigPath = cfgPath

	// 3. Install the browser (idempotent: Playwright skips ones already present).
	args := []string{"playwright", "install"}
	if opts.WithDeps {
		args = append(args, "--with-deps")
	}
	args = append(args, browser)
	if out, err := run.Run(ctx, root, "npx", args...); err != nil {
		return EnsureResult{}, iox.NewConnector("", "npx playwright install "+browser+" failed", firstLine(out), err)
	}
	res.Steps = append(res.Steps, "ensured "+browser+" browser")

	return res, nil
}

// RunOptions configures RunFunctional.
type RunOptions struct {
	ProjectRoot string
	Grep        string
	Runner      Runner
}

// RunResult is the JSON payload returned by `archetipo e2e run`. A failing test
// suite is a result (Passed=false), not a command error: the command still
// exits 0 so the caller branches on data.passed.
type RunResult struct {
	Framework string `json:"framework"`
	Passed    bool   `json:"passed"`
	Output    string `json:"output,omitempty"`
}

// RunFunctional runs the Playwright suite headless (no video, no slowMo —
// recording belongs to `e2e demo`). Requires Ensure to have been run.
func RunFunctional(ctx context.Context, opts RunOptions) (RunResult, error) {
	root := opts.ProjectRoot
	if root == "" {
		return RunResult{}, iox.NewInvalidInput("project root is empty", "run inside a project directory", nil)
	}
	run := opts.Runner
	if run == nil {
		run = osRunner{}
	}
	det, err := Detect(root)
	if err != nil {
		return RunResult{}, iox.NewInternal("detecting e2e framework", err)
	}
	if !det.Installed {
		return RunResult{}, iox.NewPrecondition(
			"Playwright is not installed in this project",
			"run `archetipo e2e ensure` first", nil)
	}
	args := []string{"playwright", "test", "--reporter=list"}
	if opts.Grep != "" {
		args = append(args, "--grep", opts.Grep)
	}
	out, runErr := run.Run(ctx, root, "npx", args...)
	return RunResult{Framework: FrameworkPlaywright, Passed: runErr == nil, Output: tail(out)}, nil
}

// Demo defaults keep the recorded video watchable by a non-technical reviewer.
const (
	DefaultSlowMoMs  = 300
	DefaultViewportW = 1280
	DefaultViewportH = 720
)

const demoConfigName = ".archetipo-e2e-demo.config.ts"

// DemoOptions configures RecordDemo.
type DemoOptions struct {
	ProjectRoot    string
	Spec           string // e.g. US-001; used as the artifact subfolder
	Grep           string // selects the single demo test to record
	TestResultsDir string // relative to ProjectRoot; defaults to docs/test-results
	SlowMoMs       int
	ViewportW      int
	ViewportH      int
	Runner         Runner
}

// DemoResult is the JSON payload returned by `archetipo e2e demo`.
type DemoResult struct {
	Framework string `json:"framework"`
	Spec      string `json:"spec"`
	Passed    bool   `json:"passed"`
	VideoPath string `json:"video_path,omitempty"` // relative to ProjectRoot; empty when none produced
	OutputDir string `json:"output_dir,omitempty"` // relative to ProjectRoot
	Output    string `json:"output,omitempty"`
	// Skipped reports that recording was deliberately not attempted (e.g. demo
	// recording disabled in config). Distinct from a recording that ran but
	// produced no video. Reason carries the human-readable explanation.
	Skipped bool   `json:"skipped"`
	Reason  string `json:"reason,omitempty"`
}

// RecordDemo runs a single demo test with deterministic recording (video on,
// slow motion, fixed viewport) injected via an ephemeral Playwright config that
// extends the project's own config — so the demo test file stays a plain
// scenario. The recorded video is collected under <TestResultsDir>/<Spec>/.
func RecordDemo(ctx context.Context, opts DemoOptions) (DemoResult, error) {
	root := opts.ProjectRoot
	if root == "" {
		return DemoResult{}, iox.NewInvalidInput("project root is empty", "run inside a project directory", nil)
	}
	if strings.TrimSpace(opts.Spec) == "" {
		return DemoResult{}, iox.NewInvalidInput("missing spec code", "usage: archetipo e2e demo --spec US-XXX --grep <demo>", nil)
	}
	run := opts.Runner
	if run == nil {
		run = osRunner{}
	}
	det, err := Detect(root)
	if err != nil {
		return DemoResult{}, iox.NewInternal("detecting e2e framework", err)
	}
	if !det.Installed {
		return DemoResult{}, iox.NewPrecondition(
			"Playwright is not installed in this project",
			"run `archetipo e2e ensure` first", nil)
	}

	slowMo := opts.SlowMoMs
	if slowMo == 0 {
		slowMo = DefaultSlowMoMs
	}
	w, h := opts.ViewportW, opts.ViewportH
	if w == 0 {
		w = DefaultViewportW
	}
	if h == 0 {
		h = DefaultViewportH
	}
	resultsDir := opts.TestResultsDir
	if resultsDir == "" {
		resultsDir = "docs/test-results"
	}
	outputRel := filepath.Join(resultsDir, opts.Spec)
	outputAbs := filepath.Join(root, outputRel)
	if err := os.MkdirAll(outputAbs, 0o755); err != nil {
		return DemoResult{}, iox.NewInternal("creating output dir", err)
	}

	cfgPath := filepath.Join(root, demoConfigName)
	if err := os.WriteFile(cfgPath, []byte(demoConfig(det.ConfigPath, filepath.ToSlash(outputAbs), slowMo, w, h)), 0o644); err != nil {
		return DemoResult{}, iox.NewInternal("writing ephemeral demo config", err)
	}
	defer os.Remove(cfgPath)

	args := []string{"playwright", "test", "--config", demoConfigName, "--reporter=list"}
	if opts.Grep != "" {
		args = append(args, "--grep", opts.Grep)
	}
	out, runErr := run.Run(ctx, root, "npx", args...)

	res := DemoResult{
		Framework: FrameworkPlaywright,
		Spec:      opts.Spec,
		Passed:    runErr == nil,
		OutputDir: outputRel,
		Output:    tail(out),
	}
	if video := findVideo(outputAbs); video != "" {
		if rel, err := filepath.Rel(root, video); err == nil {
			res.VideoPath = rel
		} else {
			res.VideoPath = video
		}
	}
	return res, nil
}

// findVideo returns the first .webm file found under dir, or "".
func findVideo(dir string) string {
	var found string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".webm") {
			found = path
		}
		return nil
	})
	return found
}

// demoConfig builds the ephemeral Playwright config. When the project already
// has a config it is imported and spread so baseURL/webServer/projects carry
// over; otherwise a standalone config is emitted.
func demoConfig(baseConfig, outputDir string, slowMo, w, h int) string {
	use := fmt.Sprintf(`video: 'on',
    viewport: { width: %d, height: %d },
    launchOptions: { slowMo: %d },`, w, h, slowMo)

	if importable(baseConfig) {
		base := "./" + strings.TrimSuffix(baseConfig, filepath.Ext(baseConfig))
		return fmt.Sprintf(`import { defineConfig } from '@playwright/test';
import base from '%s';

// Generated by 'archetipo e2e demo'. Extends the project config to record a
// watchable demo video. Safe to delete.
export default defineConfig({
  ...(base as any),
  outputDir: '%s',
  use: {
    ...((base as any).use ?? {}),
    %s
  },
});
`, base, outputDir, use)
	}

	return fmt.Sprintf(`import { defineConfig, devices } from '@playwright/test';

// Generated by 'archetipo e2e demo'. Standalone demo recording config.
export default defineConfig({
  testDir: './tests',
  outputDir: '%s',
  use: {
    trace: 'on-first-retry',
    %s
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
`, outputDir, use)
}

func importable(baseConfig string) bool {
	switch filepath.Ext(baseConfig) {
	case ".ts", ".js":
		return true
	default:
		return false
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// tail caps command output so the JSON envelope stays small; the most relevant
// information (failures, summary) is at the end of a Playwright run.
func tail(s string) string {
	const max = 4000
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return "...\n" + s[len(s)-max:]
}

const minimalPlaywrightConfig = `import { defineConfig, devices } from '@playwright/test';

// Generated by 'archetipo e2e ensure'. Video stays off globally; the demo
// recording is configured per-run by 'archetipo e2e demo'.
export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  reporter: 'list',
  use: {
    trace: 'on-first-retry',
    video: 'off',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
`
