package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// workspaceStatusResponse decodes the payload the browser receives from
// GET /api/workspace/status. Like workspaceActionsResponse it is written out by
// hand rather than reusing workspaceStatusView, so what these tests assert is
// the wire shape a client actually reads — the key names included — and not the
// server's own struct, which could be renamed without any client noticing.
type workspaceStatusResponse struct {
	Template struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Label   string `json:"label"`
	} `json:"template"`
	Stage struct {
		ID      string `json:"id"`
		Label   string `json:"label"`
		Summary string `json:"summary"`
	} `json:"stage"`
	NextStep *struct {
		Scope  string `json:"scope"`
		Action string `json:"action"`
		Label  string `json:"label"`
		Skill  string `json:"skill"`
		Spec   *struct {
			Code   string        `json:"code"`
			Title  string        `json:"title"`
			Status domain.Status `json:"status"`
		} `json:"spec"`
		Runnable          bool   `json:"runnable"`
		UnavailableReason string `json:"unavailable_reason"`
		UnlockedBy        string `json:"unlocked_by"`
	} `json:"next_step"`
	HasPRD     bool `json:"has_prd"`
	HasBacklog bool `json:"has_backlog"`
	Actions    []struct {
		ID                string `json:"id"`
		Offered           bool   `json:"offered"`
		Runnable          bool   `json:"runnable"`
		UnavailableReason string `json:"unavailable_reason"`
		UnlockedBy        string `json:"unlocked_by"`
	} `json:"actions"`
}

func readWorkspaceStatus(t *testing.T, srv *Server) workspaceStatusResponse {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace/status: %d %s", w.Code, w.Body.String())
	}
	var view workspaceStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

// stageOfArchetype returns the stage the installed process declares under id.
// Every expectation about a stage's words is read from here rather than
// rewritten as a literal, because the point of the route is that the workspace
// is described in the words of the Archetipo (AC-1): a test that spelled them
// out again would keep passing after the process changed its mind.
func stageOfArchetype(t *testing.T, id string) template.Stage {
	t.Helper()
	for _, stage := range template.Default().Stages {
		if stage.ID == id {
			return stage
		}
	}
	t.Fatalf("the Archetipo declares no %q stage: %+v", id, template.Default().Stages)
	return template.Stage{}
}

// workspaceActionLabelOf is the same read for a workspace action's label.
func workspaceActionLabelOf(t *testing.T, id string) string {
	t.Helper()
	for _, action := range template.Default().WorkspaceActions {
		if action.ID == id {
			return action.Label
		}
	}
	t.Fatalf("the Archetipo declares no %q workspace action", id)
	return ""
}

func assertStageMatchesArchetype(t *testing.T, view workspaceStatusResponse, id string) {
	t.Helper()
	want := stageOfArchetype(t, id)
	if view.Stage.ID != want.ID {
		t.Fatalf("stage.id = %q, want %q", view.Stage.ID, want.ID)
	}
	if view.Stage.Label != want.Label {
		t.Fatalf("stage.label = %q, want %q", view.Stage.Label, want.Label)
	}
	if view.Stage.Summary != want.Summary {
		t.Fatalf("stage.summary = %q, want %q", view.Stage.Summary, want.Summary)
	}
}

func TestWorkspaceStatusReportsTheStageOfAnEmptyWorkspace(t *testing.T) {
	srv, _, _ := newEmptyRunServer(t, releasedInceptionProvider("prov", nil), true)

	view := readWorkspaceStatus(t, srv)

	assertStageMatchesArchetype(t, view, template.Default().Stages[0].ID)
	if view.Template.ID != template.FabbricaDelSoftware {
		t.Fatalf("template.id = %q, want %q", view.Template.ID, template.FabbricaDelSoftware)
	}
	if view.NextStep == nil {
		t.Fatal("a workspace without a PRD has a next step, but next_step is null")
	}
	if view.NextStep.Scope != template.ScopeWorkspace {
		t.Fatalf("next_step.scope = %q, want %q", view.NextStep.Scope, template.ScopeWorkspace)
	}
	if view.NextStep.Action != "inception" {
		t.Fatalf("next_step.action = %q, want inception", view.NextStep.Action)
	}
	if want := workspaceActionLabelOf(t, "inception"); view.NextStep.Label != want {
		t.Fatalf("next_step.label = %q, want %q", view.NextStep.Label, want)
	}
	if view.NextStep.Spec != nil {
		t.Fatalf("a workspace-scoped step acts on no spec, got %+v", view.NextStep.Spec)
	}
	if view.HasPRD {
		t.Fatal("has_prd = true on a workspace that has no PRD")
	}
	if view.HasBacklog {
		t.Fatal("has_backlog = true on a workspace that has no backlog")
	}
}

func TestWorkspaceStatusMovesToTheBacklogStageOnceThePRDExists(t *testing.T) {
	srv, cfg, _ := newEmptyRunServer(t, releasedInceptionProvider("prov", nil), true)

	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	view := readWorkspaceStatus(t, srv)

	assertStageMatchesArchetype(t, view, "senza-backlog")
	if view.NextStep == nil {
		t.Fatal("a workspace with a PRD and no backlog has a next step, but next_step is null")
	}
	if view.NextStep.Action != "backlog" {
		t.Fatalf("next_step.action = %q, want backlog", view.NextStep.Action)
	}
	if view.NextStep.Scope != template.ScopeWorkspace {
		t.Fatalf("next_step.scope = %q, want %q", view.NextStep.Scope, template.ScopeWorkspace)
	}
	if !view.HasPRD {
		t.Fatal("has_prd = false after the PRD was written")
	}
	if view.HasBacklog {
		t.Fatal("has_backlog = true on a workspace that has no backlog")
	}
}

func TestWorkspaceStatusRecommendsPlanningTheFirstSpec(t *testing.T) {
	srv, cfg, _ := newRunServer(t, releasedInceptionProvider("prov", nil), true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	view := readWorkspaceStatus(t, srv)

	assertStageMatchesArchetype(t, view, "da-pianificare")
	if !view.HasBacklog {
		t.Fatal("has_backlog = false on a workspace whose backlog was seeded")
	}
	if view.NextStep == nil {
		t.Fatal("a backlog with specs in TODO has a next step, but next_step is null")
	}
	if view.NextStep.Scope != template.ScopeSpec {
		t.Fatalf("next_step.scope = %q, want %q", view.NextStep.Scope, template.ScopeSpec)
	}
	if view.NextStep.Action != "plan" {
		t.Fatalf("next_step.action = %q, want plan", view.NextStep.Action)
	}
	if view.NextStep.Spec == nil {
		t.Fatal("a spec-scoped step names the spec it acts on, but next_step.spec is null")
	}
	if view.NextStep.Spec.Code != "US-901" {
		t.Fatalf("next_step.spec.code = %q, want US-901", view.NextStep.Spec.Code)
	}
	if view.NextStep.Spec.Status != domain.StatusTodo {
		t.Fatalf("next_step.spec.status = %q, want %q", view.NextStep.Spec.Status, domain.StatusTodo)
	}
}

func TestWorkspaceStatusIsTerminalWhenNoSpecAwaitsAStep(t *testing.T) {
	srv, cfg, conn := newRunServer(t, releasedInceptionProvider("prov", nil), true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")
	for _, code := range []string{"US-901", "US-902"} {
		if _, err := conn.TransitionStatus(context.Background(), code, domain.StatusDone); err != nil {
			t.Fatal(err)
		}
	}

	view := readWorkspaceStatus(t, srv)

	assertStageMatchesArchetype(t, view, "completo")
	if view.NextStep != nil {
		t.Fatalf("nothing is pending, yet next_step = %+v", view.NextStep)
	}
}

func TestWorkspaceStatusNamesWhatUnlocksEveryUnavailableAction(t *testing.T) {
	// No provider at all is exactly the workspace of someone opening ARchetipo
	// for the first time: everything is refused, and every refusal must say
	// what unlocks it rather than stand there inert (AC-4).
	srv, _, _ := newEmptyRunServer(t, nil, false)

	view := readWorkspaceStatus(t, srv)

	if len(view.Actions) == 0 {
		t.Fatal("the payload carries no workspace action")
	}
	unavailable := 0
	for _, action := range view.Actions {
		if action.Runnable {
			continue
		}
		unavailable++
		if action.UnlockedBy == "" {
			t.Fatalf("action %q is not runnable but names no unlocking condition: %+v", action.ID, action)
		}
	}
	if unavailable == 0 {
		t.Fatal("no action is unavailable on a workspace with no provider, so nothing proves AC-4")
	}
	if got := statusActionOf(t, view, "backlog").UnlockedBy; !strings.Contains(strings.ToLower(got), "inception") {
		t.Fatalf("backlog unlocked_by = %q, want it to name the inception", got)
	}
	if got := statusActionOf(t, view, "spec-draft").UnlockedBy; !strings.Contains(strings.ToLower(got), "backlog") {
		t.Fatalf("spec-draft unlocked_by = %q, want it to name the backlog generation", got)
	}
	if view.NextStep == nil {
		t.Fatal("a workspace without a PRD has a next step, but next_step is null")
	}
	if view.NextStep.Runnable {
		t.Fatal("next_step.runnable = true with no provider configured")
	}
	if view.NextStep.UnavailableReason == "" {
		t.Fatal("next_step is not runnable but gives no reason")
	}
	if view.NextStep.UnlockedBy == "" {
		t.Fatal("next_step is not runnable but names no unlocking condition")
	}
}

// statusActionOf isolates one action row of the status payload, so a process
// that grows another workspace action cannot let a neighbour's fields answer
// for the one under test.
func statusActionOf(t *testing.T, view workspaceStatusResponse, id string) struct {
	ID                string `json:"id"`
	Offered           bool   `json:"offered"`
	Runnable          bool   `json:"runnable"`
	UnavailableReason string `json:"unavailable_reason"`
	UnlockedBy        string `json:"unlocked_by"`
} {
	t.Helper()
	for _, action := range view.Actions {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("the status payload carries no %q action: %+v", id, view.Actions)
	return view.Actions[0]
}

// TestResolveStageFollowsTheArchetypeOrder is the server-side evidence of AC-3:
// with the state of the workspace held identical, reordering the stages the
// process declares changes the recommendation. Nothing else differs between the
// two templates, so the recommendation can only come from the Archetipo.
func TestResolveStageFollowsTheArchetypeOrder(t *testing.T) {
	real := template.Default()
	planFirst := template.Template{
		ID:      real.ID,
		Actions: real.Actions,
		Stages: []template.Stage{
			stageOfArchetype(t, "da-pianificare"),
			stageOfArchetype(t, "da-rivedere"),
			stageOfArchetype(t, "completo"),
		},
	}
	reviewFirst := template.Template{
		ID:      real.ID,
		Actions: real.Actions,
		Stages: []template.Stage{
			stageOfArchetype(t, "da-rivedere"),
			stageOfArchetype(t, "da-pianificare"),
			stageOfArchetype(t, "completo"),
		},
	}
	in := stageInputs{
		offersWorkspaceAction: func(string) bool { return false },
		specsInOrder: []domain.Spec{
			{Code: "US-901", Title: "Da pianificare", Status: domain.StatusTodo},
			{Code: "US-902", Title: "Da rivedere", Status: domain.StatusReview},
		},
	}

	planStage, planSpec := resolveStage(planFirst, in)
	reviewStage, reviewSpec := resolveStage(reviewFirst, in)

	if planStage.ID != "da-pianificare" {
		t.Fatalf("stage = %q, want da-pianificare", planStage.ID)
	}
	if reviewStage.ID != "da-rivedere" {
		t.Fatalf("stage = %q, want da-rivedere", reviewStage.ID)
	}
	if planStage.ID == reviewStage.ID {
		t.Fatal("the declared order of the stages did not change the recommendation")
	}
	if planSpec == nil || planSpec.Code != "US-901" {
		t.Fatalf("planning acts on %+v, want US-901", planSpec)
	}
	if reviewSpec == nil || reviewSpec.Code != "US-902" {
		t.Fatalf("review acts on %+v, want US-902", reviewSpec)
	}

	terminal, spec := resolveStage(planFirst, stageInputs{
		offersWorkspaceAction: func(string) bool { return false },
	})
	if terminal.ID != "completo" {
		t.Fatalf("stage = %q, want completo when nothing holds", terminal.ID)
	}
	if spec != nil {
		t.Fatalf("the terminal stage acts on no spec, got %+v", spec)
	}
}

func TestResolveStageSelectsTheFirstSpecInBoardOrder(t *testing.T) {
	real := template.Default()
	tpl := template.Template{
		ID:      real.ID,
		Actions: real.Actions,
		Stages: []template.Stage{
			stageOfArchetype(t, "da-pianificare"),
			stageOfArchetype(t, "completo"),
		},
	}

	// US-902 comes first on the board even though US-901 is the lower code:
	// the recommendation must point at the card a person sees on top, not at
	// whichever code sorts first.
	stage, spec := resolveStage(tpl, stageInputs{
		offersWorkspaceAction: func(string) bool { return false },
		specsInOrder: []domain.Spec{
			{Code: "US-902", Title: "Seconda per codice", Status: domain.StatusTodo},
			{Code: "US-901", Title: "Prima per codice", Status: domain.StatusTodo},
		},
	})

	if stage.ID != "da-pianificare" {
		t.Fatalf("stage = %q, want da-pianificare", stage.ID)
	}
	if spec == nil {
		t.Fatal("a spec-scoped stage names no spec")
	}
	if spec.Code != "US-902" {
		t.Fatalf("recommended spec = %q, want US-902, the first in board order", spec.Code)
	}
}
