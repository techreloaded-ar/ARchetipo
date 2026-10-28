package web

import (
	"context"
	"net/http"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

type environmentAttention struct {
	Kind           string                      `json:"kind"`
	Title          string                      `json:"title"`
	Message        string                      `json:"message,omitempty"`
	ConversationID string                      `json:"conversation_id,omitempty"`
	Approvals      []execution.PendingApproval `json:"approvals,omitempty"`
	Inputs         []execution.PendingInput    `json:"inputs,omitempty"`
}

func (s *Server) presentEnvironment(ctx context.Context, ws *workspaceSession, target previewTarget, result *environmentPreviewView) {
	result.Services = environmentServices(target.Root, *result)
	result.Headline = "Ambiente da preparare"
	switch result.State {
	case "ready":
		result.Headline = "Ambiente pronto · preview attiva"
		if result.Preparation == nil || !result.Preparation.Passed || !result.Preparation.Stable {
			result.Headline = "Preview attiva · preparazione non verificata"
		}
	case "stopped":
		result.Headline = "Preview ferma"
	case "configuring":
		result.Headline = "Configurazione ambiente in corso"
	case "preparing":
		result.Headline = "Preparazione ambiente in corso"
	case "starting":
		result.Headline = "Avvio preview in corso"
	case "stopping":
		result.Headline = "Arresto ambiente in corso"
	case "waiting":
		result.Headline = "Ambiente in attesa"
	case "error":
		result.Headline = "Preparazione bloccata"
	}
	checks, err := verification.New(target.Root, ws.cfg.Paths.TestResults).ObserveChecks(ctx)
	if err == nil {
		result.Checks = checks
	} else if result.Configured {
		result.ChecksError = "Verifiche non leggibili: " + err.Error()
	}
	result.ChecksLabel = "Verifiche prodotto non ancora eseguite"
	if err == nil && len(checks) == 0 {
		result.ChecksLabel = "Nessuna verifica prodotto dichiarata"
	}
	failed, pending, historicalFailed := 0, 0, 0
	for _, check := range checks {
		if check.State == "failed" {
			if check.Current {
				failed++
			} else {
				historicalFailed++
			}
		}
		if !check.Current || check.State == "missing" {
			pending++
		}
	}
	if len(checks) > 0 {
		if failed > 0 {
			result.ChecksLabel = "Verifiche prodotto: sono presenti esiti falliti"
		} else if historicalFailed > 0 {
			result.ChecksLabel = "Verifiche da riconfermare · ultimi esiti falliti"
		} else if pending > 0 {
			result.ChecksLabel = "Verifiche prodotto da eseguire o riconfermare"
		} else {
			result.ChecksLabel = "Verifiche prodotto superate"
		}
	}
	id := result.Environment.ConversationID
	if id != "" {
		record, readErr := ws.conversationStore().GetMetadata(ctx, id)
		if readErr == nil {
			work := record.Work
			attention := environmentAttention{ConversationID: id}
			if snapshot, live := ws.conversation.get(id); live && snapshot.sessionProvider != nil && record.Connection != execution.ConnectionReleased {
				if observed, observeErr := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session}); observeErr == nil {
					work = observed.Work
					attention.Approvals = observed.PendingApprovals
					attention.Inputs = observed.PendingInputs
				}
			}
			switch {
			case len(attention.Approvals) > 0:
				attention.Kind = "approval"
				attention.Title = "Serve un’autorizzazione per continuare"
			case len(attention.Inputs) > 0:
				attention.Kind = "input"
				attention.Title = "L’agente aspetta una risposta"
			case work == execution.SessionWaitingApproval || work == execution.SessionWaitingInput:
				attention.Kind = "unavailable"
				attention.Title = "Apri la chat ambiente per leggere la richiesta"
				attention.Message = "La richiesta non è disponibile in questo collegamento. Nessuna risposta è stata inviata."
			case work == execution.SessionIdle && result.Environment.State == "waiting":
				attention.Kind = "message"
				attention.Title = "Configurazione da completare"
				if full, err := ws.conversationStore().Get(ctx, id); err == nil {
					for _, event := range full.Events {
						if event.Kind == "turn_start" {
							attention.Message = ""
						}
						if event.Kind == "text" {
							attention.Message += event.Text
						}
						if event.Kind == "turn_end" && event.FinalMessage != "" {
							attention.Message = event.FinalMessage
						}
					}
				}
			}
			if attention.Kind != "" {
				result.Attention = &attention
			}
		}
	}
	if result.Attention == nil && result.State == "error" {
		result.Attention = &environmentAttention{Kind: "blocked", Title: "Preparazione bloccata", Message: result.Error, ConversationID: id}
	}
}

// Serve only the selected check's retained original output, as inert text. The
// client supplies a declared name, never a filesystem path.
func (s *Server) handlePreviewCheckLog(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	target, err := s.resolvePreviewTarget(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	service := verification.New(target.Root, ws.cfg.Paths.TestResults)
	checks, err := service.ObserveChecks(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	for _, check := range checks {
		if check.Name != r.PathValue("name") || check.Report == nil {
			continue
		}
		log, err := service.Logs(*check.Report, 64000)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(log))
		return
	}
	writeError(w, iox.NewPrecondition("check report unavailable", "run the declared project check to retain original evidence", nil))
}
