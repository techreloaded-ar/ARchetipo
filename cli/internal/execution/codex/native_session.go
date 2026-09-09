package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	current        *execution.SessionTurn
	process        localrun.Process
	client         *appServer
	events         []execution.RunEvent
	nextEventID    int64
	nextSubscriber int
	subscribers    map[int]chan execution.RunEvent
	approvals      map[string]execution.PendingApproval
	inputs         map[string]execution.PendingInput
	requests       map[string]rpcMessage
}

func (p *Provider) DiscoverSession(ctx context.Context, request execution.SessionDiscoveryRequest) (execution.SessionDiscovery, error) {
	cfg, err := parseConfig(request.ProviderConfig)
	if err != nil {
		return execution.SessionDiscovery{}, err
	}
	dir, err := p.sessionWorkingDir(request.WorkingDir)
	if err != nil {
		return execution.SessionDiscovery{}, err
	}
	if err := p.Available(ctx, request.ProviderConfig); err != nil {
		return execution.SessionDiscovery{}, err
	}
	var process localrun.Process
	var client *appServer
	if request.Native != nil {
		if session := p.nativeSession(request.Native.ID); session != nil {
			session.mu.Lock()
			client = session.client
			session.mu.Unlock()
		}
	}
	if client == nil {
		process, client, err = p.startNativeClient(ctx, cfg, dir)
		if err != nil {
			return execution.SessionDiscovery{}, err
		}
		defer func() { _, _, _ = p.shutdown(process) }()
	}
	models, err := client.discoverModels(ctx)
	if err != nil {
		return execution.SessionDiscovery{}, err
	}
	skills, err := client.discoverSkills(ctx, dir)
	if err != nil {
		return execution.SessionDiscovery{}, err
	}
	return execution.SessionDiscovery{
		Capabilities: execution.NormalizeSessionCapabilities([]execution.SessionCapability{
			execution.SessionCapabilityResume, execution.SessionCapabilityTurnModel,
			execution.SessionCapabilityTurnOptions, execution.SessionCapabilitySkillDiscovery,
			execution.SessionCapabilitySkillInvocation, execution.SessionCapabilityInput,
			execution.SessionCapabilityApproval, execution.SessionCapabilitySteering,
			execution.SessionCapabilityInterrupt,
		}),
		Models: models, Skills: skills, SkillsKnown: true,
		Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: cloneCodexProviderConfig(request.ProviderConfig), Location: "local"},
	}, nil
}

func (p *Provider) CreateSession(ctx context.Context, request execution.CreateSessionRequest) (execution.SessionSnapshot, error) {
	cfg, err := parseConfig(request.Environment.ProviderConfig)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	dir, err := p.sessionWorkingDir(request.Environment.WorkingDir)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	if err := p.Available(ctx, request.Environment.ProviderConfig); err != nil {
		return execution.SessionSnapshot{}, err
	}
	process, client, err := p.startNativeClient(ctx, cfg, dir)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	thread, err := client.openThread(ctx, cfg, dir, "", false, "untrusted")
	if err != nil {
		_, _, _ = p.shutdown(process)
		return execution.SessionSnapshot{}, err
	}
	metadata := execution.SessionMetadata{
		ConversationID: request.ConversationID, ProviderID: ProviderID,
		Environment: execution.SessionEnvironment{WorkingDir: dir, ProviderConfig: cloneCodexProviderConfig(request.Environment.ProviderConfig), Location: "local"},
		Native:      execution.NativeSessionReference{Kind: "codex.thread", ID: thread.ID},
	}
	session := newCodexNativeSession(metadata, cfg)
	session.process, session.client = process, client
	session.bindClient()
	p.storeNativeSession(session)
	return session.snapshot(), nil
}

func (p *Provider) ResumeSession(ctx context.Context, request execution.ResumeSessionRequest) (execution.SessionSnapshot, error) {
	if request.Session.ProviderID != ProviderID || request.Session.Native.Kind != "codex.thread" || strings.TrimSpace(request.Session.Native.ID) == "" {
		return execution.SessionSnapshot{}, fmt.Errorf("invalid codex native session reference")
	}
	if session := p.nativeSession(request.Session.Native.ID); session != nil {
		session.mu.Lock()
		connected := session.connected
		session.mu.Unlock()
		if connected {
			return session.snapshot(), nil
		}
	}
	cfg, err := parseConfig(request.Session.Environment.ProviderConfig)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	dir, err := p.sessionWorkingDir(request.Session.Environment.WorkingDir)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	if err := p.Available(ctx, request.Session.Environment.ProviderConfig); err != nil {
		return execution.SessionSnapshot{}, err
	}
	process, client, err := p.startNativeClient(ctx, cfg, dir)
	if err != nil {
		return execution.SessionSnapshot{}, err
	}
	thread, err := client.openThread(ctx, cfg, dir, request.Session.Native.ID, false, "untrusted")
	if err != nil {
		_, _, _ = p.shutdown(process)
		return execution.SessionSnapshot{}, err
	}
	if thread.Model != "" {
		cfg.Model = thread.Model
	}
	if thread.Effort != "" {
		cfg.ReasoningEffort = thread.Effort
	}
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		session = newCodexNativeSession(request.Session, cfg)
		p.storeNativeSession(session)
	}
	session.mu.Lock()
	session.cfg, session.process, session.client, session.connected = cfg, process, client, true
	session.mu.Unlock()
	session.bindClient()
	return session.snapshot(), nil
}

func (p *Provider) ReadSession(_ context.Context, request execution.SessionRequest) (execution.SessionSnapshot, error) {
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return execution.SessionSnapshot{}, fmt.Errorf("codex session %q is not connected", request.Session.Native.ID)
	}
	return session.snapshot(), nil
}

func (p *Provider) StartTurn(ctx context.Context, request execution.StartTurnRequest) (execution.SessionTurnStarted, error) {
	delivery := execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return execution.SessionTurnStarted{Delivery: delivery}, fmt.Errorf("codex session %q is not connected", request.Session.Native.ID)
	}
	session.mu.Lock()
	if !session.connected || session.client == nil {
		session.mu.Unlock()
		return execution.SessionTurnStarted{Delivery: delivery}, fmt.Errorf("codex session %q is not connected", request.Session.Native.ID)
	}
	if session.current != nil && session.current.State == execution.TurnActive {
		session.mu.Unlock()
		return execution.SessionTurnStarted{Delivery: delivery}, fmt.Errorf("codex session already has an active turn")
	}
	cfg := session.cfg
	if request.Model != "" {
		cfg.Model = request.Model
	}
	if effort, ok := request.Options["effort"]; ok {
		cfg.ReasoningEffort = effort
	}
	client := session.client
	session.mu.Unlock()
	if err := validateCodexTurn(ctx, client, request, cfg); err != nil {
		return execution.SessionTurnStarted{Delivery: delivery}, err
	}
	turn := execution.SessionTurn{
		ID: request.TurnID, ExecutionID: request.ExecutionID, State: execution.TurnActive,
		Requested: execution.TurnConfiguration{Model: request.Model, Options: cloneCodexStringMap(request.Options)}, StartedAt: p.now().UTC(),
	}
	client.correlate(request.TurnID, request.SubmissionID)
	nativeID, err := client.startTurn(ctx, codexTurnMessage(request), cfg.Model, cfg.ReasoningEffort, request.Skills)
	if err != nil {
		delivery.State = execution.DeliveryUncertain
		return execution.SessionTurnStarted{Turn: turn, Delivery: delivery}, err
	}
	turn.NativeID = nativeID
	turn.Applied = execution.TurnConfiguration{Model: cfg.Model, Options: codexMapIfSet("effort", cfg.ReasoningEffort)}
	session.mu.Lock()
	session.cfg, session.current = cfg, &turn
	session.mu.Unlock()
	delivery.State, delivery.NativeDeliveryID = execution.DeliveryConfirmed, nativeID
	go p.finishCodexNativeTurn(session, client)
	return execution.SessionTurnStarted{Turn: turn, Delivery: delivery}, nil
}

func (p *Provider) finishCodexNativeTurn(session *nativeSession, client *appServer) {
	<-client.TurnDone()
	status, turnError := client.turnOutcome()
	declaredCompletion := client.declaredTurnEnd()
	now := p.now().UTC()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.client != client || session.current == nil {
		return
	}
	switch {
	case !declaredCompletion:
		session.current.State = execution.TurnFailed
		turnError = "the Codex app-server connection ended before turn/completed"
	case status == "completed" || status == "":
		session.current.State = execution.TurnCompleted
	case status == "interrupted" || status == "cancelled" || status == "canceled":
		session.current.State = execution.TurnInterrupted
	default:
		session.current.State = execution.TurnFailed
		if turnError == "" {
			turnError = "the Codex turn ended with status " + status
		}
	}
	session.current.Error = turnError
	session.current.CompletedAt = &now
	session.approvals = make(map[string]execution.PendingApproval)
	session.inputs = make(map[string]execution.PendingInput)
	session.requests = make(map[string]rpcMessage)
}

func (p *Provider) StreamSessionEvents(ctx context.Context, request execution.SessionRequest, afterID int64, sink func(execution.RunEvent) error) error {
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return fmt.Errorf("codex session %q is not connected", request.Session.Native.ID)
	}
	return session.stream(ctx, afterID, sink)
}

func (p *Provider) SteerTurn(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.codexNativeCommand(request, func(client *appServer) error { return client.Send(ctx, request.Message) })
}

func (p *Provider) InterruptTurn(ctx context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	return p.codexNativeCommand(request, func(client *appServer) error { return client.Interrupt(ctx) })
}

func (p *Provider) RespondSessionApproval(_ context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	if request.OptionID != localrun.ApprovalAllow && request.OptionID != localrun.ApprovalDeny {
		return execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}, fmt.Errorf("codex approval option %q is not supported", request.OptionID)
	}
	return p.respondCodexServerRequest(request, approvalResult(request.OptionID))
}

func (p *Provider) RespondSessionInput(_ context.Context, request execution.SessionCommandRequest) (execution.SessionDelivery, error) {
	if len(request.Payload) == 0 {
		return execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}, fmt.Errorf("codex input response is empty")
	}
	return p.respondCodexServerRequest(request, request.Payload)
}

func (p *Provider) respondCodexServerRequest(request execution.SessionCommandRequest, result any) (execution.SessionDelivery, error) {
	delivery := execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return delivery, fmt.Errorf("codex session %q is not connected", request.Session.Native.ID)
	}
	session.mu.Lock()
	nativeRequest, ok := session.requests[request.InteractionID]
	client, current := session.client, session.current
	session.mu.Unlock()
	if !ok || client == nil || current == nil || current.ID != request.TurnID || current.State != execution.TurnActive {
		return delivery, fmt.Errorf("codex interaction %q is not pending on turn %q", request.InteractionID, request.TurnID)
	}
	payload, err := json.Marshal(map[string]any{"id": json.RawMessage(nativeRequest.ID), "result": result})
	if err != nil {
		return delivery, err
	}
	if err := client.process.Send(payload); err != nil {
		delivery.State = execution.DeliveryUncertain
		return delivery, err
	}
	session.resolveRequest(request.InteractionID)
	delivery.State, delivery.NativeDeliveryID = execution.DeliveryConfirmed, request.InteractionID
	return delivery, nil
}

func approvalResult(option string) map[string]any {
	decision := option
	switch option {
	case localrun.ApprovalAllow:
		decision = "accept"
	case localrun.ApprovalDeny:
		decision = "decline"
	}
	return map[string]any{"decision": decision}
}

func (p *Provider) codexNativeCommand(request execution.SessionCommandRequest, command func(*appServer) error) (execution.SessionDelivery, error) {
	delivery := execution.SessionDelivery{SubmissionID: request.SubmissionID, TurnID: request.TurnID, State: execution.DeliveryUnsent}
	session := p.nativeSession(request.Session.Native.ID)
	if session == nil {
		return delivery, fmt.Errorf("codex session %q is not connected", request.Session.Native.ID)
	}
	session.mu.Lock()
	client, current := session.client, session.current
	session.mu.Unlock()
	if client == nil || current == nil || current.ID != request.TurnID || current.State != execution.TurnActive {
		return delivery, fmt.Errorf("codex turn %q is not active", request.TurnID)
	}
	if err := command(client); err != nil {
		delivery.State = execution.DeliveryUncertain
		return delivery, err
	}
	delivery.State = execution.DeliveryConfirmed
	return delivery, nil
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
		session.current.State, session.current.Error, session.current.CompletedAt = execution.TurnFailed, "the Codex runtime was released before the turn completed", &now
	}
	session.process, session.client, session.connected = nil, nil, false
	session.mu.Unlock()
	if process != nil {
		_, _, _ = p.shutdown(process)
	}
	return nil
}

func (p *Provider) startNativeClient(ctx context.Context, cfg settings, dir string) (localrun.Process, *appServer, error) {
	process, err := p.starter.Start(context.Background(), dir, cfg.Command, buildArgs())
	if err != nil {
		return nil, nil, fmt.Errorf("starting codex native session: %w", err)
	}
	client := newAppServer(process, localrun.NewSession("codex-native", p.now))
	go client.consume()
	if err := client.initialize(ctx); err != nil {
		_, _, _ = p.shutdown(process)
		return nil, nil, err
	}
	return process, client, nil
}

func (a *appServer) discoverModels(ctx context.Context) ([]execution.ModelOption, error) {
	result, err := a.call(ctx, "model/list", map[string]any{"includeHidden": false})
	if err != nil {
		return nil, fmt.Errorf("discovering Codex models: %w", err)
	}
	var response struct {
		Data []struct {
			ID            string `json:"id"`
			Model         string `json:"model"`
			DisplayName   string `json:"displayName"`
			DefaultEffort string `json:"defaultReasoningEffort"`
			Default       bool   `json:"isDefault"`
			Efforts       []struct {
				Value       string `json:"reasoningEffort"`
				Description string `json:"description"`
			} `json:"supportedReasoningEfforts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("decoding Codex models: %w", err)
	}
	out := make([]execution.ModelOption, 0, len(response.Data))
	for _, item := range response.Data {
		id := item.Model
		if id == "" {
			id = item.ID
		}
		choices := make([]execution.ModelOptionChoice, 0, len(item.Efforts))
		for _, effort := range item.Efforts {
			choices = append(choices, execution.ModelOptionChoice{Value: effort.Value, Label: effort.Value, Default: effort.Value == item.DefaultEffort})
		}
		model := execution.ModelOption{ID: id, Label: item.DisplayName, Default: item.Default}
		if model.Label == "" {
			model.Label = id
		}
		if len(choices) > 0 {
			model.Options = []execution.ModelOptionField{{Name: "effort", Label: "Reasoning effort", Choices: choices}}
		}
		out = append(out, model)
	}
	return out, nil
}

func (a *appServer) discoverSkills(ctx context.Context, dir string) ([]execution.SessionSkill, error) {
	result, err := a.call(ctx, "skills/list", map[string]any{"cwds": []string{dir}, "forceReload": true})
	if err != nil {
		return nil, fmt.Errorf("discovering Codex skills: %w", err)
	}
	var response struct {
		Data []struct {
			Skills []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Path        string `json:"path"`
				Enabled     bool   `json:"enabled"`
				Scope       string `json:"scope"`
			} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("decoding Codex skills: %w", err)
	}
	out := []execution.SessionSkill{}
	for _, group := range response.Data {
		for _, skill := range group.Skills {
			if skill.Enabled {
				namespace := ""
				if separator := strings.IndexByte(skill.Name, ':'); separator > 0 {
					namespace = skill.Name[:separator]
				}
				out = append(out, execution.SessionSkill{Name: skill.Name, Description: skill.Description, Path: skill.Path, Namespace: namespace, Origin: skill.Scope, Invocation: "$" + skill.Name})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func validateCodexTurn(ctx context.Context, client *appServer, request execution.StartTurnRequest, cfg settings) error {
	if _, err := parseModel(cfg.Model); err != nil {
		return err
	}
	if _, err := parseReasoningEffort(optionalCodexString(cfg.ReasoningEffort)); err != nil {
		return err
	}
	available, err := client.discoverSkills(ctx, request.Session.Environment.WorkingDir)
	if err != nil {
		return err
	}
	known := make(map[string]string, len(available))
	for _, skill := range available {
		known[skill.Name] = skill.Path
	}
	for _, skill := range request.Skills {
		if path, ok := known[skill.Name]; !ok || path != skill.Path {
			return fmt.Errorf("codex skill %q is not available in this workspace", skill.Name)
		}
	}
	return nil
}

func newCodexNativeSession(metadata execution.SessionMetadata, cfg settings) *nativeSession {
	return &nativeSession{metadata: metadata, cfg: cfg, connected: true, subscribers: make(map[int]chan execution.RunEvent), approvals: make(map[string]execution.PendingApproval), inputs: make(map[string]execution.PendingInput), requests: make(map[string]rpcMessage)}
}

func (s *nativeSession) bindClient() {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	client.appendEvent = s.append
	client.serverRequest = s.handleRequest
	client.notification = s.handleNotification
}

func (s *nativeSession) handleRequest(request rpcMessage) {
	id := strings.Trim(string(request.ID), `"`)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[id] = request
	switch request.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		var params struct {
			Reason  string          `json:"reason"`
			Command json.RawMessage `json:"command"`
		}
		_ = json.Unmarshal(request.Params, &params)
		title := params.Reason
		if title == "" {
			title = "Codex richiede un'approvazione"
		}
		s.approvals[id] = execution.PendingApproval{ID: id, ToolName: request.Method, Title: title, Args: request.Params, Options: localrun.ApprovalOptions(), CreatedAt: now}
	case "item/tool/requestUserInput":
		s.inputs[id] = execution.PendingInput{ID: id, Title: "Codex richiede input", Payload: request.Params, CreatedAt: now}
	default:
		delete(s.requests, id)
		payload, _ := json.Marshal(map[string]any{"id": json.RawMessage(request.ID), "error": map[string]any{"code": -32601, "message": "unsupported Codex server request"}})
		_ = s.client.process.Send(payload)
	}
}

func (s *nativeSession) handleNotification(method string, params json.RawMessage) {
	if method == "skills/changed" {
		s.append(execution.RunEvent{Kind: "skill_catalog_changed", Text: "Il catalogo skill del runtime è cambiato"})
		return
	}
	if method != "serverRequest/resolved" {
		return
	}
	var resolved struct {
		RequestID any `json:"requestId"`
	}
	if json.Unmarshal(params, &resolved) == nil {
		s.resolveRequest(fmt.Sprint(resolved.RequestID))
	}
}

func (s *nativeSession) resolveRequest(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.requests, id)
	delete(s.approvals, id)
	delete(s.inputs, id)
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
	approvals := make([]execution.PendingApproval, 0, len(s.approvals))
	for _, approval := range s.approvals {
		approvals = append(approvals, approval)
	}
	inputs := make([]execution.PendingInput, 0, len(s.inputs))
	for _, input := range s.inputs {
		inputs = append(inputs, input)
	}
	sort.Slice(approvals, func(i, j int) bool { return approvals[i].CreatedAt.Before(approvals[j].CreatedAt) })
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].CreatedAt.Before(inputs[j].CreatedAt) })
	if len(approvals) > 0 {
		work = execution.SessionWaitingApproval
	} else if len(inputs) > 0 {
		work = execution.SessionWaitingInput
	}
	var turn *execution.SessionTurn
	if s.current != nil {
		copied := *s.current
		turn = &copied
	}
	return execution.SessionSnapshot{Session: s.metadata, Archive: execution.ArchiveOpen, Connection: connection, Recovery: execution.RecoveryResumable, Work: work, CurrentTurn: turn, PendingInputs: inputs, PendingApprovals: approvals}
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
	for _, subscriber := range s.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	s.mu.Unlock()
	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
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
	live := make(chan execution.RunEvent, 4096)
	s.subscribers[id] = live
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.subscribers, id); s.mu.Unlock() }()
	for _, event := range backlog {
		if event.ID > afterID {
			if err := sink(event); err != nil {
				return err
			}
			afterID = event.ID
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
		return "", fmt.Errorf("opening codex session working directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("opening codex session working directory: %s is not a directory", dir)
	}
	return dir, nil
}

func cloneCodexProviderConfig(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
func cloneCodexStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
func codexMapIfSet(key, value string) map[string]string {
	if value == "" {
		return nil
	}
	return map[string]string{key: value}
}
func optionalCodexString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func codexTurnMessage(request execution.StartTurnRequest) string {
	if len(request.Skills) == 0 {
		return request.Message
	}
	names := make([]string, 0, len(request.Skills))
	for _, skill := range request.Skills {
		names = append(names, "$"+skill.Name)
	}
	return strings.Join(names, " ") + "\n\n" + request.Message
}
