package web

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// conversationTitleLimit is how many runes of the first human message become
// the title of a conversation. It is counted in runes and not in bytes so a
// title written in a language with accents is cut where a reader would cut it.
const conversationTitleLimit = 80

// conversationTitleEllipsis marks a title that carries only the beginning of
// the message it was derived from.
const conversationTitleEllipsis = "…"

// conversationJournal is what makes a conversation survive the process that
// held it: it keeps one conversationlog.Record for the conversation currently
// open on this workspace and rewrites it on disk while people talk.
//
// It holds *one* record because a workspace holds one conversation — the same
// reason conversationState holds one — and it holds it rather than re-reading
// it because the record is written far more often than it is read.
//
// lastWrittenID is a watermark and not a flag: the reading route is polled
// every couple of seconds for as long as the conversation lives, and a journal
// that rewrote the file on every poll would rewrite an unchanged history
// hundreds of times per conversation. Only an event newer than the watermark is
// a reason to write.
type conversationJournal struct {
	store *conversationlog.FileStore

	mu sync.Mutex
	// current is the record of the open conversation, nil when no conversation
	// has been begun on this journal. It is a pointer because "there is nothing
	// being journalled" is an answer of its own, not an empty record.
	current *conversationlog.Record
	// titleFromMessage says the title was derived from a human message. Once it
	// is set the title is never recomputed: a conversation is named by how it
	// started, and a title that kept following the history would rename a thread
	// under the reader who is looking at it.
	titleFromMessage bool
	lastWrittenID    int64
}

// newConversationJournal opens the journal of one project root. The store is
// per project root because a conversation belongs to the workspace it was held
// on, and that scoping is the whole of what keeps one workspace's threads out
// of another's index.
func newConversationJournal(projectRoot string) (*conversationJournal, error) {
	store, err := conversationlog.NewFileStore(projectRoot)
	if err != nil {
		return nil, err
	}
	return &conversationJournal{store: store}, nil
}

// begin starts journalling the conversation described by snapshot and writes it
// immediately.
//
// The record is saved before anybody has said anything on purpose: a thread
// exists in the index from the moment it was opened, so a conversation that is
// opened and never used is still something a person can find again — and the
// index cannot silently lose the ones nobody spoke in.
func (j *conversationJournal) begin(ctx context.Context, snapshot conversationSnapshot, specCode, resumedFrom string) error {
	if j == nil || j.store == nil {
		return nil
	}
	openedAt := snapshot.openedAt
	record := conversationlog.Record{
		ID:         snapshot.id,
		SpecCode:   strings.TrimSpace(specCode),
		Title:      conversationTitleOf(nil, openedAt),
		WorkingDir: snapshot.workingDir,
		ProviderID: snapshot.providerID,
		OpenedAt:   openedAt,
		// A record whose last message is the zero instant would sort behind
		// every other one in an index ordered by recency, which is not what a
		// conversation opened a second ago is. Until something is said, the
		// moment it was opened is its most recent moment.
		LastMessageAt: openedAt,
		ResumedFrom:   strings.TrimSpace(resumedFrom),
		Events:        []execution.RunEvent{},
	}
	j.mu.Lock()
	j.current = &record
	j.titleFromMessage = false
	j.lastWrittenID = 0
	saved := record
	j.mu.Unlock()
	return j.store.Save(ctx, saved)
}

// record updates the journalled conversation with the whole history it is given
// and writes it, but only when that history carries something the file does not
// already hold.
//
// events must be the *entire* history of the session — Events(0) — and not the
// tail a reader asked for: the record is the conversation, not the increment,
// and a record rewritten from a cursor's tail would lose everything said before
// that cursor.
//
// live says the events come from the session that is actually holding this
// conversation. A read of a past transcript passes false and writes nothing: a
// finished conversation is history, and reading it must not rewrite it.
//
// id is the conversation the events were read from, and it is checked against
// the record the journal is currently keeping. The check is what makes the
// write safe under a race that really happens: a caller takes a snapshot, then
// reads the session outside this lock, and in between a resume can have sealed
// that conversation and begun another. Without the check those stale events
// would be written into the *new* record — overwriting its history, its title
// and its instant, and pushing the watermark past events the new conversation
// has not emitted yet, so its own history could never be written at all.
func (j *conversationJournal) record(ctx context.Context, id string, events []execution.RunEvent, live bool) error {
	if j == nil || j.store == nil || !live || len(events) == 0 {
		return nil
	}
	j.mu.Lock()
	if j.current == nil || j.current.ID != id || events[len(events)-1].ID <= j.lastWrittenID {
		j.mu.Unlock()
		return nil
	}
	j.current.Events = append([]execution.RunEvent(nil), events...)
	j.current.LastMessageAt = events[len(events)-1].At
	j.current.MessageCount = conversationMessageCount(events)
	if !j.titleFromMessage {
		if _, spoken := firstHumanMessage(events); spoken {
			j.titleFromMessage = true
		}
		j.current.Title = conversationTitleOf(events, j.current.OpenedAt)
	}
	j.lastWrittenID = events[len(events)-1].ID
	saved := *j.current
	j.mu.Unlock()
	return j.store.Save(ctx, saved)
}

// finish seals the record with the state the conversation was left in.
//
// It is idempotent, and calling it on a journal that has nothing open is not an
// error: a workspace can be stopped twice, and a conversation can be closed by
// the route and then again by the session that is ending.
func (j *conversationJournal) finish(ctx context.Context, state execution.RunState) error {
	if j == nil || j.store == nil {
		return nil
	}
	j.mu.Lock()
	if j.current == nil {
		j.mu.Unlock()
		return nil
	}
	j.current.FinalState = string(state)
	saved := *j.current
	j.mu.Unlock()
	return j.store.Save(ctx, saved)
}

// conversationTitleOf names a conversation by the first thing the person said
// in it, falling back to the moment it was opened when nobody has said anything
// yet.
//
// The fallback is dated and not generic because it is what a reader picks a
// thread by in an index of threads that were never used: "Conversazione del
// 22/08/2026 10:14" tells them which one, "Conversazione" tells them nothing.
func conversationTitleOf(events []execution.RunEvent, openedAt time.Time) string {
	if text, spoken := firstHumanMessage(events); spoken {
		return truncateRunes(text, conversationTitleLimit)
	}
	return "Conversazione del " + openedAt.Format("02/01/2006 15:04")
}

// firstHumanMessage returns the normalized text of the first message the person
// sent, and false when the history carries none.
func firstHumanMessage(events []execution.RunEvent) (string, bool) {
	for _, event := range events {
		if event.Kind != localrun.KindUserMessage {
			continue
		}
		if text := strings.Join(strings.Fields(event.Text), " "); text != "" {
			return text, true
		}
	}
	return "", false
}

// truncateRunes cuts text to limit runes, marking the cut. Shorter text is
// returned untouched, so a title that fits carries no ellipsis to explain.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + conversationTitleEllipsis
}

// conversationMessageCount counts what was actually said: the messages of the
// person and the replies of the agent. Tool calls and lifecycle events are part
// of the transcript but not of the exchange, and counting them would make a
// conversation of two sentences look like a long one.
func conversationMessageCount(events []execution.RunEvent) int {
	count := 0
	for _, event := range events {
		if event.Kind == localrun.KindUserMessage || event.Kind == localrun.KindText {
			count++
		}
	}
	return count
}

// finalConversationState is the state a conversation is sealed with when the
// viewer is the one ending it.
//
// It asks the collaborator first because a conversation whose process had
// already crashed must be recorded as crashed: the viewer closing the handle
// afterwards does not turn a crash into an orderly end. Anything else — active,
// unreadable, no collaborator at all — is sealed as closed, because it is
// closed the instant this returns.
func finalConversationState(ctx context.Context, snapshot conversationSnapshot) execution.RunState {
	if snapshot.collaborator == nil {
		return execution.RunClosed
	}
	reported, err := snapshot.collaborator.ReadRun(ctx, execution.RunRequest{RunID: snapshot.id, ProviderConfig: snapshot.providerConfig})
	if err == nil && reported.State == execution.RunCrashed {
		return execution.RunCrashed
	}
	return execution.RunClosed
}

// sealConversation writes the last word of a conversation before the holder
// lets go of it: everything said since the last read, then the final state.
//
// Both halves live here rather than at the two call sites because the close
// route and the ending session are the same moment seen twice, and a journal
// sealed one way by one of them and another way by the other would make the
// history of a conversation depend on how the viewer happened to end.
func (ws *workspaceSession) sealConversation(ctx context.Context, snapshot conversationSnapshot) {
	if ws == nil {
		return
	}
	if session, found := conversationSessionOf(snapshot); found {
		_ = ws.journal.record(ctx, snapshot.id, session.Events(0), true)
	}
	_ = ws.journal.finish(ctx, finalConversationState(ctx, snapshot))
}
