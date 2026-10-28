package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
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
	// Work and Connection are the two facts State used to fold into one word:
	// what the harness is doing in the thread — idle, a turn in flight, a
	// question waiting for an answer — and whether a runtime is attached to
	// it. They travel apart because a reader needs them apart: "this thread
	// is waiting for you" and "this thread has no process behind it right
	// now" are different sentences, and the index draws only the first.
	Work       string `json:"work,omitempty"`
	Connection string `json:"connection,omitempty"`
	// Awaiting says the thread is stopped on a person: an action of the
	// process is still running in it and the harness is not working — it
	// asked something and is waiting for the answer. A harness that reports
	// idle after asking would otherwise leave the thread looking like any
	// other open one, and the question unanswered.
	Awaiting    bool   `json:"awaiting"`
	Live        bool   `json:"live"`
	Native      bool   `json:"native"`
	Resumable   bool   `json:"resumable"`
	Archived    bool   `json:"archived"`
	LegacyLimit string `json:"legacy_limit,omitempty"`
}

// conversationsView is the index of the open workspace. Conversations is never
// nil: a workspace nobody has talked to yet answers with an empty array, so the
// rail reads "there are none" from the list itself and never from a missing key.
type conversationsView struct {
	WorkspaceRoot string                  `json:"workspace_root"`
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

// runningExecutionIDs is the set of executions of the workspace still under
// way, read once for the whole index.
func runningExecutionIDs(ctx context.Context, ws *workspaceSession) map[string]bool {
	running := map[string]bool{}
	if ws == nil || ws.store == nil {
		return running
	}
	records, err := ws.store.List(ctx)
	if err != nil {
		return running
	}
	for _, record := range records {
		if record.Status == execution.StatusRunning {
			running[record.ID] = true
		}
	}
	return running
}

// awaitingPerson says whether a thread is stopped on a person: open, not
// working, and carrying an action of the process that has not ended.
func awaitingPerson(record conversationlog.Record, running map[string]bool) bool {
	if record.Archive == execution.ArchiveArchived || record.Work == execution.SessionTurnActive {
		return false
	}
	if record.Work == execution.SessionWaitingInput || record.Work == execution.SessionWaitingApproval {
		return true
	}
	for _, id := range record.ExecutionIDs {
		if running[id] {
			return true
		}
	}
	return false
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
func conversationEntryOf(record conversationlog.Record, live bool, running map[string]bool) conversationEntryView {
	state := record.FinalState
	if record.Native() {
		state = string(record.Work)
		if record.Connection == execution.ConnectionReleased {
			state = string(execution.ConnectionReleased)
		}
		if record.Archive == execution.ArchiveArchived {
			state = string(execution.ArchiveArchived)
		}
	}
	view := conversationEntryView{
		ID:            record.ID,
		Title:         record.Title,
		SpecCode:      record.SpecCode,
		OpenedAt:      record.OpenedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		LastMessageAt: record.LastMessageAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		MessageCount:  record.MessageCount,
		ResumedFrom:   record.ResumedFrom,
		State:         state,
		Work:          string(record.Work),
		Connection:    string(record.Connection),
		Awaiting:      awaitingPerson(record, running),
		Live:          live,
		Native:        record.Native(),
		Resumable:     record.Native() && record.Recovery == execution.RecoveryResumable,
		Archived:      record.Archive == execution.ArchiveArchived,
	}
	if !record.Native() {
		view.LegacyLimit = "no native session reference; transcript is readable but cannot be resumed natively"
	}
	return view
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
	running := runningExecutionIDs(ctx, ws)
	// Native conversation lifetime follows its archive flag, independently of
	// the runtime connection. Work and connection describe activity separately;
	// releasing a runtime must not hide an open thread or offer a new identity.
	views := make([]conversationEntryView, 0, min(len(records), closedLimit+len(alive)))
	closedCount := 0
	hasMoreClosed := false
	for _, record := range records {
		if record.Purpose == "environment" {
			continue
		}
		live := alive[record.ID] && record.Connection != execution.ConnectionReleased
		if record.Native() {
			live = record.Archive != execution.ArchiveArchived
		}
		if !live {
			if closedCount >= closedLimit {
				hasMoreClosed = true
				continue
			}
			closedCount++
		}
		views = append(views, conversationEntryOf(record, live, running))
	}
	openability := s.conversationOpenabilityOf(ctx, ws)
	writeJSON(w, http.StatusOK, conversationsView{WorkspaceRoot: ws.cfg.ProjectRoot,
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

// handleDeleteWorkspaceConversation stops and erases one conversation of the open
// workspace: the record goes, and with it the line the rail was drawing.
//
// It is a route of its own and not the DELETE of the conversation, which
// already means "release the connection". Record deletion explicitly stops
// any work first and preserves the harness's native transcript.
func (s *Server) handleDeleteWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	id := strings.TrimSpace(r.PathValue("id"))
	// Read before stopping or erasing, and return the final record for undo.
	record, err := s.readPastConversation(ctx, ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !sameOriginRemoval(w, r) {
		return
	}
	plan, allowed := s.requireRemovalChoice(w, r, ws, record, false)
	if !allowed {
		return
	}
	if r.URL.Query().Get("worktree") == "delete" {
		result, deleteErr := s.deleteIncrementEnvironment(ctx, ws, plan.Increment.Record.ID, r.URL.Query().Get("head"), false)
		if deleteErr != nil {
			writeError(w, deleteErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if err := s.stopConversationForRemoval(ctx, ws, record); err != nil {
		writeError(w, err)
		return
	}

	record, err = s.readPastConversation(ctx, ws, id)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":                  record,
		"deleted_scope":            "ARchetipo metadata and timeline only",
		"native_session_preserved": record.Native(),
	})
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
