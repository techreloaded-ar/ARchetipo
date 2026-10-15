package web

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

type documentationActionView struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Placement         string `json:"placement"`
	Runnable          bool   `json:"runnable"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type documentationActivityView struct {
	ExecutionID    string   `json:"execution_id"`
	ConversationID string   `json:"conversation_id,omitempty"`
	Label          string   `json:"label"`
	StatusLabel    string   `json:"status_label"`
	Running        bool     `json:"running"`
	ChangedPages   []string `json:"changed_pages"`
}

type documentationActionsView struct {
	Actions    []documentationActionView   `json:"actions"`
	Activities []documentationActivityView `json:"activities"`
}

func documentationActions(tpl template.Template, hasPages bool, reason string) []documentationActionView {
	out := []documentationActionView{}
	for _, action := range tpl.DocumentationActions {
		if action.RequiresWiki != hasPages {
			continue
		}
		out = append(out, documentationActionView{ID: action.ID, Label: action.Label, Placement: action.Placement, Runnable: reason == "", UnavailableReason: reason})
	}
	return out
}

func (s *Server) documentationAvailability(ctx context.Context, ws *workspaceSession) string {
	target := s.conversationAvailabilityFor(ctx, ws)
	if target.reason != "" {
		return target.reason
	}
	if target.sessions == nil {
		return "Configura un agente con sessioni native per lavorare sulla Wiki."
	}
	return target.availability.reasonFor(execution.CapabilityWorkspaceWiki)
}

func (s *Server) handleGetDocumentationActions(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	collection, readErr := readWikiDocumentation(ws)
	reason := s.documentationAvailability(r.Context(), ws)
	if readErr != nil {
		reason = "La Wiki non è leggibile. Controlla i file della documentazione."
	}
	activities, err := s.documentationActivities(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	// A Wiki is shared workspace data: two maintenance runs must not edit it concurrently.
	for _, activity := range activities {
		if activity.Running {
			reason = "Un’attività Wiki è già aperta. Continua nella sua conversazione o annullala."
			break
		}
	}
	writeJSON(w, http.StatusOK, documentationActionsView{Actions: documentationActions(tpl, len(collection.Items) > 0, reason), Activities: activities})
}

func (s *Server) documentationActivities(ctx context.Context, ws *workspaceSession) ([]documentationActivityView, error) {
	conversations, err := ws.conversationStore().List(ctx)
	if err != nil {
		return nil, err
	}
	threads := map[string]string{}
	for _, record := range conversations {
		for _, id := range record.ExecutionIDs {
			threads[id] = record.ID
		}
	}
	records, err := ws.store.ListBySpec(ctx, workspaceExecutionKey)
	if err != nil {
		return nil, err
	}
	out := []documentationActivityView{}
	for _, record := range records {
		if execution.WikiOperation(record.Action) == "" {
			continue
		}
		id := threads[record.ID]
		label := "Attività Wiki completata"
		switch record.Status {
		case execution.StatusRunning:
			label = "Attività Wiki in corso"
			if thread, readErr := ws.conversationStore().GetMetadata(ctx, id); readErr == nil {
				label, _, _ = nativeActionProgress(record, thread)
			}
		case execution.StatusFailed:
			label = "Attività Wiki interrotta o non riuscita"
			if record.Error != nil {
				label += ": " + record.Error.Message
			}
		}
		changed := []string{}
		if record.Result != nil {
			var fields struct {
				Pages []string `json:"wiki_changed_pages"`
			}
			if json.Unmarshal(record.Result.Payload, &fields) == nil && fields.Pages != nil {
				changed = fields.Pages
			}
		}
		out = append(out, documentationActivityView{ExecutionID: record.ID, ConversationID: id, Label: s.actionLabelOf(ws, record.Action, ""), StatusLabel: label, Running: record.Status == execution.StatusRunning, ChangedPages: changed})
		if len(out) == 5 {
			break
		}
	}
	return out, nil
}

func (s *Server) handleRunDocumentationAction(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	if err := ws.requireReachable(); err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Action string `json:"action"`
		PageID string `json:"page_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	index := slices.IndexFunc(tpl.DocumentationActions, func(a template.DocumentationAction) bool { return a.ID == req.Action })
	if index < 0 {
		writeError(w, iox.NewInvalidInput("Azione Wiki sconosciuta", "Scegli un’azione dalla documentazione.", nil))
		return
	}
	ctx := r.Context()
	// Serialize the read/check/start sequence across viewers, including double presses.
	lock, err := ws.conversationStore().Lock(ctx, "wiki-actions")
	if err != nil {
		writeError(w, err)
		return
	}
	defer func() { _ = lock.Unlock() }()
	collection, err := readWikiDocumentation(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	definition := tpl.DocumentationActions[index]
	if definition.RequiresWiki != (len(collection.Items) > 0) {
		writeError(w, iox.NewConflict("La disponibilità della Wiki è cambiata", "Ricarica la documentazione.", nil))
		return
	}
	if definition.Placement == "page" && !slices.ContainsFunc(collection.Items, func(p documentationPage) bool { return p.ID == req.PageID }) {
		writeError(w, iox.NewNotFound("Pagina non trovata", "Scegli una pagina dall’indice della Wiki.", nil))
		return
	}
	if reason := s.documentationAvailability(ctx, ws); reason != "" {
		writeError(w, iox.NewConflict(reason, conversationRemedy, nil))
		return
	}

	records, err := ws.store.ListBySpec(ctx, workspaceExecutionKey)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, record := range records {
		if execution.WikiOperation(record.Action) != "" && record.Status == execution.StatusRunning {
			writeError(w, iox.NewConflict("Un’attività Wiki è già aperta", "Continua nella sua conversazione o annullala.", nil))
			return
		}
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	baseline := documentationDigests(collection)
	started, err := s.startActionInSession(ctx, ws, actionInSession{
		action: execution.ActionID(req.Action), providerID: target.availability.providerID,
		providerConfig: target.availability.providerConfig, userInput: req.PageID,
		wikiBaseline: baseline, skill: definition.Skill,
		confirm: func(_ context.Context, outcome *execution.Execution) { ws.confirmWikiAction(outcome) },
	})
	if err != nil {
		writeStartError(w, err)
		return
	}
	s.broker.Publish()
	writeJSON(w, http.StatusCreated, started)
}

func documentationDigests(collection documentationCollection) map[string]string {
	result := map[string]string{}
	for _, page := range collection.Items {
		raw, _ := json.Marshal(page.Page)
		result[page.ID] = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	return result
}

// Verification uses the same Wiki service as the CLI. The receipt alone never
// proves a successful bootstrap, a readable Wiki or unchanged query evidence.
func (ws *workspaceSession) confirmWikiAction(outcome *execution.Execution) {
	if outcome.Status != execution.StatusSucceeded {
		return
	}
	collection, err := readWikiDocumentation(ws)
	if err == nil && len(collection.Items) == 0 {
		err = fmt.Errorf("la Wiki non contiene pagine")
	}
	if err == nil && outcome.Action == execution.ActionWikiCreate {
		root := ws.cfg.Paths.Wiki
		if !filepath.IsAbs(root) {
			root = filepath.Join(ws.cfg.ProjectRoot, root)
		}
		var report wiki.Report
		report, err = wiki.ValidateBootstrap(ws.cfg.ProjectRoot, root, ws.cfg.Paths.PRD)
		if err == nil && !report.OK {
			err = fmt.Errorf("la validazione iniziale della Wiki non è superata")
		}
	}
	// Lint may complete with findings; these remain visible on the pages. Refresh
	// must leave a structurally valid Wiki. Review/approval is always separate.
	if err == nil && outcome.Action == execution.ActionWikiRefresh {
		for _, finding := range collection.Findings {
			if finding.Severity == "error" {
				err = fmt.Errorf("la Wiki contiene problemi strutturali: %s", finding.Code)
				break
			}
		}
	}
	changed := []string{}
	after := documentationDigests(collection)
	for id, digest := range after {
		if outcome.WikiBaseline[id] != digest {
			changed = append(changed, id)
		}
	}
	for id := range outcome.WikiBaseline {
		if _, found := after[id]; !found {
			changed = append(changed, id)
		}
	}
	if err == nil && outcome.Action != execution.ActionWikiQuery {
		for _, page := range collection.Items {
			if outcome.WikiBaseline[page.ID] != after[page.ID] && page.Meta.Status == "reviewed" {
				err = fmt.Errorf("la pagina %s è stata approvata durante l’attività Wiki: la revisione è un’operazione separata", page.ID)
				break
			}
		}
	}
	slices.Sort(changed)
	if err == nil && outcome.Action == execution.ActionWikiQuery && len(changed) > 0 {
		err = fmt.Errorf("la consultazione ha modificato la Wiki")
	}
	if err != nil {
		outcome.Status = execution.StatusFailed
		outcome.Result = nil
		outcome.Error = &execution.ExecutionError{Code: "UNCONFIRMED_EFFECT", Message: err.Error()}
		return
	}
	if outcome.Result != nil {
		fields := map[string]any{}
		if json.Unmarshal(outcome.Result.Payload, &fields) == nil {
			fields["wiki_changed_pages"] = changed
			outcome.Result.Payload, _ = json.Marshal(fields)
		}
	}
}
