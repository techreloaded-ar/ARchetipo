package verification

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

// Qualification certifies setup repeatability, not product correctness, pristine
// installation or semantic preservation of application data. Those need explicit
// project checks chosen by the skill. All underlying reports/logs remain intact.
type Qualification struct {
	Passed       bool     `json:"passed"`
	BundleDigest string   `json:"bundle_digest"`
	InputKey     string   `json:"input_key"`
	Reports      []Result `json:"reports"`
	ReportPath   string   `json:"report_path"`
}

func (s *Service) qualificationKey(assets []string) (string, string, error) {
	recipe, err := LoadLocal(s.Root)
	if os.IsNotExist(err) {
		return "", "", iox.NewPrecondition("no candidate environment recipe", "configure the candidate successfully before qualification", err)
	}
	if err != nil {
		return "", "", err
	}
	bundle, err := environmentrecipe.Collect(s.Root, assets)
	if err != nil {
		return "", "", err
	}
	if err = validateCandidateAssets(recipe, bundle); err != nil {
		return "", "", err
	}
	digest := environmentrecipe.Digest(bundle)
	key, err := fingerprint(s.Root, recipe, recipe.Inputs, digest)
	return key, digest, err
}

func (s *Service) Qualify(ctx context.Context, assets []string) (Qualification, error) {
	s.ForCandidate()
	store, err := conversationlog.NewFileStore(s.Root)
	if err != nil {
		return Qualification{}, err
	}
	lease, err := store.TryLock(ctx, "environment-qualification")
	if err != nil {
		return Qualification{}, iox.NewConflict("another candidate qualification is running", "wait for its retained report", err)
	}
	defer func() { _ = lease.Unlock() }()
	q := Qualification{Reports: []Result{}, ReportPath: filepath.Join(s.dir(), "qualification.json")}
	// Revoke an older pass before any attempt, even if inspection fails.
	if err := recordfile.WriteAtomic(s.dir(), q.ReportPath, ".qualification-", q); err != nil {
		return q, err
	}
	key, digest, err := s.qualificationKey(assets)
	if err != nil {
		return q, err
	}
	q.InputKey, q.BundleDigest = key, digest
	// Two actual preparations expose non-repeatable setup. The third must reuse
	// the second preparation and still execute readiness probes.
	for _, fresh := range []bool{true, true, false} {
		report, runErr := s.Execute(ctx, "prepare", "", "", fresh)
		q.Reports = append(q.Reports, report)
		if err = recordfile.WriteAtomic(s.dir(), q.ReportPath, ".qualification-", q); err != nil {
			return q, err
		}
		if runErr != nil {
			return q, runErr
		}
		if !report.Passed || !report.Stable {
			return q, nil
		}
	}
	after, _, err := s.qualificationKey(assets)
	if err != nil {
		return q, err
	}
	q.Passed = after == key && q.Reports[2].Reused && q.Reports[2].PreparationID == q.Reports[1].ID
	return q, recordfile.WriteAtomic(s.dir(), q.ReportPath, ".qualification-", q)
}

func (s *Service) VerifyQualification(assets []string) (Qualification, error) {
	s.ForCandidate()
	var q Qualification
	raw, err := os.ReadFile(filepath.Join(s.dir(), "qualification.json"))
	if err != nil {
		return q, iox.NewPrecondition("no candidate qualification", "run environment qualify with every required asset", err)
	}
	if err = json.Unmarshal(raw, &q); err != nil {
		return q, err
	}
	key, digest, err := s.qualificationKey(assets)
	if err != nil {
		return q, err
	}
	if !q.Passed || q.InputKey != key || q.BundleDigest != digest || len(q.Reports) != 3 {
		return q, iox.NewPrecondition("candidate qualification is missing, failed or stale", "qualify the current recipe, declared inputs and complete helper bundle again", nil)
	}
	recipe, err := LoadLocal(s.Root)
	if err != nil {
		return q, err
	}
	for i, report := range q.Reports {
		steps := len(recipe.Prepare) + len(recipe.Probe)
		if i == 2 {
			steps = len(recipe.Probe)
		}
		retained, valid := s.retainedEvidence(report.ReportPath, report.InputKey, steps, i == 2)
		if !valid || retained.Operation != "prepare" || retained.ID != report.ID || retained.Reused != (i == 2) {
			return q, iox.NewPrecondition("candidate qualification evidence is missing or changed", "retain original reports and logs; qualify again", nil)
		}
	}
	if q.Reports[2].PreparationID != q.Reports[1].ID || q.Reports[2].ReusedReport != q.Reports[1].ReportPath {
		return q, iox.NewPrecondition("candidate reuse evidence does not match", "qualify again", nil)
	}
	return q, nil
}

// Helper contents must invalidate setup reuse. Transitive dependencies inside
// arbitrary project scripts remain an explicit discovery task of the skill.
func validateCandidateAssets(recipe Recipe, bundle environmentrecipe.Recipe) error {
	assets := map[string]bool{}
	for _, file := range bundle.Files {
		assets[file.Path] = true
		if !strings.HasPrefix(file.Path, "scripts/archetipo/") {
			continue
		}
		covered := false
		for _, input := range recipe.Inputs {
			if file.Path == input || strings.HasPrefix(file.Path, input+"/") {
				covered = true
				break
			}
		}
		if !covered {
			return invalid("candidate helper must be a declared setup input: " + file.Path)
		}
	}
	commands := append(append(append([]Command{}, recipe.Prepare...), recipe.Probe...), recipe.Cleanup...)
	for _, resource := range recipe.Resources {
		commands = append(commands, resource.Start, resource.Probe, resource.Stop, resource.Stopped)
	}
	for _, check := range recipe.Checks {
		commands = append(commands, check.Command)
	}
	for _, command := range commands {
		for _, arg := range command.Argv {
			if strings.HasPrefix(arg, "scripts/archetipo/") && !assets[arg] {
				return invalid("candidate command references an unpublished helper: " + arg)
			}
		}
	}
	return nil
}
