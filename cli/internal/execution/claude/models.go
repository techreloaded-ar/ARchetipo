package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

const modelListTimeout = 10 * time.Second

// Claude's initialize response is the same catalog exposed by the Agent SDK's
// supportedModels(). Keep value (the runtime identifier) separate from displayName
// (the native picker label); aliases must not be replaced by resolvedModel.
type claudeModel struct {
	Value                 string   `json:"value"`
	DisplayName           string   `json:"displayName"`
	SupportsEffort        bool     `json:"supportsEffort"`
	SupportedEffortLevels []string `json:"supportedEffortLevels"`
}

// Models probes only the control protocol, without a user message or inference.
// Like Codex, every provider listing asks the configured executable anew.
func (p *Provider) Models(ctx context.Context, raw map[string]any) ([]execution.ModelOption, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	workingDir := p.workingDir
	if workingDir == nil {
		workingDir = os.Getwd
	}
	dir, err := workingDir()
	if err != nil {
		return nil, fmt.Errorf("resolving the working directory to list models from the claude command %q: %w", cfg.Command, err)
	}
	return p.listModels(ctx, cfg, dir)
}

func (p *Provider) listModels(ctx context.Context, cfg settings, dir string) ([]execution.ModelOption, error) {
	listCtx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	starter := p.starter
	if starter == nil {
		starter = localrun.ExecStarter{}
	}
	// A stale configured model/effort must not prevent discovering its replacement.
	cfg.Model, cfg.Effort = "", ""
	process, err := starter.Start(listCtx, dir, cfg.Command, buildArgs(cfg))
	if err != nil {
		return nil, fmt.Errorf("the claude command %q could not be started to list models: %w", cfg.Command, err)
	}
	defer func() { _, _, _ = p.shutdown(process) }()
	client := newStreamSession(process, localrun.NewSession("claude-model-catalog", nil), true)
	go client.consume()
	return client.modelCatalog(listCtx)
}

// modelCatalog is shared by short-lived probes and connected native sessions.
// Initialization happens once per process, never opens a turn, and its catalog
// is detached so callers cannot mutate the session's declaration.
func (s *streamSession) modelCatalog(ctx context.Context) ([]execution.ModelOption, error) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.catalog != nil {
		return execution.CloneModels(s.catalog), nil
	}
	ctx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	answer, err := s.requestControl(ctx, map[string]any{"subtype": "initialize"})
	if err != nil {
		return nil, fmt.Errorf("listing models from the claude runtime: %w", err)
	}
	if answer.subtype != controlSuccess {
		return nil, fmt.Errorf("the claude runtime refused catalog initialization: %s", refusalText(answer))
	}
	models, err := decodeModels(answer.response)
	if err != nil {
		return nil, err
	}
	s.catalog = models
	return execution.CloneModels(models), nil
}

func decodeModels(raw json.RawMessage) ([]execution.ModelOption, error) {
	var response struct {
		Models []claudeModel `json:"models"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decoding the model catalog returned by the claude runtime: %w", err)
	}
	if len(response.Models) == 0 {
		return nil, fmt.Errorf("the claude runtime returned no supported models; update Claude Code to a version that reports its model catalog")
	}
	models := make([]execution.ModelOption, 0, len(response.Models))
	seen := map[string]bool{}
	for _, entry := range response.Models {
		id := strings.TrimSpace(entry.Value)
		if id == "" {
			return nil, fmt.Errorf("the claude runtime listed a model without an identifier")
		}
		if seen[id] {
			return nil, fmt.Errorf("the claude runtime listed model %q more than once", id)
		}
		seen[id] = true
		label := strings.TrimSpace(entry.DisplayName)
		if label == "" {
			label = id
		}
		model := execution.ModelOption{ID: id, Label: label, Default: id == "default"}
		if entry.SupportsEffort && len(entry.SupportedEffortLevels) > 0 {
			choices := make([]execution.ModelOptionChoice, 0, len(entry.SupportedEffortLevels))
			levels := map[string]bool{}
			for _, value := range entry.SupportedEffortLevels {
				level := strings.TrimSpace(value)
				if level == "" {
					return nil, fmt.Errorf("the claude runtime listed an empty effort level for model %q", id)
				}
				if levels[level] {
					return nil, fmt.Errorf("the claude runtime listed effort level %q more than once for model %q", level, id)
				}
				levels[level] = true
				// The protocol does not report a default effort. Leaving it unset lets
				// Claude apply the user's settings instead of presenting a guessed default.
				choices = append(choices, execution.ModelOptionChoice{Value: level, Label: level})
			}
			model.Options = []execution.ModelOptionField{{Name: "effort", Label: "Effort", Help: "How much reasoning Claude spends on the run. Left empty, no effort flag is passed and Claude applies its own level.", Choices: choices}}
		}
		models = append(models, model)
	}
	return models, nil
}

var _ execution.ModelLister = (*Provider)(nil)
