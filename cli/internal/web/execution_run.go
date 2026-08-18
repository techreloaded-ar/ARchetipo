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
	mu       sync.Mutex
	ctx      context.Context
	inFlight map[string]string
	wg       sync.WaitGroup
}

func newDispatchGroup() *dispatchGroup {
	return &dispatchGroup{ctx: context.Background(), inFlight: map[string]string{}}
}

// bind installs the context dispatches will run on. Until Run binds one, the
// group falls back to Background, so a server driven directly by a test (which
// never calls Run) dispatches on a live context instead of a nil one.
func (g *dispatchGroup) bind(ctx context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ctx = ctx
}

// reserve claims the spec for a new execution. It returns false together with
// the id of the execution already holding it — an empty id means a dispatch
// that has been reserved but has not received its id yet.
func (g *dispatchGroup) reserve(specCode string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if id, busy := g.inFlight[specCode]; busy {
		return id, false
	}
	g.inFlight[specCode] = ""
	return "", true
}

// claim records the id of the execution that holds the reservation, so a second
// request can name it instead of describing an anonymous "something running".
func (g *dispatchGroup) claim(specCode, executionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, reserved := g.inFlight[specCode]; reserved {
		g.inFlight[specCode] = executionID
	}
}

func (g *dispatchGroup) release(specCode string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inFlight, specCode)
}

// current reports the execution this server is dispatching for the spec.
func (g *dispatchGroup) current(specCode string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id, busy := g.inFlight[specCode]
	return id, busy
}

// run executes fn on the dispatch context and registers it for the shutdown
// drain.
func (g *dispatchGroup) run(fn func(context.Context)) {
	g.mu.Lock()
	ctx := g.ctx
	g.mu.Unlock()
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		fn(ctx)
	}()
}

// wait drains the in-flight dispatches for at most timeout. It is bounded on
// purpose: a provider that ignores cancellation must delay shutdown, not
// prevent it.
func (g *dispatchGroup) wait(timeout time.Duration) {
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
	ctx := r.Context()
	spec, err := s.conn.ReadSpecDetail(ctx, code)
	if err != nil {
		writeError(w, err)
		return
	}
	tpl, err := s.resolveTemplate()
	if err != nil {
		writeError(w, err)
		return
	}
	// The three refusals below are deliberately distinct, because they are three
	// different mistakes: a name the process does not know is a malformed request
	// (400), a step the current status does not admit is a state conflict (409),
	// and a legitimate step this viewer cannot yet dispatch is a capability gap
	// (409) rather than the user's fault.
	if !admitsAction(tpl.Actions, string(action)) {
		writeError(w, iox.NewInvalidInput(
			"unsupported execution action: "+string(action),
			"actions of the "+tpl.ID+" process: "+actionNames(tpl.Actions),
			nil,
		))
		return
	}
	if !admitsAction(tpl.ActionsFor(spec.Status), string(action)) {
		writeError(w, iox.NewConflict(
			fmt.Sprintf("the %s process does not admit the %q action while %s is %s", tpl.ID, action, code, spec.Status),
			"move the spec to a status that admits it, then run the action again",
			nil,
		))
		return
	}
	if _, err := execution.RequiredCapability(action); err != nil {
		writeError(w, iox.NewConflict(
			"the "+string(action)+" action cannot be started from the viewer yet",
			"run it from a coding agent; the viewer can start: "+supportedActions(),
			err,
		))
		return
	}
	if s.service == nil {
		writeError(w, iox.NewConflict("no execution provider is registered in this viewer", "start the viewer from a build that registers execution providers", nil))
		return
	}
	// The default is read from disk, not from the config the server booted with:
	// the Execution panel can change it while the viewer runs, and starting an
	// execution on a stale selection would ignore what the user just saved.
	current, _, _, _, err := readConfigState(s.cfg.ProjectRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	selection := current.Execution.DefaultProvider
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		writeError(w, iox.NewConflict("execution.default_provider is not configured", "pick a default provider in the Execution panel of the configuration", nil))
		return
	}
	providerID := strings.TrimSpace(selection.ID)
	// A runtime that cannot be used is refused before any record exists, with
	// the same sentence the spec detail already shows next to the disabled
	// action. Letting the dispatch start would create a RUNNING record only to
	// close it a moment later with this very diagnostic, and leave the user a
	// failed execution to read instead of an action they can still press once
	// the runtime is fixed. A provider that does not report availability is
	// unaffected: CheckAvailability returns nil for it.
	if s.registry != nil {
		if provider, resolveErr := s.registry.Resolve(providerID); resolveErr == nil {
			if err := execution.CheckAvailability(ctx, provider, selection.Config); err != nil {
				writeError(w, iox.NewConflict(
					"the default execution provider "+quoted(providerID)+" is not usable: "+err.Error(),
					"make its runtime usable, or pick another provider in the Execution panel of the configuration",
					err,
				))
				return
			}
		}
	}

	if err := s.guardSingleExecution(ctx, code); err != nil {
		writeError(w, err)
		return
	}
	// The claimed effect is verified inside the terminal write, not after it. The
	// browser polls every two seconds and settles on the first terminal status it
	// reads, so a success closed now and disowned a moment later would be a
	// success the user keeps looking at next to a spec that never moved.
	// The verdict travels in outcome, which the continuation is about to write:
	// the returned error only renders the same fact for a caller that has one.
	confirm := func(confirmCtx context.Context, outcome *execution.Execution) {
		_ = execution.VerifyActionEffect(confirmCtx, s.conn, outcome.Action, code, outcome)
	}
	started, continuation, err := s.service.Start(ctx, spec, action, providerID, execution.CloneConfig(selection.Config), confirm)
	if err != nil {
		s.dispatch.release(code)
		// A rejected configuration is answered by the same renderer the Execution
		// panel already understands, so the form can point at the offending field
		// instead of showing a message about an input it cannot locate.
		var configErr *execution.ConfigurationError
		if errors.As(err, &configErr) {
			writeProviderConfigError(w, err)
			return
		}
		writeError(w, mapExecutionStartError(err, providerID))
		return
	}
	s.dispatch.claim(code, started.ID)
	writeJSON(w, http.StatusCreated, started)

	s.dispatch.run(func(dispatchCtx context.Context) {
		// Deferred in this order so they unwind in the other one: the spec stops
		// being busy first, and only then is the board of every connected client
		// refreshed. Publishing while the reservation still stands would send
		// clients to re-read a spec the server is about to declare free, and hand
		// them back an action still marked unavailable.
		defer s.broker.Publish()
		// The reservation spans the whole dispatch, verdict included, so no client
		// can start a second execution against a record this one has not closed.
		defer s.dispatch.release(code)
		// The outcome is already verified: the continuation applied the
		// confirmation before writing the terminal record, so there is nothing
		// left to reconcile here.
		_, _ = continuation(dispatchCtx)
	})
}

// guardSingleExecution enforces AC-1 on the server: one press, one execution.
// Both halves are needed and both are checked under the same reservation. The
// in-memory map catches a double click or two tabs racing on this process; the
// persisted record catches a viewer restarted while an execution was still
// open, which the map alone would have forgotten.
func (s *Server) guardSingleExecution(ctx context.Context, code string) error {
	existingID, reserved := s.dispatch.reserve(code)
	if !reserved {
		message := "an execution is already running for " + code
		if existingID != "" {
			message = "execution " + existingID + " is already running for " + code
		}
		return iox.NewConflict(message, "wait for it to finish before starting another one", nil)
	}
	records, err := s.store.ListBySpec(ctx, code)
	if err != nil {
		s.dispatch.release(code)
		return iox.NewInternal("reading the executions of "+code, err)
	}
	if len(records) > 0 && records[0].Status == execution.StatusRunning {
		s.dispatch.release(code)
		return iox.NewConflict(
			"execution "+records[0].ID+" is already running for "+code,
			"wait for it to finish, or remove its record under .archetipo/executions/ if it was left behind by an interrupted run",
			nil,
		)
	}
	return nil
}

// handleGetExecution serves the polling route. It reads the persisted record and
// nothing else: contacting the provider here would turn a two-second poll into a
// remote call, and the record already holds everything the UI shows.
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, iox.NewInvalidInput("missing execution id", "use /api/execution/<id>", nil))
		return
	}
	record, err := s.store.Get(r.Context(), id)
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
func (s *Server) latestExecution(ctx context.Context, code string) (*execution.Execution, error) {
	records, err := s.store.ListBySpec(ctx, code)
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
	runningID         string
	specHasRunning    bool
}

func (s *Server) actionAvailabilityFor(ctx context.Context, code string) actionAvailability {
	availability := actionAvailability{}
	if s.service == nil {
		availability.noRegistry = true
	}
	if id, busy := s.dispatch.current(code); busy {
		availability.specHasRunning = true
		availability.runningID = id
	}
	if !availability.specHasRunning {
		if records, err := s.store.ListBySpec(ctx, code); err == nil && len(records) > 0 && records[0].Status == execution.StatusRunning {
			availability.specHasRunning = true
			availability.runningID = records[0].ID
		}
	}
	current, _, _, _, err := readConfigState(s.cfg.ProjectRoot)
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
	if s.registry == nil {
		availability.providerErr = fmt.Errorf("no provider is registered")
		return availability
	}
	provider, err := s.registry.Resolve(availability.providerID)
	if err != nil {
		availability.providerErr = err
		return availability
	}
	capabilities, err := provider.Capabilities(ctx)
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
	if a.specHasRunning {
		if a.runningID != "" {
			return "execution " + a.runningID + " is already running for this spec"
		}
		return "an execution is already running for this spec"
	}
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
func (s *Server) decorateActions(ctx context.Context, code string, actions []template.Action) []specActionView {
	availability := s.actionAvailabilityFor(ctx, code)
	out := make([]specActionView, 0, len(actions))
	for _, action := range actions {
		reason := availability.reasonFor(action.ID)
		out = append(out, specActionView{Action: action, Runnable: reason == "", UnavailableReason: reason})
	}
	return out
}

func (s *Server) resolveTemplate() (template.Template, error) {
	tpl, err := template.Resolve(s.cfg.Template.ID)
	if err != nil {
		return template.Template{}, iox.NewInvalidInput(
			"unknown template: "+s.cfg.Template.ID,
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
	known := []execution.ActionID{execution.ActionPlan}
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
