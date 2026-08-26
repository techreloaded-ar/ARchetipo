package arcipelago

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const testConversationID = "conv-7"

func conversationProvider(stub *runStub) *Provider {
	return New(Options{
		Doer:   stub.server.Client(),
		Getenv: func(string) string { return testToken },
		// A conversation must never wait on the wall clock in a test: every
		// pause the provider takes is answered immediately, and what is being
		// asserted is the sequence of calls, never their timing.
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
}

func conversationRequest(stub *runStub) execution.ConversationRequest {
	return execution.ConversationRequest{
		ConversationID: testConversationID,
		WorkingDir:     "/somewhere/local",
		ProviderConfig: runConfig(stub),
		ProcessActions: []execution.ConversationAction{
			{ID: "plan", Label: "Pianifica la spec", Scope: "spec"},
		},
	}
}

// openConversation opens one and closes it through t.Cleanup, so a test that
// fails midway still releases the follower goroutine.
func openConversation(t *testing.T, provider *Provider, stub *runStub) {
	t.Helper()
	if err := provider.OpenConversation(context.Background(), conversationRequest(stub)); err != nil {
		t.Fatalf("opening the conversation: %v", err)
	}
	t.Cleanup(func() { _ = provider.CloseConversation(context.Background(), testConversationID) })
}

func TestProviderDeclaresWorkspaceConverse(t *testing.T) {
	declared, err := execution.DeclaredCapabilities(context.Background(), New(Options{}))
	if err != nil {
		t.Fatalf("reading the declared capabilities: %v", err)
	}
	var converses bool
	for _, capability := range declared {
		if capability == execution.CapabilityWorkspaceConverse {
			converses = true
		}
	}
	if !converses {
		t.Fatalf("the provider implements Conversationalist but declares %v", declared)
	}
}

func TestOpenConversationCreatesTheTaskUnderTheConversationID(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(func(ctx context.Context, _ io.Writer, _ func()) { <-ctx.Done() })
	provider := conversationProvider(stub)

	openConversation(t, provider, stub)

	created := stub.findRequest(t, http.MethodPost, pathTasks)
	var body struct {
		ExternalID string         `json:"externalId"`
		Source     string         `json:"source"`
		Title      string         `json:"title"`
		Prompt     string         `json:"prompt"`
		Metadata   map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(created.Body), &body); err != nil {
		t.Fatalf("the created task is not readable: %v", err)
	}
	if body.ExternalID != testConversationID {
		t.Fatalf("the task carries externalId %q, want the conversation id", body.ExternalID)
	}
	if body.Source != sourceARchetipo {
		t.Fatalf("the task carries source %q", body.Source)
	}
	if body.Metadata["kind"] != "conversation" {
		t.Fatalf("the task metadata does not say it is a conversation: %#v", body.Metadata)
	}
	// The prompt is the shared declaration: what the viewer parses back is the
	// proposal line, and a conversation whose agent was never told its shape
	// would answer with prose nobody can confirm.
	if !strings.Contains(body.Prompt, execution.ActionProposalArtifact) {
		t.Fatal("the conversation prompt does not carry the proposal contract")
	}
	if !strings.Contains(body.Prompt, "- plan (spec): Pianifica la spec") {
		t.Fatal("the conversation prompt does not carry the process vocabulary it was given")
	}
	if strings.Contains(body.Prompt, plannedStatus) {
		t.Fatal("the conversation prompt asks for a receipt; a conversation has none")
	}
}

func TestBuildConversationTaskIsDeterministic(t *testing.T) {
	// The hub keys external equivalence on a fingerprint over title, prompt and
	// metadata: a retry of the same open that rendered one byte differently
	// would be answered 409 instead of being recognized as the repetition it is.
	req := execution.ConversationRequest{
		ConversationID: testConversationID,
		ProcessActions: []execution.ConversationAction{{ID: "plan", Label: "Piano", Scope: "spec"}},
		Context:        "una conversazione precedente",
	}
	firstTitle, firstPrompt, firstMetadata := buildConversationTask(testConversationID, req)
	secondTitle, secondPrompt, secondMetadata := buildConversationTask(testConversationID, req)
	if firstTitle != secondTitle || firstPrompt != secondPrompt {
		t.Fatal("two renderings of the same conversation differ")
	}
	first, _ := json.Marshal(firstMetadata)
	second, _ := json.Marshal(secondMetadata)
	if string(first) != string(second) {
		t.Fatalf("the metadata differs between renderings: %s vs %s", first, second)
	}
	if !strings.Contains(firstPrompt, "una conversazione precedente") {
		t.Fatal("the resumed transcript did not reach the prompt")
	}
}

func TestOpenConversationWaitsForTheRunTheHubAssigns(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(func(ctx context.Context, _ io.Writer, _ func()) { <-ctx.Done() })
	// The creation answers a task with no run yet: the hub has not handed it to
	// a runner. The first poll still has none, the second does.
	stub.create = hubResponse{status: http.StatusCreated, body: `{"task":{"id":"task-1","status":"queued"}}`}
	polls := 0
	provider := conversationProvider(stub)
	stub.onTaskRead = func() hubResponse {
		polls++
		if polls < 2 {
			return hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-1","status":"queued"}}`}
		}
		return hubResponse{status: http.StatusOK, body: `{"task":{"id":"task-1","status":"running","runId":"` + testRunID + `"}}`}
	}

	openConversation(t, provider, stub)

	if polls < 2 {
		t.Fatalf("the provider polled the task %d times; it must wait for the run", polls)
	}
	// And the follower is attached to the run the hub assigned, not to the task.
	waitFor(t, func() bool { return len(stub.subscriptions()) > 0 })
}

func TestConversationIsCommandedUnderItsOwnID(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(func(ctx context.Context, _ io.Writer, _ func()) { <-ctx.Done() })
	provider := conversationProvider(stub)
	openConversation(t, provider, stub)

	// The viewer knows only the conversation id and sends every command with
	// it. What must reach the hub is the run behind it.
	req := execution.RunRequest{RunID: testConversationID, ProviderConfig: runConfig(stub)}
	if err := provider.SendRunMessage(context.Background(), req, "ciao"); err != nil {
		t.Fatalf("sending a message to the conversation: %v", err)
	}
	sent := stub.findRequest(t, http.MethodPost, runPath(testRunID)+"/messages")
	if !strings.Contains(sent.Body, "ciao") {
		t.Fatalf("the message did not travel: %q", sent.Body)
	}

	snapshot, err := provider.ReadRun(context.Background(), req)
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	if snapshot.RunID != testConversationID {
		t.Fatalf("the snapshot answers %q; the caller asked about the conversation", snapshot.RunID)
	}
	if snapshot.State != execution.RunActive {
		t.Fatalf("the conversation reads as %v", snapshot.State)
	}
}

func TestConversationMirrorsTheRemoteHistory(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(func(ctx context.Context, w io.Writer, flush func()) {
		_, _ = io.WriteString(w, runEventFrameText(1, 1, textDelta("buongiorno")))
		flush()
		<-ctx.Done()
	})
	provider := conversationProvider(stub)
	openConversation(t, provider, stub)

	// The viewer reads the conversation through the very door a local one is
	// read through: the registry of sessions on the collaborator.
	var session interface {
		Events(int64) []execution.RunEvent
	}
	waitFor(t, func() bool {
		found, ok := provider.Registry().Lookup(testConversationID)
		if !ok {
			return false
		}
		session = found
		return len(found.Events(0)) > 0
	})
	events := session.Events(0)
	if events[0].Text != "buongiorno" {
		t.Fatalf("the mirrored history says %q", events[0].Text)
	}
}

func TestCloseConversationCancelsTheRemoteRunAndIsIdempotent(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(func(ctx context.Context, _ io.Writer, _ func()) { <-ctx.Done() })
	provider := conversationProvider(stub)
	if err := provider.OpenConversation(context.Background(), conversationRequest(stub)); err != nil {
		t.Fatalf("opening the conversation: %v", err)
	}

	if err := provider.CloseConversation(context.Background(), testConversationID); err != nil {
		t.Fatalf("closing the conversation: %v", err)
	}
	stub.findRequest(t, http.MethodPost, runPath(testRunID)+"/cancel")

	// Closing twice releases nothing and reports nothing: the second close is
	// the same answer as a close on a conversation that never existed.
	if err := provider.CloseConversation(context.Background(), testConversationID); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
	if err := provider.CloseConversation(context.Background(), "never-opened"); err != nil {
		t.Fatalf("closing an unknown conversation: %v", err)
	}

	// The mirror survives the close: the history of a conversation outlives the
	// provider's hold on it.
	session, ok := provider.Registry().Lookup(testConversationID)
	if !ok {
		t.Fatal("the mirrored session was dropped by the close")
	}
	if session.Active() {
		t.Fatal("the mirrored session is still active after the close")
	}
}

func TestOpenConversationRefusesASecondOpenOnTheSameID(t *testing.T) {
	stub := newRunStub(t)
	stub.setStream(func(ctx context.Context, _ io.Writer, _ func()) { <-ctx.Done() })
	provider := conversationProvider(stub)
	openConversation(t, provider, stub)

	if err := provider.OpenConversation(context.Background(), conversationRequest(stub)); err == nil {
		t.Fatal("a second open on a held id must be refused")
	}
}

func TestOpenConversationWithoutAnIDIsRefusedBeforeAnyCall(t *testing.T) {
	stub := newRunStub(t)
	provider := conversationProvider(stub)
	req := conversationRequest(stub)
	req.ConversationID = "  "
	if err := provider.OpenConversation(context.Background(), req); err == nil {
		t.Fatal("a conversation without an id must be refused")
	}
	if len(stub.recorded()) != 0 {
		t.Fatalf("the hub was called anyway: %#v", stub.recorded())
	}
}

// waitFor polls a condition the follower goroutine satisfies. It is the one
// concession to timing in this file, and it is bounded.
func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the condition was never satisfied")
}
