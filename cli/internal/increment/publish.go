package increment

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

func nextCode(specs []domain.Spec, prefix string) int {
	max := 0
	for _, st := range specs {
		code := st.Code
		if prefix == "EP-" {
			code = st.Epic.Code
		}
		if strings.HasPrefix(code, prefix) {
			if n, err := strconv.Atoi(strings.TrimPrefix(code, prefix)); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}

func (s *Service) Publish(ctx context.Context, id, expectedSHA string, revision int) (Record, error) {
	return s.locked(ctx, func() (Record, error) {
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
		if err = s.requireClean(ctx, r); err != nil {
			return r, err
		}
		head, err := git(ctx, r.Workdir, "rev-parse", "HEAD")
		if err != nil {
			return r, err
		}
		stories, err := s.completedStories(ctx, r)
		if err != nil {
			return r, err
		}
		if head != r.ReadySHA || hash(stories) != r.ReadyStories {
			return r, iox.NewConflict("code or stories changed after verification", "verify the new result before approval", nil)
		}
		if s.Connector == nil || s.Hosting == nil {
			return r, iox.NewPrecondition("publication is not configured", "configure the workspace connector and GitHub CLI", nil)
		}
		if _, err = git(ctx, r.Workdir, "remote", "get-url", "origin"); err != nil {
			return r, err
		}
		existing, err := s.Connector.FetchBacklogItems(ctx, "")
		if err != nil {
			if ce, ok := err.(*iox.CodedError); !ok || ce.Code != iox.CodePreconditionMissing {
				return r, err
			}
			existing = []domain.Spec{}
		}
		if len(r.Published) == 0 {
			summary, summaryErr := s.Connector.ReadExistingBacklog(ctx)
			if summaryErr != nil {
				if ce, ok := summaryErr.(*iox.CodedError); !ok || ce.Code != iox.CodePreconditionMissing {
					return r, summaryErr
				}
			}
			known := append([]domain.Spec(nil), existing...)
			for _, code := range summary.Codes {
				known = append(known, domain.Spec{Code: code})
			}
			for _, epic := range summary.Epics {
				known = append(known, domain.Spec{Epic: epic})
			}
			r.Published = map[string]string{}
			next := nextCode(known, "US-")
			for _, st := range stories {
				r.Published[st.Code] = fmt.Sprintf("US-%03d", next)
				next++
			}
			r.PublishedEpic = fmt.Sprintf("EP-%03d", nextCode(known, "EP-"))
		}
		r.ApprovedSHA = r.ReadySHA
		r.Status = Publishing
		if err = s.save(r); err != nil {
			return r, err
		}
		// A retry looks up the same head/base before creating, including after an
		// unknown network outcome. Story identities were persisted before any IO.
		if _, err = git(ctx, r.Workdir, "push", "origin", r.ApprovedSHA+":refs/heads/"+r.Branch); err != nil {
			return r, err
		}
		pr, err := s.Hosting.Find(ctx, r.Workdir, r.Branch, r.BaseBranch)
		if err != nil {
			return r, err
		}
		if pr == nil {
			var body strings.Builder
			fmt.Fprintf(&body, "%s\n\n%s\n\n", r.ID, r.Revisions[len(r.Revisions)-1].Intent)
			for _, st := range stories {
				fmt.Fprintf(&body, "- %s — %s\n", r.Published[st.Code], st.Title)
			}
			if r.Result != nil {
				fmt.Fprintf(&body, "\n%s\n\nVerifiche:\n", r.Result.Summary)
				for _, check := range r.Result.Checks {
					fmt.Fprintf(&body, "- %s\n", check)
				}
			}
			pr, err = s.Hosting.Create(ctx, r.Workdir, r.Branch, r.BaseBranch, r.Title, body.String())
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
		if err = s.publishStories(ctx, r, stories); err != nil {
			return r, err
		}
		r.Status = Proposed
		return r, s.save(r)
	})
}

func sourceMarker(id, code string) string {
	return "<!-- archetipo-increment:" + id + ":" + code + " -->"
}
func publishedStory(r Record, local domain.Spec) domain.Spec {
	st := canonical(local)
	st.Code = r.Published[local.Code]
	st.Epic = domain.Epic{Code: r.PublishedEpic, Title: r.Title}
	for i, dependency := range st.BlockedBy {
		st.BlockedBy[i] = r.Published[dependency]
	}
	st.Body += "\n\n" + sourceMarker(r.ID, local.Code) + "\n\nProposta di incremento: " + r.PR.URL + "\n"
	return canonical(st)
}
func (s *Service) publishStories(ctx context.Context, r Record, stories []domain.Spec) error {
	for _, local := range stories {
		st := publishedStory(r, local)
		marker := sourceMarker(r.ID, local.Code)
		existing, err := s.Connector.FetchBacklogItems(ctx, "")
		if err != nil {
			if ce, ok := err.(*iox.CodedError); !ok || ce.Code != iox.CodePreconditionMissing {
				return err
			}
			existing = []domain.Spec{}
		}
		found := false
		for _, previous := range existing {
			if previous.Code == st.Code {
				if !strings.Contains(previous.Body, marker) || hash(canonical(previous)) != hash(canonical(st)) {
					return iox.NewConflict("published story code changed or is already in use", "reconcile the existing story before retrying publication", nil)
				}
				found = true
				break
			}
		}
		if found {
			continue
		}
		if len(existing) == 0 {
			_, err = s.Connector.SaveInitialBacklog(ctx, []domain.Spec{st})
		} else {
			_, err = s.Connector.AppendSpecs(ctx, []domain.Spec{st})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Refresh(ctx context.Context, id string) (Record, error) {
	return s.locked(ctx, func() (Record, error) {
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
		if pr.HeadSHA != r.ApprovedSHA {
			return r, iox.NewConflict("PR changed after business approval", "verify the changed result before marking it delivered", nil)
		}
		r.PR = pr
		if pr.State == "MERGED" {
			// Each transition is repeatable. A crash partway through never invents a
			// merge, creates another story, or marks the increment delivered early.
			for _, local := range r.Revisions[len(r.Revisions)-1].Stories {
				code := r.Published[local.Code]
				st, readErr := s.Connector.ReadSpecDetail(ctx, code)
				if readErr != nil {
					return r, readErr
				}
				if hash(canonical(st)) != hash(publishedStory(r, local)) {
					return r, iox.NewConflict("published story changed after approval", "reconcile the story with the approved increment before marking it delivered", nil)
				}
			}
			for _, local := range r.Revisions[len(r.Revisions)-1].Stories {
				code := r.Published[local.Code]
				if _, err = s.Connector.TransitionStatus(ctx, code, domain.StatusDone); err != nil {
					return r, err
				}
			}
			r.Status = Delivered
		}
		return r, s.save(r)
	})
}
