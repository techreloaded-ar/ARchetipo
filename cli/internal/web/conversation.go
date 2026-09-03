package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
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
	// Action is the step of the process this conversation *is*, omitted for a
	// free one. It is what tells a client that the agent it is reading is doing
	// something rather than answering questions: the thread has an outcome, its
	// prompt was not written by anybody here, and closing it stops work.
	Action execution.ActionID `json:"action,omitempty"`
	// ExecutionID names the record that outcome is written into. For an action
	// conversation it is the conversation's own id — they are one session — and
	// it travels anyway, so a client never has to know that and never has to
	// guess which of the two it is holding.
	ExecutionID string `json:"execution_id,omitempty"`
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
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	ProviderID        string `json:"provider_id,omitempty"`
	// Model is the model identifier the provider named above would use, and
	// ModelOptions the values of the options that model declares — the effort
	// level among them. They travel beside the provider id because the provider
	// alone does not say what is answering: the same provider holds a
	// conversation with a different model and a different reasoning budget from
	// one workspace to the next.
	//
	// Both are omitted when the provider declares no catalog, when its catalog
	// cannot be obtained, or when nothing is configured: a viewer that has only
	// the provider id draws exactly what it drew before these fields existed.
	Model        string                    `json:"model,omitempty"`
	ModelOptions map[string]string         `json:"model_options,omitempty"`
	Conversation *conversationSnapshotView `json:"conversation"`
	Events       []execution.RunEvent      `json:"events"`
	LastID       int64                     `json:"last_id"`
	Truncated    bool                      `json:"truncated"`
	Notice       string                    `json:"notice,omitempty"`
	// Approvals are the decisions the agent of this conversation is waiting on.
	// They exist because a local agent asks before it uses a tool it may not use
	// on its own, and a question nobody can see is a conversation stopped
	// forever: the agent is waiting for an answer the viewer never offered.
	//
	// It is never nil, so a client always iterates an array. A conversation that
	// has ended carries an empty list, always: a decision can only be answered
	// while there is a process to answer it.
	Approvals []execution.PendingApproval `json:"approvals"`
	// Proposal is the action the agent has proposed and nobody has decided yet,
	// or null when there is none. It is always present, never omitted, because
	// "there is nothing to decide" is the answer a poll most often needs, and a
	// client should read it rather than infer it from a missing key.
	Proposal *conversationProposalView `json:"proposal"`
	// Outcome is what became of the last decision. Unlike Proposal it is
	// omitted while nothing has been decided: a conversation that has never
	// answered a proposal has no outcome to speak of.
	Outcome *conversationOutcomeView `json:"outcome,omitempty"`
	// Execution is the record this conversation is the outcome of, present only
	// for a conversation that *is* an action of the process. It is the whole
	// record, exactly as /api/execution/{id} serves it, so a client renders the
	// outcome of a step with the code it already has and never with a second,
	// parallel shape of the same fact.
	//
	// It is omitted — never null — for a free conversation, which has no
	// outcome to have: nothing it says is written into a record.
	Execution *execution.Execution `json:"execution,omitempty"`
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
	// ThreadID names the conversation this run is read in, empty for a run that
	// is read nowhere else.
	//
	// When it is set the block carries no history at all, and that is the point:
	// the run *is* that conversation, so quoting its whole log here would be the
	// same agent rendered twice inside one page, with the block's own composer
	// and the thread's writing into a single turn. What the block keeps is what
	// this conversation actually knows — that it asked for the step, and where
	// the step is happening.
	ThreadID string `json:"thread_id,omitempty"`
}

// conversationTarget is everything the routes need once the configuration has
// been resolved: which provider would hold the conversation, what would read
// and command it, and — when it cannot be held at all — the one sentence that
// says why.
type conversationTarget struct {
	availability providerAvailability
	runtime      execution.Provider
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
	target.runtime = provider
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
		// A run that is read in a conversation of its own is named and not
		// quoted: the block says where the work is, and the thread shows it.
		if thread := threadOfRun(ws, outcome.ExecutionID); thread != "" {
			view.ThreadID = thread
			view.AwaitingResponse = false
			views = append(views, view)
			continue
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
	// The model is read from the configuration the target carries, which for a
	// conversation already open is the one its holder was started with and not
	// today's default: the head must name what is actually answering.
	model, modelOptions := snapshot.model, cloneModelOptions(snapshot.modelOptions)
	if model == "" {
		model, modelOptions = s.conversationModelChoiceOf(ctx, target.availability.providerID, target.availability.providerConfig)
	}
	view := conversationView{
		Available:         target.reason == "",
		UnavailableReason: target.reason,
		ProviderID:        target.availability.providerID,
		Model:             model,
		ModelOptions:      modelOptions,
		Events:            []execution.RunEvent{},
		LastID:            afterID,
		// Never nil, whether or not a conversation is open: a client reading a
		// workspace that holds none still iterates an array.
		Runs:      []conversationRunView{},
		Approvals: []execution.PendingApproval{},
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
		Action:      snapshot.action,
		ExecutionID: snapshot.executionID,
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
	// The decisions the agent is waiting on are read from the collaborator that
	// holds it, on every poll, for the same reason the state above is: they are
	// the process's own statement and never a deduction of this handler. A read
	// that fails leaves the list empty and raises no notice of its own — the
	// state read just above has already spoken for a conversation that cannot be
	// reached at all.
	if snapshot.collaborator != nil {
		if pending, err := snapshot.collaborator.ReadRunApprovals(ctx, execution.RunRequest{RunID: snapshot.id, ProviderConfig: snapshot.providerConfig}); err == nil && pending != nil {
			view.Approvals = pending
		}
	}
	// The outcome comes from the holder and not from the history, so it is
	// rendered even by a viewer that cannot read the events: what was decided
	// and what it started are facts of the workspace, not lines of a timeline.
	view.Outcome = newConversationOutcomeView(snapshot.outcome)
	// The runs are composed before the history is read, and for the same
	// reason the outcome is: what a conversation started is a fact of the
	// workspace, so it is rendered even by a viewer that cannot read the
	// events of the conversation itself.
	view.Runs = s.conversationRunsOf(ctx, ws, snapshot)

	// The outcome of an action conversation is its record, read on every poll
	// like everything else about it: the record is written by the dispatch, and
	// a copy kept here would be a second statement of a fact that has one
	// author. A read that fails leaves it absent rather than raising a notice —
	// the thread is still the thread, and the next poll says the same thing.
	if strings.TrimSpace(snapshot.executionID) != "" {
		if record, err := ws.store.Get(ctx, snapshot.executionID); err == nil {
			view.Execution = &record
		}
	}

	session, found := conversationSessionOf(snapshot)
	if !found {
		// An action conversation is held the instant its record exists, which is
		// a moment before the provider has registered the session behind it: the
		// process still has to be spawned and probed. Saying the history is
		// unreadable in that window would be answering "this viewer cannot read
		// it" to a thread that is simply about to start speaking. A record that
		// has already stopped running is a different matter, and keeps the
		// sentence.
		if view.Execution != nil && view.Execution.Status == execution.StatusRunning {
			return view
		}
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

// conversationNotFound is the one refusal for an id this workspace neither
// holds alive nor keeps on disk.
//
// It exists as a function so every route that can be handed an unknown id — the
// read, the message, the close, the proposal and the resume — refuses with the
// same sentence: five wordings for one absence would make the same mistake look
// like five different problems.
func conversationNotFound(id string, err error) error {
	return iox.NewNotFound(
		"the conversation "+id+" does not exist in this workspace",
		"open the list of the conversations of the workspace to see the ones it holds",
		err,
	)
}

// conversationModelChoiceOf names the model — and the options of that model —
// the given provider configuration resolves to.
//
// It costs no subprocess: a catalog is declared statically by the provider that
// has one, so this is a read of what the package already knows plus a parse of
// the configuration. That matters because the conversation routes are polled
// every couple of seconds for as long as a conversation lives.
//
// Every way of not knowing answers the same way — two empty values — and none
// of them is an error: a provider without a catalog, a catalog that could not be
// obtained and a configuration that names no model are all "there is nothing to
// say about the model here", and the head simply names the provider alone.
func (s *Server) conversationModelChoiceOf(ctx context.Context, providerID string, config map[string]any) (string, map[string]string) {
	if s.registry == nil || strings.TrimSpace(providerID) == "" {
		return "", nil
	}
	provider, err := s.registry.Resolve(providerID)
	if err != nil {
		return "", nil
	}
	resolution := execution.ResolveModelChoice(ctx, provider, config)
	if !resolution.Declared || resolution.Reason != "" {
		return "", nil
	}
	return resolution.Choice.Model, resolution.Choice.Options
}

func cloneModelOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(options))
	for name, value := range options {
		cloned[name] = value
	}
	return cloned
}

// conversationOpenability is the answer to "is there a provider here that could
// hold a conversation", as the reads render it next to whatever they were asked
// for.
//
// It is deliberately *not* "is there room for another one". Room is a fact of
// this instant that two requests can disagree about, and the refusal that
// declares the limit is built by the open route from the live set at the moment
// it refuses — naming which conversations to close, which no boolean can do. A
// page that hid the offer while the workspace was full would also have to hide
// the only sentence that says why.
type conversationOpenability struct {
	available  bool
	reason     string
	providerID string
	// model and modelOptions travel with the provider id for the same reason
	// conversationView carries them: naming who would answer without naming
	// what would answer says only half of it.
	model        string
	modelOptions map[string]string
}

// conversationOpenabilityOf answers that question at the cost the caller can
// afford, and never says a provider is missing while one is demonstrably
// holding a conversation.
//
// The runtime is probed only when the workspace holds *nothing* alive. With a
// live conversation the answer is already settled by the fact that one is
// running — which is exactly what heldConversationTarget says, and for the same
// reason: these reads are polled every couple of seconds for as long as a
// conversation lives, and CheckAvailability costs a `--version` subprocess. A
// probe on that loop would fork a process per tick to answer a question nobody
// is asking while an agent is already talking.
func (s *Server) conversationOpenabilityOf(ctx context.Context, ws *workspaceSession) conversationOpenability {
	if live := ws.conversation.list(); len(live) > 0 {
		held := live[len(live)-1]
		model, options := s.conversationModelChoiceOf(ctx, held.providerID, held.providerConfig)
		return conversationOpenability{
			available:    true,
			providerID:   held.providerID,
			model:        model,
			modelOptions: options,
		}
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	model, options := s.conversationModelChoiceOf(ctx, target.availability.providerID, target.availability.providerConfig)
	return conversationOpenability{
		available:    target.reason == "",
		reason:       target.reason,
		providerID:   target.availability.providerID,
		model:        model,
		modelOptions: options,
	}
}

// pastConversationViewOf renders a conversation that has ended, with the very
// payload the live one travels in.
//
// One payload and not two: the client asks for a conversation by id and must not
// have to know, before asking, whether this process happens to be holding it.
// What tells the two apart is the state inside the payload, which is the
// record's own and is never interpreted here.
func pastConversationViewOf(record conversationlog.Record, openability conversationOpenability, afterID int64) conversationView {
	events := make([]execution.RunEvent, 0, len(record.Events))
	for _, event := range record.Events {
		// The cursor is honoured here exactly as the live read honours it, so a
		// client polling a conversation across its own ending never has to change
		// how it reads.
		if event.ID > afterID {
			events = append(events, event)
		}
	}
	view := conversationView{
		Available:         openability.available,
		UnavailableReason: openability.reason,
		ProviderID:        record.ProviderID,
		Model:             record.Model,
		ModelOptions:      cloneModelOptions(record.ModelOptions),
		Conversation: &conversationSnapshotView{
			ID: record.ID,
			// The state travels as the record left it, with no interpretation:
			// a conversation sealed as crashed is not turned into a closed one
			// by being read.
			State:       execution.RunState(record.FinalState),
			WorkingDir:  record.WorkingDir,
			OpenedAt:    record.OpenedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			SpecCode:    record.SpecCode,
			ResumedFrom: record.ResumedFrom,
			// Un passo del processo resta quel passo anche dopo essere finito.
			// Il journal li scrive entrambi proprio perché una trascrizione che
			// non sa nominare il proprio esito lascia il lettore con tutta la
			// discussione e nessuna strada per sapere com'è andata: dirli qui è
			// leggere il record, non interpretarlo.
			Action:      execution.ActionID(record.Action),
			ExecutionID: record.ExecutionID,
		},
		Events: events,
		LastID: afterID,
		// A conversation that has ended proposes nothing and commands nothing:
		// there is no pending proposal to decide and no run block to compose,
		// because both are read from a holder that no longer holds it.
		Proposal: nil,
		Runs:     []conversationRunView{},
		// A conversation that has ended waits on nothing: there is no process
		// left to answer, and offering a live button on one would be an offer
		// nothing can honour.
		Approvals: []execution.PendingApproval{},
	}
	if len(events) > 0 {
		view.LastID = events[len(events)-1].ID
	}
	return view
}

// handleGetWorkspaceConversation serves one conversation of the open workspace,
// live or ended, by its id.
//
// When the workspace is holding it, the history comes from the session in
// memory rather than from the file: the record is rewritten as the live
// conversation is read, so the file is at most one read behind, and the two must
// not be allowed to disagree under the eyes of whoever is reading them. When it
// is not, the record on disk *is* the conversation.
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
	id := strings.TrimSpace(r.PathValue("id"))
	if snapshot, live := ws.conversation.get(id); live {
		// The verdict costs no probe for a conversation that is being held. See
		// heldConversationTarget.
		writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, ws, heldConversationTarget(snapshot), snapshot, true, afterID))
		return
	}
	record, err := s.readPastConversation(ctx, ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	view := pastConversationViewOf(record, s.conversationOpenabilityOf(ctx, ws), afterID)
	// Il record di un passo finito si legge qui e non dentro
	// pastConversationViewOf, che non tocca nessun archivio: è la stessa lettura
	// che la gemella viva fa dal suo, con la stessa regola sul fallimento —
	// assente, non un avviso, perché il thread resta il thread.
	//
	// Senza di essa una conversazione che *è* un passo perderebbe il proprio
	// esito nel momento in cui smette di essere tenuta in memoria, e con esso
	// tutto ciò che il passo ha prodotto e che non è scritto altrove: una
	// proposta, per esempio, che finché nessuno la conferma vive solo lì.
	if id := strings.TrimSpace(record.ExecutionID); id != "" && ws.store != nil {
		if outcome, err := ws.store.Get(ctx, id); err == nil {
			view.Execution = &outcome
		}
	}
	writeJSON(w, http.StatusOK, view)
}

// readPastConversation reads one conversation from the journal of the open
// workspace, refusing with conversationNotFound for an id it does not keep.
func (s *Server) readPastConversation(ctx context.Context, ws *workspaceSession, id string) (conversationlog.Record, error) {
	store := ws.conversationStore()
	if store == nil {
		return conversationlog.Record{}, iox.NewInternal("reading the conversation "+id, errors.New("this workspace keeps no conversation journal"))
	}
	record, err := store.Get(ctx, id)
	if err != nil {
		if conversationRecordMissing(err) {
			return conversationlog.Record{}, conversationNotFound(id, err)
		}
		return conversationlog.Record{}, iox.NewInternal("reading the conversation "+id, err)
	}
	return record, nil
}

// handleOpenWorkspaceConversation opens a conversation on the open workspace,
// beside the ones it is already holding.
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
	ctx := r.Context()
	// Asked here, before the provider is told anything: an open refused only by
	// the holder would already have started an agent process, and the refusal
	// would leave it running behind a request that has no reason to hold it. It
	// is not a guarantee — the holder can be shut down in between — which is why
	// the holder asks again below and this handler keeps its recovery branch.
	if err := s.refuseConversationOpen(ctx, ws); err != nil {
		writeError(w, err)
		return
	}
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
	opened, err := s.openConversationOn(ctx, ws, conversationOpenSpec{
		specCode:     specCode,
		title:        strings.TrimSpace(body.Title),
		model:        strings.TrimSpace(body.Model),
		modelOptions: body.ModelOptions,
	})
	if err != nil {
		writeStartError(w, err)
		return
	}
	view := s.conversationViewOf(ctx, ws, opened.target, opened.snapshot, opened.held, 0)
	if opened.notice != "" && view.Notice == "" {
		view.Notice = opened.notice
	}
	writeJSON(w, http.StatusCreated, view)
}

// conversationOpenSpec is what an open is about, and nothing about how it was
// asked for: the three routes that open one — the plain open, the resume, and
// the adoption of a run that found no thread — differ in exactly these fields
// and in nothing else.
type conversationOpenSpec struct {
	// specCode is the spec the conversation is about, empty for a free one.
	specCode string
	// resumedFrom is the id of the past conversation this one takes up.
	resumedFrom string
	// transcript is the history handed to the agent as context. It travels with
	// resumedFrom on a resume and is empty everywhere else.
	transcript string
	// title is the name the thread is to carry. Empty means "name it the way
	// threads are named", which is by the first thing said in it.
	title string
	// model and modelOptions are the choice for this conversation alone. Empty
	// means that the workspace configuration is inherited unchanged.
	model        string
	modelOptions map[string]string
}

// openedConversation is a conversation that exists by the time it is returned:
// the agent process is running, the holder has the handle, and the journal has
// the record.
//
// notice is a journal write that failed. It is carried and not raised for the
// reason it always was: the conversation is open either way, and the caller has
// to learn it will not be found again rather than be told the open failed when
// it did not.
type openedConversation struct {
	target   conversationTarget
	snapshot conversationSnapshot
	held     bool
	notice   string
}

// openConversationOn starts an agent process for this workspace and hands back
// the conversation holding it.
//
// It is the single body of every open there is. It was inlined in the open
// route and copied into the resume one, and the copy is what a third caller
// would have had to make a third time: an adoption that opens the thread a run
// was started without. Everything route-shaped stays with the routes — reading
// the body, checking that the spec exists, refusing an unreachable project
// root — and everything that has to happen in order for a process not to be
// leaked lives here, once.
func (s *Server) openConversationOn(ctx context.Context, ws *workspaceSession, spec conversationOpenSpec) (openedConversation, error) {
	// Asked before the provider is told anything: an open refused only by the
	// holder would already have started an agent process, and the refusal would
	// leave it running behind a request that has no reason to hold it.
	if err := s.refuseConversationOpen(ctx, ws); err != nil {
		return openedConversation{}, err
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	if target.reason != "" {
		// The very sentence the GET renders next to available:false, so pressing
		// the button on an offer the payload declared unavailable never starts a
		// process only to refuse it a moment later with different words.
		return openedConversation{}, iox.NewConflict(target.reason, conversationRemedy, nil)
	}
	// A conversation is one provider session, so its model is chosen before the
	// provider opens that session. The shared resolver gives conversations the
	// same validation and pruning rules as a single run, while returning a clone
	// that is never persisted back to the workspace configuration.
	effectiveConfig, modelChoice, err := resolveRunModelChoice(
		ctx,
		target.runtime,
		target.availability.providerConfig,
		spec.model,
		spec.modelOptions,
	)
	if err != nil {
		return openedConversation{}, wrapRunModelChoiceError(err)
	}
	target.availability.providerConfig = effectiveConfig
	// The process vocabulary is resolved here and travels on the request,
	// because the provider does not know the process and must not learn it: the
	// agent may propose an action, and it can only name one that exists if the
	// list reaches it. An unresolvable template is already an error of every
	// other route that needs one, and it is answered the same way here.
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		return openedConversation{}, err
	}
	id, err := conversationID()
	if err != nil {
		return openedConversation{}, iox.NewInternal("generating the conversation id", err)
	}
	providerConfig := execution.CloneConfig(target.availability.providerConfig)
	model := ""
	var modelOptions map[string]string
	if modelChoice != nil {
		model = modelChoice.Model
		modelOptions = cloneModelOptions(modelChoice.Options)
	}
	openErr := target.provider.OpenConversation(ctx, execution.ConversationRequest{
		ConversationID: id,
		ProcessActions: conversationActionsOf(tpl),
		// The project root of *this* workspace: the provider is shared by every
		// workspace this process serves, and where the conversation has to run is
		// a fact of the workspace that opened it.
		WorkingDir:     ws.cfg.ProjectRoot,
		ProviderConfig: providerConfig,
		Context:        spec.transcript,
	})
	if openErr != nil {
		// A failed open records nothing, on either branch: there is no
		// conversation to hold and nothing to close.
		var configErr *execution.ConfigurationError
		if errors.As(openErr, &configErr) {
			return openedConversation{}, iox.NewConflict(configErr.Error(), conversationRemedy, openErr)
		}
		// The cause travels into the message and not only into Cause, which the
		// browser never sees. A provider holding the agent on another machine
		// fails for reasons only it knows — a credential it was not given, a hub
		// that is not answering — and the person in front of the viewer has no
		// log to go and read: without the sentence the provider wrote, all they
		// are told is that something went wrong.
		return openedConversation{}, iox.NewInternal("opening a conversation with the "+quoted(target.availability.providerID)+" provider: "+openErr.Error(), openErr)
	}
	if err := ws.conversation.open(conversationHold{
		id:             id,
		providerID:     target.availability.providerID,
		provider:       target.provider,
		collaborator:   target.collaborator,
		providerConfig: providerConfig,
		workingDir:     ws.cfg.ProjectRoot,
		openedAt:       time.Now().UTC(),
		specCode:       spec.specCode,
		resumedFrom:    spec.resumedFrom,
		model:          model,
		modelOptions:   modelOptions,
	}); err != nil {
		// The holder refused it — the workspace has been left while this open
		// was in flight. This process is the only one holding the handle, so it
		// closes what it just started instead of leaking it.
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), conversationCloseTimeout)
		defer cancel()
		_ = target.provider.CloseConversation(closeCtx, id)
		return openedConversation{}, conversationOpenRefusal(ctx, ws, err)
	}
	snapshot, held := ws.conversation.get(id)
	opened := openedConversation{target: target, snapshot: snapshot, held: held}
	if journalErr := ws.journal.begin(ctx, snapshot, spec.specCode, spec.resumedFrom); journalErr != nil {
		opened.notice = "this conversation could not be written to disk: " + journalErr.Error()
	}
	// After begin and never instead of it: the record exists first, and the name
	// is written onto it. A name that could not be written is not a failure of
	// the open — the thread exists, holds the agent and is listed, under the
	// dated title the journal falls back to — so it is dropped rather than
	// turned into a notice about a conversation that is perfectly fine.
	_ = ws.journal.name(ctx, snapshot.id, spec.title)
	return opened, nil
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
//
// Title names the thread. It exists for the thread nobody is going to type a
// first message into: the viewer opens one when a step is started outside every
// conversation, and a thread named "Conversazione del 26/08/2026 09:14" in an
// index full of them is no name at all. Left out, the journal names the thread
// as it always has — by the first thing the person says in it, and by the
// moment it was opened until they do.
type openConversationReq struct {
	SpecCode     string            `json:"spec_code"`
	Title        string            `json:"title"`
	Model        string            `json:"model"`
	ModelOptions map[string]string `json:"model_options"`
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
	id := strings.TrimSpace(r.PathValue("id"))
	snapshot, live := ws.conversation.get(id)
	if !live {
		// Not being held is not the same as not existing: an id the journal
		// knows is a conversation that has ended, and one it does not know is a
		// conversation of no workspace. The refusal says which of the two it is,
		// with the sentence every other route uses for the second.
		writeError(w, s.conversationGoneRefusal(r.Context(), ws, id, "send a message"))
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

type respondConversationApprovalReq struct {
	OptionID string `json:"option_id"`
}

// handleRespondWorkspaceConversationApproval answers a decision the agent of a
// conversation is waiting on.
//
// It is the twin of handleRespondRunApproval and refuses in the same words for
// the same reasons; what differs is only what names the session — a
// conversation id instead of an execution record. It writes nothing into the
// history: what the answer did becomes visible through what the agent does with
// it, a tool that runs or a tool result reporting the refusal, which is the
// rule every command of a local run follows.
func (s *Server) handleRespondWorkspaceConversationApproval(w http.ResponseWriter, r *http.Request) {
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	approvalID := strings.TrimSpace(r.PathValue("approvalId"))
	if approvalID == "" {
		writeError(w, iox.NewInvalidInput("missing approval id", "use /api/workspace/conversations/<id>/approvals/<approvalId>", nil))
		return
	}
	var body respondConversationApprovalReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(body.OptionID) == "" {
		writeError(w, iox.NewInvalidInput("option_id is required", "pick one of the options the approval declares", nil))
		return
	}
	ws := s.session()
	id := strings.TrimSpace(r.PathValue("id"))
	snapshot, live := ws.conversation.get(id)
	if !live {
		writeError(w, s.conversationGoneRefusal(r.Context(), ws, id, "answer a decision"))
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
	req := execution.RunRequest{RunID: snapshot.id, ProviderConfig: snapshot.providerConfig}
	if err := snapshot.collaborator.RespondRunApproval(ctx, req, approvalID, body.OptionID); err != nil {
		writeError(w, mapRunRefusal(err))
		return
	}
	writeJSON(w, http.StatusAccepted, s.conversationViewOf(ctx, ws, heldConversationTarget(snapshot), snapshot, true, afterID))
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
	ctx := r.Context()
	id := strings.TrimSpace(r.PathValue("id"))
	snapshot, live := ws.conversation.get(id)
	if !live {
		writeError(w, s.conversationGoneRefusal(ctx, ws, id, "close it"))
		return
	}
	// Sealed while the holder still holds it: the final state is read from the
	// conversation that is about to be released, and everything said since the
	// last read is written down before the process behind it goes away.
	ws.sealConversation(ctx, snapshot)
	// Only this one is released: closeOne drops the entry it names and leaves
	// every sibling of the workspace exactly as it was, which is the whole of
	// what "close this conversation" has to mean once a workspace can hold
	// several.
	if err := ws.conversation.closeOne(ctx, id); err != nil {
		writeError(w, iox.NewInternal("closing the conversation "+snapshot.id, err))
		return
	}
	// Rendered from the conversation just closed and from the provider that was
	// holding it — heldConversationTarget carries no reason, so the payload says
	// available, which is true: closing one has just made room for another.
	writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, ws, heldConversationTarget(snapshot), snapshot, true, afterID))
}

// conversationGoneRefusal explains why a command cannot reach the conversation
// it named: because that conversation has ended, or because no conversation of
// this workspace ever had that id.
//
// The two are told apart on purpose. "It ended" is a state a person can act on —
// resume it — while "it does not exist" is a mistake, and answering both with
// one sentence would hide the difference behind a single 404.
func (s *Server) conversationGoneRefusal(ctx context.Context, ws *workspaceSession, id, intent string) error {
	if _, err := s.readPastConversation(ctx, ws, id); err != nil {
		return err
	}
	return iox.NewConflict(
		"the conversation "+id+" is no longer live on this workspace",
		"resume it to go on from where it ended, or "+intent+" on a conversation that is still live",
		nil,
	)
}

// refuseConversationOpen is the one place an open — ordinary or resumed — is
// refused before any agent process exists.
//
// It used to be about the ceiling of live conversations and is now about the
// only thing left that can refuse one: a workspace that has been left. It stays
// shared with the resume route so the sentence a person reads exists once.
func (s *Server) refuseConversationOpen(ctx context.Context, ws *workspaceSession) error {
	if err := ws.conversation.canOpen(); err != nil {
		return conversationOpenRefusal(ctx, ws, err)
	}
	return nil
}

// conversationOpenRefusal turns a holder refusal into the HTTP one.
//
// It carries the holder's own sentence and adds nothing to it. It used to have
// a second half, which named the live conversations one by one so a person
// could pick which to close: with the ceiling gone there is nothing left to
// make room for, and a refusal that listed threads would be asking for a
// gesture that no longer unblocks anything.
func conversationOpenRefusal(_ context.Context, _ *workspaceSession, err error) error {
	return iox.NewConflict(err.Error(), "read the conversations of this workspace before opening one", err)
}
