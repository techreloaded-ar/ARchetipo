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
	Template templateView          `json:"template"`
	HasPRD   bool                  `json:"has_prd"`
	Actions  []workspaceActionView `json:"actions"`
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
	tpl, err := s.resolveTemplate()
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	availability := s.workspaceAvailability(ctx)
	actions := make([]workspaceActionView, 0, len(tpl.WorkspaceActions))
	for _, action := range tpl.WorkspaceActions {
		reason := availability.reasonFor(action.ID)
		actions = append(actions, workspaceActionView{
			WorkspaceAction:   action,
			Runnable:          reason == "",
			UnavailableReason: reason,
		})
	}
	latest, err := s.latestExecution(ctx, workspaceExecutionKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspaceActionsView{
		Template:  templateView{ID: tpl.ID, Version: tpl.Version},
		HasPRD:    availability.hasPRD,
		Actions:   actions,
		Execution: latest,
	})
}

// handleRunWorkspaceAction serves POST /api/workspace/execution: it starts one
// action whose object is the workspace itself and answers before the provider
// has finished, exactly as the spec route does and for the same reason — an
// inception is a conversation, and a response that waited for it would hang for
// as long as the person keeps talking.
func (s *Server) handleRunWorkspaceAction(w http.ResponseWriter, r *http.Request) {
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
	tpl, err := s.resolveTemplate()
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
	availability := s.workspaceAvailability(ctx)
	// Every refusal the GET route renders next to a disabled action is the same
	// refusal here, so pressing an action the payload declared unavailable never
	// creates a record only to close it a moment later with this very sentence.
	if reason := availability.reasonFor(string(action)); reason != "" {
		writeError(w, iox.NewConflict(reason, workspaceRemedy(availability), nil))
		return
	}
	// existedBefore is captured before the run starts, and only here: it is what
	// separates a document this run wrote from one the workspace already had, so
	// the rollback of a failed inception can never touch a pre-existing PRD.
	existedBefore := availability.hasPRD
	providerID := availability.providerID
	providerConfig := execution.CloneConfig(availability.providerConfig)

	if err := s.guardSingleWorkspaceExecution(ctx); err != nil {
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
		_ = execution.VerifyActionEffect(confirmCtx, s.conn, outcome.Action, workspaceExecutionKey, outcome)
		execution.DiscardPartialPRD(confirmCtx, s.prdDiscarder(), existedBefore, outcome)
	}
	started, continuation, err := s.service.StartWorkspace(ctx, action, providerID, providerConfig, confirm)
	if err != nil {
		s.dispatch.release(workspaceExecutionKey)
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
	s.dispatch.claim(workspaceExecutionKey, started.ID)
	writeJSON(w, http.StatusCreated, started)

	s.dispatch.run(func(dispatchCtx context.Context) {
		// Deferred in this order so they unwind in the other one: the workspace
		// stops being busy first, and only then is every connected client told to
		// re-read. Publishing while the reservation still stands would send them
		// back an action still marked unavailable.
		defer s.broker.Publish()
		defer s.dispatch.release(workspaceExecutionKey)
		_, _ = continuation(dispatchCtx)
	})
}

// guardSingleWorkspaceExecution enforces "one press, one execution" on the
// workspace, with the same two halves the spec guard has and for the same
// reasons: the in-memory reservation catches a double click or two tabs racing
// on this process, the persisted record catches a viewer restarted while an
// execution was still open.
func (s *Server) guardSingleWorkspaceExecution(ctx context.Context) error {
	existingID, reserved := s.dispatch.reserve(workspaceExecutionKey)
	if !reserved {
		message := "an execution is already running for this workspace"
		if existingID != "" {
			message = "execution " + existingID + " is already running for this workspace"
		}
		return iox.NewConflict(message, "wait for it to finish before starting another one", nil)
	}
	records, err := s.store.ListBySpec(ctx, workspaceExecutionKey)
	if err != nil {
		s.dispatch.release(workspaceExecutionKey)
		return iox.NewInternal("reading the executions of this workspace", err)
	}
	if len(records) > 0 && records[0].Status == execution.StatusRunning {
		s.dispatch.release(workspaceExecutionKey)
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
}

func (s *Server) workspaceAvailability(ctx context.Context) workspaceAvailability {
	availability := workspaceAvailability{}
	if id, busy := s.dispatch.current(workspaceExecutionKey); busy {
		availability.workspaceHasRunning = true
		availability.runningID = id
	}
	if !availability.workspaceHasRunning {
		if records, err := s.store.ListBySpec(ctx, workspaceExecutionKey); err == nil && len(records) > 0 && records[0].Status == execution.StatusRunning {
			availability.workspaceHasRunning = true
			availability.runningID = records[0].ID
		}
	}
	reader, ok := s.conn.(connector.PRDReader)
	if !ok {
		availability.prdUnreadable = "this connector does not expose a PRD"
	} else if body, err := reader.ReadPRD(ctx); err != nil {
		availability.prdUnreadable = "the PRD could not be read: " + err.Error()
	} else {
		availability.hasPRD = strings.TrimSpace(body) != ""
	}
	availability.providerAvailability = s.providerAvailabilityFor(ctx)
	return availability
}

// reasonFor returns the reason the workspace action cannot be started, or ""
// when it can. The order is the order in which the facts matter: an execution
// already running names itself before anything else, because it is the one
// refusal a second press must be able to read; only then does a PRD that is
// already there rule out a *first* inception (AC-5); the provider half is the
// same the spec route renders.
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
	if a.prdUnreadable != "" {
		return a.prdUnreadable
	}
	if a.hasPRD {
		return "this workspace already has a PRD: a first inception would overwrite it"
	}
	return a.providerAvailability.reasonFor(capability)
}

// workspaceRemedy picks the remedy that matches the refusal, so a 409 tells the
// caller what to do rather than only what stopped it.
func workspaceRemedy(a workspaceAvailability) string {
	switch {
	case a.workspaceHasRunning:
		return "wait for it to finish before starting another one"
	case a.prdUnreadable != "":
		return "use a connector that exposes the PRD"
	case a.hasPRD:
		return "edit the existing PRD instead, or remove it deliberately before running a first inception"
	default:
		return "fix the default provider in the Execution panel of the configuration"
	}
}

// prdDiscarder returns the connector as the workspace's own undo, or nil when
// it cannot take a PRD back. nil is a valid answer: skipping the rollback is
// not a failure, and DiscardPartialPRD treats it as a no-op.
func (s *Server) prdDiscarder() execution.PRDDiscarder {
	discarder, ok := s.conn.(connector.PRDDiscarder)
	if !ok {
		return nil
	}
	return discarder
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
