package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// conversationContextLimit bounds, in runes, the transcript a resumed
// conversation carries into the prompt of the new one.
//
// It is counted in runes and not in bytes for the same reason the title is: a
// conversation held in a language with accents must be cut where a reader would
// cut it, not where an encoder happens to land.
const conversationContextLimit = 20000

// conversationContextOmissionNotice is the line prefixed to a transcript that
// did not fit, so the agent reads a partial history as a partial one instead of
// believing the conversation began where the text begins.
const conversationContextOmissionNotice = "[la parte più vecchia di questa conversazione è stata omessa]"

// conversationHumanPrefix and conversationAgentPrefix are who said what in a
// rendered transcript. They are the two prefixes and there are no others,
// because the transcript carries the exchange and not the machinery around it.
const (
	conversationHumanPrefix = "tu: "
	conversationAgentPrefix = "agente: "
)

// resumeConversationReq is the body of a resume request: the message the person
// wrote in the past conversation, which is what asked for the resume in the
// first place.
type resumeConversationReq struct {
	Message string `json:"message"`
}

// transcriptOf renders a past conversation as the text a new agent is handed as
// context.
//
// It keeps only what was actually said — the messages of the person and the
// replies of the agent — and drops the machinery in between: a tool call is
// part of the timeline a reader scrolls, but handing an agent the file names
// another session happened to read would describe a search rather than a
// conversation, and it would spend the budget below on it.
//
// The cut is from the *beginning* when the whole is too long. A conversation is
// resumed for what it ended on: the tail is the part that explains why somebody
// is writing again, and the head is the part they have already acted upon.
func transcriptOf(record conversationlog.Record) string {
	lines := make([]string, 0, len(record.Events))
	for _, event := range record.Events {
		var prefix string
		switch event.Kind {
		case localrun.KindUserMessage:
			prefix = conversationHumanPrefix
		case localrun.KindText:
			prefix = conversationAgentPrefix
		default:
			continue
		}
		// The newlines inside one message are collapsed so one event is one
		// line: a line of the transcript is a turn of the exchange, and a
		// message that broke that rule would read as several.
		text := strings.Join(strings.Fields(event.Text), " ")
		if text == "" {
			continue
		}
		lines = append(lines, prefix+text)
	}
	transcript := strings.Join(lines, "\n")
	runes := []rune(transcript)
	if len(runes) <= conversationContextLimit {
		return transcript
	}
	return conversationContextOmissionNotice + "\n" + string(runes[len(runes)-conversationContextLimit:])
}

// handleResumeWorkspaceConversation takes up a past conversation by opening a
// *new* one that has been given the old one as context.
//
// Nothing of the original session is reopened: its agent process is long gone
// and its memory with it, and pretending otherwise would promise a continuity
// the provider cannot honour. What continues is the history — the new
// conversation carries the transcript in its prompt and declares, through
// resumed_from, which conversation it is taking up.
//
// The order of the checks is the one handleOpenWorkspaceConversation already
// uses, because this is a variant of that route and not a second way of opening
// a conversation.
func (s *Server) handleResumeWorkspaceConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	// First, before any body is decoded and before the provider is touched: the
	// refusal has to name the directory that disappeared, and it has to happen
	// before any process exists to be released.
	if err := ws.requireReachable(); err != nil {
		writeError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var body resumeConversationReq
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	// A resume is a message written into a past conversation, so there is no
	// resume without one: the same refusal the message route gives, for the
	// same reason.
	if strings.TrimSpace(body.Message) == "" {
		writeError(w, iox.NewInvalidInput("message is required", "send a non-empty message", nil))
		return
	}
	ctx := r.Context()
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
	live, hasLive := ws.conversation.current()
	// Resuming the conversation that is happening right now is refused rather
	// than served: that one is written to with the message route, and taking it
	// up would close it only to hand it its own history back as context.
	if hasLive && live.id == record.ID {
		writeError(w, iox.NewConflict(
			"the conversation "+record.ID+" is the one currently open for this workspace",
			"write in it directly: a conversation that is still open is continued, not resumed",
			nil,
		))
		return
	}
	if hasLive {
		// Sealed and closed before anything new is started, so the workspace is
		// never holding two live conversations, and the one being left behind is
		// written down with everything said in it and the state it ended in.
		ws.sealConversation(ctx, live)
		if err := ws.conversation.close(ctx); err != nil {
			writeError(w, iox.NewInternal("closing the conversation "+live.id, err))
			return
		}
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	if target.reason != "" {
		writeError(w, iox.NewConflict(target.reason, conversationRemedy, nil))
		return
	}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		writeError(w, err)
		return
	}
	newID, err := conversationID()
	if err != nil {
		writeError(w, iox.NewInternal("generating the conversation id", err))
		return
	}
	providerConfig := execution.CloneConfig(target.availability.providerConfig)
	openErr := target.provider.OpenConversation(ctx, execution.ConversationRequest{
		ConversationID: newID,
		ProcessActions: conversationActionsOf(tpl),
		WorkingDir:     ws.cfg.ProjectRoot,
		ProviderConfig: providerConfig,
		Context:        transcriptOf(record),
	})
	if openErr != nil {
		var configErr *execution.ConfigurationError
		if errors.As(openErr, &configErr) {
			writeError(w, iox.NewConflict(configErr.Error(), conversationRemedy, openErr))
			return
		}
		writeError(w, iox.NewInternal("opening a conversation with the "+quoted(target.availability.providerID)+" provider", openErr))
		return
	}
	// The spec of the resumed conversation is inherited: a thread taken up is
	// about whatever the thread was about, and asking the person to name it
	// again would let the same conversation change subject by being resumed.
	if err := ws.conversation.open(newID, target.availability.providerID, target.provider, target.collaborator, providerConfig, ws.cfg.ProjectRoot, time.Now().UTC(), record.SpecCode, record.ID); err != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), conversationCloseTimeout)
		defer cancel()
		_ = target.provider.CloseConversation(closeCtx, newID)
		writeError(w, iox.NewConflict(err.Error(), "read the conversation of this workspace before opening one", nil))
		return
	}
	snapshot, open := ws.conversation.current()
	journalErr := ws.journal.begin(ctx, snapshot, record.SpecCode, record.ID)
	// The message that asked for the resume is delivered to the conversation it
	// asked for, and a provider that refuses it is reported with the very same
	// vocabulary the message route uses. The conversation stays open either
	// way: it exists, and telling the caller it does not would leave a live
	// agent process nobody knows about.
	if snapshot.collaborator == nil {
		writeError(w, iox.NewConflict(
			"the conversation "+snapshot.id+" cannot be commanded from this viewer",
			"close it and resume with a provider that exposes an interactive run",
			nil,
		))
		return
	}
	if err := snapshot.collaborator.SendRunMessage(ctx, execution.RunRequest{RunID: snapshot.id, ProviderConfig: snapshot.providerConfig}, body.Message); err != nil {
		writeError(w, mapRunRefusal(err))
		return
	}
	view := s.conversationViewOf(ctx, ws, target, snapshot, open, 0)
	if journalErr != nil && view.Notice == "" {
		view.Notice = "this conversation could not be written to disk: " + journalErr.Error()
	}
	writeJSON(w, http.StatusCreated, view)
}
