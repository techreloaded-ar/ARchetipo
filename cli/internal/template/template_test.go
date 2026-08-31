package template

import (
	"errors"
	"reflect"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// processSkills is written out in full on purpose: it is the contract a
// workspace receives when it is initialized with the Fabbrica del software
// Template, so changing the process must break this test explicitly.
var processSkills = []string{
	"archetipo-autopilot",
	"archetipo-design",
	"archetipo-implement",
	"archetipo-inception",
	"archetipo-plan",
	"archetipo-review",
	"archetipo-spec",
	"archetipo-wiki",
}

// processActions is written out in full for the same reason as processSkills:
// the actions are the contract the process offers on a spec, so adding, removing
// or re-scoping one must break this test explicitly rather than silently change
// what a workspace is told it can do.
var processActions = []Action{
	{
		ID:       "plan",
		Label:    "Pianifica",
		Skill:    "archetipo-plan",
		Statuses: []domain.Status{domain.StatusTodo},
	},
	{
		ID:       "implement",
		Label:    "Implementa",
		Skill:    "archetipo-implement",
		Statuses: []domain.Status{domain.StatusPlanned, domain.StatusInProgress},
	},
	{
		ID:       "review",
		Label:    "Rivedi",
		Skill:    "archetipo-review",
		Statuses: []domain.Status{domain.StatusReview},
	},
}

// processWorkspaceActions is written out in full for the same reason as
// processActions: these are the steps the process offers on the workspace as a
// whole, so adding, removing or renaming one must break this test explicitly.
var processWorkspaceActions = []WorkspaceAction{
	{
		ID:    "inception",
		Label: "Avvia inception",
		Skill: "archetipo-inception",
	},
	{
		ID:    "backlog",
		Label: "Genera backlog",
		Skill: "archetipo-spec",
	},
	{
		ID:    "spec-draft",
		Label: "Proponi una spec",
		Skill: "archetipo-spec",
	},
}

// processStages is written out in full for the same reason as processActions:
// the stages are the sequence the process declares about itself, and the answer
// a workspace gets to "where am I and what comes next" is read from here, so
// adding, removing, reordering or re-scoping one must break this test
// explicitly rather than silently change what a person is told to do next.
var processStages = []Stage{
	{
		ID:      "senza-prd",
		Label:   "Senza PRD",
		Summary: "Il workspace non ha ancora un PRD: il processo comincia dall'inception.",
		Scope:   ScopeWorkspace,
		Action:  "inception",
	},
	{
		ID:      "senza-backlog",
		Label:   "Senza backlog",
		Summary: "Il PRD c'è: il passo successivo è generare il backlog iniziale.",
		Scope:   ScopeWorkspace,
		Action:  "backlog",
	},
	{
		ID:      "da-pianificare",
		Label:   "Da pianificare",
		Summary: "Ci sono spec in TODO: il passo successivo è pianificarne una.",
		Scope:   ScopeSpec,
		Action:  "plan",
	},
	{
		ID:      "da-implementare",
		Label:   "Da implementare",
		Summary: "Ci sono spec pianificate o in corso: il passo successivo è implementarne una.",
		Scope:   ScopeSpec,
		Action:  "implement",
	},
	{
		ID:      "da-rivedere",
		Label:   "Da rivedere",
		Summary: "Ci sono spec in review: il passo successivo è rivederne una.",
		Scope:   ScopeSpec,
		Action:  "review",
	},
	{
		ID:      "completo",
		Label:   "Nessun passo in sospeso",
		Summary: "Nessuna spec attende un passo del processo: aggiungi una spec quando serve.",
	},
}

// actionIDs collapses a result to the identifiers a caller keys on, so a table
// can name the expected content instead of asserting the absence of an error.
func actionIDs(actions []Action) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.ID)
	}
	return out
}

func TestDefaultTemplateDeclaresTheCurrentProcess(t *testing.T) {
	got := Default()
	if got.ID != FabbricaDelSoftware {
		t.Fatalf("id = %q, want %q", got.ID, FabbricaDelSoftware)
	}
	if got.Version == "" {
		t.Fatal("version is empty")
	}
	if got.Label == "" {
		t.Fatal("label is empty")
	}
	if !reflect.DeepEqual(got.Skills, processSkills) {
		t.Fatalf("skills = %v, want %v", got.Skills, processSkills)
	}
}

func TestDefaultTemplateCarriesCanonicalStatuses(t *testing.T) {
	want := domain.StatusLabels{
		Todo:       string(domain.StatusTodo),
		Planned:    string(domain.StatusPlanned),
		InProgress: string(domain.StatusInProgress),
		Review:     string(domain.StatusReview),
		Done:       string(domain.StatusDone),
	}
	if got := Default().Statuses; got != want {
		t.Fatalf("statuses = %+v, want %+v", got, want)
	}
}

func TestResolveEmptyIDReturnsDefault(t *testing.T) {
	for _, id := range []string{"", "   "} {
		got, err := Resolve(id)
		if err != nil {
			t.Fatalf("Resolve(%q) failed: %v", id, err)
		}
		if got.ID != Default().ID {
			t.Fatalf("Resolve(%q) = %q, want %q", id, got.ID, Default().ID)
		}
	}
}

func TestResolveUnknownIDFailsWithRegistryError(t *testing.T) {
	_, err := Resolve("inesistente")
	if err == nil {
		t.Fatal("expected an error for an unknown template id")
	}
	var registryErr *RegistryError
	if !errors.As(err, &registryErr) {
		t.Fatalf("error is %T, want *RegistryError", err)
	}
	if registryErr.TemplateID != "inesistente" {
		t.Fatalf("template id = %q, want %q", registryErr.TemplateID, "inesistente")
	}
}

func TestDefaultSkillsAreNotAliased(t *testing.T) {
	first := Default()
	first.Skills[0] = "tampered"
	if got := Default().Skills[0]; got != processSkills[0] {
		t.Fatalf("registry skills were mutated through the returned slice: %q", got)
	}
}

func TestDefaultTemplateDeclaresItsActions(t *testing.T) {
	got := Default().Actions
	if !reflect.DeepEqual(got, processActions) {
		t.Fatalf("actions = %+v, want %+v", got, processActions)
	}
	for _, action := range got {
		if action.ID == "" {
			t.Fatalf("action %+v has an empty id", action)
		}
		if action.Label == "" {
			t.Fatalf("action %q has an empty label", action.ID)
		}
		if action.Skill == "" {
			t.Fatalf("action %q has an empty skill", action.ID)
		}
	}
}

func TestActionsForReturnsOnlyTheActionsAdmittedInTheStatus(t *testing.T) {
	cases := []struct {
		status domain.Status
		want   []string
	}{
		{status: domain.StatusTodo, want: []string{"plan"}},
		{status: domain.StatusPlanned, want: []string{"implement"}},
		{status: domain.StatusInProgress, want: []string{"implement"}},
		{status: domain.StatusReview, want: []string{"review"}},
		{status: domain.StatusDone, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got := actionIDs(Default().ActionsFor(tc.status))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ActionsFor(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestActionsForEmptyListIsNeverNil(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusDone, domain.Status("SCONOSCIUTO"), domain.Status("")} {
		got := Default().ActionsFor(status)
		if got == nil {
			t.Fatalf("ActionsFor(%q) is nil, want an empty list", status)
		}
		if len(got) != 0 {
			t.Fatalf("ActionsFor(%q) = %+v, want an empty list", status, got)
		}
	}
}

func TestDefaultActionsAreNotAliased(t *testing.T) {
	first, err := Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template failed: %v", err)
	}
	first.Actions[0].ID = "tampered"
	first.Actions[0].Statuses[0] = domain.Status("TAMPERED")
	second, err := Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template again failed: %v", err)
	}
	if got := second.Actions[0].ID; got != processActions[0].ID {
		t.Fatalf("builtin actions were mutated through the returned slice: id = %q", got)
	}
	if got := second.Actions[0].Statuses[0]; got != processActions[0].Statuses[0] {
		t.Fatalf("builtin action statuses were mutated through the returned slice: status = %q", got)
	}
}

func TestDefaultTemplateDeclaresItsWorkspaceActions(t *testing.T) {
	got := Default().WorkspaceActions
	if !reflect.DeepEqual(got, processWorkspaceActions) {
		t.Fatalf("workspace actions = %+v, want %+v", got, processWorkspaceActions)
	}
	for _, action := range got {
		if action.ID == "" {
			t.Fatalf("workspace action %+v has an empty id", action)
		}
		if action.Label == "" {
			t.Fatalf("workspace action %q has an empty label", action.ID)
		}
		if action.Skill == "" {
			t.Fatalf("workspace action %q has an empty skill", action.ID)
		}
	}
}

// A workspace action names the skill the provider will invoke, so that skill
// must also be one the Template installs: a declared action whose skill never
// reaches the workspace is an action that cannot run.
func TestDefaultWorkspaceActionSkillsAreInstalled(t *testing.T) {
	template := Default()
	installed := make(map[string]bool, len(template.Skills))
	for _, skill := range template.Skills {
		installed[skill] = true
	}
	for _, action := range template.WorkspaceActions {
		if !installed[action.Skill] {
			t.Fatalf("workspace action %q needs skill %q, which the template does not install", action.ID, action.Skill)
		}
	}
}

func TestDefaultWorkspaceActionsAreNotAliased(t *testing.T) {
	first, err := Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template failed: %v", err)
	}
	first.WorkspaceActions[0].ID = "tampered"
	second, err := Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template again failed: %v", err)
	}
	if got := second.WorkspaceActions[0].ID; got != processWorkspaceActions[0].ID {
		t.Fatalf("builtin workspace actions were mutated through the returned slice: id = %q", got)
	}
}

func TestDefaultTemplateDeclaresItsStages(t *testing.T) {
	got := Default().Stages
	if !reflect.DeepEqual(got, processStages) {
		t.Fatalf("stages = %+v, want %+v", got, processStages)
	}
	for _, stage := range got {
		if stage.ID == "" {
			t.Fatalf("stage %+v has an empty id", stage)
		}
		if stage.Label == "" {
			t.Fatalf("stage %q has an empty label", stage.ID)
		}
		if stage.Summary == "" {
			t.Fatalf("stage %q has an empty summary", stage.ID)
		}
	}
}

// A stage names the step that advances it, so that step must be one the process
// actually declares: a stage pointing at an action nobody offers would tell a
// person to take a step the workspace cannot run. The terminal stage names no
// step at all, and must say so by leaving Action empty.
func TestStagesReferenceDeclaredActions(t *testing.T) {
	template := Default()
	specActions := make(map[string]bool, len(template.Actions))
	for _, action := range template.Actions {
		specActions[action.ID] = true
	}
	workspaceActions := make(map[string]bool, len(template.WorkspaceActions))
	for _, action := range template.WorkspaceActions {
		workspaceActions[action.ID] = true
	}
	for _, stage := range template.Stages {
		switch stage.Scope {
		case ScopeWorkspace:
			if !workspaceActions[stage.Action] {
				t.Fatalf("stage %q names workspace action %q, which the template does not declare", stage.ID, stage.Action)
			}
		case ScopeSpec:
			if !specActions[stage.Action] {
				t.Fatalf("stage %q names spec action %q, which the template does not declare", stage.ID, stage.Action)
			}
		case "":
			if stage.Action != "" {
				t.Fatalf("terminal stage %q names action %q, want no action", stage.ID, stage.Action)
			}
		default:
			t.Fatalf("stage %q has unknown scope %q", stage.ID, stage.Scope)
		}
	}
}

// The terminal stage is the answer given when no step is pending, so it must be
// reachable only after every other stage has been ruled out: exactly one stage
// carries no scope, and it is the last of the list.
func TestTerminalStageIsLastAndUnique(t *testing.T) {
	stages := Default().Stages
	if len(stages) == 0 {
		t.Fatalf("the template declares no stage")
	}
	terminals := 0
	for _, stage := range stages {
		if stage.Scope == "" {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("stages declare %d terminal stages, want exactly 1", terminals)
	}
	if last := stages[len(stages)-1]; last.Scope != "" {
		t.Fatalf("last stage %q has scope %q, want the terminal stage last", last.ID, last.Scope)
	}
}

func TestDefaultStagesAreNotAliased(t *testing.T) {
	first, err := Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template failed: %v", err)
	}
	first.Stages[0].ID = "tampered"
	second, err := Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template again failed: %v", err)
	}
	if got := second.Stages[0].ID; got != processStages[0].ID {
		t.Fatalf("builtin stages were mutated through the returned slice: id = %q", got)
	}
}
