package verification

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CheckObservation presents original reports, including failures. A historical
// pass is not current evidence. Cleanup/repreparation never erase these reports.
type CheckObservation struct {
	Name    string  `json:"name"`
	State   string  `json:"state"`
	Label   string  `json:"label"`
	Current bool    `json:"current"`
	Report  *Result `json:"report,omitempty"`
}

func (s *Service) ObserveChecks(ctx context.Context) ([]CheckObservation, error) {
	recipe, err := s.loadRecipe()
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(s.dir(), "runs", "*", "report.json"))
	if err != nil {
		return nil, err
	}
	// Scoped check reports use the same original report contract.
	if s.Results != "" {
		results := s.Results
		if !filepath.IsAbs(results) {
			results = filepath.Join(s.Root, results)
		}
		scoped, _ := filepath.Glob(filepath.Join(results, "*", "verification-*", "report.json"))
		paths = append(paths, scoped...)
	}
	latest := map[string]Result{}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			continue
		}
		safe, err := localPath(s.Root, filepath.ToSlash(rel))
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(safe)
		if err != nil {
			continue
		}
		var report Result
		if json.Unmarshal(raw, &report) != nil || report.EnvironmentID != s.environmentID() || !strings.HasPrefix(report.Operation, "check:") {
			continue
		}
		name := strings.TrimPrefix(report.Operation, "check:")
		if old, found := latest[name]; !found || report.StartedAt.After(old.StartedAt) {
			latest[name] = report
		}
	}
	resources, err := s.Resources()
	if err != nil {
		return nil, err
	}
	if len(recipe.Resources) > 0 {
		s.env = resourceEnv(resources, "")
	}
	checkpoint, checkpointErr := s.readState()
	prepared := checkpointErr == nil && s.prepared(recipe, checkpoint) == nil
	names := make([]string, 0, len(recipe.Checks))
	for name := range recipe.Checks {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]CheckObservation, 0, len(names))
	for _, name := range names {
		observation := CheckObservation{Name: name, State: "missing", Label: "Non eseguita"}
		if report, found := latest[name]; found {
			observation.Report = &report
			check := recipe.Checks[name]
			inputs := append(append([]string{RecipePath}, recipe.Inputs...), check.Inputs...)
			key, hashErr := fingerprint(s.Root, recipe, inputs, checkpoint.PreparationID)
			observation.Current = prepared && hashErr == nil && key == report.InputKey && report.PreparationID == checkpoint.PreparationID
			if report.Status == "running" {
				observation.State = "incomplete"
				observation.Label = "Esecuzione non conclusa"
			} else if !report.Passed || !report.Stable {
				observation.State = "failed"
				observation.Label = "Fallita"
			} else {
				observation.State = "passed"
				observation.Label = "Superata"
			}
			if !observation.Current {
				observation.Label = "Da riconfermare · ultimo esito: " + strings.ToLower(observation.Label)
			}

			if report.Reused {
				if original, ok := s.cached(report.ReusedReport, report.InputKey); ok {
					observation.Report = &original
				} else {
					observation.State = "unavailable"
					observation.Current = false
					observation.Label = "Evidenza originale mancante o modificata"
				}
			}
			// An intact report must retain its logs. Never silently replace missing
			// evidence with the agent's description or a different successful run.
			if report.Status == "complete" {
				for _, step := range report.Steps {
					rel, relErr := filepath.Rel(s.Root, step.LogPath)
					safe, pathErr := localPath(s.Root, filepath.ToSlash(rel))
					hash, logErr := "", error(nil)
					if relErr == nil && pathErr == nil {
						hash, logErr = hashFile(safe)
					}
					if relErr != nil || pathErr != nil || logErr != nil || hash != step.LogSHA256 {
						observation.State = "unavailable"
						observation.Current = false
						observation.Label = "Evidenza mancante o modificata"
						break
					}
				}
			}
		}
		result = append(result, observation)
	}
	return result, nil
}
