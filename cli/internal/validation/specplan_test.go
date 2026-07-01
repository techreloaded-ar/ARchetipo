package validation

import (
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

func findingCodes(r domain.ValidationResult) map[string]int {
	codes := map[string]int{}
	for _, f := range r.Findings {
		codes[f.Code]++
	}
	return codes
}

func checkStatuses(r domain.ValidationResult) map[string]string {
	statuses := map[string]string{}
	for _, check := range r.Checks {
		statuses[check.Code] = check.Status
	}
	return statuses
}

func validSpec() domain.Spec {
	return domain.Spec{
		Code:     "US-001",
		Title:    "First",
		Priority: domain.PriorityHigh,
		Points:   3,
		Status:   "TODO",
		Epic:     domain.Epic{Code: "EP-001", Title: "Epic"},
		Body:     "**User Story**\nAs a user.\n\n**Demonstrates**\nReviewer sees X.\n\n**Acceptance**\n- [ ] X works",
	}
}

func TestValidateSpecs_Valid(t *testing.T) {
	r := ValidateSpecs("specs.yaml", []domain.Spec{validSpec()})
	if !r.OK {
		t.Fatalf("expected ok, got findings %+v", r.Findings)
	}
	if r.Artifact != "spec" {
		t.Fatalf("expected artifact=spec, got %s", r.Artifact)
	}
	if len(r.Checks) == 0 {
		t.Fatal("expected non-empty checks")
	}
	statuses := checkStatuses(r)
	for _, code := range []string{"SPECS_NOT_EMPTY", "SPEC_CODES_VALID", "SPEC_BODIES_COMPLETE", "SPEC_BLOCKERS_CHECKED"} {
		if statuses[code] != "passed" {
			t.Fatalf("expected %s=passed, got %q", code, statuses[code])
		}
	}
}

func TestValidateSpecs_Empty(t *testing.T) {
	r := ValidateSpecs("specs.yaml", nil)
	if r.OK {
		t.Fatalf("expected not ok for empty payload")
	}
	if findingCodes(r)["SPECS_EMPTY"] != 1 {
		t.Fatalf("expected SPECS_EMPTY, got %+v", r.Findings)
	}
	if checkStatuses(r)["SPECS_NOT_EMPTY"] != "failed" {
		t.Fatalf("expected SPECS_NOT_EMPTY=failed, got %+v", r.Checks)
	}
}

func TestValidateSpecs_StructuralErrors(t *testing.T) {
	bad := domain.Spec{Code: "1", Title: "", Priority: "URGENT", Points: 0, Status: "", Epic: domain.Epic{Code: "E1"}, Body: "no checklist"}
	r := ValidateSpecs("specs.yaml", []domain.Spec{bad})
	if r.OK {
		t.Fatalf("expected not ok")
	}
	codes := findingCodes(r)
	for _, want := range []string{
		"SPEC_CODE_INVALID", "SPEC_TITLE_EMPTY", "SPEC_EPIC_INVALID",
		"SPEC_PRIORITY_INVALID", "SPEC_POINTS_INVALID", "SPEC_STATUS_EMPTY",
		"SPEC_DEMONSTRATES_MISSING", "SPEC_ACCEPTANCE_MISSING",
	} {
		if codes[want] == 0 {
			t.Errorf("expected finding %s, got %+v", want, r.Findings)
		}
	}
	statuses := checkStatuses(r)
	for _, code := range []string{"SPEC_CODES_VALID", "SPEC_TITLES_PRESENT", "SPEC_EPICS_VALID", "SPEC_PRIORITIES_VALID", "SPEC_POINTS_VALID", "SPEC_STATUSES_PRESENT", "SPEC_BODIES_COMPLETE"} {
		if statuses[code] != "failed" {
			t.Errorf("expected %s=failed, got %q", code, statuses[code])
		}
	}
}

func TestValidateSpecs_UnknownBlockerIsWarning(t *testing.T) {
	s := validSpec()
	s.BlockedBy = []string{"US-999"}
	r := ValidateSpecs("specs.yaml", []domain.Spec{s})
	if !r.OK {
		t.Fatalf("warnings must not block: %+v", r.Findings)
	}
	if findingCodes(r)["SPEC_BLOCKER_UNKNOWN"] != 1 {
		t.Fatalf("expected SPEC_BLOCKER_UNKNOWN warning, got %+v", r.Findings)
	}
	if checkStatuses(r)["SPEC_BLOCKERS_CHECKED"] != "warning" {
		t.Fatalf("expected SPEC_BLOCKERS_CHECKED=warning, got %+v", r.Checks)
	}
}

func canonicalTask(id string, t domain.TaskType, deps ...string) domain.Task {
	return domain.Task{
		ID:           id,
		Title:        "Task " + id,
		Type:         t,
		Status:       "TODO",
		Dependencies: deps,
		Body:         "## Descrizione\nFare il lavoro.\n\n## File Coinvolti\n- internal/x.go — logica\n\n## Criteri di Completamento\n- [ ] fatto",
	}
}

func TestValidatePlan_Valid(t *testing.T) {
	input := domain.PlanInput{
		PlanBody: "## Plan\nDo it",
		Tasks: []domain.Task{
			canonicalTask("TASK-01", domain.TaskImpl),
			canonicalTask("TASK-02", domain.TaskTest, "TASK-01"),
		},
	}
	r := ValidatePlan("plan.yaml", "US-001", input)
	if !r.OK {
		t.Fatalf("expected ok, got %+v", r.Findings)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("expected no findings for canonical plan, got %+v", r.Findings)
	}
	if len(r.Checks) == 0 {
		t.Fatal("expected non-empty checks")
	}
	statuses := checkStatuses(r)
	for _, code := range []string{"PLAN_SPEC_CODE_VALID", "PLAN_BODY_PRESENT", "PLAN_TASKS_PRESENT", "PLAN_TASK_IDS_VALID", "PLAN_TASK_BODIES_COMPLETE", "PLAN_TEST_TASK_PRESENT", "PLAN_TASK_DEPENDENCIES_VALID"} {
		if statuses[code] != "passed" {
			t.Fatalf("expected %s=passed, got %q", code, statuses[code])
		}
	}
}

func TestValidatePlan_DependencyAndMissingTest(t *testing.T) {
	input := domain.PlanInput{
		PlanBody: "## Plan",
		Tasks: []domain.Task{
			canonicalTask("TASK-01", domain.TaskImpl, "TASK-99"),
		},
	}
	r := ValidatePlan("plan.yaml", "US-001", input)
	if r.OK {
		t.Fatalf("expected not ok")
	}
	codes := findingCodes(r)
	if codes["PLAN_TASK_DEP_UNKNOWN"] == 0 || codes["PLAN_TEST_TASK_MISSING"] == 0 {
		t.Fatalf("expected dependency and missing-test findings, got %+v", r.Findings)
	}
	statuses := checkStatuses(r)
	if statuses["PLAN_TEST_TASK_PRESENT"] != "failed" {
		t.Fatalf("expected PLAN_TEST_TASK_PRESENT=failed, got %q", statuses["PLAN_TEST_TASK_PRESENT"])
	}
	if statuses["PLAN_TASK_DEPENDENCIES_VALID"] != "failed" {
		t.Fatalf("expected PLAN_TASK_DEPENDENCIES_VALID=failed, got %q", statuses["PLAN_TASK_DEPENDENCIES_VALID"])
	}
}

func TestValidatePlan_WeakContractIsWarning(t *testing.T) {
	weak := canonicalTask("TASK-01", domain.TaskImpl)
	weak.Body = "fai qualcosa"
	input := domain.PlanInput{
		PlanBody: "## Plan",
		Tasks: []domain.Task{
			weak,
			canonicalTask("TASK-02", domain.TaskTest, "TASK-01"),
		},
	}
	r := ValidatePlan("plan.yaml", "US-001", input)
	if !r.OK {
		t.Fatalf("weak contract is a warning, must not block: %+v", r.Findings)
	}
	if findingCodes(r)["PLAN_TASK_CONTRACT_WEAK"] == 0 {
		t.Fatalf("expected PLAN_TASK_CONTRACT_WEAK warning, got %+v", r.Findings)
	}
	if checkStatuses(r)["PLAN_TASK_BODIES_COMPLETE"] != "warning" {
		t.Fatalf("expected PLAN_TASK_BODIES_COMPLETE=warning, got %+v", r.Checks)
	}
}

func TestValidatePlan_Cycle(t *testing.T) {
	input := domain.PlanInput{
		PlanBody: "## Plan",
		Tasks: []domain.Task{
			canonicalTask("TASK-01", domain.TaskImpl, "TASK-02"),
			canonicalTask("TASK-02", domain.TaskTest, "TASK-01"),
		},
	}
	r := ValidatePlan("plan.yaml", "US-001", input)
	if findingCodes(r)["PLAN_TASK_DEP_CYCLE"] == 0 {
		t.Fatalf("expected cycle finding, got %+v", r.Findings)
	}
	if checkStatuses(r)["PLAN_TASK_DEPENDENCIES_VALID"] != "failed" {
		t.Fatalf("expected PLAN_TASK_DEPENDENCIES_VALID=failed, got %+v", r.Checks)
	}
}
