package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// inceptionProvider is a provider that declares the workspace capability an
// inception requires. Only the provider is a double: the connector, the record
// store, the effect confirmation, the rollback and the process Template are the
// production ones, so what these tests assert is the HTTP contract of the real
// server and not the behaviour of a stub.
type inceptionProvider struct {
	*runTestProvider
}

func (p *inceptionProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{execution.CapabilitySpecPlan, execution.CapabilityWorkspaceInception}, nil
}

func releasedInceptionProvider(id string, execute func(context.Context, execution.Request) (execution.Result, error)) *inceptionProvider {
	return &inceptionProvider{runTestProvider: releasedProvider(id, execute)}
}

func blockedInceptionProvider(id string) *inceptionProvider {
	return &inceptionProvider{runTestProvider: blockedProvider(id)}
}

// workspaceActionsResponse decodes the payload the browser receives from
// GET /api/workspace/actions. It is written out rather than reusing
// workspaceActionsView so the assertions are about the wire shape — the flat
// action fields included — and not about the server's own struct.
type workspaceActionsResponse struct {
	Template struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	} `json:"template"`
	HasPRD     bool `json:"has_prd"`
	HasBacklog bool `json:"has_backlog"`
	Actions    []struct {
		ID                string `json:"id"`
		Label             string `json:"label"`
		Skill             string `json:"skill"`
		Offered           bool   `json:"offered"`
		Runnable          bool   `json:"runnable"`
		UnavailableReason string `json:"unavailable_reason"`
	} `json:"actions"`
	Execution *execution.Execution `json:"execution"`
}

func readWorkspaceActions(t *testing.T, srv *Server) (workspaceActionsResponse, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/actions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace/actions: %d %s", w.Code, w.Body.String())
	}
	var view workspaceActionsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view, w.Body.String()
}

// inceptionActionOf isolates the one action under test, so a Template that
// grows a second workspace action does not silently change what is asserted.
func inceptionActionOf(t *testing.T, view workspaceActionsResponse) workspaceActionRow {
	t.Helper()
	return workspaceActionOf(t, view, string(execution.ActionInception))
}

// backlogActionOf is inceptionActionOf's twin for the action under test here.
func backlogActionOf(t *testing.T, view workspaceActionsResponse) workspaceActionRow {
	t.Helper()
	return workspaceActionOf(t, view, string(execution.ActionBacklog))
}

// workspaceActionRow is one decoded row of the actions payload, named so the
// two helpers above can return it without repeating the anonymous struct.
type workspaceActionRow = struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Skill             string `json:"skill"`
	Offered           bool   `json:"offered"`
	Runnable          bool   `json:"runnable"`
	UnavailableReason string `json:"unavailable_reason"`
}

func workspaceActionOf(t *testing.T, view workspaceActionsResponse, id string) workspaceActionRow {
	t.Helper()
	for _, action := range view.Actions {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("the workspace does not offer the %q action: %#v", id, view.Actions)
	return view.Actions[0]
}

// rawWorkspaceAction returns one action exactly as it travels on the wire, so a
// test can assert which keys are present on that action alone instead of
// searching the whole payload, where another action's fields would answer for
// it.
func rawWorkspaceAction(t *testing.T, raw, id string) map[string]any {
	t.Helper()
	var payload struct {
		Actions []map[string]any `json:"actions"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	for _, action := range payload.Actions {
		if action["id"] == id {
			return action
		}
	}
	t.Fatalf("the workspace does not offer the %q action: %s", id, raw)
	return nil
}

func startWorkspaceAction(t *testing.T, srv *Server, action string) (int, map[string]any) {
	t.Helper()
	payload := map[string]any{}
	if action != "" {
		payload["action"] = action
	}
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/execution", payload)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("undecodable response (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, body
}

func readPRDBody(t *testing.T, srv *Server) string {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/prd", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/prd: %d %s", w.Code, w.Body.String())
	}
	var view prdView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view.Body
}

// writePRDFile seeds the workspace with a document that already belongs to it,
// at the very path the connector reads.
func writePRDFile(t *testing.T, root, body string) string {
	t.Helper()
	path := filepath.Join(root, "docs", "PRD.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// AC-1: a workspace with no PRD offers the inception, and the payload says so
// with the same vocabulary a spec action uses.
func TestWorkspaceActionsOfferInceptionWithoutAPRD(t *testing.T) {
	srv, _, _ := newRunServer(t, releasedInceptionProvider("fake", nil), true)

	view, raw := readWorkspaceActions(t, srv)
	if view.HasPRD {
		t.Fatalf("an empty workspace reports a PRD: %s", raw)
	}
	tpl := template.Default()
	if view.Template.ID != tpl.ID || view.Template.Version != tpl.Version {
		t.Fatalf("unexpected template identity: %#v", view.Template)
	}
	if len(view.Actions) != len(tpl.WorkspaceActions) {
		t.Fatalf("expected %d workspace actions, got %#v", len(tpl.WorkspaceActions), view.Actions)
	}
	action := inceptionActionOf(t, view)
	if !action.Runnable {
		t.Fatalf("the inception is not runnable on an empty workspace: %#v", action)
	}
	if action.Label != tpl.WorkspaceActions[0].Label || action.Skill != tpl.WorkspaceActions[0].Skill {
		t.Fatalf("the action diverges from the Template: %#v", action)
	}
	// A runnable action carries no reason on the wire either, so no client can
	// render an explanation next to an action that has none. The check is
	// scoped to this action: another workspace action that is not runnable here
	// legitimately carries its own reason.
	if _, ok := rawWorkspaceAction(t, raw, string(execution.ActionInception))["unavailable_reason"]; ok {
		t.Fatalf("a runnable action carries an unavailable_reason:\n%s", raw)
	}
	if view.Execution != nil {
		t.Fatalf("a workspace that never ran anything reports an execution: %#v", view.Execution)
	}
}

// AC-1: a default provider that cannot do an inception is reported as such by
// name, and the route refuses with the very same sentence.
func TestWorkspaceInceptionRefusedWhenTheProviderLacksTheCapability(t *testing.T) {
	// releasedProvider declares only spec.plan.
	srv, cfg, _ := newRunServer(t, releasedProvider("fake", nil), true)

	view, _ := readWorkspaceActions(t, srv)
	action := inceptionActionOf(t, view)
	if action.Runnable {
		t.Fatalf("an incapable provider still offers the inception: %#v", action)
	}
	if !strings.Contains(action.UnavailableReason, string(execution.CapabilityWorkspaceInception)) {
		t.Fatalf("the reason does not name the missing capability: %q", action.UnavailableReason)
	}

	status, body := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusConflict {
		t.Fatalf("POST: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, string(execution.CapabilityWorkspaceInception)) {
		t.Fatalf("the refusal does not name the missing capability: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 0 {
		t.Fatalf("a refused start created %d records", got)
	}
}

// AC-1: with nothing configured the refusal points at the configuration rather
// than at the workspace.
func TestWorkspaceInceptionRefusedWithoutADefaultProvider(t *testing.T) {
	srv, cfg, _ := newRunServer(t, releasedInceptionProvider("fake", nil), false)

	view, _ := readWorkspaceActions(t, srv)
	action := inceptionActionOf(t, view)
	if action.Runnable {
		t.Fatalf("an unconfigured workspace offers the inception: %#v", action)
	}
	if !strings.Contains(action.UnavailableReason, "no default execution provider is configured") {
		t.Fatalf("the reason does not invite to pick a provider: %q", action.UnavailableReason)
	}

	status, body := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusConflict {
		t.Fatalf("POST: %d %v", status, body)
	}
	if message, _ := body["error"].(string); !strings.Contains(message, "no default execution provider is configured") {
		t.Fatalf("the refusal does not invite to pick a provider: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 0 {
		t.Fatalf("a refused start created %d records", got)
	}
}

// AC-5: a workspace that already has a PRD is not offered a *first* inception,
// the route refuses it, and the document is not touched by the attempt.
func TestWorkspaceInceptionRefusedWhenAPRDAlreadyExists(t *testing.T) {
	srv, cfg, _ := newRunServer(t, releasedInceptionProvider("fake", nil), true)
	const existing = "# PRD esistente\n\nQuesto documento appartiene al workspace.\n"
	path := writePRDFile(t, cfg.ProjectRoot, existing)
	before := mustReadFile(t, path)

	view, _ := readWorkspaceActions(t, srv)
	if !view.HasPRD {
		t.Fatal("a workspace with a PRD reports has_prd:false")
	}
	action := inceptionActionOf(t, view)
	if action.Runnable {
		t.Fatalf("a first inception is offered over an existing PRD: %#v", action)
	}
	if !strings.Contains(strings.ToLower(action.UnavailableReason), "prd") {
		t.Fatalf("the reason does not name the existing PRD: %q", action.UnavailableReason)
	}

	status, body := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusConflict {
		t.Fatalf("POST: %d %v", status, body)
	}
	if message, _ := body["error"].(string); !strings.Contains(strings.ToLower(message), "prd") {
		t.Fatalf("the refusal does not name the existing PRD: %q", message)
	}
	if after := mustReadFile(t, path); string(after) != string(before) {
		t.Fatalf("the existing PRD was rewritten:\n%s", after)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 0 {
		t.Fatalf("a refused inception created %d records", got)
	}
	// The document is still the one the workspace had, read back through the
	// route the UI uses.
	if got := readPRDBody(t, srv); got != existing {
		t.Fatalf("the PRD read back differs from the one on disk: %q", got)
	}
}

// AC-1, AC-3: a successful inception closes as SUCCEEDED and the document it
// wrote is readable from the same server process, with no restart.
func TestWorkspaceInceptionSuccessPersistsAReadablePRD(t *testing.T) {
	const produced = "# PRD generato\n\nVisione, personas, MVP.\n"
	var conn connector.Connector
	provider := releasedInceptionProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		if request.Action != execution.ActionInception || request.SpecCode != "" {
			return execution.Result{}, errors.New("the workspace action reached the provider as " + string(request.Action) + " on spec " + request.SpecCode)
		}
		if _, err := conn.SavePRD(ctx, produced); err != nil {
			return execution.Result{}, err
		}
		return execution.Result{Payload: json.RawMessage(`{"prd":"docs/PRD.md"}`), ExternalID: "inception-1"}, nil
	})
	srv, cfg, conn := newRunServer(t, provider, true)

	status, started := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	if started["status"] != string(execution.StatusRunning) {
		t.Fatalf("the started record is not RUNNING: %v", started)
	}
	if started["spec_code"] != "" {
		t.Fatalf("a workspace execution carries a spec code: %v", started["spec_code"])
	}
	if started["action"] != string(execution.ActionInception) || started["provider_id"] != "fake" {
		t.Fatalf("the started record does not describe the run: %v", started)
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatalf("the started record has no id: %v", started)
	}

	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusSucceeded || record.Result == nil || record.CompletedAt == nil {
		t.Fatalf("terminal record: %#v", record)
	}
	if got := readPRDBody(t, srv); got != produced {
		t.Fatalf("the PRD is not readable without a restart: %q", got)
	}
	view, _ := readWorkspaceActions(t, srv)
	if !view.HasPRD {
		t.Fatal("the workspace still reports no PRD after a successful inception")
	}
	if view.Execution == nil || view.Execution.ID != id {
		t.Fatalf("the workspace lost the execution it started: %#v", view.Execution)
	}
	if action := inceptionActionOf(t, view); action.Runnable {
		t.Fatalf("a second first-inception is offered after a successful one: %#v", action)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 1 {
		t.Fatalf("expected one workspace record, got %d", got)
	}
}

// AC-4: a run that fails halfway leaves the workspace without a PRD and says
// why. The partial document the run wrote is taken back, so the workspace is
// exactly as it was before.
func TestWorkspaceInceptionFailureLeavesNoPRDAndDeclaresWhy(t *testing.T) {
	var conn connector.Connector
	provider := releasedInceptionProvider("fake", func(ctx context.Context, _ execution.Request) (execution.Result, error) {
		if _, err := conn.SavePRD(ctx, "# PRD parziale\n\nInterrotto a metà.\n"); err != nil {
			return execution.Result{}, err
		}
		return execution.Result{}, errors.New("the agent was interrupted before finishing the PRD")
	})
	srv, cfg, conn := newRunServer(t, provider, true)

	status, started := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)

	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
		t.Fatalf("a failed inception does not declare why: %#v", record)
	}
	if !strings.Contains(record.Error.Message, "interrupted") {
		t.Fatalf("the reason of the failure was lost: %q", record.Error.Message)
	}
	if got := readPRDBody(t, srv); got != "" {
		t.Fatalf("a failed inception left a PRD behind: %q", got)
	}
	if _, err := os.Stat(filepath.Join(cfg.ProjectRoot, "docs", "PRD.md")); !os.IsNotExist(err) {
		t.Fatalf("the partial document is still on disk: %v", err)
	}
	view, _ := readWorkspaceActions(t, srv)
	if view.HasPRD {
		t.Fatal("the workspace reports a PRD after a failed inception")
	}
	// The failure does not lock the workspace out of a retry.
	if action := inceptionActionOf(t, view); !action.Runnable {
		t.Fatalf("a failed inception cannot be retried: %#v", action)
	}
}

// AC-1: one press, one execution. The second request creates no record and
// names the one already holding the workspace.
func TestWorkspaceInceptionStartsExactlyOneExecution(t *testing.T) {
	provider := blockedInceptionProvider("fake")
	srv, cfg, _ := newRunServer(t, provider, true)

	status, first := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusCreated {
		t.Fatalf("first POST: %d %v", status, first)
	}
	id, _ := first["id"].(string)
	<-provider.entered

	status, second := startWorkspaceAction(t, srv, string(execution.ActionInception))
	if status != http.StatusConflict {
		t.Fatalf("second POST: %d %v", status, second)
	}
	if message, _ := second["error"].(string); !strings.Contains(message, id) {
		t.Fatalf("the refusal does not name the running execution: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 1 {
		t.Fatalf("a second press created %d records", got)
	}
	// While it runs, the action advertises the very same refusal.
	view, _ := readWorkspaceActions(t, srv)
	action := inceptionActionOf(t, view)
	if action.Runnable || !strings.Contains(action.UnavailableReason, id) {
		t.Fatalf("the busy action is still offered: %#v", action)
	}

	close(provider.release)
	awaitTerminal(t, srv, id)
}

// A name the process does not declare is a malformed request, not a state
// conflict: the two are answered differently on purpose.
func TestWorkspaceExecutionRejectsAnUnknownAction(t *testing.T) {
	srv, _, _ := newRunServer(t, releasedInceptionProvider("fake", nil), true)

	for _, action := range []string{"", "  ", "inesistente", "plan"} {
		status, body := startWorkspaceAction(t, srv, action)
		if status != http.StatusBadRequest {
			t.Fatalf("action %q: %d %v", action, status, body)
		}
		if message, _ := body["error"].(string); strings.TrimSpace(message) == "" {
			t.Fatalf("action %q was refused without a reason", action)
		}
	}
}

// backlogProvider declares every workspace capability the process needs,
// including workspace.backlog. Only the provider is a double: the connector it
// writes through, the record store, the effect confirmation, the rollback and
// the process Template are the production ones.
type backlogProvider struct {
	*runTestProvider
}

func (p *backlogProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{
		execution.CapabilitySpecPlan,
		execution.CapabilityWorkspaceInception,
		execution.CapabilityWorkspaceBacklog,
	}, nil
}

func releasedBacklogProvider(id string, execute func(context.Context, execution.Request) (execution.Result, error)) *backlogProvider {
	return &backlogProvider{runTestProvider: releasedProvider(id, execute)}
}

func blockedBacklogProvider(id string) *backlogProvider {
	return &backlogProvider{runTestProvider: blockedProvider(id)}
}

// generatedBacklog is what a real archetipo-spec run persists: three specs
// spread over two epics, written through the connector the server serves.
func generatedBacklog() []domain.Spec {
	return []domain.Spec{
		{Code: "US-001", Title: "Prima storia", Epic: domain.Epic{Code: "EP-001", Title: "Fondamenta"}, Priority: domain.PriorityHigh, Points: 3, Status: domain.StatusTodo},
		{Code: "US-002", Title: "Seconda storia", Epic: domain.Epic{Code: "EP-001", Title: "Fondamenta"}, Priority: domain.PriorityMedium, Points: 2, Status: domain.StatusTodo},
		{Code: "US-003", Title: "Terza storia", Epic: domain.Epic{Code: "EP-002", Title: "Esperienza"}, Priority: domain.PriorityLow, Points: 5, Status: domain.StatusTodo},
	}
}

// readBoard reads the board through the very route the browser uses. A
// workspace with no backlog at all is answered by the connector as a missing
// precondition (404), which is the board's way of saying "there is nothing
// here"; it is reported as an empty board rather than as a failure, so a test
// can compare "before" and "after a rollback" with the same helper.
func readBoard(t *testing.T, srv *Server) boardView {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/board", nil)
	if w.Code == http.StatusNotFound {
		return boardView{}
	}
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/board: %d %s", w.Code, w.Body.String())
	}
	var view boardView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

// boardSpecCodes flattens the board into the codes it shows, in no particular
// column: what AC-2 and AC-4 are about is whether the specs are there at all.
func boardSpecCodes(t *testing.T, srv *Server) []string {
	t.Helper()
	codes := []string{}
	for _, column := range readBoard(t, srv).Columns {
		for _, spec := range column.Specs {
			codes = append(codes, spec.Code)
		}
	}
	sort.Strings(codes)
	return codes
}

func boardEpicCodes(t *testing.T, srv *Server) []string {
	t.Helper()
	codes := []string{}
	for _, epic := range readBoard(t, srv).Epics {
		codes = append(codes, epic.Code)
	}
	sort.Strings(codes)
	return codes
}

// backlogFiles snapshots every file the backlog is made of, so a refused start
// can be shown to have left them byte-identical.
func backlogFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	base := filepath.Join(root, ".archetipo")
	for _, dir := range []string{base, filepath.Join(base, "specs")} {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			out[path] = string(mustReadFile(t, path))
		}
	}
	return out
}

// AC-1: a workspace that has a PRD and no backlog is exactly the workspace the
// backlog action targets, and the payload offers it.
//
// The connector answers "there is no backlog here" with a missing precondition,
// not with an empty summary. Reading that as "the backlog is unreadable" would
// hide the action from the only workspace it is for, so the assertion on the
// reason is explicit and not just a consequence of runnable being true.
func TestWorkspaceActionsOfferTheBacklogWithAPRDAndNoBacklog(t *testing.T) {
	srv, cfg, _ := newEmptyRunServer(t, releasedBacklogProvider("fake", nil), true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	view, raw := readWorkspaceActions(t, srv)
	if !view.HasPRD {
		t.Fatalf("a workspace with a PRD reports has_prd:false: %s", raw)
	}
	if view.HasBacklog {
		t.Fatalf("a workspace with no backlog reports has_backlog:true: %s", raw)
	}
	action := backlogActionOf(t, view)
	if !action.Offered || !action.Runnable {
		t.Fatalf("the backlog is not offered on the workspace it targets: %#v", action)
	}
	if action.UnavailableReason != "" {
		t.Fatalf("an offered and runnable action carries a reason: %q", action.UnavailableReason)
	}
	if strings.Contains(strings.ToLower(action.UnavailableReason), "could not be read") {
		t.Fatalf("a missing backlog was read as an unreadable one: %q", action.UnavailableReason)
	}
	if _, ok := rawWorkspaceAction(t, raw, string(execution.ActionBacklog))["unavailable_reason"]; ok {
		t.Fatalf("a runnable action carries an unavailable_reason on the wire:\n%s", raw)
	}
	if offered, ok := rawWorkspaceAction(t, raw, string(execution.ActionBacklog))["offered"].(bool); !ok || !offered {
		t.Fatalf("the wire payload does not offer the backlog:\n%s", raw)
	}
	// The same PRD that offers the backlog is what rules out a first inception.
	inception := inceptionActionOf(t, view)
	if inception.Offered || inception.Runnable || strings.TrimSpace(inception.UnavailableReason) == "" {
		t.Fatalf("a first inception is still offered over an existing PRD: %#v", inception)
	}
}

// US-040 regression: with no PRD the inception is the offered action and the
// backlog is not, with a reason that names the missing PRD.
func TestWorkspaceBacklogIsNotOfferedWithoutAPRD(t *testing.T) {
	srv, cfg, _ := newEmptyRunServer(t, releasedBacklogProvider("fake", nil), true)

	view, _ := readWorkspaceActions(t, srv)
	inception := inceptionActionOf(t, view)
	if !inception.Offered || !inception.Runnable {
		t.Fatalf("the inception is not offered on an empty workspace: %#v", inception)
	}
	action := backlogActionOf(t, view)
	if action.Offered || action.Runnable {
		t.Fatalf("the backlog is offered without a PRD: %#v", action)
	}
	if !strings.Contains(strings.ToLower(action.UnavailableReason), "prd") {
		t.Fatalf("the reason does not name the missing PRD: %q", action.UnavailableReason)
	}

	status, body := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusConflict {
		t.Fatalf("POST: %d %v", status, body)
	}
	if message, _ := body["error"].(string); !strings.Contains(strings.ToLower(message), "prd") {
		t.Fatalf("the refusal does not name the missing PRD: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 0 {
		t.Fatalf("a refused start created %d records", got)
	}
}

// AC-2: a confirmed success makes the generated backlog visible on the board of
// the same running server — no restart of the viewer is part of the assertion.
func TestWorkspaceBacklogSuccessPopulatesTheBoard(t *testing.T) {
	var conn connector.Connector
	provider := releasedBacklogProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		if request.Action != execution.ActionBacklog || request.SpecCode != "" {
			return execution.Result{}, errors.New("the workspace action reached the provider as " + string(request.Action) + " on spec " + request.SpecCode)
		}
		if _, err := conn.SaveInitialBacklog(ctx, generatedBacklog()); err != nil {
			return execution.Result{}, err
		}
		return execution.Result{Payload: json.RawMessage(`{"specs":3}`), ExternalID: "backlog-1"}, nil
	})
	srv, cfg, conn := newEmptyRunServer(t, provider, true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	status, started := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	if started["status"] != string(execution.StatusRunning) {
		t.Fatalf("the started record is not RUNNING: %v", started)
	}
	if started["spec_code"] != "" {
		t.Fatalf("a workspace execution carries a spec code: %v", started["spec_code"])
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatalf("the started record has no id: %v", started)
	}

	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusSucceeded || record.Result == nil || record.CompletedAt == nil {
		t.Fatalf("terminal record: %#v", record)
	}
	if got := boardSpecCodes(t, srv); !reflect.DeepEqual(got, []string{"US-001", "US-002", "US-003"}) {
		t.Fatalf("the board does not show the generated specs: %v", got)
	}
	if got := boardEpicCodes(t, srv); !reflect.DeepEqual(got, []string{"EP-001", "EP-002"}) {
		t.Fatalf("the board does not show the generated epics: %v", got)
	}
	view, _ := readWorkspaceActions(t, srv)
	if !view.HasBacklog {
		t.Fatal("the workspace still reports no backlog after a successful generation")
	}
	action := backlogActionOf(t, view)
	if action.Offered || action.Runnable {
		t.Fatalf("a second first-generation is offered after a successful one: %#v", action)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 1 {
		t.Fatalf("expected one workspace record, got %d", got)
	}
}

// AC-3: a workspace that already has a backlog is not offered a *first*
// generation, the route refuses it with the same sentence, and nothing on disk
// changes.
func TestWorkspaceBacklogRefusedWhenABacklogAlreadyExists(t *testing.T) {
	srv, cfg, _ := newRunServer(t, releasedBacklogProvider("fake", nil), true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")
	before := backlogFiles(t, cfg.ProjectRoot)

	view, _ := readWorkspaceActions(t, srv)
	if !view.HasBacklog {
		t.Fatal("a workspace with specs reports has_backlog:false")
	}
	action := backlogActionOf(t, view)
	if action.Offered || action.Runnable {
		t.Fatalf("a first generation is offered over an existing backlog: %#v", action)
	}
	if !strings.Contains(strings.ToLower(action.UnavailableReason), "backlog") {
		t.Fatalf("the reason does not name the existing backlog: %q", action.UnavailableReason)
	}

	status, body := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusConflict {
		t.Fatalf("POST: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if message != action.UnavailableReason {
		t.Fatalf("the refusal and the payload disagree:\n%q\n%q", message, action.UnavailableReason)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 0 {
		t.Fatalf("a refused generation created %d records", got)
	}
	if got := backlogFiles(t, cfg.ProjectRoot); !reflect.DeepEqual(got, before) {
		t.Fatal("the existing backlog was rewritten by a refused generation")
	}
}

// AC-4: a generation that fails halfway leaves the workspace with no backlog at
// all, and the record declares both the original cause and the removal of the
// partial result.
func TestWorkspaceBacklogFailureLeavesNoBacklogAndDeclaresWhy(t *testing.T) {
	var conn connector.Connector
	provider := releasedBacklogProvider("fake", func(ctx context.Context, _ execution.Request) (execution.Result, error) {
		partial := generatedBacklog()[:1]
		if _, err := conn.SaveInitialBacklog(ctx, partial); err != nil {
			return execution.Result{}, err
		}
		return execution.Result{}, errors.New("the agent was interrupted before finishing the backlog")
	})
	srv, cfg, conn := newEmptyRunServer(t, provider, true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	status, started := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)

	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
		t.Fatalf("a failed generation does not declare why: %#v", record)
	}
	if !strings.Contains(record.Error.Message, "interrupted") {
		t.Fatalf("the reason of the failure was lost: %q", record.Error.Message)
	}
	if !strings.Contains(record.Error.Message, "partial backlog") {
		t.Fatalf("the record does not declare the removal of the partial backlog: %q", record.Error.Message)
	}
	if got := boardSpecCodes(t, srv); len(got) != 0 {
		t.Fatalf("a failed generation left specs on the board: %v", got)
	}
	view, _ := readWorkspaceActions(t, srv)
	if view.HasBacklog {
		t.Fatal("the workspace reports a backlog after a failed generation")
	}
	// The failure does not lock the workspace out of a retry.
	if action := backlogActionOf(t, view); !action.Offered || !action.Runnable {
		t.Fatalf("a failed generation cannot be retried: %#v", action)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 1 {
		t.Fatalf("expected one workspace record, got %d", got)
	}
}

// AC-4: a success the workspace does not back is not a success. The record is
// closed as FAILED with UNCONFIRMED_EFFECT and the backlog stays absent.
func TestWorkspaceBacklogUnconfirmedSuccessBecomesAFailure(t *testing.T) {
	provider := releasedBacklogProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{Payload: json.RawMessage(`{"specs":3}`), ExternalID: "backlog-2"}, nil
	})
	srv, cfg, _ := newEmptyRunServer(t, provider, true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	status, started := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)

	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil {
		t.Fatalf("an unbacked success stayed a success: %#v", record)
	}
	if record.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("unexpected error code: %#v", record.Error)
	}
	if got := boardSpecCodes(t, srv); len(got) != 0 {
		t.Fatalf("the board shows specs nobody wrote: %v", got)
	}
	view, _ := readWorkspaceActions(t, srv)
	if view.HasBacklog {
		t.Fatal("the workspace reports a backlog after an unconfirmed generation")
	}
}

// One press, one execution: the second request creates no record and names the
// one already holding the workspace.
func TestWorkspaceBacklogStartsExactlyOneExecution(t *testing.T) {
	provider := blockedBacklogProvider("fake")
	srv, cfg, _ := newEmptyRunServer(t, provider, true)
	writePRDFile(t, cfg.ProjectRoot, "# PRD\n\nVisione e MVP.\n")

	status, first := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusCreated {
		t.Fatalf("first POST: %d %v", status, first)
	}
	id, _ := first["id"].(string)
	<-provider.entered

	status, second := startWorkspaceAction(t, srv, string(execution.ActionBacklog))
	if status != http.StatusConflict {
		t.Fatalf("second POST: %d %v", status, second)
	}
	if message, _ := second["error"].(string); !strings.Contains(message, id) {
		t.Fatalf("the refusal does not name the running execution: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, workspaceExecutionKey); got != 1 {
		t.Fatalf("a second press created %d records", got)
	}
	// While it runs, the action advertises the very same refusal, but it is
	// still offered: a busy workspace is not a workspace that has outgrown the
	// action.
	view, _ := readWorkspaceActions(t, srv)
	action := backlogActionOf(t, view)
	if action.Runnable || !strings.Contains(action.UnavailableReason, id) {
		t.Fatalf("the busy action is still runnable: %#v", action)
	}
	if !action.Offered {
		t.Fatalf("a busy workspace stopped offering the action: %#v", action)
	}

	close(provider.release)
	awaitTerminal(t, srv, id)
}
