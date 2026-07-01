// Package validation provides deterministic, marker-based validators for
// artifact phases (PRD, backlog, etc.). Validators never evaluate strategic
// quality; they only check structural completeness against machine-readable
// markers and patterns.
package validation

import (
	"regexp"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// Required PRD section IDs that must be present (via marker) and have
// meaningful content between their marker and the next marker or EOF.
var requiredPRDSections = []string{
	"elevator_pitch",
	"vision",
	"user_personas",
	"brainstorming_insights",
	"product_scope",
	"technical_architecture",
	"functional_requirements",
	"non_functional_requirements",
	"next_steps",
}

// markerRe matches archetipo PRD section markers of the form:
//
//	<!-- archetipo:prd section=<id> required=true -->
var markerRe = regexp.MustCompile(`<!--\s*archetipo:prd\s+section=(\S+)\s+required=\S+\s*-->`)

// placeholderRe matches unresolved {{PLACEHOLDER}} tokens.
var placeholderRe = regexp.MustCompile(`\{\{[^}]+\}\}`)

// commentLineRe matches HTML comment-only lines within a section body.
var commentLineRe = regexp.MustCompile(`^\s*<!--`)

// mdStripRe removes common markdown syntax before testing if a line has
// meaningful prose content.
var mdStripRe = regexp.MustCompile(`[#*_~>` + "`" + `]`)

// ValidatePRD runs every PRD structural rule against the given
// markdown content and returns a ValidationResult. target is the file path
// used for the result envelope and finding paths.
func ValidatePRD(target string, markdown string) domain.ValidationResult {
	var findings []domain.ValidationFinding

	// --- PRD_NOT_EMPTY ---
	if strings.TrimSpace(markdown) == "" {
		findings = addFinding(findings, SeverityError, "PRD_EMPTY", target, "PRD is empty", "Run archetipo-inception to generate a PRD.")
		return domain.ValidationResult{
			OK:       false,
			Artifact: "prd",
			Target:   target,
			Checks:   buildChecks(prdCheckRules, findings),
			Findings: findings,
		}
	}

	// --- PRD_NO_UNRESOLVED_PLACEHOLDERS ---
	unresolved := placeholderRe.FindAllString(markdown, -1)
	for _, ph := range unresolved {
		findings = addFinding(findings, SeverityError, "PRD_PLACEHOLDER_LEFT", target, "Unresolved placeholder "+ph, "Replace the placeholder with concrete content or an explicit TBD note.")
	}

	// --- PRD_REQUIRED_SECTIONS (marker-based) ---
	// Collect all markers and their byte offsets.
	type markerLoc struct {
		id    string
		start int // byte position after the marker line
	}
	var markers []markerLoc
	allMatches := markerRe.FindAllStringSubmatchIndex(markdown, -1)
	for _, m := range allMatches {
		// m[0:1] is full match; m[2:3] is the section ID capture.
		id := markdown[m[2]:m[3]]
		// Content starts after the full match line (up to next newline after the marker).
		endOfLine := strings.IndexByte(markdown[m[1]:], '\n')
		start := m[1]
		if endOfLine >= 0 {
			start = m[1] + endOfLine + 1
		}
		markers = append(markers, markerLoc{id: id, start: start})
	}

	// Check each required section.
	present := map[string]bool{}
	for _, m := range markers {
		present[m.id] = true
	}
	for _, secID := range requiredPRDSections {
		if !present[secID] {
			findings = addFinding(findings, SeverityError, "PRD_MISSING_SECTION", "markers."+secID, "Missing required marker for section "+secID, "Add <!-- archetipo:prd section="+secID+" required=true --> before the section content.")
		}
	}

	// --- Content check: each marker section must have meaningful content ---
	// For each marker, content is from its start to the next marker start (or EOF).
	for i, m := range markers {
		end := len(markdown)
		if i+1 < len(markers) {
			end = markers[i+1].start
		}
		body := strings.TrimSpace(markdown[m.start:end])
		// The section body must contain at least one non-whitespace, non-comment line
		// that has some substance (>= 3 chars after stripping markdown syntax).
		if !hasMeaningfulContent(body) {
			findings = addFinding(findings, SeverityError, "PRD_SECTION_EMPTY", "markers."+m.id, "Section "+m.id+" has no meaningful content", "Fill in the "+m.id+" section with concrete information.")
		}
	}

	checks := buildChecks(prdCheckRules, findings)
	return domain.ValidationResult{
		OK:       !hasErrorFinding(findings),
		Artifact: "prd",
		Target:   target,
		Checks:   checks,
		Findings: findings,
	}
}

// hasMeaningfulContent returns true if body has at least one line that looks
// like prose (not just whitespace, not a pure HTML comment, and not too short
// after stripping markdown syntax).
func hasMeaningfulContent(body string) bool {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if commentLineRe.MatchString(line) {
			continue
		}
		// Strip common markdown syntax to check if underlying text is long enough.
		cleaned := mdStripRe.ReplaceAllString(line, "")
		cleaned = strings.TrimSpace(cleaned)
		// A heading like "## Vision" would become "Vision" after stripping — still valid.
		if len(cleaned) >= 2 {
			return true
		}
	}
	return false
}
