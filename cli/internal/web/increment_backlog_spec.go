package web

import (
	"context"
	"fmt"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// incrementBehindSpec finds the increment whose publication recorded code as
// its backlog spec. Removed increments are included: the retained product
// record still says where the spec came from. A record that merely names the
// code is not enough — a code is given again once its spec is removed, and the
// older increment would then appear behind a story it never published — so
// the spec is read and asked whose it is (increment.Record.OwnsBacklogSpec).
// The backlog is read only when some record names the code at all.
func (s *Server) incrementBehindSpec(ctx context.Context, ws *workspaceSession, code string) (increment.Record, bool, error) {
	if code == "" {
		return increment.Record{}, false, nil
	}
	records, err := s.incrementService(ws).History()
	if err != nil {
		return increment.Record{}, false, err
	}
	var spec *domain.Spec
	for _, record := range records {
		if record.BacklogSpecCode != code {
			continue
		}
		if spec == nil {
			detail, readErr := ws.conn.ReadSpecDetail(ctx, code)
			if readErr != nil {
				return increment.Record{}, false, readErr
			}
			spec = &detail
		}
		if record.OwnsBacklogSpec(*spec) {
			return record, true, nil
		}
	}
	return increment.Record{}, false, nil
}

// incrementAwaitingMergeBehindSpec reports the increment that still owns the
// closure of the spec: one that published it and whose PR is not merged yet.
// While it does, the classic review of the backlog path does not apply — there
// is no dossier to prepare, no verdict to give and no card to drag onto DONE.
// The merge closes the spec; after it, the spec is an ordinary closed story.
func (s *Server) incrementAwaitingMergeBehindSpec(ctx context.Context, ws *workspaceSession, code string) (increment.Record, bool, error) {
	record, found, err := s.incrementBehindSpec(ctx, ws, code)
	if err != nil || !found || record.Status == increment.Delivered {
		return increment.Record{}, false, err
	}
	return record, true, nil
}

// closedByIncrementReason is the one sentence every refused classic step says
// about a spec an increment published, so the detail, the board and a route
// called directly agree on why.
func closedByIncrementReason(code string) string {
	return fmt.Sprintf("la storia %s nasce da un incremento: si chiude da sola quando la sua PR viene integrata", code)
}

// refuseClassicReviewOfIncrementSpec answers the conflict for a classic review
// step asked of a spec whose closure belongs to an increment's merge, or nil
// when the spec is an ordinary one.
func (s *Server) refuseClassicReviewOfIncrementSpec(ctx context.Context, ws *workspaceSession, code string) error {
	_, owned, err := s.incrementAwaitingMergeBehindSpec(ctx, ws, code)
	if err != nil {
		return err
	}
	if !owned {
		return nil
	}
	return iox.NewConflict(closedByIncrementReason(code), "apri la conversazione dell’incremento dai lavori collegati della storia", nil)
}
