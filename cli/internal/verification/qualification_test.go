package verification

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
)

func TestCandidateQualificationDoesNotReplaceCommonRecipe(t *testing.T) {
	s, runner, recipe := fixture(t)
	if _, err := environmentrecipe.Publish(s.Root, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.Root, environmentrecipe.Path))
	if err != nil {
		t.Fatal(err)
	}
	recipe.Prepare[0].Name = "candidate-install"
	raw := mustJSON(t, recipe)
	if _, err = ConfigureCandidate(s.Root, raw); err != nil {
		t.Fatal(err)
	}
	common, err := Load(s.Root)
	if err != nil || common.Prepare[0].Name != "install" {
		t.Fatal("candidate changed common contract", err)
	}
	q, err := s.Qualify(context.Background(), nil)
	if err != nil || !q.Passed {
		t.Fatalf("qualification %+v %v", q, err)
	}
	if strings.Join(runner.calls, ",") != "candidate-install,ready,candidate-install,ready,ready" {
		t.Fatal("did not execute twice then probe reuse", runner.calls)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, environmentrecipe.Path))
	if string(before) != string(after) {
		t.Fatal("qualification published the candidate")
	}
	if _, err = s.VerifyQualification(nil); err != nil {
		t.Fatal(err)
	}
	if _, err = environmentrecipe.PublishQualified(s.Root, nil, q.BundleDigest); err != nil {
		t.Fatal(err)
	}
	common, err = Load(s.Root)
	if err != nil || common.Prepare[0].Name != "candidate-install" {
		t.Fatal("qualified publication not effective", err)
	}
}

func TestQualificationRejectsChangedBundleInputsAndLostEvidence(t *testing.T) {
	for _, cause := range []string{"helper", "input", "log", "assets"} {
		t.Run(cause, func(t *testing.T) {
			s, _, recipe := fixture(t)
			assets := []string{"scripts/archetipo/helper.sh"}
			mustWrite(t, filepath.Join(s.Root, assets[0]), "original helper")
			recipe.Inputs = append(recipe.Inputs, assets[0])
			writeRecipe(t, s.Root, recipe)
			q, err := s.Qualify(context.Background(), assets)
			if err != nil || !q.Passed {
				t.Fatalf("qualify %+v %v", q, err)
			}
			switch cause {
			case "helper":
				mustWrite(t, filepath.Join(s.Root, assets[0]), "changed helper")
			case "input":
				mustWrite(t, filepath.Join(s.Root, "lock.json"), "changed manifest")
			case "log":
				if err = os.Remove(q.Reports[0].Steps[0].LogPath); err != nil {
					t.Fatal(err)
				}
			case "assets":
				assets = nil
			}
			if _, err = s.VerifyQualification(assets); err == nil {
				t.Fatal("stale or incomplete qualification accepted")
			}
		})
	}
}

func TestFailedRequalificationRevokesPreviousPass(t *testing.T) {
	s, runner, _ := fixture(t)
	q, err := s.Qualify(context.Background(), nil)
	if err != nil || !q.Passed {
		t.Fatal(err)
	}
	runner.run = func(_ context.Context, _ string, _ Command) error { return errors.New("daemon access denied") }
	q, err = s.Qualify(context.Background(), nil)
	if err != nil || q.Passed || len(q.Reports) != 1 || q.Reports[0].Passed {
		t.Fatalf("lost failed preparation: %+v %v", q, err)
	}
	if _, err = s.VerifyQualification(nil); err == nil {
		t.Fatal("older qualification survived failure")
	}
}

func TestQualifiedPublishRefusesLaterAssetMutation(t *testing.T) {
	s, _, _ := fixture(t)
	q, err := s.Qualify(context.Background(), nil)
	if err != nil || !q.Passed {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(s.Root, environmentrecipe.Preview), `{"version":1,"url":"http://127.0.0.1:{port}/","argv":["server","{port}"],"prepare_environment":true}`)
	if _, err = environmentrecipe.PublishQualified(s.Root, nil, q.BundleDigest); err == nil {
		t.Fatal("published changed bundle")
	}
	if _, err = os.Stat(filepath.Join(s.Root, environmentrecipe.Path)); !os.IsNotExist(err) {
		t.Fatal("failed publication wrote common bundle")
	}
}

func TestQualificationRequiresReferencedHelpersAndSetupCoverage(t *testing.T) {
	s, runner, recipe := fixture(t)
	helper := "scripts/archetipo/setup.sh"
	mustWrite(t, filepath.Join(s.Root, helper), "setup")
	recipe.Prepare[0].Argv = []string{helper}
	writeRecipe(t, s.Root, recipe)
	if _, err := s.Qualify(context.Background(), nil); err == nil {
		t.Fatal("missing helper accepted")
	}
	if _, err := s.Qualify(context.Background(), []string{helper}); err == nil {
		t.Fatal("unfingerprinted helper accepted")
	}
	if len(runner.calls) != 0 {
		t.Fatal("invalid bundle started setup")
	}
	recipe.Inputs = append(recipe.Inputs, "scripts/archetipo")
	writeRecipe(t, s.Root, recipe)
	q, err := s.Qualify(context.Background(), []string{helper})
	if err != nil || !q.Passed {
		t.Fatalf("complete bundle failed: %+v %v", q, err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
