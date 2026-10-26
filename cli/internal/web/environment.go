package web

// Environment setup is an orchestration job, not a turn in the development
// conversation. Commands use the standalone verification service; the only
// agentic work is done in a separate durable native technical session.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

type environmentOperation struct {
	State          string               `json:"state"`
	Error          string               `json:"error,omitempty"`
	Log            string               `json:"log,omitempty"`
	ConversationID string               `json:"conversation_id,omitempty"`
	Preparation    *verification.Result `json:"preparation,omitempty"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

func environmentOperationPath(root string) string {
	return filepath.Join(root, ".archetipo/environment/operation.json")
}
func readEnvironmentOperation(root string) environmentOperation {
	var value environmentOperation
	if raw, err := os.ReadFile(environmentOperationPath(root)); err == nil {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}
func saveEnvironmentOperation(root string, value environmentOperation) error {
	value.UpdatedAt = time.Now().UTC()
	path := environmentOperationPath(root)
	return recordfile.WriteAtomic(filepath.Dir(path), path, ".operation-", value)
}
func environmentLease(ctx context.Context, root string) (*conversationlog.ConversationLock, error) {
	store, err := conversationlog.NewFileStore(root)
	if err != nil {
		return nil, err
	}
	return store.TryLock(ctx, "environment-preparation")
}

// queueEnvironment reserves before responding, preventing double presses and
// development racing setup. A failed job preserves the approval and its report.
func (s *Server) queueEnvironment(ctx context.Context, ws *workspaceSession, root string, needPreview, repair bool, ready func(context.Context) error, failed ...func(error)) error {
	lease, err := environmentLease(ctx, root)
	if err != nil {
		return iox.NewConflict("preparazione ambiente già in corso", "consulta il pannello Preview", err)
	}
	value := readEnvironmentOperation(root)
	value.State = "preparing"
	value.Error = ""
	value.Preparation = nil
	if err = saveEnvironmentOperation(root, value); err != nil {
		_ = lease.Unlock()
		return err
	}
	ws.dispatch.runFor("environment:"+root, func(jobCtx context.Context) {
		defer func() { _ = lease.Unlock() }()
		err := s.prepareEnvironment(jobCtx, ws, root, needPreview, repair, &value)
		if err == nil && ready != nil {
			if err = saveEnvironmentOperation(root, value); err == nil {
				err = ready(jobCtx)
			}
		}
		if err != nil {
			value.State = "error"
			value.Error = err.Error()
			if jobCtx.Err() != nil {
				value.Error = "Preparazione interrotta. Puoi riprovare dal pannello Preview."
			}
			for _, failure := range failed {
				failure(err)
			}
		} else {
			value.State = "ready"
		}
		_ = saveEnvironmentOperation(root, value)
	})
	return nil
}

func (s *Server) prepareEnvironment(ctx context.Context, ws *workspaceSession, root string, needPreview, repair bool, value *environmentOperation) error {
	// A preview started during discovery may still own the environment. Release
	// our own preview before development prepares/checks that same checkout,
	// retaining its dependency services and synthetic data across revisions.
	if !needPreview || repair {
		if err := ws.previews.SuspendOwned(root); err != nil {
			return err
		}
	}
	_, envErr := verification.Load(root)
	_, commonErr := environmentrecipe.Load(root)
	_, previewErr := preview.Load(root)
	if repair || commonErr != nil || envErr != nil || (needPreview && previewErr != nil) {
		// Only one technical agent may create/update the workspace recipe, even
		// when several increments request their first environment concurrently.
		commonStore, err := conversationlog.NewFileStore(environmentrecipe.Workspace(root))
		if err != nil {
			return err
		}
		commonLease, err := commonStore.Lock(ctx, "environment-configuration")
		if err != nil {
			return err
		}
		defer func() { _ = commonLease.Unlock() }()
		_, envErr = verification.Load(root)
		_, previewErr = preview.Load(root)
		_, commonErr = environmentrecipe.Load(root)
		if repair || commonErr != nil || envErr != nil || (needPreview && previewErr != nil) {
			value.State = "configuring"
			if err = s.configureEnvironment(ctx, ws, root, repair, value); err != nil {
				return err
			}
		}
	}
	value.State = "preparing"
	value.Error = ""
	if err := saveEnvironmentOperation(root, *value); err != nil {
		return err
	}
	service := verification.New(root, ws.cfg.Paths.TestResults)
	if ws.environmentRunner != nil {
		service.Runner = ws.environmentRunner
	}
	result, err := service.Execute(ctx, "prepare", "", "", false)
	value.Preparation = &result
	if err != nil {
		return err
	}
	if !result.Passed || !result.Stable {
		return fmt.Errorf("preparazione ambiente non riuscita; consulta %s", result.ReportPath)
	}
	return nil
}

func (s *Server) configureEnvironment(ctx context.Context, ws *workspaceSession, root string, repair bool, value *environmentOperation) error {
	// Reuse a technical session after an approval/input/restart. It is never the
	// product conversation and never receives a copied development transcript.
	var snapshot conversationSnapshot
	live := false
	if value.ConversationID != "" {
		snapshot, live = ws.conversation.get(value.ConversationID)
		if !live {
			snapshot, live = s.reopenReleasedForMessage(ctx, ws, value.ConversationID)
		}
	}
	if !live {
		opened, err := s.openConversationOn(ctx, ws, conversationOpenSpec{workingDir: root, title: "Ambiente di sviluppo", purpose: "environment"})
		if err != nil {
			return err
		}
		if opened.snapshot.sessionProvider == nil {
			return iox.NewConflict("la configurazione ambiente richiede una sessione nativa", "configura Codex o Claude", nil)
		}
		snapshot = opened.snapshot
		value.ConversationID = snapshot.id
	}
	if err := saveEnvironmentOperation(root, *value); err != nil {
		return err
	}
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil {
		return err
	}
	if record.Work == "" || record.Work == execution.SessionIdle {
		contextJSON, _ := json.Marshal(map[string]string{
			"working_directory": root, "workspace_directory": environmentrecipe.Workspace(root),
			"provider": record.ProviderID, "requested_permission_profile": record.NextTurn.PermissionProfile,
			"capability_status": "unverified: run environment capabilities inside this native session; viewer privileges are not evidence",
		})
		prompt := environmentSetupPrompt + "\nRuntime context (configuration, not a capability observation):\n" + string(contextJSON)
		if repair {
			prompt += "\nRipara la configurazione comune esistente; conserva i servizi e i dati di altri ambienti."
		}
		if err = s.sendNativeConversationMessage(ctx, ws, snapshot, sendConversationMessageReq{Message: prompt, display: "Configura e verifica l’ambiente del workspace."}); err != nil {
			return err
		}
	}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		observed, readErr := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session})
		if readErr != nil {
			return fmt.Errorf("lettura sessione tecnica: %w", readErr)
		}
		if err = ws.persistNativeSnapshot(ctx, snapshot.id, observed); err != nil {
			return err
		}
		record, err = ws.conversationStore().GetMetadata(ctx, snapshot.id)
		if err != nil {
			return err
		}
		if record.Work == execution.SessionWaitingApproval || record.Work == execution.SessionWaitingInput {
			value.State = "waiting"
			_ = saveEnvironmentOperation(root, *value)
		} else {
			value.State = "configuring"
		}
		if record.CurrentTurn != nil && terminalTurn(record.CurrentTurn.State) {
			if record.CurrentTurn.State != execution.TurnCompleted {
				return fmt.Errorf("configurazione ambiente interrotta: %s", record.CurrentTurn.Error)
			}
			if _, err = environmentrecipe.Load(root); err == nil {
				if _, err = verification.Load(root); err != nil {
					return err
				}
				bundle, loadErr := environmentrecipe.Load(root)
				if loadErr != nil {
					return loadErr
				}
				assets := []string{}
				for _, file := range bundle.Files {
					if file.Path != environmentrecipe.Environment && file.Path != environmentrecipe.Preview {
						assets = append(assets, file.Path)
					}
				}
				service := verification.New(root, ws.cfg.Paths.TestResults)
				if _, qualificationErr := service.VerifyQualification(assets); qualificationErr == nil {
					local, collectErr := environmentrecipe.Collect(root, assets)
					if collectErr == nil && environmentrecipe.Digest(local) == environmentrecipe.Digest(bundle) {
						return nil
					}
				}
				// A previous common bundle cannot certify a failed candidate repair.
				value.Error = "Ricetta non qualificata o non pubblicata: apri Dettagli ambiente per completare il collaudo della candidata e la pubblicazione."
			} else if !os.IsNotExist(err) {
				return err
			}
			// A completed turn can be a question. Keep its native context and resume
			// automatically after the person's answer in the technical details.
			value.State = "waiting"
			if value.Error == "" {
				value.Error = "Configurazione incompleta: apri Dettagli ambiente per rispondere o risolvere il requisito mancante."
			}
			if err = saveEnvironmentOperation(root, *value); err != nil {
				return err
			}
		}
		if record.Connection == execution.ConnectionDisconnected || record.Connection == execution.ConnectionReleased {
			return fmt.Errorf("sessione tecnica scollegata; riprendila dai dettagli ambiente")
		}
		select {
		case <-ctx.Done():
			interruptCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if record.CurrentTurn != nil && !terminalTurn(record.CurrentTurn.State) {
				_, _ = snapshot.sessionProvider.InterruptTurn(interruptCtx, execution.SessionCommandRequest{Session: snapshot.session, TurnID: record.CurrentTurn.ID})
			}
			cancel()
			return ctx.Err()
		case <-tick.C:
		}
	}
}

const environmentSetupPrompt = `/archetipo-environment

` + sessionInputGuidance + `

Sei nella sessione tecnica dell’ambiente, separata dalla chat di sviluppo. Applica /archetipo-environment anche nel primo avvio di un clone senza configurazione: scopri lo stack dai file e verifica le capacità eseguendo archetipo execution environment capabilities dentro questa sessione. Non attribuire alla sessione i permessi del viewer. Se una capacità necessaria è assente, usa il normale meccanismo di approvazione dell’harness o dichiara il blocco concreto; non ripetere comandi vietati e non aumentare automaticamente i permessi.
Non sviluppare funzionalità e non modificare backlog, storie o approvazioni. Crea o ripara una candidata con archetipo execution environment configure --candidate --file - e, solo per una vera app web, archetipo execution preview configure --candidate --file -. Mantieni gli helper non segreti sotto scripts/archetipo/. Qualifica la candidata con archetipo execution environment qualify ripetendo --asset per OGNI helper necessario, poi pubblica lo stesso bundle con archetipo execution environment publish --qualified e gli stessi --asset. Non aggiornare la ricetta comune attraverso configure senza --candidate. Conserva report e log originali; passed e stable non certificano automaticamente la correttezza del prodotto. Esegui i checks dichiarati con check --candidate e riporta separatamente difetti prodotto e problemi setup senza indebolire i controlli. La skill sceglie e motiva copertura, readiness reale, installazione riproducibile, fixture conservative e verifica di stop/ripresa posseduti. Non avviare il server preview: il viewer continua dopo la verifica degli effetti persistiti. Concludi con un esito sintetico e i blocchi irrisolti.`

func environmentReport(root string) string {
	if value := readEnvironmentOperation(root); value.Preparation != nil {
		return value.Preparation.ReportPath
	}
	return ""
}

// A crash before development starts has no development turn to reconcile.
// Close only actions with no submitted turn, after proving the preparation
// lease is free. The human approval and technical native session survive.
func (ws *workspaceSession) reconcilePendingEnvironmentExecution(ctx context.Context, record conversationlog.Record) {
	operation := readEnvironmentOperation(record.WorkingDir)
	if operation.State == "" {
		return
	}
	lease, err := environmentLease(ctx, record.WorkingDir)
	if err != nil {
		return
	}
	defer func() { _ = lease.Unlock() }()
	for _, id := range record.ExecutionIDs {
		if _, found := lastTurnOfExecution(record, id); found {
			continue
		}
		if stored, readErr := ws.store.Get(ctx, id); readErr == nil && stored.Status == execution.StatusRunning {
			ws.settleSessionAction(ctx, id, nil, fmt.Errorf("view was restarted before environment preparation handed off to development; retry the action"))
		}
	}
}
