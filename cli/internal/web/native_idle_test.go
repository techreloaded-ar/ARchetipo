package web

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// backdateConversation rewrites when a conversation was last spoken in, so a
// test can say "this thread has been idle for an hour" without waiting one.
func backdateConversation(t *testing.T, ws *workspaceSession, id string, lastMessageAt time.Time, work execution.SessionWorkState) {
	t.Helper()
	ctx := context.Background()
	lock, err := ws.conversationStore().Lock(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	record.LastMessageAt = lastMessageAt
	record.CurrentTurn = nil
	record.Work = work
	if err := ws.conversationStore().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
}

func conversationRecord(t *testing.T, ws *workspaceSession, id string) conversationlog.Record {
	t.Helper()
	record, err := ws.conversationStore().GetMetadata(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// A thread nobody has written in for longer than the delay loses its process
// and nothing else: the record stays open, the index still lists it, and the
// next message resumes the same native session.
func TestIdleReleaseReleasesTheProcessAndKeepsTheConversationOpen(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	ws := srv.session()
	id := openConversationOK(t, srv).Conversation.ID
	now := time.Now().UTC()
	backdateConversation(t, ws, id, now.Add(-time.Hour), execution.SessionIdle)

	if released := ws.releaseIdleConversations(context.Background(), now, 10*time.Minute); released != 1 {
		t.Fatalf("released %d conversations, want the idle one", released)
	}
	record := conversationRecord(t, ws, id)
	if record.Connection != execution.ConnectionReleased {
		t.Fatalf("the runtime was not released: connection=%s", record.Connection)
	}
	if record.Archive != execution.ArchiveOpen {
		t.Fatalf("releasing archived the conversation: archive=%s", record.Archive)
	}
	provider.store.mu.Lock()
	native := provider.store.sessions[record.Session.Native.ID]
	provider.store.mu.Unlock()
	if native == nil || native.snapshot.Connection != execution.ConnectionReleased {
		t.Fatalf("the provider was not asked to release the session: %#v", native)
	}

	// Released twice releases nothing: there is no process left to end.
	if released := ws.releaseIdleConversations(context.Background(), now, 10*time.Minute); released != 0 {
		t.Fatalf("a released conversation was released again: %d", released)
	}

	// The next message resumes the native session and is accepted as before.
	w := doJSON(t, srv, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "ci sei ancora?"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("a message after the release was refused: %d %s", w.Code, w.Body.String())
	}
	if provider.store.resumes != 1 {
		t.Fatalf("the message did not resume the native session: resumes=%d", provider.store.resumes)
	}
	if got := conversationRecord(t, ws, id).Connection; got != execution.ConnectionConnected {
		t.Fatalf("after the message the runtime is %s, want it attached again", got)
	}
}

// Idle is measured, not assumed: a thread spoken in a moment ago, a turn in
// flight, a question waiting for an answer, and an archived thread are all left
// alone.
func TestIdleReleaseLeavesEverythingThatIsNotIdleAlone(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	ws := srv.session()
	now := time.Now().UTC()

	recent := openConversationOK(t, srv).Conversation.ID
	backdateConversation(t, ws, recent, now.Add(-time.Minute), execution.SessionIdle)
	working := openConversationOK(t, srv).Conversation.ID
	backdateConversation(t, ws, working, now.Add(-time.Hour), execution.SessionTurnActive)
	asking := openConversationOK(t, srv).Conversation.ID
	backdateConversation(t, ws, asking, now.Add(-time.Hour), execution.SessionWaitingInput)

	if released := ws.releaseIdleConversations(context.Background(), now, 10*time.Minute); released != 0 {
		t.Fatalf("released %d conversations, want none: recent, working and asking are not idle", released)
	}
	for _, id := range []string{recent, working, asking} {
		if got := conversationRecord(t, ws, id).Connection; got != execution.ConnectionConnected {
			t.Fatalf("%s was released: connection=%s", id, got)
		}
	}
}

func TestIdleSince(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	hourAgo := now.Add(-time.Hour)
	ended := now.Add(-time.Minute)
	held := func(mutate func(*conversationlog.Record)) conversationlog.Record {
		record := conversationlog.Record{
			OpenedAt: hourAgo, LastMessageAt: hourAgo,
			Connection: execution.ConnectionConnected, Archive: execution.ArchiveOpen, Work: execution.SessionIdle,
		}
		if mutate != nil {
			mutate(&record)
		}
		return record
	}
	cases := []struct {
		name   string
		record conversationlog.Record
		want   bool
	}{
		{"idle for an hour", held(nil), true},
		{"released already", held(func(r *conversationlog.Record) { r.Connection = execution.ConnectionReleased }), false},
		{"archived", held(func(r *conversationlog.Record) { r.Archive = execution.ArchiveArchived }), false},
		{"turn in flight", held(func(r *conversationlog.Record) { r.Work = execution.SessionTurnActive }), false},
		{"waiting for an approval", held(func(r *conversationlog.Record) { r.Work = execution.SessionWaitingApproval }), false},
		{"turn ended a minute ago", held(func(r *conversationlog.Record) {
			r.CurrentTurn = &execution.SessionTurn{State: execution.TurnCompleted, CompletedAt: &ended}
		}), false},
		{"opened and never spoken in", held(func(r *conversationlog.Record) { r.LastMessageAt = time.Time{} }), true},
	}
	for _, tc := range cases {
		if got := idleSince(tc.record, now, 10*time.Minute); got != tc.want {
			t.Errorf("%s: idleSince = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIdleReleaseTickIsBounded(t *testing.T) {
	if got := idleReleaseTick(10 * time.Second); got != 5*time.Second {
		t.Fatalf("short delay: tick=%s", got)
	}
	if got := idleReleaseTick(10 * time.Minute); got != time.Minute {
		t.Fatalf("long delay: tick=%s", got)
	}
	if got := idleReleaseTick(2 * time.Minute); got != 30*time.Second {
		t.Fatalf("middle delay: tick=%s", got)
	}
}
