package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
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

// conversationViewOf renders the workspace's conversation as the browser reads
// it.
//
// The snapshot is passed in rather than read here so the close route can render
// the conversation it has just closed: after close() the holder is empty, and a
// view read from the holder would answer "there is none" to a caller who has
// every right to keep reading the history of what it just ended.
func (s *Server) conversationViewOf(ctx context.Context, target conversationTarget, snapshot conversationSnapshot, open bool, afterID int64) conversationView {
	view := conversationView{
		Available:         target.reason == "",
		UnavailableReason: target.reason,
		ProviderID:        target.availability.providerID,
		Events:            []execution.RunEvent{},
		LastID:            afterID,
	}
	if !open {
		return view
	}
	rendered := &conversationSnapshotView{
		ID:         snapshot.id,
		WorkingDir: snapshot.workingDir,
		OpenedAt:   snapshot.openedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
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

	session, found := conversationSessionOf(snapshot)
	if !found {
		if view.Notice == "" {
			view.Notice = "the history of this conversation is not readable from this viewer"
		}
		return view
	}
	events := session.Events(afterID)
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
	writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, target, snapshot, open, afterID))
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
	if current, open := ws.conversation.current(); open {
		writeError(w, iox.NewConflict(
			"conversation "+current.id+" is already open for this workspace",
			"close it before opening another one: a workspace holds one conversation at a time",
			nil,
		))
		return
	}
	ctx := r.Context()
	target := s.conversationAvailabilityFor(ctx, ws)
	if target.reason != "" {
		// The very sentence the GET renders next to available:false, so pressing
		// the button on an offer the payload declared unavailable never starts a
		// process only to refuse it a moment later with different words.
		writeError(w, iox.NewConflict(target.reason, conversationRemedy, nil))
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
	if err := ws.conversation.open(id, target.availability.providerID, target.provider, target.collaborator, providerConfig, ws.cfg.ProjectRoot, time.Now().UTC()); err != nil {
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
	writeJSON(w, http.StatusCreated, s.conversationViewOf(ctx, target, snapshot, open, 0))
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
	target := s.conversationAvailabilityFor(ctx, ws)
	writeJSON(w, http.StatusAccepted, s.conversationViewOf(ctx, target, snapshot, true, afterID))
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
	if err := ws.conversation.close(ctx); err != nil {
		writeError(w, iox.NewInternal("closing the conversation "+snapshot.id, err))
		return
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	writeJSON(w, http.StatusOK, s.conversationViewOf(ctx, target, snapshot, true, afterID))
}
