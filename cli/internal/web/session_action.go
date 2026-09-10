package web

// An ARchetipo action is a unit of work carried out *inside* a conversation,
// not a second agent started beside it.
//
// This file is where that becomes true. The equivalences the viewer used to
// live by — an execution *is* a conversation, a receipt *closes* the thread —
// are gone from this path: the session belongs to the conversation and outlives
// every action run in it, while the execution record is an identity of its own,
// opened before the work starts and closed exactly once when it ends. One
// conversation therefore holds many executions, one after another, with free
// messages in between; and an execution may span several turns, because an
// agent that needs an answer asks for it and the person answers in the thread
// they are already reading.
//
// What the record still owes the workspace is unchanged and deliberately so:
// the Template decides whether the action is admissible, the connector confirms
// that the effects claimed by the receipt really happened, and a claim the
// workspace does not back closes the record as FAILED. None of that is applied
// to a free message or to a native skill the model chose by itself — those are
// the session's business and produce no record at all.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// sessionActionOpening is the first line of every action prompt sent into a
// native session. It differs from the batch opening in the one way the two
// situations differ: the session is already open, it stays open after the
// action, and the person is reading it while the agent works.
const sessionActionOpening = "Work in the current working directory: it is the ARchetipo workspace, with the archetipo CLI and the ARchetipo skills already installed. You are in a conversation a person is reading: ask them when you need something only they can decide, and keep the session open after you are done — the receipt closes the action, not this conversation."

// actionInSession is everything starting one ARchetipo action inside a native
// session needs to know, once the caller has already established that the
// action is admissible and that the provider can run it.
//
// It is a struct rather than a list of arguments because the two callers — the
// spec board and the workspace panel — differ only in which of these fields
// they fill, and a positional call with eight values gives a reader no way to
// see which kind of action is being started.
type actionInSession struct {
	// conversationID is the thread the action was asked for in, empty when it
	// was pressed somewhere that belongs to no thread. An empty one is not a
	// gap: the action opens its own conversation, once, and runs in it.
	conversationID string
	action         execution.ActionID
	// specCode is empty for a workspace-scoped action, which is exactly what
	// Service.Open expects of one, and spec is the spec it names as the caller
	// read it *before* the action's own effect moved it. The record stamps the
	// status the spec had when the action was accepted, so re-reading it here
	// would stamp the status the action itself has just produced.
	specCode       string
	spec           domain.Spec
	providerID     string
	providerConfig map[string]any
	modelChoice    *execution.ModelChoice
	// model and modelOptions are the override asked for this one action, empty
	// when none was. They are kept apart from modelChoice — which is filled in
	// even when nobody chose anything — because only an explicit choice may
	// rewrite what the conversation had decided to run its next turn on.
	model        string
	modelOptions map[string]string
	// confirm is the caller's verdict on a success, applied inside the single
	// terminal write. It is registered rather than only called, because the turn
	// that ends the action may be observed by a later process than the one that
	// started it — see registerActionConfirmation.
	confirm execution.Confirmation
}

// startActionInSession opens the execution record of an action and hands the
// work to the conversation's own native session.
//
// The order is what makes a refusal leave nothing behind: the thread is
// resolved first, then the record is created, then the turn is started. A turn
// that cannot be started closes the record it just opened, so an action that
// never reached the agent is a FAILED record and never a RUNNING one nobody
// will close.
func (s *Server) startActionInSession(ctx context.Context, ws *workspaceSession, plan actionInSession) (*execution.Execution, error) {
	snapshot, err := s.conversationForAction(ctx, ws, plan)
	if err != nil {
		return nil, err
	}
	if running, found, err := s.actionAlreadyRunningIn(ctx, ws, snapshot.id); err != nil {
		return nil, err
	} else if found {
		return nil, iox.NewConflict(
			"the conversation "+snapshot.id+" is already carrying out the execution "+running,
			"wait for that action to end, or interrupt it, before starting another one in this thread",
			nil,
		)
	}
	opts := []execution.StartOption(nil)
	if plan.modelChoice != nil {
		opts = append(opts, execution.WithModelChoice(*plan.modelChoice))
	}
	opened, err := ws.service.Open(ctx, plan.spec, plan.action, plan.providerID, execution.CloneConfig(plan.providerConfig), opts...)
	if err != nil {
		var configErr *execution.ConfigurationError
		if errors.As(err, &configErr) {
			return nil, err
		}
		return nil, mapExecutionStartError(err, plan.providerID)
	}
	ws.registerActionConfirmation(opened.ID, plan.confirm)
	// An action started with an explicit model choice runs on it, so the choice
	// made on the board is the one the turn uses. Without one the conversation
	// keeps whatever it was going to run its next turn on.
	if strings.TrimSpace(plan.model) != "" || len(plan.modelOptions) > 0 {
		if err := ws.chooseNextTurn(ctx, snapshot.id, plan.model, plan.modelOptions); err != nil {
			ws.settleSessionAction(ctx, opened.ID, nil, err)
			return nil, err
		}
	}
	prompt, err := execution.ActionPrompt(sessionActionOpening, plan.action, execution.Request{
		ExecutionID: opened.ID, SpecCode: plan.specCode, Action: plan.action, WorkingDir: opened.WorkingDir,
	})
	if err != nil {
		ws.settleSessionAction(ctx, opened.ID, nil, err)
		return nil, iox.NewConflict(err.Error(), "run this action from a coding agent instead", err)
	}
	if err := s.startActionTurn(ctx, ws, snapshot, opened.ID, prompt); err != nil {
		ws.settleSessionAction(ctx, opened.ID, nil, err)
		return nil, err
	}
	// The link is written on the conversation as well as on the record, because
	// the two answer different questions: the record says what the step did, the
	// conversation says which of its turns were doing it.
	_ = ws.linkExecutionToConversation(ctx, snapshot.id, opened.ID)
	started, storeErr := ws.store.Get(ctx, opened.ID)
	if storeErr != nil {
		return &opened, nil
	}
	return &started, nil
}

// conversationForAction resolves the thread the action runs in, opening one
// when the gesture came from somewhere that has none.
//
// A start that names no conversation is not a start with nowhere to run: it
// opens its own thread, once, and the action then runs in that thread exactly
// as it would in one the person already had open. What it never does is open a
// second one beside a thread that was named.
func (s *Server) conversationForAction(ctx context.Context, ws *workspaceSession, plan actionInSession) (conversationSnapshot, error) {
	if id := strings.TrimSpace(plan.conversationID); id != "" {
		snapshot, live := ws.conversation.get(id)
		if !live {
			return conversationSnapshot{}, s.conversationGoneRefusal(ctx, ws, id, "start an action")
		}
		if snapshot.sessionProvider == nil {
			return conversationSnapshot{}, iox.NewConflict(
				"the conversation "+id+" does not hold a native session",
				"open a new conversation with a provider that holds native sessions, then start the action there",
				nil,
			)
		}
		return snapshot, nil
	}
	// The session is opened on the model the action was started with, not on the
	// workspace default: an action that overrode the model must not first open a
	// process on the model it overrode.
	opened, err := s.openConversationOn(ctx, ws, conversationOpenSpec{
		specCode: plan.specCode, model: plan.model, modelOptions: plan.modelOptions,
	})
	if err != nil {
		return conversationSnapshot{}, err
	}
	if opened.snapshot.sessionProvider == nil {
		return conversationSnapshot{}, iox.NewConflict(
			"the conversation opened for this action does not hold a native session",
			"configure a provider that holds native sessions in the Execution panel",
			nil,
		)
	}
	return opened.snapshot, nil
}

// startActionTurn sends the action prompt into the session as a turn that names
// the execution it is carrying out.
//
// It is the ordinary send of the conversation, with one field added: the turn
// carries the execution id, so every later reader — the reconciliation, a
// restart, a second tab — can tell which turns of this thread belong to which
// action without holding anything in memory.
func (s *Server) startActionTurn(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot, executionID, prompt string) error {
	return s.sendNativeConversationMessage(ctx, ws, snapshot, sendConversationMessageReq{
		Message: prompt, executionID: executionID,
	})
}

// actionAlreadyRunningIn reports the execution a conversation is already
// carrying out, if any.
//
// It is what makes a double press one action instead of two. The check is on
// the durable record rather than on a reservation held in memory, so it also
// answers correctly for a thread whose action was started before the viewer was
// restarted.
func (s *Server) actionAlreadyRunningIn(ctx context.Context, ws *workspaceSession, conversationID string) (string, bool, error) {
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return "", false, iox.NewInternal("reading the conversation "+conversationID, err)
	}
	for _, id := range record.ExecutionIDs {
		stored, storeErr := ws.store.Get(ctx, id)
		if storeErr != nil {
			continue
		}
		if stored.Status == execution.StatusRunning {
			return id, true, nil
		}
	}
	return "", false, nil
}

// linkExecutionToConversation records on the conversation that one of its turns
// is carrying out this execution.
func (ws *workspaceSession) linkExecutionToConversation(ctx context.Context, conversationID, executionID string) error {
	lock, err := ws.conversationStore().Lock(ctx, conversationID)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return err
	}
	for _, id := range record.ExecutionIDs {
		if id == executionID {
			return nil
		}
	}
	record.ExecutionIDs = append(record.ExecutionIDs, executionID)
	return ws.conversationStore().Save(ctx, record)
}

// registerActionConfirmation keeps the verdict an action still owes its record.
//
// It is held in memory and only in memory, on purpose. The verdict of a
// workspace action closes over a snapshot taken *before* the action started —
// whether the workspace already had a PRD, which spec codes the backlog already
// held — and a snapshot read again later would describe what the action itself
// did. A restart therefore loses it rather than recomputing it wrongly, and
// settleSessionAction says so on the record it closes.
func (ws *workspaceSession) registerActionConfirmation(executionID string, confirm execution.Confirmation) {
	if confirm == nil {
		return
	}
	ws.actionConfirmations.Store(executionID, confirm)
}

// pendingActionExecutionID is the execution a conversation is carrying out
// right now, so an ordinary message typed into the thread while an action is in
// flight becomes another turn *of that action* rather than an unrelated one.
//
// It is what makes an action span several turns: the agent asks, the person
// answers, and the answer stays attached to the work it is an answer to.
func pendingActionExecutionID(ctx context.Context, ws *workspaceSession, record conversationlog.Record) string {
	for index := len(record.ExecutionIDs) - 1; index >= 0; index-- {
		stored, err := ws.store.Get(ctx, record.ExecutionIDs[index])
		if err != nil {
			continue
		}
		if stored.Status == execution.StatusRunning {
			return stored.ID
		}
	}
	return ""
}

// settleSessionActions closes every action of a conversation whose work is over,
// reading what happened from the durable record and from nothing else.
//
// It is called after every batch of events the follower appends, and again when
// a conversation is read, so the close does not depend on the process that
// started the action still being alive. It is idempotent by construction:
// Service.Settle refuses a record that is no longer RUNNING, so a second
// observation of the same ending changes nothing.
func (ws *workspaceSession) settleSessionActions(ctx context.Context, conversationID string) {
	if ws == nil || ws.service == nil {
		return
	}
	// Detached from the caller's cancellation on purpose: this is the only
	// place an action's record is closed, and a shutdown racing the end of a
	// turn must not leave the record RUNNING with nobody left to close it.
	ctx = context.WithoutCancel(ctx)
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return
	}
	for _, executionID := range record.ExecutionIDs {
		stored, storeErr := ws.store.Get(ctx, executionID)
		if storeErr != nil || stored.Status != execution.StatusRunning {
			continue
		}
		turn, found := lastTurnOfExecution(record, executionID)
		if !found {
			continue
		}
		if !terminalTurn(turn.State) {
			if !ws.turnEnded(ctx, conversationID, turn.ID) {
				// No turn of this action has ended yet: the agent is still
				// working, and a record closed now would be a verdict on work in
				// progress.
				continue
			}
			// The timeline says the turn ended and the record has not caught up:
			// how it ended is asked of the session, read and not written, so a
			// stale observation can never overwrite a newer state.
			turn = ws.observedTurn(ctx, conversationID, turn)
		}
		switch turn.State {
		case execution.TurnInterrupted:
			ws.settleSessionAction(ctx, executionID, nil, fmt.Errorf("the action was interrupted in the conversation %s", conversationID))
			continue
		case execution.TurnFailed:
			reason := strings.TrimSpace(turn.Error)
			if reason == "" {
				reason = "the turn carrying this action failed"
			}
			ws.settleSessionAction(ctx, executionID, nil, fmt.Errorf("%s", reason))
			continue
		}
		message := ws.finalMessageOfTurn(ctx, conversationID, turn.ID)
		receipt, emitted, acceptErr := execution.AcceptActionReceipt(stored.Action, stored.SpecCode, message)
		if !emitted {
			// A turn that ended without a receipt has not failed: the agent
			// said something and is waiting for an answer. The action goes on
			// into the next turn of the same thread.
			continue
		}
		if acceptErr != nil {
			ws.settleSessionAction(ctx, executionID, nil, acceptErr)
			continue
		}
		payload, marshalErr := json.Marshal(struct {
			ConversationID string          `json:"conversation_id"`
			TurnID         string          `json:"turn_id"`
			Receipt        json.RawMessage `json:"receipt"`
		}{ConversationID: conversationID, TurnID: turn.ID, Receipt: receipt})
		if marshalErr != nil {
			ws.settleSessionAction(ctx, executionID, nil, marshalErr)
			continue
		}
		ws.settleSessionAction(ctx, executionID, &execution.Result{Payload: payload}, nil)
	}
}

// settleSessionAction writes the single terminal state of one action and tells
// the board to re-read.
//
// The verdict is the one the caller registered when it started the action, so
// the rule that decides what a success is stays the caller's — the connector is
// re-read, and a claim it does not back closes the record as FAILED. When that
// verdict is gone, because the viewer was restarted while the action was in
// flight, the effect is still verified and the record says what could not be
// done, instead of silently closing a success that nothing checked.
func (ws *workspaceSession) settleSessionAction(ctx context.Context, executionID string, result *execution.Result, cause error) {
	confirm := ws.actionConfirmationFor(executionID)
	settled, closed, err := ws.service.Settle(context.WithoutCancel(ctx), executionID, result, cause, confirm)
	if err != nil {
		return
	}
	ws.actionConfirmations.Delete(executionID)
	if !closed {
		return
	}
	_ = settled
	if ws.broker != nil {
		ws.broker.Publish()
	}
}

// actionConfirmationFor is the verdict registered for this action, or the
// verdict that can still be reconstructed without the snapshot a restart threw
// away: the effects claimed by the receipt are verified against the connector,
// and a workspace action that failed says on its record that the rollback of
// whatever it had half written was not applied.
func (ws *workspaceSession) actionConfirmationFor(executionID string) execution.Confirmation {
	if held, ok := ws.actionConfirmations.Load(executionID); ok {
		if confirm, isConfirmation := held.(execution.Confirmation); isConfirmation {
			return confirm
		}
	}
	return func(confirmCtx context.Context, outcome *execution.Execution) {
		key := outcome.SpecCode
		if strings.TrimSpace(key) == "" {
			key = workspaceExecutionKey
		}
		_ = execution.VerifyActionEffect(confirmCtx, ws.conn, outcome.Action, key, outcome)
		if outcome.Status != execution.StatusFailed || outcome.Error == nil {
			return
		}
		switch outcome.Action {
		case execution.ActionInception, execution.ActionBacklog, execution.ActionSpecDraft:
			outcome.Error.Message += "; whatever this action had already written was not taken back, because the viewer was restarted while it was running"
		}
	}
}

// lastTurnOfExecution is the most recent turn of a conversation that was
// carrying out this execution.
//
// Only turns that name the execution are considered, which is what keeps a
// receipt from an earlier action — or one quoted back inside an ordinary
// message — from ever closing this one. The current turn is looked at last
// because it is the newest, and it may not have been folded into the observed
// list yet.
func lastTurnOfExecution(record conversationlog.Record, executionID string) (execution.SessionTurn, bool) {
	found := false
	var latest execution.SessionTurn
	for _, turn := range record.Turns {
		if turn.ExecutionID == executionID {
			latest, found = turn, true
		}
	}
	if record.CurrentTurn != nil && record.CurrentTurn.ExecutionID == executionID {
		latest, found = *record.CurrentTurn, true
	}
	return latest, found
}

// finalMessageOfTurn is the message a turn ended on: the last thing the agent
// said in it, and the only place a receipt is looked for.
//
// Tool output and lifecycle events are not messages and are skipped, exactly as
// they are when the exchange is counted. Reading the whole timeline is what the
// journal already does on every append, and the alternative — remembering the
// tail in memory — would not survive the restart this reconciliation exists to
// survive.
func (ws *workspaceSession) finalMessageOfTurn(ctx context.Context, conversationID, turnID string) string {
	page, err := ws.conversationStore().ReadEvents(ctx, conversationID, 0, 0)
	if err != nil {
		return ""
	}
	message := ""
	for _, event := range page.Events {
		if event.TurnID != turnID {
			continue
		}
		if event.Kind == localrun.KindText {
			message = event.Text
		}
	}
	return message
}

// cancelSessionAction closes as failed the action a conversation is carrying
// out, when the person asks for it to stop and there is no active turn left to
// interrupt.
//
// It is the other half of the stop gesture. Interrupting an active turn already
// ends the action, because the turn it was running in becomes INTERRUPTED; an
// action waiting for an answer that will never come has no turn to interrupt,
// and without this it would stay RUNNING for ever.
func (ws *workspaceSession) cancelSessionAction(ctx context.Context, conversationID string) bool {
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return false
	}
	executionID := pendingActionExecutionID(ctx, ws, record)
	if executionID == "" {
		return false
	}
	ws.settleSessionAction(ctx, executionID, nil, fmt.Errorf("the action was cancelled in the conversation %s", conversationID))
	return true
}

// nativeConversationRuns are the actions of the process this conversation has
// carried out or is carrying out, drawn at the point of the thread that started
// each of them.
//
// It is the register of a native conversation, and it is read from the durable
// record rather than from the decisions the holder remembers: an action started
// before a restart is still one of this thread's actions, and the person
// reading the thread has to see how it ended.
func (s *Server) nativeConversationRuns(ctx context.Context, ws *workspaceSession, record conversationlog.Record) []conversationRunView {
	views := make([]conversationRunView, 0, len(record.ExecutionIDs))
	if len(record.ExecutionIDs) == 0 {
		return views
	}
	page, _ := ws.conversationStore().ReadEvents(ctx, record.ID, 0, 0)
	for _, executionID := range record.ExecutionIDs {
		view := conversationRunView{
			runView:       emptyRunView(""),
			ExecutionID:   executionID,
			AnchorEventID: actionAnchorOf(page.Events, record, executionID),
			InThisThread:  true,
			Decision:      conversationDecisionConfirmed,
		}
		stored, err := ws.store.Get(ctx, executionID)
		if err != nil {
			view.Notice = err.Error()
			views = append(views, view)
			continue
		}
		view.Action = stored.Action
		view.Label = s.actionLabelOf(ws, stored.Action, stored.SpecCode)
		view.Scope = runScopeOf(stored)
		view.SpecCode = stored.SpecCode
		view.Status = stored.Status
		view.CreatedAt = stored.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		views = append(views, view)
	}
	return views
}

// actionAnchorOf is the point of the thread an action was asked for at: the
// last event said before its first turn spoke.
//
// Zero means "before anything", which is where the flow already draws a block
// whose anchor precedes every event — the honest answer for an action whose
// turn has not produced an event yet.
func actionAnchorOf(events []execution.RunEvent, record conversationlog.Record, executionID string) int64 {
	turns := map[string]struct{}{}
	for _, turn := range record.Turns {
		if turn.ExecutionID == executionID {
			turns[turn.ID] = struct{}{}
		}
	}
	if record.CurrentTurn != nil && record.CurrentTurn.ExecutionID == executionID {
		turns[record.CurrentTurn.ID] = struct{}{}
	}
	previous := int64(0)
	for _, event := range events {
		if _, belongs := turns[event.TurnID]; belongs {
			return previous
		}
		previous = event.ID
	}
	return previous
}

// sessionActionRunView is what the run routes answer for an action carried out
// inside a native session: the turns of that action, read from the one durable
// timeline the conversation already owns.
//
// It is a projection and not a second history. There is exactly one journal per
// conversation, and this filters it down to the turns that name the execution —
// so the board reader sees the action, the thread reader sees the whole
// conversation, and neither is a copy of the other. What the block inside the
// thread never carries is this same log, which is what "the work is drawn once"
// means; see conversationRunView.InThisThread.
//
// The run identity is the conversation's, because that is where the work really
// is: a caller that wants to say something to the action is sent to the thread,
// and ThreadID is how it learns which one.
func (s *Server) sessionActionRunView(ctx context.Context, ws *workspaceSession, conversationID, executionID string, afterID int64) (runView, bool) {
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return runView{}, false
	}
	turns := map[string]struct{}{}
	for _, turn := range record.Turns {
		if turn.ExecutionID == executionID {
			turns[turn.ID] = struct{}{}
		}
	}
	if record.CurrentTurn != nil && record.CurrentTurn.ExecutionID == executionID {
		turns[record.CurrentTurn.ID] = struct{}{}
	}
	page, pageErr := ws.conversationStore().ReadEvents(ctx, conversationID, afterID, 0)
	view := emptyRunView("")
	if pageErr != nil {
		view.Notice = "the durable timeline could not be read: " + pageErr.Error()
	}
	for _, event := range page.Events {
		if _, belongs := turns[event.TurnID]; belongs {
			view.Events = append(view.Events, event)
		}
	}
	view.LastID = page.LastID
	view.ThreadID = conversationID
	view.Connected = record.Connection == execution.ConnectionConnected
	state := execution.RunActive
	if stored, storeErr := ws.store.Get(ctx, executionID); storeErr == nil && stored.Status != execution.StatusRunning {
		state = execution.RunClosed
	}
	view.Run = &runSnapshotView{RunID: conversationID, State: state}
	return view, true
}

// sendActionMessage delivers into the conversation a message addressed to the
// action it is carrying out, so an answer given from the board reaches the very
// turn the agent is waiting in.
func (s *Server) sendActionMessage(ctx context.Context, ws *workspaceSession, conversationID, executionID, message string) error {
	snapshot, live := ws.conversation.get(conversationID)
	if !live || snapshot.sessionProvider == nil {
		return s.conversationGoneRefusal(ctx, ws, conversationID, "answer this action")
	}
	return s.sendNativeConversationMessage(ctx, ws, snapshot, sendConversationMessageReq{
		Message: message, executionID: executionID,
	})
}

// cancelActionInSession stops an action without stopping the session it runs
// in: an active turn is interrupted, and an action that was only waiting for an
// answer is closed where it stands.
func (s *Server) cancelActionInSession(ctx context.Context, ws *workspaceSession, conversationID, executionID string) {
	snapshot, live := ws.conversation.get(conversationID)
	if live && snapshot.sessionProvider != nil {
		if record, err := ws.conversationStore().GetMetadata(ctx, conversationID); err == nil &&
			record.CurrentTurn != nil && record.CurrentTurn.ExecutionID == executionID && !terminalTurn(record.CurrentTurn.State) {
			submissionID, idErr := nativeOperationID("submission-")
			if idErr == nil {
				_, _ = snapshot.sessionProvider.InterruptTurn(ctx, execution.SessionCommandRequest{
					Session: snapshot.session, TurnID: record.CurrentTurn.ID, SubmissionID: submissionID,
				})
			}
		}
	}
	ws.settleSessionAction(ctx, executionID, nil, fmt.Errorf("the action was cancelled in the conversation %s", conversationID))
}

// chooseNextTurn writes on the conversation the model the next turn is to run
// on. It is the same durable field the composer writes, so an action started
// with a choice and a turn typed after choosing one go through one rule.
func (ws *workspaceSession) chooseNextTurn(ctx context.Context, conversationID, model string, options map[string]string) error {
	lock, err := ws.conversationStore().Lock(ctx, conversationID)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return err
	}
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		record.NextTurn.Model = trimmed
	}
	if len(options) > 0 {
		record.NextTurn.Options = cloneModelOptions(options)
	}
	return ws.conversationStore().Save(ctx, record)
}

// turnEnded reports whether the timeline already carries the end of this turn.
//
// It is asked because the two facts arrive separately: the provider appends the
// last event of a turn and marks the turn terminal a moment later, and between
// the two the follower is parked waiting for an event that will never come. The
// journal is the durable side of that pair and needs no polling — it is written
// once, it survives a restart, and it cannot be undone by a stale observation
// of the session racing a newer one.
func (ws *workspaceSession) turnEnded(ctx context.Context, conversationID, turnID string) bool {
	page, err := ws.conversationStore().ReadEvents(ctx, conversationID, 0, 0)
	if err != nil {
		return false
	}
	for _, event := range page.Events {
		if event.TurnID == turnID && event.Kind == localrun.KindTurnEnd {
			return true
		}
	}
	return false
}

// observedTurn asks the session how a turn ended, when the durable record still
// says it is running.
//
// The two facts arrive separately — the provider appends the last event of a
// turn and marks the turn terminal a moment later — so the timeline can be
// ahead of the record. Reading the session settles it. Nothing is written: a
// snapshot read here and persisted would be free to overwrite a newer turn that
// has started in the meantime.
//
// A session that cannot be asked leaves the turn as it was, which the caller
// then treats as a turn that ended without saying how: it looks for a receipt
// and, finding none, leaves the action running.
func (ws *workspaceSession) observedTurn(ctx context.Context, conversationID string, turn execution.SessionTurn) execution.SessionTurn {
	snapshot, live := ws.conversation.get(conversationID)
	if !live || snapshot.sessionProvider == nil {
		return turn
	}
	observed, err := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session})
	if err != nil || observed.CurrentTurn == nil || observed.CurrentTurn.ID != turn.ID {
		return turn
	}
	if !terminalTurn(observed.CurrentTurn.State) {
		return turn
	}
	settled := *observed.CurrentTurn
	settled.ExecutionID = turn.ExecutionID
	return settled
}

// hasRunningAction reports whether the conversation still carries an action
// nobody has closed. It is what tells a follower that the ending it just
// observed has not been accounted for yet.
func (ws *workspaceSession) hasRunningAction(ctx context.Context, conversationID string) bool {
	record, err := ws.conversationStore().GetMetadata(ctx, conversationID)
	if err != nil {
		return false
	}
	return pendingActionExecutionID(ctx, ws, record) != ""
}
