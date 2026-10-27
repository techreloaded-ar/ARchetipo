package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

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
	StatusLabel string `json:"status_label"`
	Active      bool   `json:"active"`
}
type incrementPhaseView struct {
	Label string `json:"label"`
	State string `json:"state"` // done, current, todo
}

// incrementStepView is one of the next steps offered beside the composer. A
// decision or action takes the increment action named by Action; a prompt only
// fills the composer with Prompt; an open shows Open (preview, details, story,
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
	Stories       []incrementStoryView `json:"stories"`
	ProgressLabel string               `json:"progress_label,omitempty"`
	ActivityLabel string               `json:"activity_label,omitempty"`
	Note          string               `json:"note"`
	ResultNotice  string               `json:"result_notice,omitempty"`
	increment.Record
	StatusLabel    string               `json:"status_label"`
	Actions        []incrementControl   `json:"actions"`
	Busy           bool                 `json:"busy"`
	DeleteLabel    string               `json:"delete_label,omitempty"`
	Phases         []incrementPhaseView `json:"phases,omitempty"`
	NextSteps      []incrementStepView  `json:"next_steps"`
	NextStepsLabel string               `json:"next_steps_label,omitempty"`
	NextStepsNote  string               `json:"next_steps_note,omitempty"`
}

// incrementPRChecks remembers, per increment, when the viewer last asked
// GitHub about its PR, so polling the conversation does not call gh each time.
type incrementPRChecks struct {
	mu   sync.Mutex
	byID map[string]incrementPRCheck
}
type incrementPRCheck struct {
	at      time.Time
	running bool
	err     string
}

func (c *incrementPRChecks) begin(id string, every time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[string]incrementPRCheck{}
	}
	last := c.byID[id]
	if last.running || time.Since(last.at) < every {
		return false
	}
	last.running = true
	c.byID[id] = last
	return true
}
func (c *incrementPRChecks) end(id string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	check := incrementPRCheck{at: time.Now()}
	if err != nil {
		check.err = err.Error()
	}
	c.byID[id] = check
}
func (c *incrementPRChecks) last(id string) incrementPRCheck {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byID[id]
}

const incrementPRCheckInterval = time.Minute

// checkIncrementPR reconciles a published increment with GitHub in the
// background, at most once a minute, exactly as the refresh action does.
func (s *Server) checkIncrementPR(ws *workspaceSession, id string) {
	if !ws.prChecks.begin(id, incrementPRCheckInterval) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, removeErr, err := s.refreshIncrement(ctx, ws, id)
		if err == nil {
			err = removeErr
		}
		ws.prChecks.end(id, err)
	}()
}

// refreshIncrement reads the PR; once GitHub confirms the merge the sandbox has
// nothing left to do, so the conversation is archived and worktree, branch and
// services removed, as the explicit "archive with removal" choice does. A
// failed removal (removeErr) leaves the delivered increment intact.
func (s *Server) refreshIncrement(ctx context.Context, ws *workspaceSession, id string) (current increment.Record, removeErr, err error) {
	current, err = s.incrementService(ws).Refresh(ctx, id)
	if err == nil && current.Status == increment.Delivered {
		removeErr = s.removeDeliveredIncrement(ctx, ws, current.ID)
	}
	return current, removeErr, err
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
	view.Note = "Il codice lavora in un ambiente separato; le storie sono nel backlog in lavorazione, in revisione durante la preparazione al rilascio, completate dopo il merge."
	if r.Result != nil {
		view.ResultNotice = increment.SimulatedNotice(r.Result.Simulated)
	}
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
			view.Note = "Rispondi nella conversazione «Ambiente di sviluppo»: la trovi tra i prossimi passi e nel rail, sotto questo incremento."
		case "error":
			view.ActivityLabel = "Ambiente da verificare"
			view.Note = environment.Error
		}
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
		if action.ID == "approve-intent" && (len(r.Revisions) == 0 || r.Revisions[len(r.Revisions)-1].Approved) {
			continue
		}
		offered = append(offered, action)
		view.Actions = append(view.Actions, incrementControl{ID: action.ID, Label: action.Label})
	}
	view.Phases = incrementPhases(tpl, r.Status)
	s.incrementNextSteps(ws, r, offered, &view)
	return view
}

// incrementNextSteps orders what the person can do now: what blocks the work,
// what to look at, the process decisions, then the ready-made prompts.
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
	if len(r.Revisions) > 0 && !r.Revisions[len(r.Revisions)-1].Approved {
		steps = append(steps, incrementStepView{Kind: "open", Open: "details", Target: "intent", Label: fmt.Sprintf("Leggi la proposta · v%d", r.Revisions[len(r.Revisions)-1].Number)})
	}
	for _, story := range view.Stories {
		if story.Active {
			steps = append(steps, incrementStepView{Kind: "open", Open: "story", Target: story.Code, Label: "In lavorazione: " + story.Code + " · " + story.Title})
		}
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
			notes = append(notes, "controllata alle "+check.at.Format("15:04")+", in automatico")
		}
	} else if !primary {
		notes = append(notes, view.ActivityLabel)
		if view.ProgressLabel != "" {
			notes = append(notes, view.ProgressLabel)
		}
	}
	view.NextStepsNote = strings.Join(notes, " · ")
}
func (s *Server) deletedIncrementView(ws *workspaceSession, receipt increment.DeletionResult) incrementView {
	view := incrementView{Record: receipt.Record, StatusLabel: "Ambiente eliminato", Actions: []incrementControl{}, NextSteps: []incrementStepView{}, Note: "L’ambiente locale è stato eliminato. Commit recuperabili tramite Git: " + receipt.BackupRef}
	if receipt.Record.Status == increment.Delivered {
		if tpl, err := s.resolveTemplate(ws); err == nil {
			view.Phases = incrementPhases(tpl, receipt.Record.Status)
		}
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
			deleted := s.deletedIncrementView(ws, receipt)
			view.Increment = &deleted
			return
		}
		view.Notice = "Incremento non disponibile: " + err.Error()
		return
	}
	view.RemovalChoiceRequired = true
	current := s.incrementView(ctx, ws, r)
	if r.Status == increment.Proposed && !current.Busy {
		s.checkIncrementPR(ws, r.ID)
	}
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
	message, display := "", ""
	published := current.Status == increment.Review || current.Status == increment.Publishing
	switch action {
	case "approve-intent":
		if current, err = service.Approve(ctx, current.ID, body.Revision); err == nil {
			current, err = service.SyncStories(ctx, current.ID)
		}
		message = "L’utente ha approvato questa revisione dell’intento. Esegui lo sviluppo e le verifiche delle storie nella sandbox usando il workflow esistente. Fermati al risultato da provare; non pubblicare."
		display = fmt.Sprintf("Approvo la versione %d della proposta: avvia lo sviluppo.", body.Revision)
	case "develop":
		message = "Riprendi lo sviluppo delle storie dell’intento approvato. Conserva il lavoro già verificato e fermati al risultato da provare."
		display = "Riprendi lo sviluppo."
	case "discuss":
		message = "Riprendi la definizione dell’intento con la persona. Richiesta iniziale: " + current.Request
		display = "Riprendiamo la definizione."
	case "publish":
		current, err = service.Publish(ctx, current.ID, body.HeadSHA, body.Revision)
		// The conversation records the decision and the agent acknowledges it;
		// a repeated press on a published increment says nothing again.
		if err == nil && published && current.Status == increment.Proposed && current.PR != nil {
			message = fmt.Sprintf("La persona ha approvato il risultato e la preparazione al rilascio è avviata: pull request #%d (%s). Confermalo in una frase, senza altre azioni.", current.PR.Number, current.PR.URL)
			display = fmt.Sprintf("Ho avviato la preparazione al rilascio: PR #%d su GitHub.", current.PR.Number)
		}
	case "refresh":
		var removeErr error
		current, removeErr, err = s.refreshIncrement(ctx, ws, current.ID)
		if err == nil && current.Status == increment.Delivered {
			if removeErr != nil {
				view := s.incrementView(ctx, ws, current)
				view.Note = "Incremento consegnato; ambiente non rimosso: " + removeErr.Error()
				writeJSON(w, http.StatusOK, view)
				return
			}
			if receipt, receiptErr := service.DeletionReceipt(current.ID); receiptErr == nil {
				writeJSON(w, http.StatusOK, s.deletedIncrementView(ws, receipt))
				return
			}
		}
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
		send := func(sendCtx context.Context) error {
			return s.sendNativeConversationMessage(sendCtx, ws, snapshot, sendConversationMessageReq{environmentPrepared: true, Message: incrementPrompt(current, message) + "\nEnvironment preparation report: " + environmentReport(current.Workdir) + "\nL’ambiente è stato preparato separatamente. Usa la ricetta comune e i report esistenti; non ripetere configurazione o build di setup nella chat di sviluppo. Per modifiche ai requisiti dell’ambiente, di’ che serve un intervento sulla conversazione «Ambiente di sviluppo»: View mostra alla persona come aprirla.", display: display})
		}
		switch {
		case !live:
			err = iox.NewConflict("la conversazione deve essere ripresa", "riapri la sessione dell’incremento; l’intento approvato è conservato", nil)
		case action == "approve-intent" || action == "develop":
			err = s.queueEnvironment(ctx, ws, current.Workdir, false, false, send)
		default:
			err = send(ctx)
		}
		if err != nil && action == "publish" {
			// The PR is open: only the acknowledgement in the conversation failed.
			view := s.incrementView(ctx, ws, current)
			view.Note = "Preparazione al rilascio avviata; il messaggio nella conversazione non è confermato: " + err.Error()
			writeJSON(w, http.StatusOK, view)
			return
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
