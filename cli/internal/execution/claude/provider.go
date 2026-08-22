// Package claude implements the ARchetipo execution provider backed by a Claude
// Code CLI process running on the workspace machine. It is a local provider
// like codex, and a sibling of it rather than a variation: the same
// execution.Provider contract and the same spec.plan capability, but a
// different CLI, different flags and different diagnostics, so the two are kept
// as separate implementations of the shared contracts instead of one
// parameterized "local process" provider.
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
// classification without spawning a process. Claude owns its own
// authentication: no credential, and no path to its session material, is ever
// read, stored or logged by this package.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

const (
	// availabilityTimeout bounds the probe that asks Claude for its version. It
	// is deliberately short: the question is whether the binary answers at all,
	// not whether it can do work.
	availabilityTimeout = 10 * time.Second

	// maxCapturedOutput bounds how much of a captured stream is ever echoed back
	// into a diagnostic. It is the single limit for both stdout and stderr: the
	// streams of an agent run can be arbitrarily large and can quote whatever
	// the agent read, so no message composed here may carry one whole. The value
	// lives in localrun, which is where the local process is read, so the limit
	// applied while capturing and the limit applied while quoting cannot drift.
	maxCapturedOutput = localrun.MaxCapturedOutput

	// planSkillRelPath is where `archetipo init --tool claude` installs the
	// planning skill, relative to the workspace root. Claude resolves
	// `/archetipo-plan` from there, so its absence is what makes the invocation
	// impossible rather than merely unlikely to work.
	planSkillRelPath = ".claude/skills/archetipo-plan/SKILL.md"

	// inceptionSkillRelPath is the same fact for the inception skill: it is
	// what `/archetipo-inception` resolves to, and its absence is what makes
	// the conversation impossible rather than merely unlikely to work.
	inceptionSkillRelPath = ".claude/skills/archetipo-inception/SKILL.md"

	// backlogSkillRelPath is the same fact for the backlog skill: it is what
	// `/archetipo-spec` resolves to, and its absence is what makes the
	// conversation impossible rather than merely unlikely to work.
	backlogSkillRelPath = ".claude/skills/archetipo-spec/SKILL.md"

	// implementSkillRelPath is the same fact for the implementation skill: it
	// is what `/archetipo-implement` resolves to, and its absence is what makes
	// the invocation impossible rather than merely unlikely to work.
	implementSkillRelPath = ".claude/skills/archetipo-implement/SKILL.md"

	// reviewSkillRelPath is the same fact for the review skill: it is what
	// makes `/archetipo-review` invocable at all, so its absence is the one
	// thing worth checking before a process is spawned to run it.
	reviewSkillRelPath = ".claude/skills/archetipo-review/SKILL.md"

	// shutdownGrace bounds how long the session process is given to exit on its
	// own after its input is closed, before it is signalled.
	shutdownGrace = 5 * time.Second
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
	// Runner runs one command to completion. It serves the availability probe,
	// which is a one-shot question and must stay one.
	Runner Runner
	// Starter starts the live session process. It is the seam the dialogue goes
	// through, so a test can describe a whole conversation without a machine
	// that has Claude Code on it.
	Starter    localrun.Starter
	WorkingDir func() (string, error)
	Now        func() time.Time
}

// Provider dispatches spec.plan actions to a local Claude Code session, and
// exposes that session as a run one can follow and command.
//
// The embedded collaborator is what makes the provider collaborative: it brings
// the seven methods of execution.RunCollaborator, implemented once over the
// rules of a local run, so this package adds only what really is its own — how
// the process is started and how it is spoken to.
type Provider struct {
	*localrun.Collaborator
	runner     Runner
	starter    localrun.Starter
	workingDir func() (string, error)
	now        func() time.Time

	// conversations holds the free conversations this provider is currently
	// keeping alive, keyed by conversation id. It is deliberately separate from
	// the session registry the Collaborator brings: the registry keeps every
	// session forever, because the history of a finished run stays readable,
	// while this map holds only what is still running and therefore still has a
	// process to release. A conversation that has ended leaves this map and
	// stays in the registry, which is exactly the difference between "still
	// commandable" and "still readable".
	conversationsMu sync.Mutex
	conversations   map[string]*liveConversation
}

var (
	_ execution.Provider             = (*Provider)(nil)
	_ execution.AvailabilityReporter = (*Provider)(nil)
	_ execution.RunCollaborator      = (*Provider)(nil)
	_ execution.Conversationalist    = (*Provider)(nil)
)

// New builds a provider, defaulting every unset seam to its real
// implementation. It never returns nil and never inspects the machine: nothing
// here looks for Claude, so constructing the provider stays free of side
// effects.
func New(options Options) *Provider {
	p := &Provider{
		Collaborator: localrun.NewCollaborator(localrun.NewRegistry()),
		runner:       options.Runner,
		starter:      options.Starter,
		workingDir:   options.WorkingDir,
		now:          options.Now,

		conversations: make(map[string]*liveConversation),
	}
	if p.runner == nil {
		p.runner = execRunner{}
	}
	if p.starter == nil {
		p.starter = localrun.ExecStarter{}
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

// Capabilities declares the actions this provider can dispatch, and nothing
// else. run.dialog is not among them on purpose: it is derived from the
// interfaces the provider implements, so declaring it by hand here would be
// exactly the mismatch that derivation exists to make impossible.
func (p *Provider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{
		execution.CapabilitySpecPlan,
		execution.CapabilitySpecImplement,
		execution.CapabilitySpecReview,
		execution.CapabilityWorkspaceInception,
		execution.CapabilityWorkspaceBacklog,
		execution.CapabilityWorkspaceSpecDraft,
	}, nil
}

// ValidateConfig checks the shape of the non-secret configuration only. It must
// not read the environment and must not look the command up on PATH: otherwise
// `execution provider set-default` would become unrunnable on a machine that
// does not have Claude Code installed, which is exactly the machine a person
// configures before installing it.
func (p *Provider) ValidateConfig(_ context.Context, raw map[string]any) error {
	_, err := parseConfig(raw)
	return err
}

// Execute dispatches one spec.plan action to a local Claude process and reports
// its outcome.
//
// The order is deliberate: everything that can be known before spawning is
// checked before spawning. A missing runtime and a missing skill both produce a
// process that would either fail obscurely or burn a full timeout doing
// nothing, and both are fixed by a single command the diagnostic can name. Only
// then is the session started, and only a receipt for the dispatched spec turns
// a finished turn into a success.
//
// It never returns an execution.RemoteError. There is no remote unit of work
// that outlives this call: when Execute returns, the local process has either
// exited or been killed by the timeout, so there is nothing left for a caller
// to follow.
func (p *Provider) Execute(ctx context.Context, req execution.Request) (execution.Result, error) {
	cfg, dir, err := p.prepare(ctx, req)
	if err != nil {
		return execution.Result{}, err
	}
	// The fork is on the action and nothing else. Planning keeps the flow it
	// has always had — one turn, then a receipt — because the moment a turn
	// ends without one it has failed, and that diagnostic must not become a
	// wait. Inception, backlog generation and spec drafting are the other
	// semantics, and each lives in its own function rather than as a set of
	// conditions inside this one.
	if req.Action == execution.ActionInception {
		return p.executeInception(ctx, req, cfg, dir)
	}
	if req.Action == execution.ActionBacklog {
		return p.executeBacklog(ctx, req, cfg, dir)
	}
	if req.Action == execution.ActionSpecDraft {
		return p.executeSpecDraft(ctx, req, cfg, dir)
	}
	if req.Action == execution.ActionImplement {
		return p.executeImplement(ctx, req, cfg, dir)
	}
	if req.Action == execution.ActionReview {
		return p.executeReview(ctx, req, cfg, dir)
	}
	if err := ensureSkill(dir, planSkillRelPath, "planning"); err != nil {
		return execution.Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	turn, err := p.runSingleTurn(runCtx, req, cfg, dir, buildPrompt(req), "planning")
	if err != nil {
		return execution.Result{}, err
	}

	// The acceptance rule is the shared one: a receipt this provider accepted
	// and another rejected would be a contract that exists twice. The receipt is
	// looked for in the message the run ends on, which is where the prompt asks
	// for it — never in the stream as a whole.
	receipt, err := execution.AcceptPlanReceipt(turn.final, req.SpecCode)
	if err != nil {
		turn.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without a plan for %s", req.SpecCode))
		return execution.Result{}, fmt.Errorf(
			"the claude command %q ended without having produced a plan for %s%s: %w",
			cfg.Command, req.SpecCode, diagnosticSuffix(turn.stderr), err,
		)
	}
	turn.session.Close(execution.RunClosed, "")
	return p.resultFor(cfg, turn.exitCode, turn.elapsed, receipt)
}

// singleTurn is what a completed one-turn run leaves behind: the message the
// agent ended on, the facts a payload or a diagnostic is built from, and the
// still-open session, which only the caller can close — because only the caller
// knows whether the message it was given is the receipt it asked for.
type singleTurn struct {
	session  *localrun.Session
	final    string
	exitCode int
	stderr   string
	elapsed  time.Duration
}

// runSingleTurn runs one prompt as one turn and reports the message it ended
// on, or the reason there is none.
//
// It is the shape of every non-conversational action: planning and
// implementation both ask for work that ends on a receipt, so a turn that ends
// without one has failed here and now rather than asked a question. Keeping it
// in one function is what keeps the four failures — the deadline, a process
// that could not be run to completion, a turn that never completed, and a
// session that could not be opened at all — identical for both, instead of two
// copies free to classify them differently.
//
// gerund is the only thing that varies between the callers, and it varies on
// purpose: "did not finish planning US-1" and "did not finish implementing
// US-1" are two different diagnoses of two different runs, and a shared phrase
// would make them indistinguishable in a record.
//
// Every failure closes the session before returning; a success deliberately
// leaves it open.
func (p *Provider) runSingleTurn(runCtx context.Context, req execution.Request, cfg settings, dir, prompt, gerund string) (*singleTurn, error) {
	live, err := p.openSession(runCtx, req, cfg, dir, prompt, false, false, 0)
	if err != nil {
		return nil, err
	}
	session, client := live.session, live.client

	select {
	case <-client.TurnDone():
	case <-runCtx.Done():
	}

	exitCode, stderr, waitErr := p.shutdown(live.process)
	elapsed := p.now().Sub(live.startedAt)

	if runErr := runCtx.Err(); runErr != nil {
		reason := fmt.Sprintf("the claude session was stopped after %s: %v", elapsed.Round(time.Millisecond), runErr)
		session.Close(execution.RunCrashed, reason)
		return nil, fmt.Errorf("the claude command %q did not finish %s %s within %s", cfg.Command, gerund, req.SpecCode, cfg.Timeout)
	}
	if waitErr != nil {
		session.Close(execution.RunCrashed, waitErr.Error())
		return nil, fmt.Errorf("the claude command %q could not be run to completion after %s: %w", cfg.Command, elapsed.Round(time.Millisecond), waitErr)
	}
	// Only a turn the process declared finished, and finished without error,
	// can carry a result. Two different outcomes fail this test and both must:
	// a turn that ended because the process died, and an interrupted turn —
	// which Claude also closes with a `result`, reporting `is_error`. The
	// second one is the reason the exit code cannot be the whole condition:
	// stopping the process after an interrupt is an ordinary shutdown and can
	// exit 0, so a run cancelled by the operator would otherwise be accepted
	// as a success on whatever the agent happened to have said last.
	if !client.Completed() {
		session.Close(execution.RunCrashed, fmt.Sprintf("the claude process exited %d without completing the turn", exitCode))
		return nil, fmt.Errorf(
			"the claude command %q exited %d after %s without %s %s: the turn never completed%s",
			cfg.Command, exitCode, elapsed.Round(time.Millisecond), gerund, req.SpecCode, diagnosticSuffix(stderr),
		)
	}
	return &singleTurn{
		session:  session,
		final:    client.FinalMessage(),
		exitCode: exitCode,
		stderr:   stderr,
		elapsed:  elapsed,
	}, nil
}

// prepare answers everything that can be known before a process exists: the
// configuration, the directory the session will run in, and whether the runtime
// can be used at all. It is shared by every action because none of the three
// answers depends on which action was dispatched, and a second copy of them
// would be free to diverge on the order in which they fail.
func (p *Provider) prepare(ctx context.Context, req execution.Request) (settings, string, error) {
	cfg, err := parseConfig(req.ProviderConfig)
	if err != nil {
		return settings{}, "", err
	}
	// The request wins over the provider's own default: the directory a run has
	// to execute in is a fact of the workspace that started the run, while the
	// provider is shared by every workspace the process serves and only knows
	// where that process happens to have been launched. A request that names no
	// directory falls back, so every caller that never sets it is unchanged.
	dir := strings.TrimSpace(req.WorkingDir)
	if dir == "" {
		resolved, err := p.workingDir()
		if err != nil {
			return settings{}, "", fmt.Errorf("resolving the working directory to run the claude command %q: %w", cfg.Command, err)
		}
		dir = resolved
	}
	// The availability probe is already the explicit diagnostic for a runtime
	// that is absent or broken, so it travels back unchanged rather than being
	// wrapped into a vaguer sentence.
	if err := p.Available(ctx, req.ProviderConfig); err != nil {
		return settings{}, "", err
	}
	return cfg, dir, nil
}

// liveSession is a started process that has already announced itself and can
// already be spoken to: the three things a caller has to hold on to, plus the
// instant the work began.
type liveSession struct {
	process   localrun.Process
	session   *localrun.Session
	client    *streamSession
	startedAt time.Time
}

// openSession starts the process, opens the protocol on it, gives it its
// instruction and makes the run followable and commandable. Everything in it is
// identical for every action but the mode of the session, the prompt, when the
// instruction is delivered and how much history the session keeps, which are
// the four parameters.
//
// deferOpening decides the last of those: false — what every action passes —
// writes the instruction at once and waits for the process to announce itself,
// which is the handshake of always. True holds it back for the first message of
// the person, which is what a free conversation opens with and the only thing
// that makes an open start no work; the reasoning is written out on
// streamSession.hold.
//
// retain bounds that history: 0 — what every execution passes — is the
// unlimited session of always, so no dispatched action changes behaviour, and a
// positive value is the retention window a conversation asks for, because a
// conversation has no end to be read back from and would otherwise grow with
// the time it stays open.
//
// A failure here always leaves the session closed and the process gone: a
// registered run that nothing will ever end would stay ACTIVE forever.
func (p *Provider) openSession(runCtx context.Context, req execution.Request, cfg settings, dir, prompt string, conversational, deferOpening bool, retain int) (*liveSession, error) {
	// The session is registered before anything is started, so the run is
	// followable from the instant it can produce history — including while this
	// call is still inside the agent's work.
	session := localrun.NewBoundedSession(req.ExecutionID, p.now, retain)
	p.Registry().Register(session)

	startedAt := p.now()
	process, err := p.starter.Start(runCtx, dir, cfg.Command, buildArgs(cfg))
	if err != nil {
		session.Close(execution.RunCrashed, err.Error())
		return nil, fmt.Errorf("the claude command %q could not be started: %w", cfg.Command, err)
	}

	client := newStreamSession(process, session, conversational)
	go client.consume()

	if deferOpening {
		client.hold(prompt)
	} else if err := client.start(runCtx, prompt); err != nil {
		_, _, _ = p.shutdown(process)
		session.Close(execution.RunCrashed, err.Error())
		return nil, err
	}
	// Only now can a command be delivered: before the turn exists there is
	// nothing to steer, and a command that arrives earlier is refused as
	// transient rather than delivered into nothing.
	session.AttachDialogue(client)

	return &liveSession{process: process, session: session, client: client, startedAt: startedAt}, nil
}

// shutdown ends the session process and reports how it went.
//
// Closing the standard input is what a stream-json process takes as its cue to
// exit, and it is the ordinary way a finished session ends. The signal is the
// fallback for a build that stays: a Wait with no bound would otherwise keep
// this call inside a process that has decided not to leave.
func (p *Provider) shutdown(process localrun.Process) (int, string, error) {
	_ = process.Close()
	type outcome struct {
		exitCode int
		stderr   string
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		exitCode, stderr, err := process.Wait()
		done <- outcome{exitCode: exitCode, stderr: stderr, err: err}
	}()
	select {
	case result := <-done:
		return result.exitCode, result.stderr, result.err
	case <-time.After(shutdownGrace):
		_ = process.Signal()
		result := <-done
		return result.exitCode, result.stderr, result.err
	}
}

// ensureSkill refuses to spawn Claude when the skill it is asked to invoke is
// not installed in the workspace. Without this check the run would still start,
// spend a full timeout, and fail with whatever Claude says about an unknown
// command — a diagnostic that points nowhere. The message names the command
// that fixes it instead.
//
// It is one function over a name and a path rather than one function per skill:
// the check and the diagnostic are the same fact for every skill this provider
// invokes, and two copies of them would be free to say it differently.
func ensureSkill(dir, relPath, skill string) error {
	path := filepath.Join(dir, relPath)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("the ARchetipo %s skill is not installed for claude in %s (expected %s): run `archetipo init --tool claude` in that directory", skill, dir, relPath)
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
		return execution.Result{}, fmt.Errorf("encoding the claude plan receipt: %w", err)
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
		return execution.Result{}, fmt.Errorf("encoding the claude execution payload: %w", err)
	}
	return execution.Result{Payload: payload}, nil
}

// Available reports whether the configured Claude runtime can actually be used,
// by asking the binary for its version. It answers nil only when that command
// ran and exited 0.
//
// Every failure names the command that was looked for, because the most common
// cause is a configured name that does not match what is installed, and it
// keeps the three causes apart: not on PATH at all, present but impossible to
// start, and started but failing. The last one is what a broken wrapper script
// on PATH looks like, and it must read as unavailable, not as available.
//
// Authentication is deliberately outside this probe: `claude --version` answers
// without a login, so a logged-out Claude reads as available here and fails at
// run time with its own explicit diagnostic. Probing the login would mean
// starting an agent turn on every listing of the providers.
func (p *Provider) Available(ctx context.Context, raw map[string]any) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return fmt.Errorf("the claude command %q was not found on this machine: install Claude Code or set the provider's `command` to the executable to use", cfg.Command)
	}
	dir, err := p.workingDir()
	if err != nil {
		return fmt.Errorf("resolving the working directory to probe the claude command %q: %w", cfg.Command, err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, availabilityTimeout)
	defer cancel()

	_, stderr, exitCode, err := p.runner.Run(probeCtx, dir, cfg.Command, []string{"--version"})
	if err != nil {
		return fmt.Errorf("the claude command %q was found but could not be run: %w", cfg.Command, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("the claude command %q exited %d instead of reporting its version%s", cfg.Command, exitCode, diagnosticSuffix(stderr))
	}
	return nil
}

// diagnosticSuffix appends the beginning of a failing command's stderr, bounded
// by truncate, or says plainly that it wrote nothing, so an empty stream never
// reads as a truncated message.
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
