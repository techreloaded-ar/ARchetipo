package execution

import (
	"encoding/json"
	"fmt"
)

// SpecDraftProposal is the reviewable content of a receipt, without lifecycle
// metadata. Both batch and native actions expose it to the same creation form.
type SpecDraftProposal struct {
	Title     string   `json:"title"`
	EpicCode  string   `json:"epic_code"`
	EpicTitle string   `json:"epic_title,omitempty"`
	Priority  string   `json:"priority"`
	Points    int      `json:"points"`
	Scope     string   `json:"scope"`
	BlockedBy []string `json:"blocked_by"`
	Body      string   `json:"body"`
}

// ActionResultFields projects a validated receipt into the existing result
// contract. No harness output, credentials or surrounding prose are retained.
func ActionResultFields(action ActionID, receipt json.RawMessage) (map[string]any, error) {
	fields := map[string]any{}
	var target any
	switch action {
	case ActionPlan:
		target = &PlanReceipt{}
	case ActionImplement:
		target = &ImplementReceipt{}
	case ActionReview:
		target = &ReviewReceipt{}
	case ActionInception:
		target = &PRDReceipt{}
	case ActionBacklog:
		target = &BacklogReceipt{}
	case ActionSpecDraft:
		target = &SpecDraftReceipt{}
	default:
		return nil, &ActionError{Action: action}
	}
	if err := json.Unmarshal(receipt, target); err != nil {
		return nil, fmt.Errorf("decode %s receipt: %w", action, err)
	}
	switch got := target.(type) {
	case *PlanReceipt:
		fields["plan_tasks"] = got.Tasks
	case *ImplementReceipt:
		fields["tasks_done"], fields["tests"] = got.TasksDone, got.Tests
	case *ReviewReceipt:
		fields["criteria"], fields["blockers"] = got.Criteria, got.Blockers
	case *PRDReceipt:
		fields["prd_path"] = got.Path
	case *BacklogReceipt:
		fields["backlog_epics"], fields["backlog_specs"] = got.Epics, got.Specs
	case *SpecDraftReceipt:
		if got.Proposals != nil {
			fields["spec_drafts"] = got.Drafts()
		} else {
			fields["spec_draft"] = got.Drafts()[0]
		}
	}
	return fields, nil
}
