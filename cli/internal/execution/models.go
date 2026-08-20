package execution

import "context"

// ModelFieldName is the configuration key a model catalog fills in. It lives
// here and nowhere else: a provider declares its catalog for its own field with
// this name, and the viewer reports the same name to the browser, so provider
// and viewer cannot drift apart. A caller that renders the form never writes
// the literal name — it compares the one the view carries.
const ModelFieldName = "model"

// ModelOption is one entry of a provider's model catalog.
type ModelOption struct {
	// ID is the identifier passed to the runtime, exactly as the runtime
	// accepts it. It is what gets stored in the provider configuration.
	ID string `json:"id"`
	// Label is what a person reads when it differs from the identifier. When
	// it is empty the identifier is what gets read.
	Label string `json:"label,omitempty"`
	// Default marks the model the provider itself uses when no model is
	// configured. At most one entry of a catalog carries it.
	Default bool `json:"default,omitempty"`
}

// ModelLister is implemented by providers that declare which model identifiers
// their ModelFieldName configuration field accepts. It is deliberately optional
// and separate from Provider, for the same reason ConfigDescriber and
// AvailabilityReporter are: Provider is a stable contract, and adding a method
// to it would force every existing implementation to answer a question only
// some providers can meaningfully ask.
//
// The returned error is the diagnostic the caller shows, so it must say in
// plain words why the catalog cannot be produced.
type ModelLister interface {
	Models(ctx context.Context, config map[string]any) ([]ModelOption, error)
}

// ListModels asks a provider for its model catalog.
//
// The second return value keeps apart the two situations a UI has to render
// differently: a provider that declares no catalog at all (declared == false —
// the plain text field of always, with no warning), and a provider whose
// catalog was declared but could not be obtained (declared == true with a
// non-nil error — the plain text field plus the reason). Collapsing them into
// "no models" would leave the reader with an empty list and no way to read it.
//
// The provider receives a copy of the configuration, so listing cannot mutate
// the caller's map. The provider's error is forwarded unchanged: wrapping it
// would bury the explicit text that is the whole point of showing a reason.
// On success the returned slice is detached and never nil.
func ListModels(ctx context.Context, provider Provider, config map[string]any) ([]ModelOption, bool, error) {
	lister, ok := provider.(ModelLister)
	if !ok {
		return nil, false, nil
	}
	models, err := lister.Models(ctx, CloneConfig(config))
	if err != nil {
		return nil, true, err
	}
	out := make([]ModelOption, len(models))
	copy(out, models)
	return out, true, nil
}
