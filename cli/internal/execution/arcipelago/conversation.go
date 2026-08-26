package arcipelago

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// The assertion is what keeps the two methods from drifting apart from the
// interface, and it is also what makes this provider declare
// workspace.converse: DeclaredCapabilities derives that capability from this
// very interface, so the declaration and the implementation cannot disagree.
var _ execution.Conversationalist = (*Provider)(nil)

// conversationRetainedEvents bounds the history one conversation mirrors in
// memory, and is the same bound a locally held conversation carries: what the
// viewer may scroll back through must not depend on where the agent happens to
// run.
const conversationRetainedEvents = 2000

// conversationAssignmentGrace bounds the wait for the hub to put a runner
// behind the conversation task.
//
// It is deliberately not cfg.Timeout. That one bounds a whole planning run —
// an hour by default — and a person who pressed "open a conversation" is
// standing in front of the viewer: a minute of silence is already a failure to
// report, not a wait to sit through.
const conversationAssignmentGrace = 60 * time.Second

// liveConversation is one conversation this provider is holding open.
//
// The remote run is what every command is really addressed to, and the local
// session is the mirror the viewer reads: the hub streams events and never
// serves a transcript, while the viewer asks for "everything after id N" on
// every poll. Keeping the mirror is what turns the one into the other.
type liveConversation struct {
	remoteRunID string
	// providerConfig is kept because closing a conversation is a remote call
	// and CloseConversation is given nothing but an id: the credential and the
	// endpoint have to come from what opening the conversation already parsed.
	providerConfig map[string]any
	// reconnectAfter is how long a dropped stream waits before it is picked up
	// again, read once at open time so the follower never re-parses the
	// configuration on a loop.
	reconnectAfter time.Duration
	session        *localrun.Session
	cancel         context.CancelFunc
}

// Registry exposes the mirrored sessions, which is how the viewer reads the
// history of a conversation this provider holds.
//
// It is the same door a locally held conversation is read through, and that is
// the point: the viewer discovers it on the collaborator instead of asking
// whether the agent is near or far, so a remote conversation needs no second
// reading path — only a mirror to be read from.
func (p *Provider) Registry() *localrun.Registry {
	p.conversationsMu.Lock()
	defer p.conversationsMu.Unlock()
	if p.registry == nil {
		p.registry = localrun.NewRegistry()
	}
	return p.registry
}

// remoteRunOf translates an id the caller commands with into the id the hub
// knows.
//
// A conversation is followed and commanded under its own id — the viewer sends
// every RunRequest with it — while the hub only ever knew the run behind the
// task. An id this provider is not holding a conversation for is returned
// untouched, which is every ordinary run: the translation is a lookup, never a
// rewrite.
func (p *Provider) remoteRunOf(runID string) string {
	id := strings.TrimSpace(runID)
	if id == "" {
		return runID
	}
	p.conversationsMu.RLock()
	defer p.conversationsMu.RUnlock()
	if live, held := p.conversations[id]; held {
		return live.remoteRunID
	}
	return runID
}

// OpenConversation opens a conversation on the hub and returns without waiting
// for anything said in it.
//
// It borrows the whole machinery of a task — the same credential, the same
// external identity, the same run behind it — and borrows none of its record:
// nothing is written under .archetipo/executions, there is no action to satisfy
// and no receipt to accept. The task is the only way this hub starts an agent
// at all; what keeps the conversation from becoming an action of the process is
// one layer up, where no execution record is ever written for it.
//
// The remote task settles `completed` as soon as the agent finishes its opening
// turn, and that is not the end of anything: the runner reports the task and
// keeps the session, so the run stays active and goes on taking messages. The
// conversation ends when somebody closes it, exactly as a local one does.
func (p *Provider) OpenConversation(ctx context.Context, req execution.ConversationRequest) error {
	id := strings.TrimSpace(req.ConversationID)
	if id == "" {
		return fmt.Errorf("an arcipelago conversation cannot be opened without an id")
	}
	cfg, token, err := p.prepare(execution.RunRequest{ProviderConfig: req.ProviderConfig})
	if err != nil {
		return err
	}
	// The id is reserved before anything remote is created, under the same lock
	// that will hold the live conversation: two opens racing on one id would
	// otherwise both create a task, and only one of them would ever be closable.
	p.conversationsMu.Lock()
	if p.conversations == nil {
		p.conversations = map[string]*liveConversation{}
	}
	if _, exists := p.conversations[id]; exists {
		p.conversationsMu.Unlock()
		return fmt.Errorf("the arcipelago conversation %q is already open", id)
	}
	live := &liveConversation{}
	p.conversations[id] = live
	p.conversationsMu.Unlock()

	remoteRunID, err := p.startConversationTask(ctx, cfg, token, id, req)
	if err != nil {
		p.forgetConversation(id)
		return err
	}

	// The follower is deliberately detached from the context that opened the
	// conversation. That context belongs to the request that asked for it and
	// dies with its response, and a stream anchored to it would die the instant
	// the conversation was reported as open.
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	session := localrun.NewBoundedSession(id, p.now, conversationRetainedEvents)

	p.conversationsMu.Lock()
	live.remoteRunID = remoteRunID
	live.providerConfig = req.ProviderConfig
	live.reconnectAfter = cfg.PollInterval
	live.session = session
	live.cancel = cancel
	p.conversationsMu.Unlock()

	p.Registry().Register(session)
	go p.mirrorConversation(streamCtx, id, remoteRunID, req.ProviderConfig, cfg.PollInterval, session)
	return nil
}

// startConversationTask creates the remote task and answers with the run the
// hub put behind it.
func (p *Provider) startConversationTask(ctx context.Context, cfg settings, token, id string, req execution.ConversationRequest) (string, error) {
	title, prompt, metadata := buildConversationTask(id, req)
	body := createTaskRequest{
		WorkspaceID: cfg.WorkspaceID,
		Source:      sourceARchetipo,
		ExternalID:  id,
		Title:       title,
		Prompt:      prompt,
		Metadata:    metadata,
	}
	var envelope taskEnvelope
	status, err := p.do(ctx, cfg, token, http.MethodPost, pathTasks, body, &envelope)
	if err != nil {
		if status == http.StatusConflict {
			err = p.describeConflict(ctx, cfg, token, id, err)
		}
		return "", err
	}
	taskID := strings.TrimSpace(envelope.Task.ID)
	if taskID == "" {
		return "", fmt.Errorf("arcipelago answered the conversation creation with a task without an identifier, so the conversation cannot be followed")
	}
	return p.awaitConversationRun(ctx, cfg, token, taskID, envelope.Task)
}

// awaitConversationRun polls the task until the hub has assigned it a run.
//
// A task with no run yet is not a failure: the hub takes a moment to hand it to
// a runner. A task that reaches a terminal status without ever carrying one is,
// and it is reported as such rather than waited out — nothing is coming.
func (p *Provider) awaitConversationRun(ctx context.Context, cfg settings, token, taskID string, task remoteTask) (string, error) {
	deadline := p.now().Add(conversationAssignmentGrace)
	for {
		if runID := strings.TrimSpace(task.RunID); runID != "" {
			return runID, nil
		}
		switch task.Status {
		case statusFailed, statusCancelled:
			return "", fmt.Errorf("the arcipelago conversation task %s ended %s before a run was assigned to it%s", taskID, task.Status, summarySuffix(task.ResultSummary))
		}
		if !p.now().Before(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for arcipelago to assign a run to the conversation task %s, last observed status %q: the remote task exists and is not cancelled by this failure", conversationAssignmentGrace, taskID, task.Status)
		}
		if err := p.sleep(ctx, cfg.PollInterval); err != nil {
			return "", fmt.Errorf("waiting for the arcipelago conversation task %s was interrupted: %w", taskID, err)
		}
		var envelope taskEnvelope
		if _, err := p.do(ctx, cfg, token, http.MethodGet, pathTasks+"/"+url.PathEscape(taskID), nil, &envelope); err != nil {
			return "", err
		}
		task = envelope.Task
	}
}

// mirrorConversation copies the remote event stream into the local session for
// as long as the conversation lives.
//
// A dropped stream is reconnected from the last event mirrored and never from
// the start: the hub replays from an id, so reconnecting from zero would
// duplicate the whole history into the mirror. It gives up only when the
// conversation is closed or the run is over, and it says so in the session
// rather than in a log nobody attached to this process would ever read.
func (p *Provider) mirrorConversation(ctx context.Context, id, remoteRunID string, providerConfig map[string]any, reconnectAfter time.Duration, session *localrun.Session) {
	req := execution.RunRequest{RunID: remoteRunID, ProviderConfig: providerConfig}
	var lastID int64
	for {
		err := p.StreamRunEvents(ctx, req, lastID, func(event execution.RunEvent) error {
			mirrored := session.Append(event)
			lastID = mirrored.ID
			return nil
		})
		if ctx.Err() != nil {
			// The conversation was closed. CloseConversation owns the final
			// state of the session, so nothing is written here.
			return
		}
		if err == nil {
			// The stream ended on its own: the remote run is over, and so is the
			// conversation, whoever ends up reading it.
			session.Close(execution.RunClosed, "the remote run behind this conversation ended")
			p.forgetConversation(id)
			return
		}
		var refused *execution.RunCommandError
		if errors.As(err, &refused) && refused.Reason != execution.RunRefusedRunnerOffline {
			session.Close(execution.RunCrashed, err.Error())
			p.forgetConversation(id)
			return
		}
		// Anything else is transient — a dropped socket, a runner coming back.
		// Waiting before reconnecting is what keeps a hub that is down from
		// being hammered by a follower that never sleeps.
		if sleepErr := p.sleep(ctx, reconnectAfter); sleepErr != nil {
			return
		}
	}
}

// CloseConversation releases the remote run behind a conversation and closes
// its mirror. It is idempotent: an id this provider is not holding — never
// opened, or already closed — is answered with nil and nothing is released.
func (p *Provider) CloseConversation(ctx context.Context, conversationID string) error {
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return nil
	}
	p.conversationsMu.Lock()
	live, held := p.conversations[id]
	delete(p.conversations, id)
	p.conversationsMu.Unlock()
	if !held {
		return nil
	}
	// The mirror stops first. Cancelling afterwards would let the follower see
	// the cancelled run as a crash and write that over the state below.
	if live.cancel != nil {
		live.cancel()
	}
	if live.session != nil {
		live.session.Close(execution.RunClosed, "closed by the operator")
	}
	if live.remoteRunID == "" {
		return nil
	}
	// A run the hub no longer considers active is not a failure to close: the
	// conversation is over either way, and reporting it would leave the viewer
	// holding a conversation it has already let go of.
	err := p.CancelRun(ctx, execution.RunRequest{RunID: live.remoteRunID, ProviderConfig: live.providerConfig})
	var refused *execution.RunCommandError
	if errors.As(err, &refused) {
		return nil
	}
	return err
}

// forgetConversation gives an id back, without touching the mirror: a session
// already registered stays readable, because the history of a conversation
// outlives the provider's hold on it.
func (p *Provider) forgetConversation(id string) {
	p.conversationsMu.Lock()
	defer p.conversationsMu.Unlock()
	delete(p.conversations, id)
}
