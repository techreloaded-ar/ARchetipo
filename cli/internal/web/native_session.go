package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

const nativeConversationPageSize = 500

// nativeSessionFollowers are the only readers of provider event streams. They
// append on receipt, so browser polling is never part of the durability path.
type nativeSessionFollowers struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running map[string]context.CancelFunc
}

func newNativeSessionFollowers() *nativeSessionFollowers {
	return &nativeSessionFollowers{running: map[string]context.CancelFunc{}}
}

func (f *nativeSessionFollowers) bind(parent context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	f.ctx, f.cancel = context.WithCancel(parent)
}

func (f *nativeSessionFollowers) start(id string, follow func(context.Context)) {
	f.mu.Lock()
	if _, exists := f.running[id]; exists {
		f.mu.Unlock()
		return
	}
	parent := f.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	f.running[id] = cancel
	f.mu.Unlock()
	go func() {
		defer func() {
			f.mu.Lock()
			delete(f.running, id)
			f.mu.Unlock()
		}()
		follow(ctx)
	}()
}

func (f *nativeSessionFollowers) stop(id string) {
	f.mu.Lock()
	cancel := f.running[id]
	delete(f.running, id)
	f.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (f *nativeSessionFollowers) closeAll() {
	f.mu.Lock()
	if f.cancel != nil {
		f.cancel()
	}
	for _, cancel := range f.running {
		cancel()
	}
	f.running = map[string]context.CancelFunc{}
	f.mu.Unlock()
}

func (ws *workspaceSession) restoreNativeConversations(registry *execution.Registry) error {
	if ws == nil || registry == nil || ws.conversationStore() == nil {
		return nil
	}
	records, err := ws.conversationStore().List(context.Background())
	if err != nil {
		return err
	}
	for _, record := range records {
		if !record.Native() || record.Archive == execution.ArchiveArchived {
			continue
		}
		provider, resolveErr := registry.Resolve(record.Session.ProviderID)
		if resolveErr != nil {
			continue
		}
		sessions, supported := execution.SessionProviderFor(provider)
		if !supported {
			continue
		}
		openedAt := record.OpenedAt
		if openedAt.IsZero() {
			openedAt = time.Now().UTC()
		}
		if err := ws.conversation.open(conversationHold{
			id:              record.ID,
			providerID:      record.Session.ProviderID,
			sessionProvider: sessions,
			session:         *record.Session,
			providerConfig:  execution.CloneConfig(record.Session.Environment.ProviderConfig),
			model:           record.NextTurn.Model,
			modelOptions:    cloneModelOptions(record.NextTurn.Options),
			workingDir:      record.Session.Environment.WorkingDir,
			openedAt:        openedAt,
			specCode:        record.SpecCode,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (ws *workspaceSession) startNativeFollowers() {
	if ws == nil {
		return
	}
	for _, snapshot := range ws.conversation.list() {
		if snapshot.sessionProvider != nil {
			ws.startNativeFollower(snapshot)
		}
	}
}

func (ws *workspaceSession) startNativeFollower(snapshot conversationSnapshot) {
	if ws == nil || snapshot.sessionProvider == nil || ws.nativeFollowers == nil {
		return
	}
	ws.nativeFollowers.start(snapshot.id, func(ctx context.Context) {
		for ctx.Err() == nil {
			record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
			if err != nil {
				return
			}
			appender, err := ws.conversationStore().NewEventAppender(ctx, snapshot.id)
			if err != nil {
				return
			}
			afterID := appender.LastID()
			var lastReceived *execution.RunEvent
			err = snapshot.sessionProvider.StreamSessionEvents(ctx, execution.SessionRequest{Session: snapshot.session}, afterID, func(event execution.RunEvent) error {
				if event.ID <= afterID {
					return nil
				}
				if appendErr := appender.Append(event); appendErr != nil {
					return appendErr
				}
				afterID = event.ID
				copy := event
				lastReceived = &copy
				return nil
			})
			_ = appender.Close()
			if lastReceived != nil {
				_ = ws.updateNativeRecordFromEvent(ctx, record, *lastReceived)
			}
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				_ = ws.markNativeDisconnected(context.Background(), snapshot.id)
				return
			}
			if observed, readErr := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session}); readErr == nil {
				_ = ws.persistNativeSnapshot(ctx, snapshot.id, observed)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
}

func (ws *workspaceSession) releaseNativeConversation(ctx context.Context, snapshot conversationSnapshot) error {
	if ws == nil || snapshot.sessionProvider == nil {
		return nil
	}
	ws.nativeFollowers.stop(snapshot.id)
	if err := snapshot.sessionProvider.ReleaseSession(ctx, execution.SessionRequest{Session: snapshot.session}); err != nil {
		return err
	}
	lock, err := ws.conversationStore().Lock(ctx, snapshot.id)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil {
		return err
	}
	record.Connection = execution.ConnectionReleased
	if err := ws.conversationStore().Save(ctx, record); err != nil {
		return err
	}
	ws.conversation.forget(snapshot.id)
	return nil
}

func (ws *workspaceSession) updateNativeRecordFromEvent(ctx context.Context, prior conversationlog.Record, event execution.RunEvent) error {
	lock, err := ws.conversationStore().Lock(ctx, prior.ID)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, prior.ID)
	if err != nil {
		return err
	}
	record.LastMessageAt = event.At
	if record.LastMessageAt.IsZero() {
		record.LastMessageAt = time.Now().UTC()
	}
	page, pageErr := ws.conversationStore().ReadEvents(ctx, prior.ID, 0, 0)
	if pageErr != nil {
		return pageErr
	}
	record.MessageCount = conversationMessageCount(page.Events)
	if (record.Title == "" || strings.HasPrefix(record.Title, "Conversazione del ")) && len(page.Events) > 0 {
		record.Title = conversationTitleOf(page.Events, record.OpenedAt)
	}
	return ws.conversationStore().Save(ctx, record)
}

func (ws *workspaceSession) persistNativeSnapshot(ctx context.Context, id string, snapshot execution.SessionSnapshot) error {
	lock, err := ws.conversationStore().Lock(ctx, id)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		return err
	}
	record.Session = &snapshot.Session
	record.Archive = snapshot.Archive
	record.Connection = snapshot.Connection
	record.Recovery = snapshot.Recovery
	record.Work = snapshot.Work
	record.CurrentTurn = snapshot.CurrentTurn
	if snapshot.CurrentTurn != nil {
		for index := range record.Deliveries {
			if record.Deliveries[index].State == execution.DeliveryUncertain && record.Deliveries[index].TurnID == snapshot.CurrentTurn.ID {
				record.Deliveries[index].State = execution.DeliveryConfirmed
			}
		}
	}
	if snapshot.CurrentTurn != nil && terminalTurn(snapshot.CurrentTurn.State) {
		record.Turns = upsertTurn(record.Turns, *snapshot.CurrentTurn)
	}
	return ws.conversationStore().Save(ctx, record)
}

func (ws *workspaceSession) markNativeDisconnected(ctx context.Context, id string) error {
	lock, err := ws.conversationStore().Lock(ctx, id)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil {
		return err
	}
	record.Connection = execution.ConnectionDisconnected
	if record.CurrentTurn != nil && record.CurrentTurn.State == execution.TurnActive {
		record.CurrentTurn.State = execution.TurnFailed
		record.CurrentTurn.Error = "the native turn connection ended before completion was observed"
		record.Turns = upsertTurn(record.Turns, *record.CurrentTurn)
		record.Work = execution.SessionIdle
	}
	return ws.conversationStore().Save(ctx, record)
}

func terminalTurn(state execution.TurnState) bool {
	return state == execution.TurnCompleted || state == execution.TurnInterrupted || state == execution.TurnFailed
}

func upsertTurn(turns []execution.SessionTurn, turn execution.SessionTurn) []execution.SessionTurn {
	for index := range turns {
		if turns[index].ID == turn.ID {
			turns[index] = turn
			return turns
		}
	}
	return append(turns, turn)
}

func upsertDelivery(deliveries []execution.SessionDelivery, delivery execution.SessionDelivery) []execution.SessionDelivery {
	for index := range deliveries {
		if deliveries[index].SubmissionID == delivery.SubmissionID {
			deliveries[index] = delivery
			return deliveries
		}
	}
	return append(deliveries, delivery)
}

func nativeOperationID(prefix string) (string, error) {
	id, err := execution.RandomID()
	if err != nil {
		return "", err
	}
	return prefix + strings.TrimPrefix(id, "exec-"), nil
}

func (s *Server) ensureNativeSession(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot) (execution.SessionSnapshot, error) {
	observed, err := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session})
	if err == nil && observed.Connection == execution.ConnectionConnected {
		_ = ws.persistNativeSnapshot(ctx, snapshot.id, observed)
		return observed, nil
	}
	resumed, resumeErr := snapshot.sessionProvider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: snapshot.session})
	if resumeErr != nil {
		_ = ws.markNativeDisconnected(context.Background(), snapshot.id)
		return execution.SessionSnapshot{}, resumeErr
	}
	if persistErr := ws.persistNativeSnapshot(ctx, snapshot.id, resumed); persistErr != nil {
		return execution.SessionSnapshot{}, persistErr
	}
	ws.startNativeFollower(snapshot)
	return resumed, nil
}

func (s *Server) sendNativeConversationMessage(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot, body sendConversationMessageReq) error {
	lock, err := ws.conversationStore().Lock(ctx, snapshot.id)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil {
		return err
	}
	if record.Archive == execution.ArchiveArchived {
		return iox.NewConflict("the conversation "+snapshot.id+" is archived", "reopen it before sending another turn", nil)
	}
	observed, readErr := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session})
	if readErr != nil || observed.Connection != execution.ConnectionConnected {
		observed, err = snapshot.sessionProvider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: snapshot.session})
		if err != nil {
			record.Connection = execution.ConnectionDisconnected
			_ = ws.conversationStore().Save(context.WithoutCancel(ctx), record)
			return err
		}
		record.Connection = observed.Connection
		record.Recovery = observed.Recovery
		record.Work = observed.Work
		record.CurrentTurn = observed.CurrentTurn
	}
	for _, delivery := range record.Deliveries {
		if delivery.State == execution.DeliveryUncertain {
			return iox.NewConflict("the last command delivery is uncertain", "reconcile the native session before sending another command", nil)
		}
	}
	if record.Work != "" && record.Work != execution.SessionIdle {
		if strings.TrimSpace(body.Skill) != "" {
			return iox.NewConflict("a native skill cannot be added through steering", "wait for the active turn to finish, then invoke the skill", nil)
		}
		discovery, discoveryErr := snapshot.sessionProvider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{
			ProviderConfig: snapshot.providerConfig, WorkingDir: snapshot.workingDir, Native: &snapshot.session.Native,
		})
		if discoveryErr != nil || !execution.SupportsSessionCapability(discovery.Capabilities, execution.SessionCapabilitySteering) {
			return iox.NewConflict("the active turn does not accept steering", "wait for it to finish or interrupt that turn", discoveryErr)
		}
		if record.CurrentTurn == nil {
			return iox.NewConflict("the conversation "+snapshot.id+" has no identifiable active turn", "refresh the session before sending another command", nil)
		}
		submissionID, operationErr := nativeOperationID("submission-")
		if operationErr != nil {
			return operationErr
		}
		delivery := execution.SessionDelivery{SubmissionID: submissionID, TurnID: record.CurrentTurn.ID, State: execution.DeliveryUncertain}
		record.Deliveries = upsertDelivery(record.Deliveries, delivery)
		if saveErr := ws.conversationStore().Save(ctx, record); saveErr != nil {
			return saveErr
		}
		delivery, steerErr := snapshot.sessionProvider.SteerTurn(ctx, execution.SessionCommandRequest{
			Session: snapshot.session, TurnID: record.CurrentTurn.ID, SubmissionID: submissionID, Message: body.Message,
		})
		if delivery.State != "" {
			record.Deliveries = upsertDelivery(record.Deliveries, delivery)
		}
		if saveErr := ws.conversationStore().Save(context.WithoutCancel(ctx), record); saveErr != nil {
			return saveErr
		}
		if steerErr != nil {
			return iox.NewConflict("the steering delivery was not confirmed", "reconcile the native session before retrying", steerErr)
		}
		return nil
	}
	var selectedSkills []execution.SessionSkill
	if requestedName := strings.TrimSpace(body.Skill); requestedName != "" {
		discovery, discoveryErr := snapshot.sessionProvider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{
			ProviderConfig: snapshot.providerConfig, WorkingDir: snapshot.workingDir, Native: &snapshot.session.Native,
		})
		if discoveryErr != nil {
			return iox.NewConflict("the native skill catalog is not available", "refresh the catalog before invoking a skill", discoveryErr)
		}
		if !discovery.SkillsKnown {
			return iox.NewConflict("the native skill catalog is not known yet", "complete one ordinary turn so the runtime can report its catalog", nil)
		}
		for _, skill := range discovery.Skills {
			if skill.Name == requestedName {
				selectedSkills = []execution.SessionSkill{skill}
				break
			}
		}
		if len(selectedSkills) == 0 {
			return iox.NewInvalidInput("skill is not available in this native session", "choose a skill from the current session catalog", nil)
		}
	}
	turnID, err := nativeOperationID("turn-")
	if err != nil {
		return err
	}
	submissionID, err := nativeOperationID("submission-")
	if err != nil {
		return err
	}
	requested := record.NextTurn
	turn := execution.SessionTurn{ID: turnID, State: execution.TurnActive, Requested: requested, StartedAt: time.Now().UTC()}
	delivery := execution.SessionDelivery{SubmissionID: submissionID, TurnID: turnID, State: execution.DeliveryUnsent}
	record.CurrentTurn = &turn
	record.Work = execution.SessionTurnActive
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if err := ws.conversationStore().Save(ctx, record); err != nil {
		return err
	}
	// From this durable boundary onward a crash cannot prove whether the
	// provider saw the command, so automatic retry is forbidden.
	delivery.State = execution.DeliveryUncertain
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if err := ws.conversationStore().Save(ctx, record); err != nil {
		return err
	}
	started, startErr := snapshot.sessionProvider.StartTurn(ctx, execution.StartTurnRequest{
		Session: snapshot.session, TurnID: turnID, SubmissionID: submissionID,
		Message: body.Message, Model: requested.Model, Options: cloneModelOptions(requested.Options), Skills: selectedSkills,
	})
	if started.Delivery.State != "" {
		delivery = started.Delivery
	}
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if started.Turn.ID != "" {
		record.CurrentTurn = &started.Turn
		record.Work = execution.SessionTurnActive
	}
	if saveErr := ws.conversationStore().Save(context.WithoutCancel(ctx), record); saveErr != nil {
		return saveErr
	}
	if startErr != nil {
		return startErr
	}
	ws.startNativeFollower(snapshot)
	return nil
}

func (s *Server) nativeConversationView(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot, afterID int64) conversationView {
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil {
		return conversationView{Conversation: &conversationSnapshotView{ID: snapshot.id}, Events: []execution.RunEvent{}, Runs: []conversationRunView{}, Approvals: []execution.PendingApproval{}, Notice: err.Error()}
	}
	observed, observeErr := s.ensureNativeSession(ctx, ws, snapshot)
	if observeErr == nil {
		record.Connection = observed.Connection
		record.Recovery = observed.Recovery
		record.Work = observed.Work
		record.CurrentTurn = observed.CurrentTurn
	}
	page, pageErr := ws.conversationStore().ReadEvents(ctx, snapshot.id, afterID, nativeConversationPageSize)
	view := conversationView{
		Available: true, ProviderID: record.ProviderID, Model: record.NextTurn.Model,
		ModelOptions: cloneModelOptions(record.NextTurn.Options), Events: page.Events, LastID: page.LastID,
		HasMore: page.HasMore, Approvals: []execution.PendingApproval{}, Runs: []conversationRunView{},
		Conversation: &conversationSnapshotView{ID: record.ID, WorkingDir: record.WorkingDir,
			OpenedAt: record.OpenedAt.UTC().Format("2006-01-02T15:04:05.000Z"), SpecCode: record.SpecCode},
	}
	view.NextTurn = record.NextTurn
	view.Deliveries = append([]execution.SessionDelivery(nil), record.Deliveries...)
	if record.Session != nil {
		view.ProviderID = record.Session.ProviderID
		view.Conversation.WorkingDir = record.Session.Environment.WorkingDir
	}
	view.Conversation.State = execution.RunActive
	if record.Archive == execution.ArchiveArchived {
		view.Conversation.State = execution.RunClosed
	}
	if observeErr != nil {
		view.Notice = "the native session could not be resumed: " + observeErr.Error()
	}
	if pageErr != nil {
		view.Notice = "the durable timeline could not be read: " + pageErr.Error()
	}
	if observeErr == nil {
		view.Approvals = observed.PendingApprovals
		copy := observed
		view.Session = &copy
	} else if record.Session != nil {
		view.Session = &execution.SessionSnapshot{Session: *record.Session, Archive: record.Archive, Connection: record.Connection,
			Recovery: record.Recovery, Work: record.Work, CurrentTurn: record.CurrentTurn,
			PendingInputs: []execution.PendingInput{}, PendingApprovals: []execution.PendingApproval{}}
	}
	return view
}

func pastNativeConversationView(ws *workspaceSession, record conversationlog.Record, afterID int64) conversationView {
	page, err := ws.conversationStore().ReadEvents(context.Background(), record.ID, afterID, nativeConversationPageSize)
	view := conversationView{
		ProviderID: record.ProviderID, Model: record.NextTurn.Model, ModelOptions: cloneModelOptions(record.NextTurn.Options),
		Events: page.Events, LastID: page.LastID, HasMore: page.HasMore, Approvals: []execution.PendingApproval{}, Runs: []conversationRunView{},
		Conversation: &conversationSnapshotView{ID: record.ID, State: execution.RunClosed, WorkingDir: record.WorkingDir,
			OpenedAt: record.OpenedAt.UTC().Format("2006-01-02T15:04:05.000Z"), SpecCode: record.SpecCode},
	}
	view.NextTurn = record.NextTurn
	view.Deliveries = append([]execution.SessionDelivery(nil), record.Deliveries...)
	if record.Session != nil {
		view.Session = &execution.SessionSnapshot{Session: *record.Session, Archive: record.Archive, Connection: record.Connection,
			Recovery: record.Recovery, Work: record.Work, CurrentTurn: record.CurrentTurn,
			PendingInputs: []execution.PendingInput{}, PendingApprovals: []execution.PendingApproval{}}
	}
	if record.Session != nil {
		view.ProviderID = record.Session.ProviderID
		view.Conversation.WorkingDir = record.Session.Environment.WorkingDir
	}
	if err != nil {
		view.Notice = "the durable timeline could not be read: " + err.Error()
	}
	return view
}

func (s *Server) handleArchiveNativeConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	id := strings.TrimSpace(r.PathValue("id"))
	record, err := ws.conversationStore().GetMetadata(r.Context(), id)
	if err != nil {
		writeError(w, conversationNotFound(id, err))
		return
	}
	if !record.Native() {
		writeError(w, iox.NewConflict("the conversation "+id+" is a legacy transcript", "legacy transcripts can be deleted but have no native session to archive", nil))
		return
	}
	if snapshot, live := ws.conversation.get(id); live {
		if err := ws.releaseNativeConversation(r.Context(), snapshot); err != nil {
			writeError(w, iox.NewInternal("releasing the native conversation "+id, err))
			return
		}
		record, err = ws.conversationStore().GetMetadata(r.Context(), id)
		if err != nil {
			writeError(w, iox.NewInternal("reading the released conversation "+id, err))
			return
		}
	}
	record.Archive = execution.ArchiveArchived
	record.Connection = execution.ConnectionReleased
	if err := ws.conversationStore().Save(r.Context(), record); err != nil {
		writeError(w, iox.NewInternal("archiving the conversation "+id, err))
		return
	}
	writeJSON(w, http.StatusOK, pastNativeConversationView(ws, record, 0))
}

func (s *Server) handleReopenNativeConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	id := strings.TrimSpace(r.PathValue("id"))
	if _, live := ws.conversation.get(id); live {
		writeError(w, iox.NewConflict("the conversation "+id+" is already open", "continue it directly", nil))
		return
	}
	record, err := ws.conversationStore().GetMetadata(r.Context(), id)
	if err != nil {
		writeError(w, conversationNotFound(id, err))
		return
	}
	if !record.Native() {
		writeError(w, iox.NewConflict("the conversation "+id+" has no native session reference", "keep reading the legacy transcript or explicitly create a new seeded conversation", nil))
		return
	}
	if s.registry == nil {
		writeError(w, iox.NewConflict("no execution provider registry is available", "start View with the original provider registered", nil))
		return
	}
	provider, err := s.registry.Resolve(record.Session.ProviderID)
	if err != nil {
		writeError(w, iox.NewConflict("the original provider "+record.Session.ProviderID+" is not available", "restore that provider; changing the default does not move this session", err))
		return
	}
	sessions, ok := execution.SessionProviderFor(provider)
	if !ok {
		writeError(w, iox.NewConflict("the original provider no longer supports native sessions", "restore a compatible provider adapter", nil))
		return
	}
	record.Archive = execution.ArchiveOpen
	if err := ws.conversationStore().Save(r.Context(), record); err != nil {
		writeError(w, iox.NewInternal("reopening the conversation "+id, err))
		return
	}
	if err := ws.conversation.open(conversationHold{id: id, providerID: record.Session.ProviderID, sessionProvider: sessions,
		session: *record.Session, providerConfig: execution.CloneConfig(record.Session.Environment.ProviderConfig),
		model: record.NextTurn.Model, modelOptions: cloneModelOptions(record.NextTurn.Options), workingDir: record.Session.Environment.WorkingDir,
		openedAt: record.OpenedAt, specCode: record.SpecCode}); err != nil {
		writeError(w, conversationOpenRefusal(r.Context(), ws, err))
		return
	}
	snapshot, _ := ws.conversation.get(id)
	if _, err := s.ensureNativeSession(r.Context(), ws, snapshot); err != nil {
		ws.conversation.forget(id)
		writeError(w, iox.NewConflict("the native session could not be resumed", "restore its provider runtime and try again", err))
		return
	}
	ws.startNativeFollower(snapshot)
	writeJSON(w, http.StatusOK, s.nativeConversationView(r.Context(), ws, snapshot, 0))
}

func (s *Server) handleInterruptNativeConversation(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	id := strings.TrimSpace(r.PathValue("id"))
	snapshot, live := ws.conversation.get(id)
	if !live || snapshot.sessionProvider == nil {
		writeError(w, s.conversationGoneRefusal(r.Context(), ws, id, "interrupt its turn"))
		return
	}
	lock, err := ws.conversationStore().Lock(r.Context(), id)
	if err != nil {
		writeError(w, iox.NewInternal("locking the conversation "+id, err))
		return
	}
	record, err := ws.conversationStore().GetMetadata(r.Context(), id)
	if err != nil || record.CurrentTurn == nil || record.Work == execution.SessionIdle {
		_ = lock.Unlock()
		writeError(w, iox.NewConflict("the conversation "+id+" has no active turn", "start a turn before interrupting it", err))
		return
	}
	submissionID, err := nativeOperationID("submission-")
	if err != nil {
		_ = lock.Unlock()
		writeError(w, iox.NewInternal("generating the interrupt correlation", err))
		return
	}
	delivery := execution.SessionDelivery{SubmissionID: submissionID, TurnID: record.CurrentTurn.ID, State: execution.DeliveryUncertain}
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if err := ws.conversationStore().Save(r.Context(), record); err != nil {
		_ = lock.Unlock()
		writeError(w, iox.NewInternal("persisting the interrupt", err))
		return
	}
	delivery, interruptErr := snapshot.sessionProvider.InterruptTurn(r.Context(), execution.SessionCommandRequest{
		Session: snapshot.session, TurnID: record.CurrentTurn.ID, SubmissionID: submissionID,
	})
	if delivery.State != "" {
		record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	}
	if observed, readErr := snapshot.sessionProvider.ReadSession(r.Context(), execution.SessionRequest{Session: snapshot.session}); readErr == nil {
		record.Work, record.CurrentTurn = observed.Work, observed.CurrentTurn
		if observed.CurrentTurn != nil {
			record.Turns = upsertTurn(record.Turns, *observed.CurrentTurn)
		}
	}
	_ = ws.conversationStore().Save(context.WithoutCancel(r.Context()), record)
	_ = lock.Unlock()
	if interruptErr != nil {
		writeError(w, iox.NewConflict("the native turn was not confirmed interrupted", "reconcile the session before retrying", interruptErr))
		return
	}
	writeJSON(w, http.StatusAccepted, s.nativeConversationView(r.Context(), ws, snapshot, 0))
}

func (s *Server) respondNativeConversationApproval(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot, approvalID, optionID string) error {
	lock, err := ws.conversationStore().Lock(ctx, snapshot.id)
	if err != nil {
		return iox.NewInternal("locking the conversation "+snapshot.id, err)
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil || record.CurrentTurn == nil {
		return iox.NewConflict("the conversation has no active turn", "answer an approval requested by the current turn", err)
	}
	submissionID, err := nativeOperationID("submission-")
	if err != nil {
		return err
	}
	delivery := execution.SessionDelivery{SubmissionID: submissionID, TurnID: record.CurrentTurn.ID, State: execution.DeliveryUncertain}
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if err := ws.conversationStore().Save(ctx, record); err != nil {
		return err
	}
	delivery, commandErr := snapshot.sessionProvider.RespondSessionApproval(ctx, execution.SessionCommandRequest{
		Session: snapshot.session, TurnID: record.CurrentTurn.ID, SubmissionID: submissionID,
		InteractionID: approvalID, OptionID: optionID,
	})
	if delivery.State != "" {
		record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	}
	if saveErr := ws.conversationStore().Save(context.WithoutCancel(ctx), record); saveErr != nil {
		return saveErr
	}
	if commandErr != nil {
		return iox.NewConflict("the approval delivery was not confirmed", "reconcile the native session before retrying", commandErr)
	}
	return nil
}

func (s *Server) respondNativeConversationInput(ctx context.Context, ws *workspaceSession, snapshot conversationSnapshot, inputID string, payload json.RawMessage) error {
	lock, err := ws.conversationStore().Lock(ctx, snapshot.id)
	if err != nil {
		return iox.NewInternal("locking the conversation "+snapshot.id, err)
	}
	defer func() { _ = lock.Unlock() }()
	record, err := ws.conversationStore().GetMetadata(ctx, snapshot.id)
	if err != nil || record.CurrentTurn == nil {
		return iox.NewConflict("the conversation has no active turn", "answer input requested by the current turn", err)
	}
	submissionID, err := nativeOperationID("submission-")
	if err != nil {
		return err
	}
	delivery := execution.SessionDelivery{SubmissionID: submissionID, TurnID: record.CurrentTurn.ID, State: execution.DeliveryUncertain}
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if err := ws.conversationStore().Save(ctx, record); err != nil {
		return err
	}
	delivery, commandErr := snapshot.sessionProvider.RespondSessionInput(ctx, execution.SessionCommandRequest{
		Session: snapshot.session, TurnID: record.CurrentTurn.ID, SubmissionID: submissionID,
		InteractionID: inputID, Payload: payload,
	})
	if delivery.State != "" {
		record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	}
	if saveErr := ws.conversationStore().Save(context.WithoutCancel(ctx), record); saveErr != nil {
		return saveErr
	}
	if commandErr != nil {
		return iox.NewConflict("the input delivery was not confirmed", "reconcile the native session before retrying", commandErr)
	}
	return nil
}

type nativeNextTurnRequest struct {
	Model        string            `json:"model"`
	ModelOptions map[string]string `json:"model_options"`
}

type nativeConversationModelChoiceView struct {
	executionModelChoiceView
	Capabilities []execution.SessionCapability `json:"capabilities"`
	Environment  execution.SessionEnvironment  `json:"environment"`
	Skills       []execution.SessionSkill      `json:"skills"`
	SkillsKnown  bool                          `json:"skills_known"`
}

func (s *Server) nativeConversationDiscovery(ctx context.Context, ws *workspaceSession, id string) (conversationlog.Record, execution.SessionDiscovery, error) {
	record, err := ws.conversationStore().GetMetadata(ctx, id)
	if err != nil || !record.Native() {
		return conversationlog.Record{}, execution.SessionDiscovery{}, conversationNotFound(id, err)
	}
	if s.registry == nil {
		return conversationlog.Record{}, execution.SessionDiscovery{}, iox.NewConflict("no execution provider registry is available", "start View with the original provider registered", nil)
	}
	provider, err := s.registry.Resolve(record.Session.ProviderID)
	if err != nil {
		return conversationlog.Record{}, execution.SessionDiscovery{}, iox.NewConflict("the original provider "+record.Session.ProviderID+" is not available", "restore that provider; changing the default does not move this session", err)
	}
	sessions, ok := execution.SessionProviderFor(provider)
	if !ok {
		return conversationlog.Record{}, execution.SessionDiscovery{}, iox.NewConflict("the original provider no longer supports native sessions", "restore a compatible provider adapter", nil)
	}
	discovery, err := sessions.DiscoverSession(ctx, execution.SessionDiscoveryRequest{
		ProviderConfig: record.Session.Environment.ProviderConfig,
		WorkingDir:     record.Session.Environment.WorkingDir,
		Native:         &record.Session.Native,
	})
	if err != nil {
		return conversationlog.Record{}, execution.SessionDiscovery{}, iox.NewConflict("the native session catalog is not available", "restore its provider runtime and try again", err)
	}
	return record, discovery, nil
}

func (s *Server) handleGetNativeConversationModelChoice(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, discovery, err := s.nativeConversationDiscovery(r.Context(), ws, strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, err)
		return
	}
	view := nativeConversationModelChoiceView{
		executionModelChoiceView: executionModelChoiceView{
			ProviderID: record.Session.ProviderID, ModelField: execution.ModelFieldName,
			Model: record.NextTurn.Model, Options: cloneModelOptions(record.NextTurn.Options),
			ModelSource: "session", Models: discovery.Models, Available: len(discovery.Models) > 0,
		},
		Capabilities: execution.NormalizeSessionCapabilities(discovery.Capabilities),
		Environment:  discovery.Environment,
		Skills:       discovery.Skills,
		SkillsKnown:  discovery.SkillsKnown,
	}
	if len(discovery.Models) == 0 {
		view.UnavailableReason = "the session provider returned an empty model catalog"
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleUpdateNativeConversationNextTurn(w http.ResponseWriter, r *http.Request) {
	var body nativeNextTurnRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, err)
		return
	}
	ws := s.session()
	id := strings.TrimSpace(r.PathValue("id"))
	lock, err := ws.conversationStore().Lock(r.Context(), id)
	if err != nil {
		writeError(w, iox.NewInternal("locking the conversation "+id, err))
		return
	}
	defer func() { _ = lock.Unlock() }()
	record, discovery, err := s.nativeConversationDiscovery(r.Context(), ws, id)
	if err != nil {
		writeError(w, err)
		return
	}
	nextTurn := execution.TurnConfiguration{Model: strings.TrimSpace(body.Model), Options: cloneModelOptions(body.ModelOptions)}
	if err := execution.ValidateTurnConfiguration(discovery.Models, nextTurn); err != nil {
		writeError(w, iox.NewInvalidInput(err.Error(), "choose a model and options declared by this session provider", err))
		return
	}
	record.NextTurn = nextTurn
	if err := ws.conversationStore().Save(r.Context(), record); err != nil {
		writeError(w, iox.NewInternal("saving the next turn configuration", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"next_turn": record.NextTurn})
}
