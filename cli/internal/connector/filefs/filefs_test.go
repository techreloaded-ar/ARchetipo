package filefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
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
	if strings.Contains(text, "description:") {
		t.Fatalf("did not expect canonical YAML plan to persist description, got:\n%s", text)
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

	raw, err := os.ReadFile(filepath.Join(c.cfg.ProjectRoot, ".archetipo", "specs", "US-001.yaml"))
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

// seedDeclaredEpicsWorkspace writes a backlog that declares two epics — one
// that owns a spec and one that owns none — plus a spec whose epic is only
// known through the spec file itself. It is the on-disk state where the
// declared-but-empty epic used to disappear.
func seedDeclaredEpicsWorkspace(t *testing.T, c *Connector) {
	t.Helper()
	specsDir := filepath.Join(c.cfg.ProjectRoot, ".archetipo", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backlog := "schema: archetipo/backlog/v2\n" +
		"version: 2\n" +
		"epics:\n" +
		"  - code: EP-900\n" +
		"    title: Declared With Specs\n" +
		"  - code: EP-901\n" +
		"    title: Declared Without Specs\n" +
		"order:\n" +
		"  - US-900\n" +
		"  - US-902\n"
	if err := os.WriteFile(filepath.Join(c.cfg.ProjectRoot, ".archetipo", "backlog.yaml"), []byte(backlog), 0o644); err != nil {
		t.Fatal(err)
	}
	specWithDeclaredEpic := "schema: archetipo/spec/v2\ncode: US-900\ntitle: Prima spec\nepic:\n  code: EP-900\n  title: Declared With Specs\npriority: HIGH\npoints: 3\nstatus: TODO\n"
	if err := os.WriteFile(filepath.Join(specsDir, "US-900.yaml"), []byte(specWithDeclaredEpic), 0o644); err != nil {
		t.Fatal(err)
	}
	specWithUndeclaredEpic := "schema: archetipo/spec/v2\ncode: US-902\ntitle: Spec di epica non dichiarata\nepic:\n  code: EP-902\n  title: Undeclared From Spec\npriority: LOW\npoints: 1\nstatus: TODO\n"
	if err := os.WriteFile(filepath.Join(specsDir, "US-902.yaml"), []byte(specWithUndeclaredEpic), 0o644); err != nil {
		t.Fatal(err)
	}
}

func epicTitleByCode(epics []domain.Epic, code string) (string, bool) {
	for _, epic := range epics {
		if epic.Code == code {
			return epic.Title, true
		}
	}
	return "", false
}

func TestReadExistingBacklogIncludesDeclaredEpics(t *testing.T) {
	c := newTestConnector(t)
	seedDeclaredEpicsWorkspace(t, c)

	out, err := c.ReadExistingBacklog(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	title, ok := epicTitleByCode(out.Epics, "EP-900")
	if !ok {
		t.Fatalf("declared epic EP-900 missing from summary: %+v", out.Epics)
	}
	if title != "Declared With Specs" {
		t.Errorf("EP-900 title = %q, want %q", title, "Declared With Specs")
	}

	title, ok = epicTitleByCode(out.Epics, "EP-901")
	if !ok {
		t.Fatalf("declared epic without specs EP-901 missing from summary: %+v", out.Epics)
	}
	if title != "Declared Without Specs" {
		t.Errorf("EP-901 title = %q, want %q", title, "Declared Without Specs")
	}

	// Symmetric case that must stay unchanged: an epic known only through a
	// spec is still reported.
	title, ok = epicTitleByCode(out.Epics, "EP-902")
	if !ok {
		t.Fatalf("epic derived from specs EP-902 missing from summary: %+v", out.Epics)
	}
	if title != "Undeclared From Spec" {
		t.Errorf("EP-902 title = %q, want %q", title, "Undeclared From Spec")
	}
}

func TestAppendSpecsPreservesDeclaredEpics(t *testing.T) {
	c := newTestConnector(t)
	seedDeclaredEpicsWorkspace(t, c)

	if _, err := c.AppendSpecs(context.Background(), []domain.Spec{{
		Code:     "US-903",
		Title:    "Nuova spec",
		Epic:     domain.Epic{Code: "EP-900", Title: "Declared With Specs"},
		Priority: domain.PriorityMedium,
		Points:   2,
		Status:   domain.StatusTodo,
	}}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(c.cfg.ProjectRoot, ".archetipo", "backlog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc backlogDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	title, ok := epicTitleByCode(doc.Epics, "EP-901")
	if !ok {
		t.Fatalf("declared epic without specs erased by backlog rewrite: %+v", doc.Epics)
	}
	if title != "Declared Without Specs" {
		t.Errorf("EP-901 title = %q, want %q", title, "Declared Without Specs")
	}
	if _, ok := epicTitleByCode(doc.Epics, "EP-900"); !ok {
		t.Errorf("declared epic EP-900 erased by backlog rewrite: %+v", doc.Epics)
	}

	// Symmetric case: the epic known only through a spec must survive too.
	title, ok = epicTitleByCode(doc.Epics, "EP-902")
	if !ok {
		t.Fatalf("epic derived from specs erased by backlog rewrite: %+v", doc.Epics)
	}
	if title != "Undeclared From Spec" {
		t.Errorf("EP-902 title = %q, want %q", title, "Undeclared From Spec")
	}
}

// AC-4 on the filesystem: the rollback of an inception that ended badly really
// removes the document, and calling it again on a workspace that already has no
// PRD is a no-op rather than a failure.
func TestDiscardPRDRemovesTheDocumentAndIsIdempotent(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()

	if _, err := c.SavePRD(ctx, "# PRD\n\nVisione del prodotto.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.cfg.Paths.PRD); err != nil {
		t.Fatalf("SavePRD did not write the PRD: %v", err)
	}

	removed, err := c.DiscardPRD(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("DiscardPRD did not report the removal of an existing PRD")
	}
	if _, err := os.Stat(c.cfg.Paths.PRD); !os.IsNotExist(err) {
		t.Fatalf("the PRD file is still there: %v", err)
	}
	body, err := c.ReadPRD(ctx)
	if err != nil || body != "" {
		t.Fatalf("the PRD is still readable: %q, %v", body, err)
	}

	removed, err = c.DiscardPRD(ctx)
	if err != nil {
		t.Fatalf("discarding a workspace without a PRD failed: %v", err)
	}
	if removed {
		t.Fatal("DiscardPRD reported a removal with nothing to remove")
	}
}

// AC-4 on the filesystem: the rollback of a backlog generation that ended badly
// removes the spec files and the index, so a later read answers with an empty
// board, and calling it again on a workspace without a backlog is a no-op.
func TestDiscardBacklogOnAWorkspaceWithoutOneIsANoOp(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()

	removed, err := c.DiscardBacklog(ctx)
	if err != nil {
		t.Fatalf("discarding a workspace without a backlog failed: %v", err)
	}
	if removed {
		t.Fatal("DiscardBacklog reported a removal with nothing to remove")
	}
	if _, err := os.Stat(c.backlogPath()); !os.IsNotExist(err) {
		t.Fatalf("DiscardBacklog created the backlog index as a side effect: %v", err)
	}
	if _, err := os.Stat(c.specsDir()); !os.IsNotExist(err) {
		t.Fatalf("DiscardBacklog created the specs directory as a side effect: %v", err)
	}
}

func TestDiscardBacklogRemovesSpecFilesAndIndex(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()

	specs := []domain.Spec{
		{
			Code:     "US-001",
			Title:    "Registrazione utente",
			Epic:     domain.Epic{Code: "EP-001", Title: "Onboarding"},
			Priority: domain.PriorityHigh,
			Points:   3,
			Status:   domain.StatusTodo,
		},
		{
			Code:     "US-002",
			Title:    "Ricerca catalogo",
			Epic:     domain.Epic{Code: "EP-002", Title: "Catalogo"},
			Priority: domain.PriorityMedium,
			Points:   5,
			Status:   domain.StatusTodo,
		},
	}
	if _, err := c.SaveInitialBacklog(ctx, specs); err != nil {
		t.Fatal(err)
	}
	for _, s := range specs {
		if _, err := os.Stat(c.specPath(s.Code)); err != nil {
			t.Fatalf("SaveInitialBacklog did not write %s: %v", c.specPath(s.Code), err)
		}
	}
	if _, err := os.Stat(c.backlogPath()); err != nil {
		t.Fatalf("SaveInitialBacklog did not write the backlog index: %v", err)
	}

	removed, err := c.DiscardBacklog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("DiscardBacklog did not report the removal of an existing backlog")
	}

	for _, s := range specs {
		if _, err := os.Stat(c.specPath(s.Code)); !os.IsNotExist(err) {
			t.Fatalf("the spec file %s is still there: %v", c.specPath(s.Code), err)
		}
	}
	if _, err := os.Stat(c.backlogPath()); !os.IsNotExist(err) {
		t.Fatalf("the backlog index is still there: %v", err)
	}

	// A reread answers exactly as it did before the backlog existed: the
	// workspace is back to the state the run started from.
	summary, err := c.ReadExistingBacklog(ctx)
	if err == nil {
		if len(summary.Codes) != 0 || len(summary.Titles) != 0 || len(summary.Epics) != 0 {
			t.Fatalf("the board is not empty after the rollback: %+v", summary)
		}
	} else if !errors.Is(err, errBacklogMissing) {
		t.Fatalf("reading the backlog after the rollback failed: %v", err)
	}

	removed, err = c.DiscardBacklog(ctx)
	if err != nil {
		t.Fatalf("discarding an already discarded backlog failed: %v", err)
	}
	if removed {
		t.Fatal("DiscardBacklog reported a second removal with nothing to remove")
	}
}

// A review artifact carrying comments, a prepared dossier and a human verdict
// must survive a save and a re-read field by field: the dossier is what the
// viewer shows and the verdict is the only trace of who decided.
func TestReviewRoundTripKeepsDossierAndVerdict(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	want := domain.Review{
		Comments: []domain.ReviewComment{{File: "cli/main.go", Side: "new", Line: 12, Body: "naming", CreatedAt: "2026-08-20T10:00:00Z"}},
		Dossier: &domain.ReviewDossier{
			ExecutionID: "exec-42",
			PreparedAt:  "2026-08-20T10:05:00Z",
			Summary:     "The increment satisfies the acceptance criteria.",
			Criteria: []domain.ReviewCriterion{
				{ID: "AC-1", Verdict: domain.ReviewCriterionMet, Note: "covered by the HTTP acceptance test"},
				{ID: "AC-2", Verdict: domain.ReviewCriterionUnclear},
			},
			Blockers: []string{"the demo video is missing"},
		},
		Verdict: &domain.ReviewVerdict{
			Decision:    domain.ReviewDecisionApproved,
			DecidedAt:   "2026-08-20T10:30:00Z",
			ExecutionID: "exec-42",
		},
	}

	if err := c.SaveReview(ctx, "US-901", want); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadReview(ctx, "US-901")
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Comments) != 1 || got.Comments[0] != want.Comments[0] {
		t.Fatalf("comments = %+v, want %+v", got.Comments, want.Comments)
	}
	if got.Dossier == nil {
		t.Fatal("dossier lost on round trip")
	}
	if got.Dossier.ExecutionID != want.Dossier.ExecutionID ||
		got.Dossier.PreparedAt != want.Dossier.PreparedAt ||
		got.Dossier.Summary != want.Dossier.Summary {
		t.Fatalf("dossier header = %+v, want %+v", got.Dossier, want.Dossier)
	}
	if len(got.Dossier.Criteria) != 2 || got.Dossier.Criteria[0] != want.Dossier.Criteria[0] || got.Dossier.Criteria[1] != want.Dossier.Criteria[1] {
		t.Fatalf("dossier criteria = %+v, want %+v", got.Dossier.Criteria, want.Dossier.Criteria)
	}
	if len(got.Dossier.Blockers) != 1 || got.Dossier.Blockers[0] != want.Dossier.Blockers[0] {
		t.Fatalf("dossier blockers = %+v, want %+v", got.Dossier.Blockers, want.Dossier.Blockers)
	}
	if got.Verdict == nil || *got.Verdict != *want.Verdict {
		t.Fatalf("verdict = %+v, want %+v", got.Verdict, want.Verdict)
	}
}

// A review file written before the dossier and the verdict existed stays
// readable, and reports both as absent rather than as empty.
func TestReviewFileWithOnlyCommentsStaysReadable(t *testing.T) {
	c := newTestConnector(t)
	ctx := context.Background()
	if err := os.MkdirAll(c.reviewsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "schema: archetipo/review/v1\nspec_code: US-902\ncomments:\n  - file: a.go\n    side: new\n    line: 3\n    body: rename this\n"
	if err := os.WriteFile(c.reviewPath("US-902"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := c.ReadReview(ctx, "US-902")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 1 || got.Comments[0].Body != "rename this" {
		t.Fatalf("comments = %+v, want the single legacy comment", got.Comments)
	}
	if got.Dossier != nil {
		t.Fatalf("dossier = %+v, want nil for a file that has none", got.Dossier)
	}
	if got.Verdict != nil {
		t.Fatalf("verdict = %+v, want nil for a file that has none", got.Verdict)
	}
}

// A spec that has never been reviewed reads as an empty artifact, not as an
// error, and both optional parts are absent.
func TestReadReviewOfMissingFileHasNoDossierOrVerdict(t *testing.T) {
	c := newTestConnector(t)
	got, err := c.ReadReview(context.Background(), "US-903")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Comments) != 0 || got.Dossier != nil || got.Verdict != nil {
		t.Fatalf("review of a spec with no file = %+v, want empty comments and nil dossier/verdict", got)
	}
}
