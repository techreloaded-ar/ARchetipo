package web

// A run of a provider that holds no native session is a conversation with a
// preconfigured prompt.
//
// This is the older of the two ways an action of the process is read, and it is
// now the exception rather than the rule. A provider that holds native sessions
// carries its actions out inside the conversation the person already has open —
// see session_action.go — where the session outlives the action, the record is
// an identity of its own, and a receipt closes the action without closing the
// thread. Nothing of that is available to a provider that cannot hold a
// session: its run is a dispatch with a beginning and an end, and the only
// thread it can offer is the run itself.
//
// So the old equivalences survive here, and only here, because for this kind of
// provider they are simply true: the execution *is* the conversation, held
// under the execution's own id, and the end of the dispatch *is* the end of the
// thread. What used to make them wrong was applying them to a session that was
// meant to outlive the work; that case has moved out of this file entirely.

import (
	"context"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// holdRunAsConversation makes the run just started the thread it is read in,
// and reports the id of that thread.
//
// It answers "" for every provider whose runs are not conversations, and that
// is an answer rather than a gap. A provider that cannot converse dispatches
// work it does not talk about: its run is followed through the run projection
// exactly as before, and holding it as a thread would offer a composer to an
// agent that has no turn to receive a message into. The same is true of a
// provider that exposes no interactive run at all.
//
// It never fails the start. By the time it runs the execution is persisted and
// running, and a thread that could not be held is a place to read the work
// from, not the work: refusing the answer that announces the run would leave
// the person with a run they were never told about.
func (s *Server) holdRunAsConversation(
	ctx context.Context,
	ws *workspaceSession,
	started *execution.Execution,
	provider execution.Provider,
	providerConfig map[string]any,
) string {
	if started == nil || strings.TrimSpace(started.ID) == "" || ws == nil {
		return ""
	}
	conversationalist, converses := execution.ConversationalistFor(provider)
	if !converses {
		return ""
	}
	collaborator, collaborates := execution.RunCollaboratorFor(provider)
	if !collaborates {
		return ""
	}
	hold := conversationHold{
		// The thread is held under the execution's own id, because the session
		// behind them is one session. Two ids for one process would be two names
		// for one thing, and every route would have to know which of them it had
		// been handed.
		id:             started.ID,
		providerID:     started.ProviderID,
		provider:       conversationalist,
		collaborator:   collaborator,
		providerConfig: providerConfig,
		workingDir:     ws.cfg.ProjectRoot,
		openedAt:       started.CreatedAt,
		specCode:       started.SpecCode,
		executionID:    started.ID,
		action:         started.Action,
	}
	if started.ModelChoice != nil {
		hold.model = started.ModelChoice.Model
		hold.modelOptions = cloneModelOptions(started.ModelChoice.Options)
	}
	if hold.openedAt.IsZero() {
		hold.openedAt = time.Now().UTC()
	}
	if err := ws.conversation.open(hold); err != nil {
		// The workspace has been left while the start was in flight. Nothing is
		// released here: the dispatch owns the process and will close its record
		// on the cancelled context, which is exactly what a shutdown does to
		// every run in flight.
		return ""
	}
	snapshot, held := ws.conversation.get(hold.id)
	if !held {
		return ""
	}
	// The journal is written for the same reason a free conversation's is: what
	// was said has to survive the process that said it. A failure is dropped
	// rather than reported — the run is running either way, and the caller has
	// no response left to put a notice in.
	_ = ws.journal.begin(ctx, snapshot, started.SpecCode, "")
	// The thread carries the name of the step it is doing. Nobody is going to
	// type a first message into it, so without a name the journal would fall
	// back to the date — which among several started steps says nothing about
	// which is which.
	_ = ws.journal.name(ctx, hold.id, s.actionThreadTitle(ws, started))
	return hold.id
}

// actionThreadTitle names the thread of an action: the process's own word for
// the step, and the spec it is about when there is one.
func (s *Server) actionThreadTitle(ws *workspaceSession, started *execution.Execution) string {
	label := s.actionLabelOf(ws, started.Action, started.SpecCode)
	if code := strings.TrimSpace(started.SpecCode); code != "" {
		return label + " " + code
	}
	return label
}

// sealRunConversation ends the thread of a run whose dispatch is over.
//
// It runs after the continuation and never before: the record is closed by
// then, so the transcript written here is the whole of what was said and the
// outcome the thread reports is final. The thread is dropped from the holder
// rather than left behind — a thread whose agent has gone is a composer nobody
// can send anything into — and its transcript stays readable from the journal,
// exactly as a free conversation's does once it is closed.
//
// Nothing is released: the process behind an action thread belongs to the
// dispatch, which has already ended it. Asking the holder to close it would
// send a cancel to a run that is over, and the refusal it earns would be the
// only thing that happened.
func (s *Server) sealRunConversation(ctx context.Context, ws *workspaceSession, executionID string) {
	if ws == nil || strings.TrimSpace(executionID) == "" {
		return
	}
	snapshot, held := ws.conversation.get(executionID)
	if !held || strings.TrimSpace(snapshot.executionID) == "" {
		return
	}
	// The outcome is written before the transcript is sealed, because sealing
	// drops the entry the outcome would have been written onto. It comes from
	// the record and from nowhere else: the dispatch is its author, and a
	// verdict composed here would be a second one.
	if record, err := ws.store.Get(ctx, executionID); err == nil {
		_ = ws.journal.settle(ctx, executionID, record.Status)
	}
	// Sealed while the holder still holds it: the final state is read from the
	// conversation that is about to be dropped, and everything said since the
	// last read is written down before the handle goes away.
	ws.sealConversation(ctx, snapshot)
	ws.conversation.forget(executionID)
}
