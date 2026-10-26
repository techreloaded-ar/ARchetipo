package web

import (
	"context"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

// Real bounded project commands, without re-executing the Go test binary as
// the CLI supervisor. The real supervisor is exercised by the harness benchmark.
type directEnvironmentRunner struct{}

func (directEnvironmentRunner) Run(ctx context.Context, root string, env []string, command verification.Command, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Argv[0], command.Argv[1:]...)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
func seedCommonEnvironment(t *testing.T, srv *Server) {
	t.Helper()
	root := srv.session().cfg.ProjectRoot
	if _, err := verification.Configure(root, []byte(`{"version":1,"probe":[{"name":"ready","argv":["go","version"],"timeout_seconds":10}],"checks":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := environmentrecipe.Publish(root, nil); err != nil {
		t.Fatal(err)
	}
	srv.session().environmentRunner = directEnvironmentRunner{}
}
func awaitEnvironmentReady(t *testing.T, srv *Server, root string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		value := readEnvironmentOperation(root)
		if value.State == "ready" {
			return
		}
		if value.State == "error" {
			t.Fatalf("environment failed: %s", value.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("environment did not become ready: %+v", readEnvironmentOperation(root))
}
func TestEnvironmentPreparationStaysOutOfDevelopmentTimelineAndReusesEvidence(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	seedCommonEnvironment(t, srv)
	root := srv.session().cfg.ProjectRoot
	status, opened, body := openConversation(t, srv)
	if status != 201 {
		t.Fatalf("open: %d %s", status, body)
	}
	before := metadataOf(t, srv, opened.Conversation.ID)
	continued := make(chan struct{}, 1)
	if err := srv.queueEnvironment(context.Background(), srv.session(), root, false, false, func(context.Context) error { continued <- struct{}{}; return nil }); err != nil {
		t.Fatal(err)
	}
	awaitEnvironmentReady(t, srv, root)
	<-continued
	after := metadataOf(t, srv, opened.Conversation.ID)
	if after.MessageCount != before.MessageCount || len(after.Turns) != len(before.Turns) {
		t.Fatal("setup polluted development conversation")
	}
	first := readEnvironmentOperation(root)
	if first.Preparation == nil || !first.Preparation.Passed {
		t.Fatal("missing preparation evidence")
	}
	if err := srv.queueEnvironment(context.Background(), srv.session(), root, false, false, nil); err != nil {
		t.Fatal(err)
	}
	awaitEnvironmentReady(t, srv, root)
	second := readEnvironmentOperation(root)
	if second.Preparation == nil || !second.Preparation.Reused || second.Preparation.ReusedReport != first.Preparation.ReportPath {
		t.Fatalf("did not retain original evidence: %+v", second)
	}
	if _, err := os.Stat(second.Preparation.ReusedReport); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRecipeUsesSeparateDurableTechnicalSession(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	srv.session().environmentRunner = directEnvironmentRunner{}
	t.Cleanup(func() { srv.session().stop(time.Second) })
	root := srv.session().cfg.ProjectRoot
	status, opened, body := openConversation(t, srv)
	if status != 201 {
		t.Fatalf("open %d: %s", status, body)
	}
	productID := opened.Conversation.ID
	done := make(chan struct{}, 1)
	if err := srv.queueEnvironment(context.Background(), srv.session(), root, false, false, func(context.Context) error { done <- struct{}{}; return nil }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var technicalID string
	for time.Now().Before(deadline) {
		value := readEnvironmentOperation(root)
		if value.ConversationID != "" {
			technicalID = value.ConversationID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if technicalID == "" || technicalID == productID {
		t.Fatal("no separate technical identity")
	}
	record := awaitRecord(t, srv, technicalID, func(r conversationlog.Record) bool {
		// The journal reserves the turn before StartTurn reaches the provider.
		// Wait for delivery as well before inspecting the captured prompt.
		provider.store.mu.Lock()
		defer provider.store.mu.Unlock()
		return r.CurrentTurn != nil && r.Work == execution.SessionTurnActive && provider.lastStart.Session.ConversationID == technicalID
	}, "technical setup turn")
	if record.Purpose != "environment" || record.Session.Native.ID == metadataOf(t, srv, productID).Session.Native.ID {
		t.Fatal("setup reused product native session")
	}
	if !strings.Contains(provider.lastStart.Message, sessionInputGuidance) {
		t.Fatal("technical session bypassed native conversation input guidance")
	}
	provider.completeCurrentTurn(t, "Serve una decisione sul requisito mancante.", execution.TurnCompleted)
	awaitRecord(t, srv, technicalID, func(r conversationlog.Record) bool { return r.Work == execution.SessionIdle }, "technical question")
	deadline = time.Now().Add(5 * time.Second)
	for readEnvironmentOperation(root).State != "waiting" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if readEnvironmentOperation(root).State != "waiting" {
		t.Fatal("a question ended environment orchestration")
	}
	response := doJSON(t, srv, "POST", conversationsRoute+"/"+technicalID+"/messages", map[string]any{"message": "Usa i dati sintetici locali."})
	if response.Code != 202 {
		t.Fatalf("technical answer: %d %s", response.Code, response.Body.String())
	}
	// Simulate only the agent's chosen configuration; preparation, reports and
	// publication use the real shared contracts.
	if _, err := verification.Configure(root, []byte(`{"version":1,"probe":[{"name":"ready","argv":["go","version"],"timeout_seconds":10}],"checks":{}}`)); err != nil {
		t.Fatal(err)
	}
	service := verification.New(root, "").ForCandidate()
	service.Runner = directEnvironmentRunner{}
	q, err := service.Qualify(context.Background(), nil)
	if err != nil || !q.Passed {
		t.Fatalf("qualification: %+v %v", q, err)
	}
	if _, err := environmentrecipe.PublishQualified(root, nil, q.BundleDigest); err != nil {
		t.Fatal(err)
	}
	provider.completeCurrentTurn(t, "Ambiente configurato.", execution.TurnCompleted)
	awaitEnvironmentReady(t, srv, root)
	<-done
	product := metadataOf(t, srv, productID)
	if product.CurrentTurn != nil || product.MessageCount != 0 {
		t.Fatalf("setup polluted product chat: %+v", product)
	}
	index := doJSON(t, srv, "GET", conversationsRoute, nil)
	if strings.Contains(index.Body.String(), technicalID) {
		t.Fatal("technical session leaked into ordinary rail")
	}
	technical := doJSON(t, srv, "GET", conversationsRoute+"/"+technicalID, nil)
	if technical.Code != 200 {
		t.Fatal("technical session not inspectable")
	}
}

func TestFailedPreparationDoesNotStartDevelopment(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	seedCommonEnvironment(t, srv)
	root := srv.session().cfg.ProjectRoot
	// A valid recipe whose actual readiness command fails.
	if _, err := verification.Configure(root, []byte(`{"version":1,"probe":[{"name":"ready","argv":["go","invalid-command"],"timeout_seconds":10}],"checks":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := environmentrecipe.Publish(root, nil); err != nil {
		t.Fatal(err)
	}
	called := make(chan struct{}, 1)
	if err := srv.queueEnvironment(context.Background(), srv.session(), root, false, false, func(context.Context) error { called <- struct{}{}; return nil }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if value := readEnvironmentOperation(root); value.State == "error" {
			select {
			case <-called:
				t.Fatal("started development after failed preparation")
			default:
			}
			if value.Preparation == nil || value.Preparation.Passed {
				t.Fatal("lost failure evidence")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("failed preparation did not settle")
}

func TestRepairDoesNotAcceptPreviousCommonRecipeAsSuccess(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	seedCommonEnvironment(t, srv)
	t.Cleanup(func() { srv.session().stop(time.Second) })
	root := srv.session().cfg.ProjectRoot
	continued := make(chan struct{}, 1)
	if err := srv.queueEnvironment(context.Background(), srv.session(), root, false, true, func(context.Context) error { continued <- struct{}{}; return nil }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var technicalID string
	for time.Now().Before(deadline) {
		technicalID = readEnvironmentOperation(root).ConversationID
		if technicalID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if technicalID == "" {
		t.Fatal("technical session did not open")
	}
	awaitRecord(t, srv, technicalID, func(r conversationlog.Record) bool {
		provider.store.mu.Lock()
		defer provider.store.mu.Unlock()
		return r.Work == execution.SessionTurnActive && provider.lastStart.Session.ConversationID == technicalID
	}, "repair turn")
	if !strings.Contains(provider.lastStart.Message, "requested_permission_profile") || !strings.Contains(provider.lastStart.Message, "capabilities") {
		t.Fatal("missing runtime context")
	}
	provider.completeCurrentTurn(t, "Impossibile pubblicare: permesso negato.", execution.TurnCompleted)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		operation := readEnvironmentOperation(root)
		if operation.State == "ready" {
			t.Fatal("old common recipe certified failed repair")
		}
		if operation.State == "waiting" && strings.Contains(operation.Error, "qualificata") {
			select {
			case <-continued:
				t.Fatal("continued development after failed repair")
			default:
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("repair did not expose missing qualification: %+v", readEnvironmentOperation(root))
}
