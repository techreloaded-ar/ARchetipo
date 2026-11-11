package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// newTransitionServer builds a viewer over a real filefs connector holding one
// spec per board column, so a preview can be asked about any transition without
// each test having to invent its own backlog.
func newTransitionServer(t *testing.T, worktreeEnabled bool) (*Server, *filefs.Connector, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	cfg.Worktree.Enabled = worktreeEnabled
	conn := filefs.New(cfg)
	epic := domain.Epic{Code: "EP-999", Title: "Transizioni"}
	writeBacklogFixture(t, dir, []domain.Epic{epic}, []domain.Spec{
		{Code: "US-901", Title: "In todo", Epic: epic, Priority: domain.PriorityLow, Points: 1, Status: domain.StatusTodo},
		{Code: "US-902", Title: "In review", Epic: epic, Priority: domain.PriorityLow, Points: 1, Status: domain.StatusReview},
		{Code: "US-903", Title: "In done", Epic: epic, Priority: domain.PriorityLow, Points: 1, Status: domain.StatusDone},
	})
	srv, err := NewServer(conn, cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv, conn, dir
}

func previewOf(t *testing.T, srv *Server, code, to string) transitionPreview {
	t.Helper()
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/spec/"+code+"/transition-preview?to="+to, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("preview %s->%s: got %d, body=%s", code, to, w.Code, w.Body.String())
	}
	var preview transitionPreview
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v (body=%s)", err, w.Body.String())
	}
	return preview
}

func hasImpact(preview transitionPreview, code string) bool {
	for _, c := range preview.Impacts {
		if c == code {
			return true
		}
	}
	return false
}

func seedRunningExecution(t *testing.T, srv *Server, code string) string {
	t.Helper()
	id, err := execution.RandomID()
	if err != nil {
		t.Fatal(err)
	}
	record := execution.Execution{
		ID:         id,
		SpecCode:   code,
		Action:     execution.ActionPlan,
		Capability: execution.CapabilitySpecPlan,
		ProviderID: "fake",
		Status:     execution.StatusRunning,
		CreatedAt:  time.Now().UTC(),
	}
	if err := srv.session().store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTransitionPreviewInvalidTarget(t *testing.T) {
	srv, _, _ := newTransitionServer(t, false)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/spec/US-901/transition-preview?to=archived", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestTransitionPreviewReviewToDoneRecommendsApprove(t *testing.T) {
	srv, _, _ := newTransitionServer(t, false)
	preview := previewOf(t, srv, "US-902", "done")
	if preview.From != "review" || preview.To != "done" {
		t.Fatalf("edges: got %s->%s", preview.From, preview.To)
	}
	if !preview.Allowed || preview.RecommendedAction != "approve" {
		t.Fatalf("expected an allowed approve route, got %+v", preview)
	}
	// Closing through review is the intended road: nothing is skipped.
	if hasImpact(preview, "skips_review") {
		t.Fatalf("review->done must not warn about skipping review: %+v", preview.Impacts)
	}
}

func TestTransitionPreviewReopeningDone(t *testing.T) {
	srv, _, _ := newTransitionServer(t, false)
	preview := previewOf(t, srv, "US-903", "todo")
	if !hasImpact(preview, "reopen_done") {
		t.Fatalf("expected reopen_done, got %+v", preview.Impacts)
	}
	// A spec coming back from DONE is already described by reopen_done.
	if hasImpact(preview, "backward_move") {
		t.Fatalf("reopening must not also report backward_move: %+v", preview.Impacts)
	}
}

func TestTransitionPreviewTodoToDoneSkipsReviewAndLeavesBranch(t *testing.T) {
	srv, conn, _ := newTransitionServer(t, true)
	branch := "archetipo/US-901"
	worktree := ".archetipo/worktrees/US-901"
	if _, err := conn.UpdateSpec(context.Background(), "US-901", domain.SpecUpdate{Branch: &branch, Worktree: &worktree}); err != nil {
		t.Fatal(err)
	}
	preview := previewOf(t, srv, "US-901", "done")
	if !hasImpact(preview, "skips_review") || !hasImpact(preview, "branch_left_open") {
		t.Fatalf("expected skips_review and branch_left_open, got %+v", preview.Impacts)
	}
}

func TestTransitionPreviewPlannedWithoutPlan(t *testing.T) {
	srv, conn, _ := newTransitionServer(t, false)
	preview := previewOf(t, srv, "US-901", "planned")
	if !hasImpact(preview, "planned_without_plan") {
		t.Fatalf("expected planned_without_plan, got %+v", preview.Impacts)
	}
	if _, err := conn.SavePlan(context.Background(), "US-901", domain.PlanInput{
		PlanBody: "## Piano",
		Tasks:    []domain.Task{{ID: "TASK-01", Title: "Fare", Type: domain.TaskImpl}},
	}); err != nil {
		t.Fatal(err)
	}
	if hasImpact(previewOf(t, srv, "US-901", "planned"), "planned_without_plan") {
		t.Fatalf("a spec with a saved plan must not warn about a missing one")
	}
}

func TestTransitionPreviewManualInProgressAndRework(t *testing.T) {
	srv, conn, _ := newTransitionServer(t, false)
	rework := true
	if _, err := conn.UpdateSpec(context.Background(), "US-901", domain.SpecUpdate{Rework: &rework}); err != nil {
		t.Fatal(err)
	}
	preview := previewOf(t, srv, "US-901", "in_progress")
	if !hasImpact(preview, "manual_in_progress") || !hasImpact(preview, "rework_stuck") {
		t.Fatalf("expected manual_in_progress and rework_stuck, got %+v", preview.Impacts)
	}
	if hasImpact(previewOf(t, srv, "US-901", "todo"), "rework_stuck") {
		t.Fatalf("a move back to todo is where rework is cleared, no warning expected")
	}
}

func TestTransitionPreviewRunningExecutionBlocks(t *testing.T) {
	srv, _, _ := newTransitionServer(t, false)
	seedRunningExecution(t, srv, "US-901")
	preview := previewOf(t, srv, "US-901", "done")
	if preview.Allowed || preview.BlockedCode != blockedCodeRunningExecution {
		t.Fatalf("expected a hard block, got %+v", preview)
	}
}

func TestTransitionPreviewReviewToTodo(t *testing.T) {
	srv, conn, _ := newTransitionServer(t, false)
	var rs connector.ReviewStore = conn
	ctx := context.Background()
	// Only a dossier: nothing to convert, so the raw move would orphan it.
	if err := rs.SaveReview(ctx, "US-902", domain.Review{Dossier: &domain.ReviewDossier{Summary: "pronta"}}); err != nil {
		t.Fatal(err)
	}
	preview := previewOf(t, srv, "US-902", "todo")
	if preview.RecommendedAction != "" || !hasImpact(preview, "review_dangling") {
		t.Fatalf("expected no route and review_dangling, got %+v", preview)
	}

	// With comments there is a road that carries them into the spec body.
	if err := rs.SaveReview(ctx, "US-902", domain.Review{
		Comments: []domain.ReviewComment{{File: "a.go", Line: 1, Side: "new", Body: "da sistemare"}},
		Dossier:  &domain.ReviewDossier{Summary: "pronta"},
	}); err != nil {
		t.Fatal(err)
	}
	preview = previewOf(t, srv, "US-902", "todo")
	if preview.RecommendedAction != "request_changes" {
		t.Fatalf("expected request_changes, got %+v", preview)
	}
	if hasImpact(preview, "review_dangling") {
		t.Fatalf("a routed change leaves nothing dangling: %+v", preview.Impacts)
	}
}

func TestMoveCardRefusesRunningExecution(t *testing.T) {
	srv, _, _ := newTransitionServer(t, false)
	seedRunningExecution(t, srv, "US-901")
	body := `{"code":"US-901","to":"done"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/board/move", strings.NewReader(body))
	srv.mux.ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want 409, body=%s", w.Code, w.Body.String())
	}
}

func TestMoveCardToDoneSweepsTemporaryArtifacts(t *testing.T) {
	srv, _, dir := newTransitionServer(t, false)
	tmpRoot := filepath.Join(dir, ".archetipo", "tmp")
	staging := filepath.Join(tmpRoot, "plan-US-901")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(tmpRoot, "payload-US-901-plan.json")
	if err := os.WriteFile(payload, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	survivor := filepath.Join(tmpRoot, "payload-US-901-US-915.json")
	if err := os.WriteFile(survivor, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/board/move", strings.NewReader(`{"code":"US-901","to":"done"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	for _, gone := range []string{staging, payload} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be swept, stat err=%v", gone, err)
		}
	}
	if _, err := os.Stat(survivor); err != nil {
		t.Fatalf("a batch payload must survive one spec's sweep: %v", err)
	}
}
