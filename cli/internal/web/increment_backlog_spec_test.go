package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
)

// seedIncrementBehindSpec writes the record of an increment whose publication
// left code in the backlog, as the increment store itself would read it, and
// names the increment in the spec's body, as publication does: that line is
// what makes the spec this increment's own.
func seedIncrementBehindSpec(t *testing.T, projectRoot, code, status string) {
	t.Helper()
	const id = "inc-backlog-spec"
	seedIncrementNamingSpec(t, projectRoot, id, code, status)
	cfg := config.Default()
	cfg.ProjectRoot = projectRoot
	body := "Approved behavior\n\n---\n\nIncremento `" + id + "`\n"
	if _, err := filefs.New(cfg).UpdateSpec(context.Background(), code, domain.SpecUpdate{Body: &body}); err != nil {
		t.Fatal(err)
	}
}

// seedIncrementNamingSpec writes only the record: an increment that names code
// as its backlog spec, whatever the spec carrying that code says today.
func seedIncrementNamingSpec(t *testing.T, projectRoot, id, code, status string) {
	t.Helper()
	root := filepath.Join(projectRoot, ".archetipo", "increments")
	record := increment.Record{
		ID:              id,
		Title:           "Export customers",
		Status:          status,
		CreatedAt:       time.Now().UTC(),
		Workdir:         filepath.Join(root, "worktrees", id),
		Branch:          "archetipo/increments/" + id,
		BacklogSpecCode: code,
		PR:              &increment.PullRequest{URL: "https://github.com/example/product/pull/1", Number: 1, State: "OPEN"},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, id+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func specDetailOf(t *testing.T, srv *Server, code string) specDetailView {
	t.Helper()
	response := doJSON(t, srv, http.MethodGet, "/api/spec/"+code, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("detail %s: %d %s", code, response.Code, response.Body.String())
	}
	var detail specDetailView
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return detail
}

func TestSpecPublishedByAnIncrementIsClosedOnlyByItsMerge(t *testing.T) {
	srv, conn, dir := newTransitionServer(t, false)
	seedIncrementBehindSpec(t, dir, "US-902", increment.Proposed)
	ctx := context.Background()

	detail := specDetailOf(t, srv, "US-902")
	if detail.ClassicReview || len(detail.Actions) != 0 {
		t.Fatalf("the classic review was offered on an increment's spec: review=%v actions=%+v", detail.ClassicReview, detail.Actions)
	}
	if len(detail.Works) != 1 || detail.Works[0].Note == "" || detail.Works[0].PRURL == "" {
		t.Fatalf("the spec does not lead back to its increment: %+v", detail.Works)
	}
	if ordinary := specDetailOf(t, srv, "US-901"); ordinary.ClassicReview || len(ordinary.Works) != 0 {
		t.Fatalf("a story outside review gained a review or a work: %+v", ordinary)
	}

	blocked := previewOf(t, srv, "US-902", "done")
	if blocked.Allowed || blocked.BlockedCode != blockedCodeClosedByIncrement || blocked.BlockedMessage == "" {
		t.Fatalf("DONE was offered by hand: %+v", blocked)
	}
	if back := previewOf(t, srv, "US-902", "todo"); !back.Allowed || back.RecommendedAction != "" {
		t.Fatalf("a plain move was refused or routed through the classic review: %+v", back)
	}

	for _, route := range []string{"/api/spec/US-902/approve", "/api/spec/US-902/request-changes"} {
		if status, body := postJSON(t, srv, route, nil); status != http.StatusConflict {
			t.Fatalf("%s: %d %v", route, status, body)
		}
	}
	if status, body := postJSON(t, srv, "/api/board/move", map[string]any{"code": "US-902", "to": "done"}); status != http.StatusConflict {
		t.Fatalf("card dragged onto DONE: %d %v", status, body)
	}
	if spec, err := conn.ReadSpecDetail(ctx, "US-902"); err != nil || spec.Status != domain.StatusReview {
		t.Fatalf("a refused step changed the spec: %+v %v", spec, err)
	}
	if status, body := postJSON(t, srv, "/api/board/move", map[string]any{"code": "US-902", "to": "todo"}); status != http.StatusOK {
		t.Fatalf("project management move refused: %d %v", status, body)
	}
}

// The code of a removed backlog spec is given again to the next story written,
// while the record of the increment that published the removed one keeps naming
// it. That older increment is not a work of the story that carries the code
// today: it is not listed, its PR is not offered, and it does not take the
// classic review away.
func TestIncrementNamingAReassignedCodeIsNotAWorkOfTheStoryThatCarriesItNow(t *testing.T) {
	srv, conn, dir := newTransitionServer(t, false)
	seedIncrementNamingSpec(t, dir, "inc-removed-earlier", "US-902", increment.Proposed)

	detail := specDetailOf(t, srv, "US-902")
	if len(detail.Works) != 0 || !detail.ClassicReview {
		t.Fatalf("an increment that never published this story is shown behind it: works=%+v review=%v", detail.Works, detail.ClassicReview)
	}
	if blocked := previewOf(t, srv, "US-902", "done"); blocked.BlockedCode == blockedCodeClosedByIncrement {
		t.Fatalf("DONE refused in the name of somebody else's increment: %+v", blocked)
	}

	// The increment that did publish the story is still its one work.
	seedIncrementBehindSpec(t, dir, "US-902", increment.Proposed)
	detail = specDetailOf(t, srv, "US-902")
	if len(detail.Works) != 1 || detail.Works[0].ID != "inc-backlog-spec" || detail.ClassicReview {
		t.Fatalf("the story does not lead back to its own increment alone: works=%+v review=%v", detail.Works, detail.ClassicReview)
	}
	if spec, err := conn.ReadSpecDetail(context.Background(), "US-902"); err != nil || spec.Status != domain.StatusReview {
		t.Fatalf("reading the detail changed the spec: %+v %v", spec, err)
	}
}

// A PR closed on GitHub without a merge is said in the server's own words, and
// the removal of the increment stands beside the refresh as the way out.
func TestIncrementWhosePRWasClosedWithoutMergeSaysSoAndOffersItsRemoval(t *testing.T) {
	srv, _, dir := newTransitionServer(t, false)
	record := increment.Record{
		ID:              "inc-closed-pr",
		Title:           "Export customers",
		Status:          increment.Proposed,
		DevelopmentMode: increment.Continuous,
		Workdir:         filepath.Join(dir, ".archetipo", "increments", "worktrees", "inc-closed-pr"),
		PR:              &increment.PullRequest{URL: "https://github.com/example/product/pull/9", Number: 9, State: "OPEN"},
	}
	removalOffered := func(view incrementView) bool {
		for _, step := range view.NextSteps {
			if step.Kind == "delete" {
				return step.Label != ""
			}
		}
		return false
	}
	waiting := srv.incrementView(context.Background(), srv.session(), record)
	if !strings.Contains(waiting.NextStepsNote, "in attesa del merge della PR") || removalOffered(waiting) {
		t.Fatalf("an open PR is not simply awaited: %+v", waiting)
	}
	record.PR.State = "CLOSED"
	closed := srv.incrementView(context.Background(), srv.session(), record)
	if closed.StatusLabel != "PR chiusa senza merge" || !strings.Contains(closed.NextStepsNote, "chiusa su GitHub senza merge") || strings.Contains(closed.NextStepsNote, "in attesa del merge") {
		t.Fatalf("the closed PR is still shown as awaiting its merge: label=%q note=%q", closed.StatusLabel, closed.NextStepsNote)
	}
	if !removalOffered(closed) {
		t.Fatalf("the removal is not offered beside the closed PR: %+v", closed.NextSteps)
	}
	refresh := false
	for _, action := range closed.Actions {
		refresh = refresh || action.ID == "refresh"
	}
	if !refresh {
		t.Fatalf("a reopened PR could not be read again: %+v", closed.Actions)
	}
}

func TestSpecOfADeliveredIncrementIsAnOrdinaryStoryAgain(t *testing.T) {
	srv, conn, dir := newTransitionServer(t, false)
	seedIncrementBehindSpec(t, dir, "US-902", increment.Delivered)

	// The person moved the closed spec back under review: from here on the
	// classic path applies, because no merge is left to close it.
	detail := specDetailOf(t, srv, "US-902")
	if !detail.ClassicReview || len(detail.Works) != 1 || detail.Works[0].Note == "" {
		t.Fatalf("delivered increment still owns the review: %+v", detail)
	}
	if status, body := postJSON(t, srv, "/api/spec/US-902/approve", nil); status != http.StatusOK {
		t.Fatalf("approve after delivery: %d %v", status, body)
	}
	if spec, err := conn.ReadSpecDetail(context.Background(), "US-902"); err != nil || spec.Status != domain.StatusDone {
		t.Fatalf("approval did not close the ordinary story: %+v %v", spec, err)
	}
}
