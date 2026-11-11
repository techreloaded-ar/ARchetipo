package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// blockedCodeRunningExecution is the single reason a status change is refused
// outright: a spec whose run is in flight would have the ground moved under the
// agent that is working on it.
const blockedCodeRunningExecution = "running_execution"

// transitionPreview answers "what happens if I move this spec there?" before
// anything is written. The server owns the answer because only it can see the
// branch, the worktree, the runs, the review and the plan; the browser only
// renders the codes it hands back, whose Italian wording lives in the viewer's
// TEXT table together with every other word a person reads.
type transitionPreview struct {
	From              string   `json:"from"`
	To                string   `json:"to"`
	Allowed           bool     `json:"allowed"`
	BlockedCode       string   `json:"blocked_code"`
	Impacts           []string `json:"impacts"`
	RecommendedAction string   `json:"recommended_action"`
}

// boardColumnIDForStatus is the inverse of boardLayout: the column a spec in
// this status is drawn in. Empty for a status no column shows.
func boardColumnIDForStatus(status domain.Status) string {
	for _, col := range boardLayout {
		if col.Status == status {
			return col.ID
		}
	}
	return ""
}

// boardColumnRank is the position of a column in the workflow, and -1 when the
// id names no column. It is what makes "backwards" a question with an answer.
func boardColumnRank(id string) int {
	for i, col := range boardLayout {
		if col.ID == id {
			return i
		}
	}
	return -1
}

// specRunningExecution answers "does this spec have a run in flight?" and names
// it. There is one answer to that question, so there is one place that gives
// it: the dispatch group knows about a run this process started, the store
// knows about one an earlier process left behind, and a caller that consulted
// only one of the two would call a busy spec idle.
func (s *Server) specRunningExecution(ctx context.Context, ws *workspaceSession, code string) (id, action string, running bool) {
	if id, busy := ws.dispatch.current(code); busy {
		act := ""
		if record, err := ws.store.Get(ctx, id); err == nil {
			act = string(record.Action)
		}
		return id, act, true
	}
	if records, err := ws.store.ListBySpec(ctx, code); err == nil && len(records) > 0 && records[0].Status == execution.StatusRunning {
		return records[0].ID, string(records[0].Action), true
	}
	return "", "", false
}

// transitionImpacts lists what a status change would leave behind, as codes.
//
// Nothing here refuses anything: every one of these is a legitimate thing to
// want, and a person who is told what it costs can decide to pay it. The only
// refusal in this feature is the running execution, and it is decided by the
// caller before this runs.
//
// routed says the change will travel through the flow that would have done the
// bookkeeping — approval or request-changes — so the review is not left behind.
func transitionImpacts(from, to string, spec domain.Spec, worktreeEnabled bool, review domain.Review, hasPlan, routed bool) []string {
	impacts := []string{}
	if to == "done" && from != "review" {
		impacts = append(impacts, "skips_review")
		if worktreeEnabled && spec.Branch != "" {
			impacts = append(impacts, "branch_left_open")
		}
	}
	if from == "review" && !routed && (len(review.Comments) > 0 || review.Dossier != nil) {
		impacts = append(impacts, "review_dangling")
	}
	if from == "done" {
		impacts = append(impacts, "reopen_done")
	}
	if spec.Rework && to != "todo" {
		impacts = append(impacts, "rework_stuck")
	}
	if to == "planned" && !hasPlan {
		impacts = append(impacts, "planned_without_plan")
	}
	if to == "in_progress" {
		impacts = append(impacts, "manual_in_progress")
	}
	// DONE and REVIEW have their own codes for going back; naming the move
	// twice would only say the same thing in two ways.
	if from != "done" && from != "review" && boardColumnRank(to) < boardColumnRank(from) {
		impacts = append(impacts, "backward_move")
	}
	return impacts
}

func (s *Server) handleTransitionPreview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if boardColumnRank(to) == -1 {
		writeError(w, iox.NewInvalidInput("invalid target column "+to, "valid columns: todo|planned|in_progress|review|done", nil))
		return
	}
	ctx := r.Context()
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	from := boardColumnIDForStatus(spec.Status)
	if _, _, running := s.specRunningExecution(ctx, ws, code); running {
		writeJSON(w, http.StatusOK, transitionPreview{
			From:        from,
			To:          to,
			Allowed:     false,
			BlockedCode: blockedCodeRunningExecution,
			Impacts:     []string{},
		})
		return
	}

	// The review is read only when leaving REVIEW, which is the only place it
	// can say anything: it decides both the flow to route through and whether
	// anything would be orphaned by not routing through it.
	var review domain.Review
	recommended := ""
	if from == "review" {
		if rs, ok := ws.conn.(connector.ReviewStore); ok {
			if r, err := rs.ReadReview(ctx, code); err == nil {
				review = r
			}
			switch {
			case to == "done":
				recommended = "approve"
			// request-changes refuses a review without comments, so offering it
			// there would be offering a road that ends in a 400.
			case to == "todo" && len(review.Comments) > 0:
				recommended = "request_changes"
			}
		}
	}

	hasPlan := false
	if to == "planned" {
		if planned, err := execution.HasPersistedPlan(ctx, ws.conn, code); err == nil {
			hasPlan = planned
		}
	}

	writeJSON(w, http.StatusOK, transitionPreview{
		From:              from,
		To:                to,
		Allowed:           true,
		Impacts:           transitionImpacts(from, to, spec, ws.cfg.Worktree.Enabled, review, hasPlan, recommended != ""),
		RecommendedAction: recommended,
	})
}
