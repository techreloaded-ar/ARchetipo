package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// conversationIDPrefix is what makes a conversation id impossible to mistake
// for an execution id. The ids come from the same generator on purpose — the
// randomness and the collision resistance are the same problem — and the prefix
// is what keeps the two namespaces disjoint, so no lookup by id can ever hand a
// conversation to a route that is about a record.
const conversationIDPrefix = "conv-"

// conversationSnapshotView is the conversation itself, or null when the
// workspace holds none.
//
// It is deliberately not a runSnapshotView: a conversation has no run id and no
// record, and the two facts a caller needs about it that a run does not carry —
// the directory it was opened about and when it was opened — belong here.
type conversationSnapshotView struct {
	ID    string             `json:"id"`
	State execution.RunState `json:"state"`
	Error string             `json:"error,omitempty"`
	// WorkingDir is the project root the conversation was opened about. It
	// travels to the browser because it is the answer to "where is this agent
	// working", which after a workspace switch is not a question the browser can
	// answer from what it already has.
	WorkingDir string `json:"working_dir"`
	OpenedAt   string `json:"opened_at"`
	// SpecCode is the spec this conversation was opened about, omitted for a
	// free conversation. ResumedFrom is the past conversation it was resumed
	// from, omitted for one that started on its own. Both are additive: a
	// client that ignores them reads the payload exactly as before.
	SpecCode    string `json:"spec_code,omitempty"`
	ResumedFrom string `json:"resumed_from,omitempty"`
}

// conversationView is what the browser reads from the four conversation routes.
//
// Available and Conversation answer two different questions and must not be
// collapsed into one: Available says whether a conversation *can be opened*
// here and now — the workspace has a default provider, its runtime is usable
// and it really implements the conversation interface — while Conversation says
// whether there *is* one. A workspace can perfectly well hold an open
// conversation while Available has since turned false, because the default
// provider was changed in the Execution panel after the conversation started;
// and it can be Available with no conversation at all, which is the ordinary
// state before anybody opens one.
//
// Events is never nil, so a client always receives an array and never has to
// test for null before iterating.
type conversationView struct {
	Available bool `json:"available"`
	// UnavailableReason is omitted when a conversation can be opened, so a
	// client can never render a reason next to an offer that has none.
	UnavailableReason string                    `json:"unavailable_reason,omitempty"`
	ProviderID        string                    `json:"provider_id,omitempty"`
	Conversation      *conversationSnapshotView `json:"conversation"`
	Events            []execution.RunEvent      `json:"events"`
	LastID            int64                     `json:"last_id"`
	Truncated         bool                      `json:"truncated"`
	Notice            string                    `json:"notice,omitempty"`
	// Proposal is the action the agent has proposed and nobody has decided yet,
	// or null when there is none. It is always present, never omitted, because
	// "there is nothing to decide" is the answer a poll most often needs, and a
	// client should read it rather than infer it from a missing key.
	Proposal *conversationProposalView `json:"proposal"`
	// Outcome is what became of the last decision. Unlike Proposal it is
	// omitted while nothing has been decided: a conversation that has never
	// answered a proposal has no outcome to speak of.
	Outcome *conversationOutcomeView `json:"outcome,omitempty"`
	// Runs is one block per execution this conversation started, in the order
	// the decisions were taken. It is never nil, so a client always iterates an
	// array: a conversation that has started nothing answers with an empty list
	// and not with null.
	Runs []conversationRunView `json:"runs"`
}

// conversationRunEventWindow bounds how many events of a single run travel
// inside the conversation payload. The conversation is polled every couple of
// seconds and can carry several runs at once, so the whole log of each of them
// would make the payload grow without bound; the run panel remains the place
// where a full history is read.
const conversationRunEventWindow = 200

// conversationRunView is one execution born from this conversation, rendered at
// the point of the discourse that asked for it.
//
// The runView is embedded anonymously on purpose: run, events, last_id,
// approvals, connected, truncated and notice keep exactly the names and the
// types the browser already knows from the run panel, so the same rendering
// code reads a run whether it is met in the panel or inside the conversation.
type conversationRunView struct {
	runView
	ExecutionID string `json:"execution_id"`
	// AnchorEventID is the id of the event that carried the proposal whose
	// confirmation started this execution — "the point at which it was asked
	// for". It is what lets the flow draw the run where the conversation asked
	// for it instead of at the end.
	AnchorEventID int64              `json:"anchor_event_id"`
	Action        execution.ActionID `json:"action"`
	Label         string             `json:"label,omitempty"`
	Scope         execution.Scope    `json:"scope"`
	SpecCode      string             `json:"spec_code,omitempty"`
	// Status and CreatedAt come from the execution record, and are empty
	// exactly when the record could not be read — which Notice then explains.
	Status    execution.ExecutionStatus `json:"status,omitempty"`
	CreatedAt string                    `json:"created_at,omitempty"`
	// Decision is what was answered on the proposal, in the routes' own
	// vocabulary. A block exists only for a confirmation, since a refusal
	// starts nothing, but the field is carried so the flow never has to infer
	// it.
	Decision string `json:"decision"`
	// AwaitingResponse is a fact of the server and not a derivation the browser
	// is asked to make, by the same rule as handleGetWorkspaceRuns: pending
	// approvals exist only while a follower is attached, and attaching it is
	// this side's job.
	AwaitingResponse bool `json:"awaiting_response"`
}

// conversationTarget is everything the routes need once the configuration has
// been resolved: which provider would hold the conversation, what would read
// and command it, and — when it cannot be held at all — the one sentence that
// says why.
type conversationTarget struct {
	availability providerAvailability
	provider     execution.Conversationalist
	collaborator execution.RunCollaborator
	// reason is empty exactly when a conversation can be opened. It is the very
	// sentence providerAvailability.reasonFor produces, never a second wording
	// for the same fact.
	reason string
}

// localSessions is the door onto the sessions a local provider keeps in this
// process. It is discovered on the collaborator rather than required of it, in
// the same spirit as every other optional interface here.
//
// The conversation reads its history through this door and not through
// StreamRunEvents, for one reason: StreamRunEvents replays the backlog and then
// *follows*, and a follower opened per request would either block the request
// until the conversation ends or leave a subscriber behind. A conversation has
// no remote hub to follow — its history is in this very process — so reading
// the session directly is both the cheaper and the only non-leaking way.
type localSessions interface {
	Registry() *localrun.Registry
}

// conversationAvailabilityFor answers "could this workspace open a conversation
// right now, and with what".
//
// The capability check and the interface check are deliberately the same
// sentence: DeclaredCapabilities derives workspace.converse from the interface,
// so a provider that declares it and does not implement it cannot exist — and
// if it ever did, saying so twice with two wordings would only make the viewer
// describe one fact in two ways.
func (s *Server) conversationAvailabilityFor(ctx context.Context, ws *workspaceSession) conversationTarget {
	availability := s.providerAvailabilityFor(ctx, ws)
	target := conversationTarget{availability: availability}
	if reason := availability.reasonFor(execution.CapabilityWorkspaceConverse); reason != "" {
		target.reason = reason
		return target
	}
	if s.registry == nil {
		target.reason = availability.reasonFor(execution.CapabilityWorkspaceConverse)
		return target
	}
	provider, err := s.registry.Resolve(availability.providerID)
	if err != nil {
		denied := availability
		denied.providerErr = err
		target.reason = denied.reasonFor(execution.CapabilityWorkspaceConverse)
		return target
	}
	conversationalist, converses := execution.ConversationalistFor(provider)
	if !converses {
		// Unreachable while the capability stays derived from the interface, and
		// answered with the same sentence anyway: a capability list emptied of the
		// one the provider cannot honour is exactly the state reasonFor already
		// has a phrase for.
		denied := availability
		denied.capabilities = nil
		target.reason = denied.reasonFor(execution.CapabilityWorkspaceConverse)
		return target
	}
	target.provider = conversationalist
	target.collaborator, _ = execution.RunCollaboratorFor(provider)
	return target
}

// heldConversationTarget is the verdict for a workspace that is already holding
// a conversation, built without probing anything.
//
// It costs no `--version` subprocess on purpose. Availability answers "could a
// conversation be opened here", and with one already open nobody is deciding
// that: the panel offers no open button, only the history and the way to close
// it, and the reading route is polled every couple of seconds for as long as
// the conversation lives. Probing on that loop would fork a process per tick to
// answer a question nobody asked. The provider it names is the one actually
// holding the process, not today's default, which may well have been changed in
// the Execution panel since.
func heldConversationTarget(snapshot conversationSnapshot) conversationTarget {
	return conversationTarget{
		availability: providerAvailability{
			providerID:     snapshot.providerID,
			providerConfig: snapshot.providerConfig,
		},
		provider:     snapshot.provider,
		collaborator: snapshot.collaborator,
	}
}

// conversationRemedy is what a caller can do about a refused conversation. It
// points at the Execution panel because every reason reasonFor produces is
// about the default provider of this workspace.
const conversationRemedy = "pick a provider that can hold a conversation in the Execution panel of the configuration"

// conversationSessionOf finds the local session behind a conversation, when the
// collaborator holding it is one of this process's own.
func conversationSessionOf(snapshot conversationSnapshot) (*localrun.Session, bool) {
	if snapshot.collaborator == nil {
		return nil, false
	}
	source, ok := snapshot.collaborator.(localSessions)
	if !ok {
		return nil, false
	}
	return source.Registry().Lookup(snapshot.id)
}

// conversationRunsOf composes the run blocks of a conversation from the
// decisions its holder has recorded.
//
// It reads the register and not the history: a decision that started an
// execution is a fact of the workspace, so a run is still reported when the
// event that proposed it has fallen out of the retention window. Every
// projection is read with cursor 0, exactly as handleGetWorkspaceRuns does,
// because this route reads the run and never advances it — consuming events
// here would starve the panel following the same run.
func (s *Server) conversationRunsOf(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot) []conversationRunView {
	views := make([]conversationRunView, 0, len(snapshot.outcomes))
	for _, outcome := range snapshot.outcomes {
		if strings.TrimSpace(outcome.ExecutionID) == "" {
			// A refusal started nothing, and there is no run to show for it.
			continue
		}
		view := conversationRunView{
			runView:       emptyRunView(""),
			ExecutionID:   outcome.ExecutionID,
			AnchorEventID: outcome.ProposalID,
			Action:        execution.ActionID(outcome.Action),
			Label:         outcome.Label,
			Scope:         execution.Scope(outcome.Scope),
			SpecCode:      outcome.SpecCode,
			Decision:      outcome.Decision,
		}
		record, err := ws.store.Get(ctx, outcome.ExecutionID)
		if err != nil {
			// A run that could not be read is reported with the facts the
			// outcome already carries, never hidden: the person decided it, and
			// the conversation has to keep saying so.
			view.Notice = err.Error()
			views = append(views, view)
			continue
		}
		view.Status = record.Status
		view.CreatedAt = record.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		view.Scope = runScopeOf(record)
		if record.SpecCode != "" {
			view.SpecCode = record.SpecCode
		}
		var projection runProjection
		followed := false
		if record.Status == execution.StatusRunning {
			target, notice, resolveErr := s.resolveRunTarget(ctx, ws, outcome.ExecutionID)
			switch {
			case resolveErr != nil:
				view.Notice = resolveErr.Error()
			case target.follower == nil:
				// No run assigned yet: there is nothing to read and nothing to
				// be waiting on.
				view.Notice = notice
			default:
				projection = target.follower.snapshotView(0)
				followed = true
			}
		} else if follower, ok := ws.followers.get(outcome.ExecutionID); ok {
			// A terminal record never starts a stream: only a follower that is
			// already attached is read, so rendering the conversation cannot
			// resurrect a run that has ended.
			projection = follower.snapshotView(0)
			followed = true
		}
		if followed {
			rendered := projectionView(projection)
			// The tail is what is kept: the newest events are the ones the
			// person is reading, and dropping the head is what the window is
			// for.
			if len(rendered.Events) > conversationRunEventWindow {
				rendered.Events = rendered.Events[len(rendered.Events)-conversationRunEventWindow:]
				rendered.Truncated = true
			}
			view.runView = rendered
			view.AwaitingResponse = len(rendered.Approvals) > 0
		}
		views = append(views, view)
	}
	return views
}

// conversationViewOf renders the workspace's conversation as the browser reads
// it.
//
// The snapshot is passed in rather than read here so the close route can render
// the conversation it has just closed: after close() the holder is empty, and a
// view read from the holder would answer "there is none" to a caller who has
// every right to keep reading the history of what it just ended.
func (s *Server) conversationViewOf(ctx context.Context, ws *workspaceSession, target conversationTarget, snapshot conversationSnapshot, open bool, afterID int64) conversationView {
	view := conversationView{
		Available:         target.reason == "",
		UnavailableReason: target.reason,
		ProviderID:        target.availability.providerID,
		Events:            []execution.RunEvent{},
		LastID:            afterID,
		// Never nil, whether or not a conversation is open: a client reading a
		// workspace that holds none still iterates an array.
		Runs: []conversationRunView{},
	}
	if !open {
		return view
	}
	rendered := &conversationSnapshotView{
		ID:          snapshot.id,
		WorkingDir:  snapshot.workingDir,
		OpenedAt:    snapshot.openedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		SpecCode:    snapshot.specCode,
		ResumedFrom: snapshot.resumedFrom,
	}
	// The state is what the collaborator reports, never what this handler
	// believes: a conversation whose process has left is CLOSED or CRASHED
	// because the session observed it, and no route here writes a state.
	if snapshot.collaborator != nil {
		if reported, err := snapshot.collaborator.ReadRun(ctx, execution.RunRequest{RunID: snapshot.id, ProviderConfig: snapshot.providerConfig}); err == nil {
			rendered.State = reported.State
			rendered.Error = reported.Error
		} else {
			view.Notice = "the state of this conversation could not be read: " + err.Error()
		}
	}
	view.Conversation = rendered
	// The outcome comes from the holder and not from the history, so it is
	// rendered even by a viewer that cannot read the events: what was decided
	// and what it started are facts of the workspace, not lines of a timeline.
	view.Outcome = newConversationOutcomeView(snapshot.outcome)
	// The runs are composed before the history is read, and for the same
	// reason the outcome is: what a conversation started is a fact of the
	// workspace, so it is rendered even by a viewer that cannot read the
	// events of the conversation itself.
	view.Runs = s.conversationRunsOf(ctx, ws, snapshot)

	session, found := conversationSessionOf(snapshot)
	if !found {
		if view.Notice == "" {
			view.Notice = "the history of this conversation is not readable from this viewer"
		}
		return view
	}
	// The resolution runs only when the history actually carries a proposal
	// nobody has answered: it reads the connector and probes the provider, and
	// this route is polled every couple of seconds for as long as the
	// conversation lives. A poll with nothing to decide must cost exactly what
	// it cost before this existed.
	if proposal, proposalID, pending := pendingProposal(session, snapshot.decidedProposalID); pending {
		view.Proposal = s.resolveProposal(ctx, ws, proposal, proposalID)
	}
	events := session.Events(afterID)
	// The read of the live conversation is also what writes it down. It is
	// journalled from the whole history and not from the tail this caller asked
	// for, because the record is the conversation and not the increment; and a
	// failure to write must not fail the read, since a conversation that cannot
	// be saved is still a conversation somebody is entitled to go on reading.
	if err := ws.journal.record(ctx, snapshot.id, session.Events(0), true); err != nil && view.Notice == "" {
		view.Notice = "this conversation could not be written to disk: " + err.Error()
	}
	view.Events = events
	if len(events) > 0 {
		view.LastID = events[len(events)-1].ID
	}
	// A history is partial when the oldest event still kept is newer than the
	// one that would come right after the caller's cursor: everything between
	// the two has been dropped for good, and saying so is the whole point of the
	// retention window.
	if firstID := session.FirstID(); firstID > afterID+1 {
		view.Truncated = true
		view.Notice = fmt.Sprintf(
			"the oldest part of this conversation is no longer shown: %d events have been dropped from the history kept in memory, which begins at event %d",
			session.Dropped(), firstID,
		)
	}
	return view
}

// handleGetWorkspaceConversation serves the conversation of the open workspace,
// or the absence of one.
//
// It touches no execution record and no dispatch, and it opens no follower: a
// conversation is not an action of the process, and the whole of its history
// lives in this process already.
func (s *Server) handleGetWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ws := s.session()
	ctx := r.Context()
	snapshot, open := ws.conversation.current()
	// The verdict is computed only when there is none open: that is the single
	// state in which it decides anything, and it is the only one worth a probe of
	// the runtime. See heldConversationTarget.
	target := heldConversationTarget(snapshot)
	if !open {
		target = s.conversationAvailabilityFor(ctx, ws)
	}
	writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, ws, target, snapshot, open, afterID))
}

// handleOpenWorkspaceConversation opens the single conversation of the open
// workspace.
//
// Nothing is recorded: no execution, no reservation, no file under
// .archetipo/executions. A conversation that left a record behind would appear
// among the actions of the process, and the process never asked for it.
func (s *Server) handleOpenWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	// First check, before any body is decoded and before the provider is
	// touched, for the same reason it is first in handleRunWorkspaceAction: the
	// refusal has to name the directory that disappeared, and it has to happen
	// before any process exists to be released.
	if err := ws.requireReachable(); err != nil {
		writeError(w, err)
		return
	}
	// The body is optional: a conversation is free by default, and asking for
	// one bound to a spec is the exception a caller has to state. An empty body
	// is therefore not a malformed one.
	var body openConversationReq
	if err := decodeJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, err)
		return
	}
	specCode := strings.TrimSpace(body.SpecCode)
	if current, open := ws.conversation.current(); open {
		writeError(w, iox.NewConflict(
			"conversation "+current.id+" is already open for this workspace",
			"close it before opening another one: a workspace holds one conversation at a time",
			nil,
		))
		return
	}
	ctx := r.Context()
	// The spec is verified before the provider is even resolved, so a
	// conversation asked about a spec that is not in this backlog refuses
	// without any agent process ever existing to be released.
	if specCode != "" {
		if _, err := ws.conn.ReadSpecDetail(ctx, specCode); err != nil {
			writeError(w, iox.NewConflict(
				"the spec "+specCode+" does not exist in this workspace",
				"open the conversation with no spec, or name a spec that is in the backlog",
				err,
			))
			return
		}
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	if target.reason != "" {
		// The very sentence the GET renders next to available:false, so pressing
		// the button on an offer the payload declared unavailable never starts a
		// process only to refuse it a moment later with different words.
		writeError(w, iox.NewConflict(target.reason, conversationRemedy, nil))
		return
	}
	// The process vocabulary is resolved here and travels on the request,
	// because the provider does not know the process and must not learn it: the
	// agent may propose an action, and it can only name one that exists if the
	// list reaches it. An unresolvable template is already an error of every
	// other route that needs one, and it is answered the same way here.
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := conversationID()
	if err != nil {
		writeError(w, iox.NewInternal("generating the conversation id", err))
		return
	}
	providerConfig := execution.CloneConfig(target.availability.providerConfig)
	openErr := target.provider.OpenConversation(ctx, execution.ConversationRequest{
		ConversationID: id,
		ProcessActions: conversationActionsOf(tpl),
		// The project root of *this* workspace: the provider is shared by every
		// workspace this process serves, and where the conversation has to run is
		// a fact of the workspace that opened it.
		WorkingDir:     ws.cfg.ProjectRoot,
		ProviderConfig: providerConfig,
	})
	if openErr != nil {
		// A failed open records nothing, on either branch: there is no
		// conversation to hold and nothing to close.
		var configErr *execution.ConfigurationError
		if errors.As(openErr, &configErr) {
			writeError(w, iox.NewConflict(configErr.Error(), conversationRemedy, openErr))
			return
		}
		writeError(w, iox.NewInternal("opening a conversation with the "+quoted(target.availability.providerID)+" provider", openErr))
		return
	}
	if err := ws.conversation.open(id, target.availability.providerID, target.provider, target.collaborator, providerConfig, ws.cfg.ProjectRoot, time.Now().UTC(), specCode, ""); err != nil {
		// The holder refused it — another request won the race, or the workspace
		// has been left. Either way this process is the only one holding the
		// handle, so it closes what it just started instead of leaking it.
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), conversationCloseTimeout)
		defer cancel()
		_ = target.provider.CloseConversation(closeCtx, id)
		writeError(w, iox.NewConflict(err.Error(), "read the conversation of this workspace before opening one", nil))
		return
	}
	snapshot, open := ws.conversation.current()
	// Written down before the view is rendered, and a failure to write is
	// reported rather than raised: the conversation is open either way, and the
	// caller has to learn it will not be found again rather than be told the
	// open failed when it did not.
	journalErr := ws.journal.begin(ctx, snapshot, specCode, "")
	view := s.conversationViewOf(ctx, ws, target, snapshot, open, 0)
	if journalErr != nil && view.Notice == "" {
		view.Notice = "this conversation could not be written to disk: " + journalErr.Error()
	}
	writeJSON(w, http.StatusCreated, view)
}

// conversationActionsOf turns the steps a process declares into the vocabulary
// a conversation agent receives: the spec actions first, then the workspace
// ones, each with the scope that says what it would be run on.
//
// It carries the id, the label and the scope and nothing else. The skill that
// realizes a step and the statuses that admit it are knowledge of the process
// and are decided here, in the viewer, when a proposal is resolved — a provider
// handed them could only misuse them.
func conversationActionsOf(tpl template.Template) []execution.ConversationAction {
	actions := make([]execution.ConversationAction, 0, len(tpl.Actions)+len(tpl.WorkspaceActions))
	for _, action := range tpl.Actions {
		actions = append(actions, execution.ConversationAction{ID: action.ID, Label: action.Label, Scope: template.ScopeSpec})
	}
	for _, action := range tpl.WorkspaceActions {
		actions = append(actions, execution.ConversationAction{ID: action.ID, Label: action.Label, Scope: template.ScopeWorkspace})
	}
	return actions
}

// conversationID mints an id from the very generator the executions use, under
// a prefix of its own.
func conversationID() (string, error) {
	id, err := execution.RandomID()
	if err != nil {
		return "", err
	}
	return conversationIDPrefix + strings.TrimPrefix(id, "exec-"), nil
}

// openConversationReq is the optional body of an open request. SpecCode binds
// the conversation to a spec of this workspace; left out, or left empty, the
// conversation is free — which is what an open with no body at all asks for.
type openConversationReq struct {
	SpecCode string `json:"spec_code"`
}

type sendConversationMessageReq struct {
	Message string `json:"message"`
}

// handleSendWorkspaceConversationMessage delivers a message to the open
// conversation.
//
// It appends nothing to the history. A delivered message becomes history when
// the agent process re-emits it, never before: a line written here would show
// the operator something the agent may never have received, and it is also what
// makes a refused command free of consequences, since a command that wrote
// nothing has nothing to undo. It is the rule already written on
// handleSendRunMessage, and it is the same rule because it is the same
// vocabulary.
func (s *Server) handleSendWorkspaceConversationMessage(w http.ResponseWriter, r *http.Request) {
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body sendConversationMessageReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(w, iox.NewInvalidInput("message is required", "send a non-empty message", nil))
		return
	}
	ws := s.session()
	snapshot, open := ws.conversation.current()
	if !open {
		writeError(w, iox.NewConflict("no conversation is open for this workspace", "open one before sending a message", nil))
		return
	}
	if snapshot.collaborator == nil {
		writeError(w, iox.NewConflict(
			"the conversation "+snapshot.id+" cannot be commanded from this viewer",
			"close it and open a new one with a provider that exposes an interactive run",
			nil,
		))
		return
	}
	ctx := r.Context()
	if err := snapshot.collaborator.SendRunMessage(ctx, execution.RunRequest{RunID: snapshot.id, ProviderConfig: snapshot.providerConfig}, body.Message); err != nil {
		writeError(w, mapRunRefusal(err))
		return
	}
	// A conversation that is already open is held by the provider holding it,
	// not by today's default, so this answer names the same one the GET does:
	// the two routes render the same conversation and must not disagree about
	// who has it or about whether it is available.
	target := heldConversationTarget(snapshot)
	writeJSON(w, http.StatusAccepted, s.conversationViewOf(ctx, ws, target, snapshot, true, afterID))
}

// handleCloseWorkspaceConversation closes the conversation and releases the
// agent process behind it.
//
// The view it answers with is rendered from the conversation it has just
// closed, not from the now empty holder: the operator who closed it may still
// read what was said, and the state that view reports is the one the session
// observed — closed because the process ended, not because this route decided
// so.
func (s *Server) handleCloseWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ws := s.session()
	snapshot, open := ws.conversation.current()
	if !open {
		writeError(w, iox.NewConflict("no conversation is open for this workspace", "there is nothing to close", nil))
		return
	}
	ctx := r.Context()
	// Sealed while the holder still holds it: the final state is read from the
	// conversation that is about to be released, and everything said since the
	// last read is written down before the process behind it goes away.
	ws.sealConversation(ctx, snapshot)
	if err := ws.conversation.close(ctx); err != nil {
		writeError(w, iox.NewInternal("closing the conversation "+snapshot.id, err))
		return
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, ws, target, snapshot, true, afterID))
}
