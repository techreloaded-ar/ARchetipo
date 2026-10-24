package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

type incrementControl struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type incrementStoryView struct {
	domain.Spec
	StatusLabel string `json:"status_label"`
	Active      bool   `json:"active"`
}
type incrementView struct {
	Stories       []incrementStoryView `json:"stories"`
	ProgressLabel string               `json:"progress_label,omitempty"`
	ActivityLabel string               `json:"activity_label,omitempty"`
	Note          string               `json:"note"`
	increment.Record
	StatusLabel string             `json:"status_label"`
	Actions     []incrementControl `json:"actions"`
	Busy        bool               `json:"busy"`
	DeleteLabel string             `json:"delete_label,omitempty"`
}

func (s *Server) incrementService(ws *workspaceSession) *increment.Service {
	service := increment.New(ws.cfg, ws.conn)
	if s.incrementHosting != nil {
		service.Hosting = s.incrementHosting
	}
	return service
}
func (s *Server) incrementView(ctx context.Context, ws *workspaceSession, r increment.Record) incrementView {
	view := incrementView{Record: r, Actions: []incrementControl{}, DeleteLabel: "Elimina incremento"}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		return view
	}
	view.StatusLabel = tpl.IncrementStatusLabel(r.Status)
	view.Stories = []incrementStoryView{}
	if len(r.Revisions) > 0 {
		latest := r.Revisions[len(r.Revisions)-1]
		byCode := map[string]domain.Spec{}
		if latest.Approved {
			live, readErr := s.incrementService(ws).Stories(ctx, r)
			if readErr == nil {
				for _, story := range live {
					byCode[story.Code] = story
				}
			}
		}
		completed, known := 0, 0
		labels := map[domain.Status]string{domain.StatusTodo: tpl.Statuses.Todo, domain.StatusPlanned: tpl.Statuses.Planned, domain.StatusInProgress: tpl.Statuses.InProgress, domain.StatusReview: tpl.Statuses.Review, domain.StatusDone: tpl.Statuses.Done}
		for _, proposed := range latest.Stories {
			story := incrementStoryView{Spec: proposed, StatusLabel: "Stato non disponibile"}
			if !latest.Approved {
				story.StatusLabel = "Da approvare"
			}
			if current, ok := byCode[proposed.Code]; ok {
				story.Spec = current
				story.StatusLabel = labels[current.Status]
				if story.StatusLabel == "" {
					story.StatusLabel = string(current.Status)
				}
				story.Active = current.Status == domain.StatusInProgress
				known++
				if current.Status == domain.StatusDone {
					completed++
				}
			}
			view.Stories = append(view.Stories, story)
		}
		if known > 0 && known == len(view.Stories) {
			view.ProgressLabel = fmt.Sprintf("%d/%d storie completate", completed, known)
		}
	}
	view.ActivityLabel = "In attesa"
	view.Note = "Codice e storie lavorano nella sandbox. Pubblicazione nel backlog alla PR; consegna dopo il merge."
	if r.ConversationID != "" {
		if record, readErr := ws.conversationStore().GetMetadata(ctx, r.ConversationID); readErr == nil {
			view.Busy = record.Work != "" && record.Work != execution.SessionIdle
			switch record.Work {
			case execution.SessionTurnActive:
				view.ActivityLabel = "Agente al lavoro"
			case execution.SessionWaitingInput, execution.SessionWaitingApproval:
				view.ActivityLabel = "Serve il tuo intervento"
			}
		}
	}
	if environment := readEnvironmentOperation(r.Workdir); environment.State != "" {
		switch environment.State {
		case "preparing", "configuring":
			view.ActivityLabel = "Preparazione ambiente"
			view.Busy = true
		case "waiting":
			view.ActivityLabel = "Ambiente: serve il tuo intervento"
			view.Busy = true
			view.Note = "Apri Preview e Dettagli ambiente per rispondere nella sessione tecnica."
		case "error":
			view.ActivityLabel = "Ambiente da verificare"
			view.Note = environment.Error
		}
	}
	for _, action := range tpl.IncrementActions {
		allowed := false
		for _, status := range action.Statuses {
			if r.Status == status {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		if action.ID == "approve-intent" && (len(r.Revisions) == 0 || r.Revisions[len(r.Revisions)-1].Approved) {
			continue
		}
		view.Actions = append(view.Actions, incrementControl{ID: action.ID, Label: action.Label})
	}
	return view
}
func (s *Server) attachIncrement(ctx context.Context, ws *workspaceSession, record conversationlog.Record, view *conversationView) {
	if record.IncrementID == "" {
		return
	}
	view.SpecDrafts = nil // Sandbox stories are never accepted into the parent by the ordinary proposal form.
	r, err := s.incrementService(ws).Read(record.IncrementID)
	if err != nil {
		if receipt, receiptErr := s.incrementService(ws).DeletionReceipt(record.IncrementID); receiptErr == nil && receipt.Deleted {
			view.Increment = &incrementView{Record: receipt.Record, StatusLabel: "Ambiente eliminato", Actions: []incrementControl{}, Note: "L’ambiente locale è stato eliminato. Commit recuperabili tramite Git: " + receipt.BackupRef}
			return
		}
		view.Notice = "Incremento non disponibile: " + err.Error()
		return
	}
	view.RemovalChoiceRequired = true
	current := s.incrementView(ctx, ws, r)
	view.Increment = &current
}
func incrementPrompt(r increment.Record, message string) string {
	return fmt.Sprintf("/archetipo-increment %s\n\n%s\n\nWork only in the dedicated sandbox %s. Read the persisted increment before acting. Never modify the parent backlog or open a PR before the business approval.\nWorkflow conversation: %s", r.ID, message, r.Workdir, r.ConversationID)
}
func (s *Server) handleCreateIncrement(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	if err := ws.requireReachable(); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Title                string            `json:"title"`
		Request              string            `json:"request"`
		OriginConversationID string            `json:"origin_conversation_id"`
		Model                string            `json:"model"`
		ModelOptions         map[string]string `json:"model_options"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	target := s.conversationAvailabilityFor(ctx, ws)
	if target.reason != "" || target.sessions == nil {
		writeError(w, iox.NewConflict("serve un agente disponibile con sessioni native", "configura Codex o Claude nelle impostazioni del workspace", nil))
		return
	}
	if body.OriginConversationID != "" {
		if _, err := ws.conversationStore().GetMetadata(ctx, body.OriginConversationID); err != nil {
			writeError(w, conversationNotFound(body.OriginConversationID, err))
			return
		}
	}
	if strings.TrimSpace(body.Title) == "" {
		body.Title = truncateRunes(strings.TrimSpace(body.Request), 72)
	}
	service := s.incrementService(ws)
	created, err := service.Create(ctx, body.Title, body.Request, body.OriginConversationID)
	if err != nil {
		writeError(w, err)
		return
	}
	opened, err := s.openConversationOn(ctx, ws, conversationOpenSpec{title: body.Title, workingDir: created.Workdir, incrementID: created.ID, model: body.Model, modelOptions: body.ModelOptions})
	if err != nil {
		if rollbackErr := service.RemoveUnused(context.WithoutCancel(ctx), created.ID); rollbackErr == nil {
			writeError(w, err)
			return
		}
		writeError(w, iox.NewConflict("la sandbox "+created.ID+" è stata conservata, ma la conversazione non è partita: "+err.Error(), "riprendi l’incremento esistente prima di crearne un altro", err))
		return
	}
	created, err = service.Bind(context.WithoutCancel(ctx), created.ID, opened.snapshot.id)
	if err != nil {
		writeError(w, err)
		return
	}
	sendErr := s.sendNativeConversationMessage(ctx, ws, opened.snapshot, sendConversationMessageReq{Message: incrementPrompt(created, body.Request), display: body.Request})
	view := s.nativeConversationView(ctx, ws, opened.snapshot, 0)
	if sendErr != nil {
		view.Notice = "La sandbox e la conversazione sono conservate. Il messaggio iniziale non è confermato: " + sendErr.Error()
	}
	writeJSON(w, http.StatusCreated, view)
}
func (s *Server) handleGetIncrement(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := s.incrementService(ws).Read(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.incrementView(r.Context(), ws, record))
}
func (s *Server) requireIncrementIdle(ctx context.Context, ws *workspaceSession, r increment.Record) error {
	if snapshot, live := ws.conversation.get(r.ConversationID); live && snapshot.sessionProvider != nil {
		if err := requireSessionDirectory(snapshot); err != nil {
			return err
		}
		observed, err := s.ensureNativeSession(ctx, ws, snapshot)
		if err != nil {
			return err
		}
		if observed.Work != "" && observed.Work != execution.SessionIdle {
			return iox.NewConflict("l’agente sta ancora lavorando", "attendi la fine del turno prima di approvare", nil)
		}
	} else if r.ConversationID != "" {
		record, err := ws.conversationStore().GetMetadata(ctx, r.ConversationID)
		if err != nil {
			return err
		}
		if record.Work != "" && record.Work != execution.SessionIdle {
			return iox.NewConflict("lo stato del turno richiede riconciliazione", "riprendi la sessione prima di approvare", nil)
		}
	}
	return nil
}
func (s *Server) handleIncrementAction(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	service := s.incrementService(ws)
	current, err := service.Read(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err = s.requireIncrementIdle(ctx, ws, current); err != nil {
		writeError(w, err)
		return
	}
	action := r.PathValue("action")
	allowed := action == "publish" && (current.Status == increment.Proposed || current.Status == increment.Delivered)
	for _, control := range s.incrementView(ctx, ws, current).Actions {
		if control.ID == action {
			allowed = true
		}
	}
	if !allowed {
		writeError(w, iox.NewConflict("azione non disponibile per questo incremento", "rileggi lo stato corrente", nil))
		return
	}
	var body struct {
		Revision int    `json:"revision"`
		HeadSHA  string `json:"head_sha"`
	}
	if err = decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	message := ""
	switch action {
	case "approve-intent":
		current, err = service.Approve(ctx, current.ID, body.Revision)
		message = "L’utente ha approvato questa revisione dell’intento. Esegui lo sviluppo e le verifiche delle storie nella sandbox usando il workflow esistente. Fermati al risultato da provare; non pubblicare."
	case "develop":
		message = "Riprendi lo sviluppo delle storie dell’intento approvato. Conserva il lavoro già verificato e fermati al risultato da provare."
	case "discuss":
		message = "Riprendi la definizione dell’intento con la persona. Richiesta iniziale: " + current.Request
	case "publish":
		current, err = service.Publish(ctx, current.ID, body.HeadSHA, body.Revision)
	case "refresh":
		current, err = service.Refresh(ctx, current.ID)
	default:
		err = iox.NewInvalidInput("azione incremento sconosciuta", "usa un’azione restituita dal server", nil)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if message != "" {
		snapshot, live := ws.conversation.get(current.ConversationID)
		if !live {
			snapshot, live = s.reopenReleasedForMessage(ctx, ws, current.ConversationID)
		}
		if !live {
			writeError(w, iox.NewConflict("la conversazione deve essere ripresa", "riapri la sessione dell’incremento; l’intento approvato è conservato", nil))
			return
		}
		send := func(sendCtx context.Context) error {
			return s.sendNativeConversationMessage(sendCtx, ws, snapshot, sendConversationMessageReq{environmentPrepared: true, Message: incrementPrompt(current, message) + "\nEnvironment preparation report: " + environmentReport(current.Workdir) + "\nL’ambiente è stato preparato separatamente. Usa la ricetta comune e i report esistenti; non ripetere configurazione o build di setup nella chat di sviluppo. Per modifiche ai requisiti dell’ambiente, segnala il blocco nel pannello ambiente.", display: message})
		}
		if action == "approve-intent" || action == "develop" {
			err = s.queueEnvironment(ctx, ws, current.Workdir, false, false, send)
		} else {
			err = send(ctx)
		}
		if err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.incrementView(ctx, ws, current))
}
func (s *Server) prepareIncrementMessage(ctx context.Context, ws *workspaceSession, id string, body *sendConversationMessageReq) error {
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		return err
	}
	if record.Purpose == "environment" || record.IncrementID == "" {
		return nil
	}
	service := s.incrementService(ws)
	current, err := service.Read(record.IncrementID)
	if err != nil {
		return err
	}
	// The picker declares an explicit change request, without deciding which
	// story or requirement it changes. Release only our app/proxy so the skill
	// can run standalone verification against the existing dependency data.
	if body.PreviewFeedback && (current.Status == increment.Review || current.Status == increment.Developing) {
		if err := ws.previews.SuspendOwned(current.Workdir); err != nil {
			return err
		}
	}

	// The person's message stays readable; only the harness sees the method and
	// sandbox binding. An ordinary free conversation never enters this branch.
	body.display = body.Message
	body.Message = incrementPrompt(current, body.Message)
	return nil
}
