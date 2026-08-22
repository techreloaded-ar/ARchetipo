package web

import (
	"errors"
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
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
// It starts no process, and it probes the runtime only when the workspace holds
// nothing alive — see conversationOpenabilityOf: this route is re-read on every
// turn of the conversation poll, and a probe on that loop would fork a
// subprocess per tick. The order is the store's own — last_message_at descending — and is
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
	views := make([]conversationEntryView, 0, len(records))
	for _, record := range records {
		views = append(views, conversationEntryOf(record, alive[record.ID]))
	}
	openability := s.conversationOpenabilityOf(ctx, ws)
	writeJSON(w, http.StatusOK, conversationsView{
		Conversations:     views,
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
