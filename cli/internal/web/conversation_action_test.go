package web

// The oracle of "a run is a conversation with a preconfigured prompt".
//
// Nothing on the start path is doubled: the record store, the connector, the
// process Template, the conversation holder, the journal and the start
// functions are the production ones. Only the agent process is a double, and it
// is doubled the way a local provider really behaves — a run registers a
// localrun session under the execution's own id, and that session is what the
// thread is read and commanded through.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// runSessionProvider is a conversing provider whose runs behave like a local
// provider's: dispatching one registers a session under the execution's id, so
// the run really is followable and commandable as the conversation it is.
type runSessionProvider struct {
	*conversingProvider

	runMu       sync.Mutex
	runSessions map[string]*localrun.Session
	runDialogue map[string]*fakeDialogue
}

func newRunSessionProvider(id string) *runSessionProvider {
	return &runSessionProvider{
		conversingProvider: newConversingProvider(id, 0),
		runSessions:        map[string]*localrun.Session{},
		runDialogue:        map[string]*fakeDialogue{},
	}
}

func (p *runSessionProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{execution.CapabilitySpecPlan}, nil
}

func (p *runSessionProvider) Execute(ctx context.Context, request execution.Request) (execution.Result, error) {
	session := localrun.NewSession(request.ExecutionID, nil)
	dialogue := &fakeDialogue{}
	session.AttachDialogue(dialogue)
	p.Registry().Register(session)
	p.runMu.Lock()
	p.runSessions[request.ExecutionID] = session
	p.runDialogue[request.ExecutionID] = dialogue
	p.runMu.Unlock()
	result, err := p.conversingProvider.Execute(ctx, request)
	// The end of a run closes its session, exactly as a local provider closes
	// the one it observed ending.
	if err != nil {
		session.Close(execution.RunCrashed, "the run ended badly")
	} else {
		session.Close(execution.RunClosed, "")
	}
	return result, err
}

func (p *runSessionProvider) runSessionOf(t *testing.T, executionID string) *localrun.Session {
	t.Helper()
	p.runMu.Lock()
	defer p.runMu.Unlock()
	session, ok := p.runSessions[executionID]
	if !ok {
		t.Fatalf("the provider registered no run session for %q", executionID)
	}
	return session
}

func (p *runSessionProvider) runDialogueOf(t *testing.T, executionID string) *fakeDialogue {
	t.Helper()
	p.runMu.Lock()
	defer p.runMu.Unlock()
	dialogue, ok := p.runDialogue[executionID]
	if !ok {
		t.Fatalf("the provider registered no dialogue for %q", executionID)
	}
	return dialogue
}

// actionThreadServer is a workspace whose default provider converses and whose
// runs stay inside Execute until the test releases them, so the thread can be
// read while the work is still going.
func actionThreadServer(t *testing.T) (*Server, *runSessionProvider) {
	t.Helper()
	var conn connector.Connector
	provider := newRunSessionProvider("chatty")
	// Blocked on purpose: a released run would be over before the thread could
	// be read, and what these tests are about is what a thread says while its
	// agent is working.
	provider.runTestProvider = newRunTestProvider("chatty", func(ctx context.Context, request execution.Request) (execution.Result, error) {
		return planningExecute(conn)(ctx, request)
	})
	srv, backlog := newProposalServer(t, provider)
	conn = backlog
	return srv, provider
}

func actionThreadStart(t *testing.T, srv *Server, code string) string {
	t.Helper()
	status, started := adoptTestStart(t, srv, code, "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("POST spec execution = %d, want 201: %v", status, started)
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatalf("the start does not name the execution: %v", started)
	}
	return id
}

// The thread of a run is the run: it is held under the execution's id, it says
// which step it is doing, it carries that execution as its outcome, and its
// history is the history of the very session the provider opened for the work.
//
// One agent process, not two. Before this, pressing a step lit a second one —
// idle — only so that there was somewhere to read about the first.
func TestTheThreadOfARunIsTheRun(t *testing.T) {
	srv, provider := actionThreadServer(t)
	executionID := actionThreadStart(t, srv, "US-901")
	<-provider.entered

	// The agent speaks inside the run's own session, which is the thread.
	session := provider.runSessionOf(t, executionID)
	said := session.Append(execution.RunEvent{Kind: localrun.KindText, Text: "PIANIFICO-LA-901"})

	status, view, body := readConversation(t, srv, executionID, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}
	if view.Conversation == nil {
		t.Fatalf("the run has no thread: %s", body)
	}
	if view.Conversation.Action != "plan" || view.Conversation.ExecutionID != executionID {
		t.Fatalf("the thread does not say it is the plan run: %#v (%s)", view.Conversation, body)
	}
	if view.Conversation.State != string(execution.RunActive) {
		t.Fatalf("state = %q, want the run's own observed state", view.Conversation.State)
	}
	if view.Execution == nil {
		t.Fatalf("the thread carries no outcome: %s", body)
	}
	if view.Execution.ID != executionID || view.Execution.Status != string(execution.StatusRunning) {
		t.Fatalf("the outcome is not the running record: %#v (%s)", view.Execution, body)
	}
	if len(view.Events) != 1 || view.Events[0].ID != said.ID || view.Events[0].Text != "PIANIFICO-LA-901" {
		t.Fatalf("the thread does not carry what the agent said: %#v (%s)", view.Events, body)
	}
	if view.Notice != "" {
		t.Fatalf("notice = %q, want none: the thread is perfectly readable", view.Notice)
	}

	close(provider.release)
	awaitTerminal(t, srv, executionID)
}

// The thread of a run exists the instant the start answers, before the provider
// has registered anything. Saying "this viewer cannot read it" in that window
// would be answering the wrong question about a thread that is simply about to
// start speaking.
func TestTheThreadOfARunThatHasNotSpokenYetIsNotDeclaredUnreadable(t *testing.T) {
	srv, provider := actionThreadServer(t)
	executionID := actionThreadStart(t, srv, "US-901")

	_, view, body := readConversation(t, srv, executionID, 0)
	if view.Conversation == nil {
		t.Fatalf("the run has no thread: %s", body)
	}
	if strings.Contains(view.Notice, "not readable") {
		t.Fatalf("notice = %q, want no complaint about a thread that has not started speaking", view.Notice)
	}
	if len(view.Events) != 0 {
		t.Fatalf("the thread carries events nobody produced: %#v", view.Events)
	}

	<-provider.entered
	close(provider.release)
	awaitTerminal(t, srv, executionID)
}

// A message written into the thread of a run reaches the agent that is doing
// the work. It is the whole point of the unification: the person reading the
// step can answer the question the agent asked.
func TestAMessageInTheThreadOfARunReachesTheAgentDoingTheWork(t *testing.T) {
	srv, provider := actionThreadServer(t)
	executionID := actionThreadStart(t, srv, "US-901")
	<-provider.entered

	const answer = "Sì, procedi pure."
	status, _, body := sendConversationMessage(t, srv, executionID, answer)
	if status != http.StatusAccepted {
		t.Fatalf("POST message = %d, want 202: %s", status, body)
	}
	if got := provider.runDialogueOf(t, executionID).messages(); len(got) != 1 || got[0] != answer {
		t.Fatalf("the agent received %v; want exactly the answer", got)
	}

	close(provider.release)
	awaitTerminal(t, srv, executionID)
}

// Closing the thread of a run cancels that run, and it is not a translation
// between two vocabularies: the thread and the run are one session, so the
// gesture that ends one ends the other. Nothing here writes a terminal state —
// the run is over when the process says so.
func TestClosingTheThreadOfARunCancelsIt(t *testing.T) {
	srv, provider := actionThreadServer(t)
	executionID := actionThreadStart(t, srv, "US-901")
	<-provider.entered

	w := doJSON(t, srv, http.MethodDelete, conversationsRoute+"/"+executionID, nil)
	if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("DELETE conversation = %d: %s", w.Code, w.Body.String())
	}
	if got := provider.runDialogueOf(t, executionID).stops(); got != 1 {
		t.Fatalf("the run was asked to stop %d time(s); want once", got)
	}
	// The provider was never asked to close a conversation: it never opened one.
	if got := provider.closings(); len(got) != 0 {
		t.Fatalf("CloseConversation was called for a run: %v", got)
	}

	close(provider.release)
	awaitTerminal(t, srv, executionID)
}

// When the run ends, its thread ends with it: the transcript is sealed to disk
// with the step it carried out and what became of it, and the thread stops
// being one the workspace holds — a composer nobody can send anything into is
// not a conversation.
func TestTheThreadOfARunIsSealedWithItsOutcome(t *testing.T) {
	srv, provider := actionThreadServer(t)
	executionID := actionThreadStart(t, srv, "US-901")
	<-provider.entered
	provider.runSessionOf(t, executionID).Append(execution.RunEvent{Kind: localrun.KindText, Text: "FATTO"})

	close(provider.release)
	awaitTerminal(t, srv, executionID)
	waitFor(t, "the thread of the run to be sealed", func() bool {
		return !containsString(liveConversationIDs(srv), executionID)
	})

	record := journalTestRecordOf(t, srv, executionID)
	if record.Action != "plan" || record.ExecutionID != executionID {
		t.Fatalf("the record does not say which step it was: %#v", record)
	}
	if record.Outcome != string(execution.StatusSucceeded) {
		t.Fatalf("outcome = %q, want the terminal status of the execution", record.Outcome)
	}
	if record.SpecCode != "US-901" {
		t.Fatalf("spec_code = %q, want the spec the step was about", record.SpecCode)
	}
	if len(record.Events) == 0 {
		t.Fatalf("the sealed transcript is empty: %#v", record)
	}
	// The transcript stays readable by id, exactly as a free conversation's
	// does once it is closed.
	status, view, body := readConversation(t, srv, executionID, 0)
	if status != http.StatusOK || view.Conversation == nil {
		t.Fatalf("the sealed thread is no longer readable: %d %s", status, body)
	}
}

// A provider that cannot converse keeps the run path it always had: no thread
// is held for its runs. Holding one would offer a composer to an agent with no
// turn to receive a message into.
func TestARunOfAProviderThatCannotConverseIsNotAThread(t *testing.T) {
	provider := releasedProvider("fake", func(context.Context, execution.Request) (execution.Result, error) {
		return execution.Result{Payload: json.RawMessage(`{"ok":true}`)}, nil
	})
	srv, _, _ := newRunServer(t, provider, true)

	status, started := startAction(t, srv, "US-901", "plan")
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	if live := liveConversationIDs(srv); len(live) != 0 {
		t.Fatalf("the workspace holds %v, want nothing: this provider does not converse", live)
	}
}
