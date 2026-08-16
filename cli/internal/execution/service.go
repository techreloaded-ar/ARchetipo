package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func NewService(registry *Registry, store Store, newID IDGenerator, now Clock) (*Service, error) {
	if registry == nil || store == nil || newID == nil || now == nil {
		return nil, fmt.Errorf("execution service dependencies are required")
	}
	return &Service{registry: registry, store: store, newID: newID, now: now}, nil
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

func (s *Service) run(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any, requestID string, resolveID func() (string, error)) (Execution, error) {
	if strings.TrimSpace(spec.Code) == "" {
		return Execution{}, fmt.Errorf("spec code is required")
	}
	capability, err := RequiredCapability(action)
	if err != nil {
		return Execution{}, err
	}
	provider, err := s.registry.Resolve(providerID)
	if err != nil {
		return Execution{}, err
	}
	capabilities, err := provider.Capabilities(ctx)
	if err != nil {
		return Execution{}, &CapabilityError{ProviderID: providerID, Capability: capability, Err: err}
	}
	if !Supports(capabilities, capability) {
		return Execution{}, &CapabilityError{ProviderID: providerID, Capability: capability}
	}
	validatedConfig := CloneConfig(providerConfig)
	if err := provider.ValidateConfig(ctx, CloneConfig(validatedConfig)); err != nil {
		return Execution{}, err
	}
	id, err := resolveID()
	if err != nil {
		return Execution{}, fmt.Errorf("generate execution id: %w", err)
	}
	if !validID(id) {
		return Execution{}, fmt.Errorf("generated invalid execution id %q", id)
	}
	created := s.now().UTC()
	execution := Execution{ID: id, SpecCode: spec.Code, Action: action, Capability: capability, ProviderID: providerID, RequestID: requestID, SpecStatusBefore: spec.Status, Status: StatusRunning, CreatedAt: created}
	if err := s.store.Create(ctx, execution); err != nil {
		return Execution{}, err
	}
	request := Request{ExecutionID: id, SpecCode: spec.Code, Action: action, Capability: capability, ProviderConfig: CloneConfig(validatedConfig)}
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
	if err := s.store.Update(context.WithoutCancel(ctx), execution); err != nil {
		return Execution{}, err
	}
	return execution, nil
}
