package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// collaboratingProvider is a provider that also exposes an interactive run. It
// is the only double in these tests: the follower, the routes, the JSON
// serialization and the refusal mapping are the production ones, which is what
// makes the boundary asserted here the one the browser really talks to.
type collaboratingProvider struct {
	*runTestProvider
	*fakeCollaborator
}

// runRoutesFixture is a viewer with one execution record and a run behind it.
type runRoutesFixture struct {
	srv          *Server
	collaborator *fakeCollaborator
	executionID  string
}

func newRunRoutesServer(t *testing.T, collaborator *fakeCollaborator) *runRoutesFixture {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := filefs.New(cfg)
	seedRunSpecs(t, conn)

	var provider execution.Provider
	if collaborator == nil {
		provider = releasedProvider("plain", nil)
	} else {
		provider = &collaboratingProvider{
			runTestProvider:  releasedProvider("collaborating", nil),
			fakeCollaborator: collaborator,
		}
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

	id, err := execution.RandomID()
	if err != nil {
		t.Fatal(err)
	}
	record := execution.Execution{
		ID:         id,
		SpecCode:   "US-901",
		Action:     execution.ActionPlan,
		Capability: execution.CapabilitySpecPlan,
		ProviderID: provider.ID(),
		Status:     execution.StatusRunning,
		Result:     &execution.Result{ExternalID: "task-1"},
		CreatedAt:  time.Now().UTC(),
	}
	if err := srv.session().store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	return &runRoutesFixture{srv: srv, collaborator: collaborator, executionID: id}
}

// runViewResponse decodes the wire shape the browser receives, written out on
// purpose so the assertion is on the JSON and not on the Go struct behind it.
type runViewResponse struct {
	Run *struct {
		RunID    string `json:"run_id"`
		State    string `json:"state"`
		Error    string `json:"error"`
		ClosedAt string `json:"closed_at"`
	} `json:"run"`
	Events []struct {
		ID   int64  `json:"id"`
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"events"`
	LastID    int64 `json:"last_id"`
	Approvals []struct {
		ID       string `json:"id"`
		ToolName string `json:"tool_name"`
		Title    string `json:"title"`
		Options  []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Kind  string `json:"kind"`
		} `json:"options"`
	} `json:"approvals"`
	Connected bool   `json:"connected"`
	Truncated bool   `json:"truncated"`
	Notice    string `json:"notice"`
}

func (f *runRoutesFixture) readRun(t *testing.T, afterID int64) (int, runViewResponse, string) {
	t.Helper()
	path := "/api/execution/" + f.executionID + "/run"
	if afterID > 0 {
		path += "?after_id=" + strconv.FormatInt(afterID, 10)
	}
	w := doJSON(t, f.srv, http.MethodGet, path, nil)
	var view runViewResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatalf("undecodable run view (%d): %s", w.Code, w.Body.String())
		}
	}
	return w.Code, view, w.Body.String()
}

func (f *runRoutesFixture) post(t *testing.T, path string, payload any) (int, runViewResponse, string) {
	t.Helper()
	w := doJSON(t, f.srv, http.MethodPost, "/api/execution/"+f.executionID+path, payload)
	var view runViewResponse
	if w.Code == http.StatusAccepted {
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatalf("undecodable run view (%d): %s", w.Code, w.Body.String())
		}
	}
	return w.Code, view, w.Body.String()
}

func viewEventIDs(view runViewResponse) []int64 {
	out := make([]int64, 0, len(view.Events))
	for _, event := range view.Events {
		out = append(out, event.ID)
	}
	return out
}

// runFingerprint is everything AC-6 says a refused command must leave alone.
type runFingerprint struct {
	EventIDs  []int64
	LastID    int64
	State     string
	Approvals []string
}

func fingerprintOf(view runViewResponse) runFingerprint {
	out := runFingerprint{EventIDs: viewEventIDs(view), LastID: view.LastID, Approvals: []string{}}
	if view.Run != nil {
		out.State = view.Run.State
	}
	for _, approval := range view.Approvals {
		out.Approvals = append(out.Approvals, approval.ID)
	}
	if out.EventIDs == nil {
		out.EventIDs = []int64{}
	}
	return out
}

// primeRun makes one GET so the follower exists, then returns its stream.
func (f *runRoutesFixture) primeRun(t *testing.T) *fakeStream {
	t.Helper()
	if status, _, body := f.readRun(t, 0); status != http.StatusOK {
		t.Fatalf("GET run = %d: %s", status, body)
	}
	return f.collaborator.nextStream(t)
}

func (f *runRoutesFixture) awaitLastID(t *testing.T, want int64) runViewResponse {
	t.Helper()
	var view runViewResponse
	waitFor(t, "the projection to reach last_id "+strconv.FormatInt(want, 10), func() bool {
		_, current, _ := f.readRun(t, 0)
		view = current
		return current.LastID == want
	})
	return view
}

func pendingApproval() execution.PendingApproval {
	return execution.PendingApproval{
		ID:       "appr-1",
		ToolName: "Bash",
		Title:    "Run a command",
		Args:     json.RawMessage(`{"command":"ls"}`),
		Options: []execution.ApprovalOption{
			{ID: "allow-once", Label: "Allow once", Kind: "allow"},
			{ID: "deny", Label: "Deny", Kind: "deny"},
		},
		CreatedAt: time.UnixMilli(1755000000000).UTC(),
	}
}

// AC-1
func TestRunReadIsOrderedAndDeduplicated(t *testing.T) {
	fixture := newRunRoutesServer(t, newFakeCollaborator())
	stream := fixture.primeRun(t)

	for _, id := range []int64{1, 2, 3} {
		stream.events <- event(id, "e")
	}
	fixture.awaitLastID(t, 3)
	// The hub is entitled to redeliver an event a resumed stream overlaps on.
	for _, id := range []int64{3, 4} {
		stream.events <- event(id, "e")
	}
	view := fixture.awaitLastID(t, 4)

	if got := viewEventIDs(view); !sameIDs(got, []int64{1, 2, 3, 4}) {
		t.Fatalf("events = %v, want 1..4 in order, each exactly once", got)
	}
	if view.LastID != 4 {
		t.Fatalf("last_id = %d, want 4", view.LastID)
	}

	status, after, body := fixture.readRun(t, 4)
	if status != http.StatusOK {
		t.Fatalf("GET run after_id=4 = %d: %s", status, body)
	}
	if len(after.Events) != 0 {
		t.Fatalf("events after the cursor = %v, want none", viewEventIDs(after))
	}
	if after.LastID != 4 {
		t.Fatalf("last_id = %d, want it unchanged at 4", after.LastID)
	}
}

// AC-2
func TestRunReadResumesAfterAReconnection(t *testing.T) {
	collaborator := newFakeCollaborator()
	fixture := newRunRoutesServer(t, collaborator)
	stream := fixture.primeRun(t)

	for _, id := range []int64{1, 2, 3} {
		stream.events <- event(id, "e")
	}
	fixture.awaitLastID(t, 3)
	stream.end <- errAborted()

	resumed := collaborator.nextStream(t)
	for _, id := range []int64{4, 5} {
		resumed.events <- event(id, "e")
	}
	view := fixture.awaitLastID(t, 5)

	cursors := collaborator.recordedCursors()
	if len(cursors) < 2 || cursors[1] != 3 {
		t.Fatalf("cursors = %v, want the second subscription to resume from 3", cursors)
	}
	if got := viewEventIDs(view); !sameIDs(got, []int64{1, 2, 3, 4, 5}) {
		t.Fatalf("events = %v, want 1..5 with no gap and no repetition", got)
	}
}

// AC-3
func TestSentMessageAppearsOnlyAfterTheProviderConfirms(t *testing.T) {
	collaborator := newFakeCollaborator()
	fixture := newRunRoutesServer(t, collaborator)
	stream := fixture.primeRun(t)

	const sentinel = "ciao agente"
	status, view, body := fixture.post(t, "/run/messages", map[string]any{"message": sentinel})
	if status != http.StatusAccepted {
		t.Fatalf("POST message = %d: %s", status, body)
	}
	for _, event := range view.Events {
		if event.Text == sentinel {
			t.Fatalf("the message must not enter the history before the provider republishes it: %s", body)
		}
	}
	if got := collaborator.sentMessages(); len(got) != 1 || got[0] != sentinel {
		t.Fatalf("provider received %v, want exactly %q", got, sentinel)
	}

	// Now the hub republishes it as history.
	stream.events <- execution.RunEvent{ID: 1, Seq: 1, At: time.Now().UTC(), Kind: "user_message", Text: sentinel}
	after := fixture.awaitLastID(t, 1)

	seen := 0
	for _, event := range after.Events {
		if event.Text == sentinel {
			seen++
			if event.Kind != "user_message" {
				t.Fatalf("kind = %q, want user_message", event.Kind)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("the message appears %d times, want exactly once", seen)
	}
}

// AC-4
func TestApprovalResponseShowsTheProviderOutcome(t *testing.T) {
	collaborator := newFakeCollaborator()
	collaborator.setApprovals([]execution.PendingApproval{pendingApproval()})
	fixture := newRunRoutesServer(t, collaborator)
	fixture.primeRun(t)

	waitFor(t, "the pending approval to be projected", func() bool {
		_, view, _ := fixture.readRun(t, 0)
		return len(view.Approvals) == 1
	})
	_, view, _ := fixture.readRun(t, 0)
	if view.Approvals[0].ID != "appr-1" || len(view.Approvals[0].Options) != 2 {
		t.Fatalf("approval = %#v, want appr-1 with its two options", view.Approvals[0])
	}
	if view.Approvals[0].Options[0].ID != "allow-once" || view.Approvals[0].Options[0].Label != "Allow once" {
		t.Fatalf("options = %#v", view.Approvals[0].Options)
	}

	// The provider resolves it, so the re-read that follows the answer is what
	// reports the outcome.
	collaborator.setApprovals([]execution.PendingApproval{})
	collaborator.setSnapshot(execution.RunSnapshot{RunID: "run-9", State: execution.RunActive})

	status, answered, body := fixture.post(t, "/run/approvals/appr-1", map[string]any{"option_id": "allow-once"})
	if status != http.StatusAccepted {
		t.Fatalf("POST approval = %d: %s", status, body)
	}
	if got := collaborator.answeredApprovals(); len(got) != 1 || got[0] != [2]string{"appr-1", "allow-once"} {
		t.Fatalf("provider received %v, want a single (appr-1, allow-once)", got)
	}
	if len(answered.Approvals) != 0 {
		t.Fatalf("the resolved approval is still pending: %s", body)
	}
	if answered.Run == nil || answered.Run.State != string(execution.RunActive) {
		t.Fatalf("run = %#v, want the state the provider now reports", answered.Run)
	}
}

// AC-5, first half
func TestCancelShowsTheProviderConfirmedState(t *testing.T) {
	collaborator := newFakeCollaborator()
	fixture := newRunRoutesServer(t, collaborator)
	stream := fixture.primeRun(t)

	status, view, body := fixture.post(t, "/run/cancel", nil)
	if status != http.StatusAccepted {
		t.Fatalf("POST cancel = %d: %s", status, body)
	}
	if collaborator.cancelCount() != 1 {
		t.Fatalf("the provider recorded %d cancellations, want 1", collaborator.cancelCount())
	}
	// The runner has not acted yet, so the state must still be the one the
	// provider reports. A viewer that synthesized CLOSED here would be showing a
	// fact nobody stated.
	if view.Run == nil || view.Run.State != string(execution.RunActive) {
		t.Fatalf("run = %#v, want ACTIVE until the provider says otherwise", view.Run)
	}

	// The runner now acts on the cancellation: it closes the session, which ends
	// the stream, and the terminal state is read back from the provider. That
	// order is the criterion — the state becomes CLOSED because the provider
	// says so, never because the cancel was sent.
	closedAt := time.UnixMilli(1755000000000).UTC()
	collaborator.setSnapshot(execution.RunSnapshot{RunID: "run-9", State: execution.RunClosed, ClosedAt: &closedAt})
	stream.end <- nil

	waitFor(t, "the closed state to become visible", func() bool {
		_, current, _ := fixture.readRun(t, 0)
		return current.Run != nil && current.Run.State == string(execution.RunClosed)
	})
	_, closed, _ := fixture.readRun(t, 0)
	if closed.Run.ClosedAt == "" {
		t.Fatal("a closed run must report when it closed")
	}
}

// AC-5, second half
func TestCancelOnATerminalRunIsRefusedAndDoesNotReopenIt(t *testing.T) {
	collaborator := newFakeCollaborator()
	collaborator.setSnapshot(execution.RunSnapshot{RunID: "run-9", State: execution.RunClosed})
	fixture := newRunRoutesServer(t, collaborator)
	fixture.primeRun(t)
	waitFor(t, "the terminal state", func() bool {
		_, view, _ := fixture.readRun(t, 0)
		return view.Run != nil && view.Run.State == string(execution.RunClosed)
	})
	collaborator.refuseAll(&execution.RunCommandError{Reason: execution.RunRefusedNotActive, RunID: "run-9"})

	status, _, body := fixture.post(t, "/run/cancel", nil)
	if status != http.StatusConflict {
		t.Fatalf("POST cancel on a terminal run = %d, want 409: %s", status, body)
	}
	_, view, _ := fixture.readRun(t, 0)
	if view.Run == nil || view.Run.State != string(execution.RunClosed) {
		t.Fatalf("run = %#v, want the terminal state, not a reopened run", view.Run)
	}
}

// AC-6
func TestRefusedCommandsLeaveTheProjectionUnchanged(t *testing.T) {
	collaborator := newFakeCollaborator()
	collaborator.setApprovals([]execution.PendingApproval{pendingApproval()})
	fixture := newRunRoutesServer(t, collaborator)
	stream := fixture.primeRun(t)

	for _, id := range []int64{1, 2, 3} {
		stream.events <- event(id, "e")
	}
	fixture.awaitLastID(t, 3)
	waitFor(t, "the pending approval", func() bool {
		_, view, _ := fixture.readRun(t, 0)
		return len(view.Approvals) == 1
	})
	_, before, _ := fixture.readRun(t, 0)
	baseline := fingerprintOf(before)

	refusals := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", &execution.RunCommandError{Reason: execution.RunRefusedNotFound, RunID: "run-9"}, http.StatusNotFound},
		{"run not active", &execution.RunCommandError{Reason: execution.RunRefusedNotActive, RunID: "run-9"}, http.StatusConflict},
		{"runner offline", &execution.RunCommandError{Reason: execution.RunRefusedRunnerOffline, RunID: "run-9"}, http.StatusConflict},
		{"unauthorized", &execution.RunCommandError{Reason: execution.RunRefusedUnauthorized, RunID: "run-9"}, http.StatusConflict},
	}
	commands := []struct {
		name    string
		path    string
		payload any
	}{
		{"message", "/run/messages", map[string]any{"message": "ciao"}},
		{"approval", "/run/approvals/appr-1", map[string]any{"option_id": "allow-once"}},
		{"cancel", "/run/cancel", nil},
	}
	for _, refusal := range refusals {
		collaborator.refuseAll(refusal.err)
		for _, command := range commands {
			t.Run(refusal.name+"/"+command.name, func(t *testing.T) {
				status, _, body := fixture.post(t, command.path, command.payload)
				if status != refusal.wantStatus {
					t.Fatalf("%s = %d, want %d: %s", command.name, status, refusal.wantStatus, body)
				}
				_, after, _ := fixture.readRun(t, 0)
				if got := fingerprintOf(after); !reflect.DeepEqual(got, baseline) {
					t.Fatalf("a refused command changed the projection:\n got %#v\nwant %#v", got, baseline)
				}
			})
		}
	}
}

func TestRunReadWithoutARunYet(t *testing.T) {
	collaborator := newFakeCollaborator()
	collaborator.runID = ""
	fixture := newRunRoutesServer(t, collaborator)

	status, view, body := fixture.readRun(t, 0)
	if status != http.StatusOK {
		t.Fatalf("GET run = %d, want 200 with no run: %s", status, body)
	}
	if view.Run != nil {
		t.Fatalf("run = %#v, want null", view.Run)
	}
	if view.Events == nil || len(view.Events) != 0 {
		t.Fatalf("events = %v, want an empty array", view.Events)
	}
	// Never null on the wire: a client must not have to test before iterating.
	for _, field := range []string{`"events":[]`, `"approvals":[]`} {
		if !strings.Contains(body, field) {
			t.Fatalf("body %s does not contain %s", body, field)
		}
	}
}

func TestRunRoutesRejectABadCursor(t *testing.T) {
	fixture := newRunRoutesServer(t, newFakeCollaborator())
	for _, cursor := range []string{"abc", "-1"} {
		w := doJSON(t, fixture.srv, http.MethodGet, "/api/execution/"+fixture.executionID+"/run?after_id="+cursor, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("after_id=%s = %d, want 400: %s", cursor, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "after_id") {
			t.Fatalf("the error must name after_id: %s", w.Body.String())
		}
	}
}

func TestRunRoutesOnAProviderThatDoesNotCollaborate(t *testing.T) {
	fixture := newRunRoutesServer(t, nil)
	routes := []struct {
		method  string
		path    string
		payload any
	}{
		{http.MethodGet, "/run", nil},
		{http.MethodPost, "/run/messages", map[string]any{"message": "ciao"}},
		{http.MethodPost, "/run/approvals/appr-1", map[string]any{"option_id": "allow-once"}},
		{http.MethodPost, "/run/cancel", nil},
	}
	for _, route := range routes {
		w := doJSON(t, fixture.srv, route.method, "/api/execution/"+fixture.executionID+route.path, route.payload)
		if w.Code != http.StatusConflict {
			t.Fatalf("%s %s = %d, want 409: %s", route.method, route.path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "interactive run") {
			t.Fatalf("the error must explain that the provider exposes no interactive run: %s", w.Body.String())
		}
	}
}

func TestRunRoutesOnAnUnknownExecution(t *testing.T) {
	fixture := newRunRoutesServer(t, newFakeCollaborator())
	w := doJSON(t, fixture.srv, http.MethodGet, "/api/execution/exec-does-not-exist/run", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET run of an unknown execution = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func errAborted() error { return errors.New("the connection dropped") }
