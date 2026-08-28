package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// dispatchGroup owns the executions the server has started and not yet closed.
//
// It exists because the HTTP response and the dispatch have different
// lifetimes: the request context dies with the response, while the provider may
// poll a remote hub for an hour. The group therefore holds the context the
// dispatches run on (the server's own, cancelled at shutdown), the wait group
// shutdown drains, and the in-flight reservation per spec that makes "one
// execution per press" a server guarantee instead of a button behaviour.
type dispatchGroup struct {
	mu  sync.Mutex
	ctx context.Context
	// inFlight holds, per subject, the ids of the executions this server has
	// started and not yet closed, oldest first.
	//
	// It is a list and no longer a single id because a subject can carry more
	// than one at a time. It used to be one because a second start was refused,
	// and that refusal is gone: an action is a conversation now, so it stays
	// open for as long as the person keeps it open, and a spec locked out of
	// every other action while one of its runs waits for an answer would be a
	// lock nobody chose and no one could lift.
	inFlight map[string][]string
	wg       sync.WaitGroup
	// stopped is raised by wait, under the same mutex run takes before touching
	// the wait group. Without it a request that reached run while the drain had
	// already started would call Add concurrently with Wait — a WaitGroup misuse
	// that panics the whole process. It became reachable when stopping a session
	// stopped being something only shutdown did.
	stopped bool
}

func newDispatchGroup() *dispatchGroup {
	return &dispatchGroup{ctx: context.Background(), inFlight: map[string][]string{}}
}

// bind installs the context dispatches will run on. Until Run binds one, the
// group falls back to Background, so a server driven directly by a test (which
// never calls Run) dispatches on a live context instead of a nil one.
func (g *dispatchGroup) bind(ctx context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ctx = ctx
}

// claim records that an execution has been started for a subject and is not
// closed yet. It writes nothing about permission: whether a second one may be
// started is not a question this group answers any more.
func (g *dispatchGroup) claim(subject, executionID string) {
	if strings.TrimSpace(executionID) == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight[subject] = append(g.inFlight[subject], executionID)
}

// release drops one execution from its subject. It names the execution and not
// only the subject, because a subject can hold several: releasing by subject
// alone would let the end of one run declare every other run of the same spec
// finished.
func (g *dispatchGroup) release(subject, executionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	kept := g.inFlight[subject][:0]
	for _, id := range g.inFlight[subject] {
		if id != executionID {
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 {
		delete(g.inFlight, subject)
		return
	}
	g.inFlight[subject] = kept
}

// current names the most recent execution this server is still dispatching for
// the subject.
//
// The most recent and not the first: it is what a caller offers a way *to*, and
// with several open the one somebody just started is the one they are looking
// for. It is the same rule the store-backed answer beside it follows, where the
// newest record is the one read.
func (g *dispatchGroup) current(subject string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ids := g.inFlight[subject]
	if len(ids) == 0 {
		return "", false
	}
	return ids[len(ids)-1], true
}

// run executes fn on the dispatch context and registers it for the shutdown
// drain.
func (g *dispatchGroup) run(fn func(context.Context)) {
	g.mu.Lock()
	ctx := g.ctx
	if g.stopped {
		g.mu.Unlock()
		// The drain has already begun, so this dispatch cannot join it. It still
		// runs — on the context the group has just had cancelled, which is what
		// makes the continuation close its record as FAILED with a reason instead
		// of leaving it RUNNING for ever — but it is not registered, because
		// registering it now would be an Add racing a Wait.
		go fn(ctx)
		return
	}
	g.wg.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.wg.Done()
		fn(ctx)
	}()
}

// wait drains the in-flight dispatches for at most timeout. It is bounded on
// purpose: a provider that ignores cancellation must delay shutdown, not
// prevent it.
func (g *dispatchGroup) wait(timeout time.Duration) {
	g.mu.Lock()
	g.stopped = true
	g.mu.Unlock()
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

type runSpecActionReq struct {
	Action string `json:"action"`
	// Model and ModelOptions are the choice made for this single run. Both are
	// optional: a request that carries neither runs exactly the workspace
	// configuration, and neither of them is ever saved back to it.
	Model        string            `json:"model,omitempty"`
	ModelOptions map[string]string `json:"model_options,omitempty"`
	// ConversationID names the conversation the start was asked for in, and is
	// absent for a start pressed on the board — which belongs to no thread. It
	// changes nothing about the run: the same record, the same transition, the
	// same dispatch. What it adds is the tie, so a run asked for inside a
	// conversation is read where it was asked for.
	ConversationID string `json:"conversation_id,omitempty"`
}

// handleRunSpecAction starts one action on one spec through the workspace
// default provider and answers before the provider has finished.
//
// It answers early by design: the arcipelago provider polls a remote hub until
// it completes, so a response that waited for the outcome would hang for as
// long as the remote work takes. The record is created and persisted as RUNNING
// before the response is written, which is what lets the browser follow it, and
// what lets a reload find it.
func (s *Server) handleRunSpecAction(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	// First of all, and before anything is read or reserved: a run executes in
	// the project root of the workspace that is open, so a root that is gone is
	// a refusal that names the directory, not an obscure connector failure.
	if err := ws.requireReachable(); err != nil {
		writeError(w, err)
		return
	}
	code := strings.TrimSpace(r.PathValue("code"))
	if code == "" {
		writeError(w, iox.NewInvalidInput("missing spec code", "use /api/spec/US-XXX/execution", nil))
		return
	}
	var req runSpecActionReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	action := execution.ActionID(strings.TrimSpace(req.Action))
	if action == "" {
		writeError(w, iox.NewInvalidInput("action is required", "supported actions: "+supportedActions(), nil))
		return
	}
	started, err := s.startSpecAction(r.Context(), ws, code, action, req.Model, req.ModelOptions)
	if err != nil {
		writeStartError(w, err)
		return
	}
	// The tie is made after the start and never gates it: by here the execution
	// is persisted and running, and a conversation that ended in between is not
	// a reason to refuse the answer that announces it.
	_ = s.adoptStartedRun(r.Context(), ws, req.ConversationID, started)
	// The response is written after the dispatch has been launched, which is
	// indifferent: the dispatch is asynchronous and never touches this
	// ResponseWriter — it only runs the continuation on the server's own
	// context. What the browser needs before it can follow the run is the
	// persisted RUNNING record, and that already exists by the time startSpecAction
	// returns.
	writeJSON(w, http.StatusCreated, started)
}

// startSpecAction is the one and only place a spec-scoped execution is started.
//
// It exists as a function rather than as the body of an HTTP handler because a
// second door leads here: confirming an action proposed inside a workspace
// conversation. Both doors must produce, by construction, the same execution
// record and the same backlog transition the board produces — which can only be
// guaranteed if there is a single sequence, not two that happen to agree today.
//
// Everything an accepted start owes the system happens here: the availability
// checks in their exact order, the per-run model choice, the preflight, the
// reservation, the status transition and the dispatch. Nothing here writes an
// HTTP response: every refusal is returned as an error, and writeStartError is
// the single place that turns one into a response.
func (s *Server) startSpecAction(ctx context.Context, ws *workspaceSession, code string, action execution.ActionID, model string, modelOptions map[string]string) (*execution.Execution, error) {
	spec, err := ws.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		return nil, err
	}
	tpl, err := s.resolveTemplate(ws)
	if err != nil {
		return nil, err
	}
	// The three refusals below are deliberately distinct, because they are three
	// different mistakes: a name the process does not know is a malformed request
	// (400), a step the current status does not admit is a state conflict (409),
	// and a legitimate step this viewer cannot yet dispatch is a capability gap
	// (409) rather than the user's fault.
	if !admitsAction(tpl.Actions, string(action)) {
		return nil, iox.NewInvalidInput(
			"unsupported execution action: "+string(action),
			"actions of the "+tpl.ID+" process: "+actionNames(tpl.Actions),
			nil,
		)
	}
	if !admitsAction(tpl.ActionsFor(spec.Status), string(action)) {
		return nil, iox.NewConflict(
			fmt.Sprintf("the %s process does not admit the %q action while %s is %s", tpl.ID, action, code, spec.Status),
			"move the spec to a status that admits it, then run the action again",
			nil,
		)
	}
	if _, err := execution.RequiredCapability(action); err != nil {
		return nil, iox.NewConflict(
			"the "+string(action)+" action cannot be started from the viewer yet",
			"run it from a coding agent; the viewer can start: "+supportedActions(),
			err,
		)
	}
	if ws.service == nil {
		return nil, iox.NewConflict("no execution provider is registered in this viewer", "start the viewer from a build that registers execution providers", nil)
	}
	// The default is read from disk, not from the config the server booted with:
	// the Execution panel can change it while the viewer runs, and starting an
	// execution on a stale selection would ignore what the user just saved.
	current, _, _, _, err := readConfigState(ws.cfg.ProjectRoot)
	if err != nil {
		return nil, err
	}
	selection := current.Execution.DefaultProvider
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		return nil, iox.NewConflict("execution.default_provider is not configured", "pick a default provider in the Execution panel of the configuration", nil)
	}
	providerID := strings.TrimSpace(selection.ID)
	// A runtime that cannot be used is refused before any record exists, with
	// the same sentence the spec detail already shows next to the disabled
	// action. Letting the dispatch start would create a RUNNING record only to
	// close it a moment later with this very diagnostic, and leave the user a
	// failed execution to read instead of an action they can still press once
	// the runtime is fixed. A provider that does not report availability is
	// unaffected: CheckAvailability returns nil for it.
	var provider execution.Provider
	if s.registry != nil {
		if resolved, resolveErr := s.registry.Resolve(providerID); resolveErr == nil {
			provider = resolved
			if err := execution.CheckAvailability(ctx, provider, selection.Config); err != nil {
				return nil, iox.NewConflict(
					"the default execution provider "+quoted(providerID)+" is not usable: "+err.Error(),
					"make its runtime usable, or pick another provider in the Execution panel of the configuration",
					err,
				)
			}
		}
	}

	// The per-run choice is merged here: after the provider is known usable —
	// there is no point asking an unusable runtime for its catalog — and before
	// the Preflight, the reservation and BeginActionEffect. That position is
	// what makes a wrong override leave no trace at all: the spec is not moved,
	// no record is written, and the saved configuration is untouched, because
	// only the clone this call returns is ever passed on.
	effectiveConfig, modelChoice, err := resolveRunModelChoice(ctx, provider, selection.Config, model, modelOptions)
	if err != nil {
		return nil, wrapRunModelChoiceError(err)
	}

	// Preflight is applied here, before the reservation, before any transition
	// and before a record exists: resolving the provider, checking it declares
	// the capability the action requires and validating its configuration are
	// the whole of what an incompatible provider fails on, and failing them now
	// is what makes a refused start leave no trace at all — neither on the spec
	// status nor under .archetipo/executions/. Service.Start applies the very
	// same phase again; running it twice is cheap and keeps the rule in one
	// place instead of restating it here.
	if err := ws.service.Preflight(ctx, action, providerID, execution.CloneConfig(effectiveConfig)); err != nil {
		var configErr *execution.ConfigurationError
		if errors.As(err, &configErr) {
			return nil, err
		}
		return nil, mapExecutionStartError(err, providerID)
	}
	// An implementation carries out a plan, so a spec that has none has nothing
	// to execute. Refusing here costs nothing; refusing later would already have
	// moved the spec and burned a run.
	if action == execution.ActionImplement {
		hasPlan, err := execution.HasPersistedPlan(ctx, ws.conn, code)
		if err != nil {
			return nil, iox.NewInternal("reading the plan tasks of "+code, err)
		}
		if !hasPlan {
			return nil, iox.NewConflict(
				"the spec "+code+" has no persisted plan to implement",
				"plan it before implementing it, then run the action again",
				nil,
			)
		}
	}

	// The state change an accepted start owes the backlog is the caller's,
	// because the caller is the only one holding a connector — the execution
	// package deliberately never does. It runs after the preflight, so a
	// provider that cannot run the action never moves the spec, and before the
	// dispatch, so the spec is IN PROGRESS by the time the response is written
	// instead of depending on the agent surviving its first seconds.
	if err := execution.BeginActionEffect(ctx, ws.conn, action, spec); err != nil {
		return nil, iox.NewInternal("starting the "+string(action)+" action on "+code, err)
	}
	// The claimed effect is verified inside the terminal write, not after it. The
	// browser polls every two seconds and settles on the first terminal status it
	// reads, so a success closed now and disowned a moment later would be a
	// success the user keeps looking at next to a spec that never moved.
	// The verdict travels in outcome, which the continuation is about to write:
	// the returned error only renders the same fact for a caller that has one.
	confirm := func(confirmCtx context.Context, outcome *execution.Execution) {
		_ = execution.VerifyActionEffect(confirmCtx, ws.conn, outcome.Action, code, outcome)
	}
	startOpts := []execution.StartOption(nil)
	if modelChoice != nil {
		startOpts = append(startOpts, execution.WithModelChoice(*modelChoice))
	}
	started, continuation, err := ws.service.Start(ctx, spec, action, providerID, execution.CloneConfig(effectiveConfig), confirm, startOpts...)
	if err != nil {
		// A rejected configuration travels back typed, so the single error
		// renderer can answer it with the form the Execution panel already
		// understands, pointing at the offending field instead of showing a
		// message about an input it cannot locate.
		var configErr *execution.ConfigurationError
		if errors.As(err, &configErr) {
			return nil, err
		}
		return nil, mapExecutionStartError(err, providerID)
	}
	ws.dispatch.claim(code, started.ID)
	// The run becomes the thread it is read in, before the response is written
	// and before the dispatch has produced anything: the person who pressed must
	// find the thread already there, not a moment later.
	s.holdRunAsConversation(ctx, ws, &started, provider, effectiveConfig)

	ws.dispatch.run(func(dispatchCtx context.Context) {
		// Deferred in this order so they unwind in the other one: the spec stops
		// being busy first, and only then is the board of every connected client
		// refreshed. Publishing while the reservation still stands would send
		// clients to re-read a spec the server is about to declare free, and hand
		// them back an action still marked unavailable.
		defer s.broker.Publish()
		// The registration spans the whole dispatch, verdict included, so a
		// caller asking what is running for this spec is answered until the
		// record this dispatch is about has really been closed.
		defer ws.dispatch.release(code, started.ID)
		// The outcome is already verified: the continuation applied the
		// confirmation before writing the terminal record, so there is nothing
		// left to reconcile here.
		_, _ = continuation(dispatchCtx)
		// The thread of this run ends with it. It is sealed on a context of its
		// own for the reason every terminal write here uses one: a shutdown
		// racing the end of a run must not throw away the transcript of what the
		// agent said.
		s.sealRunConversation(context.WithoutCancel(dispatchCtx), ws, started.ID)
	})
	return &started, nil
}

// handleGetExecution serves the polling route. It reads the persisted record and
// nothing else: contacting the provider here would turn a two-second poll into a
// remote call, and the record already holds everything the UI shows.
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, iox.NewInvalidInput("missing execution id", "use /api/execution/<id>", nil))
		return
	}
	record, err := ws.store.Get(r.Context(), id)
	if err != nil {
		var storeErr *execution.StoreError
		if errors.As(err, &storeErr) {
			switch storeErr.Kind {
			case execution.StoreNotFound:
				writeError(w, iox.NewNotFound("execution not found: "+id, "pass the id of an execution this workspace started", err))
				return
			case execution.StoreInvalidID:
				writeError(w, iox.NewInvalidInput("invalid execution id", "use the id returned when the action was started", err))
				return
			}
		}
		writeError(w, iox.NewInternal("reading execution "+id, err))
		return
	}
	writeJSON(w, http.StatusOK, record)
}

// latestExecution returns the most recent execution of a spec, or nil when the
// spec has none. It is what makes a reload equivalent to the session before it:
// the client keeps no identifier, the server hands it back every time.
func (s *Server) latestExecution(ctx context.Context, ws *workspaceSession, code string) (*execution.Execution, error) {
	records, err := ws.store.ListBySpec(ctx, code)
	if err != nil {
		return nil, iox.NewInternal("reading the executions of "+code, err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	latest := records[0]
	return &latest, nil
}

// specActionView is a process action plus whether this workspace can actually
// start it now. template.Action is embedded by value so id, label, skill and
// statuses stay exactly where they were in the payload: the browser that US-028
// shipped keeps working, and the new fields are additions, not a new shape.
type specActionView struct {
	template.Action
	Runnable bool `json:"runnable"`
	// UnavailableReason is omitted when the action is runnable, so a client can
	// never render a reason next to an action that has none.
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// actionAvailability answers "can this workspace run this action on this spec?"
// once per request, so the per-action loop does not re-read the configuration
// or re-ask the provider for its capabilities.
//
// The whole computation lives on the server on purpose: the browser must not
// learn which provider exists, what a capability is, or when a spec is busy.
type actionAvailability struct {
	// providerAvailability is the half of the answer that does not depend on
	// the object of the action: which default provider this workspace has, and
	// whether it can be used at all. It is shared with the workspace-scoped
	// route so the two never describe the same configuration differently.
	providerAvailability
	runningID      string
	specHasRunning bool
	// runningAction is the action of the running execution, when it is known.
	// Since a second start is no longer refused (see dispatchGroup: an action is
	// a conversation now), a running execution only blocks *its own* action;
	// an empty value means "unknown" and keeps the old conservative refusal.
	runningAction string
	// specHasPlan says whether the spec has at least one persisted plan task.
	// Only the implement action depends on it, but it travels here rather than
	// being re-read per action because the detail has already read the plan and
	// hands the count over.
	specHasPlan bool
}

// providerAvailability answers "which provider would this workspace run with,
// and can it?" once per request, so a per-action loop does not re-read the
// configuration or re-ask the provider for its capabilities.
//
// The whole computation lives on the server on purpose: the browser must not
// learn which provider exists or what a capability is.
type providerAvailability struct {
	providerID   string
	capabilities []execution.Capability
	providerErr  error
	// unavailableReason is the default provider's own diagnostic when its
	// runtime cannot be used: a binary that is not installed, or one that is
	// not authenticated. It is empty when the runtime is usable, or when the
	// provider does not report availability at all.
	unavailableReason string
	noDefault         bool
	noRegistry        bool
	// providerConfig is the config block saved next to the default provider id.
	// It travels with the availability so a route that has just decided the
	// provider is usable can dispatch with the very configuration it probed.
	providerConfig map[string]any
}

func (s *Server) actionAvailabilityFor(ctx context.Context, ws *workspaceSession, code string, planTaskCount int) actionAvailability {
	availability := actionAvailability{specHasPlan: planTaskCount > 0}
	if id, busy := ws.dispatch.current(code); busy {
		availability.specHasRunning = true
		availability.runningID = id
		if record, err := ws.store.Get(ctx, id); err == nil {
			availability.runningAction = string(record.Action)
		}
	}
	if !availability.specHasRunning {
		if records, err := ws.store.ListBySpec(ctx, code); err == nil && len(records) > 0 && records[0].Status == execution.StatusRunning {
			availability.specHasRunning = true
			availability.runningID = records[0].ID
			availability.runningAction = string(records[0].Action)
		}
	}
	availability.providerAvailability = s.providerAvailabilityFor(ctx, ws)
	return availability
}

// providerAvailabilityFor resolves the default provider of this workspace and
// probes it. The default is read from disk, not from the config the server
// booted with: the Execution panel can change it while the viewer runs.
//
// The capabilities it records are the *declared* ones — the provider's own list
// plus the ones derived from the optional interfaces it implements — exactly
// the reading the provider panel does. The two must not diverge: a viewer that
// listed a capability in one place and denied it in another would be describing
// the same provider in two ways. The derivation is purely additive here, since
// RequiredCapability only ever maps an action onto a self-declared capability,
// so no existing refusal can change: it can only stop refusing what the
// provider actually implements.
func (s *Server) providerAvailabilityFor(ctx context.Context, ws *workspaceSession) providerAvailability {
	availability := providerAvailability{}
	if ws.service == nil {
		availability.noRegistry = true
	}
	current, _, _, _, err := readConfigState(ws.cfg.ProjectRoot)
	if err != nil {
		availability.noDefault = true
		return availability
	}
	selection := current.Execution.DefaultProvider
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		availability.noDefault = true
		return availability
	}
	availability.providerID = strings.TrimSpace(selection.ID)
	availability.providerConfig = selection.Config
	if s.registry == nil {
		availability.providerErr = fmt.Errorf("no provider is registered")
		return availability
	}
	provider, err := s.registry.Resolve(availability.providerID)
	if err != nil {
		availability.providerErr = err
		return availability
	}
	capabilities, err := execution.DeclaredCapabilities(ctx, provider)
	if err != nil {
		availability.providerErr = err
		return availability
	}
	availability.capabilities = capabilities
	// The probe runs once per request, right here, and is never cached: it
	// costs a short `--version` call, and a cached answer would keep reporting
	// a runtime as missing after the user has just installed it.
	if err := execution.CheckAvailability(ctx, provider, selection.Config); err != nil {
		availability.unavailableReason = err.Error()
	}
	return availability
}

// reasonFor returns the reason the action cannot be started, or "" when it can.
// The texts are English like the rest of the viewer, and every one of them names
// what to do about it rather than only what is missing.
func (a actionAvailability) reasonFor(actionID string) string {
	capability, err := execution.RequiredCapability(execution.ActionID(actionID))
	if err != nil {
		return "this action cannot be started from the viewer yet"
	}
	// A running execution blocks only the action it is running: since the
	// dispatchGroup decision, a second start is not refused, because an action
	// is a conversation and a run waiting on an answer must not lock the spec
	// out of every other step. When the running action is unknown, the old
	// conservative refusal stays.
	if a.specHasRunning && (a.runningAction == "" || a.runningAction == actionID) {
		if a.runningID != "" {
			return "execution " + a.runningID + " is already running for this spec"
		}
		return "an execution is already running for this spec"
	}
	if reason := a.providerAvailability.reasonFor(capability); reason != "" {
		return reason
	}
	// Last, and only for the implementation: the provider is fine, the spec is
	// free, but there is no plan to carry out. It is checked after the provider
	// for the same reason the route refuses in that order — an incompatible
	// provider is the more fundamental obstacle.
	if execution.ActionID(actionID) == execution.ActionImplement && !a.specHasPlan {
		return "this spec has no persisted plan: plan it before implementing it"
	}
	return ""
}

// reasonFor returns the reason the default provider cannot run an action that
// requires capability, or "" when it can. It is the half of the diagnosis that
// is the same for a spec action and for a workspace one, so both routes say the
// same sentence about the same configuration.
func (a providerAvailability) reasonFor(capability execution.Capability) string {
	if a.noRegistry {
		return "no execution provider is registered in this viewer"
	}
	if a.noDefault {
		return "no default execution provider is configured: pick one in the Execution panel of the configuration"
	}
	if a.providerErr != nil {
		return "the default execution provider " + quoted(a.providerID) + " is unavailable: " + a.providerErr.Error()
	}
	if a.unavailableReason != "" {
		return "the default execution provider " + quoted(a.providerID) + " is not usable: " + a.unavailableReason
	}
	if !execution.Supports(a.capabilities, capability) {
		return "the default execution provider " + quoted(a.providerID) + " does not declare the " + quoted(string(capability)) + " capability"
	}
	return ""
}

// quoted wraps an identifier in quotes inside a sentence, so a provider id or a
// capability reads as a name and not as part of the prose around it.
func quoted(value string) string { return "\"" + value + "\"" }

// decorateActions turns the process actions into the viewer's action views.
// planTaskCount is the number of plan tasks the caller has already read for the
// spec, so an action whose precondition is a persisted plan can be reported as
// unavailable without a second read of the same plan.
func (s *Server) decorateActions(ctx context.Context, ws *workspaceSession, code string, actions []template.Action, planTaskCount int) []specActionView {
	availability := s.actionAvailabilityFor(ctx, ws, code, planTaskCount)
	out := make([]specActionView, 0, len(actions))
	for _, action := range actions {
		reason := availability.reasonFor(action.ID)
		out = append(out, specActionView{Action: action, Runnable: reason == "", UnavailableReason: reason})
	}
	return out
}

func (s *Server) resolveTemplate(ws *workspaceSession) (template.Template, error) {
	tpl, err := template.Resolve(ws.cfg.Template.ID)
	if err != nil {
		return template.Template{}, iox.NewInvalidInput(
			"unknown template: "+ws.cfg.Template.ID,
			"valid: "+strings.Join(template.Builtin().IDs(), ", "),
			err,
		)
	}
	return tpl, nil
}

func admitsAction(actions []template.Action, actionID string) bool {
	for _, action := range actions {
		if action.ID == actionID {
			return true
		}
	}
	return false
}

// actionNames lists the action ids a process declares, for an error that has to
// tell the caller which names exist at all.
func actionNames(actions []template.Action) string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.ID)
	}
	return strings.Join(out, ", ")
}

// supportedActions lists the actions the viewer can dispatch, derived from the
// capability map rather than restated, so adding one there adds it here too.
func supportedActions() string {
	known := []execution.ActionID{
		execution.ActionPlan,
		execution.ActionImplement,
		execution.ActionReview,
		execution.ActionInception,
		execution.ActionBacklog,
		execution.ActionSpecDraft,
	}
	out := make([]string, 0, len(known))
	for _, action := range known {
		out = append(out, string(action))
	}
	return strings.Join(out, ", ")
}

// mapExecutionStartError renders a refused start with the same distinctions the
// CLI makes, so the same misconfiguration reads the same way in both places.
func mapExecutionStartError(err error, providerID string) error {
	var registryErr *execution.RegistryError
	if errors.As(err, &registryErr) {
		return iox.NewConflict(
			"invalid execution.default_provider.id: "+providerID,
			"pick a registered provider in the Execution panel of the configuration",
			err,
		)
	}
	var capabilityErr *execution.CapabilityError
	if errors.As(err, &capabilityErr) {
		return iox.NewConflict(
			capabilityErr.Error(),
			"pick a provider that declares "+string(capabilityErr.Capability),
			err,
		)
	}
	var actionErr *execution.ActionError
	if errors.As(err, &actionErr) {
		return iox.NewInvalidInput(actionErr.Error(), "supported actions: "+supportedActions(), err)
	}
	return iox.NewInternal("starting the execution", err)
}
