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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
		p.runner = localrun.ExecRunner{}
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

	subject := specSubject{gerund: "planning", outcome: "a plan for " + req.SpecCode}
	held, err := p.runSpecConversation(runCtx, req, cfg, dir, buildPrompt(req), subject, func(message string) bool {
		_, err := execution.AcceptPlanReceipt(message, req.SpecCode)
		return err == nil
	})
	if err != nil {
		return execution.Result{}, err
	}

	// The acceptance rule is the shared one: a receipt this provider accepted
	// and another rejected would be a contract that exists twice. The receipt is
	// looked for in the message a turn ends on, which is where the prompt asks
	// for it — never in the stream as a whole. It is parsed back here rather
	// than carried out of the conversation as a typed value, for the reason
	// written on converse: the loop recognizes an acceptable closing message,
	// and which artifact that message describes is this layer's knowledge.
	receipt, err := execution.AcceptPlanReceipt(held.final, req.SpecCode)
	if err != nil {
		held.session.Close(execution.RunCrashed, fmt.Sprintf("the session ended without a plan for %s", req.SpecCode))
		return execution.Result{}, fmt.Errorf(
			"the claude command %q ended without having produced a plan for %s%s: %w",
			cfg.Command, req.SpecCode, localrun.DiagnosticSuffix(held.stderr), err,
		)
	}
	held.session.Close(execution.RunClosed, "")
	return p.resultFor(cfg, held.exitCode, held.elapsed, receipt)
}

// specSubject is how one spec-scoped action names itself in a diagnostic.
//
// It exists because the three actions differ in exactly two words and in
// nothing else: what the agent was doing, and what it had to produce. Writing
// the four failures once and parameterizing them on those two is what keeps
// "did not finish planning US-1" and "did not finish implementing US-1" two
// different diagnoses of two different runs, without three copies of the same
// switch free to classify the same failure differently.
type specSubject struct {
	// gerund is the work in progress, as "did not finish <gerund> US-1 within
	// 1h" reads it: "planning", "implementing", "preparing the review of".
	gerund string
	// outcome is what the run had to produce, as "without having produced
	// <outcome>" reads it: "a plan for US-1", "the review of US-1".
	outcome string
}

// specConversation is what a finished spec-scoped conversation leaves behind:
// the message the agent closed on, the facts a payload or a diagnostic is built
// from, and the still-open session — which only the caller can close, because
// only the caller knows whether the message it was given is the receipt it
// asked for.
type specConversation struct {
	session *localrun.Session
	final   string
	// turns counts how many times the agent finished speaking before it was
	// done. It is the cheapest honest measure of a conversation, and it is the
	// number that says out loud that these actions are no longer single-turn.
	turns    int
	exitCode int
	stderr   string
	elapsed  time.Duration
}

// runSpecConversation runs one spec-scoped action as a conversation and reports
// the message it ended on, or the reason there is none.
//
// It is the shape of planning, implementation and review. It used to be one
// turn, and the reasoning for that was sound as far as it went: those three
// actions end on a receipt, so a turn that ends without one has not asked a
// question. What it missed is that a turn can end without a receipt for a
// reason that is neither success nor failure — the agent needs something only a
// person can give, and says so. Under one turn that sentence was the end of the
// work; here it is the agent waiting, and the answer arrives through the run's
// own dialogue exactly as it does in an inception.
//
// The receipt is therefore tried at the end of every turn and not only of the
// first. Nothing else about the acceptance changed: it is still the shared gate,
// still read from the message the turn ends on, and still a declaration of the
// agent rather than an inspection of the connector.
//
// Every failure closes the session before returning; a success deliberately
// leaves it open.
func (p *Provider) runSpecConversation(runCtx context.Context, req execution.Request, cfg settings, dir, prompt string, subject specSubject, accept func(string) bool) (*specConversation, error) {
	// Conversational, which is what the whole change amounts to at the protocol
	// level: the end of a turn no longer ends the session, so a message sent
	// after it opens the next turn instead of being refused.
	live, err := p.openSession(runCtx, req, cfg, dir, prompt, true, false, 0)
	if err != nil {
		return nil, err
	}

	final, turns, convErr := converse(runCtx, live.client, accept)

	exitCode, stderr, waitErr := p.shutdown(live.process)
	elapsed := p.now().Sub(live.startedAt)

	if convErr != nil {
		// The process left after the agent had finished speaking, and what it
		// finished on was not the receipt. That is not one of the four failures
		// below: it is a closing message that has to be judged, and only the
		// caller can judge it — it is the one layer that knows which receipt it
		// asked for and can therefore say what was wrong with the one it got.
		// Handing it over is what keeps "did not emit the expected JSON receipt
		// line" in the record instead of a flat "the run ended".
		if errors.Is(convErr, errProcessGone) && live.client.Completed() {
			return &specConversation{
				session:  live.session,
				final:    live.client.FinalMessage(),
				turns:    turns,
				exitCode: exitCode,
				stderr:   stderr,
				elapsed:  elapsed,
			}, nil
		}
		return nil, p.failSpecConversation(live, cfg, req, subject, runCtx.Err(), convErr, exitCode, stderr, elapsed)
	}
	if waitErr != nil {
		live.session.Close(execution.RunCrashed, waitErr.Error())
		return nil, fmt.Errorf("the claude command %q could not be run to completion after %s: %w", cfg.Command, elapsed.Round(time.Millisecond), waitErr)
	}
	return &specConversation{
		session:  live.session,
		final:    final,
		turns:    turns,
		exitCode: exitCode,
		stderr:   stderr,
		elapsed:  elapsed,
	}, nil
}

// failSpecConversation closes the run and composes the diagnostic for a
// spec-scoped conversation that ended without a receipt.
//
// It keeps the four causes apart, because they are four different things to
// fix: the deadline, a cancellation, a turn the agent itself closed with an
// error, and a process that left. The wording of the first two is the one those
// runs have always had, so a record written before this change and one written
// after read the same way for the same failure.
func (p *Provider) failSpecConversation(live *liveSession, cfg settings, req execution.Request, subject specSubject, runErr, convErr error, exitCode int, stderr string, elapsed time.Duration) error {
	rounded := elapsed.Round(time.Millisecond)
	switch {
	case errors.Is(convErr, errRunTerminated) && errors.Is(runErr, context.DeadlineExceeded):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude session was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q did not finish %s %s within %s", cfg.Command, subject.gerund, req.SpecCode, cfg.Timeout)
	case errors.Is(convErr, errRunTerminated):
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude session was stopped after %s: %v", rounded, runErr))
		return fmt.Errorf("the claude command %q was stopped after %s without having produced %s: %v", cfg.Command, rounded, subject.outcome, runErr)
	default:
		// A turn the agent closed with an error, and a process that left in the
		// middle of one, are the same sentence because they are the same fact:
		// the turn never completed, so whatever the agent last said cannot be
		// read as the outcome of work it never finished. An interrupted turn is
		// the reason the exit code cannot be the whole condition — stopping the
		// process afterwards is an ordinary shutdown and can exit 0.
		live.session.Close(execution.RunCrashed, fmt.Sprintf("the claude process exited %d without completing the turn", exitCode))
		return fmt.Errorf(
			"the claude command %q exited %d after %s without %s %s: the turn never completed%s",
			cfg.Command, exitCode, rounded, subject.gerund, req.SpecCode, localrun.DiagnosticSuffix(stderr),
		)
	}
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
	// Written before the reader starts, which is the one instant at which
	// nothing is looking at it yet: from `go client.consume()` on, the clock
	// belongs to the goroutine that stamps the questions the process asks.
	client.now = p.now
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
		return fmt.Errorf("the claude command %q exited %d instead of reporting its version%s", cfg.Command, exitCode, localrun.DiagnosticSuffix(stderr))
	}
	return nil
}

// diagnosticSuffix appends the beginning of a failing command's stderr, bounded
// by truncate, or says plainly that it wrote nothing, so an empty stream never
// reads as a truncated message.
