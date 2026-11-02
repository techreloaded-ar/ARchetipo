package increment

import (
	"context"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"strings"
)

// Link references the original enterprise backlog. It never clones stories or
// silently changes their scope/status when a work is tested, published or merged.
func (s *Service) Link(ctx context.Context, id string, codes []string) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if frozen(r) || r.Status == Review {
			return r, iox.NewConflict("the candidate's business references are frozen", "request a new version before changing its scope", nil)
		}
		if len(r.Revisions) > 0 {
			return r, iox.NewConflict("business references belong to the specification", "provide story_codes with increment propose to version changes", nil)
		}
		links, err := s.resolveLinks(ctx, codes)
		if err != nil {
			return r, err
		}
		r.LinkedStories = links
		return r, s.save(r)
	})
}
func (s *Service) resolveLinks(ctx context.Context, codes []string) ([]StoryLink, error) {
	links := []StoryLink{}
	seen := map[string]bool{}
	for _, raw := range codes {
		code := strings.TrimSpace(raw)
		if code == "" || seen[code] {
			return nil, iox.NewInvalidInput("empty or duplicate story reference", "use distinct existing backlog codes", nil)
		}
		if s.Connector == nil {
			return nil, iox.NewPrecondition("backlog connector is unavailable", "configure the project connector", nil)
		}
		spec, err := s.Connector.ReadSpecDetail(ctx, code)
		if err != nil {
			return nil, err
		}
		if spec.Code == "" || seen[spec.Code] {
			return nil, iox.NewInvalidInput("duplicate or missing story identity", "reference each story once", nil)
		}
		seen[code], seen[spec.Code] = true, true
		links = append(links, StoryLink{Code: spec.Code, Title: spec.Title, Body: spec.Body, URL: spec.URL})
	}
	return links, nil
}
