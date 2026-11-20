package isolated

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// cpuProvider is a fakeProvider whose engine reports its processors.
type cpuProvider struct {
	fakeProvider
	engineCPUs    int
	engineCPUsErr error
}

func (c *cpuProvider) EngineCPUs(context.Context) (int, error) {
	c.steps = append(c.steps, "cpus")
	return c.engineCPUs, c.engineCPUsErr
}

func startWithHostCPUs(t *testing.T, provider Provider, os string, host int) []string {
	t.Helper()
	defer func(previousAnswers func(context.Context, string) bool, previousOS string, previousCPUs func() int) {
		viewerAnswers, hostOS, hostCPUs = previousAnswers, previousOS, previousCPUs
	}(viewerAnswers, hostOS, hostCPUs)
	viewerAnswers = func(context.Context, string) bool { return true }
	hostOS = os
	hostCPUs = func() int { return host }
	var progress []string
	if _, err := Start(context.Background(), provider, Store{Dir: t.TempDir()}, DefaultID, "img", func(step string) { progress = append(progress, step) }); err != nil {
		t.Fatalf("Start() = %v; a processor reading must never block a start", err)
	}
	return progress
}

func cpuWarnings(progress []string) []string {
	var warnings []string
	for _, step := range progress {
		if strings.HasPrefix(step, "Nota: Docker dà ai container") {
			warnings = append(warnings, step)
		}
	}
	return warnings
}

func TestStartSaysWhenTheEngineHasFewOfTheComputersProcessors(t *testing.T) {
	// processors=4 in .wslconfig on a computer with 24: the observed case.
	provider := &cpuProvider{engineCPUs: 4}
	warnings := cpuWarnings(startWithHostCPUs(t, provider, "windows", 24))
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
	for _, want := range []string{"4 processori su 24", "processors=", ".wslconfig", "almeno 12", "wsl --shutdown"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning %q does not name %q", warnings[0], want)
		}
	}
	if !slices.Equal(provider.steps[:2], []string{"available", "cpus"}) {
		t.Errorf("steps = %v: processors must be read once the engine is known to answer", provider.steps)
	}
}

func TestStartIsSilentWithEnoughProcessorsOrNoReading(t *testing.T) {
	for name, scenario := range map[string]struct {
		provider Provider
		host     int
	}{
		"engine has every processor":        {&cpuProvider{engineCPUs: 8}, 8},
		"engine has half, a common default": {&cpuProvider{engineCPUs: 5}, 10},
		"engine has enough on a large host": {&cpuProvider{engineCPUs: 8}, 64},
		"small computer":                    {&cpuProvider{engineCPUs: 2}, 4},
		"unreadable":                        {&cpuProvider{engineCPUsErr: errors.New("template: no NCPU")}, 24},
		"engine reports nothing":            {&cpuProvider{engineCPUs: 0}, 24},
		"no reporter":                       {&fakeProvider{}, 24},
	} {
		t.Run(name, func(t *testing.T) {
			if warnings := cpuWarnings(startWithHostCPUs(t, scenario.provider, "linux", scenario.host)); len(warnings) != 0 {
				t.Fatalf("unexpected warning: %v", warnings)
			}
		})
	}
}

func TestFewEngineCPUsWarningNamesWhereEachSystemSetsTheLimit(t *testing.T) {
	for os, want := range map[string]string{
		"windows": "%USERPROFILE%\\.wslconfig",
		"darwin":  "Docker Desktop → Settings → Resources → CPU limit e riavvia",
		"linux":   "processori della macchina",
	} {
		if warning := fewEngineCPUsWarning(2, 12, os); !strings.Contains(warning, want) || !strings.Contains(warning, "almeno 6") {
			t.Errorf("%s: warning %q does not name %q and the suggested count", os, warning, want)
		}
	}
}

func TestDockerEngineCPUsReadsNCPU(t *testing.T) {
	docker := Docker{Binary: "docker", Run: func(_ context.Context, args ...string) (string, error) {
		if !slices.Equal(args, []string{"info", "--format", "{{.NCPU}}"}) {
			t.Fatalf("unexpected engine command: %v", args)
		}
		return "4\n", nil
	}}
	count, err := docker.EngineCPUs(context.Background())
	if err != nil || count != 4 {
		t.Fatalf("EngineCPUs() = %d, %v", count, err)
	}
	docker.Run = func(context.Context, ...string) (string, error) { return "<no value>", nil }
	if _, err := docker.EngineCPUs(context.Background()); err == nil {
		t.Fatal("an engine without NCPU must report an error, not zero processors")
	}
}

func TestDockerEngineCPUsIsBoundedByTheProbeLimit(t *testing.T) {
	shortEngineLimits(t)
	docker := Docker{Binary: "docker", Run: func(ctx context.Context, _ ...string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	started := time.Now()
	if _, err := docker.EngineCPUs(context.Background()); err == nil {
		t.Fatal("a silent engine must not report processors")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("EngineCPUs waited %s on a silent engine", elapsed)
	}
}
