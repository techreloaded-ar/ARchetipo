package version

// Version is the CLI version, injected at build time via -ldflags.
// Default to "dev" for unreleased local builds.
var Version = "dev"

// SourceCheckout is the root of the repository checkout a development binary
// was built from, injected via -ldflags by `npm run install:dev`. It lets
// `archetipo view --isolated` build the matching isolated image from the same
// sources, since no registry publishes one for a development version. Release
// binaries leave it empty.
var SourceCheckout = ""
