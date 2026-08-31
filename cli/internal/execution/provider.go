package execution

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type Provider interface {
	ID() string
	Capabilities(context.Context) ([]Capability, error)
	ValidateConfig(context.Context, map[string]any) error
	Execute(context.Context, Request) (Result, error)
}

// ConfigField describes one non-secret setting a provider accepts, so a caller
// that does not know the provider — the viewer's configuration form — can offer
// it without hard-coding that provider's keys.
//
// A ConfigField never carries a credential. Provider configuration holds only
// non-secret settings: a provider that needs a token names the environment
// variable holding it (a value like "ARCIPELAGO_TOKEN"), never the token.
type ConfigField struct {
	// Name is the configuration key, identical to the one the provider's own
	// validation accepts.
	Name string `json:"name"`
	// Label is what a person reads next to the input.
	Label string `json:"label"`
	// Type is "text" or "integer" and only says how to render the input; the
	// authority on what is acceptable remains Provider.ValidateConfig.
	Type string `json:"type"`
	// Help explains the field, including the default applied when it is left
	// empty.
	Help string `json:"help,omitempty"`
	// Placeholder is an example value.
	Placeholder string `json:"placeholder,omitempty"`
	// Required marks a field the provider cannot work without.
	Required bool `json:"required"`
}

// ConfigDescriber is implemented by providers that can describe their
// configuration fields. It is deliberately optional and separate from Provider:
// Provider is a stable contract and adding a method to it would break every
// existing implementation for the sake of one caller.
type ConfigDescriber interface {
	ConfigFields() []ConfigField
}

// DescribeConfig returns the fields a provider declares, or an empty list when
// it declares none. It never returns nil, so a caller that serializes the
// result always produces an empty list rather than a null.
func DescribeConfig(provider Provider) []ConfigField {
	describer, ok := provider.(ConfigDescriber)
	if !ok {
		return []ConfigField{}
	}
	fields := describer.ConfigFields()
	if fields == nil {
		return []ConfigField{}
	}
	return slices.Clone(fields)
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

// Registry resolves providers by id and preserves registration order, so the
// list a caller renders is stable across runs instead of following Go's map
// iteration.
type Registry struct {
	providers map[string]Provider
	order     []string
}

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
	r.order = append(r.order, id)
	return nil
}

// List returns the registered providers in registration order. The slice is
// detached from the registry, so a caller that reorders or truncates it cannot
// reach back into the registration state.
func (r *Registry) List() []Provider {
	if r == nil || r.providers == nil {
		return nil
	}
	out := make([]Provider, 0, len(r.order))
	for _, id := range r.order {
		if provider, ok := r.providers[id]; ok {
			out = append(out, provider)
		}
	}
	return out
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
