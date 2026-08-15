package execution

import (
	"context"
	"encoding/json"
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

func (s *Service) Run(ctx context.Context, spec domain.Spec, action ActionID, providerID string, providerConfig map[string]any) (Execution, error) {
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
	id, err := s.newID()
	if err != nil {
		return Execution{}, fmt.Errorf("generate execution id: %w", err)
	}
	if !validID(id) {
		return Execution{}, fmt.Errorf("generated invalid execution id %q", id)
	}
	created := s.now().UTC()
	execution := Execution{ID: id, SpecCode: spec.Code, Action: action, Capability: capability, ProviderID: providerID, SpecStatusBefore: spec.Status, Status: StatusRunning, CreatedAt: created}
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
