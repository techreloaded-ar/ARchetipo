package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
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
	HasPRD  bool `json:"has_prd"`
	Actions []struct {
		ID                string `json:"id"`
		Label             string `json:"label"`
		Skill             string `json:"skill"`
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
func inceptionActionOf(t *testing.T, view workspaceActionsResponse) struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Skill             string `json:"skill"`
	Runnable          bool   `json:"runnable"`
	UnavailableReason string `json:"unavailable_reason"`
} {
	t.Helper()
	for _, action := range view.Actions {
		if action.ID == string(execution.ActionInception) {
			return action
		}
	}
	t.Fatalf("the workspace does not offer the inception action: %#v", view.Actions)
	return view.Actions[0]
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
	// render an explanation next to an action that has none.
	if strings.Contains(raw, "unavailable_reason") {
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
