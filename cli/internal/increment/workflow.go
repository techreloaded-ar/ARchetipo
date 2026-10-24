package increment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/gitwt"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/verification"
)

func hash(value any) string {
	raw, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func frozen(r Record) bool {
	return r.Status == Publishing || r.Status == Proposed || r.Status == Delivered || r.Status == Concluded
}

// candidateScope binds the tested commit to the exact specification and linked
// business requests displayed for approval. Backlog statuses are independent.
func candidateScope(r Record) string {
	return hash(struct {
		Specification Proposal
		Stories       []StoryLink
	}{r.Revisions[len(r.Revisions)-1].Proposal, r.LinkedStories})
}
func (s *Service) Propose(ctx context.Context, id string, p Proposal) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if r.DevelopmentMode != Continuous {
			return r, iox.NewPrecondition("historical delivery work is read-only", "start product work from the original request; historical files are retained", nil)
		}
		if frozen(r) || r.Status == Review {
			return r, iox.NewConflict("this version is frozen", "record an explicit change request before revising a candidate; finish the PR before new work", nil)
		}
		p.Intent = strings.TrimSpace(p.Intent)
		if p.Intent == "" || len(p.Stories) != 0 {
			return r, iox.NewInvalidInput("provide the specification in intent; internal delivery stories are retired", "use increment link to reference existing business stories", nil)
		}
		if p.StoryCodes != nil {
			links, linkErr := s.resolveLinks(ctx, p.StoryCodes)
			if linkErr != nil {
				return r, linkErr
			}
			r.LinkedStories = links
		}
		if len(r.Revisions) > 0 && hash(r.Revisions[len(r.Revisions)-1].Proposal) == hash(p) && hash(r.Revisions[len(r.Revisions)-1].LinkedStories) == hash(r.LinkedStories) && r.PendingChange == nil {
			return r, nil
		}
		revision := Revision{Number: len(r.Revisions) + 1, Proposal: p, CreatedAt: time.Now().UTC(), ApprovalSource: "request", Change: r.PendingChange, LinkedStories: append([]StoryLink(nil), r.LinkedStories...)}
		if r.PendingChange != nil {
			revision.Feedback = r.PendingChange.Feedback
		}
		r.Revisions = append(r.Revisions, revision)
		r.PendingChange = nil
		r.Status = Developing
		r.ReadySHA, r.ReadyScope, r.ApprovedSHA = "", "", ""
		r.Result = nil
		return r, s.save(r)
	})
}
func (s *Service) Ready(ctx context.Context, id string, result Result) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if r.DevelopmentMode != Continuous {
			return r, iox.NewPrecondition("historical delivery work is read-only", "start a new product work", nil)
		}
		if r.Status != Developing && r.Status != Review {
			return r, iox.NewConflict("increment is not being developed", "persist the current specification first", nil)
		}
		if strings.TrimSpace(result.Summary) == "" || len(result.Checks) == 0 {
			return r, iox.NewInvalidInput("result and verification evidence are required", "provide summary and checks", nil)
		}
		for _, check := range result.Checks {
			if strings.TrimSpace(check) == "" {
				return r, iox.NewInvalidInput("empty verification evidence", "name each completed check and its result", nil)
			}
		}
		if result.PreviewURL != "" {
			parsed, parseErr := url.Parse(result.PreviewURL)
			if parseErr != nil || parsed.User != nil || parsed.Scheme != "http" || (parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1") {
				return r, iox.NewInvalidInput("preview must be a local HTTP URL", "provide the local preview address, or omit it", parseErr)
			}
		}
		if len(r.Revisions) == 0 || r.PendingChange != nil {
			return r, iox.NewConflict("specification is missing", "persist the current specification before readiness", nil)
		}
		if err = s.requireClean(ctx, r); err != nil {
			return r, err
		}
		head, err := git(ctx, r.Workdir, "rev-parse", "HEAD")
		if err != nil {
			return r, err
		}
		diff, err := git(ctx, r.Workdir, "diff", "--name-only", r.BaseSHA+"...HEAD")
		if err != nil {
			return r, err
		}
		if diff == "" {
			return r, iox.NewConflict("increment has no committed product changes", "complete and commit the development before review", nil)
		}
		result.Evidence, err = verification.New(r.Workdir, s.Config.Paths.TestResults).Evidence(ctx)
		if err != nil {
			return r, err
		}
		if err = s.requireClean(ctx, r); err != nil {
			return r, err
		}
		verifiedHead, headErr := git(ctx, r.Workdir, "rev-parse", "HEAD")
		if headErr != nil {
			return r, headErr
		}
		if verifiedHead != head {
			return r, iox.NewConflict("candidate changed during verification", "verify the current committed candidate", nil)
		}
		r.Result = &result
		r.ReadySHA = head
		r.ReadyScope = candidateScope(r)
		r.Status = Review
		latest := &r.Revisions[len(r.Revisions)-1]
		latest.Result = &result
		latest.HeadSHA = head
		return r, s.save(r)
	})
}
func (s *Service) requireClean(ctx context.Context, r Record) error {
	branch, err := git(ctx, r.Workdir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return err
	}
	if branch != r.Branch {
		return iox.NewConflict("sandbox branch has changed", "restore the increment branch", nil)
	}
	args := append([]string{"status", "--porcelain", "--untracked-files=all", "--", "."}, gitwt.RuntimeArtifactPathspecs...)
	dirty, err := git(ctx, r.Workdir, args...)
	if err != nil {
		return err
	}
	if dirty != "" {
		return iox.NewConflict("sandbox has uncommitted changes", "commit and verify the current result before approving it", nil)
	}
	return nil
}
func (s *Service) FeedbackFor(ctx context.Context, id, feedback string) (Record, error) {
	return s.RequestChanges(ctx, id, FeedbackRequest{Feedback: feedback})
}
func (s *Service) RequestChanges(ctx context.Context, id string, change FeedbackRequest) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		change.Feedback = strings.TrimSpace(change.Feedback)
		if change.Feedback == "" || change.Kind != "" || len(change.StoryCodes) != 0 {
			return r, iox.NewInvalidInput("provide feedback without delivery classifications", "describe the requested behavior; the specification carries its scope", nil)
		}
		if r.Status == Developing && r.PendingChange != nil && r.PendingChange.Feedback == change.Feedback {
			return r, nil
		}
		if r.Status != Review {
			return r, iox.NewConflict("feedback requires a verified candidate", "wait for the result before requesting another version", nil)
		}
		r.Feedback = append(r.Feedback, change.Feedback)
		r.PendingChange = &change
		r.Status = Developing
		r.Result = nil
		r.ReadySHA, r.ReadyScope, r.ApprovedSHA = "", "", ""
		return r, s.save(r)
	})
}

// VerifyCandidate is shared by presentation and publication. A label is never
// sufficient authorization: publication repeats this check at its boundary.
func (s *Service) VerifyCandidate(ctx context.Context, r Record) error {
	if r.ReadySHA == "" || len(r.Revisions) == 0 || candidateScope(r) != r.ReadyScope {
		return iox.NewPrecondition("candidate specification changed", "verify the current specification", nil)
	}
	if err := s.requireClean(ctx, r); err != nil {
		return err
	}
	head, err := git(ctx, r.Workdir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != r.ReadySHA {
		return iox.NewConflict("candidate code changed", "verify the current code", nil)
	}
	if r.Result == nil {
		return iox.NewPrecondition("candidate has no result", "verify the candidate", nil)
	}
	if err = verification.New(r.Workdir, s.Config.Paths.TestResults).VerifyEvidence(ctx, r.Result.Evidence); err != nil {
		return err
	}
	if err = s.requireClean(ctx, r); err != nil {
		return err
	}
	currentHead, err := git(ctx, r.Workdir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if currentHead != head {
		return iox.NewConflict("candidate changed during verification", "verify the current committed candidate", nil)
	}
	return nil
}
