package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

type nativeSession struct {
	mu             sync.Mutex
	metadata       execution.SessionMetadata
	cfg            settings
	connected      bool
	started        bool
	current        *execution.SessionTurn
	process        localrun.Process
	client         *streamSession
	events         []execution.RunEvent
	nextEventID    int64
	nextSubscriber int
	subscribers    map[int]chan execution.RunEvent
}

func (p *Provider) DiscoverSession(ctx context.Context, request execution.SessionDiscoveryRequest) (execution.SessionDiscovery, error) {
	if _, err := parseConfig(request.ProviderConfig); err != nil {
		return execution.SessionDiscovery{}, err
	}
	dir, err := p.sessionWorkingDir(request.WorkingDir)
	if err != nil {
		return execution.SessionDiscovery{}, err
	}
	if err := p.Available(ctx, request.ProviderConfig); err != nil {
		return execution.SessionDiscovery{}, err
	}
	return execution.SessionDiscovery{
		Capabilities: execution.NormalizeSessionCapabilities([]execution.SessionCapability{
			execution.SessionCapabilityResume, execution.SessionCapabilityTurnModel,
			execution.SessionCapabilityTurnOptions, execution.SessionCapabilitySkillDiscovery,
			execution.SessionCapabilitySkillInvocation, execution.SessionCapabilityApproval,
			execution.SessionCapabilitySteering, execution.SessionCapabilityInterrupt,
		}),
		Models:      execution.CloneModels(models),
		Skills:      discoverClaudeSkills(dir),
		Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: cloneProviderConfig(request.ProviderConfig), Location: "local"},
	}, nil
}

func (p *Provider) CreateSession(ctx context.Context, request execution.CreateSessionRequest) (execution.SessionSnapshot, error) {
	discovery, err := p.DiscoverSession(ctx, execution.SessionDiscoveryRequest{ProviderConfig: request.Environment.ProviderConfig, WorkingDir: request.Environment.WorkingDir})
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	id, err := newClaudeSessionID()
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	cfg, _ := parseConfig(discovery.Environment.ProviderConfig)
	metadata := execution.SessionMetadata{ConversationID: request.ConversationID, ProviderID: ProviderID, Environment: discovery.Environment, Native: execution.NativeSessionReference{Kind: "claude-session", ID: id}}
	session := newNativeSession(metadata, cfg, false)
	if err := p.connectNativeProcess(session, false); err != nil {
		return execution.SessionSnapshot{}, err
	}
	p.storeNativeSession(session)
	return session.snapshot(), nil
}

func (p *Provider) ResumeSession(ctx context.Context, request execution.ResumeSessionRequest) (execution.SessionSnapshot, error) {
	if request.Session.ProviderID != ProviderID || strings.TrimSpace(request.Session.Native.ID) == "" {
		return execution.SessionSnapshot{}, fmt.Errorf("invalid claude native session reference")
	}
	if _, err := os.Stat(request.Session.Environment.WorkingDir); err != nil {
		return execution.SessionSnapshot{}, fmt.Errorf("opening claude session working directory: %w", err)
	}
	if err := p.Available(ctx, request.Session.Environment.ProviderConfig); err != nil {
		return execution.SessionSnapshot{}, err
	}
	if session := p.nativeSession(request.Session.Native.ID); session != nil {
		session.mu.Lock()
		connected := session.process != nil
		session.mu.Unlock()
		if !connected {
			if err := p.connectNativeProcess(session, session.started); err != nil {
				return execution.SessionSnapshot{}, err
			}
		}
		return session.snapshot(), nil
	}
	cfg, err := parseConfig(request.Session.Environment.ProviderConfig)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	session := newNativeSession(request.Session, cfg, true)
	if err := p.connectNativeProcess(session, true); err != nil {
		return execution.SessionSnapshot{}, err
	}
	p.storeNativeSession(session)
	return session.snapshot(), nil
}

func (p *Provider) ReadSession(_ context.Context, request execution.SessionRequest) (execution.SessionSnapshot, error) {
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return execution.SessionSnapshot{}, fmt.Errorf("claude session %q is not connected", request.Session.Native.ID)
	}
	return session.snapshot(), nil
}

func (p *Provider) StartTurn(ctx context.Context, request execution.StartTurnRequest) (execution.SessionTurnStarted, error) {
	session := p.nativeSession(request.Session.Native.ID)
	delivery := execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}
	if session == nil {
		return execution.SessionTurnStarted{Delivery: delivery}, fmt.Errorf("claude session %q is not connected", request.Session.Native.ID)
	}

	session.mu.Lock()
	if session.current != nil && session.current.State == execution.TurnActive {
		session.mu.Unlock()
		return execution.SessionTurnStarted{Delivery: delivery}, fmt.Errorf("claude session already has an active turn")
	}
	cfg := session.cfg
	if request.Model != "" {
		cfg.Model = request.Model
	}
	if effort, ok := request.Options["effort"]; ok {
		cfg.Effort = effort
	}
	if err := validateNativeTurn(cfg, request.Skills, request.Session.Environment.WorkingDir); err != nil {
		session.mu.Unlock()
		return execution.SessionTurnStarted{Delivery: delivery}, err
	}
	resume := session.started
	process := session.process
	client := session.client
	if process == nil || cfg.Model != session.cfg.Model || cfg.Effort != session.cfg.Effort {
		if process != nil {
			_, _, _ = p.shutdown(process)
			session.process, session.client = nil, nil
		}
		session.cfg = cfg
		session.mu.Unlock()
		if err := p.connectNativeProcess(session, resume); err != nil {
			return execution.SessionTurnStarted{Delivery: delivery}, err
		}
		session.mu.Lock()
		process, client = session.process, session.client
	}
	client.correlate(request.TurnID, request.SubmissionID)
	message := nativeTurnMessage(request)
	turn := execution.SessionTurn{ID: request.TurnID, ExecutionID: request.ExecutionID, State: execution.TurnActive, Requested: execution.TurnConfiguration{Model: request.Model, Options: cloneStringMap(request.Options)}, StartedAt: p.now().UTC()}
	session.process, session.client, session.current = process, client, &turn
	session.connected = true
	session.mu.Unlock()
	if err := client.startNative(ctx, message); err != nil {
		_, _, _ = p.shutdown(process)
		session.failTurn(err)
		delivery.State = execution.DeliveryUncertain
		return execution.SessionTurnStarted{Turn: turn, Delivery: delivery}, err
	}
	observedID, observedModel := client.initializedAs()
	if observedID != "" && observedID != request.Session.Native.ID {
		_, _, _ = p.shutdown(process)
		err := fmt.Errorf("claude resumed session %q instead of %q", observedID, request.Session.Native.ID)
		session.failTurn(err)
		delivery.State = execution.DeliveryUncertain
		return execution.SessionTurnStarted{Turn: turn, Delivery: delivery}, err
	}
	turn.Applied = execution.TurnConfiguration{Model: cfg.Model, Options: mapIfSet("effort", cfg.Effort)}
	if observedModel != "" {
		turn.Applied.Model = observedModel
	}
	session.mu.Lock()
	session.started = true
	session.cfg.Model = cfg.Model
	session.cfg.Effort = cfg.Effort
	session.current = &turn
	session.mu.Unlock()
	delivery.State = execution.DeliveryConfirmed
	go p.finishNativeTurn(session, client, process)
	return execution.SessionTurnStarted{Turn: turn, Delivery: delivery}, nil
}

func (p *Provider) connectNativeProcess(session *nativeSession, resume bool) error {
	process, err := p.starter.Start(context.Background(), session.metadata.Environment.WorkingDir, session.cfg.Command, buildNativeSessionArgs(session.cfg, session.metadata.Native.ID, resume))
	if err != nil {
		return fmt.Errorf("starting claude native session: %w", err)
	}
	dummy := localrun.NewSession(session.metadata.ConversationID, p.now)
	client := newStreamSession(process, dummy, true)
	client.now = p.now
	client.appendEvent = session.append
	go client.consume()
	session.mu.Lock()
	session.process, session.client, session.connected = process, client, true
	session.mu.Unlock()
	return nil
}

func (p *Provider) finishNativeTurn(session *nativeSession, client *streamSession, process localrun.Process) {
	<-client.TurnDone()
	completed := client.Completed()
	_, _, _ = p.shutdown(process)
	now := p.now().UTC()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.client != client {
		return
	}
	state := execution.TurnCompleted
	if !completed {
		state = execution.TurnInterrupted
	}
	if session.current != nil {
		session.current.State = state
		session.current.CompletedAt = &now
	}
	session.process, session.client = nil, nil
}

func (p *Provider) StreamSessionEvents(ctx context.Context, request execution.SessionRequest, afterID int64, sink func(execution.RunEvent) error) error {
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return fmt.Errorf("claude session %q is not connected", request.Session.Native.ID)
	}
	return session.stream(ctx, afterID, sink)
}

func (p *Provider) SteerTurn(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.nativeCommand(request, func(client *streamSession) error { return client.Send(ctx, request.Message) })
}

func (p *Provider) RespondSessionInput(_ context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}, fmt.Errorf("claude stream-json does not expose structured input requests")
}

func (p *Provider) RespondSessionApproval(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.nativeCommand(request, func(client *streamSession) error {
		return client.RespondApproval(ctx, request.InteractionID, request.OptionID)
	})
}

func (p *Provider) InterruptTurn(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.nativeCommand(request, func(client *streamSession) error { return client.Interrupt(ctx) })
}

func (p *Provider) ReleaseSession(_ context.Context, request execution.SessionRequest) error {
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return nil
	}
	session.mu.Lock()
	process := session.process
	if session.current != nil && session.current.State == execution.TurnActive {
		now := p.now().UTC()
		session.current.State = execution.TurnFailed
		session.current.Error = "the Claude runtime was released before the turn completed"
		session.current.CompletedAt = &now
	}
	session.process = nil
	session.client = nil
	session.connected = false
	session.mu.Unlock()
	if process != nil {
		_, _, _ = p.shutdown(process)
	}
	return nil
}

func (p *Provider) nativeCommand(request execution.SessionCommandRequest, command func(*streamSession) error) (execution.SessionDelivery, error) {
	delivery := execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return delivery, fmt.Errorf("claude session %q is not connected", request.Session.Native.ID)
	}
	session.mu.Lock()
	client := session.client
	current := session.current
	session.mu.Unlock()
	if client == nil || current == nil || current.ID != request.TurnID || current.State != execution.TurnActive {
		return delivery, fmt.Errorf("claude turn %q is not active", request.TurnID)
	}
	if err := command(client); err != nil {
		delivery.State = execution.DeliveryUncertain
		return delivery, err
	}
	delivery.State = execution.DeliveryConfirmed
	return delivery, nil
}

func newNativeSession(metadata execution.SessionMetadata, cfg settings, resumed bool) *nativeSession {
	return &nativeSession{metadata: metadata, cfg: cfg, connected: true, started: resumed, subscribers: make(map[int]chan execution.RunEvent)}
}

func (s *nativeSession) snapshot() execution.SessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection := execution.ConnectionDisconnected
	if s.connected {
		connection = execution.ConnectionConnected
	}
	work := execution.SessionIdle
	if s.current != nil && s.current.State == execution.TurnActive {
		work = execution.SessionTurnActive
	}
	var turn *execution.SessionTurn
	if s.current != nil {
		copy := *s.current
		turn = &copy
	}
	var approvals []execution.PendingApproval
	if s.client != nil {
		approvals = s.client.PendingApprovals()
	}
	if len(approvals) > 0 {
		work = execution.SessionWaitingApproval
	}
	if approvals == nil {
		approvals = []execution.PendingApproval{}
	}
	return execution.SessionSnapshot{Session: s.metadata, Archive: execution.ArchiveOpen, Connection: connection, Recovery: execution.RecoveryResumable, Work: work, CurrentTurn: turn, PendingInputs: []execution.PendingInput{}, PendingApprovals: approvals}
}

func (s *nativeSession) failTurn(err error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		s.current.State = execution.TurnFailed
		s.current.Error = err.Error()
		s.current.CompletedAt = &now
	}
	s.process = nil
	s.client = nil
}

func (s *nativeSession) append(event execution.RunEvent) {
	s.mu.Lock()
	s.nextEventID++
	event.ID = s.nextEventID
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	s.events = append(s.events, event)
	subscribers := make([]chan execution.RunEvent, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		subscribers = append(subscribers, ch)
	}
	s.mu.Unlock()
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *nativeSession) stream(ctx context.Context, afterID int64, sink func(execution.RunEvent) error) error {
	s.mu.Lock()
	if afterID > s.nextEventID {
		s.nextEventID = afterID
	}
	backlog := append([]execution.RunEvent(nil), s.events...)
	s.nextSubscriber++
	id := s.nextSubscriber
	// The View follower drains this into the durable journal. Keep one full
	// conversation page worth of burst capacity so a fast tool stream cannot
	// punch a hole in the persisted cursor while the journal is flushing.
	live := make(chan execution.RunEvent, 4096)
	s.subscribers[id] = live
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.subscribers, id); s.mu.Unlock() }()
	for _, event := range backlog {
		if event.ID > afterID {
			if err := sink(event); err != nil {
				return err
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-live:
			if event.ID > afterID {
				if err := sink(event); err != nil {
					return err
				}
				afterID = event.ID
			}
		}
	}
}

func (p *Provider) nativeSession(id string) *nativeSession {
	p.nativeSessionsMu.Lock()
	defer p.nativeSessionsMu.Unlock()
	return p.nativeSessions[id]
}
func (p *Provider) storeNativeSession(session *nativeSession) {
	p.nativeSessionsMu.Lock()
	p.nativeSessions[session.metadata.Native.ID] = session
	p.nativeSessionsMu.Unlock()
}

func (p *Provider) sessionWorkingDir(requested string) (string, error) {
	dir := strings.TrimSpace(requested)
	if dir == "" {
		var err error
		dir, err = p.workingDir()
		if err != nil {
			return "", err
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("opening claude session working directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("opening claude session working directory: %s is not a directory", dir)
	}
	return dir, nil
}

func newClaudeSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	text := hex.EncodeToString(value[:])
	return text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:], nil
}

func cloneProviderConfig(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
func mapIfSet(key, value string) map[string]string {
	if value == "" {
		return nil
	}
	return map[string]string{key: value}
}

func validateNativeTurn(cfg settings, skills []execution.SessionSkill, dir string) error {
	if _, err := parseModel(cfg.Model); err != nil {
		return err
	}
	if _, err := parseEffort(optionalString(cfg.Effort)); err != nil {
		return err
	}
	available := make(map[string]bool)
	for _, skill := range discoverClaudeSkills(dir) {
		available[skill.Name] = true
	}
	for _, skill := range skills {
		if !available[skill.Name] {
			return fmt.Errorf("claude skill %q is not available in this workspace", skill.Name)
		}
	}
	return nil
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nativeTurnMessage(request execution.StartTurnRequest) string {
	if len(request.Skills) == 0 {
		return request.Message
	}
	names := make([]string, 0, len(request.Skills))
	for _, skill := range request.Skills {
		names = append(names, "/"+skill.Name)
	}
	return strings.Join(names, "\n") + "\n\n" + request.Message
}

func discoverClaudeSkills(dir string) []execution.SessionSkill {
	entries, err := os.ReadDir(filepath.Join(dir, ".claude", "skills"))
	if err != nil {
		return []execution.SessionSkill{}
	}
	out := make([]execution.SessionSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, ".claude", "skills", entry.Name(), "SKILL.md")
		if _, err := os.Stat(path); err == nil {
			out = append(out, execution.SessionSkill{Name: entry.Name(), Path: path})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
