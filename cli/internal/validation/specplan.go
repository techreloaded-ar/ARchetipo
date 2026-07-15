package validation

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// Structural identifier patterns shared by the spec and plan validators.
var (
	specCodeRE = regexp.MustCompile(`^US-\d{3,}$`)
	epicCodeRE = regexp.MustCompile(`^EP-\d{3,}$`)
	taskIDRE   = regexp.MustCompile(`^TASK-\d{2,}$`)
)

// ValidateSpecs runs deterministic structural rules over a spec add payload
// (the specs a skill is about to persist) and returns a ValidationResult.
// Both validators return a validation_result envelope with ok:false for
// structural invalidity; error envelopes are reserved for process failures.
func ValidateSpecs(target string, specs []domain.Spec) domain.ValidationResult {
	findings := []domain.ValidationFinding{}
	if len(specs) == 0 {
		findings = addFinding(findings, SeverityError, "SPECS_EMPTY", "specs", "payload must include at least one spec", "expected {specs:[...]}")
		return specResult(target, findings, buildChecks(specCheckRules, findings))
	}
	seen := map[string]struct{}{}
	for i, spec := range specs {
		base := fmt.Sprintf("specs[%d]", i)
		if !specCodeRE.MatchString(spec.Code) {
			findings = addFinding(findings, SeverityError, "SPEC_CODE_INVALID", base+".code", "spec code must match US-NNN", "use a zero-padded code such as US-001")
		}
		if _, ok := seen[spec.Code]; spec.Code != "" && ok {
			findings = addFinding(findings, SeverityError, "SPEC_CODE_DUPLICATE", base+".code", "spec code is duplicated", "each spec in the payload must have a unique code")
		}
		seen[spec.Code] = struct{}{}
		if strings.TrimSpace(spec.Title) == "" {
			findings = addFinding(findings, SeverityError, "SPEC_TITLE_EMPTY", base+".title", "spec title is required", "")
		}
		if !epicCodeRE.MatchString(spec.Epic.Code) {
			findings = addFinding(findings, SeverityError, "SPEC_EPIC_INVALID", base+".epic.code", "epic code must match EP-NNN", "assign the spec to an explicit epic")
		}
		if !validPriority(spec.Priority) {
			findings = addFinding(findings, SeverityError, "SPEC_PRIORITY_INVALID", base+".priority", "priority must be HIGH, MEDIUM, or LOW", "")
		}
		if spec.Points <= 0 {
			findings = addFinding(findings, SeverityError, "SPEC_POINTS_INVALID", base+".points", "points must be greater than zero", "")
		}
		if spec.Status == "" {
			findings = addFinding(findings, SeverityError, "SPEC_STATUS_EMPTY", base+".status", "status is required", "use the configured TODO status for new specs")
		}
		body := strings.TrimSpace(spec.Body)
		if body == "" {
			findings = addFinding(findings, SeverityError, "SPEC_BODY_EMPTY", base+".body", "spec body is required", "include user story, Demonstrates, and acceptance criteria")
			continue
		}
		lower := strings.ToLower(body)
		if !strings.Contains(lower, "demonstr") && !strings.Contains(lower, "dimostra") {
			findings = addFinding(findings, SeverityError, "SPEC_DEMONSTRATES_MISSING", base+".body", "spec body must include a concrete Demonstrates section", "state what a reviewer can observe after implementation")
		}
		if !strings.Contains(body, "- [ ]") {
			findings = addFinding(findings, SeverityError, "SPEC_ACCEPTANCE_MISSING", base+".body", "spec body must include checklist acceptance criteria", "add one or more '- [ ]' acceptance criteria")
		}
	}
	for i, spec := range specs {
		for _, dep := range spec.BlockedBy {
			if _, ok := seen[dep]; dep != "" && !ok {
				findings = addFinding(findings, SeverityWarning, "SPEC_BLOCKER_UNKNOWN", fmt.Sprintf("specs[%d].blocked_by", i), fmt.Sprintf("blocked_by references %s, which is not in this payload", dep), "ensure the dependency already exists in the backlog")
			}
		}
	}
	return specResult(target, findings, buildChecks(specCheckRules, findings))
}

// ValidatePlan runs deterministic structural rules over a plan payload for a
// single spec and returns a ValidationResult with ok:false on errors.
func ValidatePlan(target, specCode string, input domain.PlanInput) domain.ValidationResult {
	findings := []domain.ValidationFinding{}
	if !specCodeRE.MatchString(specCode) {
		findings = addFinding(findings, SeverityError, "PLAN_SPEC_CODE_INVALID", "spec_code", "spec code must match US-NNN", "")
	}
	if strings.TrimSpace(input.PlanBody) == "" {
		findings = addFinding(findings, SeverityError, "PLAN_BODY_EMPTY", "plan_body", "plan body is required", "")
	}
	if len(input.Tasks) == 0 {
		findings = addFinding(findings, SeverityError, "PLAN_TASKS_EMPTY", "tasks", "plan must include at least one task", "")
		return planResult(target, findings, buildChecks(planCheckRules, findings))
	}
	if len(input.Tasks) > 15 {
		findings = addFinding(findings, SeverityWarning, "PLAN_TOO_MANY_TASKS", "tasks", "plan has more than 15 tasks", "consider splitting the spec")
	}
	ids := map[string]int{}
	hasTest := false
	for i, task := range input.Tasks {
		base := fmt.Sprintf("tasks[%d]", i)
		if !taskIDRE.MatchString(task.ID) {
			findings = addFinding(findings, SeverityError, "PLAN_TASK_ID_INVALID", base+".id", "task id must match TASK-NN", "")
		}
		if prev, ok := ids[task.ID]; task.ID != "" && ok {
			findings = addFinding(findings, SeverityError, "PLAN_TASK_ID_DUPLICATE", base+".id", fmt.Sprintf("task id duplicates tasks[%d]", prev), "task ids must be unique")
		}
		ids[task.ID] = i
		if strings.TrimSpace(task.Title) == "" {
			findings = addFinding(findings, SeverityError, "PLAN_TASK_TITLE_EMPTY", base+".title", "task title is required", "")
		}
		switch task.Type {
		case domain.TaskImpl:
		case domain.TaskTest:
			hasTest = true
		case domain.TaskFix:
		default:
			findings = addFinding(findings, SeverityError, "PLAN_TASK_TYPE_INVALID", base+".type", "task type must be Impl, Test, or Fix", "Fix is reserved for rework plans")
		}
		if strings.TrimSpace(string(task.Status)) == "" {
			findings = addFinding(findings, SeverityError, "PLAN_TASK_STATUS_EMPTY", base+".status", "task status is required", "use TODO for new tasks")
		}
		if strings.TrimSpace(task.Body) == "" {
			findings = addFinding(findings, SeverityError, "PLAN_TASK_BODY_EMPTY", base+".body", "task body must contain an execution contract", "include objective, allowed changes, steps, verification, done criteria, and blockers")
		} else {
			findings = append(findings, validateTaskContract(base+".body", task.Body)...)
		}
	}
	if !hasTest {
		findings = addFinding(findings, SeverityError, "PLAN_TEST_TASK_MISSING", "tasks", "plan must include at least one Test task", "")
	}
	findings = append(findings, validateTaskDependencies(input.Tasks, ids)...)
	return planResult(target, findings, buildChecks(planCheckRules, findings))
}

func specResult(target string, findings []domain.ValidationFinding, checks []domain.ValidationCheck) domain.ValidationResult {
	return domain.ValidationResult{
		OK:       !hasErrorFinding(findings),
		Artifact: "spec",
		Target:   target,
		Checks:   checks,
		Findings: findings,
	}
}

func planResult(target string, findings []domain.ValidationFinding, checks []domain.ValidationCheck) domain.ValidationResult {
	return domain.ValidationResult{
		OK:       !hasErrorFinding(findings),
		Artifact: "plan",
		Target:   target,
		Checks:   checks,
		Findings: findings,
	}
}

// canonical task body sections, in the order a smaller implementation model
// reads them. Validation matches the markdown sections persisted by `spec plan`.
var requiredTaskSections = []struct{ token, label string }{
	{"objective", "## Objective"},
	{"read", "## Read"},
	{"change", "## Change"},
	{"steps", "## Steps"},
	{"verify", "## Verify"},
	{"done", "## Done"},
	{"blockers", "## Blockers"},
}

func validateTaskContract(path, body string) []domain.ValidationFinding {
	findings := []domain.ValidationFinding{}
	for _, section := range requiredTaskSections {
		if !hasMarkdownH2(body, section.token) {
			findings = addFinding(findings, SeverityWarning, "PLAN_TASK_CONTRACT_WEAK", path, "task execution contract is missing "+section.label, "make the contract explicit for smaller implementation models")
		}
	}
	return findings
}

func hasMarkdownH2(body, heading string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "## "+heading) {
			return true
		}
	}
	return false
}

func validateTaskDependencies(tasks []domain.Task, ids map[string]int) []domain.ValidationFinding {
	findings := []domain.ValidationFinding{}
	graph := map[string][]string{}
	for i, task := range tasks {
		for _, dep := range task.Dependencies {
			dep = strings.TrimSpace(dep)
			depIndex, ok := ids[dep]
			if !ok {
				findings = addFinding(findings, SeverityError, "PLAN_TASK_DEP_UNKNOWN", fmt.Sprintf("tasks[%d].dependencies", i), fmt.Sprintf("%s depends on unknown task %s", task.ID, dep), "dependencies must reference tasks in the same plan")
				continue
			}
			if depIndex >= i {
				findings = addFinding(findings, SeverityError, "PLAN_TASK_DEP_FUTURE", fmt.Sprintf("tasks[%d].dependencies", i), fmt.Sprintf("%s depends on %s, which is not earlier in the task list", task.ID, dep), "order tasks by dependency")
			}
			graph[task.ID] = append(graph[task.ID], dep)
		}
	}
	for _, cycle := range findTaskCycles(graph) {
		findings = addFinding(findings, SeverityError, "PLAN_TASK_DEP_CYCLE", "tasks", "task dependency cycle detected: "+strings.Join(cycle, " -> "), "remove the cycle before saving the plan")
	}
	return findings
}

func findTaskCycles(graph map[string][]string) [][]string {
	seen := map[string]bool{}
	stack := map[string]bool{}
	var cycles [][]string
	var visit func(string, []string)
	visit = func(id string, path []string) {
		if stack[id] {
			start := 0
			for i, p := range path {
				if p == id {
					start = i
					break
				}
			}
			cycles = append(cycles, append(path[start:], id))
			return
		}
		if seen[id] {
			return
		}
		seen[id] = true
		stack[id] = true
		for _, dep := range graph[id] {
			visit(dep, append(path, dep))
		}
		stack[id] = false
	}
	keys := make([]string, 0, len(graph))
	for id := range graph {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		visit(id, []string{id})
	}
	return cycles
}

func validPriority(p domain.Priority) bool {
	switch p {
	case domain.PriorityHigh, domain.PriorityMedium, domain.PriorityLow:
		return true
	default:
		return false
	}
}
