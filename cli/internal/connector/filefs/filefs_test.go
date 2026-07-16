package filefs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

func newTestConnector(t *testing.T) *Connector {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	cfg.File.Backlog = filepath.Join(dir, ".archetipo", "backlog.yaml")
	cfg.File.Planning = filepath.Join(dir, ".archetipo", "plans")
	cfg.Paths.PRD = filepath.Join(dir, "PRD.md")
	return New(cfg)
}

func TestSpecMarkerRoundTrip(t *testing.T) {
	s := domain.Spec{
		Code:      "US-007",
		Title:     "Login utente",
		Epic:      domain.Epic{Code: "EP-002", Title: "Auth Foundations"},
		Priority:  domain.PriorityHigh,
		Points:    5,
		Status:    domain.StatusPlanned,
		BlockedBy: []string{"US-002", "US-003"},
		Scope:     "MVP",
	}
	line := specMarker(s)
	mk, ok := parseMarker(line)
	if !ok {
		t.Fatalf("failed to parse generated marker: %s", line)
	}
	got, err := specFromMarker(mk)
	if err != nil {
		t.Fatal(err)
	}
	got.Title = s.Title // marker doesn't carry title
	if got.Code != s.Code || got.Priority != s.Priority || got.Points != s.Points || got.Status != s.Status || got.Scope != s.Scope {
		t.Errorf("structured fields differ: got=%+v want=%+v", got, s)
	}
	if got.Epic.Code != s.Epic.Code || got.Epic.Title != s.Epic.Title {
		t.Errorf("epic differs: got=%+v want=%+v", got.Epic, s.Epic)
	}
	if len(got.BlockedBy) != 2 || got.BlockedBy[0] != "US-002" || got.BlockedBy[1] != "US-003" {
		t.Errorf("blocked_by differs: %v", got.BlockedBy)
	}
}

func TestSpecFromMarkerRejectsMalformedCodes(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"garbage spec code", `<!-- archetipo:spec code=garbage epic=EP-001 priority=HIGH points=3 status=TODO -->`},
		{"missing spec code", `<!-- archetipo:spec epic=EP-001 priority=HIGH points=3 status=TODO -->`},
		{"garbage epic code", `<!-- archetipo:spec code=US-001 epic=nope priority=HIGH points=3 status=TODO -->`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mk, ok := parseMarker(tc.line)
			if !ok {
				t.Fatalf("failed to parse marker: %s", tc.line)
			}
			if _, err := specFromMarker(mk); err == nil {
				t.Fatalf("expected error for %s", tc.line)
			}
		})
	}
}

func TestRenderBacklogIsDeterministic(t *testing.T) {
	specs := []domain.Spec{
		{
			Code: "US-001", Title: "Setup",
			Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
			Priority: domain.PriorityHigh,
			Points:   3,
			Status:   domain.StatusTodo,
			Scope:    "MVP",
			Body:     "## Spec\n\nAs a user, I want X.\n",
		},
		{
			Code: "US-002", Title: "Auth",
			Epic:      domain.Epic{Code: "EP-001", Title: "Foundations"},
			Priority:  domain.PriorityMedium,
			Points:    5,
			Status:    domain.StatusTodo,
			BlockedBy: []string{"US-001"},
			Body:      "## Spec\n\nLogin.\n",
		},
	}
	a := renderBacklog(specs)
	b := renderBacklog(specs)
	if a != b {
		t.Fatalf("non-deterministic rendering")
	}
}

func TestRoundTripBacklog(t *testing.T) {
	specs := []domain.Spec{
		{
			Code: "US-001", Title: "Setup",
			Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
			Priority: domain.PriorityHigh,
			Points:   3,
			Status:   domain.StatusTodo,
			Scope:    "MVP",
			Body:     "## Spec\n\nAs a user, I want X.",
		},
		{
			Code: "US-002", Title: "Auth",
			Epic:      domain.Epic{Code: "EP-001", Title: "Foundations"},
			Priority:  domain.PriorityMedium,
			Points:    5,
			Status:    domain.StatusTodo,
			BlockedBy: []string{"US-001"},
			Body:      "## Spec\n\nLogin.",
		},
	}
	rendered := renderBacklog(specs)
	parsed, err := parseBacklog(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(parsed))
	}
	for i, want := range specs {
		got := parsed[i]
		if got.Code != want.Code || got.Title != want.Title {
			t.Errorf("spec[%d] head: got %s/%q want %s/%q", i, got.Code, got.Title, want.Code, want.Title)
		}
		if got.Priority != want.Priority || got.Points != want.Points || got.Status != want.Status {
			t.Errorf("spec[%d] fields: got=%+v want=%+v", i, got, want)
		}
		if strings.TrimSpace(got.Body) != strings.TrimSpace(want.Body) {
			t.Errorf("spec[%d] body mismatch: got=%q want=%q", i, got.Body, want.Body)
		}
	}
	// Round-trip: render again should produce the same bytes.
	again := renderBacklog(parsed)
	if again != rendered {
		t.Errorf("round-trip not byte-stable\n--- first ---\n%s\n--- second ---\n%s", rendered, again)
	}
}

func TestParseBacklogMissingMarkerFails(t *testing.T) {
	content := "# Backlog\n\n#### US-001: Setup\n\nbody only, no marker\n"
	_, err := parseBacklog(content)
	if err == nil {
		t.Fatal("expected error for missing marker")
	}
}

func TestPlanRoundTrip(t *testing.T) {
	tasks := []domain.Task{
		{ID: "TASK-01", Title: "Schema DB", Description: "Create schema", Type: domain.TaskImpl, Status: domain.StatusTodo},
		{ID: "TASK-02", Title: "Test schema", Description: "Verify migration", Type: domain.TaskTest, Status: domain.StatusTodo, Dependencies: []string{"TASK-01"}},
	}
	plan := domain.PlanInput{
		PlanBody: "## Soluzione Tecnica\n\nSpiegazione.",
		Tasks:    tasks,
	}
	rendered := renderPlan("US-001", plan)
	body, parsedTasks, err := parsePlan(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Soluzione Tecnica") {
		t.Errorf("plan body lost: %q", body)
	}
	if len(parsedTasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(parsedTasks))
	}
	if parsedTasks[1].ID != "TASK-02" || len(parsedTasks[1].Dependencies) != 1 || parsedTasks[1].Dependencies[0] != "TASK-01" {
		t.Errorf("dependency lost: %+v", parsedTasks[1])
	}
	again := renderPlan("US-001", domain.PlanInput{PlanBody: body, Tasks: parsedTasks})
	if again != rendered {
		t.Errorf("plan round-trip not byte-stable")
	}
}

func TestSavePlanRoundTripKeepsRichTaskBody(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	if _, err := c.SaveInitialBacklog(ctx, []domain.Spec{{
		Code:     "US-001",
		Title:    "Setup",
		Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
		Priority: domain.PriorityHigh,
		Points:   3,
		Status:   domain.StatusPlanned,
	}}); err != nil {
		t.Fatal(err)
	}

	const taskMarkdownBody = "## Descrizione\n\nParagraph\n\n## File Coinvolti\n- internal/schema.sql — creare lo schema\n\n## Criteri di Completamento\n- [ ] checklist"
	if _, err := c.SavePlan(ctx, "US-001", domain.PlanInput{
		PlanBody: "## Plan",
		Tasks: []domain.Task{{
			ID:     "TASK-01",
			Title:  "Schema DB",
			Body:   taskMarkdownBody,
			Type:   domain.TaskImpl,
			Status: domain.StatusTodo,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	tasks, err := c.ReadSpecTasks(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Body != taskMarkdownBody {
		t.Fatalf("task markdown did not survive in body: got %q want %q", tasks[0].Body, taskMarkdownBody)
	}

	raw, err := os.ReadFile(c.planPath("US-001"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "body: |") {
		t.Fatalf("expected YAML task body block, got:\n%s", text)
	}
	if strings.Contains(text, "\n      description:") {
		t.Fatalf("did not expect canonical Wiki task metadata to persist legacy description, got:\n%s", text)
	}
	if !strings.Contains(text, planBodyMarker) || !strings.Contains(text, "## Implementation tasks") {
		t.Fatalf("expected human-navigable Wiki plan sections, got:\n%s", text)
	}
}

func TestSavePlanLegacyDescriptionFallbackNormalizesBody(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	if _, err := c.SaveInitialBacklog(ctx, []domain.Spec{{
		Code:     "US-001",
		Title:    "Setup",
		Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
		Priority: domain.PriorityHigh,
		Points:   3,
		Status:   domain.StatusPlanned,
	}}); err != nil {
		t.Fatal(err)
	}

	const taskMarkdownBody = "Paragraph\n\n- item\n\n`code`"
	if _, err := c.SavePlan(ctx, "US-001", domain.PlanInput{
		PlanBody: "## Plan",
		Tasks: []domain.Task{{
			ID:          "TASK-01",
			Title:       "Schema DB",
			Description: taskMarkdownBody,
			Type:        domain.TaskImpl,
			Status:      domain.StatusTodo,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	tasks, err := c.ReadSpecTasks(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Body != taskMarkdownBody {
		t.Fatalf("task markdown did not normalize into body: got %q want %q", tasks[0].Body, taskMarkdownBody)
	}
	if tasks[0].Description != taskMarkdownBody {
		t.Fatalf("legacy task description should still round-trip: got %q want %q", tasks[0].Description, taskMarkdownBody)
	}
}

func TestUpdateSpec(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	_, err := c.SaveInitialBacklog(ctx, []domain.Spec{{
		Code:     "US-001",
		Title:    "Setup",
		Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
		Priority: domain.PriorityMedium,
		Points:   3,
		Status:   domain.StatusTodo,
		Scope:    "MVP",
		Body:     "## Spec\n\nOriginal.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	newTitle := "Setup project"
	newPriority := domain.PriorityHigh
	newPoints := 5
	newBody := "## Spec\n\nUpdated."
	patch := domain.SpecUpdate{
		Title:    &newTitle,
		Priority: &newPriority,
		Points:   &newPoints,
		Body:     &newBody,
	}
	if _, err := c.UpdateSpec(ctx, "US-001", patch); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadSpecDetail(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != newTitle {
		t.Errorf("title not updated: %q", got.Title)
	}
	if got.Priority != newPriority {
		t.Errorf("priority not updated: %q", got.Priority)
	}
	if got.Points != newPoints {
		t.Errorf("points not updated: %d", got.Points)
	}
	if got.Body != newBody {
		t.Errorf("body not updated: %q", got.Body)
	}
	// untouched fields preserved
	if got.Scope != "MVP" {
		t.Errorf("scope unexpectedly changed: %q", got.Scope)
	}
	if got.Epic.Code != "EP-001" {
		t.Errorf("epic unexpectedly changed: %+v", got.Epic)
	}
}

func TestUpdateSpecUnknownReturnsPrecondition(t *testing.T) {
	c := newTestConnector(t)
	_, err := c.SaveInitialBacklog(context.Background(), []domain.Spec{{
		Code: "US-001", Title: "Setup",
		Epic: domain.Epic{Code: "EP-001", Title: "F"}, Priority: domain.PriorityHigh, Points: 1, Status: domain.StatusTodo,
	}})
	if err != nil {
		t.Fatal(err)
	}
	title := "ghost"
	_, err = c.UpdateSpec(context.Background(), "US-404", domain.SpecUpdate{Title: &title})
	if err == nil {
		t.Fatal("expected error for unknown spec")
	}
}

func TestSpecFilesStoreEpicWithCodeAndTitle(t *testing.T) {
	c := newTestConnector(t)
	_, err := c.SaveInitialBacklog(context.Background(), []domain.Spec{{
		Code:     "US-001",
		Title:    "Setup",
		Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
		Priority: domain.PriorityHigh,
		Points:   3,
		Status:   domain.StatusTodo,
		Body:     "## Spec\n\nAs a user, I want X.",
	}})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(c.specPath("US-001"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "code: EP-001") {
		t.Fatalf("expected epic code in spec file, got:\n%s", text)
	}
	if !strings.Contains(text, "title: Foundations") {
		t.Fatalf("expected epic title in spec file, got:\n%s", text)
	}
	if !strings.Contains(text, "schema: archetipo/spec-wiki/v1") || !strings.Contains(text, specBodyMarker) {
		t.Fatalf("expected canonical Wiki spec format, got:\n%s", text)
	}

	store, err := c.loadStore()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Specs["US-001"].Epic.Title; got != "Foundations" {
		t.Fatalf("expected epic title preserved, got %q", got)
	}
}

func TestSpecFilesReadLegacyScalarEpic(t *testing.T) {
	c := newTestConnector(t)
	specsDir := filepath.Join(c.cfg.ProjectRoot, ".archetipo", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backlog := "schema: archetipo/backlog/v2\nversion: 2\nepics:\n  - code: EP-001\n    title: Foundations\norder: []\n"
	if err := os.WriteFile(filepath.Join(c.cfg.ProjectRoot, ".archetipo", "backlog.yaml"), []byte(backlog), 0o644); err != nil {
		t.Fatal(err)
	}
	legacySpec := "schema: archetipo/spec/v2\ncode: US-001\ntitle: Setup\nepic: EP-001\npriority: HIGH\npoints: 3\nstatus: TODO\n"
	if err := os.WriteFile(filepath.Join(specsDir, "US-001.yaml"), []byte(legacySpec), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := c.loadStore()
	if err != nil {
		t.Fatal(err)
	}
	st, ok := store.Specs["US-001"]
	if !ok {
		t.Fatalf("spec US-001 not loaded; got %+v", store.Specs)
	}
	if st.Epic.Code != "EP-001" {
		t.Errorf("epic code lost from legacy scalar: %q", st.Epic.Code)
	}
	if st.Epic.Title != "Foundations" {
		t.Errorf("epic title fallback failed; got %q want %q", st.Epic.Title, "Foundations")
	}
}

func TestDeleteSpecRemovesStoreAndArtifacts(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	_, err := c.SaveInitialBacklog(ctx, []domain.Spec{
		{Code: "US-001", Title: "Setup", Epic: domain.Epic{Code: "EP-001", Title: "Foundations"}, Priority: domain.PriorityHigh, Points: 3, Status: domain.StatusTodo},
		{Code: "US-002", Title: "Auth", Epic: domain.Epic{Code: "EP-001", Title: "Foundations"}, Priority: domain.PriorityMedium, Points: 5, Status: domain.StatusPlanned},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SavePlan(ctx, "US-001", domain.PlanInput{PlanBody: "## Plan", Tasks: []domain.Task{{ID: "TASK-01", Title: "Ship", Type: domain.TaskImpl, Status: domain.StatusTodo}}}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveReview(ctx, "US-001", domain.Review{Comments: []domain.ReviewComment{{File: "x.go", Line: 7, Side: "new", Body: "check this"}}}); err != nil {
		t.Fatal(err)
	}

	res, err := c.DeleteSpec(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatal("expected ok write result")
	}
	for _, path := range []string{c.specPath("US-001"), c.planPath("US-001"), c.reviewPath("US-001")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
	if _, err := c.ReadSpecDetail(ctx, "US-001"); err == nil {
		t.Fatal("expected deleted spec to be unreadable")
	}
	store, err := c.loadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Backlog.Order) != 1 || store.Backlog.Order[0] != "US-002" {
		t.Fatalf("unexpected backlog order after delete: %+v", store.Backlog.Order)
	}
	if _, ok := store.Specs["US-001"]; ok {
		t.Fatal("deleted spec still present in store")
	}
	if _, ok := store.Specs["US-002"]; !ok {
		t.Fatal("remaining spec missing from store")
	}
}

func TestDeleteSpecIgnoresMissingOptionalArtifacts(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	_, err := c.SaveInitialBacklog(ctx, []domain.Spec{{
		Code:     "US-001",
		Title:    "Setup",
		Epic:     domain.Epic{Code: "EP-001", Title: "Foundations"},
		Priority: domain.PriorityHigh,
		Points:   3,
		Status:   domain.StatusTodo,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteSpec(ctx, "US-001"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.specPath("US-001")); !os.IsNotExist(err) {
		t.Fatalf("expected spec file removed, stat err=%v", err)
	}
}

func TestBacklogSpecsAndPlansAreNavigableWikiPages(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	_, err := c.SaveInitialBacklog(ctx, []domain.Spec{{
		Code: "US-001", Title: "Wiki backlog", Epic: domain.Epic{Code: "EP-001", Title: "Knowledge"},
		Priority: domain.PriorityHigh, Points: 3, Status: domain.StatusTodo, Body: "## Acceptance Criteria\n\n- AC-1: searchable",
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.SavePlan(ctx, "US-001", domain.PlanInput{PlanBody: "## Solution\n\nPersist in Wiki.", Tasks: []domain.Task{{ID: "TASK-01", Title: "Implement", Type: domain.TaskImpl, Status: domain.StatusTodo}}})
	if err != nil {
		t.Fatal(err)
	}

	for _, pageType := range []string{"backlog", "spec", "plan"} {
		pages, err := wiki.Search(c.cfg.ProjectRoot, c.wikiRoot(), "", pageType, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != 1 {
			t.Fatalf("expected one %s Wiki page, got %+v", pageType, pages)
		}
	}
	report := wiki.Validate(c.cfg.ProjectRoot, c.wikiRoot())
	if !report.OK {
		t.Fatalf("generated backlog Wiki is invalid: %+v", report.Findings)
	}
	if _, err := os.Stat(c.legacyYAMLBacklogPath()); !os.IsNotExist(err) {
		t.Fatalf("legacy backlog must not be generated, stat err=%v", err)
	}
}

func TestOperationalWritePreservesUnchangedReviewedSpec(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	_, err := c.SaveInitialBacklog(ctx, []domain.Spec{
		{Code: "US-001", Title: "First", Epic: domain.Epic{Code: "EP-001", Title: "E"}, Priority: domain.PriorityHigh, Points: 1, Status: domain.StatusTodo},
		{Code: "US-002", Title: "Second", Epic: domain.Epic{Code: "EP-001", Title: "E"}, Priority: domain.PriorityMedium, Points: 1, Status: domain.StatusTodo},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wiki.Approve(c.cfg.ProjectRoot, c.wikiRoot(), []string{"backlog/specs/US-002"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.TransitionStatus(ctx, "US-001", domain.StatusPlanned); err != nil {
		t.Fatal(err)
	}
	pages, err := wiki.Search(c.cfg.ProjectRoot, c.wikiRoot(), "US-002", "spec", "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("unrelated reviewed spec was reset: %+v", pages)
	}
}

func TestLegacyYAMLStoreMigratesToWikiOnWrite(t *testing.T) {
	c := newTestConnector(t)
	if err := os.MkdirAll(c.legacyYAMLSpecsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(c.legacyYAMLPlanPath("US-001")), 0o755); err != nil {
		t.Fatal(err)
	}
	backlog := "schema: archetipo/backlog/v2\nversion: 2\nepics:\n  - code: EP-001\n    title: Foundations\norder: [US-001]\n"
	spec := "schema: archetipo/spec/v2\ncode: US-001\ntitle: Setup\nepic:\n  code: EP-001\n  title: Foundations\npriority: HIGH\npoints: 3\nstatus: TODO\nbody: Legacy body\n"
	plan := "schema: archetipo/plan/v2\nspec_code: US-001\nbody: Legacy plan\ntasks:\n  - id: TASK-01\n    title: Migrate\n    type: Impl\n    status: TODO\n"
	if err := os.WriteFile(c.legacyYAMLBacklogPath(), []byte(backlog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.legacyYAMLSpecsDir(), "US-001.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.legacyYAMLPlanPath("US-001"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := c.TransitionStatus(context.Background(), "US-001", domain.StatusPlanned); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{c.backlogPath(), c.specPath("US-001"), c.planPath("US-001")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing migrated Wiki page %s: %v", path, err)
		}
	}
	for _, path := range []string{c.legacyYAMLBacklogPath(), filepath.Join(c.legacyYAMLSpecsDir(), "US-001.yaml"), c.legacyYAMLPlanPath("US-001")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy artifact remains at %s: %v", path, err)
		}
	}
	got, err := c.ReadSpecDetail(context.Background(), "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusPlanned || got.Body != "Legacy body" {
		t.Fatalf("migration lost spec data: %+v", got)
	}
	tasks, err := c.ReadSpecTasks(context.Background(), "US-001")
	if err != nil || len(tasks) != 1 || tasks[0].ID != "TASK-01" {
		t.Fatalf("migration lost plan tasks: %+v, %v", tasks, err)
	}
}

func TestWikiSpecRejectsInvalidManagedIdentity(t *testing.T) {
	tests := []struct {
		name        string
		replaceFrom string
		replaceTo   string
		wantError   string
	}{
		{
			name:        "unsupported schema",
			replaceFrom: "schema: archetipo/spec-wiki/v1",
			replaceTo:   "schema: archetipo/spec-wiki/v99",
			wantError:   "unsupported spec Wiki schema",
		},
		{
			name:        "code differs from filename",
			replaceFrom: "code: US-001",
			replaceTo:   "code: US-999",
			wantError:   "does not match file US-001.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConnector(t)
			_, err := c.SaveInitialBacklog(context.Background(), []domain.Spec{{
				Code: "US-001", Title: "Managed identity", Epic: domain.Epic{Code: "EP-001", Title: "E"},
				Priority: domain.PriorityHigh, Points: 1, Status: domain.StatusTodo,
			}})
			if err != nil {
				t.Fatal(err)
			}
			path := c.specPath("US-001")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			changed := strings.Replace(string(raw), tt.replaceFrom, tt.replaceTo, 1)
			if changed == string(raw) {
				t.Fatalf("test fixture did not contain %q", tt.replaceFrom)
			}
			if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err = c.ReadSpecDetail(context.Background(), "US-001")
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestMalformedWikiPreflightPreventsManagedWrites(t *testing.T) {
	t.Run("backlog", func(t *testing.T) {
		c := newTestConnector(t)
		if err := os.MkdirAll(c.wikiRoot(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(c.wikiRoot(), "broken.md"), []byte("# Missing frontmatter\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := c.SaveInitialBacklog(context.Background(), []domain.Spec{{
			Code: "US-001", Title: "Preflight", Epic: domain.Epic{Code: "EP-001", Title: "E"},
			Priority: domain.PriorityHigh, Points: 1, Status: domain.StatusTodo,
		}})
		if err == nil || !strings.Contains(err.Error(), "cannot refresh Wiki catalog") {
			t.Fatalf("expected catalog preflight error, got %v", err)
		}
		for _, path := range []string{c.backlogPath(), c.specPath("US-001")} {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("managed state changed despite failed preflight at %s: %v", path, statErr)
			}
		}
	})

	t.Run("plan", func(t *testing.T) {
		c := newTestConnector(t)
		_, err := c.SaveInitialBacklog(context.Background(), []domain.Spec{{
			Code: "US-001", Title: "Preflight", Epic: domain.Epic{Code: "EP-001", Title: "E"},
			Priority: domain.PriorityHigh, Points: 1, Status: domain.StatusTodo,
		}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(c.wikiRoot(), "broken.md"), []byte("# Missing frontmatter\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err = c.SavePlan(context.Background(), "US-001", domain.PlanInput{
			PlanBody: "## Plan", Tasks: []domain.Task{{ID: "TASK-01", Title: "Implement", Type: domain.TaskImpl, Status: domain.StatusTodo}},
		})
		if err == nil || !strings.Contains(err.Error(), "cannot refresh Wiki catalog") {
			t.Fatalf("expected catalog preflight error, got %v", err)
		}
		if _, statErr := os.Stat(c.planPath("US-001")); !os.IsNotExist(statErr) {
			t.Fatalf("plan changed despite failed preflight: %v", statErr)
		}
	})
}
