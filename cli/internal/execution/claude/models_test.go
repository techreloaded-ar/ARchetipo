package claude

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

const testModelResponse = `{"models":[
 {"value":"default","displayName":"Default (recommended)","resolvedModel":"claude-next","supportsEffort":true,"supportedEffortLevels":["low","high"]},
 {"value":"opus","displayName":"Opus from runtime","supportsEffort":true,"supportedEffortLevels":["low","high"]},
 {"value":"sonnet","displayName":"Sonnet from runtime","supportsEffort":true,"supportedEffortLevels":["low","high"]},
 {"value":"haiku","displayName":"Haiku from runtime"}
]}`

func catalogProvider(fake *fakeClaude) *Provider {
	return New(Options{Starter: fake, WorkingDir: func() (string, error) { return "/catalog-workspace", nil }})
}

func TestModelsLoadsRuntimeNamesIdentifiersAndOptionsWithoutStartingATurn(t *testing.T) {
	fake := newFakeClaude()
	fake.modelResponse = `{"models":[
 {"value":"default","displayName":"Native default","resolvedModel":"claude-future","supportsEffort":true,"supportedEffortLevels":["minimal","future-effort"]},
 {"value":"future-model","displayName":"Future Model"},
 {"value":"fallback"}
 ]}`
	models, err := catalogProvider(fake).Models(context.Background(), map[string]any{"command": "custom-claude", "model": "stale-model", "effort": "stale-effort"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 || models[0].ID != "default" || models[0].Label != "Native default" || !models[0].Default || models[1].ID != "future-model" || models[1].Label != "Future Model" || models[2].Label != "fallback" {
		t.Fatalf("catalog = %#v", models)
	}
	option := models[0].Options[0]
	want := []execution.ModelOptionChoice{{Value: "minimal", Label: "minimal"}, {Value: "future-effort", Label: "future-effort"}}
	if option.Name != "effort" || !reflect.DeepEqual(option.Choices, want) || len(models[1].Options) != 0 {
		t.Fatalf("options = %#v", models)
	}
	for _, choice := range option.Choices {
		if _, err := parseConfig(map[string]any{"effort": choice.Value}); err != nil {
			t.Fatal(err)
		}
	}
	frames := fake.framesReceived()
	if len(frames) != 1 || !strings.Contains(string(frames[0]), `"subtype":"initialize"`) {
		t.Fatalf("probe opened work: %s", frames)
	}
	starts, command, args := fake.spawned()
	if starts != 1 || command != "custom-claude" || fake.startedIn() != "/catalog-workspace" {
		t.Fatalf("spawn = %d %s %s", starts, command, fake.startedIn())
	}
	for _, flag := range []string{"--model", "--effort", "--session-id", "--resume"} {
		for _, arg := range args {
			if arg == flag {
				t.Fatalf("probe carried %s: %v", flag, args)
			}
		}
	}
	if fake.alive() {
		t.Fatal("probe process was not released")
	}
}

func TestModelsRefreshesTheCatalogOnEveryProviderRead(t *testing.T) {
	first, second := newFakeClaude(), newFakeClaude()
	second.modelResponse = `{"models":[{"value":"new","displayName":"New model"}]}`
	provider := New(Options{Starter: &claudeProcessSequence{processes: []*fakeClaude{first, second}}})
	before, err := provider.Models(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := provider.Models(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].ID != "default" || len(after) != 1 || after[0].ID != "new" || after[0].Default {
		t.Fatalf("catalog did not refresh: %#v -> %#v", before, after)
	}
}

func TestModelsRejectsInvalidRuntimeCatalogs(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"missing", `{}`, "no supported models"},
		{"null", `{"models":null}`, "no supported models"},
		{"empty", `{"models":[]}`, "no supported models"},
		{"bad shape", `{"models":{}}`, "decoding"},
		{"nameless", `{"models":[{"displayName":"Unknown"}]}`, "without an identifier"},
		{"duplicate", `{"models":[{"value":"same"},{"value":"same"}]}`, "more than once"},
		{"empty effort", `{"models":[{"value":"new","supportsEffort":true,"supportedEffortLevels":[""]}]}`, "empty effort"},
		{"duplicate effort", `{"models":[{"value":"new","supportsEffort":true,"supportedEffortLevels":["high","high"]}]}`, "more than once"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeClaude()
			fake.modelResponse = tc.body
			models, err := catalogProvider(fake).Models(context.Background(), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) || models != nil {
				t.Fatalf("models=%#v err=%v; want %s", models, err, tc.want)
			}
			if fake.alive() {
				t.Fatal("failed probe leaked its process")
			}
		})
	}
}

func TestModelsReportsConfigurationStartupRefusalAndCancellation(t *testing.T) {
	t.Run("configuration", func(t *testing.T) {
		fake := newFakeClaude()
		_, err := catalogProvider(fake).Models(context.Background(), map[string]any{"model": true})
		starts, _, _ := fake.spawned()
		if err == nil || starts != 0 {
			t.Fatalf("err=%v starts=%d", err, starts)
		}
	})
	t.Run("startup", func(t *testing.T) {
		provider := New(Options{Starter: &claudeProcessSequence{}})
		_, err := provider.Models(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "could not be started") {
			t.Fatal(err)
		}
	})
	t.Run("refusal", func(t *testing.T) {
		fake := newFakeClaude()
		fake.controlSubtype = "error"
		fake.controlError = "catalog unavailable"
		_, err := catalogProvider(fake).Models(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "catalog unavailable") {
			t.Fatal(err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		fake := newFakeClaude()
		fake.controlSilent = true
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := catalogProvider(fake).Models(ctx, nil)
		if !errors.Is(err, context.DeadlineExceeded) || fake.alive() {
			t.Fatalf("err=%v alive=%t", err, fake.alive())
		}
	})
	t.Run("process exit", func(t *testing.T) {
		fake := newFakeClaude()
		fake.controlSilent = true
		fake.end()
		_, err := catalogProvider(fake).Models(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "ended before answering") {
			t.Fatal(err)
		}
	})
}
