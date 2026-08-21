package web

// This file is where a sentence becomes a decision a person can take.
//
// The agent of a conversation never starts anything: it closes a message with
// one JSON line declaring what it *would* start, and that line is recognized by
// execution.ParseActionProposal. What it declares is a name and, sometimes, a
// spec code — nothing about whether the process admits that step here and now.
// That knowledge lives in the viewer, and resolving a proposal is the act of
// confronting the declaration with it.
//
// Deriving a proposal starts nothing and records nothing: it reads the process,
// the backlog and the provider, exactly like the routes that render the board,
// and every refusal it reports is the very sentence a start route would answer
// with. That is deliberate: the card a person presses must never promise what
// the start would then refuse, and must never refuse in words the start does
// not use.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// conversationProposalView is the pending proposal as the browser reads it: the
// action, what it would be run on, and whether the process admits it right now.
//
// EventID is the identity of the proposal, because the proposal *is* the event
// that carried it: the decision route names it, and the holder remembers it as
// the watermark past which a proposal counts as answered.
type conversationProposalView struct {
	EventID int64  `json:"event_id"`
	Action  string `json:"action"`
	Label   string `json:"label"`
	Scope   string `json:"scope"`
	// SpecCode, SpecTitle and SpecStatus name the target of a spec-scoped
	// proposal. They are omitted whole for a workspace-scoped one, which has no
	// spec to name — an absence, not a missing lookup.
	SpecCode   string        `json:"spec_code,omitempty"`
	SpecTitle  string        `json:"spec_title,omitempty"`
	SpecStatus domain.Status `json:"spec_status,omitempty"`
	Runnable   bool          `json:"runnable"`
	// UnavailableReason and UnlockedBy are omitted exactly when the action is
	// runnable, by the rule already written on every other action view of this
	// viewer: never a reason next to something a person can press.
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	UnlockedBy        string `json:"unlocked_by,omitempty"`
}

// conversationOutcomeView is the last decision as the browser reads it. It is
// the JSON face of conversationOutcome and adds nothing to it: what was
// decided, about what, and the execution the confirmation started.
type conversationOutcomeView struct {
	ProposalID  int64  `json:"proposal_id"`
	Decision    string `json:"decision"`
	Action      string `json:"action"`
	Label       string `json:"label,omitempty"`
	Scope       string `json:"scope"`
	SpecCode    string `json:"spec_code,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
}

func newConversationOutcomeView(outcome *conversationOutcome) *conversationOutcomeView {
	if outcome == nil {
		return nil
	}
	return &conversationOutcomeView{
		ProposalID:  outcome.ProposalID,
		Decision:    outcome.Decision,
		Action:      outcome.Action,
		Label:       outcome.Label,
		Scope:       outcome.Scope,
		SpecCode:    outcome.SpecCode,
		ExecutionID: outcome.ExecutionID,
	}
}

// pendingProposal finds the proposal awaiting a decision in the history of a
// conversation, together with the id of the event that carried it.
//
// It scans backwards and stops at the first text event that carries one,
// because the pending proposal is the *last* thing the agent proposed: an
// earlier one has been superseded by the conversation itself, and offering it
// again would ask a person to confirm something the agent has moved past.
//
// It reads session.Events(0) — the whole retained history — and never the page
// the caller is about to render. The after_id cursor belongs to the client and
// tracks what it has already drawn; a proposal filtered out by it would vanish
// from the payload on the very next poll after being delivered, which is the
// one moment a person is about to answer it.
//
// A proposal whose event id is not newer than decidedID has already been
// answered, and is not pending: that is what makes a decision hold while the
// agent keeps talking, without any event being rewritten or removed.
func pendingProposal(session *localrun.Session, decidedID int64) (execution.ActionProposal, int64, bool) {
	if session == nil {
		return execution.ActionProposal{}, 0, false
	}
	events := session.Events(0)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Kind != localrun.KindText {
			continue
		}
		proposal, ok := execution.ParseActionProposal(event.Text)
		if !ok {
			continue
		}
		if event.ID <= decidedID {
			return execution.ActionProposal{}, 0, false
		}
		return proposal, event.ID, true
	}
	return execution.ActionProposal{}, 0, false
}

// resolveProposal confronts what the agent declared with what the process, the
// backlog and the provider say about it, and answers with the card a person
// decides on.
//
// It never fails the request. A proposal that cannot be resolved — an
// unresolvable process, a spec that does not exist — comes back runnable:false
// with the obstacle as its reason: the conversation must stay readable, and an
// error page in place of the history would hide everything that was said
// because of one line the agent got wrong.
//
// The cascade below is the start routes' own, in their order, and it says
// nothing they would not say: a name the process does not declare, then a
// status that does not admit the step, then the availability that decides
// whether it can run now. AC-3 is exactly this — the reason on a refused
// proposal is word for word the reason of the start it stands for.
func (s *Server) resolveProposal(ctx context.Context, ws *workspaceSession, proposal execution.ActionProposal, eventID int64) *conversationProposalView {
	view := &conversationProposalView{
		EventID: eventID,
		Action:  proposal.Action,
		// The id is the label until the process gives a better one: a proposal
		// must name what it is about even when nothing else about it can be
		// resolved.
		Label: proposal.Action,
		Scope: template.ScopeWorkspace,
	}
	code := strings.TrimSpace(proposal.Spec)
	if code != "" {
		view.Scope = template.ScopeSpec
		view.SpecCode = code
	}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		view.UnavailableReason = err.Error()
		return view
	}
	if view.Scope == template.ScopeSpec {
		s.resolveSpecProposal(ctx, ws, tpl, view)
	} else {
		s.resolveWorkspaceProposal(ctx, ws, tpl, view)
	}
	view.Runnable = view.UnavailableReason == ""
	if view.Runnable {
		view.UnlockedBy = ""
	}
	return view
}

// resolveSpecProposal fills the spec-scoped half of the cascade. It writes only
// the reason, the remedy and the facts about the spec: whether the action is
// runnable is decided once, by the caller, from the reason alone.
func (s *Server) resolveSpecProposal(ctx context.Context, ws *workspaceSession, tpl template.Template, view *conversationProposalView) {
	for _, action := range tpl.Actions {
		if action.ID == view.Action {
			view.Label = action.Label
			break
		}
	}
	if !admitsAction(tpl.Actions, view.Action) {
		view.UnavailableReason = fmt.Sprintf(
			"unsupported execution action: %s; actions of the %s process: %s",
			view.Action, tpl.ID, actionNames(tpl.Actions),
		)
		view.UnlockedBy = specStageRemedy
		return
	}
	spec, err := ws.conn.ReadSpecDetail(ctx, view.SpecCode)
	if err != nil {
		// A spec the backlog does not hold arrives here, and it is a reason like
		// any other: the agent named a target that is not there, and the person
		// reading the card is the one who can tell it so.
		view.UnavailableReason = err.Error()
		return
	}
	view.SpecTitle = spec.Title
	view.SpecStatus = spec.Status
	if !admitsAction(tpl.ActionsFor(spec.Status), view.Action) {
		// The identical sentence of handleRunSpecAction's start path, so a
		// proposal refused here and a start refused there are one refusal in one
		// wording.
		view.UnavailableReason = fmt.Sprintf(
			"the %s process does not admit the %q action while %s is %s",
			tpl.ID, view.Action, view.SpecCode, spec.Status,
		)
		view.UnlockedBy = specStageRemedy
		return
	}
	// The plan is read for the same reason nextStepFor reads it: the
	// implementation action depends on it, and reporting a proposal runnable
	// without it would be a promise the start route immediately refuses. A plan
	// that cannot be read is no plan, exactly as there.
	tasks, _, err := s.readPlanForSpec(ctx, ws, view.SpecCode)
	if err != nil {
		tasks = nil
	}
	reason := s.actionAvailabilityFor(ctx, ws, view.SpecCode, len(tasks)).reasonFor(view.Action)
	view.UnavailableReason = reason
	// The spec-scoped refusals are already the condition to satisfy — install
	// the provider, plan the spec, wait for the run in flight — so the remedy is
	// the reason itself, as nextStepFor already has it.
	view.UnlockedBy = reason
}

// resolveWorkspaceProposal fills the workspace-scoped half of the cascade, with
// the vocabulary of startWorkspaceAction and workspaceRemedy.
func (s *Server) resolveWorkspaceProposal(ctx context.Context, ws *workspaceSession, tpl template.Template, view *conversationProposalView) {
	for _, action := range tpl.WorkspaceActions {
		if action.ID == view.Action {
			view.Label = action.Label
			break
		}
	}
	if !admitsWorkspaceAction(tpl.WorkspaceActions, view.Action) {
		view.UnavailableReason = fmt.Sprintf(
			"unsupported workspace action: %s; workspace actions of the %s process: %s",
			view.Action, tpl.ID, workspaceActionNames(tpl.WorkspaceActions),
		)
		return
	}
	availability := s.workspaceAvailability(ctx, ws)
	reason := availability.reasonFor(view.Action)
	view.UnavailableReason = reason
	if reason != "" {
		view.UnlockedBy = workspaceRemedy(availability, view.Action)
	}
}

// decideConversationProposalReq is what a person answers with: which proposal,
// and what about it. The id travels because a decision is about the proposal
// the person was looking at, never simply "the pending one" — the agent may
// have proposed something else while the card was on screen.
type decideConversationProposalReq struct {
	ProposalID int64  `json:"proposal_id"`
	Decision   string `json:"decision"`
}

const (
	conversationDecisionAccept  = "accept"
	conversationDecisionDecline = "decline"
	// conversationDecisionDeclined is what the outcome records for a refusal.
	// It is the past tense of the decision on purpose: the request says what is
	// being done, the outcome says what was done.
	conversationDecisionDeclined  = "declined"
	conversationDecisionConfirmed = "confirmed"
)

// handleDecideWorkspaceConversationProposal answers a pending proposal, and is
// the only place a conversation can lead to an execution.
//
// The availability is **recomputed here** and never trusted from the proposal
// that is being answered. Between the moment the agent proposed a step and the
// moment a person confirms it, the workspace can have moved: the spec may have
// changed status, another run may have taken the workspace, the provider may
// have been swapped in the Execution panel. AC-3 is exactly that gap — an
// acceptance the process no longer admits must be refused with the process's own
// sentence, and must start nothing.
//
// A refusal of any kind leaves the proposal undecided on purpose: the watermark
// only moves when something actually happened — a decline, or a start that
// succeeded — so a proposal blocked by a passing obstacle can be confirmed again
// once the obstacle is gone, without the agent having to propose it a second
// time.
func (s *Server) handleDecideWorkspaceConversationProposal(w http.ResponseWriter, r *http.Request) {
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body decideConversationProposalReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	decision := strings.TrimSpace(body.Decision)
	if decision != conversationDecisionAccept && decision != conversationDecisionDecline {
		writeError(w, iox.NewInvalidInput(
			"unsupported decision: "+quoted(decision),
			"decide with "+quoted(conversationDecisionAccept)+" or "+quoted(conversationDecisionDecline),
			nil,
		))
		return
	}
	ws := s.session()
	snapshot, open := ws.conversation.current()
	if !open {
		// The very sentence of the twin routes: there is one absence here, and it
		// is said in one way.
		writeError(w, iox.NewConflict("no conversation is open for this workspace", "open one before deciding a proposal", nil))
		return
	}
	session, found := conversationSessionOf(snapshot)
	if !found {
		writeError(w, iox.NewConflict(
			"the proposals of the conversation "+snapshot.id+" cannot be decided from this viewer",
			"the history of this conversation is not readable from this viewer: close it and open a new one with a provider that exposes an interactive run",
			nil,
		))
		return
	}
	proposal, proposalID, pending := pendingProposal(session, snapshot.decidedProposalID)
	if !pending || proposalID != body.ProposalID {
		// One decides what one is looking at. A proposal that is no longer the
		// pending one has been superseded or already answered, and confirming it
		// would start something nobody currently sees on screen.
		writeError(w, iox.NewConflict(
			"that proposal is no longer the pending one of this conversation",
			"read the conversation again and decide the proposal it shows",
			nil,
		))
		return
	}
	ctx := r.Context()
	if decision == conversationDecisionDecline {
		// A refusal starts nothing, transitions nothing and writes no record: the
		// only thing that happens is the watermark moving, so the card disappears
		// and the conversation stays as it was.
		outcome := conversationOutcome{
			ProposalID: proposalID,
			Decision:   conversationDecisionDeclined,
			Action:     proposal.Action,
			Scope:      template.ScopeWorkspace,
			SpecCode:   strings.TrimSpace(proposal.Spec),
		}
		if outcome.SpecCode != "" {
			outcome.Scope = template.ScopeSpec
		}
		if err := ws.conversation.decide(proposalID, outcome); err != nil {
			writeError(w, iox.NewConflict(err.Error(), "read the conversation of this workspace before deciding a proposal", nil))
			return
		}
		decided, stillOpen := ws.conversation.current()
		writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, ws, heldConversationTarget(decided), decided, stillOpen, afterID))
		return
	}
	// The resolution is the same one the GET renders, run again now: what it
	// says here is what the process admits at the instant of the confirmation.
	view := s.resolveProposal(ctx, ws, proposal, proposalID)
	if !view.Runnable {
		writeError(w, iox.NewConflict(view.UnavailableReason, view.UnlockedBy, nil))
		return
	}
	// The start goes through the board's own functions, with no model chosen for
	// this run: a confirmation starts with the configuration of the workspace,
	// exactly as the panel does when nobody picks anything.
	var started *execution.Execution
	var startErr error
	if view.Scope == template.ScopeSpec {
		started, startErr = s.startSpecAction(ctx, ws, view.SpecCode, execution.ActionID(view.Action), "", nil)
	} else {
		started, startErr = s.startWorkspaceAction(ctx, ws, execution.ActionID(view.Action), "", nil)
	}
	if startErr != nil {
		writeStartError(w, startErr)
		return
	}
	outcome := conversationOutcome{
		ProposalID:  proposalID,
		Decision:    conversationDecisionConfirmed,
		Action:      view.Action,
		Label:       view.Label,
		Scope:       view.Scope,
		SpecCode:    view.SpecCode,
		ExecutionID: started.ID,
	}
	if err := ws.conversation.decide(proposalID, outcome); err != nil {
		// The execution exists: it was started a moment ago and it is running.
		// The holder refusing the outcome — the workspace was left mid-request —
		// cannot unstart it, so the response says what happened rather than
		// pretending nothing did.
		writeError(w, iox.NewConflict(
			"the execution "+started.ID+" was started, but this workspace no longer holds the conversation that proposed it: "+err.Error(),
			"read the executions of this workspace to follow it",
			nil,
		))
		return
	}
	decided, stillOpen := ws.conversation.current()
	writeJSON(w, http.StatusCreated, s.conversationViewOf(ctx, ws, heldConversationTarget(decided), decided, stillOpen, afterID))
}
