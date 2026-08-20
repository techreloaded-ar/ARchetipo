package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// workspaceExecutionKey is the reservation key of a workspace-scoped execution.
// It is the empty spec code on purpose: an execution whose object is the
// workspace carries no spec, so it is stored, listed and reserved under "" —
// the same key store.ListBySpec answers with, and one no spec can ever collide
// with.
const workspaceExecutionKey = ""

// workspaceActionView is a process action on the workspace plus whether this
// workspace can actually start it now. template.WorkspaceAction is embedded by
// value so id, label and skill keep the shape the spec actions already have,
// and the browser can render both with the same chip.
type workspaceActionView struct {
	template.WorkspaceAction
	// Offered is the precondition the *state of the workspace* puts on the
	// action: whether this workspace, as it is now, is a workspace the action
	// makes sense on at all. Runnable is Offered plus a usable provider and no
	// execution in flight. The client draws only what is Offered, so an action
	// the workspace has outgrown — a first inception over an existing PRD, a
	// first generation over an existing backlog — disappears instead of showing
	// up disabled, while an offered action that merely lacks a provider stays
	// visible with its reason.
	Offered  bool `json:"offered"`
	Runnable bool `json:"runnable"`
	// UnavailableReason is omitted when the action is runnable, so a client can
	// never render a reason next to an action that has none.
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// workspaceActionsView is the answer to "what can I do with this workspace
// now?". HasPRD is part of it because it is the single fact that decides
// whether a first inception is offered at all (AC-1/AC-5), and the browser must
// not have to derive it from a separate call to the PRD route.
type workspaceActionsView struct {
	Template templateView `json:"template"`
	HasPRD   bool         `json:"has_prd"`
	// HasBacklog is here for the same reason HasPRD is: it is the single fact
	// that decides whether a *first* backlog generation is offered at all, and
	// the browser must not have to derive it from a second call to the board.
	HasBacklog bool                  `json:"has_backlog"`
	Actions    []workspaceActionView `json:"actions"`
	// Execution is the most recent workspace execution, or null when there is
	// none. It travels with the actions for the same reason it travels with a
	// spec detail: a reloaded browser finds the run it started without having
	// remembered any identifier.
	Execution *execution.Execution `json:"execution"`
}

type runWorkspaceActionReq struct {
	Action string `json:"action"`
}

// handleGetWorkspaceActions serves GET /api/workspace/actions.
//
// It is the workspace-scoped twin of the actions carried by a spec detail, and
// it deliberately answers with the same vocabulary: an action is either
// runnable, or it is not and says why. Everything the decision needs — the
// default provider, its capabilities, whether its runtime is usable, whether a
// workspace execution is already running, whether a PRD is already there —
// stays on the server.
func (s *Server) handleGetWorkspaceActions(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	availability := s.workspaceAvailability(ctx, ws)
	actions := make([]workspaceActionView, 0, len(tpl.WorkspaceActions))
	for _, action := range tpl.WorkspaceActions {
		reason := availability.reasonFor(action.ID)
		actions = append(actions, workspaceActionView{
			WorkspaceAction:   action,
			Offered:           availability.offers(action.ID) == "",
			Runnable:          reason == "",
			UnavailableReason: reason,
		})
	}
	latest, err := s.latestExecution(ctx, ws, workspaceExecutionKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceActionsView{
		Template:   templateView{ID: tpl.ID, Version: tpl.Version},
		HasPRD:     availability.hasPRD,
		HasBacklog: availability.hasBacklog,
		Actions:    actions,
		Execution:  latest,
	})
}

// handleRunWorkspaceAction serves POST /api/workspace/execution: it starts one
// action whose object is the workspace itself and answers before the provider
// has finished, exactly as the spec route does and for the same reason — an
// inception is a conversation, and a response that waited for it would hang for
// as long as the person keeps talking.
func (s *Server) handleRunWorkspaceAction(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	var req runWorkspaceActionReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	action := execution.ActionID(strings.TrimSpace(req.Action))
	if action == "" {
		writeError(w, iox.NewInvalidInput("action is required", "supported actions: "+supportedActions(), nil))
		return
	}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	// A name the process does not declare is a malformed request (400); a
	// declared name this viewer cannot dispatch, or a workspace that cannot host
	// the action right now, is a state conflict (409).
	if !admitsWorkspaceAction(tpl.WorkspaceActions, string(action)) {
		writeError(w, iox.NewInvalidInput(
			"unsupported workspace action: "+string(action),
			"workspace actions of the "+tpl.ID+" process: "+workspaceActionNames(tpl.WorkspaceActions),
			nil,
		))
		return
	}
	if _, err := execution.RequiredCapability(action); err != nil {
		writeError(w, iox.NewConflict(
			"the "+string(action)+" action cannot be started from the viewer yet",
			"run it from a coding agent; the viewer can start: "+supportedActions(),
			err,
		))
		return
	}
	ctx := r.Context()
	availability := s.workspaceAvailability(ctx, ws)
	// Every refusal the GET route renders next to a disabled action is the same
	// refusal here, so pressing an action the payload declared unavailable never
	// creates a record only to close it a moment later with this very sentence.
	if reason := availability.reasonFor(string(action)); reason != "" {
		writeError(w, iox.NewConflict(reason, workspaceRemedy(availability, string(action)), nil))
		return
	}
	// existedBefore is captured before the run starts, and only here: it is what
	// separates a document this run wrote from one the workspace already had, so
	// the rollback of a failed inception can never touch a pre-existing PRD.
	existedBefore := availability.hasPRD
	// backlogExistedBefore is captured here for exactly the same reason: read
	// after the run it would already describe what the run itself wrote, and a
	// rollback deciding on it could remove a backlog that belongs to the
	// workspace rather than to this execution.
	backlogExistedBefore := availability.hasBacklog
	// specCodesBefore is captured here for the same reason and with the same
	// force: a spec drafting run is confirmed by the backlog *not* having
	// grown, and a list read after the run would already contain whatever that
	// run wrote — the check would then confirm exactly what it exists to catch.
	specCodesBefore := append([]string(nil), availability.specCodes...)
	providerID := availability.providerID
	providerConfig := execution.CloneConfig(availability.providerConfig)

	if err := s.guardSingleWorkspaceExecution(ctx, ws); err != nil {
		writeError(w, err)
		return
	}
	// The claimed effect is verified inside the terminal write, not after it: a
	// browser polling every two seconds settles on the first terminal status it
	// reads, so a success closed now and disowned a moment later is a success
	// the user keeps looking at next to a workspace with no PRD. The rollback
	// runs in the same window, on the record the continuation is about to write,
	// so a failed run is persisted already knowing what happened to the partial
	// document.
	confirm := func(confirmCtx context.Context, outcome *execution.Execution) {
		_ = execution.VerifyActionEffect(confirmCtx, ws.conn, outcome.Action, workspaceExecutionKey, outcome)
		// Each action takes back only its own artifact, with its own flag: the
		// rollback follows what the run was about, never what the workspace
		// happens to hold.
		switch outcome.Action {
		case execution.ActionInception:
			execution.DiscardPartialPRD(confirmCtx, s.prdDiscarder(ws), existedBefore, outcome)
		case execution.ActionBacklog:
			execution.DiscardPartialBacklog(confirmCtx, s.backlogDiscarder(ws), backlogExistedBefore, outcome)
		case execution.ActionSpecDraft:
			// The only action whose confirmation is not VerifyActionEffect's:
			// what has to be established is that the backlog did not grow, and
			// that needs the snapshot taken above, which only this scope holds.
			execution.ConfirmSpecDraft(confirmCtx, ws.conn, s.specDeleter(ws), specCodesBefore, outcome)
		}
	}
	started, continuation, err := ws.service.StartWorkspace(ctx, action, providerID, providerConfig, confirm)
	if err != nil {
		ws.dispatch.release(workspaceExecutionKey)
		// A rejected configuration is answered by the same renderer the Execution
		// panel already understands, so the form can point at the offending field.
		var configErr *execution.ConfigurationError
		if errors.As(err, &configErr) {
			writeProviderConfigError(w, err)
			return
		}
		writeError(w, mapExecutionStartError(err, providerID))
		return
	}
	ws.dispatch.claim(workspaceExecutionKey, started.ID)
	writeJSON(w, http.StatusCreated, started)

	ws.dispatch.run(func(dispatchCtx context.Context) {
		// Deferred in this order so they unwind in the other one: the workspace
		// stops being busy first, and only then is every connected client told to
		// re-read. Publishing while the reservation still stands would send them
		// back an action still marked unavailable.
		defer s.broker.Publish()
		defer ws.dispatch.release(workspaceExecutionKey)
		_, _ = continuation(dispatchCtx)
	})
}

// guardSingleWorkspaceExecution enforces "one press, one execution" on the
// workspace, with the same two halves the spec guard has and for the same
// reasons: the in-memory reservation catches a double click or two tabs racing
// on this process, the persisted record catches a viewer restarted while an
// execution was still open.
func (s *Server) guardSingleWorkspaceExecution(ctx context.Context, ws *workspaceSession) error {
	existingID, reserved := ws.dispatch.reserve(workspaceExecutionKey)
	if !reserved {
		message := "an execution is already running for this workspace"
		if existingID != "" {
			message = "execution " + existingID + " is already running for this workspace"
		}
		return iox.NewConflict(message, "wait for it to finish before starting another one", nil)
	}
	records, err := ws.store.ListBySpec(ctx, workspaceExecutionKey)
	if err != nil {
		ws.dispatch.release(workspaceExecutionKey)
		return iox.NewInternal("reading the executions of this workspace", err)
	}
	if len(records) > 0 && records[0].Status == execution.StatusRunning {
		ws.dispatch.release(workspaceExecutionKey)
		return iox.NewConflict(
			"execution "+records[0].ID+" is already running for this workspace",
			"wait for it to finish, or remove its record under .archetipo/executions/ if it was left behind by an interrupted run",
			nil,
		)
	}
	return nil
}

// workspaceAvailability answers "can this workspace run this workspace action
// now?" once per request. It is the spec-scoped actionAvailability with the
// spec replaced by the two facts a workspace action depends on instead: whether
// a workspace execution is running, and whether a PRD is already there.
type workspaceAvailability struct {
	providerAvailability
	runningID           string
	workspaceHasRunning bool
	hasPRD              bool
	// prdUnreadable is why the workspace cannot say whether it has a PRD: a
	// connector that does not expose one, or one that failed to read it. It
	// makes the action unavailable with a reason rather than failing the whole
	// request, because not knowing is not the same as a broken viewer.
	prdUnreadable string
	hasBacklog    bool
	// specCodes are the spec codes the backlog holds at the moment this
	// availability was computed. They come from the very read that answered
	// hasBacklog, so there is no second read and no window between the two, and
	// they are what a spec drafting run is later confirmed against: the codes
	// that existed before it started.
	specCodes []string
	// backlogUnreadable is why the workspace cannot say whether it has a
	// backlog. "There is no backlog here" is *not* one of those reasons: the
	// connector reports it as a missing precondition, which is an answer — the
	// very answer the backlog action exists for — and reading it as a failure
	// would hide the action from exactly the workspace it targets.
	backlogUnreadable string
}

func (s *Server) workspaceAvailability(ctx context.Context, ws *workspaceSession) workspaceAvailability {
	availability := workspaceAvailability{}
	if id, busy := ws.dispatch.current(workspaceExecutionKey); busy {
		availability.workspaceHasRunning = true
		availability.runningID = id
	}
	if !availability.workspaceHasRunning {
		if records, err := ws.store.ListBySpec(ctx, workspaceExecutionKey); err == nil && len(records) > 0 && records[0].Status == execution.StatusRunning {
			availability.workspaceHasRunning = true
			availability.runningID = records[0].ID
		}
	}
	reader, ok := ws.conn.(connector.PRDReader)
	if !ok {
		availability.prdUnreadable = "this connector does not expose a PRD"
	} else if body, err := reader.ReadPRD(ctx); err != nil {
		availability.prdUnreadable = "the PRD could not be read: " + err.Error()
	} else {
		availability.hasPRD = strings.TrimSpace(body) != ""
	}
	// ReadExistingBacklog belongs to the base connector interface, so there is
	// no capability to assert here. A missing backlog comes back as a missing
	// precondition and is read as "no backlog yet", exactly as backlogEffect
	// reads it in execution/effect.go; only anything else is a workspace that
	// cannot answer.
	if summary, err := ws.conn.ReadExistingBacklog(ctx); err != nil {
		var coded *iox.CodedError
		if errors.As(err, &coded) && coded.Code == iox.CodePreconditionMissing {
			availability.hasBacklog = false
		} else {
			availability.backlogUnreadable = "the backlog could not be read: " + err.Error()
		}
	} else {
		availability.hasBacklog = len(summary.Codes) > 0
		availability.specCodes = append([]string(nil), summary.Codes...)
	}
	availability.providerAvailability = s.providerAvailabilityFor(ctx, ws)
	return availability
}

// reasonFor returns the reason the workspace action cannot be started, or ""
// when it can. The order is the order in which the facts matter: an execution
// already running names itself before anything else, because it is the one
// refusal a second press must be able to read; only then does the state of the
// workspace speak, through offers, ruling out a *first* inception over an
// existing PRD (AC-5) or a *first* backlog over an existing one; the provider
// half is the same the spec route renders.
func (a workspaceAvailability) reasonFor(actionID string) string {
	capability, err := execution.RequiredCapability(execution.ActionID(actionID))
	if err != nil {
		return "this action cannot be started from the viewer yet"
	}
	if a.workspaceHasRunning {
		if a.runningID != "" {
			return "execution " + a.runningID + " is already running for this workspace"
		}
		return "an execution is already running for this workspace"
	}
	if reason := a.offers(actionID); reason != "" {
		return reason
	}
	return a.providerAvailability.reasonFor(capability)
}

// offers isolates the half of the decision that depends on the *state of the
// workspace* alone, with no provider in it: it returns why this workspace does
// not admit the action, or "" when it does. It is what the payload publishes as
// offered, and it is deliberately separate from reasonFor so that "this
// workspace is past this action" and "this workspace could run it but the
// provider cannot" stay two different answers for the client.
func (a workspaceAvailability) offers(actionID string) string {
	switch execution.ActionID(actionID) {
	case execution.ActionInception:
		if a.prdUnreadable != "" {
			return a.prdUnreadable
		}
		if a.hasPRD {
			return "this workspace already has a PRD: a first inception would overwrite it"
		}
		return ""
	case execution.ActionBacklog:
		// The PRD comes first because it is the input of the generation: a
		// workspace that has none cannot produce a backlog at all, and saying
		// so is more useful than any answer about the backlog itself.
		if a.prdUnreadable != "" {
			return a.prdUnreadable
		}
		if !a.hasPRD {
			return "this workspace has no PRD yet: generate it with a first inception before the backlog"
		}
		if a.backlogUnreadable != "" {
			return a.backlogUnreadable
		}
		if a.hasBacklog {
			return "this workspace already has a backlog: the initial generation would replace it"
		}
		return ""
	case execution.ActionSpecDraft:
		// The mirror image of the backlog generation: that action is offered
		// only to a workspace without a backlog, this one only to a workspace
		// with one, because a spec is filed under an epic the backlog declares.
		//
		// The PRD is deliberately not consulted. A backlog exists only after a
		// PRD, so the condition below already implies it, and a second check
		// would produce two different sentences about one and the same fact.
		if a.backlogUnreadable != "" {
			return a.backlogUnreadable
		}
		if !a.hasBacklog {
			return "this workspace has no backlog yet: generate it before adding a spec"
		}
		return ""
	default:
		return "this action cannot be started from the viewer yet"
	}
}

// workspaceRemedy picks the remedy that matches the refusal, so a 409 tells the
// caller what to do rather than only what stopped it. It takes the action
// because the same fact means two different things depending on it: an existing
// PRD is what stops an inception, while a *missing* PRD is what stops a backlog
// generation, and each deserves its own next step.
func workspaceRemedy(a workspaceAvailability, actionID string) string {
	if a.workspaceHasRunning {
		return "wait for it to finish before starting another one"
	}
	if a.prdUnreadable != "" {
		return "use a connector that exposes the PRD"
	}
	// Each action answers only for the facts that can stop *it*. The switch is
	// exhaustive on purpose: an action falling through to another one's remedy
	// is how a workspace stopped by an unusable provider ends up being told to
	// edit its PRD.
	switch execution.ActionID(actionID) {
	case execution.ActionSpecDraft:
		switch {
		case a.backlogUnreadable != "":
			return "use a connector that exposes the backlog"
		case !a.hasBacklog:
			return "generate the initial backlog first, then come back to add a spec"
		}
	case execution.ActionBacklog:
		switch {
		case !a.hasPRD:
			return "run a first inception to obtain the PRD before generating the backlog"
		case a.backlogUnreadable != "":
			return "use a connector that exposes the backlog"
		case a.hasBacklog:
			return "add specs with archetipo spec add instead, or remove the existing backlog deliberately before generating it again"
		}
	case execution.ActionInception:
		if a.hasPRD {
			return "edit the existing PRD instead, or remove it deliberately before running a first inception"
		}
	}
	// Nothing about the state of the workspace stops this action, so what is
	// left is the provider.
	return "fix the default provider in the Execution panel of the configuration"
}

// prdDiscarder returns the connector as the workspace's own undo, or nil when
// it cannot take a PRD back. nil is a valid answer: skipping the rollback is
// not a failure, and DiscardPartialPRD treats it as a no-op.
func (s *Server) prdDiscarder(ws *workspaceSession) execution.PRDDiscarder {
	discarder, ok := ws.conn.(connector.PRDDiscarder)
	if !ok {
		return nil
	}
	return discarder
}

// backlogDiscarder is prdDiscarder's twin: it returns the connector as the
// workspace's own undo for the backlog, or nil when it cannot take one back.
// nil is a valid answer — skipping the rollback is not a failure, and
// DiscardPartialBacklog treats it as a no-op.
func (s *Server) backlogDiscarder(ws *workspaceSession) execution.BacklogDiscarder {
	discarder, ok := ws.conn.(connector.BacklogDiscarder)
	if !ok {
		return nil
	}
	return discarder
}

// specDeleter returns the connector as the undo of a spec drafting run, or nil
// when it cannot take a spec back. It is the twin of backlogDiscarder, and nil
// is a valid answer for the same reason: skipping the rollback is not a
// failure, and ConfirmSpecDraft treats it as a no-op.
func (s *Server) specDeleter(ws *workspaceSession) execution.SpecDeleter {
	deleter, ok := ws.conn.(connector.SpecDeleter)
	if !ok {
		return nil
	}
	return deleter
}

func admitsWorkspaceAction(actions []template.WorkspaceAction, actionID string) bool {
	for _, action := range actions {
		if action.ID == actionID {
			return true
		}
	}
	return false
}

// workspaceActionNames lists the workspace action ids a process declares, for
// an error that has to tell the caller which names exist at all.
func workspaceActionNames(actions []template.WorkspaceAction) string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.ID)
	}
	return strings.Join(out, ", ")
}
