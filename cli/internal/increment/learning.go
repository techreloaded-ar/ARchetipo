package increment

import (
	"context"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Learn records product evidence independently of software completion. A useful
// investigation can conclude without a code diff or a pull request.
func (s *Service) Learn(ctx context.Context, id, observation, decision string, conclude bool) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		observation, decision = strings.TrimSpace(observation), strings.TrimSpace(decision)
		if len(r.Learning) > 0 {
			last := r.Learning[len(r.Learning)-1]
			if last.Observation == observation && last.Decision == decision && (!conclude || r.Status == Concluded) {
				return r, nil
			}
		}
		if r.DevelopmentMode != Continuous || (conclude && frozen(r)) {
			return r, iox.NewConflict("work cannot record this decision", "use an open product work", nil)
		}
		if observation == "" || (conclude && decision == "") {
			return r, iox.NewInvalidInput("observation and conclusion decision are required", "describe what was observed and why the work concludes", nil)
		}
		r.Learning = append(r.Learning, Learning{Observation: observation, Decision: decision, CreatedAt: time.Now().UTC()})
		if conclude {
			r.Conclusion = decision
			r.Status = Concluded
			r.ReadySHA = ""
			r.ReadyScope = ""
			r.Result = nil
		}
		return r, s.save(r)
	})
}
