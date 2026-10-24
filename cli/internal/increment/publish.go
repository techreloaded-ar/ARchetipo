package increment

import (
	"context"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func (s *Service) Publish(ctx context.Context, id, expectedSHA string, revision int) (Record, error) {
	return s.lockedWorkspaceRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if r.Status == Proposed || r.Status == Delivered {
			return r, nil
		}
		if r.Status != Review && r.Status != Publishing {
			return r, iox.NewConflict("increment is not ready for publication", "complete development and review its result first", nil)
		}
		if r.ReadySHA == "" || expectedSHA != r.ReadySHA || len(r.Revisions) == 0 || r.Revisions[len(r.Revisions)-1].Number != revision {
			return r, iox.NewConflict("the result changed after it was displayed", "read and approve the current result", nil)
		}
		if err = s.VerifyCandidate(ctx, r); err != nil {
			return r, err
		}
		if s.Hosting == nil {
			return r, iox.NewPrecondition("publication is not configured", "configure the Git hosting adapter", nil)
		}
		if _, err = git(ctx, r.Workdir, "remote", "get-url", "origin"); err != nil {
			return r, err
		}
		r.ApprovedSHA = r.ReadySHA
		if r.ApprovedAt == nil {
			now := time.Now().UTC()
			r.ApprovedAt = &now
		}
		r.Status = Publishing
		if err = s.save(r); err != nil {
			return r, err
		}
		// A retry looks up the same head/base before creating, including after an
		// unknown network outcome. The human approval is persisted before any external IO.
		if _, err = git(ctx, r.Workdir, "push", "origin", r.ApprovedSHA+":refs/heads/"+r.Branch); err != nil {
			return r, err
		}
		pr, err := s.Hosting.Find(ctx, r.Workdir, r.Branch, r.BaseBranch)
		if err != nil {
			return r, err
		}
		if pr == nil {
			pr, err = s.Hosting.Create(ctx, r.Workdir, r.Branch, r.BaseBranch, r.Title, Dossier(r))
			if err != nil {
				return r, err
			}
		}
		if pr.HeadSHA != r.ApprovedSHA {
			return r, iox.NewConflict("PR head differs from the approved result", "verify and approve the actual PR revision", nil)
		}
		if pr.State == "CLOSED" {
			return r, iox.NewConflict("the increment PR was closed without merging", "reopen the existing PR before retrying", nil)
		}
		r.PR = pr
		if err = s.save(r); err != nil {
			return r, err
		}
		r.Status = Proposed
		return r, s.save(r)
	})
}

func (s *Service) Refresh(ctx context.Context, id string) (Record, error) {
	return s.lockedWorkspaceRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if r.Status == Delivered {
			return r, nil
		}
		if r.PR == nil || r.Status != Proposed {
			return r, iox.NewConflict("increment has no published PR", "publish its approved result first", nil)
		}
		pr, err := s.Hosting.Read(ctx, r.Workdir, r.PR.Number)
		if err != nil {
			return r, err
		}
		r.PR = pr
		if pr.State == "MERGED" {
			r.IntegrationSHA = pr.HeadSHA
			// Integration is a developer decision. Preserve the business
			// candidate SHA without approving the developers' later commits.
			r.Status = Delivered
		}
		return r, s.save(r)
	})
}
