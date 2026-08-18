package arcipelago

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const testRunID = "run-9"

// recordedRequest is what the stub saw, so an assertion can be made on the
// request the provider really built instead of on its own expectation of it.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// runStub serves the shape of the hub's run namespace. Every route answers a
// response the test installs, and the stream writes exactly the frames the test
// hands it: nothing here advances on a timer, so no assertion depends on
// wall-clock timing.
type runStub struct {
	mu sync.Mutex

	task      hubResponse
	byRef     hubResponse
	run       hubResponse
	approvals hubResponse
	messages  hubResponse
	respond   hubResponse
	cancel    hubResponse

	// stream writes the SSE body. It receives the request context so a test can
	// keep the connection open until it decides otherwise.
	stream func(ctx context.Context, w io.Writer, flush func())

	cursors  []string
	requests []recordedRequest

	server *httptest.Server
}

func newRunStub(t *testing.T) *runStub {
	t.Helper()
	stub := &runStub{
		task:      hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-1","status":"running","runId":"` + testRunID + `"}}`},
		byRef:     hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-1","status":"running","runId":"` + testRunID + `"}}`},
		run:       hubResponse{status: http.StatusOK, body: `{"run":{"id":"` + testRunID + `","state":"active"}}`},
		approvals: hubResponse{status: http.StatusOK, body: `{"approvals":[]}`},
		messages:  hubResponse{status: http.StatusAccepted, body: `{"ok":true}`},
		respond:   hubResponse{status: http.StatusAccepted, body: `{"ok":true}`},
		cancel:    hubResponse{status: http.StatusAccepted, body: `{"ok":true}`},
	}
	stub.server = httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *runStub) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		write(w, hubResponse{status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`})
		return
	}
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, recordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   string(body),
	})
	s.mu.Unlock()

	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == pathByReference:
		write(w, s.pick(func() hubResponse { return s.byRef }))
	case r.Method == http.MethodGet && strings.HasPrefix(path, pathTasks+"/"):
		write(w, s.pick(func() hubResponse { return s.task }))
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/events"):
		s.serveStream(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/approvals"):
		write(w, s.pick(func() hubResponse { return s.approvals }))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/messages"):
		write(w, s.pick(func() hubResponse { return s.messages }))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/respond"):
		write(w, s.pick(func() hubResponse { return s.respond }))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/cancel"):
		write(w, s.pick(func() hubResponse { return s.cancel }))
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/api/external/runs/"):
		write(w, s.pick(func() hubResponse { return s.run }))
	default:
		write(w, hubResponse{status: http.StatusNotFound, body: `{"error":"not_found"}`})
	}
}

func (s *runStub) pick(read func() hubResponse) hubResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return read()
}

func (s *runStub) serveStream(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("afterId")
	if cursor == "" {
		cursor = r.Header.Get("Last-Event-ID")
	}
	s.mu.Lock()
	s.cursors = append(s.cursors, cursor)
	stream := s.stream
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	if stream != nil {
		stream(r.Context(), w, flush)
	}
}

func (s *runStub) setStream(fn func(ctx context.Context, w io.Writer, flush func())) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stream = fn
}

// writeFrames returns a stream that writes the frames and closes.
func writeFrames(frames ...string) func(context.Context, io.Writer, func()) {
	return func(_ context.Context, w io.Writer, flush func()) {
		for _, frame := range frames {
			_, _ = io.WriteString(w, frame)
			flush()
		}
	}
}

func (s *runStub) recorded() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *runStub) subscriptions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.cursors))
	copy(out, s.cursors)
	return out
}

func (s *runStub) findRequest(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	for _, request := range s.recorded() {
		if request.Method == method && request.Path == path {
			return request
		}
	}
	t.Fatalf("no %s %s among %#v", method, path, s.recorded())
	return recordedRequest{}
}

func runConfig(stub *runStub) map[string]any {
	return map[string]any{
		"base_url":              stub.server.URL,
		"workspace_id":          "ws-1",
		"poll_interval_seconds": 1,
		"timeout_seconds":       60,
	}
}

func runRequest(stub *runStub) execution.RunRequest {
	return execution.RunRequest{RunID: testRunID, ProviderConfig: runConfig(stub)}
}

func sseFrame(name, data string) string {
	return "event: " + name + "\ndata: " + data + "\n\n"
}

func runEventFrameText(id int64, seq int, event string) string {
	return "id: " + fmt.Sprint(id) + "\n" +
		sseFrame(sseEventRun, fmt.Sprintf(`{"id":%d,"runId":%q,"seq":%d,"ts":1755000000000,"event":%s}`, id, testRunID, seq, event))
}

func textDelta(delta string) string {
	return fmt.Sprintf(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":%q}}`, delta)
}

func collectStream(t *testing.T, provider *Provider, stub *runStub, afterID int64) ([]execution.RunEvent, error) {
	t.Helper()
	var events []execution.RunEvent
	err := provider.StreamRunEvents(context.Background(), runRequest(stub), afterID, func(event execution.RunEvent) error {
		events = append(events, event)
		return nil
	})
	return events, err
}

func TestResolveRunReadsTheRunIdOfTheRemoteTask(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})
	record := execution.Execution{ID: "exec-1", Status: execution.StatusSucceeded, Result: &execution.Result{ExternalID: "task-1"}}

	runID, err := provider.ResolveRun(context.Background(), record, runConfig(stub))
	if err != nil {
		t.Fatal(err)
	}
	if runID != testRunID {
		t.Fatalf("run id = %q, want %q", runID, testRunID)
	}
	request := stub.findRequest(t, http.MethodGet, pathTasks+"/task-1")
	if request.Path != pathTasks+"/task-1" {
		t.Fatalf("path = %q", request.Path)
	}
	// The credential must have travelled: a stub that answered without it would
	// prove nothing about the real call.
	if len(stub.recorded()) != 1 {
		t.Fatalf("expected exactly one call, got %#v", stub.recorded())
	}
}

func TestResolveRunSendsTheCredential(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{Getenv: func(string) string { return "wrong-token" }})
	record := execution.Execution{ID: "exec-1", Result: &execution.Result{ExternalID: "task-1"}}

	_, err := provider.ResolveRun(context.Background(), record, runConfig(stub))
	reason, ok := execution.RefusalOf(err)
	if !ok || reason != execution.RunRefusedUnauthorized {
		t.Fatalf("got %q, %v (%v); want unauthorized", reason, ok, err)
	}
	assertNoSecret(t, err)
}

func TestResolveRunAcceptsATaskWithoutARunYet(t *testing.T) {
	stub := newRunStub(t)
	stub.task = hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-1","status":"running"}}`}
	provider := newTestProvider(t, stub.server, Options{})
	record := execution.Execution{ID: "exec-1", Result: &execution.Result{ExternalID: "task-1"}}

	runID, err := provider.ResolveRun(context.Background(), record, runConfig(stub))
	if err != nil {
		t.Fatalf("a task without a run must not be an error: %v", err)
	}
	if runID != "" {
		t.Fatalf("run id = %q, want empty", runID)
	}
}

func TestResolveRunFallsBackToTheExternalReference(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})
	record := execution.Execution{ID: "exec-1", Status: execution.StatusRunning}

	runID, err := provider.ResolveRun(context.Background(), record, runConfig(stub))
	if err != nil {
		t.Fatal(err)
	}
	if runID != testRunID {
		t.Fatalf("run id = %q, want %q", runID, testRunID)
	}
	request := stub.findRequest(t, http.MethodGet, pathByReference)
	for _, want := range []string{"workspaceId=ws-1", "source=" + sourceARchetipo, "externalId=exec-1"} {
		if !strings.Contains(request.Query, want) {
			t.Fatalf("query %q does not contain %q", request.Query, want)
		}
	}
}

func TestResolveRunClassifiesRefusals(t *testing.T) {
	cases := []struct {
		status  int
		want    execution.RunRefusalReason
		refusal bool
	}{
		{http.StatusUnauthorized, execution.RunRefusedUnauthorized, true},
		{http.StatusNotFound, execution.RunRefusedNotFound, true},
		{http.StatusConflict, execution.RunRefusedNotActive, true},
		{http.StatusInternalServerError, "", false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			stub := newRunStub(t)
			stub.task = hubResponse{status: tc.status, body: `{"error":"nope"}`}
			provider := newTestProvider(t, stub.server, Options{})
			record := execution.Execution{ID: "exec-1", Result: &execution.Result{ExternalID: "task-1"}}

			_, err := provider.ResolveRun(context.Background(), record, runConfig(stub))
			if err == nil {
				t.Fatal("expected an error")
			}
			reason, ok := execution.RefusalOf(err)
			if ok != tc.refusal || reason != tc.want {
				t.Fatalf("got %q, %v; want %q, %v", reason, ok, tc.want, tc.refusal)
			}
			assertNoSecret(t, err)
		})
	}
}

func TestStreamRunEventsDeliversHistoryInOrder(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(writeFrames(
		runEventFrameText(1, 1, textDelta("one")),
		runEventFrameText(2, 1, textDelta("two")),
		runEventFrameText(3, 2, textDelta("three")),
		sseFrame(sseEventEnd, `{"reason":"run_closed"}`),
	))
	provider := newTestProvider(t, stub.server, Options{})

	events, err := collectStream(t, provider, stub, 0)
	if err != nil {
		t.Fatalf("a run that closes normally must end the stream with nil: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %#v", len(events), events)
	}
	for i, want := range []int64{1, 2, 3} {
		if events[i].ID != want {
			t.Fatalf("event %d has id %d, want %d", i, events[i].ID, want)
		}
	}
	if events[0].At.IsZero() || events[0].At.Location() != time.UTC {
		t.Fatalf("timestamp = %v, want a UTC instant", events[0].At)
	}
	if len(events[0].Raw) == 0 {
		t.Fatal("the raw agent event must survive the translation")
	}
}

func TestStreamRunEventsSendsTheCursor(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(writeFrames(sseFrame(sseEventEnd, `{"reason":"run_closed"}`)))
	provider := newTestProvider(t, stub.server, Options{})

	if _, err := collectStream(t, provider, stub, 7); err != nil {
		t.Fatal(err)
	}
	request := stub.findRequest(t, http.MethodGet, "/api/external/runs/"+testRunID+"/events")
	if !strings.Contains(request.Query, "afterId=7") {
		t.Fatalf("query = %q, want afterId=7", request.Query)
	}
	// The oracle is the cursor the subscription really carried, not the one the
	// caller believes it passed.
	if got := stub.subscriptions(); len(got) != 1 || got[0] != "7" {
		t.Fatalf("subscriptions = %#v, want exactly one at cursor 7", got)
	}

	stub.mu.Lock()
	stub.requests = nil
	stub.mu.Unlock()
	if _, err := collectStream(t, provider, stub, 0); err != nil {
		t.Fatal(err)
	}
	request = stub.findRequest(t, http.MethodGet, "/api/external/runs/"+testRunID+"/events")
	if strings.Contains(request.Query, "afterId") {
		t.Fatalf("query = %q, want no cursor at all", request.Query)
	}
	if got := stub.subscriptions(); len(got) != 2 || got[1] != "" {
		t.Fatalf("subscriptions = %#v, want the second one without a cursor", got)
	}
}

func TestStreamRunEventsIgnoresKeepAliveAndEmptyFrames(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(writeFrames(
		": keep-alive\n\n",
		runEventFrameText(1, 1, textDelta("one")),
		"event: run_event\n\n",
		": keep-alive\n",
		runEventFrameText(2, 1, textDelta("two")),
		sseFrame(sseEventEnd, `{"reason":"run_closed"}`),
	))
	provider := newTestProvider(t, stub.server, Options{})

	events, err := collectStream(t, provider, stub, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %#v", len(events), events)
	}
}

func TestStreamRunEventsTranslatesEventKinds(t *testing.T) {
	cases := []struct {
		name     string
		event    string
		wantKind string
		wantText string
		wantTool string
	}{
		{"user_message", `{"type":"user_message","text":"ciao"}`, "user_message", "ciao", ""},
		{"text_delta", textDelta("hello"), "text", "hello", ""},
		{"thinking_delta", `{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"hmm"}}`, "thinking", "hmm", ""},
		{"tool_start", `{"type":"tool_execution_start","toolName":"Bash"}`, "tool_start", "", "Bash"},
		{"tool_end", `{"type":"tool_execution_end","toolName":"Bash","isError":false}`, "tool_end", "", "Bash"},
		{"tool_error", `{"type":"tool_execution_end","toolName":"Bash","isError":true}`, "tool_error", "", "Bash"},
		{"turn_end_error", `{"type":"message_end","message":{"stopReason":"error","errorMessage":"boom"}}`, "turn_end", "boom", ""},
		{"turn_end_ok", `{"type":"message_end","message":{"stopReason":"end_turn"}}`, "turn_end", "", ""},
		// The escape hatch: a type this build has never seen must still produce an
		// event, or the history would silently lose rows the day the hub adds one.
		{"unknown", `{"type":"brand_new_thing"}`, "brand_new_thing", "", ""},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newRunStub(t)
			stub.setStream(writeFrames(
				runEventFrameText(int64(i+1), 1, tc.event),
				sseFrame(sseEventEnd, `{"reason":"run_closed"}`),
			))
			provider := newTestProvider(t, stub.server, Options{})

			events, err := collectStream(t, provider, stub, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1: %#v", len(events), events)
			}
			got := events[0]
			if got.Kind != tc.wantKind || got.Text != tc.wantText || got.Tool != tc.wantTool {
				t.Fatalf("got kind=%q text=%q tool=%q; want kind=%q text=%q tool=%q",
					got.Kind, got.Text, got.Tool, tc.wantKind, tc.wantText, tc.wantTool)
			}
			// The cursor is the frame id, never the seq: a seq is reused by an
			// operator message and would make the cursor lose or repeat a row.
			if got.ID != int64(i+1) {
				t.Fatalf("id = %d, want %d", got.ID, i+1)
			}
		})
	}
}

func TestStreamRunEventsEndsUnauthorized(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(writeFrames(sseFrame(sseEventEnd, `{"reason":"unauthorized"}`)))
	provider := newTestProvider(t, stub.server, Options{})

	_, err := collectStream(t, provider, stub, 0)
	reason, ok := execution.RefusalOf(err)
	if !ok || reason != execution.RunRefusedUnauthorized {
		t.Fatalf("got %q, %v (%v); want unauthorized", reason, ok, err)
	}
}

func TestStreamRunEventsStopsWhenTheSinkFails(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(writeFrames(
		runEventFrameText(1, 1, textDelta("one")),
		runEventFrameText(2, 1, textDelta("two")),
		runEventFrameText(3, 1, textDelta("three")),
		sseFrame(sseEventEnd, `{"reason":"run_closed"}`),
	))
	provider := newTestProvider(t, stub.server, Options{})
	sentinel := errors.New("the consumer is done")

	var delivered []int64
	err := provider.StreamRunEvents(context.Background(), runRequest(stub), 0, func(event execution.RunEvent) error {
		delivered = append(delivered, event.ID)
		if event.ID == 2 {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("the sink error must reach the caller unchanged, got %v", err)
	}
	if len(delivered) != 2 || delivered[1] != 2 {
		t.Fatalf("delivered %v, want the stream to stop right after event 2", delivered)
	}
}

func TestStreamRunEventsHonoursContextCancellation(t *testing.T) {
	t.Run("already cancelled", func(t *testing.T) {
		stub := newRunStub(t)
		stub.setStream(func(ctx context.Context, _ io.Writer, _ func()) { <-ctx.Done() })
		provider := newTestProvider(t, stub.server, Options{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := provider.StreamRunEvents(ctx, runRequest(stub), 0, func(execution.RunEvent) error { return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	})

	t.Run("cancelled while streaming", func(t *testing.T) {
		stub := newRunStub(t)
		stub.setStream(func(ctx context.Context, w io.Writer, flush func()) {
			_, _ = io.WriteString(w, runEventFrameText(1, 1, textDelta("one")))
			flush()
			<-ctx.Done()
		})
		provider := newTestProvider(t, stub.server, Options{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- provider.StreamRunEvents(ctx, runRequest(stub), 0, func(execution.RunEvent) error {
				cancel()
				return nil
			})
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("StreamRunEvents did not return after the context was cancelled")
		}
	})
}

func TestReadRunTranslatesState(t *testing.T) {
	cases := []struct {
		remote    string
		want      execution.RunState
		wantNote  bool
		closedAt  int64
		wantError string
	}{
		{remote: "active", want: execution.RunActive},
		{remote: "closed", want: execution.RunClosed, closedAt: 1755000000000},
		{remote: "crashed", want: execution.RunCrashed},
		// An unknown state must never read as active, or the UI would offer
		// commands on a run that may already be over.
		{remote: "hibernating", want: execution.RunCrashed, wantNote: true},
	}
	for _, tc := range cases {
		t.Run(tc.remote, func(t *testing.T) {
			stub := newRunStub(t)
			stub.run = hubResponse{status: http.StatusOK, body: fmt.Sprintf(
				`{"run":{"id":%q,"state":%q,"closedAt":%d}}`, testRunID, tc.remote, tc.closedAt)}
			provider := newTestProvider(t, stub.server, Options{})

			got, err := provider.ReadRun(context.Background(), runRequest(stub))
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.want {
				t.Fatalf("state = %q, want %q", got.State, tc.want)
			}
			if got.RunID != testRunID {
				t.Fatalf("run id = %q", got.RunID)
			}
			if tc.wantNote && !strings.Contains(got.Error, tc.remote) {
				t.Fatalf("error = %q, want it to preserve the original state %q", got.Error, tc.remote)
			}
			if !tc.wantNote && got.Error != "" {
				t.Fatalf("error = %q, want empty for a known state", got.Error)
			}
			if tc.closedAt > 0 && got.ClosedAt == nil {
				t.Fatal("expected the closing instant to be reported")
			}
			if tc.closedAt == 0 && got.ClosedAt != nil {
				t.Fatalf("closed at = %v, want nil for a run that is not closed", got.ClosedAt)
			}
		})
	}
}

func TestReadRunApprovalsTranslatesOptions(t *testing.T) {
	stub := newRunStub(t)
	stub.approvals = hubResponse{status: http.StatusOK, body: `{"approvals":[{
		"id":"appr-1","runId":"run-9","runnerId":"runner-1","createdAt":1755000000000,
		"request":{"toolName":"Bash","title":"Run a command","args":{"command":"ls"},
		"options":[{"optionId":"allow-once","name":"Allow once","kind":"allow"},
		           {"optionId":"deny","name":"Deny","kind":"deny"}]}}]}`}
	provider := newTestProvider(t, stub.server, Options{})

	approvals, err := provider.ReadRunApprovals(context.Background(), runRequest(stub))
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) != 1 {
		t.Fatalf("got %d approvals, want 1", len(approvals))
	}
	got := approvals[0]
	if got.ID != "appr-1" || got.ToolName != "Bash" || got.Title != "Run a command" {
		t.Fatalf("approval = %#v", got)
	}
	if !strings.Contains(string(got.Args), "ls") {
		t.Fatalf("args = %q, want the tool arguments preserved", got.Args)
	}
	want := []execution.ApprovalOption{
		{ID: "allow-once", Label: "Allow once", Kind: "allow"},
		{ID: "deny", Label: "Deny", Kind: "deny"},
	}
	if len(got.Options) != len(want) {
		t.Fatalf("got %d options, want %d", len(got.Options), len(want))
	}
	for i := range want {
		if got.Options[i] != want[i] {
			t.Fatalf("option %d = %#v, want %#v", i, got.Options[i], want[i])
		}
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("expected the creation instant to be translated")
	}
}

func TestReadRunApprovalsReturnsAnEmptySliceNotNil(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})

	approvals, err := provider.ReadRunApprovals(context.Background(), runRequest(stub))
	if err != nil {
		t.Fatal(err)
	}
	if approvals == nil {
		t.Fatal("an empty list must not serialize as null")
	}
	if len(approvals) != 0 {
		t.Fatalf("got %d approvals, want 0", len(approvals))
	}
}

func TestSendRunMessagePostsTheBody(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})

	if err := provider.SendRunMessage(context.Background(), runRequest(stub), "ciao"); err != nil {
		t.Fatal(err)
	}
	request := stub.findRequest(t, http.MethodPost, "/api/external/runs/"+testRunID+"/messages")
	if request.Body != `{"message":"ciao"}` {
		t.Fatalf("body = %q", request.Body)
	}
}

func TestSendRunMessageRejectsAnEmptyMessage(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})

	err := provider.SendRunMessage(context.Background(), runRequest(stub), "   ")
	reason, ok := execution.RefusalOf(err)
	if !ok || reason != execution.RunRefusedUnsupported {
		t.Fatalf("got %q, %v (%v); want unsupported", reason, ok, err)
	}
	// The caller's own mistake must not cost a round trip.
	if calls := stub.recorded(); len(calls) != 0 {
		t.Fatalf("expected no remote call, got %#v", calls)
	}
}

func TestRespondRunApprovalPostsTheOption(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})

	if err := provider.RespondRunApproval(context.Background(), runRequest(stub), "appr-1", "allow-once"); err != nil {
		t.Fatal(err)
	}
	request := stub.findRequest(t, http.MethodPost, "/api/external/runs/"+testRunID+"/approvals/appr-1/respond")
	if request.Body != `{"optionId":"allow-once"}` {
		t.Fatalf("body = %q", request.Body)
	}
}

func TestCancelRunPostsWithoutABody(t *testing.T) {
	stub := newRunStub(t)
	provider := newTestProvider(t, stub.server, Options{})

	if err := provider.CancelRun(context.Background(), runRequest(stub)); err != nil {
		t.Fatal(err)
	}
	request := stub.findRequest(t, http.MethodPost, "/api/external/runs/"+testRunID+"/cancel")
	if request.Body != "" {
		t.Fatalf("body = %q, want none", request.Body)
	}
}

// runCommands are the three commands, invoked through one signature so the
// refusal table can be a real matrix instead of three copies of it.
func runCommands() []struct {
	name   string
	invoke func(*Provider, execution.RunRequest) error
	fail   func(*runStub, hubResponse)
} {
	return []struct {
		name   string
		invoke func(*Provider, execution.RunRequest) error
		fail   func(*runStub, hubResponse)
	}{
		{
			name: "message",
			invoke: func(p *Provider, req execution.RunRequest) error {
				return p.SendRunMessage(context.Background(), req, "ciao")
			},
			fail: func(s *runStub, response hubResponse) { s.messages = response },
		},
		{
			name: "approval",
			invoke: func(p *Provider, req execution.RunRequest) error {
				return p.RespondRunApproval(context.Background(), req, "appr-1", "allow-once")
			},
			fail: func(s *runStub, response hubResponse) { s.respond = response },
		},
		{
			name: "cancel",
			invoke: func(p *Provider, req execution.RunRequest) error {
				return p.CancelRun(context.Background(), req)
			},
			fail: func(s *runStub, response hubResponse) { s.cancel = response },
		},
	}
}

func TestRunCommandsClassifyRefusals(t *testing.T) {
	refusals := []struct {
		name     string
		response hubResponse
		want     execution.RunRefusalReason
	}{
		{"not found", hubResponse{status: http.StatusNotFound, body: `{"error":"not_found"}`}, execution.RunRefusedNotFound},
		{"run not active", hubResponse{status: http.StatusConflict, body: `{"error":"run_not_active"}`}, execution.RunRefusedNotActive},
		{"runner offline", hubResponse{status: http.StatusConflict, body: `{"error":"runner_offline"}`}, execution.RunRefusedRunnerOffline},
		{"unauthorized", hubResponse{status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`}, execution.RunRefusedUnauthorized},
	}
	for _, command := range runCommands() {
		for _, refusal := range refusals {
			t.Run(command.name+"/"+refusal.name, func(t *testing.T) {
				stub := newRunStub(t)
				command.fail(stub, refusal.response)
				provider := newTestProvider(t, stub.server, Options{})

				err := command.invoke(provider, runRequest(stub))
				reason, ok := execution.RefusalOf(err)
				if !ok || reason != refusal.want {
					t.Fatalf("got %q, %v (%v); want %q", reason, ok, err, refusal.want)
				}
			})
		}
	}
}

func TestRunCommandsDoNotLeakTheToken(t *testing.T) {
	for _, command := range runCommands() {
		t.Run(command.name, func(t *testing.T) {
			stub := newRunStub(t)
			command.fail(stub, hubResponse{status: http.StatusConflict, body: `{"error":"run_not_active"}`})
			provider := newTestProvider(t, stub.server, Options{})

			assertNoSecret(t, command.invoke(provider, runRequest(stub)))
		})
	}
}

func TestRunReadsClassifyRefusals(t *testing.T) {
	reads := []struct {
		name   string
		fail   func(*runStub, hubResponse)
		invoke func(*Provider, execution.RunRequest) error
	}{
		{
			name: "read run",
			fail: func(s *runStub, response hubResponse) { s.run = response },
			invoke: func(p *Provider, req execution.RunRequest) error {
				_, err := p.ReadRun(context.Background(), req)
				return err
			},
		},
		{
			name: "read approvals",
			fail: func(s *runStub, response hubResponse) { s.approvals = response },
			invoke: func(p *Provider, req execution.RunRequest) error {
				_, err := p.ReadRunApprovals(context.Background(), req)
				return err
			},
		},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			stub := newRunStub(t)
			read.fail(stub, hubResponse{status: http.StatusNotFound, body: `{"error":"not_found"}`})
			provider := newTestProvider(t, stub.server, Options{})

			err := read.invoke(provider, runRequest(stub))
			reason, ok := execution.RefusalOf(err)
			if !ok || reason != execution.RunRefusedNotFound {
				t.Fatalf("got %q, %v (%v); want not_found", reason, ok, err)
			}
			assertNoSecret(t, err)
		})
	}
}

// TestStreamUsesAClientWithoutATotalTimeout pins the one property that decides
// whether a run can be followed at all.
//
// http.Client.Timeout bounds the whole exchange, body included. A client
// carrying it cannot hold a text/event-stream open: the connection dies on the
// deadline no matter how healthy it is, the follower above reads a perfectly
// good stream as a drop, and the run spends the rest of its life reconnecting
// on a fixed period. No test that injects a fake Doer can see this, because a
// fake has no timeout — so the property is asserted on the real client the
// provider builds for itself.
func TestStreamUsesAClientWithoutATotalTimeout(t *testing.T) {
	provider := New(Options{})

	stream, ok := provider.streamDoer.(*http.Client)
	if !ok {
		t.Fatalf("the stream client is %T, want *http.Client", provider.streamDoer)
	}
	if stream.Timeout != 0 {
		t.Fatalf("the stream client carries a total timeout of %v; an event stream cannot outlive it", stream.Timeout)
	}
	// The header phase is still bounded: unbounded everywhere would hang on a
	// hub that accepts the connection and never answers.
	transport, ok := stream.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("the stream transport is %T, want *http.Transport", stream.Transport)
	}
	if transport.ResponseHeaderTimeout != streamHeaderTimeout {
		t.Fatalf("response header timeout = %v, want %v", transport.ResponseHeaderTimeout, streamHeaderTimeout)
	}
	// The command and poll routes keep their total timeout: it is right there
	// and wrong only on the stream.
	command, ok := provider.doer.(*http.Client)
	if !ok {
		t.Fatalf("the command client is %T, want *http.Client", provider.doer)
	}
	if command.Timeout == 0 {
		t.Fatal("the command client must keep a total timeout")
	}
	if provider.doer == provider.streamDoer {
		t.Fatal("the stream and the commands must not share one client")
	}
}

// TestInjectedDoerAlsoServesTheStream keeps the test seam honest: a caller that
// injects one client means it for every call, including the stream.
func TestInjectedDoerAlsoServesTheStream(t *testing.T) {
	stub := newRunStub(t)
	provider := New(Options{Doer: stub.server.Client(), Getenv: func(string) string { return testToken }})
	if provider.streamDoer != provider.doer {
		t.Fatal("an injected Doer must serve the stream too, or a test would reach the network")
	}
}
