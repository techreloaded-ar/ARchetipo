package connector

import (
	"context"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// Optional capabilities
//
// The Connector interface is the contract every connector must satisfy. Beyond
// it, the web viewer (`archetipo view`) consumes a handful of *optional*
// capabilities that only some connectors can provide — reading the PRD,
// listing mockups, exposing the plan body, persisting board order or reviews.
//
// These are modelled as small single-purpose interfaces rather than added to
// Connector so a backend that has no natural way to provide them (e.g. a remote
// issue tracker with no local board-order file) is not forced to stub them out.
// Consumers discover a capability with a type assertion:
//
//	if r, ok := conn.(connector.PRDReader); ok {
//	    body, _ := r.ReadPRD(ctx)
//	}
//
// Defining them here — exported, in the connector package — gives connector
// authors a single discoverable list of what the viewer looks for, and lets a
// connector opt in explicitly with a compile-time assertion:
//
//	var _ connector.PRDReader = (*Connector)(nil)
//
// which turns "this connector forgot to expose the PRD" from a silent runtime
// gap into a build error.

// PRDReader exposes the raw PRD markdown so the viewer can render it alongside
// specs and plans. The PRD lives as a local file for every connector, so this
// is implementable even by remote backends.
type PRDReader interface {
	ReadPRD(ctx context.Context) (string, error)
}

// PlanBodyReader exposes the strategic plan body of a spec (the prose attached
// to the plan, separate from its tasks).
type PlanBodyReader interface {
	ReadPlanBody(ctx context.Context, specCode string) (string, error)
}

// MockupLister lists the design mockups produced by archetipo-design (HTML
// folders under paths.mockups). Mockups are local files, so this is
// connector-agnostic.
type MockupLister interface {
	ListMockups(ctx context.Context) ([]domain.MockupEntry, error)
}

// BoardOrderReader exposes the global ordering produced by drag-and-drop on the
// board. Connectors without a persisted ordering simply do not implement it and
// the viewer falls back to FetchBacklogItems order.
type BoardOrderReader interface {
	ReadBoardOrder(ctx context.Context) ([]string, error)
}

// SpecDeleter removes a spec and its local viewer artifacts. It is modelled as
// an optional capability because not every connector can safely support
// destructive deletion from the web viewer.
type SpecDeleter interface {
	DeleteSpec(ctx context.Context, code string) (domain.WriteResult, error)
}

// ReviewStore persists the inline review comments left on a spec's diff during
// the human acceptance gate. Connectors that cannot store them leave the
// capability unimplemented and the viewer's review panel is unavailable.
type ReviewStore interface {
	ReadReview(ctx context.Context, code string) (domain.Review, error)
	SaveReview(ctx context.Context, code string, r domain.Review) error
}
