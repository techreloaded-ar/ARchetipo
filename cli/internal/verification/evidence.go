package verification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// prepared checks retained setup evidence without installing dependencies or
// changing service ownership. Live checks must never repair a running app.
func (s *Service) prepared(recipe Recipe, value state) error {
	key, err := s.preparationFingerprint()
	if err != nil {
		return err
	}
	_, retained := s.cachedEvidence(filepath.Join(s.dir(), "runs", filepath.Base(value.PreparationID), "report.json"), key, len(recipe.Prepare)+len(recipe.Probe))
	if value.PreparationID == "" || value.PreparationKey != key || !retained || !s.outputsExist(recipe.Outputs) {
		return iox.NewPrecondition("project setup has changed or its evidence is missing", "stop the preview and prepare the environment before running checks", nil)
	}
	return nil
}

// Evidence returns original, intact reports for every declared project check.
// It never runs commands and never treats the agent's narrative as proof.
func (s *Service) Evidence(ctx context.Context) ([]Result, error) {
	unlock, err := s.readLock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	recipe, err := s.loadRecipe()
	if err != nil {
		return nil, err
	}
	if len(recipe.Checks) == 0 {
		return nil, iox.NewPrecondition("no project checks declared", "declare and run the checks needed to verify this product change", nil)
	}
	resources, err := s.Resources()
	if err != nil {
		return nil, err
	}
	if len(recipe.Resources) > 0 {
		s.env = resourceEnv(resources, "")
	}
	value, err := s.readState()
	if err != nil {
		return nil, err
	}
	if err = s.prepared(recipe, value); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(recipe.Checks))
	for name := range recipe.Checks {
		names = append(names, name)
	}
	sort.Strings(names)
	reports := make([]Result, 0, len(names))
	for _, name := range names {
		check := recipe.Checks[name]
		inputs := append(append([]string{RecipePath}, recipe.Inputs...), check.Inputs...)
		key, hashErr := fingerprint(s.Root, recipe, inputs, value.PreparationID)
		if hashErr != nil {
			return nil, hashErr
		}
		report, valid := s.cached(value.Checks[name], key)
		if !valid || report.Operation != "check:"+name || report.PreparationID != value.PreparationID {
			return nil, iox.NewPrecondition("missing or stale verification: "+name, "run execution environment check "+name+" for the current candidate; keep its original report and logs", nil)
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// A live check shares services, never their lifecycle. Cancel promptly when the
// owning preview/run stops, allowing its guard to clean up after the supervisor.
func monitorLifetime(ctx context.Context, cancel context.CancelFunc, store *conversationlog.FileStore, root string, watched []string) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			owned := false
			for _, name := range watched {
				lock, err := store.TryLock(ctx, name)
				if err == nil {
					_ = lock.Unlock()
				} else if errors.Is(err, conversationlog.ErrLockHeld) {
					owned = true
					if name == "environment-lifetime" {
						if _, stopErr := os.Stat(filepath.Join(root, ".archetipo", "environment", "stopping")); stopErr == nil {
							cancel()
							return
						}
					}
				} else {
					cancel()
					return
				}
			}
			if !owned {
				cancel()
				return
			}
		}
	}
}

// VerifyEvidence retains the exact reports displayed with a candidate. It reads
// the recipe and the original reports only: stopping services, a cleanup or a
// new preparation never revoke a recorded candidate, while a later completed
// run of the same check that failed on the current declared inputs does.
func (s *Service) VerifyEvidence(ctx context.Context, expected []Result) error {
	unlock, err := s.readLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	recipe, err := s.loadRecipe()
	if err != nil {
		return err
	}
	if len(expected) == 0 || len(expected) != len(recipe.Checks) {
		return iox.NewPrecondition("candidate has no complete frozen evidence", "verify and record the current candidate", nil)
	}
	latest, err := s.latestCheckReports(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, report := range expected {
		name := strings.TrimPrefix(report.Operation, "check:")
		check, declared := recipe.Checks[name]
		original, retained := s.cached(report.ReportPath, report.InputKey)
		if !declared || seen[name] || !retained || original.ID != report.ID || original.Operation != report.Operation || original.PreparationID != report.PreparationID {
			return iox.NewPrecondition("candidate verification evidence changed or was removed", "verify and record the current candidate before approval", nil)
		}
		seen[name] = true
		inputs := append(append([]string{RecipePath}, recipe.Inputs...), check.Inputs...)
		key, hashErr := fingerprint(s.Root, recipe, inputs, report.PreparationID)
		if hashErr != nil {
			return hashErr
		}
		if key != report.InputKey {
			return iox.NewPrecondition("candidate check inputs changed: "+name, "run execution environment check "+name+" and record the current candidate", nil)
		}
		newer, found := latest[name]
		if !found || !newer.StartedAt.After(original.StartedAt) || newer.Status != "complete" || (newer.Passed && newer.Stable) {
			continue
		}
		newerKey, hashErr := fingerprint(s.Root, recipe, inputs, newer.PreparationID)
		if hashErr != nil {
			return hashErr
		}
		if newerKey == newer.InputKey {
			return iox.NewPrecondition("a later run of check "+name+" failed on this candidate", "fix the defect, run the check again and record the current candidate", nil)
		}
	}
	return nil
}

// readLock queues a read-only presentation briefly behind a running command
// rather than letting it masquerade as an active test.
func (s *Service) readLock(ctx context.Context) (func(), error) {
	store, err := conversationlog.NewFileStore(s.Root)
	if err != nil {
		return nil, err
	}
	lockCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	lock, err := store.Lock(lockCtx, "verification-environment")
	if err != nil {
		return nil, iox.NewConflict("verification is still running", "wait for the current check to finish", err)
	}
	return func() { _ = lock.Unlock() }, nil
}
