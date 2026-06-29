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

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
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
