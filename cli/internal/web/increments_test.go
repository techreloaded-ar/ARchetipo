package web

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
)

func incrementGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s %v", args, raw, err)
	}
	return strings.TrimSpace(string(raw))
}
func finishIncrementTurn(t *testing.T, srv *Server, p *persistentFakeNativeProvider, view conversationView) {
	t.Helper()
	p.store.mu.Lock()
	session := p.store.sessions[view.Session.Session.Native.ID]
	turn := session.snapshot.CurrentTurn
	session.snapshot.Work = execution.SessionIdle
	if turn != nil {
		turn.State = execution.TurnCompleted
	}
	session.events = append(session.events, execution.RunEvent{ID: int64(len(session.events) + 1), Kind: "turn_end", TurnID: turn.ID, FinalMessage: "Proposta pronta da leggere.", At: time.Now().UTC()})
	select {
	case session.notify <- struct{}{}:
	default:
	}
	p.store.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		body := doJSON(t, srv, "GET", conversationsRoute+"/"+view.Conversation.ID, nil)
		var current conversationView
		if json.Unmarshal(body.Body.Bytes(), &current) == nil && current.Increment != nil && !current.Increment.Busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("native turn did not become idle")
}
func TestIncrementUsesDedicatedNativeContextAndKeepsFreeConversationOperational(t *testing.T) {
	p := newPersistentFakeNativeProvider("native-fake", nil)
	srv, cfg, conn := newRunServer(t, p, true)
	seedCommonEnvironment(t, srv)
	incrementGit(t, cfg.ProjectRoot, "init", "-b", "main")
	incrementGit(t, cfg.ProjectRoot, "config", "user.email", "tests@example.test")
	incrementGit(t, cfg.ProjectRoot, "config", "user.name", "Tests")
	if err := os.WriteFile(filepath.Join(cfg.ProjectRoot, ".archetipo", "shared-runtime.md"), []byte("runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ProjectRoot, "hello.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	incrementGit(t, cfg.ProjectRoot, "add", "-A")
	incrementGit(t, cfg.ProjectRoot, "commit", "-m", "initial")
	free := openConversationOK(t, srv)
	response := doJSON(t, srv, "POST", "/api/increments", map[string]any{"request": "Esportare i clienti", "origin_conversation_id": free.Conversation.ID})
	if response.Code != 201 {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var view conversationView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Increment == nil || view.Session == nil {
		t.Fatalf("increment context missing: %s", response.Body.String())
	}
	r := view.Increment.Record
	t.Cleanup(func() {
		srv.session().nativeFollowers.closeAll()
		_, _ = gitForIncrementCleanup(cfg.ProjectRoot, r.Workdir)
	})
	if r.ConversationID != view.Conversation.ID || r.OriginConversationID != free.Conversation.ID || view.Session.Session.Environment.WorkingDir != r.Workdir || r.Workdir == cfg.ProjectRoot {
		t.Fatalf("wrong context: %+v %+v", r, view.Session)
	}
	freeRecord, _ := srv.session().conversationStore().GetMetadata(context.Background(), free.Conversation.ID)
	if view.Session.Session.Native.ID == freeRecord.Session.Native.ID {
		t.Fatal("increment reused the free conversation's native session")
	}
	if !strings.Contains(p.lastStart.Message, "/archetipo-increment "+r.ID) || p.lastStart.Session.Environment.WorkingDir != r.Workdir {
		t.Fatal("initial turn lost the increment context")
	}
	service := increment.New(cfg, conn)
	proposal := increment.Proposal{Intent: "Esportare i clienti in Excel."}
	if _, err := service.Propose(context.Background(), r.ID, proposal); err != nil {
		t.Fatal(err)
	}
	draftResponse := doJSON(t, srv, "GET", "/api/increments/"+r.ID, nil)
	var draft incrementView
	if err := json.Unmarshal(draftResponse.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	if draft.Status != increment.Developing || draft.Revisions[0].Approved || len(draft.Stories) != 0 {
		t.Fatalf("wrong product state: %+v", draft)
	}
	finishIncrementTurn(t, srv, p, view)
	removed := doJSON(t, srv, "POST", "/api/increments/"+r.ID+"/approve-intent", map[string]any{"revision": 1})
	if removed.Code != 409 {
		t.Fatal("retired approval is still offered", removed.Code)
	}
	untouched, err := conn.FetchBacklogItems(context.Background(), "")
	if err != nil || len(untouched) != 2 {
		t.Fatal("intent approval wrote stories to the parent")
	}
	// A separate free conversation can still execute a normal turn in the
	// original workspace, while the increment is being developed.
	plain := doJSON(t, srv, "POST", conversationsRoute+"/"+free.Conversation.ID+"/messages", map[string]any{"message": "Sviluppiamo liberamente"})
	if plain.Code != 202 {
		t.Fatalf("free chat: %d %s", plain.Code, plain.Body.String())
	}
	if p.lastStart.Message != "Sviluppiamo liberamente" || p.lastStart.Session.Environment.WorkingDir != cfg.ProjectRoot {
		t.Fatal("free development was routed into the increment")
	}
	if raw, _ := os.ReadFile(filepath.Join(cfg.ProjectRoot, "hello.txt")); string(raw) != "original" {
		t.Fatal("parent code changed")
	}
	// Process actions tied to the main connector are never offered inside the
	// increment. The native skill catalog still belongs to its own harness.
	activities := doJSON(t, srv, "GET", "/api/workspace/activities?conversation_id="+view.Conversation.ID, nil)
	var catalog conversationActivitiesView
	if err = json.Unmarshal(activities.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	for _, item := range catalog.Items {
		if item.Kind == "action" {
			t.Fatal("main-workspace action offered inside the sandbox")
		}
	}
	// A provider that silently changes cwd is refused before creating its native
	// session, and the unused sandbox is rolled back.
	p.workingDir = cfg.ProjectRoot
	refused := doJSON(t, srv, "POST", "/api/increments", map[string]any{"request": "Another increment"})
	if refused.Code != 409 {
		t.Fatalf("provider changed sandbox cwd: %d %s", refused.Code, refused.Body.String())
	}
	p.workingDir = ""
	increments, listErr := service.List()
	if listErr != nil || len(increments) != 1 {
		t.Fatalf("failed open left an orphan increment: %v %v", increments, listErr)
	}
	srv.session().stop(time.Second)
	p.store.loseRuntime()
	restoredProvider := newPersistentFakeNativeProvider("native-fake", p.store)
	registry := execution.NewRegistry()
	if err = registry.Register(restoredProvider); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.session().stop(time.Second)
	restarted.session().environmentRunner = directEnvironmentRunner{}
	resumesBefore := p.store.resumes
	restored := doJSON(t, restarted, "GET", conversationsRoute+"/"+view.Conversation.ID, nil)
	var afterRestart conversationView
	if err = json.Unmarshal(restored.Body.Bytes(), &afterRestart); err != nil {
		t.Fatal(err)
	}
	if afterRestart.Increment == nil || afterRestart.Increment.ID != r.ID || afterRestart.Session.Session.Native.ID != view.Session.Session.Native.ID || afterRestart.Session.Session.Environment.WorkingDir != r.Workdir {
		t.Fatalf("restart lost sandbox or native identity: %s", restored.Body.String())
	}
	if p.store.resumes != resumesBefore {
		t.Fatal("reading a released conversation started its native runtime")
	}
	nativeSessionsBefore := p.store.next
	resumeAction := func() {
		t.Helper()
		before := p.store.resumes
		response := doJSON(t, restarted, "POST", "/api/increments/"+r.ID+"/develop", map[string]any{})
		if response.Code != 200 {
			t.Fatalf("resume increment: %d %s", response.Code, response.Body.String())
		}
		awaitEnvironmentReady(t, restarted, r.Workdir)
		if p.store.resumes != before+1 || p.store.next != nativeSessionsBefore || len(restoredProvider.turnStarts) == 0 {
			t.Fatalf("resume did not reuse exactly one native session: resumes=%d starts=%d", p.store.resumes, p.store.next)
		}
		last := restoredProvider.lastStart
		if last.Session.Native.ID != view.Session.Session.Native.ID || last.Session.Environment.WorkingDir != r.Workdir || !strings.Contains(last.Message, "/archetipo-increment "+r.ID) {
			t.Fatalf("resume changed the increment context: %#v", last)
		}
	}
	// A restarted viewer holds the durable conversation without having an
	// in-memory Codex connection. The action must resume before checking idle.
	resumeAction()
	finishIncrementTurn(t, restarted, restoredProvider, view)
	// The same recovery also applies if the runtime drops without a viewer
	// restart, rather than exposing a raw "session is not connected" error.
	p.store.loseRuntime()
	resumeAction()
}
func gitForIncrementCleanup(root, workdir string) ([]byte, error) {
	cmd := exec.Command("git", "worktree", "remove", "--force", workdir)
	cmd.Dir = root
	return cmd.CombinedOutput()
}
