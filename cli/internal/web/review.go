package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/gitwt"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

type diffView struct {
	Base   string           `json:"base"`
	Branch string           `json:"branch"`
	Ahead  int              `json:"ahead"`
	Behind int              `json:"behind"`
	Files  []gitwt.FileDiff `json:"files"`
}

// handleGetDiff returns the structured diff for a spec under review. When the
// spec has a recorded branch (worktree workflow) the diff is
// `git diff <fork_base>...<branch>`; otherwise it falls back to `git diff
// <base>` against the working tree, where base comes from ?base= or the
// configured worktree base.
func (s *Server) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "use /api/spec/US-XXX/diff", nil))
		return
	}
	ctx := r.Context()
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	root := ws.cfg.ProjectRoot
	if spec.Branch != "" {
		forkBase := spec.ForkBase
		if forkBase == "" {
			forkBase = ws.cfg.Worktree.Base
		}
		files, err := gitwt.Diff(ctx, root, forkBase, spec.Branch)
		if err != nil {
			writeError(w, err)
			return
		}
		ahead, behind, _ := gitwt.AheadBehind(ctx, root, ws.cfg.Worktree.Base, spec.Branch)
		writeJSON(w, http.StatusOK, diffView{Base: forkBase, Branch: spec.Branch, Ahead: ahead, Behind: behind, Files: files})
		return
	}
	base := strings.TrimSpace(r.URL.Query().Get("base"))
	if base == "" {
		base = ws.cfg.Worktree.Base
	}
	files, err := gitwt.DiffWorkingTree(ctx, root, base)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, diffView{Base: base, Files: files})
}

func (s *Server) handleGetReview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	rs, ok := ws.conn.(connector.ReviewStore)
	if !ok {
		writeJSON(w, http.StatusOK, domain.Review{Comments: []domain.ReviewComment{}})
		return
	}
	review, err := rs.ReadReview(r.Context(), code)
	if err != nil {
		writeError(w, err)
		return
	}
	if review.Comments == nil {
		review.Comments = []domain.ReviewComment{}
	}
	writeJSON(w, http.StatusOK, review)
}

func (s *Server) handleSaveReview(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	rs, ok := ws.conn.(connector.ReviewStore)
	if !ok {
		writeError(w, iox.NewConnector(iox.CodePreconditionMissing, "this connector does not persist review comments", "use the file connector", nil))
		return
	}
	var review domain.Review
	if err := decodeJSON(r, &review); err != nil {
		writeError(w, err)
		return
	}
	if err := rs.SaveReview(r.Context(), code, review); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

// handleRequestChanges writes into the spec body, as a "## Rework Feedback"
// section, everything the review already knows — the inline comments, the
// dossier's blockers and unmet criteria, and the free text the person adds —
// flags the spec as in rework, transitions it back to TODO, and clears the
// review. The feedback now lives inside the spec: the next archetipo-plan run
// reads it (inside the spec's worktree) and turns each item into a Fix task
// before re-planning.
//
// Assembling the items here rather than demanding them from the caller is what
// makes the viewer's gate the same gate as the archetipo-review skill's: a
// person who reads a dossier full of blockers should not have to retype them on
// the lines of the diff to reject the increment.
func (s *Server) handleRequestChanges(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	ctx := r.Context()
	rs, ok := ws.conn.(connector.ReviewStore)
	if !ok {
		writeError(w, iox.NewConnector(iox.CodePreconditionMissing, "this connector does not persist review comments", "use the file connector", nil))
		return
	}
	review, err := rs.ReadReview(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	// The body is optional: rejecting with nothing to add is legitimate when the
	// dossier and the comments already say what has to change.
	var payload struct {
		Feedback string `json:"feedback"`
	}
	_ = decodeJSON(r, &payload)
	items := domain.ReworkFeedbackItems(review, payload.Feedback)
	if len(items) == 0 {
		writeError(w, iox.NewInvalidInput("no feedback to send back", "write what must change, or add inline comments", nil))
		return
	}
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	body := domain.AppendReworkFeedback(spec.Body, items)
	rework := true
	if _, err := ws.conn.UpdateSpec(ctx, code, domain.SpecUpdate{Body: &body, Rework: &rework}); err != nil {
		writeError(w, err)
		return
	}
	if _, err := ws.conn.TransitionStatus(ctx, code, domain.StatusTodo); err != nil {
		writeError(w, err)
		return
	}
	// Clear the review, but keep the verdict: the feedback now lives in the spec
	// body, and the dossier goes with the comments because evidence that has
	// just been rejected no longer describes the increment. What survives is the
	// decision itself, named together with the execution that had prepared the
	// evidence it was taken on — the same trace an approval leaves, so a spec
	// that came back and one that was accepted are equally accountable.
	verdict := domain.ReviewVerdict{
		Decision:    domain.ReviewDecisionChangesRequested,
		DecidedAt:   time.Now().UTC().Format(time.RFC3339),
		ExecutionID: s.decidedExecutionID(ctx, ws, code, review),
	}
	if err := rs.SaveReview(ctx, code, domain.Review{Comments: []domain.ReviewComment{}, Verdict: &verdict}); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": len(items)})
}

// decidedExecutionID names the execution whose evidence a verdict was taken on.
//
// The dossier is the first source because it is the only one that is certainly
// right: it was written by the run that prepared it. The latest execution of the
// spec is the fallback for a spec decided without a prepared dossier, and an
// empty string is the honest answer when there is no run to name at all — a
// verdict pointing at the wrong execution would be worse than one pointing at
// none.
func (s *Server) decidedExecutionID(ctx context.Context, ws *workspaceSession, code string, review domain.Review) string {
	if review.Dossier != nil && strings.TrimSpace(review.Dossier.ExecutionID) != "" {
		return review.Dossier.ExecutionID
	}
	latest, err := s.latestExecution(ctx, ws, code)
	if err != nil || latest == nil {
		return ""
	}
	return latest.ID
}

// handleApprove is the human acceptance gate: the one entry point that closes a
// spec waiting under review, and the one no provider can reach.
//
// It exists as a route of its own rather than as a branch of handleIntegrate
// because integration is only half the story. handleIntegrate refuses outright
// when the worktree workflow is off, and in that configuration the only way to
// close a spec from the viewer was dragging its card onto DONE — which records
// no verdict at all. Approving must mean the same thing in both configurations,
// so the route decides internally whether closing the spec means integrating a
// branch or transitioning it.
//
// The order of the two writes is deliberate. The verdict is persisted *before*
// the transition: if the transition then fails, what is left is a spec still in
// review carrying a recorded decision, which a person can see and retry. The
// reverse order would leave a spec closed with no trace of who closed it, which
// is precisely the state AC-3 exists to make impossible.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "use /api/spec/US-XXX/approve", nil))
		return
	}
	ctx := r.Context()
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	// Nothing that is not waiting for a decision can be decided, and nothing can
	// be decided twice: a spec already closed answers the same way as one that
	// never reached review.
	if spec.Status != domain.StatusReview {
		writeError(w, iox.NewConflict(
			fmt.Sprintf("cannot approve %s: status is %s, expected %s", code, spec.Status, domain.StatusReview),
			"only a spec waiting under review can be approved", nil))
		return
	}
	rs, ok := ws.conn.(connector.ReviewStore)
	if !ok {
		// Approving without being able to record the verdict is refused rather
		// than performed silently: a decision nobody can trace back is not the
		// acceptance this gate is for.
		writeError(w, iox.NewConnector(iox.CodePreconditionMissing,
			"this connector does not persist review verdicts",
			"use the file connector, which stores them under .archetipo/reviews/", nil))
		return
	}
	review, err := rs.ReadReview(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	executionID := s.decidedExecutionID(ctx, ws, code, review)
	review.Verdict = &domain.ReviewVerdict{
		Decision:    domain.ReviewDecisionApproved,
		DecidedAt:   time.Now().UTC().Format(time.RFC3339),
		ExecutionID: executionID,
	}
	if err := rs.SaveReview(ctx, code, review); err != nil {
		writeError(w, err)
		return
	}
	integrated := ws.cfg.Worktree.Enabled && spec.Branch != ""
	if integrated {
		if err := s.integrateSpec(ctx, ws, code, spec); err != nil {
			writeError(w, err)
			return
		}
	} else {
		if _, err := ws.conn.TransitionStatus(ctx, code, domain.StatusDone); err != nil {
			writeError(w, err)
			return
		}
		sweepSpecTemporaryArtifacts(ws, code)
	}
	s.broker.Publish()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"status":       string(domain.StatusDone),
		"execution_id": executionID,
		"integrated":   integrated,
	})
}

// handleIntegrate merges the spec's branch into base, removes the worktree and
// branch, and transitions the spec to DONE. Mirrors `archetipo spec integrate`.
func (s *Server) handleIntegrate(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	ctx := r.Context()
	if !ws.cfg.Worktree.Enabled {
		writeError(w, iox.NewConflict("worktree workflow is disabled", "enable worktree.enabled in config.yaml", nil))
		return
	}
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.integrateSpec(ctx, ws, code, spec); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "merged_at": time.Now().UTC().Format(time.RFC3339)})
}

// integrateSpec merges the spec's branch into base, removes the worktree and
// the branch, clears the persisted worktree metadata and transitions the spec
// to DONE. It returns errors already rendered with iox and writes nothing to
// the response, so both the integration route and the approval gate can perform
// the very same sequence: a spec closed through one path and one closed through
// the other must end in the identical state, and a second copy of this sequence
// would be free to drift.
func (s *Server) integrateSpec(ctx context.Context, ws *workspaceSession, code string, spec domain.Spec) error {
	if err := gitwt.EnsureRepo(ctx, ws.cfg.ProjectRoot, ws.cfg.Worktree.Base); err != nil {
		return err
	}
	if spec.Branch == "" {
		return iox.NewPrecondition(fmt.Sprintf("spec %s has no worktree branch", code), "", nil)
	}
	allSpecs, err := ws.conn.FetchBacklogItems(ctx, "")
	if err != nil {
		return err
	}
	blockers, err := gitwt.UnintegratedBlockers(ctx, ws.cfg.ProjectRoot, ws.cfg.Worktree, spec, allSpecs)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return iox.NewConflict(fmt.Sprintf("unintegrated blockers: %s", strings.Join(blockers, ", ")), "integrate the blockers first", nil)
	}
	if err := gitwt.Integrate(ctx, ws.cfg.ProjectRoot, ws.cfg.Worktree, spec.Branch, spec.Worktree); err != nil {
		return err
	}
	// Clear persisted worktree metadata after a successful integrate.
	emptyStr := ""
	_, _ = ws.conn.UpdateSpec(ctx, code, domain.SpecUpdate{
		Branch:   &emptyStr,
		Worktree: &emptyStr,
		ForkBase: &emptyStr,
	}) // best-effort: ignore error, merge succeeded.
	if _, err := ws.conn.TransitionStatus(ctx, code, domain.StatusDone); err != nil {
		return err
	}
	sweepSpecTemporaryArtifacts(ws, code)
	return nil
}

// sweepSpecTemporaryArtifacts removes the staging leftovers of a spec that has
// just reached the end of its lifecycle. Every door the viewer opens onto DONE
// calls it, so a spec closed from the web leaves the workspace as clean as one
// closed from the CLI. A leftover that resists deletion is logged and not
// returned: the transition already succeeded, and undoing it over a stray file
// would be worse than the file.
func sweepSpecTemporaryArtifacts(ws *workspaceSession, code string) {
	if err := workspace.RemoveSpecTemporaryArtifacts(ws.cfg.ProjectRoot, code); err != nil {
		log.Printf("sweep temporary artifacts for %s: %v", code, err)
	}
}
