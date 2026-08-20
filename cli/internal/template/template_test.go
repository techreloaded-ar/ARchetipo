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

func TestRegistryRejectsEmptyAndDuplicateIDs(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Template{ID: "  "}); err == nil {
		t.Fatal("expected an empty id to be rejected")
	}
	if err := registry.Register(Template{ID: "one"}); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := registry.Register(Template{ID: "one"}); err == nil {
		t.Fatal("expected a duplicate id to be rejected")
	}
	if got := registry.IDs(); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("ids = %v, want [one]", got)
	}
}

func TestBuiltinRegistryListsTheDefaultTemplate(t *testing.T) {
	if got := Builtin().IDs(); !reflect.DeepEqual(got, []string{FabbricaDelSoftware}) {
		t.Fatalf("ids = %v, want [%s]", got, FabbricaDelSoftware)
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

// TestDefaultActionsAreNotAliased resolves twice from the SAME registry on
// purpose. Two calls to Default() each build a fresh registry, so they can
// never alias each other and the test would pass even with no copy at all:
// only a second resolution of the same instance can observe a write that
// reached the registry.
func TestDefaultActionsAreNotAliased(t *testing.T) {
	registry := Builtin()
	first, err := registry.Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template failed: %v", err)
	}
	first.Actions[0].ID = "tampered"
	first.Actions[0].Statuses[0] = domain.Status("TAMPERED")
	second, err := registry.Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template again failed: %v", err)
	}
	if got := second.Actions[0].ID; got != processActions[0].ID {
		t.Fatalf("registry actions were mutated through the returned slice: id = %q", got)
	}
	if got := second.Actions[0].Statuses[0]; got != processActions[0].Statuses[0] {
		t.Fatalf("registry action statuses were mutated through the returned slice: status = %q", got)
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

// TestDefaultWorkspaceActionsAreNotAliased resolves twice from the SAME
// registry, for the same reason as TestDefaultActionsAreNotAliased: two calls
// to Default() build two registries and could never alias each other.
func TestDefaultWorkspaceActionsAreNotAliased(t *testing.T) {
	registry := Builtin()
	first, err := registry.Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template failed: %v", err)
	}
	first.WorkspaceActions[0].ID = "tampered"
	second, err := registry.Resolve(DefaultID)
	if err != nil {
		t.Fatalf("resolving the default template again failed: %v", err)
	}
	if got := second.WorkspaceActions[0].ID; got != processWorkspaceActions[0].ID {
		t.Fatalf("registry workspace actions were mutated through the returned slice: id = %q", got)
	}
}
