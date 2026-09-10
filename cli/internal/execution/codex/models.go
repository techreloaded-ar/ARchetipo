package codex

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

const (
	modelListPageLimit = 100
	modelListTimeout   = 10 * time.Second
)

// codexModel is the part of a model/list entry ARchetipo renders. The app
// server can add fields without affecting this decoder; the provider keeps the
// model identifier, presentation label, default marker and reasoning choices.
type codexModel struct {
	ID                        string                 `json:"id"`
	Model                     string                 `json:"model"`
	DisplayName               string                 `json:"displayName"`
	Hidden                    bool                   `json:"hidden"`
	DefaultReasoningEffort    string                 `json:"defaultReasoningEffort"`
	SupportedReasoningEfforts []codexReasoningEffort `json:"supportedReasoningEfforts"`
	IsDefault                 bool                   `json:"isDefault"`
}

type codexReasoningEffort struct {
	ReasoningEffort string `json:"reasoningEffort"`
}

type codexModelPage struct {
	Data       json.RawMessage `json:"data"`
	NextCursor *string         `json:"nextCursor"`
}

// Models asks the configured Codex app server for the picker-visible models
// available to this machine and account. The request is intentionally made on
// every read: model availability is provider state, not a list ARchetipo can
// keep correct in source code.
//
// The short-lived process performs only initialize + model/list and is then
// closed; no thread or agent turn is started. Pagination is followed until
// nextCursor is null, and each model's supported reasoning efforts are carried
// into the provider-neutral option shape already consumed by the viewer.
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
		return nil, fmt.Errorf("resolving the working directory to list models from the codex command %q: %w", cfg.Command, err)
	}
	starter := p.starter
	if starter == nil {
		starter = localrun.ExecStarter{}
	}

	listCtx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	process, err := starter.Start(listCtx, dir, cfg.Command, buildArgs())
	if err != nil {
		return nil, fmt.Errorf("the codex command %q could not be started to list models: %w", cfg.Command, err)
	}
	defer func() { _, _, _ = p.shutdown(process) }()

	// This probe opens no turn at all — it initializes, pages through
	// model/list and leaves — so it never binds the client to a session.
	client := newAppServer(process, localrun.NewSession("codex-model-catalog", nil))
	go client.consume()
	if err := client.initialize(listCtx); err != nil {
		return nil, fmt.Errorf("listing models from the codex command %q: %w", cfg.Command, err)
	}

	return listModels(listCtx, client)
}

// listModels pages through model/list on an already-initialized client and
// returns the catalog the app server declares.
//
// It is shared with the native session, which asks the very same question of
// the very same protocol: a second decoder would be a second answer, free to
// drift from this one — and it would be the poorer answer, because the paging,
// the hidden filter and the single-default check all live here.
func listModels(ctx context.Context, client *appServer) ([]execution.ModelOption, error) {
	models := make([]execution.ModelOption, 0)
	seenModels := map[string]struct{}{}
	seenCursors := map[string]struct{}{}
	cursor := ""
	for {
		params := map[string]any{
			"limit":         modelListPageLimit,
			"includeHidden": false,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := client.call(ctx, methodModelList, params)
		if err != nil {
			return nil, fmt.Errorf("the codex app server could not list models: %w", err)
		}
		page, err := decodeModelPage(result)
		if err != nil {
			return nil, err
		}
		for _, entry := range page.models {
			if entry.Hidden {
				continue
			}
			model, err := modelOption(entry)
			if err != nil {
				return nil, err
			}
			if _, duplicate := seenModels[model.ID]; duplicate {
				return nil, fmt.Errorf("the codex app server listed model %q more than once", model.ID)
			}
			seenModels[model.ID] = struct{}{}
			models = append(models, model)
		}

		next := ""
		if page.nextCursor != nil {
			next = strings.TrimSpace(*page.nextCursor)
		}
		if next == "" {
			break
		}
		if _, repeated := seenCursors[next]; repeated {
			return nil, fmt.Errorf("the codex app server repeated model-list cursor %q", next)
		}
		seenCursors[next] = struct{}{}
		cursor = next
	}

	defaults := 0
	for _, model := range models {
		if model.Default {
			defaults++
		}
	}
	if len(models) > 0 && defaults != 1 {
		return nil, fmt.Errorf("the codex app server marked %d default models, expected exactly one", defaults)
	}
	return models, nil
}

type decodedModelPage struct {
	models     []codexModel
	nextCursor *string
}

func decodeModelPage(result json.RawMessage) (decodedModelPage, error) {
	var wire codexModelPage
	if err := json.Unmarshal(result, &wire); err != nil {
		return decodedModelPage{}, fmt.Errorf("decoding the model catalog returned by the codex app server: %w", err)
	}
	if len(wire.Data) == 0 || string(wire.Data) == "null" {
		return decodedModelPage{}, fmt.Errorf("the codex app server returned a model catalog without a data list")
	}
	var models []codexModel
	if err := json.Unmarshal(wire.Data, &models); err != nil {
		return decodedModelPage{}, fmt.Errorf("decoding the model list returned by the codex app server: %w", err)
	}
	return decodedModelPage{models: models, nextCursor: wire.NextCursor}, nil
}

func modelOption(entry codexModel) (execution.ModelOption, error) {
	id := strings.TrimSpace(entry.Model)
	if id == "" {
		id = strings.TrimSpace(entry.ID)
	}
	if id == "" {
		return execution.ModelOption{}, fmt.Errorf("the codex app server listed a model without an identifier")
	}
	label := strings.TrimSpace(entry.DisplayName)
	if label == "" {
		label = id
	}
	option := execution.ModelOption{ID: id, Label: label, Default: entry.IsDefault}
	if len(entry.SupportedReasoningEfforts) == 0 {
		return option, nil
	}

	defaultEffort := strings.TrimSpace(entry.DefaultReasoningEffort)
	seen := make(map[string]struct{}, len(entry.SupportedReasoningEfforts))
	choices := make([]execution.ModelOptionChoice, 0, len(entry.SupportedReasoningEfforts))
	defaults := 0
	for _, supported := range entry.SupportedReasoningEfforts {
		effort := strings.TrimSpace(supported.ReasoningEffort)
		if effort == "" {
			return execution.ModelOption{}, fmt.Errorf("the codex app server listed an empty reasoning effort for model %q", id)
		}
		if _, duplicate := seen[effort]; duplicate {
			return execution.ModelOption{}, fmt.Errorf("the codex app server listed reasoning effort %q more than once for model %q", effort, id)
		}
		seen[effort] = struct{}{}
		isDefault := effort == defaultEffort
		if isDefault {
			defaults++
		}
		choices = append(choices, execution.ModelOptionChoice{
			Value:   effort,
			Label:   effort,
			Default: isDefault,
		})
	}
	if defaults != 1 {
		return execution.ModelOption{}, fmt.Errorf("the codex app server marked %d default reasoning efforts for model %q, expected exactly one", defaults, id)
	}
	option.Options = []execution.ModelOptionField{{
		Name:    effortField,
		Label:   "Reasoning effort",
		Help:    "How much reasoning Codex spends on the run. Left empty, no override is sent and Codex applies its own setting.",
		Choices: choices,
	}}
	return option, nil
}

var _ execution.ModelLister = (*Provider)(nil)
