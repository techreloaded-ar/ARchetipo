package execution

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Provider interface {
	ID() string
	Capabilities(context.Context) ([]Capability, error)
	ValidateConfig(context.Context, map[string]any) error
	Execute(context.Context, Request) (Result, error)
}

type ConfigurationError struct {
	Field  string
	Reason string
	Err    error
}

func (e *ConfigurationError) Error() string {
	field := strings.TrimSpace(e.Field)
	if field == "" {
		field = "config"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" && e.Err != nil {
		reason = e.Err.Error()
	}
	if reason == "" {
		reason = "is invalid"
	}
	return fmt.Sprintf("provider configuration field %q %s", field, reason)
}

func (e *ConfigurationError) Unwrap() error { return e.Err }

func CloneConfig(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	return cloneStringMap(config)
}

func cloneStringMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneConfigValue(value)
	}
	return out
}

func cloneConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = cloneConfigValue(typed[i])
		}
		return out
	default:
		return typed
	}
}

type RegistryError struct {
	ProviderID string
	Reason     string
}

func (e *RegistryError) Error() string {
	return fmt.Sprintf("execution provider %q %s", e.ProviderID, e.Reason)
}

type Registry struct{ providers map[string]Provider }

func NewRegistry() *Registry { return &Registry{providers: make(map[string]Provider)} }

func (r *Registry) Register(provider Provider) error {
	if r == nil || r.providers == nil {
		return &RegistryError{Reason: "registry is not initialized"}
	}
	if provider == nil || strings.TrimSpace(provider.ID()) == "" {
		return &RegistryError{Reason: "has an empty id"}
	}
	id := strings.TrimSpace(provider.ID())
	if _, exists := r.providers[id]; exists {
		return &RegistryError{ProviderID: id, Reason: "is already registered"}
	}
	r.providers[id] = provider
	return nil
}

func (r *Registry) Resolve(id string) (Provider, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &RegistryError{Reason: "id is empty"}
	}
	provider, ok := r.providers[id]
	if !ok {
		return nil, &RegistryError{ProviderID: id, Reason: "is not registered"}
	}
	return provider, nil
}

func NormalizeCapabilities(capabilities []Capability) []Capability {
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability != "" {
			seen[capability] = struct{}{}
		}
	}
	out := make([]Capability, 0, len(seen))
	for capability := range seen {
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func Supports(capabilities []Capability, required Capability) bool {
	capabilities = NormalizeCapabilities(capabilities)
	i := sort.Search(len(capabilities), func(i int) bool { return capabilities[i] >= required })
	return i < len(capabilities) && capabilities[i] == required
}
