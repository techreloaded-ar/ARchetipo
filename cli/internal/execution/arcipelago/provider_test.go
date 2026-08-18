package arcipelago

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const testToken = "secret-token"

// hubResponse is one canned answer of the simulated hub.
type hubResponse struct {
	status int
	body   string
}

// hubScript drives the stub. createResponses is consumed in order (the last one
// repeats); the same holds for getResponses.
type hubScript struct {
	createResponses []hubResponse
	getResponses    []hubResponse
	byReference     hubResponse
}

type hubCalls struct {
	mu              sync.Mutex
	creates         int
	gets            int
	byReferences    int
	lastCreateBody  createTaskRequest
	lastCreateRaw   map[string]any
	createBodyError error
}

func (c *hubCalls) snapshot() (creates, gets, byReferences int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creates, c.gets, c.byReferences
}

func pick(responses []hubResponse, index int) hubResponse {
	if len(responses) == 0 {
		return hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}
	}
	if index >= len(responses) {
		return responses[len(responses)-1]
	}
	return responses[index]
}

// newStubHub serves the shape of the real external namespace. The handler never
// calls t.Fatal: it records what it saw so the test can assert after the run.
func newStubHub(t *testing.T, script hubScript) (*httptest.Server, *hubCalls) {
	t.Helper()
	calls := &hubCalls{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			write(w, hubResponse{status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`})
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == pathTasks:
			calls.mu.Lock()
			index := calls.creates
			calls.creates++
			raw := map[string]any{}
			decoded := createTaskRequest{}
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				calls.createBodyError = err
			} else {
				body, _ := json.Marshal(raw)
				if err := json.Unmarshal(body, &decoded); err != nil {
					calls.createBodyError = err
				}
			}
			calls.lastCreateRaw = raw
			calls.lastCreateBody = decoded
			calls.mu.Unlock()
			write(w, pick(script.createResponses, index))
		// by-reference must be routed before :id, exactly as the hub does,
		// otherwise the literal segment would be read as a task identifier.
		case r.Method == http.MethodGet && r.URL.Path == pathByReference:
			calls.mu.Lock()
			calls.byReferences++
			calls.mu.Unlock()
			write(w, script.byReference)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, pathTasks+"/"):
			calls.mu.Lock()
			index := calls.gets
			calls.gets++
			calls.mu.Unlock()
			write(w, pick(script.getResponses, index))
		default:
			write(w, hubResponse{status: http.StatusNotFound, body: `{"error":"not_found"}`})
		}
	}))
	t.Cleanup(server.Close)
	return server, calls
}

func write(w http.ResponseWriter, response hubResponse) {
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(response.body))
}

// stepClock advances by one poll interval every time it is read after the
// deadline has been computed, so polling never depends on real time.
func stepClock(step time.Duration) func() time.Time {
	current := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	first := true
	return func() time.Time {
		if first {
			first = false
			return current
		}
		current = current.Add(step)
		return current
	}
}

func newTestProvider(t *testing.T, server *httptest.Server, options Options) *Provider {
	t.Helper()
	if options.Doer == nil {
		options.Doer = server.Client()
	}
	if options.Getenv == nil {
		options.Getenv = func(string) string { return testToken }
	}
	if options.Sleep == nil {
		options.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }
	}
	return New(options)
}

func testRequest(server *httptest.Server) execution.Request {
	return execution.Request{
		ExecutionID: "req-abc123",
		SpecCode:    "US-001",
		Action:      execution.ActionPlan,
		Capability:  execution.CapabilitySpecPlan,
		ProviderConfig: map[string]any{
			"base_url":              server.URL,
			"workspace_id":          "ws-1",
			"poll_interval_seconds": 1,
			"timeout_seconds":       60,
		},
	}
}

func validReceipt(tasks int) string {
	return fmt.Sprintf(`{\"spec_code\":\"US-001\",\"status\":\"PLANNED\",\"tasks\":%d}`, tasks)
}

func completedBody(resultSummary string) string {
	return `{"task":{"id":"task-remote-1","status":"completed","resultSummary":"` + resultSummary + `"}}`
}

// assertNoSecret is the cross-cutting guarantee: no failure path may echo the
// bearer value.
func assertNoSecret(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error leaked the credential: %v", err)
	}
}

func TestExecuteCreatesTaskAndReturnsExternalID(t *testing.T) {
	server, calls := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
		getResponses:    []hubResponse{{status: http.StatusOK, body: completedBody(validReceipt(9))}},
	})
	req := testRequest(server)
	got, err := newTestProvider(t, server, Options{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalID != "task-remote-1" {
		t.Fatalf("external id = %q", got.ExternalID)
	}
	var payload struct {
		TaskID      string `json:"task_id"`
		WorkspaceID string `json:"workspace_id"`
		Status      string `json:"status"`
		PlanTasks   int    `json:"plan_tasks"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("payload is not valid JSON (%s): %v", got.Payload, err)
	}
	if payload.TaskID != "task-remote-1" || payload.WorkspaceID != "ws-1" || payload.Status != "completed" || payload.PlanTasks != 9 {
		t.Fatalf("payload = %#v", payload)
	}
	calls.mu.Lock()
	defer calls.mu.Unlock()
	if calls.createBodyError != nil {
		t.Fatal(calls.createBodyError)
	}
	if calls.creates != 1 {
		t.Fatalf("creates = %d", calls.creates)
	}
	if calls.lastCreateBody.Source != sourceARchetipo || calls.lastCreateBody.ExternalID != req.ExecutionID || calls.lastCreateBody.WorkspaceID != "ws-1" {
		t.Fatalf("create body = %#v", calls.lastCreateBody)
	}
	if !strings.Contains(calls.lastCreateBody.Title, "US-001") {
		t.Fatalf("title does not name the spec: %q", calls.lastCreateBody.Title)
	}
	if _, ok := calls.lastCreateRaw["metadata"].(map[string]any); !ok {
		t.Fatalf("metadata did not travel as a JSON object: %#v", calls.lastCreateRaw["metadata"])
	}
}

// A 200 means ARcipelago recognized an identical repetition: no second task.
func TestExecuteAcceptsIdempotentTwoHundred(t *testing.T) {
	server, calls := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"running"}}`}},
		getResponses:    []hubResponse{{status: http.StatusOK, body: completedBody(validReceipt(3))}},
	})
	got, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalID != "task-remote-1" {
		t.Fatalf("external id = %q", got.ExternalID)
	}
	if creates, _, _ := calls.snapshot(); creates != 1 {
		t.Fatalf("creates = %d", creates)
	}
}

// buildTask is part of the remote idempotency contract: ARcipelago fingerprints
// title, prompt and metadata on top of the identity triple, and asRecord
// rejects a null metadata with 400.
func TestBuildTaskIsDeterministicAndCarriesObjectMetadata(t *testing.T) {
	req := execution.Request{ExecutionID: "req-abc123", SpecCode: "US-001", Action: execution.ActionPlan, Capability: execution.CapabilitySpecPlan}
	firstTitle, firstPrompt, firstMetadata := buildTask(req)
	secondTitle, secondPrompt, secondMetadata := buildTask(req)
	if firstTitle != secondTitle || firstPrompt != secondPrompt {
		t.Fatal("buildTask is not deterministic")
	}
	if len(firstMetadata) != len(secondMetadata) {
		t.Fatalf("metadata size drifted: %d vs %d", len(firstMetadata), len(secondMetadata))
	}
	for key, want := range firstMetadata {
		if secondMetadata[key] != want {
			t.Fatalf("metadata[%q] drifted: %v vs %v", key, want, secondMetadata[key])
		}
	}
	bare := execution.Request{ExecutionID: "req-1", SpecCode: "US-002"}
	if _, _, metadata := buildTask(bare); metadata == nil {
		t.Fatal("metadata is nil for a minimal request")
	}
	body, err := json.Marshal(createTaskRequest{WorkspaceID: "ws-1", Source: sourceARchetipo, ExternalID: req.ExecutionID, Title: firstTitle, Prompt: firstPrompt, Metadata: firstMetadata})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["metadata"].(map[string]any); !ok {
		t.Fatalf("serialized metadata is not an object: %#v", raw["metadata"])
	}
}

// buildTaskGolden is the exact body ARcipelago fingerprints. Comparing two calls
// made in the same instant would miss a non-determinism of coarse resolution — a
// date stamped into the prompt drifts once a day, not once a call — so the
// contract is pinned to a literal instead. A deliberate change to the prompt is
// expected to update this constant and to be read as what it is: a new
// fingerprint, hence a 409 for any request repeated across the change.
const buildTaskGolden = `{"workspaceId":"ws-1","source":"archetipo","externalId":"req-abc123","title":"ARchetipo plan US-001","prompt":"Work in the runner working directory: it is a checkout of the ARchetipo project with the archetipo CLI and the ARchetipo skills already installed.\nPlan the spec US-001 by invoking the ARchetipo planning skill:\n\n/archetipo-plan US-001\n\nPersist the plan through the configured connector, exactly as the skill prescribes. Do not paste the plan into your final message.\nClose your run with a single JSON receipt line and nothing after it:\n\n{\"spec_code\":\"US-001\",\"status\":\"PLANNED\",\"tasks\":\u003cN\u003e}\n\n\u003cN\u003e is the number of tasks of the plan you actually persisted. Emit the receipt only after the plan is persisted and the spec is PLANNED.","metadata":{"action":"plan","capability":"spec.plan","execution_id":"req-abc123","spec_code":"US-001"}}`

func TestBuildTaskBodyMatchesTheGoldenFingerprint(t *testing.T) {
	req := execution.Request{ExecutionID: "req-abc123", SpecCode: "US-001", Action: execution.ActionPlan, Capability: execution.CapabilitySpecPlan}
	title, prompt, metadata := buildTask(req)
	body, err := json.Marshal(createTaskRequest{WorkspaceID: "ws-1", Source: sourceARchetipo, ExternalID: req.ExecutionID, Title: title, Prompt: prompt, Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != buildTaskGolden {
		t.Fatalf("the request body drifted from the golden fingerprint:\n got %s\nwant %s", body, buildTaskGolden)
	}
}

// The prompt asks for one word and the gate accepts another unless both are the
// same constant, and the canonical spelling belongs to the domain.
func TestReceiptStatusIsBoundToTheCanonicalSpecStatus(t *testing.T) {
	if plannedStatus != string(domain.StatusPlanned) {
		t.Fatalf("plannedStatus = %q, canonical = %q", plannedStatus, domain.StatusPlanned)
	}
	_, prompt, _ := buildTask(execution.Request{ExecutionID: "req-1", SpecCode: "US-001"})
	if !strings.Contains(prompt, `"status":"`+string(domain.StatusPlanned)+`"`) {
		t.Fatalf("the prompt does not ask for the canonical status: %s", prompt)
	}
	server, _ := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
		getResponses:    []hubResponse{{status: http.StatusOK, body: completedBody(fmt.Sprintf(`{\"spec_code\":\"US-001\",\"status\":\"%s\",\"tasks\":2}`, domain.StatusPlanned))}},
	})
	if _, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server)); err != nil {
		t.Fatalf("the gate rejected a receipt carrying the canonical status: %v", err)
	}
}

// A body cut on a byte boundary would emit U+FFFD inside a message meant to be
// read by an operator.
func TestTruncateDoesNotSplitARune(t *testing.T) {
	body := strings.Repeat("è", maxErrorBody)
	got := truncate([]byte(body))
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("an oversized body was not truncated: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncation produced invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("truncation split a rune: %q", got)
	}
	if short := truncate([]byte(" ok ")); short != "ok" {
		t.Fatalf("a short body was altered: %q", short)
	}
}

func TestExecutePollsUntilTerminal(t *testing.T) {
	server, calls := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
		getResponses: []hubResponse{
			{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"running"}}`},
			{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"assigned"}}`},
			{status: http.StatusOK, body: completedBody(validReceipt(2))},
		},
	})
	if _, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server)); err != nil {
		t.Fatal(err)
	}
	if _, gets, _ := calls.snapshot(); gets != 3 {
		t.Fatalf("gets = %d", gets)
	}
}

// Without this branch a remote task that terminates without planning would
// produce a SUCCEEDED execution on a spec still in TODO.
func TestExecuteRejectsCompletedWithoutValidReceipt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary string
	}{
		{"no summary at all", ""},
		{"free text without a JSON line", "done, everything looks fine"},
		{"status is not PLANNED", `{\"spec_code\":\"US-001\",\"status\":\"TODO\",\"tasks\":3}`},
		{"no tasks in the plan", `{\"spec_code\":\"US-001\",\"status\":\"PLANNED\",\"tasks\":0}`},
		{"receipt for another spec", `{\"spec_code\":\"US-999\",\"status\":\"PLANNED\",\"tasks\":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newStubHub(t, hubScript{
				createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
				getResponses:    []hubResponse{{status: http.StatusOK, body: completedBody(tc.summary)}},
			})
			_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
			assertNoSecret(t, err)
			if !strings.Contains(err.Error(), "without having produced a plan") || !strings.Contains(err.Error(), "US-001") {
				t.Fatalf("message does not report the missing plan: %v", err)
			}
			assertNamesRemoteTask(t, err)
		})
	}
}

// assertNamesRemoteTask is the guarantee every failure that follows the creation
// of the remote task must honour: the task is still on the hub, so the record
// must carry its identifier in a structured field and the message must say how
// to find it again.
func assertNamesRemoteTask(t *testing.T, err error) {
	t.Helper()
	var remoteErr *execution.RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("the failure does not carry the remote identifier in a structured field: %v", err)
	}
	if remoteErr.ExternalID != "task-remote-1" {
		t.Fatalf("structured external id = %q", remoteErr.ExternalID)
	}
	for _, want := range []string{"task-remote-1", "by-reference", "ws-1", sourceARchetipo, "req-abc123"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("message misses %q: %v", want, err)
		}
	}
}

// A transient 5xx while polling is the likeliest failure of a long wait, and it
// leaves exactly the same live remote task a timeout leaves.
func TestExecutePollErrorStillNamesRemoteTask(t *testing.T) {
	server, _ := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
		getResponses:    []hubResponse{{status: http.StatusBadGateway, body: `{"error":"upstream"}`}},
	})
	_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
	assertNoSecret(t, err)
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("message does not report the HTTP status: %v", err)
	}
	assertNamesRemoteTask(t, err)
}

func TestExecuteRejectsEmptyTaskID(t *testing.T) {
	server, calls := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"","status":"queued"}}`}},
	})
	_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
	assertNoSecret(t, err)
	if !strings.Contains(err.Error(), "without an identifier") {
		t.Fatalf("message does not report the missing identifier: %v", err)
	}
	if strings.Contains(err.Error(), "404") {
		t.Fatalf("message misdiagnoses the failure as a 404: %v", err)
	}
	if _, gets, _ := calls.snapshot(); gets != 0 {
		t.Fatalf("polling started despite the missing identifier: gets = %d", gets)
	}
}

func TestExecuteFailedAndCancelledRemoteOutcomes(t *testing.T) {
	for _, tc := range []struct {
		status  string
		summary string
	}{
		{statusFailed, "planning aborted: missing tool"},
		{statusCancelled, "cancelled by the operator"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			server, _ := newStubHub(t, hubScript{
				createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
				getResponses:    []hubResponse{{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"` + tc.status + `","resultSummary":"` + tc.summary + `"}}`}},
			})
			_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
			assertNoSecret(t, err)
			for _, want := range []string{tc.status, "task-remote-1", tc.summary} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("message misses %q: %v", want, err)
				}
			}
			assertNamesRemoteTask(t, err)
		})
	}
}

// There is deliberately no 403 case: applicationAuth answers 401 when the
// bearer does not resolve, and workspace authorization is expressed as 404.
func TestExecuteHTTPErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response hubResponse
		contains []string
	}{
		{"unauthorized", hubResponse{status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`}, []string{"401", "ARCIPELAGO_TOKEN"}},
		{"workspace not granted", hubResponse{status: http.StatusNotFound, body: `{"error":"not_found"}`}, []string{"404", "ws-1"}},
		{"rejected by validation", hubResponse{status: http.StatusBadRequest, body: `{"error":"metadata must be an object"}`}, []string{"400", "metadata must be an object"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newStubHub(t, hubScript{createResponses: []hubResponse{tc.response}})
			_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
			assertNoSecret(t, err)
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("message misses %q: %v", want, err)
				}
			}
		})
	}
}

func TestExecuteDiscriminatesConflictCauses(t *testing.T) {
	t.Run("same external id with a different assignment", func(t *testing.T) {
		server, calls := newStubHub(t, hubScript{
			createResponses: []hubResponse{{status: http.StatusConflict, body: `{"error":"external_task_conflict"}`}},
			byReference:     hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-remote-0","status":"running"}}`},
		})
		_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
		assertNoSecret(t, err)
		for _, want := range []string{"external_task_conflict", "--request-id", "task-remote-0"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("message misses %q: %v", want, err)
			}
		}
		if _, _, byReferences := calls.snapshot(); byReferences != 1 {
			t.Fatalf("the external reference route was queried %d times", byReferences)
		}
	})
	t.Run("archived workspace", func(t *testing.T) {
		server, _ := newStubHub(t, hubScript{
			createResponses: []hubResponse{{status: http.StatusConflict, body: `{"error":"workspace_archived"}`}},
		})
		_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
		assertNoSecret(t, err)
		if !strings.Contains(err.Error(), "ws-1") || !strings.Contains(err.Error(), "archived") {
			t.Fatalf("message does not report the archived workspace: %v", err)
		}
		if strings.Contains(err.Error(), "external_task_conflict") {
			t.Fatalf("archived workspace reported as an identity conflict: %v", err)
		}
	})
	t.Run("undecodable conflict body", func(t *testing.T) {
		server, _ := newStubHub(t, hubScript{
			createResponses: []hubResponse{{status: http.StatusConflict, body: `not json at all`}},
		})
		_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), testRequest(server))
		assertNoSecret(t, err)
		if !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "not json at all") {
			t.Fatalf("generic message misses status or body: %v", err)
		}
	})
}

func TestExecuteTimesOutNamingRemoteTask(t *testing.T) {
	server, calls := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
		getResponses:    []hubResponse{{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"running"}}`}},
	})
	provider := newTestProvider(t, server, Options{Now: stepClock(90 * time.Second)})
	_, err := provider.Execute(context.Background(), testRequest(server))
	assertNoSecret(t, err)
	for _, want := range []string{"timed out", `"running"`, "task-remote-1", "by-reference", "ws-1", "archetipo", "req-abc123"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("message misses %q: %v", want, err)
		}
	}
	assertNamesRemoteTask(t, err)
	_, gets, _ := calls.snapshot()
	if gets == 0 || gets > 5 {
		t.Fatalf("polling did not stop in a bounded number of reads: gets = %d", gets)
	}
}

func TestExecuteMissingTokenNamesEnvVariable(t *testing.T) {
	server, calls := newStubHub(t, hubScript{})
	provider := newTestProvider(t, server, Options{Getenv: func(string) string { return "" }})
	_, err := provider.Execute(context.Background(), testRequest(server))
	assertNoSecret(t, err)
	if !strings.Contains(err.Error(), "ARCIPELAGO_TOKEN") {
		t.Fatalf("message does not name the environment variable: %v", err)
	}
	if creates, gets, byReferences := calls.snapshot(); creates != 0 || gets != 0 || byReferences != 0 {
		t.Fatalf("the hub was contacted without a credential: %d/%d/%d", creates, gets, byReferences)
	}
}

func TestExecuteHonoursContextCancellation(t *testing.T) {
	server, _ := newStubHub(t, hubScript{
		createResponses: []hubResponse{{status: http.StatusCreated, body: `{"task":{"id":"task-remote-1","status":"queued"}}`}},
		getResponses:    []hubResponse{{status: http.StatusOK, body: `{"task":{"id":"task-remote-1","status":"running"}}`}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newTestProvider(t, server, Options{}).Execute(ctx, testRequest(server))
	assertNoSecret(t, err)
}

func TestExecuteRejectsInvalidConfigBeforeAnyCall(t *testing.T) {
	server, calls := newStubHub(t, hubScript{})
	req := testRequest(server)
	req.ProviderConfig = map[string]any{"workspace_id": "ws-1"}
	_, err := newTestProvider(t, server, Options{}).Execute(context.Background(), req)
	assertNoSecret(t, err)
	var configErr *execution.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Field != "base_url" {
		t.Fatalf("expected a base_url configuration error, got %v", err)
	}
	if creates, gets, _ := calls.snapshot(); creates != 0 || gets != 0 {
		t.Fatalf("the hub was contacted with an invalid configuration: %d/%d", creates, gets)
	}
}
