package increment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Dossier is a portable product record. It excludes the harness transcript,
// runtime ownership and local machine paths; the PR carries the same document.
func Dossier(r Record) string {
	var body strings.Builder
	fmt.Fprintf(&body, "# %s\n\n%s · opened %s\n\n## Original request\n\n%s\n", r.Title, r.ID, r.CreatedAt.Format("2006-01-02T15:04:05Z"), r.Request)
	for _, revision := range r.Revisions {
		fmt.Fprintf(&body, "\n## Specification v%d · %s\n\n%s\n", revision.Number, revision.CreatedAt.Format("2006-01-02T15:04:05Z"), revision.Intent)
		if revision.Feedback != "" {
			fmt.Fprintf(&body, "\nRequested change: %s\n", revision.Feedback)
		}
		for _, story := range revision.LinkedStories {
			fmt.Fprintf(&body, "\n### %s — %s\n\n%s\n\n%s\n", story.Code, story.Title, story.Body, story.URL)
		}
		for _, story := range revision.Stories {
			fmt.Fprintf(&body, "\n### Historical delivery story %s — %s\n\n%s\n", story.Code, story.Title, story.Body)
		}
		if revision.Result != nil {
			fmt.Fprintf(&body, "\n### Verified candidate `%s`\n\n%s\n", revision.HeadSHA, revision.Result.Summary)
			for _, check := range revision.Result.Checks {
				fmt.Fprintf(&body, "- %s\n", check)
			}
		}
	}
	if len(r.Learning) > 0 {
		fmt.Fprint(&body, "\n## Observations and decisions\n")
		for _, item := range r.Learning {
			fmt.Fprintf(&body, "\n%s\n\n%s\n\nDecision: %s\n", item.CreatedAt.Format("2006-01-02T15:04:05Z"), item.Observation, item.Decision)
		}
	}
	if r.ApprovedSHA != "" {
		fmt.Fprintf(&body, "\n## Business approval\n\nCandidate: `%s`\n", r.ApprovedSHA)
	}
	if r.ApprovedAt != nil {
		fmt.Fprintf(&body, "\nApproved at: %s\n", r.ApprovedAt.Format("2006-01-02T15:04:05Z"))
	}
	if r.Conclusion != "" {
		fmt.Fprintf(&body, "\n## Conclusion\n\n%s\n", r.Conclusion)
	}
	if r.PR != nil {
		fmt.Fprintf(&body, "\n## Developer handover\n\n%s\n", r.PR.URL)
	}
	if r.IntegrationSHA != "" {
		fmt.Fprintf(&body, "\nIntegrated PR head: `%s`\n", r.IntegrationSHA)
	}
	return body.String()
}

// ReadDossier preserves enterprise history after explicit environment removal.
func (s *Service) ReadDossier(id string) (Record, error) {
	r, err := s.Read(id)
	if err == nil {
		return r, nil
	}
	receipt, archiveErr := s.DeletionReceipt(id)
	if archiveErr == nil && receipt.Deleted {
		return receipt.Record, nil
	}
	return r, err
}

// History combines live work with the product record retained by deletion.
// No environment files, processes or native transcripts are restored.
func (s *Service) History() ([]Record, error) {
	records, err := s.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, r := range records {
		seen[r.ID] = true
	}
	entries, err := os.ReadDir(filepath.Join(s.Config.ProjectRoot, ".archetipo", "increment-archives"))
	if errors.Is(err, os.ErrNotExist) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "inc-") || seen[entry.Name()] {
			continue
		}
		receipt, readErr := s.DeletionReceipt(entry.Name())
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		if receipt.Deleted {
			records = append(records, receipt.Record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	return records, nil
}
