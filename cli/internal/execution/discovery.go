package execution

import "context"

// WorkspaceRef is one remote destination a provider could dispatch to,
// described well enough for a person to pick it out by name.
type WorkspaceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Detail is the line shown beside the name — where the work lands, how much
	// of the fleet is awake — and is empty when there is nothing worth saying.
	Detail string `json:"detail,omitempty"`
	// Ready is false when the destination exists but work sent there could not
	// run today. It is not an error: an unusable destination is a fact worth
	// showing, and hiding it would leave a person unable to see why the one
	// they expected is missing.
	Ready          bool   `json:"ready"`
	NotReadyReason string `json:"not_ready_reason,omitempty"`
}

// WorkspaceDiscoverer is implemented by providers that dispatch to a remote
// place a person has to choose. It is optional and separate from Provider for
// the same reason ConfigDescriber and AvailabilityReporter are: Provider is a
// stable contract, and a method on it would force every local provider to
// answer a question only remote ones can be asked.
//
// The configuration it receives may be incomplete — specifically, it may not
// yet name a workspace, since this is the call that finds one. A provider that
// requires more than it is given says so through the usual ConfigurationError.
type WorkspaceDiscoverer interface {
	DiscoverWorkspaces(ctx context.Context, config map[string]any) ([]WorkspaceRef, error)
}

// DiscoverWorkspaces asks a provider where it could dispatch to.
//
// The boolean reports whether the provider answers this question at all, which
// a caller needs to tell "there are no destinations" apart from "there is no
// such thing as a destination here" — the first is a setup problem, the second
// is how every local provider works.
//
// The provider receives a copy of the configuration, so probing cannot mutate
// the caller's map.
func DiscoverWorkspaces(
	ctx context.Context,
	provider Provider,
	config map[string]any,
) ([]WorkspaceRef, bool, error) {
	discoverer, ok := provider.(WorkspaceDiscoverer)
	if !ok {
		return nil, false, nil
	}
	refs, err := discoverer.DiscoverWorkspaces(ctx, CloneConfig(config))
	return refs, true, err
}
