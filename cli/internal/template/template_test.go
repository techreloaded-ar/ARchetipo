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
