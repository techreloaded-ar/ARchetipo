package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/metrics"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// boardColumnView is the JSON shape of one Kanban column in GET /api/board.
type boardColumnView struct {
	ID     string        `json:"id"`
	Title  string        `json:"title"`
	Status domain.Status `json:"status"`
	Specs  []domain.Spec `json:"specs"`
}

type boardView struct {
	Columns []boardColumnView `json:"columns"`
	Epics   []domain.Epic     `json:"epics"`
}

// canonical board layout: keeps the order TODO → PLANNED → IN PROGRESS → REVIEW → DONE.
var boardLayout = []struct {
	ID     string
	Status domain.Status
}{
	{"todo", domain.StatusTodo},
	{"planned", domain.StatusPlanned},
	{"in_progress", domain.StatusInProgress},
	{"review", domain.StatusReview},
	{"done", domain.StatusDone},
}

// handleGetMetrics returns the same aggregation as `archetipo metrics`.
func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	specs, err := ws.conn.FetchBacklogItems(r.Context(), "")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, metrics.Compute(specs))
}

func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	info, err := ws.conn.InitializeConnector(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	labels := info.Workflow.Statuses
	specs, err := ws.conn.FetchBacklogItems(ctx, "")
	if err != nil {
		writeError(w, err)
		return
	}
	summary, err := ws.conn.ReadExistingBacklog(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	view := boardView{Epics: summary.Epics}
	titleFor := func(id string) string {
		switch id {
		case "todo":
			return labels.Todo
		case "planned":
			return labels.Planned
		case "in_progress":
			return labels.InProgress
		case "review":
			return labels.Review
		case "done":
			return labels.Done
		}
		return id
	}
	columnSpecs := make(map[string][]domain.Spec, len(boardLayout))
	for _, sp := range s.specsInBoardOrder(ctx, ws, specs) {
		for _, col := range boardLayout {
			if col.Status == sp.Status {
				columnSpecs[col.ID] = append(columnSpecs[col.ID], sp)
				break
			}
		}
	}
	for _, col := range boardLayout {
		c := boardColumnView{ID: col.ID, Title: titleFor(col.ID), Status: col.Status, Specs: columnSpecs[col.ID]}
		view.Columns = append(view.Columns, c)
	}
	writeJSON(w, http.StatusOK, view)
}

// specsInBoardOrder returns specs in the order the workspace reads them: first
// the ones the persisted board order names — the order a person produced by
// dragging cards — and then everything the order does not mention, in the order
// the connector listed it.
//
// It is shared rather than private to the board because "which spec comes
// first" is a single question with a single answer: the recommended next step
// must point at the same spec the board shows at the top of its column, or the
// two would disagree about what to do next. An unreadable board order is not a
// failure here for the same reason it is not one in handleGetBoard: the
// connector's own order is a complete answer, just not a personalized one.
func (s *Server) specsInBoardOrder(ctx context.Context, ws *workspaceSession, specs []domain.Spec) []domain.Spec {
	var boardOrder []string
	if reader, ok := ws.conn.(connector.BoardOrderReader); ok {
		if order, err := reader.ReadBoardOrder(ctx); err == nil {
			boardOrder = order
		}
	}
	specByCode := make(map[string]domain.Spec, len(specs))
	for _, sp := range specs {
		specByCode[sp.Code] = sp
	}
	out := make([]domain.Spec, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, code := range boardOrder {
		sp, ok := specByCode[code]
		if !ok || seen[code] {
			continue
		}
		out = append(out, sp)
		seen[code] = true
	}
	for _, sp := range specs {
		if seen[sp.Code] {
			continue
		}
		out = append(out, sp)
		seen[sp.Code] = true
	}
	return out
}

// handleStreamBoard streams Server-Sent Events to the browser. The handler
// keeps the connection open for as long as the client is connected; every
// time the filesystem watcher publishes a change, a `board_changed` event is
// flushed. A periodic comment line acts as a heartbeat so intermediary proxies
// (and the browser itself) do not close the connection as idle.
func (s *Server) handleStreamBoard(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := s.broker.Subscribe()
	defer unsub()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, "event: board_changed\ndata: {}\n\n")
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// templateView identifies the process Template the actions were derived from.
// It is the resolved definition, not the pair persisted in config.yaml — the
// same distinction `archetipo spec actions` makes.
type templateView struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	// Label is the name the process gives itself, so a client can say which
	// Archetipo a workspace is running in that process's own words instead of
	// rendering its id.
	Label string `json:"label"`
}

// newTemplateView is the single place a templateView is built, so no route can
// publish a process identified by only half of what identifies it.
func newTemplateView(tpl template.Template) templateView {
	return templateView{ID: tpl.ID, Version: tpl.Version, Label: tpl.Label}
}

type specDetailView struct {
	Spec     domain.Spec   `json:"spec"`
	PlanBody string        `json:"plan_body"`
	Tasks    []domain.Task `json:"tasks"`
	// Template and Actions answer "what can I do with this spec now?" with the
	// process rules, so the browser never has to know them. Actions is always a
	// list: a status with no admissible action is an empty one, never a null.
	Template templateView     `json:"template"`
	Actions  []specActionView `json:"actions"`
	// Execution is the most recent execution of this spec, or null when it has
	// none. It travels with the detail rather than behind a separate call so a
	// browser that was just reloaded finds the run it started without having
	// remembered any identifier.
	Execution *execution.Execution `json:"execution"`
}

func (s *Server) handleGetSpec(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "use /api/spec/US-XXX", nil))
		return
	}
	ctx := r.Context()
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	tasks, planBody, err := s.readPlanForSpec(ctx, ws, code)
	if err != nil {
		writeError(w, err)
		return
	}
	if tasks == nil {
		tasks = []domain.Task{}
	}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	// A record that cannot be read is a failed request, not an absent execution:
	// answering "no execution" would tell the browser it may start a second one.
	latest, err := s.latestExecution(ctx, ws, code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, specDetailView{
		Spec:      spec,
		PlanBody:  planBody,
		Tasks:     tasks,
		Template:  newTemplateView(tpl),
		Actions:   s.decorateActions(ctx, ws, code, tpl.ActionsFor(spec.Status), len(tasks)),
		Execution: latest,
	})
}

// readPlanForSpec returns the tasks and (when readable) the plan body for a
// spec. The connector interface only exposes ReadSpecTasks, so for connectors
// that also store a plan body (filefs) we look it up via the optional
// planBodyReader. A missing plan is not an error: the viewer should still be
// able to display the spec with an empty plan.
func (s *Server) readPlanForSpec(ctx context.Context, ws *workspaceSession, code string) ([]domain.Task, string, error) {
	tasks, err := ws.conn.ReadSpecTasks(ctx, code)
	if err != nil {
		var ce *iox.CodedError
		if errors.As(err, &ce) && ce.Code == iox.CodePreconditionMissing {
			return nil, "", nil
		}
		return nil, "", err
	}
	domain.NormalizeTaskBodies(tasks)
	body := ""
	if pr, ok := ws.conn.(connector.PlanBodyReader); ok {
		if b, err := pr.ReadPlanBody(ctx, code); err == nil {
			body = b
		}
	}
	return tasks, body, nil
}

type prdView struct {
	Body string `json:"body"`
}

func (s *Server) handleGetPRD(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	pr, ok := ws.conn.(connector.PRDReader)
	if !ok {
		writeError(w, iox.NewConnector(iox.CodePreconditionMissing, "this connector does not expose a PRD", "use the file connector to read the PRD", nil))
		return
	}
	body, err := pr.ReadPRD(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prdView{Body: body})
}

func (s *Server) handleSavePRD(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	var req prdView
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if _, err := ws.conn.SavePRD(r.Context(), req.Body); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prdView{Body: req.Body})
}

type mockupsView struct {
	Mockups []domain.MockupEntry `json:"mockups"`
}

func (s *Server) handleListMockups(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ml, ok := ws.conn.(connector.MockupLister)
	if !ok {
		writeJSON(w, http.StatusOK, mockupsView{Mockups: []domain.MockupEntry{}})
		return
	}
	list, err := ml.ListMockups(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []domain.MockupEntry{}
	}
	writeJSON(w, http.StatusOK, mockupsView{Mockups: list})
}

func (s *Server) handleUpdateSpec(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	var patch domain.SpecUpdate
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	if _, err := ws.conn.UpdateSpec(r.Context(), code, patch); err != nil {
		writeError(w, err)
		return
	}
	spec, err := ws.conn.ReadSpecDetail(r.Context(), code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spec": spec})
}

func (s *Server) handleDeleteSpec(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	deleter, ok := ws.conn.(connector.SpecDeleter)
	if !ok {
		writeError(w, iox.NewConnector(
			iox.CodePreconditionMissing,
			"this connector does not support deleting specs from the viewer",
			"use the local file connector",
			nil,
		))
		return
	}
	res, err := deleter.DeleteSpec(r.Context(), code)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type savePlanReq struct {
	PlanBody string        `json:"plan_body"`
	Tasks    []domain.Task `json:"tasks"`
}

func (s *Server) handleSavePlan(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	code := r.PathValue("code")
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "", nil))
		return
	}
	var req savePlanReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	input := domain.PlanInput{PlanBody: req.PlanBody, Tasks: req.Tasks}
	domain.NormalizePlanInput(&input)
	res, err := ws.conn.SavePlan(r.Context(), code, input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type moveReq struct {
	Code   string  `json:"code"`
	To     string  `json:"to"`
	Before *string `json:"before,omitempty"`
	After  *string `json:"after,omitempty"`
}

func (s *Server) handleMoveCard(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	var req moveReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Code == "" || req.To == "" {
		writeError(w, iox.NewInvalidInput("code and to are required", "", nil))
		return
	}
	anchor := domain.ReorderAnchor{}
	if req.Before != nil {
		anchor.Before = *req.Before
	}
	if req.After != nil {
		anchor.After = *req.After
	}
	res, err := ws.conn.MoveBoardCard(r.Context(), req.Code, req.To, anchor)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// helpers

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return iox.NewInvalidInput("invalid JSON body", "", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	var ce *iox.CodedError
	if !errors.As(err, &ce) {
		ce = iox.NewInternal(err.Error(), err)
	}
	status := http.StatusInternalServerError
	switch ce.Code {
	case iox.CodeInvalidInput:
		status = http.StatusBadRequest
	case iox.CodeNotFound, iox.CodePreconditionMissing:
		status = http.StatusNotFound
	case iox.CodeConflict:
		status = http.StatusConflict
	case iox.CodeConnectorAuth, iox.CodeConnectorNetwork, iox.CodeConnectorBackend:
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]any{
		"error": ce.Message,
		"code":  ce.Code,
		"hint":  ce.Hint,
	})
}
