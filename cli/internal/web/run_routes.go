package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// runSnapshotView is the run itself, or null when the remote work has no run.
type runSnapshotView struct {
	RunID    string             `json:"run_id"`
	State    execution.RunState `json:"state"`
	Error    string             `json:"error,omitempty"`
	ClosedAt *string            `json:"closed_at,omitempty"`
}

// runView is what the browser reads. Events and Approvals are never nil, so a
// client always receives an array and never has to test for null before
// iterating.
type runView struct {
	Run       *runSnapshotView            `json:"run"`
	Events    []execution.RunEvent        `json:"events"`
	LastID    int64                       `json:"last_id"`
	Approvals []execution.PendingApproval `json:"approvals"`
	Connected bool                        `json:"connected"`
	Truncated bool                        `json:"truncated"`
	Notice    string                      `json:"notice,omitempty"`
}

// runTarget is everything a run route needs once the plumbing has been
// resolved: the projection to read, the collaborator to command, and the
// request that names the run.
type runTarget struct {
	follower     *runFollower
	collaborator execution.RunCollaborator
	request      execution.RunRequest
}

func emptyRunView(notice string) runView {
	return runView{
		Events:    []execution.RunEvent{},
		Approvals: []execution.PendingApproval{},
		Notice:    notice,
	}
}

func projectionView(projection runProjection) runView {
	view := runView{
		Events:    projection.Events,
		LastID:    projection.LastID,
		Approvals: projection.Approvals,
		Connected: projection.Connected,
		Truncated: projection.Truncated,
		Notice:    projection.Notice,
	}
	if view.Events == nil {
		view.Events = []execution.RunEvent{}
	}
	if view.Approvals == nil {
		view.Approvals = []execution.PendingApproval{}
	}
	if projection.Snapshot.RunID != "" {
		snapshot := &runSnapshotView{
			RunID: projection.Snapshot.RunID,
			State: projection.Snapshot.State,
			Error: projection.Snapshot.Error,
		}
		if projection.Snapshot.ClosedAt != nil {
			closedAt := projection.Snapshot.ClosedAt.UTC().Format("2006-01-02T15:04:05.000Z")
			snapshot.ClosedAt = &closedAt
		}
		view.Run = snapshot
	}
	return view
}

// resolveRunTarget walks from an execution id to a followable run.
//
// Every step that can fail has its own error, because they are different
// situations with different remedies: a record that does not exist, a workspace
// with no default provider, a provider that does not expose an interactive run.
// A remote task that has no run yet is not one of them — it is an answer, and
// the caller renders it as a 200 with no run.
func (s *Server) resolveRunTarget(ctx context.Context, ws *workspaceSession, executionID string) (runTarget, string, error) {
	record, err := ws.store.Get(ctx, executionID)
	if err != nil {
		var storeErr *execution.StoreError
		if errors.As(err, &storeErr) {
			switch storeErr.Kind {
			case execution.StoreNotFound:
				return runTarget{}, "", iox.NewNotFound("execution not found: "+executionID, "pass the id of an execution this workspace started", err)
			case execution.StoreInvalidID:
				return runTarget{}, "", iox.NewInvalidInput("invalid execution id", "use the id returned when the action was started", err)
			}
		}
		return runTarget{}, "", iox.NewInternal("reading execution "+executionID, err)
	}
	// The default is read from disk and not from the config the server booted
	// with, for the same reason the dispatch route does it: the Execution panel
	// can change it while the viewer runs.
	current, _, _, _, err := readConfigState(ws.cfg.ProjectRoot)
	if err != nil {
		return runTarget{}, "", err
	}
	selection := current.Execution.DefaultProvider
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		return runTarget{}, "", iox.NewConflict("execution.default_provider is not configured", "pick a default provider in the Execution panel of the configuration", nil)
	}
	providerID := strings.TrimSpace(selection.ID)
	if s.registry == nil {
		return runTarget{}, "", iox.NewConflict("no execution provider is registered in this viewer", "start the viewer from a build that registers execution providers", nil)
	}
	provider, err := s.registry.Resolve(providerID)
	if err != nil {
		return runTarget{}, "", iox.NewConflict("invalid execution.default_provider.id: "+providerID, "pick a registered provider in the Execution panel of the configuration", err)
	}
	collaborator, ok := execution.RunCollaboratorFor(provider)
	if !ok {
		return runTarget{}, "", iox.NewConflict(
			"the execution provider "+quoted(providerID)+" does not expose an interactive run",
			"this execution can be followed to its outcome, but it has no run to read or command",
			nil,
		)
	}
	providerConfig := execution.CloneConfig(selection.Config)
	runID, err := collaborator.ResolveRun(ctx, record, providerConfig)
	if err != nil {
		return runTarget{}, "", mapRunRefusal(err)
	}
	if strings.TrimSpace(runID) == "" {
		// Absence, not failure: the remote work exists and has not been handed to
		// a run yet.
		return runTarget{}, "the remote work has not been assigned a run yet", nil
	}
	follower := ws.followers.ensure(ctx, executionID, runID, providerConfig, collaborator)
	if follower == nil {
		// The workspace this run belongs to has been left: its followers were
		// closed with it, and starting a new one would open a stream towards the
		// hub that nothing in this process could ever close.
		return runTarget{}, "this run belongs to a workspace that is no longer open", nil
	}
	return runTarget{
		follower:     follower,
		collaborator: collaborator,
		request:      execution.RunRequest{RunID: runID, ProviderConfig: providerConfig},
	}, "", nil
}

// mapRunRefusal renders a refusal at the HTTP boundary.
//
// It branches on the reason only. The message of a provider error is a
// diagnostic, not a contract, and reading it here would make the status of this
// route depend on the wording of another package.
func mapRunRefusal(err error) error {
	reason, refused := execution.RefusalOf(err)
	if !refused {
		return iox.NewInternal("commanding the remote run", err)
	}
	switch reason {
	case execution.RunRefusedNotFound:
		return iox.NewNotFound(
			"the remote run is not visible to this workspace (not_found)",
			"check that the configured credential is still granted the workspace",
			err,
		)
	case execution.RunRefusedNotActive:
		return iox.NewConflict(
			"the remote run is no longer active (run_not_active)",
			"a run that has ended cannot be reopened; start a new execution instead",
			err,
		)
	case execution.RunRefusedRunnerOffline:
		return iox.NewConflict(
			"no runner is attached to the remote run (runner_offline)",
			"this is transient: wait for the runner to come back and try again",
			err,
		)
	case execution.RunRefusedUnauthorized:
		return iox.NewConflict(
			"the remote run refused the configured credential (unauthorized)",
			"check the token exported for the configured execution provider",
			err,
		)
	case execution.RunRefusedUnsupported:
		return iox.NewInvalidInput(
			"the remote run cannot accept this command (unsupported)",
			"check the command payload before sending it again",
			err,
		)
	default:
		return iox.NewConflict("the remote run refused the command ("+string(reason)+")", "", err)
	}
}

// parseAfterID reads the browser's cursor.
func parseAfterID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("after_id"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, iox.NewInvalidInput("invalid after_id: "+raw, "after_id must be a non-negative integer", err)
	}
	return value, nil
}

func executionIDOf(r *http.Request) (string, error) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return "", iox.NewInvalidInput("missing execution id", "use /api/execution/<id>/run", nil)
	}
	return id, nil
}

// handleGetExecutionRun serves the projection of the run behind an execution.
func (s *Server) handleGetExecutionRun(w http.ResponseWriter, r *http.Request) {
	id, err := executionIDOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ws := s.session()
	target, notice, err := s.resolveRunTarget(r.Context(), ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if target.follower == nil {
		writeJSON(w, http.StatusOK, emptyRunView(notice))
		return
	}
	writeJSON(w, http.StatusOK, projectionView(target.follower.snapshotView(afterID)))
}

type sendRunMessageReq struct {
	Message string `json:"message"`
}

// handleSendRunMessage delivers a message to the run.
//
// It appends nothing to the projection. A 202 from the hub means the message
// was delivered to the runner, not that it is part of the history: it becomes
// history when the hub republishes it on the event stream. Writing it locally
// would show the user a line the run may never have received — and it is also
// what makes a refused command free of consequences, since a command that wrote
// nothing has nothing to undo.
func (s *Server) handleSendRunMessage(w http.ResponseWriter, r *http.Request) {
	id, err := executionIDOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body sendRunMessageReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(w, iox.NewInvalidInput("message is required", "send a non-empty message", nil))
		return
	}
	ws := s.session()
	target, notice, err := s.resolveRunTarget(r.Context(), ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if target.follower == nil {
		writeError(w, iox.NewConflict("the execution has no run to talk to", notice, nil))
		return
	}
	if err := target.collaborator.SendRunMessage(r.Context(), target.request, body.Message); err != nil {
		writeError(w, mapRunRefusal(err))
		return
	}
	writeJSON(w, http.StatusAccepted, projectionView(target.follower.snapshotView(afterID)))
}

type respondRunApprovalReq struct {
	OptionID string `json:"option_id"`
}

// handleRespondRunApproval answers a pending approval and reports the outcome
// the provider gives back, re-read rather than assumed.
func (s *Server) handleRespondRunApproval(w http.ResponseWriter, r *http.Request) {
	id, err := executionIDOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	approvalID := strings.TrimSpace(r.PathValue("approvalId"))
	if approvalID == "" {
		writeError(w, iox.NewInvalidInput("missing approval id", "use /api/execution/<id>/run/approvals/<approvalId>", nil))
		return
	}
	var body respondRunApprovalReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(body.OptionID) == "" {
		writeError(w, iox.NewInvalidInput("option_id is required", "pick one of the options the approval declares", nil))
		return
	}
	ws := s.session()
	target, notice, err := s.resolveRunTarget(r.Context(), ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if target.follower == nil {
		writeError(w, iox.NewConflict("the execution has no run to answer", notice, nil))
		return
	}
	if err := target.collaborator.RespondRunApproval(r.Context(), target.request, approvalID, body.OptionID); err != nil {
		writeError(w, mapRunRefusal(err))
		return
	}
	// The outcome is what the provider reports afterwards, not what this handler
	// believes the answer did.
	target.follower.refresh(r.Context(), target.collaborator)
	writeJSON(w, http.StatusAccepted, projectionView(target.follower.snapshotView(afterID)))
}

// handleCancelRun asks the runner to close the session.
//
// It never sets a terminal state locally. The run is over when the hub says it
// is, and the response therefore carries whatever ReadRun reports right now —
// which may still be ACTIVE, because the cancel was delivered to the runner and
// not yet acted upon.
func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	id, err := executionIDOf(r)
	if err != nil {
		writeError(w, err)
		return
	}
	afterID, err := parseAfterID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	ws := s.session()
	target, notice, err := s.resolveRunTarget(r.Context(), ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if target.follower == nil {
		writeError(w, iox.NewConflict("the execution has no run to cancel", notice, nil))
		return
	}
	if err := target.collaborator.CancelRun(r.Context(), target.request); err != nil {
		writeError(w, mapRunRefusal(err))
		return
	}
	if snapshot, readErr := target.collaborator.ReadRun(r.Context(), target.request); readErr == nil {
		target.follower.applySnapshot(snapshot)
	}
	writeJSON(w, http.StatusAccepted, projectionView(target.follower.snapshotView(afterID)))
}
