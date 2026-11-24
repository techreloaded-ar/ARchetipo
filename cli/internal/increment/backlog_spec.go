package increment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// BacklogSpecEpicTitle is the epic that collects the backlog specs of the
// increments that started from no business story.
const BacklogSpecEpicTitle = "Incrementi"

// ConversationQueryParameter is the viewer address parameter that opens a
// conversation. The backlog spec links the increment's conversation with it.
const ConversationQueryParameter = "conversation"

// BacklogSpecTitle is the title of the backlog spec an increment publishes.
func BacklogSpecTitle(r Record) string {
	return strings.TrimSpace(r.Title)
}

// BacklogSpecBody is the approved specification of the increment, verbatim,
// followed by one line that leads back to the work: the increment identity, the
// PR and — only where the reader can reach it — the conversation.
//
// The conversation lives in the workspace of the person who developed the
// increment, so its link is written only when the backlog itself is local to
// that workspace (the file connector). In a shared tracker it would be a link
// that works for nobody else.
func BacklogSpecBody(r Record, linkConversation bool) string {
	var body strings.Builder
	if len(r.Revisions) > 0 {
		body.WriteString(strings.TrimSpace(r.Revisions[len(r.Revisions)-1].Intent))
		body.WriteString("\n\n---\n\n")
	}
	fmt.Fprintf(&body, "Incremento `%s`", r.ID)
	if linkConversation && r.ConversationID != "" {
		fmt.Fprintf(&body, " · [Apri la conversazione dell’incremento](?%s=%s)", ConversationQueryParameter, r.ConversationID)
	}
	if r.PR != nil && r.PR.URL != "" {
		fmt.Fprintf(&body, " · [Pull request #%d](%s)", r.PR.Number, r.PR.URL)
	}
	body.WriteString("\n")
	return body.String()
}

// backlogIsAbsent tells an empty workspace from a failed read: a project that
// never had a backlog answers a missing precondition, and that is a backlog
// with no specs rather than a reason to refuse the publication.
func backlogIsAbsent(err error) bool {
	var coded *iox.CodedError
	return errors.As(err, &coded) && coded.Code == iox.CodePreconditionMissing
}

// ensureBacklogSpec records the published increment as one backlog spec waiting
// in REVIEW. It runs only after the person approved the candidate and the PR is
// open, and it is safe to repeat: a retried publication adopts the spec an
// interrupted one already wrote instead of writing a second one.
func (s *Service) ensureBacklogSpec(ctx context.Context, r *Record) error {
	if s.Connector == nil {
		return iox.NewPrecondition("backlog connector is unavailable", "configure the project connector", nil)
	}
	existing, err := s.Connector.FetchBacklogItems(ctx, "")
	if err != nil {
		if !backlogIsAbsent(err) {
			return err
		}
		existing = nil
	}
	title := BacklogSpecTitle(*r)
	for _, candidate := range existing {
		if candidate.Code != r.BacklogSpecCode && candidate.Title != title {
			continue
		}
		detail, readErr := s.Connector.ReadSpecDetail(ctx, candidate.Code)
		if readErr != nil {
			return readErr
		}
		// The increment id in the body is what makes a spec this increment's
		// own: a code or a title alone could belong to someone else's story.
		if !strings.Contains(detail.Body, r.ID) {
			continue
		}
		r.BacklogSpecCode = detail.Code
		return nil
	}
	epic, err := s.backlogSpecEpic(ctx, *r, existing)
	if err != nil {
		return err
	}
	spec := domain.Spec{
		Code:     domain.NextSpecCode(existing),
		Title:    title,
		Epic:     epic,
		Priority: domain.Priority("MEDIUM"),
		Status:   domain.StatusReview,
		Body:     BacklogSpecBody(*r, s.Config.Connector == config.ConnectorFile),
	}
	spec.Ref = spec.Code
	if _, err = s.Connector.AppendSpecs(ctx, []domain.Spec{spec}); err != nil {
		return err
	}
	// A tracker may open every new item in its first column: the spec is then
	// moved where the increment actually is, without asking which tracker it is.
	saved, err := s.Connector.ReadSpecDetail(ctx, spec.Code)
	if err != nil {
		return err
	}
	if saved.Status != domain.StatusReview {
		if _, err = s.Connector.TransitionStatus(ctx, spec.Code, domain.StatusReview); err != nil {
			return err
		}
	}
	r.BacklogSpecCode = spec.Code
	return nil
}

// backlogSpecEpic files the spec under the epic of the first business story the
// increment links, so the trace sits beside the requirement it answers. An
// increment that links none goes under the dedicated epic, created on first use
// and found again by its title afterwards.
func (s *Service) backlogSpecEpic(ctx context.Context, r Record, existing []domain.Spec) (domain.Epic, error) {
	for _, linked := range r.LinkedStories {
		story, err := s.Connector.ReadSpecDetail(ctx, linked.Code)
		if err == nil && story.Epic.Code != "" {
			return story.Epic, nil
		}
	}
	epics := []domain.Epic{}
	summary, err := s.Connector.ReadExistingBacklog(ctx)
	if err != nil && !backlogIsAbsent(err) {
		return domain.Epic{}, err
	}
	epics = append(epics, summary.Epics...)
	for _, spec := range existing {
		epics = append(epics, spec.Epic)
	}
	highest := 0
	for _, epic := range epics {
		if strings.EqualFold(strings.TrimSpace(epic.Title), BacklogSpecEpicTitle) && epic.Code != "" {
			return epic, nil
		}
		if suffix, ok := strings.CutPrefix(epic.Code, "EP-"); ok {
			if number, convErr := strconv.Atoi(suffix); convErr == nil && number > highest {
				highest = number
			}
		}
	}
	return domain.Epic{Code: fmt.Sprintf("EP-%03d", highest+1), Title: BacklogSpecEpicTitle}, nil
}

// closeBacklogSpec moves the increment's backlog spec to DONE once its PR has
// been merged. It is the only closure that spec has: nobody approves it by hand.
//
// A spec the person removed from the backlog is not put back, and one already
// closed is left alone. Any other failure is returned, so the increment is not
// declared delivered while its backlog trace still says it is under review.
func (s *Service) closeBacklogSpec(ctx context.Context, r Record) error {
	if r.BacklogSpecCode == "" || s.Connector == nil {
		return nil
	}
	spec, err := s.Connector.ReadSpecDetail(ctx, r.BacklogSpecCode)
	if err != nil {
		items, listErr := s.Connector.FetchBacklogItems(ctx, "")
		if listErr != nil {
			return err
		}
		for _, item := range items {
			if item.Code == r.BacklogSpecCode {
				return err
			}
		}
		return nil
	}
	if spec.Status == domain.StatusDone {
		return nil
	}
	_, err = s.Connector.TransitionStatus(ctx, r.BacklogSpecCode, domain.StatusDone)
	return err
}
