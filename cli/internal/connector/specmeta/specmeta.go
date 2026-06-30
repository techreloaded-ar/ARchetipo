// Package specmeta manages the hidden HTML comment marker embedded in issue
// descriptions (GitHub and Jira) to round-trip spec fields that have no native
// storage in the backend.
//
// Marker format:
//
//	<!-- archetipo:spec-meta {"schema":"archetipo/spec-meta/v1","scope":"MVP","blocked_by":["US-003"],"branch":"archetipo/US-001","worktree":".archetipo/worktrees/US-001","fork_base":"abc123","rework":true} -->
//
// Rules:
//   - One marker per body, replaced idempotently on every write.
//   - Stripped from Body before returning it to callers.
//   - Tolerant: absent marker → empty metadata.
//   - The marker is versioned (schema field) for forward compatibility.
package specmeta

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Meta holds the fields stored in the hidden marker.
type Meta struct {
	Schema    string   `json:"schema"`
	Scope     string   `json:"scope,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Worktree  string   `json:"worktree,omitempty"`
	ForkBase  string   `json:"fork_base,omitempty"`
	Rework    bool     `json:"rework,omitempty"`
}

// currentSchema is the active marker schema version.
const currentSchema = "archetipo/spec-meta/v1"

// markerRegexp matches a JSON-based spec-meta marker on a single line.
var markerRegexp = regexp.MustCompile(`(?m)^<!-- archetipo:spec-meta (.*?) -->\s*$`)

// Parse extracts the metadata marker (if present) and returns the clean body
// without the marker.
func Parse(rawBody string) (cleanBody string, meta Meta) {
	m := markerRegexp.FindStringSubmatch(rawBody)
	if m != nil {
		// Best-effort JSON decode; if the marker is corrupt we drop it silently.
		_ = json.Unmarshal([]byte(m[1]), &meta)
	}
	cleanBody = strings.TrimSpace(markerRegexp.ReplaceAllString(rawBody, ""))
	return cleanBody, meta
}

// Render appends (or replaces) the spec-meta marker at the end of the body.
// Returns the body unchanged when the marker would be empty (no fields set).
func Render(cleanBody string, meta Meta) string {
	meta.Schema = currentSchema
	if meta.isEmpty() {
		return cleanBody
	}
	j, err := json.Marshal(meta)
	if err != nil {
		return cleanBody
	}
	out := strings.TrimRight(cleanBody, "\n")
	if out != "" {
		out += "\n\n"
	}
	out += "<!-- archetipo:spec-meta " + string(j) + " -->"
	return out
}

// isEmpty returns true when no non-schema field carries a value.
func (m Meta) isEmpty() bool {
	return m.Scope == "" &&
		len(m.BlockedBy) == 0 &&
		m.Branch == "" &&
		m.Worktree == "" &&
		m.ForkBase == "" &&
		!m.Rework
}
