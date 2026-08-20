package execution

import "context"

// ModelFieldName is the configuration key a model catalog fills in. It lives
// here and nowhere else: a provider declares its catalog for its own field with
// this name, and the viewer reports the same name to the browser, so provider
// and viewer cannot drift apart. A caller that renders the form never writes
// the literal name — it compares the one the view carries.
const ModelFieldName = "model"

// ModelOptionChoice is one admissible value of a model option.
//
// The set is closed: an option declares every value it accepts, so a caller
// that renders it never has to know the provider. At most one choice of an
// option carries Default, with the same discipline as the Default marker of a
// catalog entry — it is a best-effort hint about what the runtime does when the
// option is not set, not a verified fact about the machine.
type ModelOptionChoice struct {
	// Value is what gets stored in the provider configuration, exactly as the
	// runtime accepts it.
	Value string `json:"value"`
	// Label is what a person reads when it differs from the value. When it is
	// empty the value is what gets read.
	Label string `json:"label,omitempty"`
	// Default marks the choice the runtime applies when the option is left
	// unset. At most one choice of an option carries it.
	Default bool `json:"default,omitempty"`
}

// ModelOptionField is one option a model accepts, declared by the provider
// together with the model itself.
type ModelOptionField struct {
	// Name is a key of the provider configuration, in the very same namespace
	// as command, model or timeout_seconds: a saved option is a plain top-level
	// key of that configuration, not a nested container. It therefore must not
	// collide with any other key the provider accepts.
	Name string `json:"name"`
	// Label is what a person reads next to the control.
	Label string `json:"label"`
	// Help says, in plain words, what leaving the option unset does.
	Help string `json:"help,omitempty"`
	// Choices are the admissible values, in the order they are offered.
	Choices []ModelOptionChoice `json:"choices"`
}

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
	// Options are the options this model accepts. They live in the catalog
	// entry, and not behind a second optional interface, because the entry
	// already *is* the per-model declaration: ListModels produces it in one
	// pass, while a second interface would add a second call, a second way to
	// fail, and two lists describing the same model that have to be kept in
	// sync. The accepted trade-off is that a provider whose options depend on
	// runtime state cannot express them, since the catalog is declared
	// statically — the same constraint the catalog itself already carries.
	Options []ModelOptionField `json:"options,omitempty"`
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
	return CloneModels(models), true, nil
}

// CloneModels detaches a catalog in depth. A shallow copy would leave the
// Options and Choices slices shared with the provider's own package-level
// catalog, so a caller that mutated the result would corrupt every later
// listing. It is exported because a provider declaring a static catalog has
// the very same problem to solve, and solving it twice is how the two copies
// drift apart. The returned slice is never nil.
func CloneModels(models []ModelOption) []ModelOption {
	out := make([]ModelOption, len(models))
	for i, model := range models {
		out[i] = model
		if model.Options == nil {
			continue
		}
		options := make([]ModelOptionField, len(model.Options))
		for j, option := range model.Options {
			options[j] = option
			if option.Choices == nil {
				continue
			}
			choices := make([]ModelOptionChoice, len(option.Choices))
			copy(choices, option.Choices)
			options[j].Choices = choices
		}
		out[i].Options = options
	}
	return out
}
