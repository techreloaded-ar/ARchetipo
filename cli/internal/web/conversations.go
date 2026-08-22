package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// conversationEntryView is one past — or currently held — conversation of the
// open workspace, as the thread rail reads it.
//
// Live is derived from the holder and never from the record: FinalState is
// history, and a viewer restarted over a directory full of records would
// otherwise resurrect as "in corso" a conversation whose process is long gone.
// State travels as the record left it, with no interpretation.
type conversationEntryView struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	SpecCode      string `json:"spec_code"`
	OpenedAt      string `json:"opened_at"`
	LastMessageAt string `json:"last_message_at"`
	MessageCount  int    `json:"message_count"`
	ResumedFrom   string `json:"resumed_from"`
	State         string `json:"state"`
	Live          bool   `json:"live"`
}

// conversationsView is the index of the open workspace. Conversations is never
// nil: a workspace nobody has talked to yet answers with an empty array, so the
// rail reads "there are none" from the list itself and never from a missing key.
type conversationsView struct {
	Conversations []conversationEntryView `json:"conversations"`
}

// conversationTranscriptView is one conversation with the whole of what was
// said in it. It carries the same header fields as the entry so a client that
// opened a transcript directly — a reload on a deep link — does not have to
// have read the index first.
//
// Events is never nil and travels in the order it was recorded, which is the
// ascending order of event ids: a transcript read out of order would be a
// different conversation from the one that happened.
type conversationTranscriptView struct {
	conversationEntryView
	Events []execution.RunEvent `json:"events"`
}

// conversationEntryOf renders one record. live is decided by the caller, which
// is the only place that can see the holder.
func conversationEntryOf(record conversationlog.Record, live bool) conversationEntryView {
	return conversationEntryView{
		ID:            record.ID,
		Title:         record.Title,
		SpecCode:      record.SpecCode,
		OpenedAt:      record.OpenedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		LastMessageAt: record.LastMessageAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		MessageCount:  record.MessageCount,
		ResumedFrom:   record.ResumedFrom,
		State:         record.FinalState,
		Live:          live,
	}
}

// handleListWorkspaceConversations lists the conversations held on the open
// workspace, most recent first.
//
// It probes no runtime and starts no process, for the reason already written on
// heldConversationTarget: this is a read, and a read of an index must not cost a
// subprocess. The order is the store's own — last_message_at descending — and is
// deliberately not recomputed here, so the index a person sees cannot disagree
// with the order the records are kept in.
func (s *Server) handleListWorkspaceConversations(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	store := ws.conversationStore()
	if store == nil {
		writeError(w, iox.NewInternal("listing the conversations of the workspace", errors.New("this workspace keeps no conversation journal")))
		return
	}
	records, err := store.List(ctx)
	if err != nil {
		// A store that could not be read is an error and never an empty list:
		// "there are no conversations" and "the conversations could not be
		// read" are opposite answers, and a person who lost their history must
		// not be told they never had one.
		writeError(w, iox.NewInternal("listing the conversations of the workspace", err))
		return
	}
	snapshot, open := ws.conversation.current()
	views := make([]conversationEntryView, 0, len(records))
	for _, record := range records {
		views = append(views, conversationEntryOf(record, open && snapshot.id == record.ID))
	}
	writeJSON(w, http.StatusOK, conversationsView{Conversations: views})
}

// handleGetWorkspaceConversationTranscript serves everything that was said in
// one conversation of the open workspace.
//
// When the id is the conversation currently held, the history comes from the
// session in memory rather than from the file: the record is rewritten as the
// live conversation is read, so the file is at most one read behind, and the two
// must not be allowed to disagree under the eyes of whoever is reading them.
func (s *Server) handleGetWorkspaceConversationTranscript(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	id := strings.TrimSpace(r.PathValue("id"))
	store := ws.conversationStore()
	if store == nil {
		writeError(w, iox.NewInternal("reading the conversation "+id, errors.New("this workspace keeps no conversation journal")))
		return
	}
	record, err := store.Get(ctx, id)
	if err != nil {
		var storeErr *conversationlog.StoreError
		if errors.As(err, &storeErr) && (storeErr.Kind == conversationlog.StoreNotFound || storeErr.Kind == conversationlog.StoreInvalidID) {
			writeError(w, iox.NewNotFound(
				"the conversation "+id+" does not exist in this workspace",
				"open the list of the conversations of the workspace to see the ones it holds",
				err,
			))
			return
		}
		writeError(w, iox.NewInternal("reading the conversation "+id, err))
		return
	}
	snapshot, open := ws.conversation.current()
	live := open && snapshot.id == record.ID
	events := record.Events
	if live {
		if session, found := conversationSessionOf(snapshot); found {
			// Cursor 0 on purpose: a transcript is the whole conversation, and
			// this route has no cursor of its own to advance.
			events = session.Events(0)
		}
	}
	if events == nil {
		events = []execution.RunEvent{}
	}
	writeJSON(w, http.StatusOK, conversationTranscriptView{
		conversationEntryView: conversationEntryOf(record, live),
		Events:                events,
	})
}

// conversationStore is where the conversations of this workspace are kept. It
// is read through the journal because the journal owns the store for this
// project root, and a second store opened here could point somewhere else after
// a workspace switch.
func (ws *workspaceSession) conversationStore() *conversationlog.FileStore {
	if ws == nil || ws.journal == nil {
		return nil
	}
	return ws.journal.store
}
