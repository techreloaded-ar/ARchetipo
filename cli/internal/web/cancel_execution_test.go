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
)

func TestRemovingWorkStopsItsExecutionAndAllowsAnotherAction(t *testing.T) {
	for _, gesture := range []string{"archive", "delete-conversation", "delete-spec", "cancel-run"} {
		for _, waiting := range []bool{false, true} {
			t.Run(gesture+"/waiting="+map[bool]string{false: "false", true: "true"}[waiting], func(t *testing.T) {
				srv, provider, _ := newActionSessionServer(t)
				status, started := startSpecActionIn(t, srv, "US-901", "plan", "")
				if status != http.StatusCreated {
					t.Fatalf("start = %d: %v", status, started)
				}
				id := started["id"].(string)
				thread := conversationOfExecution(t, srv, id)
				if waiting {
					provider.completeCurrentTurn(t, "Descrivi meglio il bisogno", execution.TurnCompleted)
					awaitRecordSettled(t, srv, thread)
				}
				method, path, want := http.MethodPost, conversationsRoute+"/"+thread+"/archive", http.StatusOK
				switch gesture {
				case "delete-conversation":
					method, path = http.MethodDelete, conversationsRoute+"/"+thread+"/record"
				case "delete-spec":
					method, path = http.MethodDelete, "/api/spec/US-901"
				case "cancel-run":
					path, want = "/api/execution/"+id+"/run/cancel", http.StatusAccepted
				}
				w := doJSON(t, srv, method, path, nil)
				if w.Code != want {
					t.Fatalf("stop = %d: %s", w.Code, w.Body.String())
				}
				awaitExecutionStatus(t, srv, id, execution.StatusFailed)
				provider.store.mu.Lock()
				for _, session := range provider.store.sessions {
					if session.snapshot.Work != execution.SessionIdle {
						t.Errorf("removed work still active: %s", session.snapshot.Work)
					}
				}
				provider.store.mu.Unlock()
				if gesture == "delete-conversation" {
					if !strings.Contains(w.Body.String(), `"native_session_preserved":true`) {
						t.Fatal("deletion did not preserve native transcript")
					}
				}
				code := "US-901"
				if gesture == "delete-spec" {
					code = "US-902"
				}
				status, next := startSpecActionIn(t, srv, code, "plan", "")
				if status != http.StatusCreated {
					t.Fatalf("next action blocked after stop = %d: %v", status, next)
				}
			})
		}
	}
}

func TestWorkspaceProposalReachesTheActionTurnAndItsRecord(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	provider.capabilities = append(provider.capabilities, execution.CapabilityWorkspaceSpecDraft)
	proposal := "Come visitatore voglio cercare le storie per titolo."
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/execution", map[string]any{"action": "spec-draft", "user_input": proposal})
	if w.Code != http.StatusCreated {
		t.Fatalf("start = %d: %s", w.Code, w.Body.String())
	}
	var record execution.Execution
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.UserInput != proposal {
		t.Fatalf("record lost input: %+v", record)
	}
	if len(provider.turnStarts) != 1 || !strings.Contains(provider.turnStarts[0].Message, proposal) {
		t.Fatalf("proposal did not reach the first action turn: %+v", provider.turnStarts)
	}
	thread := conversationOfExecution(t, srv, record.ID)
	if err := srv.stopNativeConversationWork(context.Background(), srv.session(), thread); err != nil {
		t.Fatal(err)
	}
}

func TestFailedInterruptPreservesTheCardAndRunningExecution(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	status, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("start = %d: %v", status, started)
	}
	provider.interruptErr = errors.New("runtime refused to interrupt")
	w := doJSON(t, srv, http.MethodDelete, "/api/spec/US-901", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete = %d: %s", w.Code, w.Body.String())
	}
	_, record := readExecution(t, srv, started["id"].(string))
	if record.Status != execution.StatusRunning {
		t.Fatalf("failed stop closed work as %s", record.Status)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/spec/US-901", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("failed stop deleted card: %d %s", w.Code, w.Body.String())
	}
}

func TestDeletingACardStopsOnlyItsBatchDispatch(t *testing.T) {
	provider := blockedProvider("batch-fake")
	srv, _, _ := newRunServer(t, provider, true)
	for _, code := range []string{"US-901", "US-902"} {
		status, body := startSpecActionIn(t, srv, code, "plan", "")
		if status != http.StatusCreated {
			t.Fatalf("start %s = %d: %v", code, status, body)
		}
		select {
		case <-provider.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("dispatch did not start")
		}
	}
	w := doJSON(t, srv, http.MethodDelete, "/api/spec/US-901", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", w.Code, w.Body.String())
	}
	for _, request := range provider.requests() {
		_, record := readExecution(t, srv, request.ExecutionID)
		want := execution.StatusRunning
		if request.SpecCode == "US-901" {
			want = execution.StatusFailed
		}
		if record.Status != want {
			t.Fatalf("%s is %s, want %s", request.SpecCode, record.Status, want)
		}
	}
	w = doJSON(t, srv, http.MethodDelete, "/api/spec/US-902", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("cleanup second dispatch = %d: %s", w.Code, w.Body.String())
	}
}

func TestCancellingAReleasedNativeActionUsesItsDurableLink(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	status, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("start = %d: %v", status, started)
	}
	id := started["id"].(string)
	thread := conversationOfExecution(t, srv, id)
	provider.completeCurrentTurn(t, "Quale proposta vuoi definire?", execution.TurnCompleted)
	awaitRecordSettled(t, srv, thread)
	w := doJSON(t, srv, http.MethodDelete, conversationsRoute+"/"+thread, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("release = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/execution/"+id+"/run/cancel", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("cancel released action = %d: %s", w.Code, w.Body.String())
	}
	awaitExecutionStatus(t, srv, id, execution.StatusFailed)
}

type detachedCancellationProvider struct{ *collaboratingProvider }

func (*detachedCancellationProvider) WorkOutlivesDispatcher() bool { return true }

func TestDetachedWorkerMustStopBeforeItsDispatchIsCancelled(t *testing.T) {
	worker := newFakeCollaborator()
	provider := &detachedCancellationProvider{&collaboratingProvider{blockedProvider("remote-fake"), worker}}
	srv, _, _ := newRunServer(t, provider, true)
	status, started := startSpecActionIn(t, srv, "US-901", "plan", "")
	if status != http.StatusCreated {
		t.Fatalf("start = %d: %v", status, started)
	}
	select {
	case <-provider.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch did not start")
	}
	id := started["id"].(string)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := srv.cancelExecution(ctx, srv.session(), id); err == nil {
		t.Fatal("a cancel acknowledgement was treated as proof the remote worker stopped")
	}
	_, record := readExecution(t, srv, id)
	if record.Status != execution.StatusRunning {
		t.Fatalf("still-active remote work closed as %s", record.Status)
	}
	worker.setSnapshot(execution.RunSnapshot{RunID: "run-9", State: execution.RunClosed})
	if err := srv.cancelExecution(context.Background(), srv.session(), id); err != nil {
		t.Fatal(err)
	}
	awaitExecutionStatus(t, srv, id, execution.StatusFailed)
}
