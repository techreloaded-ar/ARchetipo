// Package claude adapts its local harness to the shared Provider and
// SessionProvider contracts. The batch workflow runner lives in localrun;
// this package owns process startup, protocol messages and native sessions.
// Persistence and effect verification belong to the connector and service.
package claude

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
type Runner = localrun.Runner

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

	nativeSessionsMu sync.Mutex
	nativeSessions   map[string]*nativeSession
}

var (
	_ execution.Provider             = (*Provider)(nil)
	_ execution.AvailabilityReporter = (*Provider)(nil)
	_ execution.RunCollaborator      = (*Provider)(nil)
	_ execution.Conversationalist    = (*Provider)(nil)
	_ execution.SessionProvider      = (*Provider)(nil)
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

		conversations:  make(map[string]*liveConversation),
		nativeSessions: make(map[string]*nativeSession),
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
	return execution.WorkflowCapabilities(), nil
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

// Execute dispatches any supported workflow action through the common batch
// runner. Native conversations use SessionProvider and keep their own lifecycle.
func (p *Provider) Execute(ctx context.Context, req execution.Request) (execution.Result, error) {
	cfg, dir, err := p.prepare(ctx, req)
	if err != nil {
		return execution.Result{}, err
	}
	req.WorkingDir = dir
	return localrun.ExecuteAction(ctx, req, localrun.ActionOptions{
		ProviderID: p.ID(), Command: cfg.Command, Model: cfg.Model, SkillsDir: ".claude/skills", Opening: localOpening,
		Timeout: cfg.Timeout, ShutdownGrace: shutdownGrace, Now: p.now,
	}, actionOpener{provider: p, cfg: cfg, dir: dir})
}

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
	return localrun.Shutdown(process, p.ID(), shutdownGrace)
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
