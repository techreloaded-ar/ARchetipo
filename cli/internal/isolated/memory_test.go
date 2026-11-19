package isolated

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// memoryProvider is a fakeProvider whose engine reports its memory.
type memoryProvider struct {
	fakeProvider
	memory    int64
	memoryErr error
}

func (m *memoryProvider) EngineMemory(context.Context) (int64, error) {
	m.steps = append(m.steps, "memory")
	return m.memory, m.memoryErr
}

func startWithEngineMemory(t *testing.T, provider Provider, os string) []string {
	t.Helper()
	defer func(previousAnswers func(context.Context, string) bool, previousOS string) {
		viewerAnswers, hostOS = previousAnswers, previousOS
	}(viewerAnswers, hostOS)
	viewerAnswers = func(context.Context, string) bool { return true }
	hostOS = os
	var progress []string
	if _, err := Start(context.Background(), provider, Store{Dir: t.TempDir()}, DefaultID, "img", func(step string) { progress = append(progress, step) }); err != nil {
		t.Fatalf("Start() = %v; a memory reading must never block a start", err)
	}
	return progress
}

func memoryWarnings(progress []string) []string {
	var warnings []string
	for _, step := range progress {
		if strings.HasPrefix(step, "Attenzione: Docker dà ai container solo") {
			warnings = append(warnings, step)
		}
	}
	return warnings
}

func TestStartWarnsWhenTheEngineHasTooLittleMemory(t *testing.T) {
	// 2 GB configured in .wslconfig, as the engine reports it.
	provider := &memoryProvider{memory: 2 << 30}
	warnings := memoryWarnings(startWithEngineMemory(t, provider, "windows"))
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
	for _, want := range []string{"2.0 GB", ".wslconfig", "wsl --shutdown", "Settings → Resources"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning %q does not name %q", warnings[0], want)
		}
	}
	if !slices.Equal(provider.steps[:2], []string{"available", "memory"}) {
		t.Errorf("steps = %v: memory must be read once the engine is known to answer", provider.steps)
	}
}

func TestStartDoesNotWarnWithEnoughMemoryOrNoReading(t *testing.T) {
	for name, provider := range map[string]Provider{
		// 4 GB configured reads as a little less than 4 GiB.
		"four gigabytes configured": &memoryProvider{memory: 4_000_000_000},
		"eight gigabytes":           &memoryProvider{memory: 8_329_089_024},
		"unreadable":                &memoryProvider{memoryErr: errors.New("template: no MemTotal")},
		"no reporter":               &fakeProvider{},
	} {
		t.Run(name, func(t *testing.T) {
			if warnings := memoryWarnings(startWithEngineMemory(t, provider, "windows")); len(warnings) != 0 {
				t.Fatalf("unexpected warning: %v", warnings)
			}
		})
	}
}

func TestLowEngineMemoryWarningNamesWhereEachSystemSetsTheLimit(t *testing.T) {
	for os, want := range map[string]string{
		"windows": "%USERPROFILE%\\.wslconfig",
		"darwin":  "Docker Desktop → Settings → Resources → Memory",
		"linux":   "memoria della macchina",
	} {
		if warning := lowEngineMemoryWarning(1<<30, os); !strings.Contains(warning, want) {
			t.Errorf("%s: warning %q does not name %q", os, warning, want)
		}
	}
}

func TestDockerEngineMemoryReadsMemTotal(t *testing.T) {
	docker := Docker{Binary: "docker", Run: func(_ context.Context, args ...string) (string, error) {
		if !slices.Equal(args, []string{"info", "--format", "{{.MemTotal}}"}) {
			t.Fatalf("unexpected engine command: %v", args)
		}
		return "8329089024\n", nil
	}}
	total, err := docker.EngineMemory(context.Background())
	if err != nil || total != 8_329_089_024 {
		t.Fatalf("EngineMemory() = %d, %v", total, err)
	}
	docker.Run = func(context.Context, ...string) (string, error) { return "<no value>", nil }
	if _, err := docker.EngineMemory(context.Background()); err == nil {
		t.Fatal("an engine without MemTotal must report an error, not zero memory")
	}
}

func TestDockerEngineMemoryIsBoundedByTheProbeLimit(t *testing.T) {
	shortEngineLimits(t)
	docker := Docker{Binary: "docker", Run: func(ctx context.Context, _ ...string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	started := time.Now()
	if _, err := docker.EngineMemory(context.Background()); err == nil {
		t.Fatal("a silent engine must not report memory")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("EngineMemory waited %s on a silent engine", elapsed)
	}
}
