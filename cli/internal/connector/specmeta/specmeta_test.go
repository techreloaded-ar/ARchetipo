package specmeta

import (
	"testing"
)

func TestParseNoMarker(t *testing.T) {
	body, meta := Parse("## Hello\n\nWorld.")
	if body != "## Hello\n\nWorld." {
		t.Errorf("body changed: %q", body)
	}
	if meta.Scope != "" || len(meta.BlockedBy) > 0 {
		t.Errorf("unexpected meta: %+v", meta)
	}
}

func TestParseWithMarker(t *testing.T) {
	raw := "## Spec\n\nSome body.\n\n<!-- archetipo:spec-meta {\"schema\":\"archetipo/spec-meta/v1\",\"scope\":\"MVP\",\"blocked_by\":[\"US-003\"],\"rework\":true} -->"
	body, meta := Parse(raw)
	if body != "## Spec\n\nSome body." {
		t.Errorf("unexpected body: %q", body)
	}
	if meta.Scope != "MVP" {
		t.Errorf("scope: %q", meta.Scope)
	}
	if len(meta.BlockedBy) != 1 || meta.BlockedBy[0] != "US-003" {
		t.Errorf("blocked_by: %v", meta.BlockedBy)
	}
	if !meta.Rework {
		t.Errorf("rework should be true")
	}
}

func TestParseCorruptMarker(t *testing.T) {
	raw := "## Spec\n\n<!-- archetipo:spec-meta this-is-not-json -->"
	body, meta := Parse(raw)
	if body != "## Spec" {
		t.Errorf("unexpected body: %q", body)
	}
	if meta.Scope != "" {
		t.Errorf("corrupt marker should produce empty meta")
	}
}

func TestRenderEmpty(t *testing.T) {
	got := Render("Hello world.", Meta{})
	if got != "Hello world." {
		t.Errorf("empty meta should return body unchanged: %q", got)
	}
}

func TestRenderWithFields(t *testing.T) {
	got := Render("## Spec\n\nBody.", Meta{
		Scope:     "MVP",
		BlockedBy: []string{"US-001", "US-002"},
		Rework:    true,
	})
	// Round-trip.
	body, meta := Parse(got)
	if body != "## Spec\n\nBody." {
		t.Errorf("round-trip body: %q", body)
	}
	if meta.Scope != "MVP" {
		t.Errorf("round-trip scope: %q", meta.Scope)
	}
	if len(meta.BlockedBy) != 2 {
		t.Errorf("round-trip blocked_by: %v", meta.BlockedBy)
	}
	if !meta.Rework {
		t.Errorf("round-trip rework should be true")
	}
}

func TestRenderReplacesExisting(t *testing.T) {
	// Simulate an existing marker.
	raw := "Body.\n\n<!-- archetipo:spec-meta {\"schema\":\"archetipo/spec-meta/v1\",\"scope\":\"Old\"} -->"
	body, meta := Parse(raw)
	if meta.Scope != "Old" {
		t.Errorf("parse failed: %+v", meta)
	}
	// Update scope, add branch.
	meta.Scope = "New"
	meta.Branch = "branch/US-001"
	rendered := Render(body, meta)
	// Should contain only the new marker.
	if count := markerCount(rendered); count != 1 {
		t.Errorf("expected 1 marker, got %d in: %q", count, rendered)
	}
	_, round := Parse(rendered)
	if round.Scope != "New" {
		t.Errorf("scope: %q", round.Scope)
	}
	if round.Branch != "branch/US-001" {
		t.Errorf("branch: %q", round.Branch)
	}
}

func markerCount(s string) int {
	return len(markerRegexp.FindAllString(s, -1))
}
