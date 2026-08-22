package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// workspaceRunsCollaborator is a run collaborator that answers *per record*.
//
// It is deliberately not fakeCollaborator: that double resolves every execution
// to the same run and therefore cannot express the only thing these tests are
// about — that one run of the workspace is waiting for a decision while another
// is not, and that a run which cannot be resolved at all does not take the
// others down with it.
type workspaceRunsCollaborator struct {
	mu sync.Mutex

	// runs maps an execution id to the run behind it, and refusals maps an
	// execution id to the refusal ResolveRun answers with instead.
	runs     map[string]string
	refusals map[string]error

	// approvals is what each run is waiting on, keyed by run id.
	approvals map[string][]execution.PendingApproval

	// approvalReads counts, per run, how many times the pending approvals were
	// really read. Without it "awaiting_response is false" could equally mean
	// "this run has nothing pending" or "nobody has looked yet", and only the
	// first of the two is an assertion worth making.
	approvalReads map[string]int
}

func newWorkspaceRunsCollaborator() *workspaceRunsCollaborator {
	return &workspaceRunsCollaborator{
		runs:          map[string]string{},
		refusals:      map[string]error{},
		approvals:     map[string][]execution.PendingApproval{},
		approvalReads: map[string]int{},
	}
}

func (c *workspaceRunsCollaborator) ResolveRun(_ context.Context, exec execution.Execution, _ map[string]any) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.refusals[exec.ID]; err != nil {
		return "", err
	}
	return c.runs[exec.ID], nil
}

func (c *workspaceRunsCollaborator) ReadRun(_ context.Context, req execution.RunRequest) (execution.RunSnapshot, error) {
	return execution.RunSnapshot{RunID: req.RunID, State: execution.RunActive}, nil
}

func (c *workspaceRunsCollaborator) ReadRunApprovals(_ context.Context, req execution.RunRequest) ([]execution.PendingApproval, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.approvalReads[req.RunID]++
	pending := c.approvals[req.RunID]
	out := make([]execution.PendingApproval, len(pending))
	copy(out, pending)
	return out, nil
}

// StreamRunEvents stays open until the follower is closed: these tests are
// about the pending decision, not about the event history, and a stream that
// returned immediately would only make the follower reconnect in a loop.
func (c *workspaceRunsCollaborator) StreamRunEvents(ctx context.Context, _ execution.RunRequest, _ int64, _ func(execution.RunEvent) error) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *workspaceRunsCollaborator) SendRunMessage(context.Context, execution.RunRequest, string) error {
	return nil
}

func (c *workspaceRunsCollaborator) RespondRunApproval(context.Context, execution.RunRequest, string, string) error {
	return nil
}

func (c *workspaceRunsCollaborator) CancelRun(context.Context, execution.RunRequest) error {
	return nil
}

func (c *workspaceRunsCollaborator) setRun(executionID, runID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs[executionID] = runID
}

func (c *workspaceRunsCollaborator) refuse(executionID string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refusals[executionID] = err
}

func (c *workspaceRunsCollaborator) setApprovals(runID string, approvals []execution.PendingApproval) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.approvals[runID] = approvals
}

func (c *workspaceRunsCollaborator) approvalReadCount(runID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.approvalReads[runID]
}

// workspaceRunsProvider is a provider that also exposes interactive runs. Only
// the provider side is a double: the store, the followers, the route and the
// JSON serialization are the production ones.
type workspaceRunsProvider struct {
	*runTestProvider
	*workspaceRunsCollaborator
}

// workspaceRunsResponse decodes the wire shape the rail receives. It is written
// out instead of reusing workspaceRunsView so the assertions are on the JSON
// contract and not on the Go struct behind it.
type workspaceRunsResponse struct {
	Runs []struct {
		ID       string `json:"id"`
		Scope    string `json:"scope"`
		SpecCode string `json:"spec_code"`
		Action   string `json:"action"`
		Status   string `json:"status"`
		Pending  *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"pending"`
		AwaitingResponse bool   `json:"awaiting_response"`
		Notice           string `json:"notice"`
	} `json:"runs"`
}

func (r workspaceRunsResponse) byID(id string) (int, bool) {
	for i, run := range r.Runs {
		if run.ID == id {
			return i, true
		}
	}
	return 0, false
}

// workspaceRunsFixture is a viewer over a real workspace whose execution
// records the test seeds itself.
type workspaceRunsFixture struct {
	srv          *Server
	collaborator *workspaceRunsCollaborator

	mu sync.Mutex
	// requested records every path this fixture asked the viewer for. It is the
	// oracle of AC-4: see readRuns.
	requested []string
}

func newWorkspaceRunsFixture(t *testing.T) *workspaceRunsFixture {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := filefs.New(cfg)
	seedRunSpecs(t, conn)

	collaborator := newWorkspaceRunsCollaborator()
	provider := &workspaceRunsProvider{
		runTestProvider:           releasedProvider("workspace-runs", nil),
		workspaceRunsCollaborator: collaborator,
	}
	registry := execution.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if _, err := config.UpdateDefaultProvider(dir, config.DefaultProviderConfig{ID: provider.ID(), Config: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srv.session().followers.closeAll()
		srv.session().dispatch.wait(5 * time.Second)
	})
	return &workspaceRunsFixture{srv: srv, collaborator: collaborator}
}

// seedRun writes one execution record straight into the store and returns its
// id. No provider is dispatched: what this route reads is the record, and
// starting a real run would only add a scheduler to the oracle.
func (f *workspaceRunsFixture) seedRun(t *testing.T, specCode string, action execution.ActionID, status execution.ExecutionStatus) string {
	t.Helper()
	id, err := execution.RandomID()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := execution.RequiredCapability(action)
	if err != nil {
		t.Fatal(err)
	}
	record := execution.Execution{
		ID:         id,
		SpecCode:   specCode,
		Action:     action,
		Capability: capability,
		ProviderID: "workspace-runs",
		Status:     status,
		Result:     &execution.Result{ExternalID: "task-" + id},
		CreatedAt:  time.Now().UTC(),
	}
	if status != execution.StatusRunning {
		completed := time.Now().UTC()
		record.CompletedAt = &completed
	}
	if err := f.srv.session().store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	return id
}

// readRuns is the only way these tests talk to the viewer, and it records the
// path it asked for.
//
// That record is what makes TestWorkspaceRunsFlagsThePendingDecisionWithoutOpeningThePanel
// an assertion rather than a hope: the pending decision must be reported by the
// list route alone, so the test may never touch GET /api/execution/{id}/run —
// the route that attaches a follower for an *open panel*. If it did, the
// awaiting flag could be coming from that follower and the route under test
// could be doing nothing at all.
func (f *workspaceRunsFixture) readRuns(t *testing.T) (int, workspaceRunsResponse, string) {
	t.Helper()
	const path = "/api/workspace/runs"
	f.mu.Lock()
	f.requested = append(f.requested, path)
	f.mu.Unlock()

	w := doJSON(t, f.srv, http.MethodGet, path, nil)
	var view workspaceRunsResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatalf("undecodable runs view (%d): %s", w.Code, w.Body.String())
		}
	}
	return w.Code, view, w.Body.String()
}

// assertNeverOpenedARunPanel fails as soon as this test has asked the viewer
// for a run panel. See readRuns.
func (f *workspaceRunsFixture) assertNeverOpenedARunPanel(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, path := range f.requested {
		if strings.HasPrefix(path, "/api/execution/") {
			t.Fatalf("this test asked %q: the pending decision must be reported by the list route alone, or the assertion proves nothing", path)
		}
	}
}

func (f *workspaceRunsFixture) okRuns(t *testing.T) workspaceRunsResponse {
	t.Helper()
	status, view, body := f.readRuns(t)
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspace/runs = %d, want 200: %s", status, body)
	}
	return view
}

func workspaceApproval(id, title string) execution.PendingApproval {
	return execution.PendingApproval{
		ID:       id,
		ToolName: "Bash",
		Title:    title,
		Args:     json.RawMessage(`{"command":"ls"}`),
		Options: []execution.ApprovalOption{
			{ID: "allow-once", Label: "Allow once", Kind: "allow"},
			{ID: "deny", Label: "Deny", Kind: "deny"},
		},
		CreatedAt: time.UnixMilli(1755000000000).UTC(),
	}
}

// AC-3: the rail lists what is in flight in this workspace, whatever it is
// about — a spec or the workspace itself — and nothing that has already ended.
func TestWorkspaceRunsListsRunningExecutionsOfBothScopes(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	planning := fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusRunning)
	inception := fixture.seedRun(t, "", execution.ActionInception, execution.StatusRunning)
	finished := fixture.seedRun(t, "US-902", execution.ActionPlan, execution.StatusSucceeded)
	fixture.collaborator.setRun(planning, "run-plan")
	fixture.collaborator.setRun(inception, "run-inception")

	view := fixture.okRuns(t)
	if len(view.Runs) != 2 {
		t.Fatalf("runs = %d, want the two running executions: %#v", len(view.Runs), view.Runs)
	}

	at, ok := view.byID(planning)
	if !ok {
		t.Fatalf("the running plan of US-901 is missing: %#v", view.Runs)
	}
	if got := view.Runs[at]; got.Scope != string(execution.ScopeSpec) ||
		got.SpecCode != "US-901" ||
		got.Action != string(execution.ActionPlan) ||
		got.Status != string(execution.StatusRunning) {
		t.Fatalf("the plan entry does not describe the run: %#v", got)
	}

	at, ok = view.byID(inception)
	if !ok {
		t.Fatalf("the running inception of the workspace is missing: %#v", view.Runs)
	}
	if got := view.Runs[at]; got.Scope != string(execution.ScopeWorkspace) ||
		got.SpecCode != "" ||
		got.Action != string(execution.ActionInception) ||
		got.Status != string(execution.StatusRunning) {
		t.Fatalf("the inception entry does not describe the run: %#v", got)
	}

	if _, present := view.byID(finished); present {
		t.Fatalf("a terminal execution is listed as in flight: %#v", view.Runs)
	}
}

// AC-4: a decision waiting on a run is visible without that run having been
// opened. The whole point is that the list route attaches the follower itself,
// so this test never asks for GET /api/execution/{id}/run — see readRuns.
func TestWorkspaceRunsFlagsThePendingDecisionWithoutOpeningThePanel(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	waiting := fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusRunning)
	quiet := fixture.seedRun(t, "US-902", execution.ActionPlan, execution.StatusRunning)
	fixture.collaborator.setRun(waiting, "run-waiting")
	fixture.collaborator.setRun(quiet, "run-quiet")
	fixture.collaborator.setApprovals("run-waiting", []execution.PendingApproval{
		workspaceApproval("appr-7", "Eseguire il comando"),
	})

	// The first read is what attaches the followers; the flag lands as soon as
	// each of them has read its own pending approvals, which happens off the
	// request. Waiting for both reads is what makes the false below mean "this
	// run has nothing pending" and not "nobody has looked yet".
	var view workspaceRunsResponse
	waitFor(t, "both runs to have been asked for their pending approvals", func() bool {
		view = fixture.okRuns(t)
		return fixture.collaborator.approvalReadCount("run-waiting") > 0 &&
			fixture.collaborator.approvalReadCount("run-quiet") > 0
	})
	waitFor(t, "the waiting run to report its pending decision", func() bool {
		view = fixture.okRuns(t)
		at, ok := view.byID(waiting)
		return ok && view.Runs[at].AwaitingResponse
	})

	at, ok := view.byID(waiting)
	if !ok {
		t.Fatalf("the waiting run is missing: %#v", view.Runs)
	}
	got := view.Runs[at]
	if got.Pending == nil {
		t.Fatalf("the waiting run carries no decision: %#v", got)
	}
	if got.Pending.ID != "appr-7" || got.Pending.Title != "Eseguire il comando" {
		t.Fatalf("the decision is not named: %#v", got.Pending)
	}

	at, ok = view.byID(quiet)
	if !ok {
		t.Fatalf("the quiet run is missing: %#v", view.Runs)
	}
	if other := view.Runs[at]; other.AwaitingResponse || other.Pending != nil {
		t.Fatalf("a run with nothing pending is reported as waiting: %#v", other)
	}

	fixture.assertNeverOpenedARunPanel(t)
}

// One run that cannot be resolved is an entry with a notice, never an error
// that costs the person the sight of all the others.
func TestWorkspaceRunsKeepsListingWhenOneRunCannotBeResolved(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	reachable := fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusRunning)
	broken := fixture.seedRun(t, "US-902", execution.ActionPlan, execution.StatusRunning)
	fixture.collaborator.setRun(reachable, "run-reachable")
	fixture.collaborator.refuse(broken, &execution.RunCommandError{
		Reason: execution.RunRefusedRunnerOffline,
		RunID:  "run-broken",
	})

	status, view, body := fixture.readRuns(t)
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspace/runs = %d, want 200 despite the refused run: %s", status, body)
	}
	if len(view.Runs) != 2 {
		t.Fatalf("runs = %d, want both in-flight executions: %#v", len(view.Runs), view.Runs)
	}

	at, ok := view.byID(broken)
	if !ok {
		t.Fatalf("the unresolvable run was dropped from the list: %#v", view.Runs)
	}
	got := view.Runs[at]
	if got.Notice == "" {
		t.Fatalf("the unresolvable run carries no notice, so a false awaiting flag reads as a confident \"nothing to decide\": %#v", got)
	}
	if got.AwaitingResponse || got.Pending != nil {
		t.Fatalf("a run that could not even be asked is reported as waiting: %#v", got)
	}

	if _, ok := view.byID(reachable); !ok {
		t.Fatalf("the reachable run disappeared because another one refused: %#v", view.Runs)
	}
}

// An empty list is an array, never null: the rail iterates without testing.
func TestWorkspaceRunsAnswersEmptyListWhenNothingIsRunning(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusSucceeded)

	status, view, body := fixture.readRuns(t)
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspace/runs = %d, want 200: %s", status, body)
	}
	if len(view.Runs) != 0 {
		t.Fatalf("runs = %#v, want empty with nothing in flight", view.Runs)
	}
	if !strings.Contains(body, `"runs":[]`) {
		t.Fatalf("the empty list is not an array on the wire: %s", body)
	}
}

// Without an open workspace there is no list to give: the route declares the
// missing workspace exactly like every other workspace-scoped route.
func TestWorkspaceRunsRefusedWithoutAnOpenWorkspace(t *testing.T) {
	srv, _ := homeServer(t)

	rec := callRoute(t, srv, scopedRoute{
		pattern: "GET /api/workspace/runs",
		method:  http.MethodGet,
		path:    "/api/workspace/runs",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("GET /api/workspace/runs = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the refusal: %v (%s)", err, rec.Body.String())
	}
	if open, ok := body["workspaceOpen"].(bool); !ok || open {
		t.Fatalf("workspaceOpen = %v, want false", body["workspaceOpen"])
	}
	if _, present := body["runs"]; present {
		t.Fatalf("the refusal carries a runs list — it answered emptily instead of declaring that no workspace is open: %s", rec.Body.String())
	}
}

// openConversation installs a conversation on the workspace the fixture serves,
// so a decision can be recorded against it. The provider is the same stub the
// holder's own tests use: what these tests are about is the anchor the register
// keeps, not what closing a conversation releases.
func (f *workspaceRunsFixture) openConversation(t *testing.T, id string) {
	t.Helper()
	if err := f.srv.session().conversation.open(id, "stub", &stubConversationalist{}, nil, nil, t.TempDir(), time.Now(), "", ""); err != nil {
		t.Fatalf("opening the conversation: %v", err)
	}
}

// confirmRun records, in the conversation named by conversationID, that the
// proposal carried by proposalID was confirmed and started executionID. The
// conversation is named because a decision belongs to one of them: the anchor
// the run rail navigates to is a point of that history and of no other.
func (f *workspaceRunsFixture) confirmRun(t *testing.T, conversationID string, proposalID int64, executionID string) {
	t.Helper()
	outcome := confirmedOutcome(proposalID, "Implementa US-901", executionID)
	if err := f.srv.session().conversation.decide(conversationID, proposalID, outcome); err != nil {
		t.Fatalf("recording the decision: %v", err)
	}
}

// rawRunEntries decodes the response body into plain maps, keyed by run id.
//
// It exists because the two fields under test are `omitempty`: only the raw
// JSON can tell "the key is absent" from "the key is there and zero", and a
// typed struct would read both as the same empty string.
func rawRunEntries(t *testing.T, body string) map[string]map[string]any {
	t.Helper()
	var raw struct {
		Runs []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("undecodable runs view: %v (%s)", err, body)
	}
	entries := make(map[string]map[string]any, len(raw.Runs))
	for _, run := range raw.Runs {
		id, _ := run["id"].(string)
		entries[id] = run
	}
	return entries
}

func assertNoAnchorKeys(t *testing.T, entry map[string]any, what string) {
	t.Helper()
	for _, key := range []string{"conversation_id", "anchor_event_id"} {
		if _, present := entry[key]; present {
			t.Fatalf("%s carries %q on the wire: a run nobody asked for in a conversation must not promise a place to reach: %#v", what, key, entry)
		}
	}
}

// AC-6: an entry born from a conversation says which one and at which point,
// so the notice that a run is waiting has something to navigate to.
func TestWorkspaceRunsCarryTheConversationThatAskedThem(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	started := fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusRunning)
	fixture.collaborator.setRun(started, "run-started")
	fixture.openConversation(t, "conv-60")
	fixture.confirmRun(t, "conv-60", 42, started)

	status, _, body := fixture.readRuns(t)
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspace/runs = %d, want 200: %s", status, body)
	}
	entry, ok := rawRunEntries(t, body)[started]
	if !ok {
		t.Fatalf("the run started from the conversation is missing: %s", body)
	}
	if got, _ := entry["conversation_id"].(string); got != "conv-60" {
		t.Fatalf("conversation_id = %v, want the conversation that asked for the run: %#v", entry["conversation_id"], entry)
	}
	if got, _ := entry["anchor_event_id"].(float64); int64(got) != 42 {
		t.Fatalf("anchor_event_id = %v, want the event that carried the proposal: %#v", entry["anchor_event_id"], entry)
	}
}

// A run started from the board was born in no conversation: the two fields must
// be absent from its JSON, not present and empty.
func TestWorkspaceRunsOmitTheAnchorForABoardRun(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	asked := fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusRunning)
	fromBoard := fixture.seedRun(t, "US-902", execution.ActionPlan, execution.StatusRunning)
	fixture.collaborator.setRun(asked, "run-asked")
	fixture.collaborator.setRun(fromBoard, "run-board")
	fixture.openConversation(t, "conv-60")
	fixture.confirmRun(t, "conv-60", 42, asked)

	status, _, body := fixture.readRuns(t)
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspace/runs = %d, want 200: %s", status, body)
	}
	entries := rawRunEntries(t, body)
	if _, ok := entries[asked]; !ok {
		t.Fatalf("the run started from the conversation is missing: %s", body)
	}
	entry, ok := entries[fromBoard]
	if !ok {
		t.Fatalf("the run started from the board is missing: %s", body)
	}
	assertNoAnchorKeys(t, entry, "the board run")
}

// With no conversation open there is no history to point at, so no entry may
// carry the anchor.
func TestWorkspaceRunsOmitTheAnchorWithNoConversationOpen(t *testing.T) {
	fixture := newWorkspaceRunsFixture(t)
	planning := fixture.seedRun(t, "US-901", execution.ActionPlan, execution.StatusRunning)
	inception := fixture.seedRun(t, "", execution.ActionInception, execution.StatusRunning)
	fixture.collaborator.setRun(planning, "run-plan")
	fixture.collaborator.setRun(inception, "run-inception")

	status, _, body := fixture.readRuns(t)
	if status != http.StatusOK {
		t.Fatalf("GET /api/workspace/runs = %d, want 200: %s", status, body)
	}
	entries := rawRunEntries(t, body)
	if len(entries) != 2 {
		t.Fatalf("runs = %d, want the two running executions: %s", len(entries), body)
	}
	for id, entry := range entries {
		assertNoAnchorKeys(t, entry, "run "+id)
	}
}
