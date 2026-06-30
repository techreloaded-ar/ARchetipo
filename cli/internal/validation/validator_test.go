package validation

import (
	"testing"
)

func TestValidatePRD_Valid(t *testing.T) {
	prd := `<!-- archetipo:prd section=elevator_pitch required=true -->
The Elevator Pitch is a concise statement.

<!-- archetipo:prd section=vision required=true -->
Our vision is to create a platform.

<!-- archetipo:prd section=user_personas required=true -->
The target users are developers.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Key insights from brainstorming.

<!-- archetipo:prd section=product_scope required=true -->
MVP scope includes the core features.

<!-- archetipo:prd section=technical_architecture required=true -->
The stack uses Go and React.

<!-- archetipo:prd section=functional_requirements required=true -->
FR-01: Users can log in.

<!-- archetipo:prd section=non_functional_requirements required=true -->
NFR-01: 99.9% uptime.

<!-- archetipo:prd section=next_steps required=true -->
Finalize the backlog.`

	result := ValidatePRD("docs/PRD.md", prd)
	if !result.OK {
		t.Fatalf("expected OK=true, got false. findings=%v", result.Findings)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected 0 findings, got %d: %v", len(result.Findings), result.Findings)
	}
}

func TestValidatePRD_Empty(t *testing.T) {
	result := ValidatePRD("docs/PRD.md", "")
	if result.OK {
		t.Fatal("expected OK=false for empty PRD")
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected at least one finding for empty PRD")
	}
	f := result.Findings[0]
	if f.Code != "PRD_EMPTY" {
		t.Fatalf("expected PRD_EMPTY finding, got %s", f.Code)
	}
}

func TestValidatePRD_WhitespaceOnly(t *testing.T) {
	result := ValidatePRD("docs/PRD.md", "   \n  \n  ")
	if result.OK {
		t.Fatal("expected OK=false for whitespace-only PRD")
	}
}

func TestValidatePRD_UnresolvedPlaceholder(t *testing.T) {
	prd := `<!-- archetipo:prd section=elevator_pitch required=true -->
A pitch.

<!-- archetipo:prd section=vision required=true -->
Vision: {{BACKEND_FRAMEWORK}} is used.

<!-- archetipo:prd section=user_personas required=true -->
Users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights.

<!-- archetipo:prd section=product_scope required=true -->
Scope.

<!-- archetipo:prd section=technical_architecture required=true -->
Architecture: {{SOME_PLACEHOLDER}}.

<!-- archetipo:prd section=functional_requirements required=true -->
FR-01.

<!-- archetipo:prd section=non_functional_requirements required=true -->
NFR-01.

<!-- archetipo:prd section=next_steps required=true -->
Next.`

	result := ValidatePRD("docs/PRD.md", prd)
	if result.OK {
		t.Fatal("expected OK=false with placeholder left")
	}
	hasPlaceholderFinding := false
	for _, f := range result.Findings {
		if f.Code == "PRD_PLACEHOLDER_LEFT" {
			hasPlaceholderFinding = true
			break
		}
	}
	if !hasPlaceholderFinding {
		t.Fatalf("expected PRD_PLACEHOLDER_LEFT finding, got: %v", result.Findings)
	}
}

func TestValidatePRD_MissingSection(t *testing.T) {
	// vision marker is missing
	prd := `<!-- archetipo:prd section=elevator_pitch required=true -->
Pitch.

<!-- archetipo:prd section=user_personas required=true -->
Users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights.

<!-- archetipo:prd section=product_scope required=true -->
Scope.

<!-- archetipo:prd section=technical_architecture required=true -->
Architecture.

<!-- archetipo:prd section=functional_requirements required=true -->
FR.

<!-- archetipo:prd section=non_functional_requirements required=true -->
NFR.

<!-- archetipo:prd section=next_steps required=true -->
Next.`

	result := ValidatePRD("docs/PRD.md", prd)
	if result.OK {
		t.Fatal("expected OK=false with missing section")
	}
	hasMissing := false
	for _, f := range result.Findings {
		if f.Code == "PRD_MISSING_SECTION" {
			hasMissing = true
			break
		}
	}
	if !hasMissing {
		t.Fatalf("expected PRD_MISSING_SECTION finding, got: %v", result.Findings)
	}
}

func TestValidatePRD_EmptySection(t *testing.T) {
	prd := `<!-- archetipo:prd section=elevator_pitch required=true -->
Some pitch.

<!-- archetipo:prd section=vision required=true -->
<!-- archetipo:prd section=user_personas required=true -->
Users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights.

<!-- archetipo:prd section=product_scope required=true -->
Scope.

<!-- archetipo:prd section=technical_architecture required=true -->
Architecture.

<!-- archetipo:prd section=functional_requirements required=true -->
FR.

<!-- archetipo:prd section=non_functional_requirements required=true -->
NFR.

<!-- archetipo:prd section=next_steps required=true -->
Next.`

	result := ValidatePRD("docs/PRD.md", prd)
	if result.OK {
		t.Fatal("expected OK=false with empty section")
	}
	hasEmpty := false
	for _, f := range result.Findings {
		if f.Code == "PRD_SECTION_EMPTY" && f.Path == "markers.vision" {
			hasEmpty = true
			break
		}
	}
	if !hasEmpty {
		t.Fatalf("expected PRD_SECTION_EMPTY for vision, got: %v", result.Findings)
	}
}

func TestValidatePRD_NoPlaceholder_ButFoundWordWithBraces(t *testing.T) {
	// Single braces with non-placeholder content should not trigger placeholder detection.
	prd := `<!-- archetipo:prd section=elevator_pitch required=true -->
Pitch.

<!-- archetipo:prd section=vision required=true -->
Vision {some} text.

<!-- archetipo:prd section=user_personas required=true -->
Users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights.

<!-- archetipo:prd section=product_scope required=true -->
Scope.

<!-- archetipo:prd section=technical_architecture required=true -->
Architecture.

<!-- archetipo:prd section=functional_requirements required=true -->
FR.

<!-- archetipo:prd section=non_functional_requirements required=true -->
NFR.

<!-- archetipo:prd section=next_steps required=true -->
Next.`

	result := ValidatePRD("docs/PRD.md", prd)
	if !result.OK {
		t.Fatalf("expected OK=true for PRD without double braces, got findings=%v", result.Findings)
	}
}

func TestValidatePRD_AllChecksPassed(t *testing.T) {
	prd := validPRD()
	result := ValidatePRD("docs/PRD.md", prd)
	if !result.OK {
		t.Fatalf("expected OK=true, got false")
	}
	// Verify the three checks are present and passed.
	checks := map[string]string{}
	for _, c := range result.Checks {
		checks[c.Code] = c.Status
	}
	for _, code := range []string{"PRD_NOT_EMPTY", "PRD_NO_UNRESOLVED_PLACEHOLDERS", "PRD_REQUIRED_SECTIONS"} {
		if checks[code] != "passed" {
			t.Errorf("expected %s to be passed, got %s", code, checks[code])
		}
	}
}

func validPRD() string {
	return `<!-- archetipo:prd section=elevator_pitch required=true -->
A concise elevator pitch summarizing the product.

<!-- archetipo:prd section=vision required=true -->
The long-term vision for the product.

<!-- archetipo:prd section=user_personas required=true -->
Detailed personas describing target users.

<!-- archetipo:prd section=brainstorming_insights required=true -->
Insights gathered during brainstorming sessions.

<!-- archetipo:prd section=product_scope required=true -->
MVP scope and out-of-scope items.

<!-- archetipo:prd section=technical_architecture required=true -->
The chosen tech stack and architecture decisions.

<!-- archetipo:prd section=functional_requirements required=true -->
List of functional requirements with IDs.

<!-- archetipo:prd section=non_functional_requirements required=true -->
Performance, security, and reliability requirements.

<!-- archetipo:prd section=next_steps required=true -->
Concrete next steps and owners.
`
}
