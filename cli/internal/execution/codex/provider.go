// Package codex implements the ARchetipo execution provider backed by a Codex
// CLI process running on the workspace machine. It is the local counterpart of
// the arcipelago provider: the same execution.Provider contract, the same
// spec.plan capability, but the agent runs here instead of on a remote hub, so
// no ARcipelago installation is needed to plan a spec through managed
// execution.
//
// The provider never touches the ARchetipo connector. The local agent owns the
// persistence of the plan; this package only starts the process, waits for it
// and reports the outcome. That separation is what guarantees that a failed run
// cannot move the spec.
//
// It also bounds what this package can prove. The receipt it demands is a
// declaration of the agent, not an inspection of the connector, so it rules out
// an agent that simply terminated but not one that lied. Confirming that the
// spec really is PLANNED with a readable plan stays one layer up, in
// execution.VerifyActionEffect, which already holds the connector.
//
// Everything that touches the operating system goes through the injectable
// Runner, so tests exercise the real command building and outcome
// classification without spawning a process. Codex owns its own
// authentication: no credential, and no path to its session material, is ever
// read, stored or logged by this package.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const (
	// availabilityTimeout bounds the probe that asks Codex for its version. It
	// is deliberately short: the question is whether the binary answers at all,
	// not whether it can do work.
	availabilityTimeout = 10 * time.Second

	// maxCapturedOutput bounds how much of a captured stream is ever echoed back
	// into a diagnostic. It is the single limit for both stdout and stderr: the
	// streams of an agent run can be arbitrarily large and can quote whatever
	// the agent read, so no message composed here may carry one whole.
	maxCapturedOutput = 512

	// planSkillRelPath is where `archetipo init --tool codex` installs the
	// planning skill, relative to the workspace root. Codex resolves
	// `/archetipo-plan` from there, so its absence is what makes the invocation
	// impossible rather than merely unlikely to work.
	planSkillRelPath = ".agents/skills/archetipo-plan/SKILL.md"
)

// Runner is the single seam between this package and the operating system. It
// runs one command to completion and reports its streams and exit code, so a
// test can describe an outcome — missing binary, broken binary, non-zero exit —
// without owning a real process.
//
// A non-nil err means the process could not be run to completion at all; in
// that case exitCode carries -1. A command that ran and failed reports a
// non-zero exitCode with a nil err.
type Runner interface {
	Run(ctx context.Context, dir string, name string, args []string) (stdout string, stderr string, exitCode int, err error)
}

// Options carries the injectable seams of the provider. Every field is
// optional: New fills the zero values with the real implementations.
type Options struct {
	Runner     Runner
	WorkingDir func() (string, error)
	Now        func() time.Time
}

// Provider dispatches spec.plan actions to a local Codex process.
type Provider struct {
	runner     Runner
	workingDir func() (string, error)
	now        func() time.Time
}

var (
	_ execution.Provider             = (*Provider)(nil)
	_ execution.AvailabilityReporter = (*Provider)(nil)
)

// New builds a provider, defaulting every unset seam to its real
// implementation. It never returns nil and never inspects the machine: nothing
// here looks for Codex, so constructing the provider stays free of side
// effects.
func New(options Options) *Provider {
	p := &Provider{
		runner:     options.Runner,
		workingDir: options.WorkingDir,
		now:        options.Now,
	}
	if p.runner == nil {
		p.runner = execRunner{}
	}
	if p.workingDir == nil {
		p.workingDir = os.Getwd
	}
	if p.now == nil {
		p.now = time.Now
	}
	return p
}

func (p *Provider) ID() string { return ProviderID }

func (p *Provider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{execution.CapabilitySpecPlan}, nil
}

// ValidateConfig checks the shape of the non-secret configuration only. It must
// not read the environment and must not look the command up on PATH: otherwise
// `execution provider set-default` would become unrunnable on a machine that
// does not have Codex installed, which is exactly the machine a person
// configures before installing it.
func (p *Provider) ValidateConfig(_ context.Context, raw map[string]any) error {
	_, err := parseConfig(raw)
	return err
}

// Execute dispatches one spec.plan action to a local Codex process and reports
// its outcome.
//
// The order is deliberate: everything that can be known before spawning is
// checked before spawning. A missing runtime and a missing skill both produce a
// process that would either fail obscurely or burn a full timeout doing
// nothing, and both are fixed by a single command the diagnostic can name. Only
// then is the process started, and only a receipt for the dispatched spec turns
// its exit code 0 into a success.
//
// It never returns an execution.RemoteError. There is no remote unit of work
// that outlives this call: when Execute returns, the local process has either
// exited or been killed by the timeout, so there is nothing left for a caller
// to follow.
func (p *Provider) Execute(ctx context.Context, req execution.Request) (execution.Result, error) {
	cfg, err := parseConfig(req.ProviderConfig)
	if err != nil {
		return execution.Result{}, err
	}
	dir, err := p.workingDir()
	if err != nil {
		return execution.Result{}, fmt.Errorf("resolving the working directory to run the codex command %q: %w", cfg.Command, err)
	}
	// The availability probe is already the explicit diagnostic for a runtime
	// that is absent or broken, so it travels back unchanged rather than being
	// wrapped into a vaguer sentence.
	if err := p.Available(ctx, req.ProviderConfig); err != nil {
		return execution.Result{}, err
	}
	if err := ensurePlanSkill(dir); err != nil {
		return execution.Result{}, err
	}

	prompt := buildPrompt(req)
	args := buildArgs(cfg, prompt)

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	startedAt := p.now()
	stdout, stderr, exitCode, err := p.runner.Run(runCtx, dir, cfg.Command, args)
	elapsed := p.now().Sub(startedAt)

	if err != nil {
		return execution.Result{}, fmt.Errorf("the codex command %q could not be run to completion after %s: %w", cfg.Command, elapsed.Round(time.Millisecond), err)
	}
	if exitCode != 0 {
		return execution.Result{}, fmt.Errorf(
			"the codex command %q exited %d after %s without planning %s%s",
			cfg.Command, exitCode, elapsed.Round(time.Millisecond), req.SpecCode, diagnosticSuffix(stderr),
		)
	}
	// The acceptance rule is the shared one: a receipt this provider accepted
	// and another rejected would be a contract that exists twice.
	receipt, err := execution.AcceptPlanReceipt(stdout, req.SpecCode)
	if err != nil {
		return execution.Result{}, fmt.Errorf(
			"the codex command %q exited 0 without having produced a plan for %s: %w",
			cfg.Command, req.SpecCode, err,
		)
	}
	return p.resultFor(cfg, exitCode, elapsed, receipt)
}

// ensurePlanSkill refuses to spawn Codex when the planning skill it is asked to
// invoke is not installed in the workspace. Without this check the run would
// still start, spend a full timeout, and fail with whatever Codex says about an
// unknown command — a diagnostic that points nowhere. The message names the
// command that fixes it instead.
func ensurePlanSkill(dir string) error {
	path := filepath.Join(dir, planSkillRelPath)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("the ARchetipo planning skill is not installed for codex in %s (expected %s): run `archetipo init --tool codex` in that directory", dir, planSkillRelPath)
	}
	return nil
}

// resultFor builds the execution payload. It carries exactly six fields, and
// none of them is a stream: the agent's stdout and stderr never enter the
// record. result_summary is the receipt line alone, re-rendered from the parsed
// receipt rather than sliced out of the output, so nothing the agent printed
// around it can travel with it.
func (p *Provider) resultFor(cfg settings, exitCode int, elapsed time.Duration, receipt execution.PlanReceipt) (execution.Result, error) {
	summary, err := json.Marshal(receipt)
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the codex plan receipt: %w", err)
	}
	payload, err := json.Marshal(struct {
		Command       string `json:"command"`
		Model         string `json:"model"`
		ExitCode      int    `json:"exit_code"`
		ResultSummary string `json:"result_summary"`
		PlanTasks     int    `json:"plan_tasks"`
		DurationMS    int64  `json:"duration_ms"`
	}{
		Command:       cfg.Command,
		Model:         cfg.Model,
		ExitCode:      exitCode,
		ResultSummary: string(summary),
		PlanTasks:     receipt.Tasks,
		DurationMS:    elapsed.Milliseconds(),
	})
	if err != nil {
		return execution.Result{}, fmt.Errorf("encoding the codex execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}

// Available reports whether the configured Codex runtime can actually be used,
// by asking the binary for its version. It answers nil only when that command
// ran and exited 0.
//
// Every failure names the command that was looked for, because the most common
// cause is a configured name that does not match what is installed, and it
// keeps the three causes apart: not on PATH at all, present but impossible to
// start, and started but failing. The last one is the case the baseline
// observed — a wrapper script on PATH that dies with `spawn ... ENOENT` — and
// it must read as unavailable, not as available.
func (p *Provider) Available(ctx context.Context, raw map[string]any) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return fmt.Errorf("the codex command %q was not found on this machine: install Codex or set the provider's `command` to the executable to use", cfg.Command)
	}
	dir, err := p.workingDir()
	if err != nil {
		return fmt.Errorf("resolving the working directory to probe the codex command %q: %w", cfg.Command, err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, availabilityTimeout)
	defer cancel()

	_, stderr, exitCode, err := p.runner.Run(probeCtx, dir, cfg.Command, []string{"--version"})
	if err != nil {
		return fmt.Errorf("the codex command %q was found but could not be run: %w", cfg.Command, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("the codex command %q exited %d instead of reporting its version%s", cfg.Command, exitCode, diagnosticSuffix(stderr))
	}
	return nil
}

// diagnosticSuffix appends the tail of a failing command's stderr, or says
// plainly that it wrote nothing, so an empty stream never reads as a truncated
// message.
func diagnosticSuffix(stderr string) string {
	body := strings.TrimSpace(stderr)
	if body == "" {
		return " (it wrote nothing on standard error)"
	}
	return ": " + truncate(body)
}

// truncate bounds an echoed stream and cuts on a rune boundary, so a clipped
// message never ends in half a character.
func truncate(body string) string {
	if len(body) <= maxCapturedOutput {
		return body
	}
	cut := maxCapturedOutput
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + "..."
}
