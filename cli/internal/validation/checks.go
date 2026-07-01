package validation

import "github.com/techreloaded-ar/ARchetipo/cli/internal/domain"

// Canonical status values for ValidationCheck.Status.
//
//	CheckPassed  — no findings matched for the rule
//	CheckFailed  — at least one error-severity finding matched
//	CheckWarning — only warning-severity findings matched, no errors
const (
	CheckPassed  = "passed"
	CheckFailed  = "failed"
	CheckWarning = "warning"
)

// Canonical severity values for ValidationFinding.Severity. They pair the
// status constants above: Severities are the source, statuses are derived
// from them by checkStatus.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// validationRule binds a single user-visible check to the set of finding
// codes that influence its aggregated status.
type validationRule struct {
	checkCode    string
	message      string
	findingCodes []string
}

// prdCheckRules maps each PRD structural rule to the finding codes that
// influence its aggregated status.
var prdCheckRules = []validationRule{
	{checkCode: "PRD_NOT_EMPTY", message: "PRD is not empty", findingCodes: []string{"PRD_EMPTY"}},
	{checkCode: "PRD_NO_UNRESOLVED_PLACEHOLDERS", message: "no unresolved {{PLACEHOLDER}} tokens", findingCodes: []string{"PRD_PLACEHOLDER_LEFT"}},
	{checkCode: "PRD_REQUIRED_SECTIONS", message: "all required section markers present with meaningful content", findingCodes: []string{"PRD_MISSING_SECTION", "PRD_SECTION_EMPTY"}},
}

// specCheckRules is the rule table for ValidateSpecs.
var specCheckRules = []validationRule{
	{checkCode: "SPECS_NOT_EMPTY", message: "spec payload includes at least one spec", findingCodes: []string{"SPECS_EMPTY"}},
	{checkCode: "SPEC_CODES_VALID", message: "spec codes are present, unique, and match US-NNN", findingCodes: []string{"SPEC_CODE_INVALID", "SPEC_CODE_DUPLICATE"}},
	{checkCode: "SPEC_TITLES_PRESENT", message: "spec titles are present", findingCodes: []string{"SPEC_TITLE_EMPTY"}},
	{checkCode: "SPEC_EPICS_VALID", message: "spec epics match EP-NNN", findingCodes: []string{"SPEC_EPIC_INVALID"}},
	{checkCode: "SPEC_PRIORITIES_VALID", message: "spec priorities are valid", findingCodes: []string{"SPEC_PRIORITY_INVALID"}},
	{checkCode: "SPEC_POINTS_VALID", message: "spec points are greater than zero", findingCodes: []string{"SPEC_POINTS_INVALID"}},
	{checkCode: "SPEC_STATUSES_PRESENT", message: "spec statuses are present", findingCodes: []string{"SPEC_STATUS_EMPTY"}},
	{checkCode: "SPEC_BODIES_COMPLETE", message: "spec bodies include story, demonstrates, and acceptance criteria", findingCodes: []string{"SPEC_BODY_EMPTY", "SPEC_DEMONSTRATES_MISSING", "SPEC_ACCEPTANCE_MISSING"}},
	{checkCode: "SPEC_BLOCKERS_CHECKED", message: "spec blockers reference known dependencies or are flagged as warnings", findingCodes: []string{"SPEC_BLOCKER_UNKNOWN"}},
}

// planCheckRules is the rule table for ValidatePlan.
var planCheckRules = []validationRule{
	{checkCode: "PLAN_SPEC_CODE_VALID", message: "plan spec code matches US-NNN", findingCodes: []string{"PLAN_SPEC_CODE_INVALID"}},
	{checkCode: "PLAN_BODY_PRESENT", message: "plan body is present", findingCodes: []string{"PLAN_BODY_EMPTY"}},
	{checkCode: "PLAN_TASKS_PRESENT", message: "plan includes at least one task", findingCodes: []string{"PLAN_TASKS_EMPTY"}},
	{checkCode: "PLAN_TASK_COUNT_REASONABLE", message: "plan task count stays within the recommended limit", findingCodes: []string{"PLAN_TOO_MANY_TASKS"}},
	{checkCode: "PLAN_TASK_IDS_VALID", message: "task ids are present, unique, and match TASK-NN", findingCodes: []string{"PLAN_TASK_ID_INVALID", "PLAN_TASK_ID_DUPLICATE"}},
	{checkCode: "PLAN_TASK_TITLES_PRESENT", message: "task titles are present", findingCodes: []string{"PLAN_TASK_TITLE_EMPTY"}},
	{checkCode: "PLAN_TASK_TYPES_VALID", message: "task types are valid", findingCodes: []string{"PLAN_TASK_TYPE_INVALID"}},
	{checkCode: "PLAN_TASK_STATUSES_PRESENT", message: "task statuses are present", findingCodes: []string{"PLAN_TASK_STATUS_EMPTY"}},
	{checkCode: "PLAN_TASK_BODIES_COMPLETE", message: "task bodies include a complete execution contract", findingCodes: []string{"PLAN_TASK_BODY_EMPTY", "PLAN_TASK_CONTRACT_WEAK"}},
	{checkCode: "PLAN_TEST_TASK_PRESENT", message: "plan includes at least one test task", findingCodes: []string{"PLAN_TEST_TASK_MISSING"}},
	{checkCode: "PLAN_TASK_DEPENDENCIES_VALID", message: "task dependencies reference earlier valid tasks without cycles", findingCodes: []string{"PLAN_TASK_DEP_UNKNOWN", "PLAN_TASK_DEP_FUTURE", "PLAN_TASK_DEP_CYCLE"}},
}

// buildChecks constructs one ValidationCheck per rule, deriving the status
// from whether the given findings contain any of the rule's findingCodes.
func buildChecks(rules []validationRule, findings []domain.ValidationFinding) []domain.ValidationCheck {
	checks := make([]domain.ValidationCheck, 0, len(rules))
	for _, rule := range rules {
		checks = append(checks, domain.ValidationCheck{
			Code:    rule.checkCode,
			Status:  checkStatus(rule.findingCodes, findings),
			Message: rule.message,
		})
	}
	return checks
}

// checkStatus returns the aggregated status for a single rule given the
// collected findings. It returns CheckFailed on any error, CheckWarning
// on only warnings, and CheckPassed if no matching finding exists.
func checkStatus(codes []string, findings []domain.ValidationFinding) string {
	hasWarning := false
	for _, finding := range findings {
		if !containsCode(codes, finding.Code) {
			continue
		}
		if finding.Severity == SeverityError {
			return CheckFailed
		}
		if finding.Severity == SeverityWarning {
			hasWarning = true
		}
	}
	if hasWarning {
		return CheckWarning
	}
	return CheckPassed
}

// containsCode reports whether codes contains code (exact match).
func containsCode(codes []string, code string) bool {
	for _, candidate := range codes {
		if candidate == code {
			return true
		}
	}
	return false
}

// addFinding appends a single validation finding to the slice.
func addFinding(findings []domain.ValidationFinding, severity, code, path, message, hint string) []domain.ValidationFinding {
	return append(findings, domain.ValidationFinding{
		Code:     code,
		Severity: severity,
		Path:     path,
		Message:  message,
		Hint:     hint,
	})
}

// hasErrorFinding reports whether any finding has error severity. Warnings
// are surfaced but never block persistence.
func hasErrorFinding(findings []domain.ValidationFinding) bool {
	for _, f := range findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}
