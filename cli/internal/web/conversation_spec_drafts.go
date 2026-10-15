package web

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
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
	TargetSpec     string `json:"target_spec,omitempty"`
	BaseHash       string `json:"base_hash,omitempty"`
}

func (r specDraftReference) key() string { return fmt.Sprintf("%d:%d", r.EventID, r.Index) }

type conversationSpecDraftDestination struct {
	ID          string                       `json:"id"`
	Label       string                       `json:"label"`
	AcceptLabel string                       `json:"accept_label"`
	StateLabel  string                       `json:"state_label"`
	BaseHash    string                       `json:"base_hash,omitempty"`
	Previous    *execution.SpecDraftProposal `json:"previous,omitempty"`
	Changes     []string                     `json:"changes,omitempty"`
}
type conversationSpecDraftItem struct {
	execution.SpecDraftProposal
	SpecCode          string                             `json:"spec_code,omitempty"`
	Metadata          []string                           `json:"metadata"`
	DestinationID     string                             `json:"destination_id,omitempty"`
	Destinations      []conversationSpecDraftDestination `json:"destinations,omitempty"`
	UnavailableReason string                             `json:"unavailable_reason,omitempty"`
	ReadOnly          bool                               `json:"read_only,omitempty"`
}
type conversationSpecDraftVersion struct {
	EventID int64                       `json:"event_id"`
	Label   string                      `json:"label"`
	Current bool                        `json:"current"`
	Items   []conversationSpecDraftItem `json:"items"`
}
type conversationSpecDraftsView struct {
	EventID      int64                          `json:"event_id"`
	Items        []conversationSpecDraftItem    `json:"items"`
	ReceiptLines []string                       `json:"receipt_lines"`
	Labels       map[string]string              `json:"labels"`
	Versions     []conversationSpecDraftVersion `json:"versions"`
	Summary      string                         `json:"summary"`
}

// Receipts stay at their own point in the discourse. Streamed text and the
// authoritative end-of-turn message are two observations of the same version.
func proposedSpecDrafts(events []execution.RunEvent) *conversationSpecDraftsView {
	view := &conversationSpecDraftsView{}
	seenLines := map[string]bool{}
	message, turnID, messageID, signature, signatureTurn := "", "", "", "", ""
	var messageEventID int64
	consider := func(text string, id int64, turn string) {
		receipt, err := execution.AcceptSpecDraftReceipt(text)
		if err != nil {
			return
		}
		for _, line := range strings.Split(text, "\n") {
			if _, err := execution.AcceptSpecDraftReceipt(line); err == nil {
				trimmed := strings.TrimSpace(line)
				if !seenLines[trimmed] {
					view.ReceiptLines = append(view.ReceiptLines, trimmed)
					seenLines[trimmed] = true
				}
			}
		}
		drafts := receipt.Drafts()
		raw, _ := json.Marshal(drafts)
		version := conversationSpecDraftVersion{EventID: id, Items: []conversationSpecDraftItem{}}
		for _, draft := range drafts {
			version.Items = append(version.Items, conversationSpecDraftItem{SpecDraftProposal: draft, Metadata: specDraftMetadata(draft)})
		}
		if len(view.Versions) > 0 && turn != "" && signature == string(raw) && signatureTurn == turn {
			view.Versions[len(view.Versions)-1] = version
		} else {
			view.Versions = append(view.Versions, version)
		}
		signature, signatureTurn = string(raw), turn
	}
	flush := func() {
		if message != "" {
			consider(message, messageEventID, turnID)
		}
		message = ""
	}
	for _, event := range events {
		if event.Kind == localrun.KindText && event.Tool == "" {
			if turnID != event.TurnID || (messageID != "" && event.MessageID != "" && messageID != event.MessageID) {
				flush()
			}
			message += event.Text
			messageEventID, turnID, messageID = event.ID, event.TurnID, event.MessageID
			continue
		}
		flush()
		if event.Kind == localrun.KindTurnEnd && event.FinalMessage != "" {
			consider(event.FinalMessage, event.ID, event.TurnID)
		}
	}
	flush()
	if len(view.Versions) == 0 {
		return nil
	}
	for i := range view.Versions {
		version := &view.Versions[i]
		version.Current = i == len(view.Versions)-1
		version.Label = fmt.Sprintf("Versione %d · %s", i+1, specDraftCount(len(version.Items), "storia", "storie"))
		if version.Current {
			version.Label += " · attuale"
		}
		for j := range version.Items {
			version.Items[j].ReadOnly = !version.Current
		}
	}
	latest := view.Versions[len(view.Versions)-1]
	view.EventID, view.Items = latest.EventID, latest.Items
	view.Labels = map[string]string{
		"title": "Proposte di storie", "intro": "Rivedi le proposte e approva quelle pronte.",
		"pending": "Proposta", "accepted": "Nel backlog", "accept": "Accetta nel backlog", "edit": "Modifica proposta", "revise": "Continua in chat", "open": "Apri storia", "blocked_by": "Dipende da",
		"show": "Vedi proposta", "close": "Chiudi proposte", "version": "Versione", "destination": "Destinazione", "choose_destination": "Scegli la storia da aggiornare o crea una nuova storia", "previous": "Versione nel backlog", "changes": "Modifiche proposte", "historical": "Versione precedente · sola lettura",
	}
	return view
}

func proposalFromSpec(spec domain.Spec) execution.SpecDraftProposal {
	return execution.SpecDraftProposal{Title: spec.Title, EpicCode: spec.Epic.Code, Priority: string(spec.Priority), Points: spec.Points, Scope: string(spec.Scope), BlockedBy: append([]string{}, spec.BlockedBy...), Body: spec.Body}
}
func specDraftHash(spec domain.Spec) string {
	raw, _ := json.Marshal(proposalFromSpec(spec))
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func specDraftChanges(before, after execution.SpecDraftProposal) []string {
	changes := []string{}
	if before.Title != after.Title {
		changes = append(changes, "Titolo")
	}
	if before.Body != after.Body {
		changes = append(changes, "Descrizione e criteri di accettazione")
	}
	if before.EpicCode != after.EpicCode {
		changes = append(changes, "Epica")
	}
	if before.Priority != after.Priority {
		changes = append(changes, "Priorità")
	}
	if before.Points != after.Points {
		changes = append(changes, "Stima")
	}
	if before.Scope != after.Scope {
		changes = append(changes, "Perimetro")
	}
	if !slices.Equal(before.BlockedBy, after.BlockedBy) {
		changes = append(changes, "Dipendenze")
	}
	return changes
}

func (ws *workspaceSession) specDraftsView(ctx context.Context, id string, events []execution.RunEvent) *conversationSpecDraftsView {
	view := proposedSpecDrafts(events)
	if view == nil {
		return nil
	}
	record, _ := ws.conversationStore().GetMetadata(ctx, id)
	acceptedSpecs := map[string]domain.Spec{}
	identities := map[string]string{}
	for _, version := range view.Versions {
		if version.Current {
			continue
		}
		for i, draft := range version.Items {
			key := specDraftReference{EventID: version.EventID, Index: i}.key()
			if code := record.SpecDraftAcceptances[key]; code != "" {
				if saved, err := ws.conn.ReadSpecDetail(ctx, code); err == nil {
					acceptedSpecs[code] = saved
				}
				if draft.DraftID != "" {
					identities[draft.DraftID] = code
				}
			}
		}
	}
	codes := make([]string, 0, len(acceptedSpecs))
	for code := range acceptedSpecs {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	pending, revisions := 0, 0
	for i := range view.Items {
		item := &view.Items[i]
		key := specDraftReference{EventID: view.EventID, Index: i}.key()
		item.SpecCode = record.SpecDraftAcceptances[key]
		if item.SpecCode != "" {
			if snapshot, ok := record.SpecDraftAcceptedContent[key]; ok {
				item.SpecDraftProposal = snapshot
				item.Metadata = specDraftMetadata(snapshot)
			}
			continue
		}
		pending++
		target := item.RevisesSpec
		if target == "" && item.DraftID != "" {
			target = identities[item.DraftID]
		}
		// Legacy receipts can retain a title but have no stable identity. Only an
		// exact, unique match in this thread is automatic; renamed items ask the
		// person to select a destination instead of guessing from array positions.
		if target == "" {
			for _, code := range codes {
				if normalizeSpecTitle(acceptedSpecs[code].Title) == normalizeSpecTitle(item.Title) {
					if target != "" {
						target = ""
						break
					}
					target = code
				}
			}
		}
		item.Destinations = []conversationSpecDraftDestination{{ID: "__new__", Label: "Crea nuova storia", AcceptLabel: "Accetta nel backlog", StateLabel: "Nuova storia"}}
		targets := append([]string{}, codes...)
		if target != "" && !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
		for _, code := range targets {
			saved, ok := acceptedSpecs[code]
			if !ok {
				var err error
				saved, err = ws.conn.ReadSpecDetail(ctx, code)
				if err != nil {
					item.UnavailableReason = "La storia " + code + " non è più disponibile"
					continue
				}
			}
			previous := proposalFromSpec(saved)
			item.Destinations = append(item.Destinations, conversationSpecDraftDestination{ID: code, Label: "Aggiorna " + code + " · " + saved.Title, AcceptLabel: "Approva modifiche", StateLabel: "Modifiche a " + code, BaseHash: specDraftHash(saved), Previous: &previous, Changes: specDraftChanges(previous, item.SpecDraftProposal)})
		}
		if target != "" {
			item.DestinationID = target
			revisions++
		} else if len(codes) == 0 {
			item.DestinationID = "__new__"
		}
	}
	switch {
	case pending == 0:
		view.Summary = specDraftCount(len(view.Items), "storia", "storie") + " nel backlog"
	case revisions == pending:
		view.Summary = specDraftCount(pending, "modifica", "modifiche") + " da approvare"
	default:
		view.Summary = specDraftCount(pending, "proposta", "proposte") + " da rivedere"
	}
	return view
}

func specDraftCount(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(count) + " " + plural
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
type specDraftResolution struct {
	Accepted *domain.Spec
	Target   *domain.Spec
}

func (ws *workspaceSession) resolveSpecDraftSource(ctx context.Context, ref *specDraftReference) (specDraftResolution, error) {
	result := specDraftResolution{}
	if ref == nil {
		return result, nil
	}
	record, err := ws.conversationStore().GetMetadata(ctx, ref.ConversationID)
	if err != nil {
		return result, conversationNotFound(ref.ConversationID, err)
	}
	if record.IncrementID != "" {
		return result, iox.NewConflict("increment stories are internal to its sandbox", "approve the increment intent before development; publish its stories with the PR", nil)
	}
	if code := record.SpecDraftAcceptances[ref.key()]; code != "" {
		spec, err := ws.conn.ReadSpecDetail(ctx, code)
		result.Accepted = &spec
		return result, err
	}
	events, err := ws.specDraftHistory(ctx, ref.ConversationID)
	if err != nil {
		return result, err
	}
	view := ws.specDraftsView(ctx, ref.ConversationID, events)
	if view == nil || view.EventID != ref.EventID || ref.Index < 0 || ref.Index >= len(view.Items) {
		return result, iox.NewConflict("this story proposal has been superseded", "read the latest proposal before accepting it", nil)
	}
	if record.Native() && record.Work != execution.SessionIdle {
		return result, iox.NewConflict("the agent is still revising this conversation", "wait for the turn to finish before accepting the proposal", nil)
	}
	item := view.Items[ref.Index]
	target := ref.TargetSpec
	if target == "" {
		target = item.DestinationID
	}
	if target == "" || item.UnavailableReason != "" {
		return result, iox.NewConflict("choose the destination of this proposal", "select the existing story to update or explicitly create a new story", nil)
	}
	for _, destination := range item.Destinations {
		if destination.ID != target {
			continue
		}
		if target == "__new__" {
			return result, nil
		}
		if ref.BaseHash == "" || destination.BaseHash != ref.BaseHash {
			return result, iox.NewConflict("the story changed since this proposal was reviewed", "read the latest story before approving these changes", nil)
		}
		spec, err := ws.conn.ReadSpecDetail(ctx, target)
		if err != nil {
			return result, err
		}
		if specDraftHash(spec) != ref.BaseHash {
			return result, iox.NewConflict("the story changed since this proposal was reviewed", "read the latest story before approving these changes", nil)
		}
		result.Target = &spec
		return result, nil
	}
	return result, iox.NewInvalidInput("unknown story proposal destination", "select a destination offered for this proposal", nil)
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
	saved, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		return err
	}
	if record.SpecDraftAcceptedContent == nil {
		record.SpecDraftAcceptedContent = map[string]execution.SpecDraftProposal{}
	}
	record.SpecDraftAcceptedContent[ref.key()] = proposalFromSpec(saved)
	// Legacy conversations are also cached by the journal. Update that copy
	// so its next transcript write retains the person's acceptance.
	ws.journal.mu.Lock()
	defer ws.journal.mu.Unlock()
	if entry := ws.journal.entries[ref.ConversationID]; entry != nil {
		entry.record.SpecDraftAcceptances = record.SpecDraftAcceptances
		entry.record.SpecDraftAcceptedContent = record.SpecDraftAcceptedContent
	}
	return store.Save(ctx, record)
}
