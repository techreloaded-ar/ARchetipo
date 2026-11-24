package web

import (
	"fmt"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/increment"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// incrementBehindSpec finds the increment whose publication recorded code as
// its backlog spec. Removed increments are included: the retained product
// record still says where the spec came from.
func (s *Server) incrementBehindSpec(ws *workspaceSession, code string) (increment.Record, bool, error) {
	if code == "" {
		return increment.Record{}, false, nil
	}
	records, err := s.incrementService(ws).History()
	if err != nil {
		return increment.Record{}, false, err
	}
	for _, record := range records {
		if record.BacklogSpecCode == code {
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
func (s *Server) incrementAwaitingMergeBehindSpec(ws *workspaceSession, code string) (increment.Record, bool, error) {
	record, found, err := s.incrementBehindSpec(ws, code)
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
func (s *Server) refuseClassicReviewOfIncrementSpec(ws *workspaceSession, code string) error {
	_, owned, err := s.incrementAwaitingMergeBehindSpec(ws, code)
	if err != nil {
		return err
	}
	if !owned {
		return nil
	}
	return iox.NewConflict(closedByIncrementReason(code), "apri la conversazione dell’incremento dai lavori collegati della storia", nil)
}
