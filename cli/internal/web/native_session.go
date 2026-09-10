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
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

const nativeConversationPageSize = 500

// nativeTurnSettleAttempts and nativeTurnSettleInterval bound how long a
// follower keeps asking, after a turn has ended, whether the action that turn
// was carrying out can be closed. They are small because the gap they cover is
// the provider's own — the moment between its last event and its verdict on the
// turn — and an action still running after them is one that is legitimately
// waiting for a person.
const (
	nativeTurnSettleAttempts = 20
	nativeTurnSettleInterval = 100 * time.Millisecond
)

// nativeSessionFollowers are the only readers of provider event streams. They
// append on receipt, so browser polling is never part of the durability path.
type nativeSessionFollowers struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running map[string]context.CancelFunc
	// wg is what makes closeAll a real end and not only a cancellation. A
	// follower writes into this workspace's journal, so one still running after
	// the workspace has been left would write the timeline of a project the
	// viewer no longer serves.
	wg sync.WaitGroup
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
	f.wg.Add(1)
	f.mu.Unlock()
	go func() {
		defer f.wg.Done()
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
	// Waited on outside the lock: a follower on its way out takes the same
	// mutex to unregister itself, so waiting under it would deadlock.
	f.wg.Wait()
}

func (ws *workspaceSession) restoreNativeConversations(registry *execution.Registry) error {
	if ws == nil || registry == nil || ws.conversationStore() == nil {
		return nil
	}
	records, err := ws.conversationStore().List(context.Background())
	if err != nil {
		return err
	}

	ctx := context.Background()
	for _, record := range records {
		if !record.Native() {
			continue
		}
		// Claimed before anything is decided about this conversation: a viewer
		// that does not own the runtime must neither reconcile its turns nor
		// follow its stream, because the process that does own it is still
		// working in that very session.
		if err := ws.nativeRuntime.take(ctx, ws.conversationStore(), record.ID); err != nil {
			continue
		}
		// Before the provider is resolved and before the archive is looked at,
		// so the conversations that would otherwise be skipped — an unavailable
		// provider, an archived thread, a session that will turn out to be
		// unresumable — are reconciled too. A turn this record still calls
		// running belongs to a runtime that died with the previous View process.
		ws.reconcileInterruptedTurn(ctx, record)
		if record.Archive == execution.ArchiveArchived {
			ws.nativeRuntime.drop(record.ID)
			continue
		}
		provider, resolveErr := registry.Resolve(record.Session.ProviderID)
		if resolveErr != nil {
			ws.nativeRuntime.drop(record.ID)
			continue
		}
		sessions, supported := execution.SessionProviderFor(provider)
		if !supported {
			ws.nativeRuntime.drop(record.ID)
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
			// Every hold taken by this restore goes back, and not only the one
			// that failed: the session being built is discarded by its caller,
			// so nothing would be left to release the others.
			ws.nativeRuntime.dropAll()
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
		// A provider whose stream stays open for the life of the session never
		// returns from the loop below, so the end of an action cannot be noticed
		// after the stream: it has to be noticed *in* it. The signal is the turn
		// end the timeline itself carries, and settling on it costs no polling
		// and asks the provider nothing.
		ended := make(chan struct{}, 1)
		settling := make(chan struct{})
		// Its own cancellation, so the settler ends when *this* follower ends
		// and not only when the whole workspace does. Without it a stream that
		// failed would leave the follower blocked on the wait below, and its id
		// registered — which is what stops the next message from starting a new
		// follower for the same conversation.
		settleCtx, stopSettling := context.WithCancel(ctx)
		go func() {
			defer close(settling)
			for {
				select {
				case <-settleCtx.Done():
					return
				case <-ended:
					// Retried, because the two facts arrive separately: the
					// provider appends the last event of a turn and marks the
					// turn terminal a moment later. The first attempt often sees
					// a turn that has ended without yet saying how, and an
					// action left running by that attempt has nothing else
					// coming to wake it.
					for attempt := 0; attempt < nativeTurnSettleAttempts; attempt++ {
						ws.settleSessionActions(settleCtx, snapshot.id)
						if !ws.hasRunningAction(settleCtx, snapshot.id) {
							break
						}
						select {
						case <-settleCtx.Done():
							return
						case <-time.After(nativeTurnSettleInterval):
						}
					}
				}
			}
		}()
		// Declared in this order so they unwind in the other one: the settler is
		// told to stop first, and only then is it waited for.
		defer func() { <-settling }()
		defer stopSettling()
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
				// The summary of the record — its title, how many messages it
				// holds, when the last one was said — is written here and not
				// only when the stream returns, because a provider whose stream
				// stays open for the life of the session never returns from it.
				// A conversation would otherwise keep for ever the dated name it
				// was given before anybody had spoken.
				//
				// The two kinds are the only ones that can change the answer and
				// are rare, one per turn each; the flush is what makes the
				// journal say, to the read that follows, what has just been
				// appended.
				if event.Kind == localrun.KindUserMessage || event.Kind == localrun.KindTurnEnd {
					_ = appender.Flush()
					_ = ws.updateNativeRecordFromEvent(context.WithoutCancel(ctx), record, event)
				}
				if event.Kind == localrun.KindTurnEnd {
					select {
					case ended <- struct{}{}:
					default:
					}
				}
				afterID = event.ID
				copy := event
				lastReceived = &copy
				return nil
			})
			_ = appender.Close()
			if lastReceived != nil {
				// Detached from the follower's own cancellation: the stream
				// returns *because* the workspace is being left, and what it saw
				// last would otherwise be lost on the way out.
				_ = ws.updateNativeRecordFromEvent(context.WithoutCancel(ctx), record, *lastReceived)
			}
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				_ = ws.markNativeDisconnected(context.Background(), snapshot.id)
				// A stream that ended mid-turn fails that turn, and an action
				// running in it has therefore ended too.
				ws.settleSessionActions(context.Background(), snapshot.id)
				return
			}
			if observed, readErr := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session}); readErr == nil {
				_ = ws.persistNativeSnapshot(ctx, snapshot.id, observed)
			}
			// The action, if this conversation is carrying one out, is closed
			// from what the record now says: a turn that ended, a receipt the
			// connector backs, an interrupt. It happens here rather than in a
			// handler because the end of a turn is observed by the follower and
			// by nothing else.
			ws.settleSessionActions(ctx, snapshot.id)
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
	// The runtime is gone, so the ownership of it goes too: another viewer of
	// this workspace may now take the session up.
	ws.nativeRuntime.drop(snapshot.id)
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
	// The observed turn replaces the recorded one, except for the action it is
	// carrying out: that link was written when the turn was started and belongs
	// to ARchetipo, so a provider that reports the turn without echoing it back
	// must not be able to detach it.
	if snapshot.CurrentTurn != nil {
		observed := *snapshot.CurrentTurn
		if observed.ExecutionID == "" && record.CurrentTurn != nil && record.CurrentTurn.ID == observed.ID {
			observed.ExecutionID = record.CurrentTurn.ExecutionID
		}
		record.CurrentTurn = &observed
	} else {
		record.CurrentTurn = nil
	}
	if snapshot.CurrentTurn != nil {
		for index := range record.Deliveries {
			if record.Deliveries[index].State == execution.DeliveryUncertain && record.Deliveries[index].TurnID == snapshot.CurrentTurn.ID {
				record.Deliveries[index].State = execution.DeliveryConfirmed
			}
		}
	}
	if record.CurrentTurn != nil && terminalTurn(record.CurrentTurn.State) {
		record.Turns = upsertTurn(record.Turns, *record.CurrentTurn)
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
	// Before the provider is asked anything: resuming a session whose directory
	// has gone fails inside the harness, as a chdir error that names neither
	// this conversation nor what has to be restored.
	if err := requireSessionDirectory(snapshot); err != nil {
		return err
	}
	observed, readErr := snapshot.sessionProvider.ReadSession(ctx, execution.SessionRequest{Session: snapshot.session})
	if readErr != nil || observed.Connection != execution.ConnectionConnected {
		observed, err = snapshot.sessionProvider.ResumeSession(ctx, execution.ResumeSessionRequest{Session: snapshot.session})
		if err != nil {
			record.Connection = execution.ConnectionDisconnected
			_ = ws.conversationStore().Save(context.WithoutCancel(ctx), record)
			return err
		}
	}
	// What the session is doing is taken from the session itself and not from
	// the record, in both branches. The record is written by the follower after
	// it has appended the events of a turn, so between the last event of a turn
	// and that write it still says the turn is running — and a message sent in
	// that window would be steered into a turn that has already ended.
	record.Connection = observed.Connection
	record.Recovery = observed.Recovery
	record.Work = observed.Work
	if observed.CurrentTurn != nil {
		live := *observed.CurrentTurn
		if live.ExecutionID == "" && record.CurrentTurn != nil && record.CurrentTurn.ID == live.ID {
			live.ExecutionID = record.CurrentTurn.ExecutionID
		}
		record.CurrentTurn = &live
	}
	ws.reconcileUncertainDeliveries(ctx, &record, observed)
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
	// A turn typed into a thread that is carrying out an action belongs to that
	// action: it is the answer to what the agent asked, and losing the link
	// would leave the answer floating beside the work it answers. The caller
	// that starts an action names it explicitly; every other caller inherits
	// whatever the conversation is still carrying out, and inherits nothing
	// when it is carrying out nothing.
	executionID := strings.TrimSpace(body.executionID)
	if executionID == "" {
		executionID = pendingActionExecutionID(ctx, ws, record)
	}
	requested := record.NextTurn
	turn := execution.SessionTurn{ID: turnID, ExecutionID: executionID, State: execution.TurnActive, Requested: requested, StartedAt: time.Now().UTC()}
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
		Session: snapshot.session, TurnID: turnID, SubmissionID: submissionID, ExecutionID: executionID,
		Message: body.Message, Model: requested.Model, Options: cloneModelOptions(requested.Options), Skills: selectedSkills,
	})
	if started.Delivery.State != "" {
		delivery = started.Delivery
	}
	record.Deliveries = upsertDelivery(record.Deliveries, delivery)
	if started.Turn.ID != "" {
		observed := started.Turn
		// The link is ours, not the provider's: a provider that reports the turn
		// without echoing the execution back must not be able to detach the turn
		// from the action it was started for.
		if observed.ExecutionID == "" {
			observed.ExecutionID = executionID
		}
		record.CurrentTurn = &observed
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

// reconcileUncertainDeliveries settles the commands a crash left in doubt,
// against the native session itself.
//
// A command is written UNCERTAIN before it is submitted, because from that
// moment on a crash cannot prove whether the harness saw it, and an automatic
// retry could double an effect. What the boundary must not do is keep the
// conversation shut for ever: without a way out, one crashed submission would
// leave a thread that refuses every further message.
//
// Three facts together, and nothing less, settle one: the turn it names is over
// in the durable record, the session is not running it, and the timeline holds
// not one event of it. A turn that ended without ever existing anywhere is a
// turn that never began, so the command never landed and its delivery becomes
// UNSENT — the person writes again, and this time it is a first attempt and not
// a repetition.
//
// Everything else stays UNCERTAIN. A submission that has just failed in this
// very process is the case the boundary was written for: its turn is still the
// live one, nobody has declared it over, and only a person can say what the
// harness did with it.
func (ws *workspaceSession) reconcileUncertainDeliveries(ctx context.Context, record *conversationlog.Record, observed execution.SessionSnapshot) {
	if record == nil {
		return
	}
	uncertain := map[string]struct{}{}
	for _, delivery := range record.Deliveries {
		if delivery.State != execution.DeliveryUncertain {
			continue
		}
		state, known := recordedTurnState(*record, delivery.TurnID)
		if !known || !terminalTurn(state) {
			continue
		}
		if observed.CurrentTurn != nil && observed.CurrentTurn.ID == delivery.TurnID && !terminalTurn(observed.CurrentTurn.State) {
			continue
		}
		uncertain[delivery.TurnID] = struct{}{}
	}
	if len(uncertain) == 0 {
		return
	}
	page, err := ws.conversationStore().ReadEvents(ctx, record.ID, 0, 0)
	if err != nil {
		return
	}
	for _, event := range page.Events {
		delete(uncertain, event.TurnID)
	}
	if len(uncertain) == 0 {
		return
	}
	for index := range record.Deliveries {
		if record.Deliveries[index].State != execution.DeliveryUncertain {
			continue
		}
		if _, unlanded := uncertain[record.Deliveries[index].TurnID]; unlanded {
			record.Deliveries[index].State = execution.DeliveryUnsent
		}
	}
	_ = ws.conversationStore().Save(ctx, *record)
}

// recordedTurnState is what the durable record says about one turn, and whether
// it says anything at all.
func recordedTurnState(record conversationlog.Record, turnID string) (execution.TurnState, bool) {
	if record.CurrentTurn != nil && record.CurrentTurn.ID == turnID {
		return record.CurrentTurn.State, true
	}
	for _, turn := range record.Turns {
		if turn.ID == turnID {
			return turn.State, true
		}
	}
	return "", false
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
	// Reading the thread is also the moment to close whatever action of the
	// process has finished in it. The follower normally gets there first, but a
	// viewer restarted while an action was in flight has no follower that saw
	// the turn end, and the record must not stay RUNNING because of that.
	ws.settleSessionActions(ctx, snapshot.id)
	if refreshed, refreshErr := ws.conversationStore().GetMetadata(ctx, snapshot.id); refreshErr == nil {
		record.ExecutionIDs = refreshed.ExecutionIDs
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
	view.Runs = s.nativeConversationRuns(ctx, ws, record)
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
	// Claimed before the archive flag is lifted: reopening is where a second
	// viewer would otherwise start its own runtime on a session this workspace
	// is already holding elsewhere.
	if err := ws.takeNativeRuntime(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	record.Archive = execution.ArchiveOpen
	if err := ws.conversationStore().Save(r.Context(), record); err != nil {
		ws.nativeRuntime.drop(id)
		writeError(w, iox.NewInternal("reopening the conversation "+id, err))
		return
	}
	if err := ws.conversation.open(conversationHold{id: id, providerID: record.Session.ProviderID, sessionProvider: sessions,
		session: *record.Session, providerConfig: execution.CloneConfig(record.Session.Environment.ProviderConfig),
		model: record.NextTurn.Model, modelOptions: cloneModelOptions(record.NextTurn.Options), workingDir: record.Session.Environment.WorkingDir,
		openedAt: record.OpenedAt, specCode: record.SpecCode}); err != nil {
		ws.nativeRuntime.drop(id)
		writeError(w, conversationOpenRefusal(r.Context(), ws, err))
		return
	}
	snapshot, _ := ws.conversation.get(id)
	if err := requireSessionDirectory(snapshot); err != nil {
		ws.conversation.forget(id)
		ws.nativeRuntime.drop(id)
		writeError(w, err)
		return
	}
	if _, err := s.ensureNativeSession(r.Context(), ws, snapshot); err != nil {
		ws.conversation.forget(id)
		ws.nativeRuntime.drop(id)
		writeError(w, iox.NewConflict("the native session could not be resumed", "restore its provider runtime and try again; nothing of this conversation has been deleted", err))
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
		// There is no turn to interrupt, but there may still be an action of the
		// process waiting for an answer that is not coming: the agent asked
		// something and stopped speaking. Stopping is then cancelling that
		// action, which closes its record and leaves the conversation itself
		// alive and writable — the whole point of a session that outlives the
		// work run in it.
		if err == nil && ws.cancelSessionAction(r.Context(), id) {
			writeJSON(w, http.StatusAccepted, s.nativeConversationView(r.Context(), ws, snapshot, 0))
			return
		}
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
		record.Work = observed.Work
		if observed.CurrentTurn != nil {
			interrupted := *observed.CurrentTurn
			// The link to the action survives the interrupt for the same reason
			// it survives an ordinary observation: it is ARchetipo's, not the
			// provider's, and an interrupted turn that forgot which action it
			// was running would leave that action's record open for ever.
			if interrupted.ExecutionID == "" && record.CurrentTurn != nil && record.CurrentTurn.ID == interrupted.ID {
				interrupted.ExecutionID = record.CurrentTurn.ExecutionID
			}
			record.CurrentTurn = &interrupted
			record.Turns = upsertTurn(record.Turns, interrupted)
		} else {
			record.CurrentTurn = nil
		}
	}
	_ = ws.conversationStore().Save(context.WithoutCancel(r.Context()), record)
	_ = lock.Unlock()
	// The action the interrupted turn was carrying out ends with it: the record
	// is closed here rather than left to the follower, so the board is right by
	// the time this response is read.
	ws.settleSessionActions(r.Context(), id)
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
