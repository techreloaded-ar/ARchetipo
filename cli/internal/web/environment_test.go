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
	// The journal marks the turn active before the provider sees it (the durable
	// boundary of sendNativeConversationMessage); the fake holds a turn to
	// complete only once that delivery is confirmed.
	record := awaitRecord(t, srv, technicalID, func(r conversationlog.Record) bool {
		if r.CurrentTurn == nil || r.Work != execution.SessionTurnActive {
			return false
		}
		for _, delivery := range r.Deliveries {
			if delivery.TurnID == r.CurrentTurn.ID && delivery.State == execution.DeliveryConfirmed {
				return true
			}
		}
		return false
	}, "technical setup turn")
	if record.Purpose != "environment" || record.Session.Native.ID == metadataOf(t, srv, productID).Session.Native.ID {
		t.Fatal("setup reused product native session")
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
	if _, err := environmentrecipe.Publish(root, nil); err != nil {
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
