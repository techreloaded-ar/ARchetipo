package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

const (
	defaultClosedConversationLimit = 20
	maxClosedConversationLimit     = 5000
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
	HasMoreClosed bool                    `json:"has_more_closed"`
	ClosedLimit   int                     `json:"closed_limit"`
	// Available, UnavailableReason and ProviderID say whether *another*
	// conversation could be opened here, and with which provider. They travel
	// with the index because the index is the only read a workspace holding
	// nothing alive still makes: with the singular route retired there is no
	// other place left to ask "can I open one", and a rail offering a button it
	// cannot honour would be promising what the workspace cannot do.
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	ProviderID        string `json:"provider_id,omitempty"`
}

func closedConversationLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("closed_limit"))
	if raw == "" {
		return defaultClosedConversationLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxClosedConversationLimit {
		return 0, iox.NewInvalidInput(
			"closed_limit must be an integer between 1 and "+strconv.Itoa(maxClosedConversationLimit),
			"use a positive page size for the closed conversations",
			err,
		)
	}
	return limit, nil
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

// handleListWorkspaceConversations lists every live conversation and a bounded
// prefix of the closed conversations of the open workspace, most recent first.
//
// It starts no process, and it probes the runtime only when the workspace holds
// nothing alive — see conversationOpenabilityOf: this route is re-read on every
// turn of the conversation poll, and a probe on that loop would fork a
// subprocess per tick. The order is the store's own — last_message_at descending — and is
// deliberately not recomputed here, so the index a person sees cannot disagree
// with the order the records are kept in.
func (s *Server) handleListWorkspaceConversations(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	closedLimit, err := closedConversationLimit(r)
	if err != nil {
		writeError(w, err)
		return
	}
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
	// The live set is read once and turned into a lookup, not consulted per
	// row: which conversations are alive does not change inside a single
	// response, and asking the holder for every record would cost a lock per
	// row for a fact that is already settled. This is AC-3 at the level of the
	// route — *all* the live ones are marked, and no record is marked because
	// it happens to be the one this process is holding.
	alive := map[string]bool{}
	for _, snapshot := range ws.conversation.list() {
		alive[snapshot.id] = true
	}
	views := make([]conversationEntryView, 0, min(len(records), closedLimit+len(alive)))
	closedCount := 0
	hasMoreClosed := false
	for _, record := range records {
		live := alive[record.ID]
		if !live {
			if closedCount >= closedLimit {
				hasMoreClosed = true
				continue
			}
			closedCount++
		}
		views = append(views, conversationEntryOf(record, live))
	}
	openability := s.conversationOpenabilityOf(ctx, ws)
	writeJSON(w, http.StatusOK, conversationsView{
		Conversations:     views,
		HasMoreClosed:     hasMoreClosed,
		ClosedLimit:       closedLimit,
		Available:         openability.available,
		UnavailableReason: openability.reason,
		ProviderID:        openability.providerID,
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

// handleDeleteWorkspaceConversation erases one past conversation of the open
// workspace: the record goes, and with it the line the rail was drawing.
//
// It is a route of its own and not the DELETE of the conversation, which
// already means "close it". The two commands must stay apart: one ends a
// session and keeps everything that was said, this one throws it away. A live
// conversation is refused for the same reason — someone who has not closed a
// thread has not decided to lose it.
func (s *Server) handleDeleteWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	id := strings.TrimSpace(r.PathValue("id"))
	if _, live := ws.conversation.get(id); live {
		writeError(w, iox.NewConflict(
			"the conversation "+id+" is still live on this workspace",
			"close it first, then delete it",
			nil,
		))
		return
	}
	// Read before erasing. It is what tells "this workspace never had that
	// conversation" — a 404 the rail can explain — from a store that could not
	// be written, it proves the journal is there before Delete is asked for it,
	// and it is the copy the undo will put back.
	record, err := s.readPastConversation(ctx, ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := ws.conversationStore().Delete(ctx, id); err != nil {
		writeError(w, iox.NewInternal("deleting the conversation "+id, err))
		return
	}
	// The whole record travels back, and not just the id it was asked about.
	// It is what makes the undo possible at all: the index the rail holds is a
	// summary — no events, no provider, no model — so a page that had only that
	// could put back a conversation stripped of everything that was said in it.
	writeJSON(w, http.StatusOK, map[string]any{"deleted": record})
}

// handleRestoreWorkspaceConversation writes back a conversation that was just
// deleted: it is the other half of the undo, and it takes the very record the
// delete answered with.
//
// It refuses to write over a conversation that is there. An undo puts back what
// was erased, and one that could overwrite would be a second way to lose a
// history — the opposite of what it is for.
func (s *Server) handleRestoreWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	id := strings.TrimSpace(r.PathValue("id"))
	var record conversationlog.Record
	if err := decodeJSON(r, &record); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(record.ID) != id {
		writeError(w, iox.NewInvalidInput(
			"the body describes the conversation "+record.ID+", and the route names "+id,
			"put a record back under its own id",
			nil,
		))
		return
	}
	store := ws.conversationStore()
	if store == nil {
		writeError(w, iox.NewInternal("putting the conversation "+id+" back", errors.New("this workspace keeps no conversation journal")))
		return
	}
	switch _, err := store.Get(ctx, id); {
	case err == nil:
		writeError(w, iox.NewConflict(
			"the conversation "+id+" is already on this workspace",
			"there is nothing to put back",
			nil,
		))
		return
	case !conversationRecordMissing(err):
		writeError(w, iox.NewInternal("putting the conversation "+id+" back", err))
		return
	}
	if err := store.Save(ctx, record); err != nil {
		writeError(w, iox.NewInternal("putting the conversation "+id+" back", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": record})
}

// conversationRecordMissing tells "this workspace keeps no such record" from
// "the store could not be read". They are opposite answers: only the first one
// is a 404 to a reader, and only the first one leaves room to write.
func conversationRecordMissing(err error) bool {
	var storeErr *conversationlog.StoreError
	return errors.As(err, &storeErr) &&
		(storeErr.Kind == conversationlog.StoreNotFound || storeErr.Kind == conversationlog.StoreInvalidID)
}
