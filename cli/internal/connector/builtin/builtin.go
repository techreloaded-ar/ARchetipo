// Package builtin registers every connector compiled into the binary. Importing
// this package for side-effects ensures registry.New can resolve "file" and
// "github" without the cli package needing to know the concrete types.
package builtin

import (
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/github"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/inmemory"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/jira"
)

func init() {
	filefs.Register()
	inmemory.Register()
	github.Register()
	jira.Register()
}
