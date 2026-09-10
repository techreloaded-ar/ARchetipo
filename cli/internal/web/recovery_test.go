package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// The tests in this file are the operating cases that separate a demonstration
// from a session somebody works in for days: a viewer that is restarted, one
// that is killed in the middle of a submission, two of them opened on the same
// workspace, a directory that has been removed under a live session.
//
// They are deterministic on purpose. Every one of them injects the fault at a
// named boundary and then waits for a *condition*, never for a duration: a
// recovery that stops working would fail them rather than make them flaky.

// crashViewer is View disappearing without the chance to clean up anything.
//
// The followers stop where they are, no session is released, no record is
// sealed and no lock is unlocked by the code that took it — the runtime holds
// go because the process that owned them is, in the story this tells, gone, and
// a pid that no longer exists is exactly what the next viewer recovers a hold
// from. What is left on disk afterwards is what a kill -9 leaves.
func crashViewer(srv *Server, store *persistentFakeNativeStore) {
	ws := srv.session()
	ws.nativeFollowers.closeAll()
	ws.nativeRuntime.dropAll()
	if store != nil {
		store.loseRuntime()
	}
}

// restartViewer is the next View process over the same workspace and the same
// durable state.
//
// The provider is rebuilt around the same native sessions, because that is what
// survives: the harness processes died with the previous viewer, and the only
// way back into one of those sessions is to resume it.
func restartViewer(t *testing.T, cfg config.Config, conn connector.Connector, store *persistentFakeNativeStore, providerID string) (*Server, *persistentFakeNativeProvider) {
	t.Helper()
	provider := newPersistentFakeNativeProvider(providerID, store)
	provider.capabilities = []execution.Capability{
		execution.CapabilitySpecPlan,
		execution.CapabilitySpecImplement,
		execution.CapabilitySpecReview,
	}
	registry := execution.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.session().stop(2 * time.Second) })
	return srv, provider
}

// metadataOf reads one conversation record straight from disk, which is the
// only oracle that survives the process that wrote it.
func metadataOf(t *testing.T, srv *Server, id string) conversationlog.Record {
	t.Helper()
	record, err := srv.session().conversationStore().GetMetadata(context.Background(), id)
	if err != nil {
		t.Fatalf("reading the record of %s: %v", id, err)
	}
	return record
}

// awaitRecord waits for the durable record to satisfy a condition. It is the
// bounded wait every reconciliation in this file is observed through: the work
// is done by a follower or by a restart, and both are asynchronous by design.
func awaitRecord(t *testing.T, srv *Server, id string, want func(conversationlog.Record) bool, what string) conversationlog.Record {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var record conversationlog.Record
	for time.Now().Before(deadline) {
		record = metadataOf(t, srv, id)
		if want(record) {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the record of %s never %s: work=%s connection=%s turn=%#v", id, what, record.Work, record.Connection, record.CurrentTurn)
	return record
}

// awaitEvents waits for the journal of a conversation to hold at least n
// events. It is the oracle for "the agent has finished saying it", and the
// record is not: the terminal state of a turn can be written by the route that
// reads the session, a moment before the follower has appended the text of that
// turn.
func awaitEvents(t *testing.T, srv *Server, id string, n int) {
	t.Helper()
	awaitRecord(t, srv, id, func(conversationlog.Record) bool {
		page, err := srv.session().conversationStore().ReadEvents(context.Background(), id, 0, 0)
		return err == nil && len(page.Events) >= n
	}, fmt.Sprintf("held %d events", n))
}

// ageConversation is the passage of time, without waiting for it: the record is
// rewritten as one that was last spoken in a month ago. It is how "days of
// inactivity" is proved without a clock the test does not control.
func ageConversation(t *testing.T, srv *Server, id string, age time.Duration) {
	t.Helper()
	store := srv.session().conversationStore()
	record := metadataOf(t, srv, id)
	record.OpenedAt = record.OpenedAt.Add(-age)
	record.LastMessageAt = time.Now().UTC().Add(-age)
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

// TestAConversationIdleForDaysIsResumedByTheNextViewer is the plain case the
// whole feature exists for: nobody has written in a month, the viewer has been
// restarted since, and the thread takes a turn on the same native session
// without a new one being invented for it.
func TestAConversationIdleForDaysIsResumedByTheNextViewer(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	cfg, conn := srv.session().cfg, srv.session().conn
	id := openConversationOK(t, srv).Conversation.ID
	if status, _, body := sendConversationMessage(t, srv, id, "prima del silenzio"); status != http.StatusAccepted {
		t.Fatalf("first message = %d: %s", status, body)
	}
	provider.completeCurrentTurn(t, "detto", execution.TurnCompleted)
	awaitEvents(t, srv, id, 2)
	nativeID := metadataOf(t, srv, id).Session.Native.ID
	ageConversation(t, srv, id, 30*24*time.Hour)

	crashViewer(srv, provider.store)
	restarted, restartedProvider := restartViewer(t, cfg, conn, provider.store, provider.ID())

	// Nothing expired: the thread is still open, still native, still resumable.
	record := metadataOf(t, restarted, id)
	if record.Archive != execution.ArchiveOpen || !record.Native() || record.Recovery != execution.RecoveryResumable {
		t.Fatalf("a month of silence changed the conversation: archive=%s native=%v recovery=%s", record.Archive, record.Native(), record.Recovery)
	}
	if status, _, body := sendConversationMessage(t, restarted, id, "dopo un mese"); status != http.StatusAccepted {
		t.Fatalf("message after a month and a restart = %d: %s", status, body)
	}
	after := metadataOf(t, restarted, id)
	if after.Session.Native.ID != nativeID {
		t.Fatalf("the resumed conversation changed native session: %s then %s", nativeID, after.Session.Native.ID)
	}
	if restartedProvider.store.resumes == 0 {
		t.Fatal("the session was written to without ever being resumed")
	}
	// The history is the same history: the new turn is appended to it, and the
	// events of the old one are still numbered from one.
	status, page, body := readConversation(t, restarted, id, 0)
	if status != http.StatusOK || len(page.Events) < 3 || page.Events[0].ID != 1 {
		t.Fatalf("the resumed thread lost its history: %d, %s", status, body)
	}
}

// TestARestartWhileAnActionWasRunningClosesItInsteadOfLeavingItRunning is the
// crash that matters most: the viewer went away while an action of the process
// was in flight. The conversation must survive it and the execution must not.
func TestARestartWhileAnActionWasRunningClosesItInsteadOfLeavingItRunning(t *testing.T) {
	srv, provider, conn := newActionSessionServer(t)
	cfg := srv.session().cfg
	persistPlanFor(t, conn, "US-901")
	status, started := startSpecActionIn(t, srv, "US-901", "implement", "")
	if status != http.StatusCreated {
		t.Fatalf("starting the action = %d: %v", status, started)
	}
	executionID, _ := started["id"].(string)
	conversationID := conversationOfExecution(t, srv, executionID)
	if _, record := readExecution(t, srv, executionID); record.Status != execution.StatusRunning {
		t.Fatalf("the action is %s before the crash, want RUNNING", record.Status)
	}

	crashViewer(srv, provider.store)
	restarted, _ := restartViewer(t, cfg, conn, provider.store, provider.ID())

	closed := awaitExecutionStatus(t, restarted, executionID, execution.StatusFailed)
	if closed.Error == nil || !strings.Contains(closed.Error.Message, "restarted") {
		t.Fatalf("the failure does not say the viewer was restarted: %#v", closed.Error)
	}
	// The conversation is not the execution: it survived, it is readable and it
	// is writable, and its turn is recorded as the interrupted work it was.
	record := metadataOf(t, restarted, conversationID)
	if record.Work != execution.SessionIdle || record.CurrentTurn == nil || record.CurrentTurn.State != execution.TurnFailed {
		t.Fatalf("the interrupted turn was not reconciled: work=%s turn=%#v", record.Work, record.CurrentTurn)
	}
	if status, _, body := sendConversationMessage(t, restarted, conversationID, "riprendiamo"); status != http.StatusAccepted {
		t.Fatalf("the thread did not survive its action: %d %s", status, body)
	}
}

// TestARestartWithoutTheOriginalProviderStillClosesTheAction is the lost link.
// The provider that held the session is not registered any more, so nothing can
// be resumed — which is exactly the case where an execution would otherwise
// stay RUNNING for ever, with nobody left able to close it.
func TestARestartWithoutTheOriginalProviderStillClosesTheAction(t *testing.T) {
	srv, provider, conn := newActionSessionServer(t)
	cfg := srv.session().cfg
	persistPlanFor(t, conn, "US-901")
	status, started := startSpecActionIn(t, srv, "US-901", "implement", "")
	if status != http.StatusCreated {
		t.Fatalf("starting the action = %d: %v", status, started)
	}
	executionID, _ := started["id"].(string)
	conversationID := conversationOfExecution(t, srv, executionID)

	crashViewer(srv, provider.store)
	// A viewer that knows a provider by another name: the session's own
	// provider is simply not there any more.
	restarted, _ := restartViewer(t, cfg, conn, nil, "another-provider")

	closed := awaitExecutionStatus(t, restarted, executionID, execution.StatusFailed)
	if closed.Error == nil || strings.TrimSpace(closed.Error.Message) == "" {
		t.Fatal("the action was closed without saying why")
	}
	// Nothing was deleted and nothing was silently moved onto the provider that
	// happens to be available now.
	record := metadataOf(t, restarted, conversationID)
	if !record.Native() || record.Session.ProviderID != provider.ID() {
		t.Fatalf("the conversation changed provider: %#v", record.Session)
	}
	if _, live := restarted.session().conversation.get(conversationID); live {
		t.Fatal("a conversation whose provider is gone is being held as if it could be commanded")
	}
	status, _, body := readConversation(t, restarted, conversationID, 0)
	if status != http.StatusOK {
		t.Fatalf("the history of an unrecoverable conversation is not readable: %d %s", status, body)
	}
	w := doJSON(t, restarted, http.MethodPost, conversationsRoute+"/"+conversationID+"/resume", map[string]any{"message": "riprendi"})
	if w.Code != http.StatusConflict || !strings.Contains(refusalMessage(t, w.Body.String()), provider.ID()) {
		t.Fatalf("the refusal does not name the provider that is missing: %d %s", w.Code, w.Body.String())
	}
}

// TestASecondViewerDoesNotTakeTheNativeSessionOfTheFirst is two View processes
// on one workspace. The second must not start a runtime of its own on a session
// the first is holding: two harness processes on one native session write the
// same transcript.
func TestASecondViewerDoesNotTakeTheNativeSessionOfTheFirst(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	cfg, conn := srv.session().cfg, srv.session().conn
	id := openConversationOK(t, srv).Conversation.ID
	if status, _, body := sendConversationMessage(t, srv, id, "il primo viewer sta lavorando"); status != http.StatusAccepted {
		t.Fatalf("first message = %d: %s", status, body)
	}
	provider.completeCurrentTurn(t, "fatto", execution.TurnCompleted)
	awaitEvents(t, srv, id, 2)

	// A second viewer, started while the first is alive and holding.
	second, secondProvider := restartViewer(t, cfg, conn, provider.store, provider.ID())
	if _, live := second.session().conversation.get(id); live {
		t.Fatal("the second viewer took a session the first one owns")
	}
	starts := provider.store.starts
	w := doJSON(t, second, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "dal secondo viewer"})
	if w.Code != http.StatusConflict || !strings.Contains(refusalMessage(t, w.Body.String()), "another View process") {
		t.Fatalf("the second viewer was not told who holds the thread: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, second, http.MethodPost, conversationsRoute+"/"+id+"/resume", map[string]any{"message": "dal secondo viewer"})
	if w.Code != http.StatusConflict || !strings.Contains(refusalMessage(t, w.Body.String()), "another View process") {
		t.Fatalf("the second viewer resumed a held session: %d %s", w.Code, w.Body.String())
	}
	if provider.store.starts != starts || secondProvider.store.resumes != 0 {
		t.Fatalf("a second runtime was started on a held session: starts=%d resumes=%d", provider.store.starts, secondProvider.store.resumes)
	}
	// Reading is not owning: the whole history is there, from the first event,
	// and the second viewer is not shown an empty thread.
	status, page, body := readConversation(t, second, id, 0)
	if status != http.StatusOK || len(page.Events) < 2 || page.Events[0].ID != 1 {
		t.Fatalf("the second viewer sees a reset thread: %d %s", status, body)
	}
	// The first viewer is unaffected and still owns its session.
	if status, _, body := sendConversationMessage(t, srv, id, "e il primo continua"); status != http.StatusAccepted {
		t.Fatalf("the holder lost its own session: %d %s", status, body)
	}
	// The journal holds one copy of each message and no duplicate of any.
	_, page, _ = readConversation(t, srv, id, 0)
	seen := map[int64]int{}
	for _, event := range page.Events {
		seen[event.ID]++
	}
	for eventID, count := range seen {
		if count != 1 {
			t.Fatalf("event %d appears %d times", eventID, count)
		}
	}
}

// TestTheHoldOfADeadViewerIsRecovered is the other half of the rule above: an
// ownership that outlived its owner must not lock a conversation out for ever.
func TestTheHoldOfADeadViewerIsRecovered(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	cfg, conn := srv.session().cfg, srv.session().conn
	id := openConversationOK(t, srv).Conversation.ID
	// A hold whose owner is a pid that cannot exist: the shape a crashed viewer
	// leaves behind, without the test having to kill a real process.
	crashViewer(srv, provider.store)
	lockPath := filepath.Join(cfg.ProjectRoot, ".archetipo", "conversations", runtimeLockID(id)+".lock")
	if err := os.MkdirAll(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, "owner"), []byte("4194304\n"+time.Now().UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
		t.Fatal(err)
	}

	restarted, _ := restartViewer(t, cfg, conn, provider.store, provider.ID())
	if _, live := restarted.session().conversation.get(id); !live {
		t.Fatal("the next viewer did not recover a hold left by a dead one")
	}
	if status, _, body := sendConversationMessage(t, restarted, id, "dopo il crash"); status != http.StatusAccepted {
		t.Fatalf("the recovered conversation is not writable: %d %s", status, body)
	}
}

// TestACrashBetweenSubmissionAndConfirmationIsSettledByTheSession is the crash
// boundary a durable record cannot see past. The command was written down as
// uncertain and then the viewer died; on the way back the session itself is the
// witness, and a turn that left no trace anywhere is one the harness never
// started.
func TestACrashBetweenSubmissionAndConfirmationIsSettledByTheSession(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	cfg, conn := srv.session().cfg, srv.session().conn
	id := openConversationOK(t, srv).Conversation.ID
	provider.failStart = true
	if w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "forse arrivato"}); w.Code == http.StatusAccepted {
		t.Fatalf("a failed submission was reported successful: %s", w.Body.String())
	}
	record := metadataOf(t, srv, id)
	if len(record.Deliveries) != 1 || record.Deliveries[0].State != execution.DeliveryUncertain {
		t.Fatalf("the crash boundary was not recorded: %#v", record.Deliveries)
	}
	// While this process still believes the turn may be in flight the boundary
	// holds and nothing is retried.
	if w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "ritento"}); w.Code != http.StatusConflict {
		t.Fatalf("an uncertain command was retried before anybody reconciled it: %d %s", w.Code, w.Body.String())
	}

	crashViewer(srv, provider.store)
	restarted, restartedProvider := restartViewer(t, cfg, conn, provider.store, provider.ID())
	restartedProvider.failStart = false

	// The restart declares the turn over, and only then does the session settle
	// the delivery: no event of that turn was ever written, so it never landed.
	reconciled := metadataOf(t, restarted, id)
	if reconciled.CurrentTurn == nil || reconciled.CurrentTurn.State != execution.TurnFailed {
		t.Fatalf("the turn of a crashed submission was left running: %#v", reconciled.CurrentTurn)
	}
	status, _, body := sendConversationMessage(t, restarted, id, "scrivo di nuovo")
	if status != http.StatusAccepted {
		t.Fatalf("the thread stayed shut after a crashed submission: %d %s", status, body)
	}
	settled := metadataOf(t, restarted, id)
	for _, delivery := range settled.Deliveries {
		if delivery.State == execution.DeliveryUncertain {
			t.Fatalf("an uncertain delivery survived its own reconciliation: %#v", settled.Deliveries)
		}
	}
	// The message that was never delivered was never delivered twice either:
	// the timeline holds the second one and not the first.
	_, page, _ := readConversation(t, restarted, id, 0)
	for _, event := range page.Events {
		if strings.Contains(event.Text, "forse arrivato") {
			t.Fatalf("a submission nobody confirmed was replayed: %#v", event)
		}
	}
}

// TestACrashDuringTheAppendLosesNoEventAndDuplicatesNone injects the fault in
// the middle of the journal write, which is the one place where a duplicate
// could be created silently.
func TestACrashDuringTheAppendLosesNoEventAndDuplicatesNone(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	cfg, conn := srv.session().cfg, srv.session().conn
	id := openConversationOK(t, srv).Conversation.ID
	if status, _, body := sendConversationMessage(t, srv, id, "mentre parla"); status != http.StatusAccepted {
		t.Fatalf("message = %d: %s", status, body)
	}
	provider.store.mu.Lock()
	var native *persistentFakeNativeSession
	for _, candidate := range provider.store.sessions {
		native = candidate
	}
	for len(native.events) < 40 {
		native.events = append(native.events, execution.RunEvent{ID: int64(len(native.events) + 1), Kind: "text", Text: "prima del crash", At: time.Now().UTC()})
	}
	select {
	case native.notify <- struct{}{}:
	default:
	}
	provider.store.mu.Unlock()
	awaitEvents(t, srv, id, 20)

	crashViewer(srv, provider.store)
	// The harness said more while nobody was listening.
	provider.store.mu.Lock()
	for len(native.events) < 60 {
		native.events = append(native.events, execution.RunEvent{ID: int64(len(native.events) + 1), Kind: "text", Text: "dopo il crash", At: time.Now().UTC()})
	}
	provider.store.mu.Unlock()

	restarted, _ := restartViewer(t, cfg, conn, provider.store, provider.ID())
	// A resume is what reattaches the stream, and the follower picks up from the
	// last event it had written rather than from the beginning.
	if status, _, body := sendConversationMessage(t, restarted, id, "riprendo"); status != http.StatusAccepted {
		t.Fatalf("message after the crash = %d: %s", status, body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		page, err := restarted.session().conversationStore().ReadEvents(context.Background(), id, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Events) >= 60 {
			for index, event := range page.Events {
				if event.ID != int64(index+1) {
					t.Fatalf("the journal is not a contiguous sequence: event %d at position %d", event.ID, index)
				}
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the events said while nobody was listening never arrived: %d", len(page.Events))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestASessionWhoseDirectoryIsGoneIsRefusedByNameAndKept is a worktree that was
// removed under a live conversation. The session's directory is frozen when it
// is created, so the only honest answers are to name the directory or to run
// the work somewhere nobody chose.
func TestASessionWhoseDirectoryIsGoneIsRefusedByNameAndKept(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	root := srv.session().cfg.ProjectRoot
	worktree := filepath.Join(root, ".worktrees", "US-901")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	provider.workingDir = worktree
	id := openConversationOK(t, srv).Conversation.ID
	if status, _, body := sendConversationMessage(t, srv, id, "dentro il worktree"); status != http.StatusAccepted {
		t.Fatalf("message in the worktree = %d: %s", status, body)
	}
	provider.completeCurrentTurn(t, "fatto", execution.TurnCompleted)
	awaitEvents(t, srv, id, 2)
	before := metadataOf(t, srv, id)
	if before.Session.Environment.WorkingDir != worktree {
		t.Fatalf("the session did not run in the worktree: %q", before.Session.Environment.WorkingDir)
	}

	if err := os.RemoveAll(filepath.Join(root, ".worktrees")); err != nil {
		t.Fatal(err)
	}
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "e adesso?"})
	if w.Code != http.StatusConflict || !strings.Contains(refusalMessage(t, w.Body.String()), worktree) {
		t.Fatalf("the refusal does not name the directory that disappeared: %d %s", w.Code, w.Body.String())
	}
	// Nothing was moved and nothing was thrown away: the record still points at
	// the directory that has to come back.
	after := metadataOf(t, srv, id)
	if after.Session.Environment.WorkingDir != worktree {
		t.Fatalf("the session was silently repointed to %q", after.Session.Environment.WorkingDir)
	}
	if status, page, body := readConversation(t, srv, id, 0); status != http.StatusOK || len(page.Events) == 0 {
		t.Fatalf("the history of a conversation with no directory is not readable: %d %s", status, body)
	}
	// Restoring the directory is the whole of the repair.
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if status, _, body := sendConversationMessage(t, srv, id, "il worktree è tornato"); status != http.StatusAccepted {
		t.Fatalf("the conversation did not come back with its directory: %d %s", status, body)
	}
}

// TestANativeSessionThatCannotBeResumedDeclaresTheLimit is the harness having
// forgotten the session. A new one is not invented: a conversation whose native
// context is gone is a conversation whose native context is gone, and saying so
// is the only answer that does not silently replace it.
func TestANativeSessionThatCannotBeResumedDeclaresTheLimit(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	cfg, conn := srv.session().cfg, srv.session().conn
	id := openConversationOK(t, srv).Conversation.ID
	if status, _, body := sendConversationMessage(t, srv, id, "prima dell'oblio"); status != http.StatusAccepted {
		t.Fatalf("message = %d: %s", status, body)
	}
	provider.completeCurrentTurn(t, "detto", execution.TurnCompleted)
	awaitEvents(t, srv, id, 2)
	nativeID := metadataOf(t, srv, id).Session.Native.ID

	crashViewer(srv, provider.store)
	// A harness that no longer knows the session: nothing to read, nothing to
	// resume.
	restarted, restartedProvider := restartViewer(t, cfg, conn, nil, provider.ID())

	w := doJSON(t, restarted, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "ci sei ancora?"})
	if w.Code == http.StatusAccepted {
		t.Fatalf("a forgotten session accepted a turn: %s", w.Body.String())
	}
	if restartedProvider.store.starts != 0 {
		t.Fatalf("a new native session was started in place of the lost one: starts=%d", restartedProvider.store.starts)
	}
	record := metadataOf(t, restarted, id)
	if record.Session.Native.ID != nativeID {
		t.Fatalf("the native reference was replaced: %s then %s", nativeID, record.Session.Native.ID)
	}
	if record.Connection == execution.ConnectionConnected {
		t.Fatalf("a session nobody can reach is reported connected: %s", record.Connection)
	}
	if status, page, body := readConversation(t, restarted, id, 0); status != http.StatusOK || len(page.Events) == 0 {
		t.Fatalf("the transcript of an unrecoverable session was lost: %d %s", status, body)
	}
}

// TestTheConversationAPICarriesNoCredential checks the other direction of the
// same boundary: the new metadata and the new routes say plenty about the work
// and nothing about how the harness authenticates.
func TestTheConversationAPICarriesNoCredential(t *testing.T) {
	const secret = "sk-live-ARCHETIPO-NEVER-PERSIST-ME"
	t.Setenv("ARCHETIPO_TEST_PROVIDER_TOKEN", secret)
	srv, provider, _ := newActionSessionServer(t)
	// The configuration names the variable that holds the credential, which is
	// exactly what a provider is allowed to persist, and never its value.
	provider.providerConfig = map[string]any{"token_env": "ARCHETIPO_TEST_PROVIDER_TOKEN", "model": "fake-large"}
	id := openConversationOK(t, srv).Conversation.ID
	if status, _, body := sendConversationMessage(t, srv, id, "lavora"); status != http.StatusAccepted {
		t.Fatalf("message = %d: %s", status, body)
	}
	provider.completeCurrentTurn(t, "fatto", execution.TurnCompleted)
	awaitEvents(t, srv, id, 2)

	surface := []string{
		conversationsRoute,
		conversationsRoute + "/" + id,
		conversationsRoute + "/" + id + "/model-choice",
	}
	for _, path := range surface {
		w := doJSON(t, srv, http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("GET %s carries the credential", path)
		}
		if strings.Contains(w.Body.String(), "ARCHETIPO_TEST_PROVIDER_TOKEN=") {
			t.Fatalf("GET %s carries an environment assignment", path)
		}
	}
	// And the work is still visible: a payload that hid everything would pass
	// the check above for the wrong reason.
	_, page, body := readConversation(t, srv, id, 0)
	if len(page.Events) < 2 || !strings.Contains(body, "lavora") || !strings.Contains(body, "fatto") {
		t.Fatalf("the useful work events are not visible: %s", body)
	}
	// Nothing on disk either: the record is what a later process reads back.
	for _, name := range []string{id + ".json", id + ".events.jsonl"} {
		path := filepath.Join(srv.session().cfg.ProjectRoot, ".archetipo", "conversations", name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(content), secret) {
			t.Fatalf("%s holds the credential", name)
		}
	}
}
