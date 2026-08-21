package claude

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// conversationRetainedEvents bounds the history a conversation keeps in memory.
//
// A dispatched action ends, and its whole history is worth keeping until it
// does. A conversation does not: it stays open for as long as somebody keeps it
// open, so an unlimited history would grow with the time the workspace stayed
// open rather than with any unit of work. The window is what lets the
// conversation say "this story begins here" instead of pretending it holds the
// whole of it — the reason is written out on localrun.NewBoundedSession.
const conversationRetainedEvents = 2000

// liveConversation is one conversation this provider is still keeping alive:
// the way to end its context, the process to release, and the session its
// history is written into.
//
// release exists because two paths can end the same conversation — the operator
// closing it and the process leaving on its own — and both have to release the
// process. Waiting for a process twice is not a thing that can be done, so the
// once is what makes the second path observe the outcome of the first instead
// of asking the operating system a question it has already answered.
type liveConversation struct {
	cancel  context.CancelFunc
	process localrun.Process
	session *localrun.Session

	once     sync.Once
	exitCode int
	stderr   string
	waitErr  error
}

// OpenConversation starts a Claude process on the requested directory, makes it
// followable under the conversation id and returns without waiting for
// anything.
//
// It borrows the whole machinery of a run — the same preparation, the same
// session, the same protocol client, the same registry — and borrows none of
// its record: nothing is written under .archetipo/executions, there is no
// action to satisfy and no receipt to accept. That is what keeps a conversation
// from ever appearing as an execution of the process.
//
// No skill is checked and none is invoked: a free conversation asks the agent
// to read and to answer, and requiring an installed skill for that would refuse
// a conversation a workspace can perfectly well hold.
func (p *Provider) OpenConversation(ctx context.Context, req execution.ConversationRequest) error {
	id := strings.TrimSpace(req.ConversationID)
	if id == "" {
		return fmt.Errorf("a claude conversation cannot be opened without an id")
	}
	// The preparation is the shared one, so a conversation fails on a broken
	// configuration or an unusable runtime with exactly the diagnostics every
	// dispatched action fails with.
	cfg, dir, err := p.prepare(ctx, execution.Request{WorkingDir: req.WorkingDir, ProviderConfig: req.ProviderConfig})
	if err != nil {
		return err
	}
	// The id is reserved before anything is started, under the same lock that
	// will hold the live conversation: two opens racing on one id would
	// otherwise both start a process, and only one of them would ever be
	// closable.
	live := &liveConversation{}
	p.conversationsMu.Lock()
	if _, exists := p.conversations[id]; exists {
		p.conversationsMu.Unlock()
		return fmt.Errorf("the claude conversation %q is already open", id)
	}
	p.conversations[id] = live
	p.conversationsMu.Unlock()

	// The conversation is deliberately detached from the context that opened
	// it. That context belongs to the request that asked for the conversation
	// and dies with its response, and a conversation anchored to it would die
	// the instant it was reported as open. What bounds it instead is the
	// provider's own timeout, and the operator who closes it.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeout)

	session, err := p.openSession(runCtx, execution.Request{
		ExecutionID:    id,
		WorkingDir:     dir,
		ProviderConfig: req.ProviderConfig,
	}, cfg, dir, buildConversationPrompt(req.ProcessActions), true, conversationRetainedEvents)
	if err != nil {
		// openSession already closed the session it had registered and released
		// the process, so all that is left is to give back the id: a failed open
		// must never leave a conversation registered as open, and never an ACTIVE
		// run that nobody will ever close.
		cancel()
		p.forgetConversation(id)
		return err
	}

	live.cancel = cancel
	live.process = session.process
	live.session = session.session

	go p.watchConversation(id, live, runCtx, session)
	return nil
}

// CloseConversation releases the process behind a conversation and closes its
// session. It is idempotent: an id this provider is not keeping alive — never
// opened, or already closed — is answered with nil and nothing is released.
//
// Removing the conversation from *this* map does not remove its session from
// the localrun registry, and that is on purpose: the history of a conversation
// that has ended stays readable, and a message sent to it is refused with the
// reason that is true — this run is no longer active — instead of the reason
// that would merely be convenient. It is the rule already written on
// localrun.Registry.
func (p *Provider) CloseConversation(_ context.Context, conversationID string) error {
	id := strings.TrimSpace(conversationID)
	live, ok := p.forgetConversation(id)
	if !ok {
		return nil
	}
	// Release first, cancel after — the order watchConversation already uses.
	// The process comes from exec.CommandContext, so cancelling is an immediate
	// SIGKILL: doing it first would make the polite shutdown of release (closing
	// stdin, then waiting out shutdownGrace) dead code on this path. Cancel is
	// the safety net that guarantees the process really ends, not the first move.
	if live.process != nil {
		p.release(live)
	}
	if live.cancel != nil {
		live.cancel()
	}
	if live.session != nil {
		live.session.Close(execution.RunClosed, "")
	}
	return nil
}

// watchConversation follows a conversation until it ends and records how it
// ended.
//
// The state it writes is observed and never deduced, which is the invariant of
// the whole localrun package: it waits for the output of the process to end or
// for the context to be over, releases the process, and only then closes the
// session. A conversation still registered at that moment ended without anybody
// closing it — the process left, or the timeout fired — and that is a crash
// with a reason. One already gone from the map was closed by the operator, who
// has already recorded the ordinary end.
func (p *Provider) watchConversation(id string, live *liveConversation, runCtx context.Context, session *liveSession) {
	defer live.cancel()
	select {
	case <-session.client.Gone():
	case <-runCtx.Done():
	}
	p.release(live)

	if _, stillOpen := p.forgetConversation(id); !stillOpen {
		live.session.Close(execution.RunClosed, "")
		return
	}
	live.session.Close(execution.RunCrashed, p.conversationEndReason(live, runCtx.Err()))
}

// conversationEndReason names why a conversation ended on its own. It is never
// empty: a run reported as crashed without a reason is a run nobody can act on.
func (p *Provider) conversationEndReason(live *liveConversation, runErr error) string {
	switch {
	case runErr != nil:
		return fmt.Sprintf("the claude conversation was stopped: %v", runErr)
	case live.waitErr != nil:
		return fmt.Sprintf("the claude conversation process could not be run to completion: %v", live.waitErr)
	default:
		return fmt.Sprintf("the claude conversation process exited %d%s", live.exitCode, diagnosticSuffix(live.stderr))
	}
}

// release ends the process of a conversation exactly once and keeps what it
// reported, so the path that did not win the race reads the outcome instead of
// waiting for a process that has already been waited for.
func (p *Provider) release(live *liveConversation) {
	live.once.Do(func() {
		live.exitCode, live.stderr, live.waitErr = p.shutdown(live.process)
	})
}

// forgetConversation takes a conversation out of the live map and reports
// whether it was still there, which is how the two paths that can end one tell
// apart who got there first.
func (p *Provider) forgetConversation(id string) (*liveConversation, bool) {
	p.conversationsMu.Lock()
	defer p.conversationsMu.Unlock()
	live, ok := p.conversations[id]
	if ok {
		delete(p.conversations, id)
	}
	return live, ok
}
