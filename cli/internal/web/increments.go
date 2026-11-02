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
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

type incrementControl struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type incrementStoryView struct {
	domain.Spec
	RequirementsChanged bool   `json:"requirements_changed,omitempty"`
	StatusLabel         string `json:"status_label"`
	Active              bool   `json:"active"`
}
type incrementPhaseView struct {
	Label string `json:"label"`
	State string `json:"state"` // done, current, todo
}

// incrementStepView is one of the next steps offered beside the composer. A
// decision or action takes the increment action named by Action; a prompt only
// fills the composer with Prompt; an open shows Open (preview, details,
// conversation or url) at Target.
type incrementStepView struct {
	Kind    string                     `json:"kind"`
	Label   string                     `json:"label"`
	Action  string                     `json:"action,omitempty"`
	Prompt  string                     `json:"prompt,omitempty"`
	Open    string                     `json:"open,omitempty"`
	Target  string                     `json:"target,omitempty"`
	Tone    string                     `json:"tone,omitempty"`
	Confirm *template.IncrementConfirm `json:"confirm,omitempty"`
}
type incrementView struct {
	ResultNotice     string               `json:"result_notice,omitempty"`
	Phases           []incrementPhaseView `json:"phases,omitempty"`
	NextSteps        []incrementStepView  `json:"next_steps"`
	NextStepsLabel   string               `json:"next_steps_label,omitempty"`
	NextStepsNote    string               `json:"next_steps_note,omitempty"`
	CandidateLabel   string               `json:"candidate_label,omitempty"`
	VerificationNote string               `json:"verification_note,omitempty"`
	QuickActions     []incrementControl   `json:"quick_actions"`
	PreviewAutoStart bool                 `json:"preview_auto_start"`
	Stories          []incrementStoryView `json:"stories"`
	ProgressLabel    string               `json:"progress_label,omitempty"`
	ActivityLabel    string               `json:"activity_label,omitempty"`
	Note             string               `json:"note"`
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
	view := incrementView{Record: r, Actions: []incrementControl{}, QuickActions: []incrementControl{}, DeleteLabel: "Elimina incremento"}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		return view
	}
	view.StatusLabel = tpl.IncrementStatusLabel(r.Status)
	if r.Result != nil {
		view.ResultNotice = increment.SimulatedNotice(r.Result.Simulated)
	}
	if r.Status == increment.Concluded {
		view.StatusLabel = "Apprendimento registrato"
	}

	view.Stories = []incrementStoryView{}
	labels := map[domain.Status]string{domain.StatusTodo: tpl.Statuses.Todo, domain.StatusPlanned: tpl.Statuses.Planned, domain.StatusInProgress: tpl.Statuses.InProgress, domain.StatusReview: tpl.Statuses.Review, domain.StatusDone: tpl.Statuses.Done}
	for _, linked := range r.LinkedStories {
		story := incrementStoryView{Spec: domain.Spec{Code: linked.Code, Title: linked.Title, Body: linked.Body, URL: linked.URL}, StatusLabel: "Storia collegata"}
		if current, readErr := ws.conn.ReadSpecDetail(ctx, linked.Code); readErr == nil {
			story.StatusLabel = labels[current.Status]
			story.RequirementsChanged = current.Title != linked.Title || current.Body != linked.Body
			if story.RequirementsChanged {
				story.StatusLabel += " · requisiti aggiornati nel backlog"
			}
		}
		view.Stories = append(view.Stories, story)
	}
	view.ActivityLabel = "In attesa"
	view.Note = "Specifiche, storie collegate, osservazioni e versioni del lavoro. L’approvazione riguarda il risultato provato; i developer decidono l’integrazione."
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
			view.Note = "Apri la conversazione Ambiente di sviluppo dai prossimi passi o dal rail."
		case "error":
			view.ActivityLabel = "Ambiente da verificare"
			view.Note = environment.Error
		}
	}
	view.PreviewAutoStart = r.DevelopmentMode == increment.Continuous && (r.Status == increment.Review || r.Status == increment.Developing) && !view.Busy
	view.CandidateLabel = view.StatusLabel
	if r.Status == increment.Developing || r.Status == increment.Review {
		view.CandidateLabel = "In lavorazione"
	}
	candidateValid := false
	if r.Status == increment.Review && !view.Busy {
		if verifyErr := s.incrementService(ws).VerifyCandidate(ctx, r); verifyErr == nil {
			candidateValid = true
			view.CandidateLabel = "Versione verificata"
		} else {
			view.CandidateLabel = "Da verificare"
			view.VerificationNote = "La versione corrente richiede nuove verifiche prima dell’approvazione."
		}
	}
	if r.DevelopmentMode != increment.Continuous {
		view.StatusLabel = "Lavoro storico"
		view.CandidateLabel = view.StatusLabel
		view.Note = "Il percorso precedente è stato ritirato. Richiesta, specifiche e risultati restano consultabili ed esportabili; avvia un nuovo lavoro per continuare."
		view.PreviewAutoStart = false
		return view
	}
	offered := []template.IncrementAction{}
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
		if action.ID == "publish" && !candidateValid {
			continue
		}
		if action.ID == "approve-intent" {
			continue
		}
		offered = append(offered, action)
		view.Actions = append(view.Actions, incrementControl{ID: action.ID, Label: action.Label})
		if action.ID == "publish" || action.ID == "refresh" {
			view.QuickActions = append(view.QuickActions, incrementControl{ID: action.ID, Label: action.Label})
		}
	}
	view.Phases = incrementPhases(tpl, r.Status)
	s.incrementNextSteps(ws, r, offered, &view)
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
	if r.Status == increment.Proposed && !current.Busy {
		s.checkIncrementPR(ws, r.ID)
	}
}
func incrementPrompt(r increment.Record, message string) string {
	return fmt.Sprintf("/archetipo-increment %s\n\n%s\n\nWork in the dedicated worktree %s. Read the persisted increment before acting. Keep product changes in this worktree; never modify the parent backlog or open a PR before the business approval. This worktree is local working separation, not an OS security sandbox. Environment configuration and repair belong to /archetipo-environment in its separate technical session. That session may publish the common recipe and its non-secret helpers through archetipo execution environment publish from this worktree; this does not publish product code or require manual edits in the parent checkout. Respect the harness's actual permissions and report missing capabilities. You are running inside ARchetipo View: the viewer owns preview startup. Do not invoke execution preview run or launch another application server in this session. Configure the managed recipe and finish the turn so the viewer can launch it. If browser control is unavailable, report that limitation and let the person try the managed Preview; do not keep the turn open waiting for manual validation.\n%s\nWorkflow conversation: %s", r.ID, message, r.Workdir, sessionInputGuidance, r.ConversationID)
}
func (s *Server) handleCreateIncrement(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	ctx := r.Context()
	if err := ws.requireReachable(); err != nil {
		writeError(w, err)
		return
	}
	var body struct {
		Attachments          []attachmentUpload `json:"attachments"`
		DevelopmentMode      string             `json:"development_mode"`
		StoryCodes           []string           `json:"story_codes"`
		Title                string             `json:"title"`
		Request              string             `json:"request"`
		OriginConversationID string             `json:"origin_conversation_id"`
		Model                string             `json:"model"`
		ModelOptions         map[string]string  `json:"model_options"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMessageBodyBytes)
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
	created, err := service.CreateWithMode(ctx, body.Title, body.Request, body.OriginConversationID, body.DevelopmentMode)
	if err != nil {
		writeError(w, err)
		return
	}
	if len(body.StoryCodes) > 0 {
		created, err = service.Link(ctx, created.ID, body.StoryCodes)
		if err != nil {
			_ = service.RemoveUnused(context.WithoutCancel(ctx), created.ID)
			writeError(w, err)
			return
		}
	}
	opened, err := s.openConversationOn(ctx, ws, conversationOpenSpec{title: body.Title, workingDir: created.Workdir, incrementID: created.ID, model: body.Model, modelOptions: body.ModelOptions})
	if err != nil {
		rollbackErr := service.RemoveUnused(context.WithoutCancel(ctx), created.ID)
		if rollbackErr == nil {
			writeError(w, err)
			return
		}
		writeError(w, iox.NewConflict("la sandbox "+created.ID+" è stata conservata, ma la conversazione non è partita: "+err.Error(), "riprendi l’incremento esistente prima di crearne un altro; rollback: "+rollbackErr.Error(), err))
		return
	}
	created, err = service.Bind(context.WithoutCancel(ctx), created.ID, opened.snapshot.id)
	if err != nil {
		writeError(w, err)
		return
	}
	// The images attached to the request travel with the first message, as
	// they would with any other: stored beside the conversation that was just
	// opened and handed to the adapter by reference.
	var attachments []execution.Attachment
	sendErr := error(nil)
	if len(body.Attachments) > 0 {
		if sendErr = requireAttachmentsCapability(ctx, opened.snapshot); sendErr == nil {
			attachments, sendErr = storeConversationAttachments(ws.conversationStore(), opened.snapshot.id, body.Attachments)
		}
	}
	if sendErr == nil {
		sendErr = s.sendNativeConversationMessage(ctx, ws, opened.snapshot, sendConversationMessageReq{Message: incrementPrompt(created, body.Request), display: body.Request, attachments: attachments})
	}
	view := s.nativeConversationView(ctx, ws, opened.snapshot, 0)
	if sendErr != nil {
		removeConversationAttachments(attachments)
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
	case "develop":
		message = "Riprendi il lavoro dalla specifica e dalle decisioni registrate. Conserva il lavoro già verificato e fermati al risultato da provare."
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
		if action == "develop" {
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

	// The person's message stays readable; only the harness sees the method and
	// sandbox binding. An ordinary free conversation never enters this branch.
	body.display = body.Message
	body.Message = incrementPrompt(current, body.Message)
	return nil
}

func (s *Server) handleIncrementDossier(w http.ResponseWriter, r *http.Request) {
	record, err := s.incrementService(s.session()).ReadDossier(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": record.ID, "markdown": increment.Dossier(record)})
}

func incrementPhases(tpl template.Template, status string) []incrementPhaseView {
	current := -1
	for i, phase := range tpl.IncrementPhases {
		for _, candidate := range phase.Statuses {
			if candidate == status {
				current = i
			}
		}
	}
	if current < 0 {
		return nil
	}
	phases := make([]incrementPhaseView, len(tpl.IncrementPhases))
	for i, phase := range tpl.IncrementPhases {
		phases[i] = incrementPhaseView{Label: phase.Label, State: "todo"}
		if i < current {
			phases[i].State = "done"
		} else if i == current {
			phases[i].State = "current"
		}
	}
	return phases
}

func (s *Server) incrementNextSteps(ws *workspaceSession, r increment.Record, offered []template.IncrementAction, view *incrementView) {
	steps := []incrementStepView{}
	if environment := readEnvironmentOperation(r.Workdir); environment.ConversationID != "" {
		switch environment.State {
		case "waiting":
			steps = append(steps, incrementStepView{Kind: "open", Open: "conversation", Target: environment.ConversationID, Tone: "warn", Label: "L’ambiente di sviluppo ti chiede una risposta · Apri"})
		case "error":
			steps = append(steps, incrementStepView{Kind: "open", Open: "conversation", Target: environment.ConversationID, Tone: "warn", Label: "L’ambiente di sviluppo ha un problema · Apri"})
		}
	}
	if r.Status == increment.Review {
		steps = append(steps, incrementStepView{Kind: "open", Open: "preview", Label: "Apri l’anteprima"})
	}
	if r.Status == increment.Proposed && r.PR != nil && r.PR.URL != "" {
		steps = append(steps, incrementStepView{Kind: "open", Open: "url", Target: r.PR.URL, Label: fmt.Sprintf("Apri la PR #%d su GitHub ↗", r.PR.Number)})
	}
	primary := false
	for _, action := range offered {
		kind := "action"
		if action.Primary {
			kind, primary = "decision", true
		}
		steps = append(steps, incrementStepView{Kind: kind, Action: action.ID, Label: action.Label, Confirm: action.Confirm})
	}
	for _, action := range offered {
		for _, prompt := range action.Prompts {
			steps = append(steps, incrementStepView{Kind: "prompt", Label: prompt.Label, Prompt: prompt.Text})
		}
	}
	view.NextSteps = steps
	view.NextStepsLabel = "Prossimi passi"
	if !primary {
		view.NextStepsLabel = view.StatusLabel
	}
	notes := []string{}
	if r.Status == increment.Proposed {
		notes = append(notes, "in attesa del merge della PR")
		if check := ws.prChecks.last(r.ID); check.err != "" {
			notes = append(notes, "ultimo controllo non riuscito: "+check.err)
		} else if !check.at.IsZero() {
			notes = append(notes, "controllata automaticamente alle "+check.at.Format("15:04"))
		}

	} else if !primary {
		notes = append(notes, view.ActivityLabel)
	}
	view.NextStepsNote = strings.Join(notes, " · ")
}
