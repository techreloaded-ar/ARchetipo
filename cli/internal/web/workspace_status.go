package web

// This file answers a different question from workspace_execution.go, and the
// two must not be merged. GET /api/workspace/actions serves the Execution panel
// of the PRD modal: it is a menu — "what can I press here" — and it carries the
// running execution because that panel is where a run is watched. This route
// answers "where am I in the process, and what comes next", is strictly
// read-only, carries no execution, and is re-read by the board on every
// board_changed event so the answer follows the workspace as it changes.
//
// The sequence of the process is not written here: it is declared by the
// Archetipo as template.Stages, and this file only decides which of those
// stages is true right now, given the real state of the workspace.

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// specStageRemedy is what unlocks a spec-scoped step whose refusal is not
// itself an actionable condition. It is deliberately generic: the process
// declares which statuses admit the action, not how a spec gets into one of
// them.
const specStageRemedy = "porta la spec in uno stato che ammette questa azione"

// stageView is the stage in the words of the process. Summary is carried
// because the point of the route is to be readable by someone who does not know
// the process: an id and a label alone would still require knowing it.
type stageView struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Summary string `json:"summary"`
}

// nextStepSpecView names the spec a spec-scoped step acts on. It is the minimum
// a client needs to render the step and open the card, not a second copy of the
// spec detail.
type nextStepSpecView struct {
	Code   string        `json:"code"`
	Title  string        `json:"title"`
	Status domain.Status `json:"status"`
}

// nextStepView is the recommended step: which action advances the current
// stage, on what, and whether it can be started right now. Spec is null for a
// workspace-scoped step, which is how a client tells the two apart without
// parsing Scope.
type nextStepView struct {
	Scope             string            `json:"scope"`
	Action            string            `json:"action"`
	Label             string            `json:"label"`
	Skill             string            `json:"skill"`
	Spec              *nextStepSpecView `json:"spec"`
	Runnable          bool              `json:"runnable"`
	UnavailableReason string            `json:"unavailable_reason,omitempty"`
	UnlockedBy        string            `json:"unlocked_by,omitempty"`
	// RunningExecutionID is the run already under way on this very target, and
	// is present exactly when that is what refuses the step. It is carried
	// because "you cannot start this, it is already running" is the one refusal
	// a person does not answer by satisfying a condition: they answer it by
	// going to the run. Without the id a client can only offer an inert button
	// next to a sentence naming a run it has no way to reach — which is a step
	// recommended, refused, and unreachable at the same time.
	RunningExecutionID string `json:"running_execution_id,omitempty"`
}

// workspaceStatusView is the whole answer in one payload, because the three
// parts are one thought: the workspace is at this stage, therefore this is the
// next step, and these are the workspace actions with the condition that
// unlocks each. NextStep is null at a terminal stage — nothing is pending — and
// that is an answer, not an absence.
type workspaceStatusView struct {
	Template   templateView          `json:"template"`
	Stage      stageView             `json:"stage"`
	NextStep   *nextStepView         `json:"next_step"`
	HasPRD     bool                  `json:"has_prd"`
	HasBacklog bool                  `json:"has_backlog"`
	Actions    []workspaceActionView `json:"actions"`
}

// stageInputs are the facts a stage condition is evaluated against. The
// template says which stages exist and in which order; these say which one is
// true right now.
type stageInputs struct {
	offersWorkspaceAction func(actionID string) bool
	specsInOrder          []domain.Spec
}

// resolveStage returns the first stage of the Archetipo whose condition holds,
// and the spec it is about when the stage is spec-scoped.
//
// It is a pure function taking the Archetipo as a parameter on purpose: no
// stage id and no ordering of the process lives in the server, so a different
// Archetipo produces a different sequence without a line changing here. A
// template whose stages never hold — one with no terminal stage — yields the
// zero Stage and nil, which the caller renders as an unknown stage. It is never
// a panic and never a failed request: an incoherent process is not a broken
// viewer.
func resolveStage(tpl template.Template, in stageInputs) (template.Stage, *domain.Spec) {
	for _, stage := range tpl.Stages {
		switch stage.Scope {
		case template.ScopeWorkspace:
			if in.offersWorkspaceAction != nil && in.offersWorkspaceAction(stage.Action) {
				return stage, nil
			}
		case template.ScopeSpec:
			// The statuses that admit the action are the ones the Archetipo
			// declares for it: the table of states is read, never rewritten.
			statuses := actionStatuses(tpl, stage.Action)
			for i := range in.specsInOrder {
				if admitsStatus(statuses, in.specsInOrder[i].Status) {
					// The first spec in board order, so the recommended step
					// points at the very card the board shows on top.
					spec := in.specsInOrder[i]
					return stage, &spec
				}
			}
		case "":
			// A stage with no scope declares no condition: it is the terminal
			// one, and it holds as soon as the stages before it do not.
			return stage, nil
		}
	}
	return template.Stage{}, nil
}

// actionStatuses returns the workflow statuses in which the process admits the
// spec action, or nil when it declares no such action.
func actionStatuses(tpl template.Template, actionID string) []domain.Status {
	for _, action := range tpl.Actions {
		if action.ID == actionID {
			return action.Statuses
		}
	}
	return nil
}

func admitsStatus(statuses []domain.Status, status domain.Status) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}

// handleGetWorkspaceStatus serves GET /api/workspace/status.
//
// With ?spec=<code> the answer is scoped to that one spec: the stage walk sees
// only it, skips the workspace-scoped stages, and the recommended step becomes
// the next action for that spec — or nothing, when the spec is done. HasPRD,
// HasBacklog and Actions stay workspace-wide: they describe the workspace the
// spec lives in, not the spec.
func (s *Server) handleGetWorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := r.Context()
	// One availability per request, like every other route: the answer must be
	// a single consistent reading of the workspace, not several taken moments
	// apart.
	availability := s.workspaceAvailability(ctx, ws)
	inputs := stageInputs{}
	if scopeCode := strings.TrimSpace(r.URL.Query().Get("spec")); scopeCode != "" {
		scoped, err := ws.conn.ReadSpecDetail(ctx, scopeCode)
		if err != nil {
			writeError(w, err)
			return
		}
		inputs.specsInOrder = []domain.Spec{scoped}
	} else {
		specs, err := ws.conn.FetchBacklogItems(ctx, "")
		if err != nil {
			// A workspace with no backlog yet is exactly the workspace this route
			// exists for: reading its missing precondition as a failure would deny
			// an answer to the only person who cannot guess it. Anything else is a
			// viewer that genuinely cannot answer.
			var coded *iox.CodedError
			if !errors.As(err, &coded) || coded.Code != iox.CodePreconditionMissing {
				writeError(w, err)
				return
			}
			specs = nil
		}
		inputs.specsInOrder = s.specsInBoardOrder(ctx, ws, specs)
		// A stage is reached when the workspace *admits* the step, which is the
		// state half of the decision alone: an unusable provider must not move
		// the workspace to a later stage, it must leave it here with a reason.
		inputs.offersWorkspaceAction = func(id string) bool { return availability.offers(id) == "" }
	}

	stage, targetSpec := resolveStage(tpl, inputs)

	writeJSON(w, http.StatusOK, workspaceStatusView{
		Template:   newTemplateView(tpl),
		Stage:      stageView{ID: stage.ID, Label: stage.Label, Summary: stage.Summary},
		NextStep:   s.nextStepFor(ctx, ws, tpl, stage, targetSpec, availability),
		HasPRD:     availability.hasPRD,
		HasBacklog: availability.hasBacklog,
		Actions:    workspaceActionViews(tpl.WorkspaceActions, availability),
	})
}

// nextStepFor turns the resolved stage into the step that advances it, or nil
// when nothing is pending. A spec-scoped stage without a spec is treated as
// terminal: it can only come from an incoherent Archetipo, and answering "do
// this, on nothing" would be worse than answering "nothing is pending".
func (s *Server) nextStepFor(
	ctx context.Context,
	ws *workspaceSession,
	tpl template.Template,
	stage template.Stage,
	targetSpec *domain.Spec,
	availability workspaceAvailability,
) *nextStepView {
	if stage.Action == "" {
		return nil
	}
	switch stage.Scope {
	case template.ScopeWorkspace:
		step := &nextStepView{Scope: stage.Scope, Action: stage.Action}
		for _, action := range tpl.WorkspaceActions {
			if action.ID == stage.Action {
				step.Label = action.Label
				step.Skill = action.Skill
				break
			}
		}
		reason := availability.reasonFor(stage.Action)
		step.Runnable = reason == ""
		step.UnavailableReason = reason
		if reason != "" {
			step.UnlockedBy = workspaceRemedy(availability, stage.Action)
		}
		return step
	case template.ScopeSpec:
		if targetSpec == nil {
			return nil
		}
		step := &nextStepView{
			Scope:  stage.Scope,
			Action: stage.Action,
			Spec: &nextStepSpecView{
				Code:   targetSpec.Code,
				Title:  targetSpec.Title,
				Status: targetSpec.Status,
			},
		}
		for _, action := range tpl.Actions {
			if action.ID == stage.Action {
				step.Label = action.Label
				step.Skill = action.Skill
				break
			}
		}
		// The plan is read even though only the implementation depends on it:
		// a step reported runnable without checking it would be a promise the
		// spec route immediately refuses.
		tasks, _, err := s.readPlanForSpec(ctx, ws, targetSpec.Code)
		if err != nil {
			tasks = nil
		}
		availability := s.actionAvailabilityFor(ctx, ws, targetSpec.Code, len(tasks))
		reason := availability.reasonFor(stage.Action)
		step.Runnable = reason == ""
		step.UnavailableReason = reason
		// The run that refuses the step travels with the refusal, and only when
		// it is the refusal: an id next to a step blocked by anything else
		// would name a run that has nothing to do with why it is blocked.
		if !step.Runnable && availability.specHasRunning {
			step.RunningExecutionID = availability.runningID
		}
		if reason != "" {
			// The spec-scoped refusals are already the condition to satisfy —
			// install the provider, plan the spec, wait for the run — so the
			// remedy is the reason itself. Only a stage whose action the
			// process does not admit here needs the generic sentence.
			step.UnlockedBy = reason
			if !admitsStatus(actionStatuses(tpl, stage.Action), targetSpec.Status) {
				step.UnlockedBy = specStageRemedy
			}
		}
		return step
	default:
		return nil
	}
}
