// Package template models the process Template that shapes an ARchetipo
// workspace: which skills the workspace installs and which workflow statuses
// its process uses.
//
// Before Templates existed, both were implicit: the skill list lived in a
// variable inside the init command and the statuses only in the packaged
// config.yaml, so a workspace could not name the process it was running.
// A Template makes that process a declared, addressable object without
// changing what a default initialization produces.
//
// Templates are resolved in-process by id, the same way execution providers
// are: the registry is the single place where a process is written down.
package template

import (
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

const (
	// FabbricaDelSoftware is the id of the built-in Template: the ARchetipo
	// process as it ships today.
	FabbricaDelSoftware = "fabbrica-del-software"

	// DefaultID is the Template selected when the user does not choose one.
	DefaultID = FabbricaDelSoftware
)

// fabbricaVersion is the revision of the built-in process, deliberately not
// version.Version: that value identifies the CLI binary, defaults to "dev" on
// local builds, and would therefore record something that is not the process.
const fabbricaVersion = "1.0.0"

// Template is a declared development process. Skills are the skill directories
// a workspace receives at initialization; Statuses are the workflow labels that
// process works with.
type Template struct {
	ID       string              `json:"id"`
	Version  string              `json:"version"`
	Label    string              `json:"label"`
	Skills   []string            `json:"skills"`
	Statuses domain.StatusLabels `json:"statuses"`
}

// RegistryError is the typed failure of registry operations. Callers branch on
// the type, never on the rendered message.
type RegistryError struct {
	TemplateID string
	Reason     string
}

func (e *RegistryError) Error() string {
	if strings.TrimSpace(e.TemplateID) == "" {
		return fmt.Sprintf("process template %s", e.Reason)
	}
	return fmt.Sprintf("process template %q %s", e.TemplateID, e.Reason)
}

// Registry resolves Templates by id and preserves registration order so the
// list of valid ids is stable in error hints.
type Registry struct {
	templates map[string]Template
	order     []string
}

func NewRegistry() *Registry {
	return &Registry{templates: make(map[string]Template)}
}

func (r *Registry) Register(t Template) error {
	if r == nil || r.templates == nil {
		return &RegistryError{Reason: "registry is not initialized"}
	}
	id := strings.TrimSpace(t.ID)
	if id == "" {
		return &RegistryError{Reason: "has an empty id"}
	}
	if _, exists := r.templates[id]; exists {
		return &RegistryError{TemplateID: id, Reason: "is already registered"}
	}
	t.ID = id
	r.templates[id] = t
	r.order = append(r.order, id)
	return nil
}

// Resolve returns the Template registered under id. An empty id is not an
// error: it means "no explicit selection" and resolves to the default Template,
// which is what keeps an initialization without --template on today's process.
func (r *Registry) Resolve(id string) (Template, error) {
	if r == nil || r.templates == nil {
		return Template{}, &RegistryError{Reason: "registry is not initialized"}
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = DefaultID
	}
	t, ok := r.templates[id]
	if !ok {
		return Template{}, &RegistryError{TemplateID: id, Reason: "is not registered"}
	}
	return t.clone(), nil
}

// IDs lists the registered ids in registration order.
func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// clone detaches the mutable slice so a caller that edits Skills cannot reach
// back into the registry.
func (t Template) clone() Template {
	skills := make([]string, len(t.Skills))
	copy(skills, t.Skills)
	t.Skills = skills
	return t
}

// Builtin returns the registry the CLI runs with. Registering a single static
// Template cannot fail — the registry is fresh and the id is a non-empty
// constant — so the error is deliberately discarded rather than turned into an
// unreachable failure path.
func Builtin() *Registry {
	registry := NewRegistry()
	_ = registry.Register(Template{
		ID:      FabbricaDelSoftware,
		Version: fabbricaVersion,
		Label:   "Fabbrica del software",
		Skills: []string{
			"archetipo-autopilot",
			"archetipo-design",
			"archetipo-implement",
			"archetipo-inception",
			"archetipo-plan",
			"archetipo-review",
			"archetipo-spec",
			"archetipo-wiki",
		},
		Statuses: domain.StatusLabels{
			Todo:       string(domain.StatusTodo),
			Planned:    string(domain.StatusPlanned),
			InProgress: string(domain.StatusInProgress),
			Review:     string(domain.StatusReview),
			Done:       string(domain.StatusDone),
		},
	})
	return registry
}

// Default returns the Template used when no selection is made.
func Default() Template {
	t, _ := Builtin().Resolve(DefaultID)
	return t
}

// Resolve looks up id in the built-in registry. An empty id yields Default().
func Resolve(id string) (Template, error) {
	return Builtin().Resolve(id)
}
