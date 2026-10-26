package verification

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCapabilitiesWorkWithoutRecipeAndRetainFailedProbe(t *testing.T) {
	s := New(t.TempDir(), "")
	// init creates this directory; there is deliberately no recipe/known stack.
	if err := os.MkdirAll(s.Root+"/.archetipo", 0700); err != nil {
		t.Fatal(err)
	}
	s.Runner = &testRunner{run: func(_ context.Context, _ string, _ Command) error { return errors.New("socket access denied") }}
	report, err := s.Inspect(context.Background(), []string{"archetipo-unavailable-tool-793"}, []Command{{Name: "daemon", Argv: []string{"project-daemon", "status"}, TimeoutSeconds: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Network.State != "unknown" || report.LocalWrite.State != "available" || report.Tools["archetipo-unavailable-tool-793"].State != "unavailable" {
		t.Fatalf("invented capability: %+v", report)
	}
	if len(report.Probes) != 1 || report.Probes[0].Passed || report.Probes[0].Steps[0].Error == "" {
		t.Fatal("failed probe hidden", report)
	}
	if _, err = os.Stat(report.Probes[0].Steps[0].LogPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(report.ReportPath); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilitiesRejectUnboundedProbeBeforeExecuting(t *testing.T) {
	s := New(t.TempDir(), "")
	runner := &testRunner{}
	s.Runner = runner
	if _, err := s.Inspect(context.Background(), nil, []Command{{Name: "probe", Argv: []string{"tool"}, TimeoutSeconds: 31}}); err == nil || len(runner.calls) != 0 {
		t.Fatal("invalid probe executed")
	}
}
