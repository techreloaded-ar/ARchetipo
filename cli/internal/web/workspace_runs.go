package web

import (
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// workspacePendingView names the decision a run is waiting on. It carries the
// id and the title and nothing else: the rail says *that* something is being
// asked, the run panel is where it is answered.
type workspacePendingView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// workspaceRunView is one non-terminal execution of the open workspace, as the
// rail reads it.
//
// AwaitingResponse is a plain boolean and not a derivation the browser is asked
// to make, because the browser cannot make it: pending approvals exist only
// while a follower is attached, and attaching it is this route's job.
type workspaceRunView struct {
	ID       string             `json:"id"`
	Scope    execution.Scope    `json:"scope"`
	SpecCode string             `json:"spec_code"`
	Action   execution.ActionID `json:"action"`
	// Status travels exactly as the record declares it. No process rule is
	// applied here: the route reports what was written, it does not interpret
	// it.
	Status           execution.ExecutionStatus `json:"status"`
	CreatedAt        string                    `json:"created_at"`
	AwaitingResponse bool                      `json:"awaiting_response"`
	Pending          *workspacePendingView     `json:"pending,omitempty"`
	// Notice is why this entry could not be resolved to a run. It is present
	// exactly when the answer to "is this run waiting for me?" is unknown
	// rather than no, so a client never reads a false AwaitingResponse as a
	// confident "nothing to decide".
	Notice string `json:"notice,omitempty"`
	// ConversationID and AnchorEventID say which conversation asked for this
	// run and at which point of its history. The id is carried and not implied
	// by the workspace: a workspace holds several live conversations, so "the
	// one that asked for this run" is a different answer for every row, and the
	// rail can only lead a person back to it by being told which. They are
	// omitempty because a run started from the board was born in no conversation
	// at all, and an empty id travelling anyway would be a promise of navigation
	// that cannot be kept.
	ConversationID string `json:"conversation_id,omitempty"`
	AnchorEventID  int64  `json:"anchor_event_id,omitempty"`
}

// workspaceRunsView is never nil in Runs: a client always iterates an array.
type workspaceRunsView struct {
	Runs []workspaceRunView `json:"runs"`
}

// runScopeOf derives the scope from the action, and falls back to the shape of
// the record when the action is one this build does not know. An unknown action
// is not a reason to drop the run from the list: the person still has work in
// flight, and the rail must still show it.
func runScopeOf(record execution.Execution) execution.Scope {
	if scope, err := execution.ActionScope(record.Action); err == nil {
		return scope
	}
	if record.SpecCode == "" {
		return execution.ScopeWorkspace
	}
	return execution.ScopeSpec
}

// handleGetWorkspaceRuns lists the executions of the open workspace that have
// not ended, and says which of them is waiting for a human answer.
//
// It attaches the follower itself, for every listed run, instead of reporting
// only what some already-open panel happens to know. That is the whole point of
// the route: a person must be able to learn that a run is blocked on a decision
// without having opened that run first.
func (s *Server) handleGetWorkspaceRuns(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	records, err := ws.store.List(ctx)
	if err != nil {
		writeError(w, iox.NewInternal("listing the executions of the workspace", err))
		return
	}
	views := make([]workspaceRunView, 0, len(records))
	for _, record := range records {
		if record.Status != execution.StatusRunning {
			continue
		}
		view := workspaceRunView{
			ID:        record.ID,
			Scope:     runScopeOf(record),
			SpecCode:  record.SpecCode,
			Action:    record.Action,
			Status:    record.Status,
			CreatedAt: record.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		}
		// The anchor is looked up across every live conversation, and it brings
		// back the id of the one that holds it: with several alive, "the
		// conversation of this workspace" identifies nothing, so a rail that
		// navigated to it would be navigating to whichever one it happened to
		// read.
		if conversationID, anchor, ok := ws.conversation.anchorOf(record.ID); ok {
			view.ConversationID = conversationID
			view.AnchorEventID = anchor
		}
		target, notice, resolveErr := s.resolveRunTarget(ctx, ws, record.ID)
		switch {
		case resolveErr != nil:
			// A run that could not be asked is reported, not hidden and not
			// turned into an HTTP error: one unreachable run must not cost the
			// person the sight of all the others.
			view.Notice = resolveErr.Error()
		case target.follower == nil:
			// No run assigned yet: there is nothing to be waiting on.
			view.Notice = notice
		default:
			// Cursor 0 on purpose: this route reads the projection, it does not
			// advance it, and consuming events here would starve the panel that
			// is following the same run.
			projection := target.follower.snapshotView(0)
			if len(projection.Approvals) > 0 {
				pending := projection.Approvals[0]
				view.AwaitingResponse = true
				view.Pending = &workspacePendingView{ID: pending.ID, Title: pending.Title}
			}
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, workspaceRunsView{Runs: views})
}
