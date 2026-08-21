package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

type IDGenerator func() (string, error)
type Clock func() time.Time

type Service struct {
	registry *Registry
	store    Store
	newID    IDGenerator
	now      Clock
	// workingRoot is the project root of the workspace this service serves.
	// The service is already per-workspace (it is built with that workspace's
	// store), so the root belongs to exactly the same object. It is required:
	// a service that does not know where it executes is the defect this field
	// exists to make unrepresentable.
	workingRoot string
}

type CapabilityError struct {
	ProviderID string
	Capability Capability
	Err        error
}

func (e *CapabilityError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("provider %q capability discovery for %s failed: %v", e.ProviderID, e.Capability, e.Err)
	}
	return fmt.Sprintf("provider %q does not support required capability %s", e.ProviderID, e.Capability)
}
func (e *CapabilityError) Unwrap() error { return e.Err }

func NewService(registry *Registry, store Store, newID IDGenerator, now Clock, workingRoot string) (*Service, error) {
	if registry == nil || store == nil || newID == nil || now == nil {
		return nil, fmt.Errorf("execution service dependencies are required")
	}
	if strings.TrimSpace(workingRoot) == "" {
		return nil, fmt.Errorf("execution service working root is required")
	}
	// Abs already cleans the result, so the root a run is stamped with is
	// absolute and normalised whatever shape the caller wrote it in.
	root, err := filepath.Abs(strings.TrimSpace(workingRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve execution working root: %w", err)
	}
	return &Service{registry: registry, store: store, newID: newID, now: now, workingRoot: root}, nil
}

// Continuation dispatches an already persisted execution to its provider and
// closes the record with the outcome. It is returned by Start and takes its own
// context, because the caller that dispatches is not necessarily the one that
// created the record: an HTTP handler answers and dies, while the dispatch has
// to outlive the response.
type Continuation func(context.Context) (Execution, error)

// Confirmation is the caller's last word on a dispatch that declared success.
// It runs inside the continuation, after the provider has answered and before
// the record is closed, and may rewrite outcome into a failure — which is then
// the only terminal state that ever reaches the store.
//
// It exists for the asynchronous caller. A record dispatched on a goroutine is
// readable while it works, so a success closed first and demoted afterwards is
// a success a client can read in between; verifying inside the single terminal
// write leaves no such window. VerifyActionEffect is the implementation both
// callers share.
//
// It returns nothing on purpose. The continuation is about to write whatever
// outcome holds and has nobody to report to, so mutating outcome is the only
// channel there is — an error return would promise one that cannot be honoured,
// and a verdict expressed that way would be written out as a success.
type Confirmation func(ctx context.Context, outcome *Execution)

// Start performs every phase up to persisting the RUNNING record and returns
// that record together with the continuation that dispatches it.
//
// When Start returns an error no record was created, so there is nothing to
// close. When it returns successfully the record already exists on the store as
// RUNNING, and the caller MUST invoke the continuation exactly once: skipping it
// leaves the execution RUNNING for ever, because nothing else will ever close
// it, while invoking it twice dispatches to the provider twice and closes the
// same record twice.
//
// confirm may be nil, which means the outcome is taken at face value. When it
// is not, it runs before the terminal write, so the record is never briefly
// something the caller is about to disown.
//
// Start deliberately starts no goroutine. Whether the dispatch runs inline (the
// CLI) or on a background context (the viewer) is the caller's choice, not this
// package's.
func (s *Service) Start(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any, confirm Confirmation, opts ...StartOption) (Execution, Continuation, error) {
	return s.start(ctx, spec, action, providerID, providerConfig, "", s.newID, confirm, opts...)
}

// StartOption decorates the record a start is about to create, before it
// reaches the store. It is a variadic tail on purpose: every existing caller
// passes none and keeps compiling untouched.
type StartOption func(*Execution)

// WithModelChoice records on the execution the model and options the run uses.
// The Options map is copied in depth, so the caller and the stored record never
// share it: a caller that keeps mutating the map it passed cannot rewrite the
// history of a run that already started.
func WithModelChoice(choice ModelChoice) StartOption {
	stored := ModelChoice{Model: choice.Model, Source: choice.Source}
	if len(choice.Options) > 0 {
		stored.Options = make(map[string]string, len(choice.Options))
		for name, value := range choice.Options {
			stored.Options[name] = value
		}
	}
	return func(execution *Execution) {
		copied := stored
		if len(stored.Options) > 0 {
			copied.Options = make(map[string]string, len(stored.Options))
			for name, value := range stored.Options {
				copied.Options[name] = value
			}
		}
		execution.ModelChoice = &copied
	}
}

// StartWorkspace is Start for an action whose object is the workspace itself
// rather than a spec. It deliberately does not duplicate the pipeline: the
// capability check, the config validation, the RUNNING record and the
// continuation are exactly the ones spec-scoped executions go through, and the
// only difference is the zero spec, which lands on the record as an empty
// spec_code. The same contract on the continuation applies — it MUST be invoked
// exactly once.
func (s *Service) StartWorkspace(ctx context.Context, action ActionID, providerID string, providerConfig map[string]any, confirm Confirmation, opts ...StartOption) (Execution, Continuation, error) {
	return s.start(ctx, domain.Spec{}, action, providerID, providerConfig, "", s.newID, confirm, opts...)
}

// Run dispatches the action through the provider with a freshly generated
// execution id. Every invocation creates a new record.
func (s *Service) Run(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any) (Execution, error) {
	return s.run(ctx, spec, action, providerID, providerConfig, "", s.newID)
}

// RunIdempotent keys the execution on requestID: the id is derived
// deterministically from spec code, action, provider and request key, so
// repeating the same request returns the record already created instead of
// dispatching a second time. Reuse is unconditional — a failed record is
// returned as it is, so retrying after a failure means using a new request key.
// The bool reports whether the returned record was reused.
func (s *Service) RunIdempotent(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any, requestID string) (Execution, bool, error) {
	if strings.TrimSpace(requestID) == "" {
		return Execution{}, false, fmt.Errorf("request id is required")
	}
	id := DeriveID(spec.Code, action, providerID, requestID)
	existing, err := s.store.Get(ctx, id)
	if err == nil {
		return existing, true, nil
	}
	var storeErr *StoreError
	if !errors.As(err, &storeErr) || storeErr.Kind != StoreNotFound {
		return Execution{}, false, err
	}
	outcome, runErr := s.run(ctx, spec, action, providerID, providerConfig, requestID, func() (string, error) { return id, nil })
	if runErr != nil {
		// A concurrent request won the create race: the record it wrote is the
		// one this request would have produced, so reading it back is reuse.
		if errors.As(runErr, &storeErr) && storeErr.Kind == StoreAlreadyExist {
			if reused, getErr := s.store.Get(ctx, id); getErr == nil {
				return reused, true, nil
			}
		}
		return outcome, false, runErr
	}
	return outcome, false, nil
}

// run is Start immediately followed by its continuation: the synchronous shape
// the CLI needs, expressed over the same single code path so the two callers
// can never drift. It passes no Confirmation because the synchronous caller
// holds the closed record and confirms it itself, with nobody able to read it
// in between.
func (s *Service) run(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any, requestID string, resolveID func() (string, error)) (Execution, error) {
	_, continuation, err := s.start(ctx, spec, action, providerID, providerConfig, requestID, resolveID, nil)
	if err != nil {
		return Execution{}, err
	}
	return continuation(ctx)
}

// validateActionObject keeps the two rules about the object of an action side
// by side: a spec-scoped action needs a spec, and a workspace-scoped one must
// not carry any — a workspace action with a spec on it is a caller mistake, not
// a more permissive execution.
func validateActionObject(action ActionID, specCode string) error {
	scope, err := ActionScope(action)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(specCode)
	switch scope {
	case ScopeSpec:
		if code == "" {
			return fmt.Errorf("spec code is required")
		}
	case ScopeWorkspace:
		if code != "" {
			return fmt.Errorf("action %q is workspace-scoped and takes no spec code", action)
		}
	}
	return nil
}

// Preflight applies the acceptance rule for a dispatch — the action has a
// required capability, the provider exists, declares that capability and
// accepts the configuration — and writes nothing at all.
//
// It is exactly the phase start() runs before it creates any record, exposed as
// a method because a caller that holds the connector needs to refuse an
// incompatible provider *before* producing any effect of its own: an action
// that moves the spec before dispatching must not move it for a provider that
// was never going to be able to run. Exposing the phase instead of copying it
// keeps that rule in one place, so the viewer, the CLI and start() can never
// disagree about which dispatch is acceptable.
func (s *Service) Preflight(ctx context.Context, action ActionID, providerID string, providerConfig map[string]any) error {
	_, _, err := s.preflight(ctx, action, providerID, providerConfig)
	return err
}

// preflight is Preflight with its intermediate results kept, so start() can
// reuse the resolved provider and the required capability instead of resolving
// them a second time.
func (s *Service) preflight(ctx context.Context, action ActionID, providerID string, providerConfig map[string]any) (Provider, Capability, error) {
	capability, err := RequiredCapability(action)
	if err != nil {
		return nil, "", err
	}
	provider, err := s.registry.Resolve(providerID)
	if err != nil {
		return nil, "", err
	}
	capabilities, err := provider.Capabilities(ctx)
	if err != nil {
		return nil, "", &CapabilityError{ProviderID: providerID, Capability: capability, Err: err}
	}
	if !Supports(capabilities, capability) {
		return nil, "", &CapabilityError{ProviderID: providerID, Capability: capability}
	}
	if err := provider.ValidateConfig(ctx, CloneConfig(providerConfig)); err != nil {
		return nil, "", err
	}
	return provider, capability, nil
}

func (s *Service) start(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any, requestID string, resolveID func() (string, error), confirm Confirmation, opts ...StartOption) (Execution, Continuation, error) {
	if err := validateActionObject(action, spec.Code); err != nil {
		return Execution{}, nil, err
	}
	provider, capability, err := s.preflight(ctx, action, providerID, providerConfig)
	if err != nil {
		return Execution{}, nil, err
	}
	validatedConfig := CloneConfig(providerConfig)
	id, err := resolveID()
	if err != nil {
		return Execution{}, nil, fmt.Errorf("generate execution id: %w", err)
	}
	if !validID(id) {
		return Execution{}, nil, fmt.Errorf("generated invalid execution id %q", id)
	}
	created := s.now().UTC()
	started := Execution{ID: id, SpecCode: spec.Code, Action: action, Capability: capability, ProviderID: providerID, RequestID: requestID, SpecStatusBefore: spec.Status, Status: StatusRunning, CreatedAt: created, WorkingDir: s.workingRoot}
	// The options are applied *before* the create, never after: the model a run
	// uses has to be readable while that run is RUNNING, and decorating the
	// record only on the terminal write would hide it until the run is over —
	// the exact opposite of what it is for.
	for _, opt := range opts {
		if opt != nil {
			opt(&started)
		}
	}
	if err := s.store.Create(ctx, started); err != nil {
		return Execution{}, nil, err
	}
	// The request derives its root from the record, not from the service, so
	// there is a single source of truth and the value the provider receives is
	// literally the one the record says the run started with. The request is
	// built here and captured by the continuation's closure: a run already in
	// flight carries its own root and no later workspace switch can rewrite it.
	request := Request{ExecutionID: id, SpecCode: spec.Code, Action: action, Capability: capability, WorkingDir: started.WorkingDir, ProviderConfig: CloneConfig(validatedConfig)}
	continuation := func(ctx context.Context) (Execution, error) {
		execution := started
		result, dispatchErr := provider.Execute(ctx, request)
		if dispatchErr == nil {
			dispatchErr = ctx.Err()
		}
		completed := s.now().UTC()
		execution.CompletedAt = &completed
		if dispatchErr != nil {
			execution.Status = StatusFailed
			execution.Error = &ExecutionError{Code: "PROVIDER_ERROR", Message: dispatchErr.Error()}
			// A failure that happened after the remote work existed keeps that
			// identifier in a structured field: the record is the only trace left of
			// work that may still be running on the other side.
			var remoteErr *RemoteError
			if errors.As(dispatchErr, &remoteErr) && strings.TrimSpace(remoteErr.ExternalID) != "" {
				execution.Error.ExternalID = strings.TrimSpace(remoteErr.ExternalID)
			}
		} else {
			if len(result.Payload) == 0 || !json.Valid(result.Payload) {
				execution.Status = StatusFailed
				execution.Error = &ExecutionError{Code: "INVALID_PROVIDER_RESULT", Message: fmt.Sprintf("provider %q returned invalid JSON payload", providerID)}
			} else {
				execution.Status = StatusSucceeded
				execution.Result = &result
			}
		}
		// The caller's verdict is applied before the record is closed, never after,
		// so the store only ever receives the terminal state the caller stands
		// behind. WithoutCancel for the same reason as the write below: a shutdown
		// racing the verdict must not turn a real success into an unverified one.
		if confirm != nil {
			confirm(context.WithoutCancel(ctx), &execution)
		}
		// WithoutCancel is what makes the record survive a cancelled dispatch: an
		// interrupted run closes as FAILED with the reason instead of staying
		// RUNNING for ever with nobody left to close it.
		if err := s.store.Update(context.WithoutCancel(ctx), execution); err != nil {
			return Execution{}, err
		}
		return execution, nil
	}
	return started, continuation, nil
}
