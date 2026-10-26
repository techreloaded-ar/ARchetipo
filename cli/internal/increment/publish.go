package increment

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
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
		// Stories published at intent approval keep their codes; older increments
		// approved before that get them here, persisted before any external write.
		if err = s.allocate(ctx, &r, stories); err != nil {
			return r, err
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
		if err = s.mirror(ctx, &r, stories, domain.StatusReview); err != nil {
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
	st.Body += "\n\n" + sourceMarker(r.ID, local.Code) + "\n"
	if r.PR != nil {
		st.Body += "\nProposta di incremento: " + r.PR.URL + "\n"
	}
	return canonical(st)
}

// officialBacklog reads the original backlog; a workspace without one is empty.
func (s *Service) officialBacklog(ctx context.Context) ([]domain.Spec, error) {
	existing, err := s.Connector.FetchBacklogItems(ctx, "")
	if err != nil {
		if ce, ok := err.(*iox.CodedError); !ok || ce.Code != iox.CodePreconditionMissing {
			return nil, err
		}
		existing = []domain.Spec{}
	}
	return existing, nil
}

// allocate assigns official codes to the local stories that have none yet and
// persists them before any connector write, so a retry reuses the same codes.
func (s *Service) allocate(ctx context.Context, r *Record, stories []domain.Spec) error {
	missing := r.PublishedEpic == ""
	for _, st := range stories {
		missing = missing || r.Published[st.Code] == ""
	}
	if !missing {
		return nil
	}
	existing, err := s.officialBacklog(ctx)
	if err != nil {
		return err
	}
	summary, err := s.Connector.ReadExistingBacklog(ctx)
	if err != nil {
		if ce, ok := err.(*iox.CodedError); !ok || ce.Code != iox.CodePreconditionMissing {
			return err
		}
	}
	known := append([]domain.Spec(nil), existing...)
	for _, code := range summary.Codes {
		known = append(known, domain.Spec{Code: code})
	}
	for _, epic := range summary.Epics {
		known = append(known, domain.Spec{Epic: epic})
	}
	for _, code := range r.Published {
		known = append(known, domain.Spec{Code: code})
	}
	known = append(known, domain.Spec{Epic: domain.Epic{Code: r.PublishedEpic}})
	if r.Published == nil {
		r.Published = map[string]string{}
	}
	next := nextCode(known, "US-")
	for _, st := range stories {
		if r.Published[st.Code] == "" {
			r.Published[st.Code] = fmt.Sprintf("US-%03d", next)
			next++
		}
	}
	if r.PublishedEpic == "" {
		r.PublishedEpic = fmt.Sprintf("EP-%03d", nextCode(known, "EP-"))
	}
	return s.save(*r)
}

// mirror makes the original backlog show the current approved stories in the
// given status. Until delivery the increment is the authority for their content.
func (s *Service) mirror(ctx context.Context, r *Record, stories []domain.Spec, status domain.Status) error {
	if err := s.allocate(ctx, r, stories); err != nil {
		return err
	}
	existing, err := s.officialBacklog(ctx)
	if err != nil {
		return err
	}
	byCode := map[string]domain.Spec{}
	for _, st := range existing {
		byCode[st.Code] = st
	}
	keep := map[string]bool{}
	for _, local := range stories {
		keep[local.Code] = true
		st := publishedStory(*r, local)
		previous, found := byCode[st.Code]
		if found {
			// List entries of some connectors (github) carry no body: the marker
			// and the content can only be judged on the detail.
			if previous, err = s.Connector.ReadSpecDetail(ctx, st.Code); err != nil {
				return err
			}
		}
		switch {
		case found && !strings.Contains(previous.Body, sourceMarker(r.ID, local.Code)):
			return iox.NewConflict("published story code "+st.Code+" is already in use", "reconcile the existing story before retrying", nil)
		case !found:
			if len(byCode) == 0 {
				_, err = s.Connector.SaveInitialBacklog(ctx, []domain.Spec{st})
			} else {
				_, err = s.Connector.AppendSpecs(ctx, []domain.Spec{st})
			}
			byCode[st.Code] = st
			previous = st
		case hash(canonical(previous)) != hash(st):
			_, err = s.Connector.UpdateSpec(ctx, st.Code, domain.SpecUpdate{Title: &st.Title, Priority: &st.Priority, Points: &st.Points, Scope: &st.Scope, BlockedBy: &st.BlockedBy, Body: &st.Body, Epic: &st.Epic})
		}
		if err != nil {
			return err
		}
		if previous.Status != status {
			if _, err = s.Connector.TransitionStatus(ctx, st.Code, status); err != nil {
				return err
			}
		}
	}
	if err = s.retire(ctx, r, byCode, keep); err != nil {
		return err
	}
	return s.save(*r)
}

// retire removes the published stories this increment no longer contains. A
// story is removed only when it still carries this increment's marker.
func (s *Service) retire(ctx context.Context, r *Record, byCode map[string]domain.Spec, keep map[string]bool) error {
	deleter, canDelete := s.Connector.(connector.SpecDeleter)
	for local, code := range r.Published {
		if keep[local] {
			continue
		}
		if _, found := byCode[code]; found {
			previous, err := s.Connector.ReadSpecDetail(ctx, code)
			if err != nil {
				return err
			}
			if strings.Contains(previous.Body, sourceMarker(r.ID, local)) {
				// ponytail: connectors without deletion (github, jira) only put the
				// retired story back to TODO; it is closed by hand.
				if canDelete {
					_, err = deleter.DeleteSpec(ctx, code)
				} else {
					_, err = s.Connector.TransitionStatus(ctx, code, domain.StatusTodo)
				}
				if err != nil {
					return err
				}
			}
		}
		delete(r.Published, local)
	}
	return nil
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
