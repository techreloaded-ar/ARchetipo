package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/validation"
)

// createSpecReq is the JSON body of POST /api/spec. The spec code is
// deliberately absent: it is a result of the operation, assigned by the
// server from the persisted backlog, never chosen by the browser.
type createSpecReq struct {
	Title     string   `json:"title"`
	EpicCode  string   `json:"epic_code"`
	Priority  string   `json:"priority"`
	Points    int      `json:"points"`
	Scope     string   `json:"scope"`
	BlockedBy []string `json:"blocked_by"`
	Body      string   `json:"body"`
}

// createSpecView is the JSON response of POST /api/spec. Created is false
// when an identical spec already existed and nothing was written.
type createSpecView struct {
	Spec    domain.Spec `json:"spec"`
	Created bool        `json:"created"`
}

// fieldError binds a validation failure to the form field that produced it,
// so the viewer can render the message under the offending input instead of
// as a single global message.
type fieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// nextSpecCode returns the next progressive US code for a backlog. It takes
// the highest numeric suffix among the existing US- codes and increments it,
// so a gap in the numbering never re-issues a code that was already used.
// Codes that are not US-<number> are ignored rather than treated as an error:
// a backlog may legitimately carry epic or task identifiers.
func nextSpecCode(specs []domain.Spec) string {
	highest := 0
	for _, spec := range specs {
		suffix, ok := strings.CutPrefix(strings.TrimSpace(spec.Code), "US-")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(suffix)
		if err != nil || n <= 0 {
			continue
		}
		if n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("US-%03d", highest+1)
}

// normalizeSpecTitle reduces a title to its comparable identity: trimmed,
// lowercased, and with every run of whitespace collapsed to a single space.
func normalizeSpecTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// findExistingSpec looks for a spec with the same content identity (same epic
// and same normalized title) among the specs already in the backlog.
//
// The duplicate is searched in the persisted backlog and not in a process
// cache on purpose: an in-memory idempotency map dies with a page reload and
// with a viewer restart, and would not see a spec first created through the
// CLI. Reading the backlog makes the guarantee survive both.
func findExistingSpec(specs []domain.Spec, epicCode, title string) (domain.Spec, bool) {
	epic := strings.ToLower(strings.TrimSpace(epicCode))
	wanted := normalizeSpecTitle(title)
	for _, spec := range specs {
		if strings.ToLower(strings.TrimSpace(spec.Epic.Code)) != epic {
			continue
		}
		if normalizeSpecTitle(spec.Title) == wanted {
			return spec, true
		}
	}
	return domain.Spec{}, false
}

// createSpecFieldFor maps a validation finding path onto the name of the form
// field that carries it. Paths the form does not own — the code and the status,
// both assigned by the server — map to the empty string: they are form-level
// errors, not field errors.
func createSpecFieldFor(path string) string {
	leaf := strings.TrimPrefix(strings.TrimSpace(path), "specs[0].")
	switch leaf {
	case "title", "priority", "points", "body", "scope", "blocked_by":
		return leaf
	case "epic.code":
		return "epic_code"
	default:
		return ""
	}
}

// createSpecFieldErrors converts the blocking findings of a validation result
// into the per-field errors returned to the viewer. Warnings are quality
// feedback and never reach the form.
func createSpecFieldErrors(findings []domain.ValidationFinding) []fieldError {
	out := make([]fieldError, 0, len(findings))
	for _, f := range findings {
		if f.Severity != validation.SeverityError {
			continue
		}
		out = append(out, fieldError{
			Field:   createSpecFieldFor(f.Path),
			Code:    f.Code,
			Message: f.Message,
		})
	}
	return out
}

// handleCreateSpec serves POST /api/spec: it validates the submitted form,
// assigns the progressive spec code from the persisted backlog, and writes
// the new spec through the configured connector. The backlog is touched only
// after validation succeeds, so an invalid payload never consumes a code.
func (s *Server) handleCreateSpec(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createSpecReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}

	existing, err := s.conn.FetchBacklogItems(ctx, "")
	if err != nil {
		var ce *iox.CodedError
		if !errors.As(err, &ce) || ce.Code != iox.CodePreconditionMissing {
			writeError(w, err)
			return
		}
		// A missing backlog is an empty backlog, not a failure: creation is
		// then refused below only because no epic is declared to assign the
		// spec to. When the backlog does declare epics but holds no spec yet,
		// the SaveInitialBacklog branch further down is the reachable one.
		existing = nil
	}

	// The epics come from the same call that feeds GET /api/board, so the
	// values the form offers and the ones the route accepts are the same list
	// by construction — including an epic declared without any spec yet.
	summary, err := s.conn.ReadExistingBacklog(ctx)
	if err != nil {
		var ce *iox.CodedError
		if !errors.As(err, &ce) || ce.Code != iox.CodePreconditionMissing {
			writeError(w, err)
			return
		}
		summary = domain.BacklogSummary{}
	}

	epic, ok := resolveEpic(summary.Epics, req.EpicCode)
	if !ok {
		writeCreateSpecFieldErrors(w, []fieldError{{
			Field:   "epic_code",
			Code:    "SPEC_EPIC_UNKNOWN",
			Message: fmt.Sprintf("epic %q does not exist in this workspace", strings.TrimSpace(req.EpicCode)),
		}})
		return
	}

	if found, dup := findExistingSpec(existing, req.EpicCode, req.Title); dup {
		writeJSON(w, http.StatusOK, createSpecView{Spec: found, Created: false})
		return
	}

	spec := domain.Spec{
		Code:      nextSpecCode(existing),
		Title:     strings.TrimSpace(req.Title),
		Epic:      epic,
		Priority:  domain.Priority(strings.ToUpper(strings.TrimSpace(req.Priority))),
		Points:    req.Points,
		Status:    domain.StatusTodo,
		Scope:     domain.Scope(strings.TrimSpace(req.Scope)),
		BlockedBy: trimCodes(req.BlockedBy),
		Body:      req.Body,
	}
	spec.Ref = spec.Code

	if result := validation.ValidateSpecs("POST /api/spec", []domain.Spec{spec}); !result.OK {
		writeCreateSpecFieldErrors(w, createSpecFieldErrors(result.Findings))
		return
	}

	if len(existing) == 0 {
		_, err = s.conn.SaveInitialBacklog(ctx, []domain.Spec{spec})
	} else {
		_, err = s.conn.AppendSpecs(ctx, []domain.Spec{spec})
	}
	if err != nil {
		writeError(w, err)
		return
	}

	// Read the spec back through the connector: this is what makes the new
	// content "readable from the connector" rather than merely accepted.
	saved, err := s.conn.ReadSpecDetail(ctx, spec.Code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createSpecView{Spec: saved, Created: true})
}

// resolveEpic looks the submitted epic code up among the epics the workspace
// already knows. The title always comes from the workspace, never from the
// client, so the viewer cannot rename an epic by creating a spec.
func resolveEpic(epics []domain.Epic, code string) (domain.Epic, bool) {
	wanted := strings.ToLower(strings.TrimSpace(code))
	if wanted == "" {
		return domain.Epic{}, false
	}
	for _, epic := range epics {
		if strings.ToLower(strings.TrimSpace(epic.Code)) == wanted {
			return epic, true
		}
	}
	return domain.Epic{}, false
}

// trimCodes normalizes a list of spec codes, dropping the empty entries.
func trimCodes(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if t := strings.TrimSpace(c); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// writeCreateSpecFieldErrors answers 400 keeping the error/code/hint keys the
// rest of the viewer already understands, and adds the per-field detail under
// "fields" so an older client still shows the message.
func writeCreateSpecFieldErrors(w http.ResponseWriter, fields []fieldError) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":  "the spec could not be created: some fields are invalid",
		"code":   iox.CodeInvalidInput,
		"hint":   "fix the highlighted fields and confirm again",
		"fields": fields,
	})
}
