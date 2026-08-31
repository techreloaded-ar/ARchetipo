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
	"slices"
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

// Action is a step the process offers on a spec. ID is stable and is what a
// program keys on; Label is what a person reads; Skill is the capability that
// realizes it; Statuses are the workflow statuses in which it is admissible.
type Action struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Skill    string          `json:"skill"`
	Statuses []domain.Status `json:"statuses"`
}

// clone detaches the mutable slice so a caller that edits Statuses cannot reach
// back into the registry.
func (a Action) clone() Action {
	a.Statuses = slices.Clone(a.Statuses)
	return a
}

// WorkspaceAction is a step the process offers on the workspace as a whole
// rather than on a spec. It deliberately has no Statuses field: an action on a
// spec is admissible in a set of workflow statuses, which the process knows,
// while an action on the workspace is admissible under a condition the process
// does not know — whether a PRD already exists, for instance, is a fact about
// the filesystem that only the server can establish. Declaring the action here
// says the process offers it; whether it can run now is decided elsewhere.
type WorkspaceAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Skill string `json:"skill"`
}

const (
	// ScopeWorkspace marks a Stage advanced by a WorkspaceAction, that is by a
	// step the process offers on the workspace as a whole.
	ScopeWorkspace = "workspace"

	// ScopeSpec marks a Stage advanced by an Action, that is by a step the
	// process offers on a single spec.
	ScopeSpec = "spec"
)

// Stage is one point of the process a workspace can be at. ID is stable and is
// what a program keys on; Label is what a person reads; Summary explains the
// stage in the words of the process; Scope says whether the step that advances
// it acts on the workspace (ScopeWorkspace) or on a spec (ScopeSpec); Action is
// the id of that step, an Action id when Scope is ScopeSpec and a
// WorkspaceAction id when it is ScopeWorkspace.
//
// A Stage with both Scope and Action empty is the terminal stage — no step is
// pending — and must be the last of the list.
//
// A Stage declares *which* step advances it, never *when* it holds: the
// condition of each stage belongs to the caller, because this package knows
// nothing about the state of a workspace and reads no filesystem.
type Stage struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Summary string `json:"summary"`
	Scope   string `json:"scope"`
	Action  string `json:"action"`
}

// Template is a declared development process. Skills are the skill directories
// a workspace receives at initialization; Actions are the steps the process
// offers on a spec and the statuses that admit them; WorkspaceActions are the
// steps it offers on the workspace itself; Stages are the ordered sequence of
// the process, where the first stage whose condition holds is the current one —
// the condition being evaluated by the caller, never here; Statuses are the
// workflow labels that process works with.
type Template struct {
	ID               string              `json:"id"`
	Version          string              `json:"version"`
	Label            string              `json:"label"`
	Skills           []string            `json:"skills"`
	Actions          []Action            `json:"actions"`
	WorkspaceActions []WorkspaceAction   `json:"workspace_actions"`
	Stages           []Stage             `json:"stages"`
	Statuses         domain.StatusLabels `json:"statuses"`
}

// ActionsFor returns the actions the process admits in status, in declaration
// order. The result is always non-nil: a status with no admissible action is
// an empty list, never an absence and never a failure.
func (t Template) ActionsFor(status domain.Status) []Action {
	out := make([]Action, 0, len(t.Actions))
	for _, action := range t.Actions {
		for _, candidate := range action.Statuses {
			if candidate == status {
				out = append(out, action.clone())
				break
			}
		}
	}
	return out
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
	return slices.Clone(r.order)
}

// clone detaches the mutable slices so a caller that edits Skills or Actions
// cannot reach back into the registry.
func (t Template) clone() Template {
	t.Skills = slices.Clone(t.Skills)
	actions := make([]Action, len(t.Actions))
	for i, action := range t.Actions {
		actions[i] = action.clone()
	}
	t.Actions = actions
	t.WorkspaceActions = slices.Clone(t.WorkspaceActions)
	t.Stages = slices.Clone(t.Stages)
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
		Actions: []Action{
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
		},
		WorkspaceActions: []WorkspaceAction{
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
		},
		Stages: []Stage{
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
