package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// newActionSessionServer is a viewer whose default provider holds native
// sessions *and* declares the actions of the process. It is the only shape in
// which an action can be carried out inside a conversation, and therefore the
// only shape these tests need.
func newActionSessionServer(t *testing.T) (*Server, *persistentFakeNativeProvider, connector.Connector) {
	t.Helper()
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	provider.capabilities = []execution.Capability{
		execution.CapabilitySpecPlan,
		execution.CapabilitySpecImplement,
		execution.CapabilitySpecReview,
	}
	srv, _, conn := newRunServer(t, provider, true)
	// The followers write into the workspace directory the framework is about
	// to delete, so they are ended before it goes: a cancellation alone would
	// leave one mid-write.
	t.Cleanup(func() { srv.session().nativeFollowers.closeAll() })
	return srv, provider, conn
}

// completeCurrentTurn is the agent finishing a turn: it says one last thing and
// the turn reaches the state given. It is the only way a turn ends in these
// tests, so a receipt is always something the agent wrote in a turn, never
// something the test injected into the record.
func (p *persistentFakeNativeProvider) completeCurrentTurn(t *testing.T, message string, state execution.TurnState) {
	t.Helper()
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	for _, session := range p.store.sessions {
		if session.snapshot.CurrentTurn == nil || session.snapshot.Work != execution.SessionTurnActive {
			continue
		}
		turn := session.snapshot.CurrentTurn
		session.events = append(session.events, execution.RunEvent{
			ID: int64(len(session.events) + 1), Kind: "text", Text: message,
			TurnID: turn.ID, At: time.Now().UTC(),
		})
		completed := time.Now().UTC()
		turn.State, turn.CompletedAt = state, &completed
		session.snapshot.Work = execution.SessionIdle
		select {
		case session.notify <- struct{}{}:
		default:
		}
		return
	}
	t.Fatalf("no active turn to complete")
}

// awaitExecutionStatus polls the record the way the browser does. It is bounded
// so a reconciliation that never happens fails the test instead of hanging it.
func awaitExecutionStatus(t *testing.T, srv *Server, id string, want execution.ExecutionStatus) execution.Execution {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, record := readExecution(t, srv, id)
		if status == http.StatusOK && record.Status == want {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, record := readExecution(t, srv, id)
	t.Fatalf("execution %s is %s, want %s (%v)", id, record.Status, want, record.Error)
	return execution.Execution{}
}

func startSpecActionIn(t *testing.T, srv *Server, code, action, conversationID string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{"action": action}
	if conversationID != "" {
		body["conversation_id"] = conversationID
	}
	w := doJSON(t, srv, http.MethodPost, "/api/spec/"+code+"/execution", body)
	var decoded map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("undecodable response (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, decoded
}

func conversationOfExecution(t *testing.T, srv *Server, executionID string) string {
	t.Helper()
	store, err := conversationlog.NewFileStore(srv.session().cfg.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		for _, id := range record.ExecutionIDs {
			if id == executionID {
				return record.ID
			}
		}
	}
	t.Fatalf("no conversation carries the execution %s", executionID)
	return ""
}

func persistPlanFor(t *testing.T, conn connector.Connector, code string) {
	t.Helper()
	persistImplementablePlan(t, conn, code)
	moveSpecTo(t, conn, code, domain.StatusPlanned)
}

// TestOneConversationCarriesSeveralActionsAndStaysOpen is the whole criterion of
// the task in one thread: a plan that asks a question and is answered, a model
// changed between actions, an implementation, and a free message afterwards —
// all in one native session, with one execution record per action, each closed
// exactly once, and the conversation still writable at the end.
func TestOneConversationCarriesSeveralActionsAndStaysOpen(t *testing.T) {
	srv, provider, conn := newActionSessionServer(t)

	status, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("start plan = %d: %v", status, started)
	}
	planID, _ := started["id"].(string)
	if planID == "" {
		t.Fatalf("the start did not answer with an execution: %v", started)
	}
	conversationID := conversationOfExecution(t, srv, planID)
	if provider.store.starts != 1 {
		t.Fatalf("the plan opened %d turns, want exactly one", provider.store.starts)
	}
	if provider.turnStarts[0].ExecutionID != planID {
		t.Fatalf("the first turn carries %q, want the plan execution %q", provider.turnStarts[0].ExecutionID, planID)
	}

	// The agent asks instead of finishing. A turn that ends without a receipt is
	// not a failure: the action goes on into the next turn.
	provider.completeCurrentTurn(t, "Su quale epica va questa spec?", execution.TurnCompleted)
	awaitRecordSettled(t, srv, conversationID)
	if _, record := readExecution(t, srv, planID); record.Status != execution.StatusRunning {
		t.Fatalf("a question closed the plan as %s: %v", record.Status, record.Error)
	}

	// The person answers in the very thread they are reading. The answer belongs
	// to the action it answers.
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/messages", map[string]any{"message": "EP-009"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("answer = %d: %s", w.Code, w.Body.String())
	}
	if provider.turnStarts[1].ExecutionID != planID {
		t.Fatalf("the answer opened a turn carrying %q, want the plan execution", provider.turnStarts[1].ExecutionID)
	}

	persistPlanFor(t, conn, "US-901")
	provider.completeCurrentTurn(t, "Fatto.\n{\"spec_code\":\"US-901\",\"status\":\"PLANNED\",\"tasks\":2}", execution.TurnCompleted)
	planned := awaitExecutionStatus(t, srv, planID, execution.StatusSucceeded)
	if planned.Result == nil {
		t.Fatalf("the closed plan carries no result: %v", planned)
	}

	// The next turn is asked to run on another model, and the action started
	// afterwards uses it: choosing is a fact of the session, not of the action.
	w = doJSON(t, srv, http.MethodPut, conversationsRoute+"/"+conversationID+"/next-turn",
		map[string]any{"model": "fake-large", "model_options": map[string]string{"effort": "high"}})
	if w.Code != http.StatusOK {
		t.Fatalf("next-turn = %d: %s", w.Code, w.Body.String())
	}

	status, startedImplement := startSpecActionIn(t, srv, "US-901", "implement", conversationID)
	if status != http.StatusCreated {
		t.Fatalf("start implement = %d: %v", status, startedImplement)
	}
	implementID, _ := startedImplement["id"].(string)
	if implementID == planID || implementID == "" {
		t.Fatalf("the implementation reused the plan record: %v", startedImplement)
	}
	// The record stamps the status the spec had when the action was accepted,
	// not the one the action's own transition has just produced.
	if startedImplement["spec_status_before"] != string(domain.StatusPlanned) {
		t.Fatalf("the implementation record says the spec was %v, want %s", startedImplement["spec_status_before"], domain.StatusPlanned)
	}
	if conversationOfExecution(t, srv, implementID) != conversationID {
		t.Fatalf("the implementation was started in another conversation")
	}
	if len(provider.store.sessions) != 1 {
		t.Fatalf("the workspace holds %d native sessions, want exactly one", len(provider.store.sessions))
	}
	last := provider.turnStarts[len(provider.turnStarts)-1]
	if last.ExecutionID != implementID || last.Model != "fake-large" || last.Options["effort"] != "high" {
		t.Fatalf("the implementation turn is %+v, want the chosen model and its own execution", last)
	}

	moveSpecTo(t, conn, "US-901", domain.StatusReview)
	completeEveryTask(t, conn, "US-901")
	provider.completeCurrentTurn(t, "{\"spec_code\":\"US-901\",\"status\":\"REVIEW\",\"tasks_done\":2,\"tests\":\"verdi\"}", execution.TurnCompleted)
	awaitExecutionStatus(t, srv, implementID, execution.StatusSucceeded)

	// The session is still the person's to talk in, and the free message it
	// takes belongs to no action at all.
	w = doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/messages", map[string]any{"message": "grazie"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("free message after the action = %d: %s", w.Code, w.Body.String())
	}
	free := provider.turnStarts[len(provider.turnStarts)-1]
	if free.ExecutionID != "" {
		t.Fatalf("a free message was attached to the execution %q", free.ExecutionID)
	}

	_, view, body := readConversation(t, srv, conversationID, 0)
	if view.Conversation == nil || view.Conversation.State != string(execution.RunActive) {
		t.Fatalf("the conversation did not stay open: %s", body)
	}
	if len(view.Runs) != 2 {
		t.Fatalf("the thread shows %d actions, want the two it carried out: %s", len(view.Runs), body)
	}
	for _, run := range view.Runs {
		if run.ThreadID != "" {
			t.Fatalf("an action of this thread points at another one: %s", body)
		}
	}
}

// completeEveryTask closes every task of the persisted plan, which is what the
// implementation gate reads before it accepts the receipt.
func completeEveryTask(t *testing.T, conn connector.Connector, code string) {
	t.Helper()
	ctx := context.Background()
	tasks, err := conn.ReadSpecTasks(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if _, err := conn.CompleteTask(ctx, code, task.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// awaitRecordSettled waits for the follower to have observed the end of the
// turn, so an assertion about "the action is still running" is made after the
// reconciliation had its chance rather than before it.
func awaitRecordSettled(t *testing.T, srv *Server, conversationID string) {
	t.Helper()
	store, err := conversationlog.NewFileStore(srv.session().cfg.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		record, readErr := store.GetMetadata(context.Background(), conversationID)
		if readErr == nil && record.CurrentTurn != nil && terminalTurn(record.CurrentTurn.State) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the end of the turn of %s was never observed", conversationID)
}

// TestASecondActionIsRefusedWhileOneIsRunningInTheThread covers the duplicate
// submission: pressing twice starts one action, not two, and the refusal names
// the one already under way.
func TestASecondActionIsRefusedWhileOneIsRunningInTheThread(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	status, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("start plan = %d: %v", status, started)
	}
	planID, _ := started["id"].(string)
	conversationID := conversationOfExecution(t, srv, planID)

	second, refusal := startSpecActionIn(t, srv, "US-901", "plan", conversationID)
	if second != http.StatusConflict {
		t.Fatalf("the second press answered %d, want 409: %v", second, refusal)
	}
	if provider.store.starts != 1 {
		t.Fatalf("the second press opened another turn: %d", provider.store.starts)
	}
}

// TestCancellingAnActionWaitingForAnAnswerClosesItsRecord covers the other half
// of the stop gesture: an action whose turn already ended, and which is waiting
// for an answer nobody is going to give, is closed by the stop button while the
// conversation stays open and writable.
func TestCancellingAnActionWaitingForAnAnswerClosesItsRecord(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	_, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	planID, _ := started["id"].(string)
	conversationID := conversationOfExecution(t, srv, planID)

	provider.completeCurrentTurn(t, "Mi serve una risposta.", execution.TurnCompleted)
	awaitRecordSettled(t, srv, conversationID)

	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/interrupt", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("cancel = %d: %s", w.Code, w.Body.String())
	}
	cancelled := awaitExecutionStatus(t, srv, planID, execution.StatusFailed)
	if cancelled.Error == nil {
		t.Fatalf("the cancelled action carries no reason: %v", cancelled)
	}

	w = doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/messages", map[string]any{"message": "riparliamone"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("the conversation did not survive the cancellation: %d %s", w.Code, w.Body.String())
	}
}

// TestAnInterruptedTurnFailsTheActionItWasCarryingOut is the cancellation of an
// action that is still speaking: the turn is interrupted, and the record closes
// as failed instead of waiting for a receipt that will never come.
func TestAnInterruptedTurnFailsTheActionItWasCarryingOut(t *testing.T) {
	srv, _, _ := newActionSessionServer(t)
	_, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	planID, _ := started["id"].(string)
	conversationID := conversationOfExecution(t, srv, planID)

	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/interrupt", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("interrupt = %d: %s", w.Code, w.Body.String())
	}
	awaitExecutionStatus(t, srv, planID, execution.StatusFailed)
}

// TestAReceiptSaidBeforeTheActionCannotCloseIt is the correlation rule: only a
// receipt emitted in a turn that belongs to the action closes it. A receipt said
// in an ordinary turn — quoted, pasted, left over from before — is part of the
// transcript and nothing else.
func TestAReceiptSaidBeforeTheActionCannotCloseIt(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	opened := openConversationOK(t, srv)
	conversationID := opened.Conversation.ID

	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+conversationID+"/messages", map[string]any{"message": "che ne pensi?"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("free message = %d: %s", w.Code, w.Body.String())
	}
	provider.completeCurrentTurn(t, "Una ricevuta si scrive così:\n{\"spec_code\":\"US-901\",\"status\":\"PLANNED\",\"tasks\":2}", execution.TurnCompleted)
	awaitRecordSettled(t, srv, conversationID)

	status, started := startSpecActionIn(t, srv, "US-901", "plan", conversationID)
	if status != http.StatusCreated {
		t.Fatalf("start plan = %d: %v", status, started)
	}
	planID, _ := started["id"].(string)
	// The action has produced nothing of its own, so the old receipt must not
	// close it. Giving the follower time to run is the whole point of the wait.
	time.Sleep(300 * time.Millisecond)
	if _, record := readExecution(t, srv, planID); record.Status != execution.StatusRunning {
		t.Fatalf("a receipt from an earlier turn closed the action as %s", record.Status)
	}
}

// TestAnActionWhoseReceiptTheWorkspaceDeniesIsRefused keeps the verification of
// the persisted effects on the new path: the agent declares a plan, the backlog
// does not have one, and the record closes as failed.
func TestAnActionWhoseReceiptTheWorkspaceDeniesIsRefused(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	_, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	planID, _ := started["id"].(string)

	provider.completeCurrentTurn(t, "{\"spec_code\":\"US-901\",\"status\":\"PLANNED\",\"tasks\":2}", execution.TurnCompleted)
	refused := awaitExecutionStatus(t, srv, planID, execution.StatusFailed)
	if refused.Error == nil || refused.Error.Code != "UNCONFIRMED_EFFECT" {
		t.Fatalf("the unverified claim closed as %+v, want UNCONFIRMED_EFFECT", refused.Error)
	}
}
