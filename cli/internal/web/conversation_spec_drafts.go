package web

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

type specDraftReference struct {
	ConversationID string `json:"conversation_id"`
	EventID        int64  `json:"event_id"`
	Index          int    `json:"index"`
}

func (r specDraftReference) key() string { return fmt.Sprintf("%d:%d", r.EventID, r.Index) }

type conversationSpecDraftItem struct {
	execution.SpecDraftProposal
	SpecCode string   `json:"spec_code,omitempty"`
	Metadata []string `json:"metadata"`
}

type conversationSpecDraftsView struct {
	EventID      int64                       `json:"event_id"`
	Items        []conversationSpecDraftItem `json:"items"`
	ReceiptLines []string                    `json:"receipt_lines"`
	Labels       map[string]string           `json:"labels"`
}

// Read the complete history, independent of the browser's pagination cursor.
// Only assistant messages count. Revisions replace the complete proposed set;
// quoted receipts in user input and tool output can never create a card.
func proposedSpecDrafts(events []execution.RunEvent) *conversationSpecDraftsView {
	var view *conversationSpecDraftsView
	var lines []string
	seenLines := map[string]bool{}
	message, turnID, messageID := "", "", ""
	var messageEventID int64
	consider := func(text string, id int64) {
		receipt, err := execution.AcceptSpecDraftReceipt(text)
		if err != nil {
			return
		}
		for _, line := range strings.Split(text, "\n") {
			if _, err := execution.AcceptSpecDraftReceipt(line); err == nil {
				trimmed := strings.TrimSpace(line)
				if !seenLines[trimmed] {
					lines = append(lines, trimmed)
					seenLines[trimmed] = true
				}
			}
		}
		view = &conversationSpecDraftsView{EventID: id, Items: []conversationSpecDraftItem{}}
		for _, draft := range receipt.Drafts() {
			view.Items = append(view.Items, conversationSpecDraftItem{SpecDraftProposal: draft})
		}
	}
	flush := func() {
		if message != "" {
			consider(message, messageEventID)
		}
		message = ""
	}
	for _, event := range events {
		if event.Kind == localrun.KindText && event.Tool == "" {
			if turnID != event.TurnID || (messageID != "" && event.MessageID != "" && messageID != event.MessageID) {
				flush()
			}
			message += event.Text
			messageEventID = event.ID
			turnID, messageID = event.TurnID, event.MessageID
			continue
		}
		flush()
		if event.Kind == localrun.KindTurnEnd && event.FinalMessage != "" {
			consider(event.FinalMessage, event.ID)
		}
	}
	flush()
	if view != nil {
		view.ReceiptLines = lines
		view.Labels = map[string]string{
			"title": "Proposte di storie", "intro": "Rivedi le proposte. Ogni storia entra nel backlog solo quando la accetti.",
			"pending": "Proposta", "accepted": "Nel backlog", "accept": "Accetta nel backlog",
			"edit": "Modifica proposta", "revise": "Continua in chat", "open": "Apri storia", "blocked_by": "Dipende da",
		}
		for i := range view.Items {
			draft := &view.Items[i]
			draft.Metadata = specDraftMetadata(draft.SpecDraftProposal)
		}
	}
	return view
}

func (ws *workspaceSession) specDraftsView(ctx context.Context, id string, events []execution.RunEvent) *conversationSpecDraftsView {
	view := proposedSpecDrafts(events)
	if view == nil {
		return nil
	}
	record, _ := ws.conversationStore().GetMetadata(ctx, id)
	for i := range view.Items {
		item := &view.Items[i]
		item.SpecCode = record.SpecDraftAcceptances[specDraftReference{EventID: view.EventID, Index: i}.key()]
		if item.SpecCode != "" {
			if saved, err := ws.conn.ReadSpecDetail(ctx, item.SpecCode); err == nil {
				item.SpecDraftProposal = execution.SpecDraftProposal{Title: saved.Title, EpicCode: saved.Epic.Code, Priority: string(saved.Priority), Points: saved.Points, Scope: string(saved.Scope), BlockedBy: saved.BlockedBy, Body: saved.Body}
				item.Metadata = specDraftMetadata(item.SpecDraftProposal)
			}
		}
	}
	return view
}

func specDraftMetadata(draft execution.SpecDraftProposal) []string {
	epic := draft.EpicCode
	if epic == "" {
		epic = draft.EpicTitle
	}
	metadata := []string{}
	for _, value := range []string{epic, draft.Priority, draft.Scope} {
		if value != "" {
			metadata = append(metadata, value)
		}
	}
	if draft.Points > 0 {
		metadata = append(metadata, strconv.Itoa(draft.Points)+" pt")
	}
	return metadata
}

func (ws *workspaceSession) specDraftHistory(ctx context.Context, id string) ([]execution.RunEvent, error) {
	if snapshot, live := ws.conversation.get(id); live && snapshot.sessionProvider == nil {
		if session, found := conversationSessionOf(snapshot); found {
			return session.Events(0), nil
		}
	}
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		return nil, err
	}
	if !record.Native() {
		return record.Events, nil
	}
	page, err := ws.conversationStore().ReadEvents(ctx, id, 0, 0)
	return page.Events, err
}

// Return an already accepted spec on a retry, including after title edits or a
// newer proposal. Otherwise require the exact version the person reviewed.
func (ws *workspaceSession) resolveSpecDraftSource(ctx context.Context, ref *specDraftReference) (*domain.Spec, error) {
	if ref == nil {
		return nil, nil
	}
	record, err := ws.conversationStore().GetMetadata(ctx, ref.ConversationID)
	if err != nil {
		return nil, conversationNotFound(ref.ConversationID, err)
	}
	if code := record.SpecDraftAcceptances[ref.key()]; code != "" {
		spec, err := ws.conn.ReadSpecDetail(ctx, code)
		return &spec, err
	}
	events, err := ws.specDraftHistory(ctx, ref.ConversationID)
	if err != nil {
		return nil, err
	}
	view := proposedSpecDrafts(events)
	if view == nil || view.EventID != ref.EventID || ref.Index < 0 || ref.Index >= len(view.Items) {
		return nil, iox.NewConflict("this story proposal has been superseded", "read the latest proposal before accepting it", nil)
	}
	if record.Native() && record.Work != execution.SessionIdle {
		return nil, iox.NewConflict("the agent is still revising this conversation", "wait for the turn to finish before accepting the proposal", nil)
	}
	return nil, nil
}

func (ws *workspaceSession) rememberSpecDraftAcceptance(ctx context.Context, ref *specDraftReference, code string) error {
	if ref == nil {
		return nil
	}
	// The connector has already committed. A client disconnect must not lose
	// the link needed to make its next confirmation idempotent.
	ctx = context.WithoutCancel(ctx)
	store := ws.conversationStore()
	lock, err := store.Lock(ctx, ref.ConversationID)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := store.GetMetadata(ctx, ref.ConversationID)
	if err != nil {
		return err
	}
	if record.SpecDraftAcceptances == nil {
		record.SpecDraftAcceptances = map[string]string{}
	}
	record.SpecDraftAcceptances[ref.key()] = code
	// Legacy conversations are also cached by the journal. Update that copy
	// so its next transcript write retains the person's acceptance.
	ws.journal.mu.Lock()
	defer ws.journal.mu.Unlock()
	if entry := ws.journal.entries[ref.ConversationID]; entry != nil {
		entry.record.SpecDraftAcceptances = record.SpecDraftAcceptances
	}
	return store.Save(ctx, record)
}
