package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workflowrun"
)

func TestWorkflowContinuesSameNativeSessionAfterConnectionLoss(t *testing.T) {
	p := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, p, true)
	ws := srv.session()
	t.Cleanup(ws.nativeFollowers.closeAll)
	id := openConversationOK(t, srv).Conversation.ID
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "complete the approved work"})
	if w.Code != http.StatusAccepted {
		t.Fatal(w.Body.String())
	}
	record := conversationRecord(t, ws, id)
	run, err := workflowrun.New(record.WorkingDir).Start(context.Background(), workflowrun.Start{Skill: "archetipo-example", Invocation: "/archetipo-example", Scope: "ALL", ConversationID: id, State: json.RawMessage(`{"completed":["TASK-01"]}`)})
	if err != nil {
		t.Fatal(err)
	}
	p.store.loseRuntime()
	deadline := time.Now().Add(3 * time.Second)
	for {
		p.store.mu.Lock()
		starts, resumes := p.store.starts, p.store.resumes
		p.store.mu.Unlock()
		if starts == 2 && resumes == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no continuation: starts=%d resumes=%d", starts, resumes)
		}
		time.Sleep(time.Millisecond)
	}
	p.store.mu.Lock()
	request := p.lastStart
	p.store.mu.Unlock()
	if request.Session.Native.ID != record.Session.Native.ID || !strings.Contains(request.Message, "Resume workflow run "+run.ID) {
		t.Fatalf("wrong continuation: %+v", request)
	}
	if !strings.HasPrefix(request.Message, "/archetipo-example\n") || strings.Contains(request.Message, "/archetipo-example ALL") {
		t.Fatal("normalized scope was used as executable arguments")
	}
	stored, err := workflowrun.New(record.WorkingDir).Read(run.ID)
	if err != nil || stored.RecoveryAttempts != 1 || !strings.Contains(string(stored.State), `"TASK-01"`) {
		t.Fatalf("checkpoint changed: %+v %v", stored, err)
	}
}

func TestIdleDisconnectReconcilesTerminalProviderStateBeforeRecovery(t *testing.T) {
	p := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, p, true)
	ws := srv.session()
	t.Cleanup(ws.nativeFollowers.closeAll)
	id := openConversationOK(t, srv).Conversation.ID
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "work"})
	if w.Code != http.StatusAccepted {
		t.Fatal(w.Body.String())
	}
	r := conversationRecord(t, ws, id)
	_, err := workflowrun.New(r.WorkingDir).Start(context.Background(), workflowrun.Start{Skill: "archetipo-example", Invocation: "/archetipo-example", Scope: "example", ConversationID: id, State: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	p.store.mu.Lock()
	native := p.store.sessions[r.Session.Native.ID]
	native.snapshot.CurrentTurn.State = execution.TurnCompleted
	native.snapshot.Work = execution.SessionIdle
	native.snapshot.Connection = execution.ConnectionDisconnected
	p.store.mu.Unlock()
	snapshot, _ := ws.conversation.get(id)
	ws.observeNativeDisconnect(context.Background(), snapshot)
	if got := conversationRecord(t, ws, id); got.CurrentTurn.State != execution.TurnCompleted {
		t.Fatalf("completed work became %s", got.CurrentTurn.State)
	}
	if ws.recoverWorkflow(context.Background(), snapshot) {
		t.Fatal("a completed turn was continued")
	}
}

func TestStopIntentIsDurableBeforeInterruptAcknowledgement(t *testing.T) {
	p := newPersistentFakeNativeProvider("native-fake", nil)
	p.interruptErr = errors.New("transport lost during interrupt")
	srv, _, _ := newRunServer(t, p, true)
	ws := srv.session()
	t.Cleanup(ws.nativeFollowers.closeAll)
	id := openConversationOK(t, srv).Conversation.ID
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "work"})
	if w.Code != http.StatusAccepted {
		t.Fatal(w.Body.String())
	}
	if err := srv.stopNativeConversationWork(context.Background(), ws, id); err == nil {
		t.Fatal("uncertain stop reported success")
	}
	r := conversationRecord(t, ws, id)
	if r.StopRequestedTurnID != r.CurrentTurn.ID {
		t.Fatal("interrupt intent was lost")
	}
	r.CurrentTurn.State = execution.TurnFailed
	r.Deliveries = nil
	if err := ws.conversationStore().Save(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	_, err := workflowrun.New(r.WorkingDir).Start(context.Background(), workflowrun.Start{Skill: "archetipo-example", Invocation: "/archetipo-example", Scope: "example", ConversationID: id, State: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := ws.conversation.get(id)
	if ws.recoverWorkflow(context.Background(), snapshot) {
		t.Fatal("stopped work was automatically restarted")
	}
}

func TestWorkflowRecoveryRespectsHumanInterruptAndUncertainDelivery(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "human-interrupt", true: "uncertain-delivery"}[uncertain], func(t *testing.T) {
			p := newPersistentFakeNativeProvider("native-fake", nil)
			srv, _, _ := newRunServer(t, p, true)
			ws := srv.session()
			t.Cleanup(ws.nativeFollowers.closeAll)
			id := openConversationOK(t, srv).Conversation.ID
			record := conversationRecord(t, ws, id)
			record.CurrentTurn = &execution.SessionTurn{ID: "stopped", State: execution.TurnInterrupted}
			if uncertain {
				record.CurrentTurn.State = execution.TurnFailed
				record.Deliveries = []execution.SessionDelivery{{TurnID: "stopped", State: execution.DeliveryUncertain}}
			}
			if err := ws.conversationStore().Save(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			_, err := workflowrun.New(record.WorkingDir).Start(context.Background(), workflowrun.Start{Skill: "archetipo-example", Invocation: "/archetipo-example", Scope: "US-001", ConversationID: id, State: json.RawMessage(`{}`)})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, _ := ws.conversation.get(id)
			if ws.recoverWorkflow(context.Background(), snapshot) {
				t.Fatal("forbidden continuation was started")
			}
			p.store.mu.Lock()
			defer p.store.mu.Unlock()
			if p.store.starts != 0 || p.store.resumes != 0 {
				t.Fatal("runtime was touched")
			}
		})
	}
}
