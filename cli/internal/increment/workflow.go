package increment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/gitwt"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/preview"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/validation"
)

func canonical(st domain.Spec) domain.Spec {
	body := strings.TrimSpace(strings.ReplaceAll(st.Body, "\r\n", "\n"))
	// The existing request-changes operation appends technical review notes.
	// They remain in local history but are not new product requirements.
	reviewSection := "\n" + strings.TrimSpace(domain.AppendReworkFeedback("", nil))
	body, _, _ = strings.Cut(body, reviewSection)
	return domain.Spec{Code: st.Code, Title: st.Title, Epic: st.Epic, Priority: st.Priority, Points: st.Points, Scope: st.Scope, Body: strings.TrimSpace(body), BlockedBy: append([]string(nil), st.BlockedBy...), Status: domain.StatusTodo}
}
func hash(value any) string {
	raw, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func (s *Service) local(r Record) (*filefs.Connector, error) {
	cfg, err := config.LoadExact(r.Workdir)
	if err != nil {
		return nil, err
	}
	if cfg.Increment == nil || cfg.Increment.ID != r.ID {
		return nil, iox.NewConflict("increment sandbox metadata is missing", "restore its workspace before continuing", nil)
	}
	return filefs.New(cfg), nil
}

// Stories reads current sandbox facts without changing the accepted revision.
// Callers keep the proposal readable when an older sandbox is unavailable.
func (s *Service) Stories(ctx context.Context, r Record) ([]domain.Spec, error) {
	local, err := s.local(r)
	if err != nil {
		return nil, err
	}
	return local.FetchBacklogItems(ctx, "")
}

func frozen(r Record) bool {
	return r.Status == Publishing || r.Status == Proposed || r.Status == Delivered
}

func (s *Service) Propose(ctx context.Context, id string, p Proposal) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if frozen(r) {
			return r, iox.NewConflict("increment already approved for publication", "finish the existing PR before starting other work", nil)
		}
		if r.PendingChange != nil && r.PendingChange.Kind == "correction" {
			return r, iox.NewConflict("correction story reopening is incomplete", "retry increment feedback with the same classified request", nil)
		}
		if r.Status == Review {
			return r, iox.NewConflict("record the requested changes before revising the intent", "use increment feedback for an explicit change request; ordinary questions keep the verified result", nil)
		}
		p.Intent = strings.TrimSpace(p.Intent)
		if p.Intent == "" {
			return r, iox.NewInvalidInput("intent is required", "describe the outcome, scope and demonstration", nil)
		}
		known := map[string]bool{}
		for i, st := range p.Stories {
			st = canonical(st)
			p.Stories[i] = st
			known[st.Code] = true
		}
		valid := validation.ValidateSpecs(id, p.Stories)
		if !valid.OK {
			return r, iox.NewInvalidInput("invalid increment stories", fmt.Sprintf("%+v", valid.Findings), nil)
		}
		for _, st := range p.Stories {
			for _, dependency := range st.BlockedBy {
				if !known[dependency] {
					return r, iox.NewInvalidInput("story dependency is outside the increment", "include every dependency in this proposal", nil)
				}
			}
		}
		if change := r.PendingChange; change != nil && change.Kind == "refinement" {
			previous := r.Revisions[len(r.Revisions)-1].Stories
			if len(previous) != len(p.Stories) {
				return r, iox.NewInvalidInput("a refinement must keep existing story identities", "declare a scope change to add or retire stories", nil)
			}
			affected := map[string]bool{}
			for _, code := range change.StoryCodes {
				affected[code] = true
			}
			proposed := map[string]domain.Spec{}
			for _, st := range p.Stories {
				proposed[st.Code] = st
			}
			for _, st := range previous {
				next, exists := proposed[st.Code]
				changed := hash(canonical(st)) != hash(canonical(next))
				if !exists || changed != affected[st.Code] {
					return r, iox.NewInvalidInput("refinement stories do not match the affected codes", "update the linked stories with the requested criteria and preserve unrelated stories", nil)
				}
			}
		}
		// A changed intent always needs a new human decision, even if the same
		// stories remain. Old revisions and their evidence remain inspectable.
		revision := Revision{Number: len(r.Revisions) + 1, Proposal: p, CreatedAt: time.Now().UTC()}
		revision.Change = r.PendingChange
		if len(r.Feedback) > 0 {
			revision.Feedback = r.Feedback[len(r.Feedback)-1]
		}
		r.Revisions = append(r.Revisions, revision)
		r.PendingChange = nil
		r.Status = Discovery
		r.ReadySHA = ""
		r.ReadyStories = ""
		r.ApprovedSHA = ""
		r.Result = nil
		return r, s.save(r)
	})
}

func (s *Service) Approve(ctx context.Context, id string, revision int) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if len(r.Revisions) == 0 || r.Revisions[len(r.Revisions)-1].Number != revision {
			return r, iox.NewConflict("intent revision has changed", "read the latest intent before approving", nil)
		}
		if frozen(r) {
			return r, iox.NewConflict("increment is already published", "read its PR", nil)
		}
		latest := &r.Revisions[len(r.Revisions)-1]
		if r.Status != Discovery && (r.Status != Developing || !latest.Approved) {
			return r, iox.NewConflict("intent cannot be approved in this state", "read the current increment", nil)
		}
		latest.Approved = true
		r.Status = Developing
		// Persist the decision first. Repeating this command reconciles the same
		// story set after a crash; it never creates a second set of local codes.
		if err = s.save(r); err != nil {
			return r, err
		}
		local, err := s.local(r)
		if err != nil {
			return r, err
		}
		existing, err := local.FetchBacklogItems(ctx, "")
		if err != nil {
			return r, err
		}
		desired := map[string]domain.Spec{}
		for _, st := range latest.Stories {
			desired[st.Code] = st
		}
		present := map[string]bool{}
		for _, st := range existing {
			target, keep := desired[st.Code]
			if keep && hash(canonical(st)) == hash(canonical(target)) {
				present[st.Code] = true
				continue
			}
			if keep {
				// Replanning invalidates stale work artifacts, not the story's
				// identity or the history of its earlier delivery rounds.
				target.History = append([]domain.StatusChange(nil), st.History...)
				if st.Status != domain.StatusTodo {
					target.History = append(target.History, domain.StatusChange{Status: domain.StatusTodo, At: time.Now().UTC().Format(time.RFC3339)})
				}
				desired[st.Code] = target
			}
			if _, err = local.DeleteSpec(ctx, st.Code); err != nil {
				return r, err
			}
		}
		missing := []domain.Spec{}
		for _, st := range latest.Stories {
			if !present[st.Code] {
				missing = append(missing, desired[st.Code])
			}
		}
		if len(missing) > 0 {
			_, err = local.AppendSpecs(ctx, missing)
		}
		return r, err
	})
}

// SyncStories shows the approved stories in the original backlog as in progress;
// publication moves them to review and the confirmed merge to done. It is apart
// from Approve so an agent confined to the sandbox can still approve: the next
// caller allowed to write the workspace, or publication, catches up.
func (s *Service) SyncStories(ctx context.Context, id string) (Record, error) {
	return s.lockedWorkspaceRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil || s.Connector == nil || frozen(r) || len(r.Revisions) == 0 || !r.Revisions[len(r.Revisions)-1].Approved {
			return r, err
		}
		return r, s.mirror(ctx, &r, r.Revisions[len(r.Revisions)-1].Stories, domain.StatusInProgress)
	})
}

func (s *Service) Ready(ctx context.Context, id string, result Result) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if r.Status != Developing && r.Status != Review {
			return r, iox.NewConflict("increment is not being developed", "approve its current intent first", nil)
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
		stories, err := s.completedStories(ctx, r)
		if err != nil {
			return r, err
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
		result.Simulated = nil
		if recipe, previewErr := preview.Load(r.Workdir); previewErr == nil {
			result.Simulated = append([]string(nil), recipe.Simulated...)
		}
		r.Result = &result
		r.ReadySHA = head
		r.ReadyStories = hash(stories)
		r.Status = Review
		latest := &r.Revisions[len(r.Revisions)-1]
		latest.Result = &result
		latest.HeadSHA = head
		return r, s.save(r)
	})
}
func (s *Service) completedStories(ctx context.Context, r Record) ([]domain.Spec, error) {
	if r.PendingChange != nil && r.PendingChange.Kind == "correction" {
		return nil, iox.NewConflict("correction story reopening is incomplete", "retry increment feedback with the same classified request", nil)
	}
	if len(r.Revisions) == 0 || !r.Revisions[len(r.Revisions)-1].Approved {
		return nil, iox.NewConflict("the latest intent is not approved", "approve it before development", nil)
	}
	local, err := s.local(r)
	if err != nil {
		return nil, err
	}
	stories, err := local.FetchBacklogItems(ctx, "")
	if err != nil {
		return nil, err
	}
	expected := r.Revisions[len(r.Revisions)-1].Stories
	if len(stories) != len(expected) {
		return nil, iox.NewConflict("sandbox stories differ from the accepted proposal", "reconcile the stories with the current intent", nil)
	}
	byCode := map[string]domain.Spec{}
	for _, st := range stories {
		byCode[st.Code] = st
	}
	ordered := []domain.Spec{}
	for _, accepted := range expected {
		st, ok := byCode[accepted.Code]
		if !ok || st.Status != domain.StatusDone || hash(canonical(st)) != hash(canonical(accepted)) {
			return nil, iox.NewConflict("story "+accepted.Code+" is not complete or changed after approval", "finish implementation and technical review against the accepted intent", nil)
		}
		tasks, taskErr := local.ReadSpecTasks(ctx, st.Code)
		// A missing plan is valid for direct delivery. Malformed or unreadable
		// existing plans still block readiness; their tasks cannot be ignored.
		if taskErr != nil && !errors.Is(taskErr, os.ErrNotExist) {
			return nil, taskErr
		}
		for _, task := range tasks {
			if task.Status != domain.StatusDone {
				return nil, iox.NewConflict("story "+st.Code+" has unfinished tasks", "complete all tasks before review", nil)
			}
		}
		ordered = append(ordered, canonical(st))
	}
	return ordered, nil
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
		if change.Feedback == "" {
			return r, iox.NewInvalidInput("feedback is required", "describe what should change", nil)
		}
		if change.Kind == "" {
			change.Kind = "scope"
		}
		if r.Status == Developing && r.PendingChange != nil && r.PendingChange.Kind == "correction" && hash(*r.PendingChange) == hash(change) {
			return s.reopenCorrection(ctx, r)
		}
		if r.Status != Review {
			return r, iox.NewConflict("feedback requires a result ready to review", "wait for development to finish", nil)
		}
		if change.Kind != "correction" && change.Kind != "refinement" && change.Kind != "scope" {
			return r, iox.NewInvalidInput("unknown feedback kind", "use correction, refinement or scope", nil)
		}
		if (change.Kind == "correction" || change.Kind == "refinement") && len(change.StoryCodes) == 0 {
			return r, iox.NewInvalidInput("affected stories are required", "link the request to existing story codes", nil)
		}
		if len(r.Revisions) == 0 || !r.Revisions[len(r.Revisions)-1].Approved {
			return r, iox.NewConflict("feedback requires an approved intent", "read the current increment", nil)
		}
		latest := r.Revisions[len(r.Revisions)-1]
		known := map[string]bool{}
		for _, st := range latest.Stories {
			known[st.Code] = true
		}
		seen := map[string]bool{}
		for _, code := range change.StoryCodes {
			if !known[code] || seen[code] {
				return r, iox.NewInvalidInput("unknown or duplicate affected story: "+code, "use distinct codes from the approved intent", nil)
			}
			seen[code] = true
		}
		if change.Kind == "correction" {
			if _, err = s.completedStories(ctx, r); err != nil {
				return r, err
			}
			latest.Number = len(r.Revisions) + 1
			latest.CreatedAt = time.Now().UTC()
			latest.Feedback = change.Feedback
			latest.Change = &change
			latest.Result = nil
			latest.HeadSHA = ""
			r.Revisions = append(r.Revisions, latest)
		}
		r.Feedback = append(r.Feedback, change.Feedback)
		r.Status = Discovery
		r.PendingChange = &change
		if change.Kind == "correction" {
			r.Status = Developing
		}
		r.Result = nil
		r.ReadySHA = ""
		r.ReadyStories = ""
		r.ApprovedSHA = ""
		// Save the request and revoke the verified result before reopening any
		// story. A crash leaves an explicit, retryable pending correction.
		if err = s.save(r); err != nil {
			return r, err
		}
		if change.Kind == "correction" {
			return s.reopenCorrection(ctx, r)
		}
		return r, nil
	})
}

func (s *Service) reopenCorrection(ctx context.Context, r Record) (Record, error) {
	local, err := s.local(r)
	if err != nil {
		return r, err
	}
	// Keep the story, its plan, tasks and status history. Repeating a partial
	// reopen adds no duplicate transitions and never starts another revision.
	for _, code := range r.PendingChange.StoryCodes {
		if _, err = local.TransitionStatus(ctx, code, domain.StatusTodo); err != nil {
			return r, err
		}
	}
	r.PendingChange = nil
	return r, s.save(r)
}
