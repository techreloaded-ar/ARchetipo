package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// implementTestProvider is runTestProvider with the declared capabilities made
// explicit. It exists next to the planning double rather than replacing it
// because the two declarations are the whole point of AC-2: a provider that can
// plan is not thereby a provider that can implement, and the tests need both
// shapes over the same Execute machinery.
//
// Everything else stays production: connector, record store, Template and
// effect confirmation.
type implementTestProvider struct {
	*runTestProvider
	capabilities []execution.Capability
}

func (p *implementTestProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return p.capabilities, nil
}

// blockedImplementProvider declares spec.implement and stays inside Execute
// until the test releases it, which is how the IN PROGRESS window becomes
// observable without a single sleep.
func blockedImplementProvider(id string) *implementTestProvider {
	return &implementTestProvider{
		runTestProvider: blockedProvider(id),
		capabilities:    []execution.Capability{execution.CapabilitySpecImplement},
	}
}

// releasedImplementProvider declares spec.implement and is already free to run.
func releasedImplementProvider(id string, execute func(context.Context, execution.Request) (execution.Result, error)) *implementTestProvider {
	return &implementTestProvider{
		runTestProvider: releasedProvider(id, execute),
		capabilities:    []execution.Capability{execution.CapabilitySpecImplement},
	}
}

// persistImplementablePlan brings US-901 to the real starting state of an
// implementation: a persisted plan with more than one open task. It writes
// through the production connector, so the plan the route later refuses to run
// without is the very one the connector reads back.
func persistImplementablePlan(t *testing.T, conn connector.Connector, code string) {
	t.Helper()
	plan := domain.PlanInput{
		PlanBody: "# Piano di " + code + "\n\nDue passi.\n",
		Tasks: []domain.Task{
			{ID: "TASK-01", Title: "Scrivere il codice", Type: domain.TaskImpl, Status: domain.StatusTodo, Body: "## Objective\nCodice.\n"},
			{ID: "TASK-02", Title: "Scrivere i test", Type: domain.TaskTest, Status: domain.StatusTodo, Body: "## Objective\nTest.\n"},
		},
	}
	if _, err := conn.SavePlan(context.Background(), code, plan); err != nil {
		t.Fatal(err)
	}
}

// moveSpecTo puts the spec in an arbitrary workflow status, which is what the
// per-status half of AC-1 needs: the Template answer must depend on the status
// alone, not on how the spec reached it.
func moveSpecTo(t *testing.T, conn connector.Connector, code string, status domain.Status) {
	t.Helper()
	if _, err := conn.TransitionStatus(context.Background(), code, status); err != nil {
		t.Fatal(err)
	}
}

// actionChip returns the action the detail exposes under id. A chip the process
// does not offer in the current status is simply absent, which is itself an
// answer the tests assert on.
func actionChip(detail specDetailResponse, id string) (bool, bool, string) {
	for _, action := range detail.Actions {
		if action.ID == id {
			return true, action.Runnable, action.UnavailableReason
		}
	}
	return false, false, ""
}

// implementingExecute is the provider behaviour of an agent that really carried
// the plan out: it completes every task through the connector, moves the spec to
// REVIEW, and only then hands back its receipt with the summary of code and
// tests.
func implementingExecute(conn connector.Connector) func(context.Context, execution.Request) (execution.Result, error) {
	return func(ctx context.Context, request execution.Request) (execution.Result, error) {
		tasks, err := conn.ReadSpecTasks(ctx, request.SpecCode)
		if err != nil {
			return execution.Result{}, err
		}
		for _, task := range tasks {
			if _, err := conn.CompleteTask(ctx, request.SpecCode, task.ID); err != nil {
				return execution.Result{}, err
			}
		}
		if _, err := conn.TransitionStatus(ctx, request.SpecCode, domain.StatusReview); err != nil {
			return execution.Result{}, err
		}
		payload := fmt.Sprintf(
			`{"spec_code":%q,"status":"REVIEW","tasks_done":%d,"tests":"go test ./... — 42 passed, 0 failed","result_summary":"implementati %d task, codice e test verdi"}`,
			request.SpecCode, len(tasks), len(tasks),
		)
		return execution.Result{Payload: json.RawMessage(payload), ExternalID: "task-implement-1"}, nil
	}
}

// AC-1: the implement action exists only where the Template admits it. The
// status alone decides, on the start route and on the chip the browser renders,
// and everything else about the workspace is held constant — plan persisted,
// provider capable, nothing running.
func TestRunSpecActionImplementIsAdmittedOnlyByTheTemplateStatuses(t *testing.T) {
	cases := []struct {
		status   domain.Status
		admitted bool
	}{
		{domain.StatusTodo, false},
		{domain.StatusPlanned, true},
		{domain.StatusInProgress, true},
		{domain.StatusReview, false},
		{domain.StatusDone, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			provider := blockedImplementProvider("fake")
			srv, _, conn := newRunServer(t, provider, true)
			persistImplementablePlan(t, conn, "US-901")
			moveSpecTo(t, conn, "US-901", tc.status)

			// The chip is read before the start, so no running execution can be
			// mistaken for the status answer.
			found, runnable, reason := actionChip(runSpecDetail(t, srv, "US-901"), "implement")
			if tc.admitted && (!found || !runnable || reason != "") {
				t.Fatalf("implement is not offered on %s: found=%v runnable=%v reason=%q", tc.status, found, runnable, reason)
			}
			if !tc.admitted && found && runnable {
				t.Fatalf("implement is offered on %s: reason=%q", tc.status, reason)
			}

			status, body := startAction(t, srv, "US-901", "implement")
			if !tc.admitted {
				if status != http.StatusConflict {
					t.Fatalf("implement on %s: %d %v", tc.status, status, body)
				}
				message, _ := body["error"].(string)
				for _, want := range []string{"implement", "US-901", string(tc.status)} {
					if !strings.Contains(message, want) {
						t.Fatalf("the refusal does not name %q: %q", want, message)
					}
				}
				return
			}
			if status != http.StatusCreated {
				t.Fatalf("implement on %s: %d %v", tc.status, status, body)
			}
			id, _ := body["id"].(string)
			close(provider.release)
			awaitTerminal(t, srv, id)
		})
	}
}

// AC-2: a provider that does not declare spec.implement is refused before
// anything exists. No record under .archetipo/executions/, no transition on the
// spec, and the same sentence next to the disabled chip.
func TestRunSpecActionImplementRefusesAProviderWithoutTheCapability(t *testing.T) {
	// The default double declares spec.plan only, which is exactly the
	// incompatible provider this criterion is about.
	srv, cfg, conn := newRunServer(t, releasedProvider("fake", nil), true)
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	status, body := startAction(t, srv, "US-901", "implement")
	if status != http.StatusConflict {
		t.Fatalf("incompatible provider: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, string(execution.CapabilitySpecImplement)) {
		t.Fatalf("the refusal does not name the missing capability: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
		t.Fatalf("a refused implementation created %d records", got)
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusPlanned {
		t.Fatalf("a refused implementation moved the spec to %s", detail.Spec.Status)
	}
	if detail.Execution != nil {
		t.Fatalf("a refused implementation left an execution on the spec: %#v", detail.Execution)
	}
	found, runnable, reason := actionChip(detail, "implement")
	if !found || runnable || !strings.Contains(reason, string(execution.CapabilitySpecImplement)) {
		t.Fatalf("the chip does not explain the missing capability: found=%v runnable=%v reason=%q", found, runnable, reason)
	}
}

// AC-3: an accepted start moves the spec to IN PROGRESS and the record names
// what is running — provider, spec and the status the run started from — while
// the provider is still working.
func TestRunSpecActionImplementMovesTheSpecToInProgressAndNamesTheRun(t *testing.T) {
	provider := blockedImplementProvider("fake")
	srv, _, conn := newRunServer(t, provider, true)
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	status, started := startAction(t, srv, "US-901", "implement")
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	if started["status"] != string(execution.StatusRunning) ||
		started["spec_code"] != "US-901" ||
		started["provider_id"] != "fake" ||
		started["action"] != string(execution.ActionImplement) ||
		started["spec_status_before"] != string(domain.StatusPlanned) {
		t.Fatalf("the started record does not identify the run: %v", started)
	}
	id, _ := started["id"].(string)
	<-provider.entered

	// The transition is not deferred to the agent: it is already visible while
	// the execution is still open.
	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusInProgress {
		t.Fatalf("an accepted implementation left the spec %s", detail.Spec.Status)
	}
	if detail.Execution == nil || detail.Execution.ID != id || detail.Execution.Status != execution.StatusRunning {
		t.Fatalf("the running execution is not readable on the spec: %#v", detail.Execution)
	}

	close(provider.release)
	awaitTerminal(t, srv, id)
}

// AC-3, the other precondition: an implementation carries out a plan, so a
// PLANNED spec with no persisted plan is refused before any effect, and the chip
// says the same thing.
func TestRunSpecActionImplementRefusesASpecWithoutAPersistedPlan(t *testing.T) {
	srv, cfg, conn := newRunServer(t, blockedImplementProvider("fake"), true)
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	status, body := startAction(t, srv, "US-901", "implement")
	if status != http.StatusConflict {
		t.Fatalf("implement without a plan: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, "plan") || !strings.Contains(message, "US-901") {
		t.Fatalf("the refusal does not name the missing plan: %q", message)
	}
	if got := recordFileCount(t, cfg.ProjectRoot, "US-901"); got != 0 {
		t.Fatalf("a refused implementation created %d records", got)
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusPlanned {
		t.Fatalf("a refused implementation moved the spec to %s", detail.Spec.Status)
	}
	found, runnable, reason := actionChip(detail, "implement")
	if !found || runnable || !strings.Contains(reason, "plan") {
		t.Fatalf("the chip does not explain the missing plan: found=%v runnable=%v reason=%q", found, runnable, reason)
	}
}

// AC-4: success is proved by the spec reaching REVIEW with its plan carried
// out, and the record keeps a verifiable summary of the code and tests the run
// produced.
func TestRunSpecActionImplementSuccessIsProvedByTheReviewedSpec(t *testing.T) {
	var conn connector.Connector
	provider := releasedImplementProvider("fake", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return implementingExecute(conn)(ctx, request)
	})
	srv, _, conn := newRunServer(t, provider, true)
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	status, started := startAction(t, srv, "US-901", "implement")
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusSucceeded || record.Result == nil || record.CompletedAt == nil {
		t.Fatalf("terminal record: %#v", record)
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Result.Payload, &payload); err != nil {
		t.Fatalf("undecodable receipt: %s", record.Result.Payload)
	}
	for _, key := range []string{"tasks_done", "tests", "result_summary"} {
		if value, ok := payload[key]; !ok || value == nil || value == "" {
			t.Fatalf("the receipt does not expose %q: %v", key, payload)
		}
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusReview {
		t.Fatalf("a confirmed implementation left the spec %s", detail.Spec.Status)
	}
	if len(detail.Tasks) == 0 {
		t.Fatal("the plan disappeared from the implemented spec")
	}
	for _, task := range detail.Tasks {
		if task.Status != domain.StatusDone {
			t.Fatalf("task %s is %s after a confirmed implementation", task.ID, task.Status)
		}
	}
	if detail.Execution == nil || detail.Execution.ID != id || detail.Execution.Status != execution.StatusSucceeded {
		t.Fatalf("the execution is not readable on the implemented spec: %#v", detail.Execution)
	}
}

// AC-5a: a dispatch that fails keeps a readable reason on the record and leaves
// the spec out of REVIEW.
func TestRunSpecActionImplementFailureKeepsTheSpecOutOfReview(t *testing.T) {
	provider := releasedImplementProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{}, errRemoteRefused
	})
	srv, _, conn := newRunServer(t, provider, true)
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	_, started := startAction(t, srv, "US-901", "implement")
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
		t.Fatalf("failed record: %#v", record)
	}
	// The reason stays interrogable on the polling route, not only in the
	// terminal value the dispatch happened to hold.
	code, polled := readExecution(t, srv, id)
	if code != http.StatusOK || polled.Error == nil || polled.Error.Message != record.Error.Message {
		t.Fatalf("the failure is not interrogable: %d %#v", code, polled)
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status == domain.StatusReview {
		t.Fatal("a failed implementation moved the spec to REVIEW")
	}
	for _, task := range detail.Tasks {
		if task.Status == domain.StatusDone {
			t.Fatalf("a failed implementation completed task %s", task.ID)
		}
	}
}

// AC-5b: a receipt that declares an implementation nobody performed is not
// believed. The very first terminal state a client can read is already the
// demoted one, so there is no SUCCEEDED window a poll could settle on.
func TestRunSpecActionImplementDemotesASuccessTheConnectorDenies(t *testing.T) {
	provider := releasedImplementProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{
			Payload:    json.RawMessage(`{"spec_code":"US-901","status":"REVIEW","tasks_done":2,"tests":"tutto verde","result_summary":"fatto"}`),
			ExternalID: "task-implement-9",
		}, nil
	})
	srv, _, conn := newRunServer(t, provider, true)
	persistImplementablePlan(t, conn, "US-901")
	moveSpecTo(t, conn, "US-901", domain.StatusPlanned)

	_, started := startAction(t, srv, "US-901", "implement")
	id, _ := started["id"].(string)
	record := awaitTerminal(t, srv, id)
	if record.Status != execution.StatusFailed || record.Error == nil || record.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("an unconfirmed implementation was believed: %#v", record)
	}
	if record.Result != nil || record.Error.ExternalID != "task-implement-9" {
		t.Fatalf("the demoted record lost the remote identifier: %#v", record)
	}

	detail := runSpecDetail(t, srv, "US-901")
	if detail.Spec.Status == domain.StatusReview {
		t.Fatal("an unconfirmed implementation moved the spec to REVIEW")
	}
	if detail.Execution == nil || detail.Execution.Status != execution.StatusFailed {
		t.Fatalf("the spec detail disagrees with the record: %#v", detail.Execution)
	}
}

// AC-5b, at the only instant where believing the provider would be visible: the
// implementation receipt is answered, the confirmation is held open, and the
// route the browser polls must still report RUNNING rather than a success that
// is about to be withdrawn.
func TestRunSpecActionImplementNeverPublishesAnUnverifiedSuccess(t *testing.T) {
	gate := &gatedReadConnector{entered: make(chan struct{}, 1), release: make(chan struct{})}
	provider := blockedImplementProvider("fake")
	provider.execute = func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{
			Payload:    json.RawMessage(`{"spec_code":"US-901","status":"REVIEW","tasks_done":2}`),
			ExternalID: "task-implement-9",
		}, nil
	}
	srv, _, served := newRunServerWithConnector(t, provider, true, func(conn connector.Connector) connector.Connector {
		gate.Connector = conn
		return gate
	})
	persistImplementablePlan(t, served, "US-901")
	moveSpecTo(t, served, "US-901", domain.StatusPlanned)

	_, started := startAction(t, srv, "US-901", "implement")
	id, _ := started["id"].(string)

	// Armed only now: the POST itself reads the spec, and blocking that read
	// would stall the request instead of the confirmation.
	gate.armed.Store(true)
	close(provider.release)
	select {
	case <-gate.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the confirmation never re-read the spec")
	}

	status, record := readExecution(t, srv, id)
	if status != http.StatusOK {
		t.Fatalf("polling the execution failed with %d", status)
	}
	if record.Status != execution.StatusRunning || record.CompletedAt != nil || record.Result != nil {
		t.Fatalf("an unverified implementation was published as %s: %#v", record.Status, record)
	}

	gate.armed.Store(false)
	close(gate.release)

	final := awaitTerminal(t, srv, id)
	if final.Status != execution.StatusFailed || final.Error == nil || final.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("the verified outcome is not the demoted one: %#v", final)
	}
	if detail := runSpecDetail(t, srv, "US-901"); detail.Spec.Status == domain.StatusReview {
		t.Fatal("an unconfirmed implementation moved the spec to REVIEW")
	}
}
