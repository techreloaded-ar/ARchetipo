package execution

import "context"

// AvailabilityReporter is implemented by providers whose work depends on a
// runtime that may simply not be there: a local binary that is not installed,
// or an installed one that is not authenticated. It is deliberately optional
// and separate from Provider, for the same reason ConfigDescriber is: Provider
// is a stable contract, and adding a method to it would force every existing
// implementation to answer a question only some providers can meaningfully ask.
//
// The returned error is the diagnostic the caller shows, so it must say what is
// missing in plain words. A provider that is available returns nil.
type AvailabilityReporter interface {
	Available(ctx context.Context, config map[string]any) error
}

// CheckAvailability asks a provider whether its runtime is usable. A provider
// that does not declare the interface has nothing that can be missing, so it is
// treated as available and the call returns nil.
//
// The provider receives a copy of the configuration, so probing cannot mutate
// the caller's map. The provider's error is forwarded unchanged: wrapping it
// would bury the explicit text that is the whole point of the probe.
func CheckAvailability(ctx context.Context, provider Provider, config map[string]any) error {
	reporter, ok := provider.(AvailabilityReporter)
	if !ok {
		return nil
	}
	return reporter.Available(ctx, CloneConfig(config))
}
