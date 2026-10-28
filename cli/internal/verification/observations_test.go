package verification

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCheckObservationsRetainFailuresAfterRepreparation(t *testing.T) {
	s, runner, _ := fixture(t)
	runner.run = func(_ context.Context, _ string, c Command) error {
		if c.Name == "test" {
			return errors.New("product assertion failed")
		}
		return nil
	}
	failed := execute(t, s, "check")
	if failed.Passed {
		t.Fatal("expected failed check")
	}
	observed, err := s.ObserveChecks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0].State != "failed" || !observed[0].Current {
		t.Fatalf("failure lost: %+v", observed)
	}
	if _, err = s.Execute(context.Background(), "prepare", "", "", true); err != nil {
		t.Fatal(err)
	}
	observed, err = s.ObserveChecks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observed[0].State != "failed" || observed[0].Current || observed[0].Report.ID != failed.ID {
		t.Fatalf("historical result incorrectly certified or erased: %+v", observed)
	}
}

func TestCheckObservationsDetectMissingOriginalLogs(t *testing.T) {
	s, _, _ := fixture(t)
	original := execute(t, s, "check")
	reused := execute(t, s, "check")
	if !reused.Reused {
		t.Fatal("expected reuse")
	}
	if err := os.Remove(original.Steps[0].LogPath); err != nil {
		t.Fatal(err)
	}
	observed, err := s.ObserveChecks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observed[0].State != "unavailable" || observed[0].Current {
		t.Fatalf("missing evidence presented as passed: %+v", observed)
	}
}

func TestCheckLogsAllowScopedEvidenceAndRefuseOtherProjectFiles(t *testing.T) {
	s, _, _ := fixture(t)
	report := execute(t, s, "check")
	if _, err := s.Logs(report, 1000); err != nil {
		t.Fatal("scoped report unreadable", err)
	}
	report.Steps[0].LogPath = s.Root + "/lock.json"
	if _, err := s.Logs(report, 1000); err == nil {
		t.Fatal("read file outside report directories")
	}
}
