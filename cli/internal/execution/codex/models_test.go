package codex

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

func providerWithCatalog(fake *fakeCodex) *Provider {
	return New(Options{
		Starter: fake,
		WorkingDir: func() (string, error) {
			return "/catalog-workspace", nil
		},
	})
}

func TestModelsLoadsTheLiveCatalogAndReasoningOptions(t *testing.T) {
	fake := newFakeCodex()
	fake.modelPages = map[string]string{"": `{
		"data": [
			{
				"id": "catalog-entry-1",
				"model": "gpt-provider-default",
				"displayName": "Provider default",
				"hidden": false,
				"defaultReasoningEffort": "xhigh",
				"supportedReasoningEfforts": [
					{"reasoningEffort": "minimal", "description": "quick"},
					{"reasoningEffort": "xhigh", "description": "deep"}
				],
				"isDefault": true
			},
			{
				"id": "id-fallback-model",
				"displayName": "ID fallback",
				"hidden": false,
				"supportedReasoningEfforts": [],
				"isDefault": false
			},
			{
				"id": "hidden-model",
				"model": "hidden-model",
				"displayName": "Hidden",
				"hidden": true,
				"isDefault": false
			}
		],
		"nextCursor": null
	}`}

	models, err := providerWithCatalog(fake).Models(context.Background(), map[string]any{"model": "not-used-for-listing"})
	if err != nil {
		t.Fatalf("listing models failed: %v", err)
	}
	want := []execution.ModelOption{
		{
			ID:      "gpt-provider-default",
			Label:   "Provider default",
			Default: true,
			Options: []execution.ModelOptionField{{
				Name:  reasoningEffortField,
				Label: "Reasoning effort",
				Help:  "How much reasoning Codex spends on the run. Left empty, no override is sent and Codex applies its own setting.",
				Choices: []execution.ModelOptionChoice{
					{Value: "minimal", Label: "minimal"},
					{Value: "xhigh", Label: "xhigh", Default: true},
				},
			}},
		},
		{ID: "id-fallback-model", Label: "ID fallback"},
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v\nwant   = %#v", models, want)
	}
	if got := fake.methodsCalled(); !reflect.DeepEqual(got, []string{methodInitialize, methodInitialized, methodModelList}) {
		t.Fatalf("protocol methods = %v, want initialize, initialized, model/list only", got)
	}
	if fake.startedIn() != "/catalog-workspace" {
		t.Fatalf("the catalog process started in %q", fake.startedIn())
	}
	params := fake.paramsOf(methodModelList)
	if params["includeHidden"] != false || params["limit"] != float64(modelListPageLimit) {
		t.Fatalf("model/list params = %#v", params)
	}
	if params["cursor"] != nil {
		t.Fatalf("the first model/list request carried a cursor: %#v", params["cursor"])
	}
}

func TestModelsFollowsEveryNextCursorInOrder(t *testing.T) {
	fake := newFakeCodex()
	fake.modelPages = map[string]string{
		"":       `{"data":[{"model":"first","displayName":"First","isDefault":true}],"nextCursor":"page-2"}`,
		"page-2": `{"data":[{"model":"second","displayName":"Second"}],"nextCursor":null}`,
	}

	models, err := providerWithCatalog(fake).Models(context.Background(), nil)
	if err != nil {
		t.Fatalf("listing paginated models failed: %v", err)
	}
	if got := []string{models[0].ID, models[1].ID}; !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("model order = %v", got)
	}
	requests := fake.allParamsOf(methodModelList)
	if len(requests) != 2 {
		t.Fatalf("model/list was called %d times, want 2", len(requests))
	}
	if requests[0]["cursor"] != nil || requests[1]["cursor"] != "page-2" {
		t.Fatalf("pagination cursors = %#v", requests)
	}
}

func TestModelsRejectsInvalidProviderCatalogs(t *testing.T) {
	cases := []struct {
		name string
		page string
		want string
	}{
		{"missing data", `{"nextCursor":null}`, "without a data list"},
		{"model without id", `{"data":[{"displayName":"Nameless","isDefault":true}],"nextCursor":null}`, "without an identifier"},
		{"duplicate models", `{"data":[{"model":"same","isDefault":true},{"model":"same"}],"nextCursor":null}`, "more than once"},
		{"no default model", `{"data":[{"model":"only"}],"nextCursor":null}`, "marked 0 default models"},
		{"two default models", `{"data":[{"model":"one","isDefault":true},{"model":"two","isDefault":true}],"nextCursor":null}`, "marked 2 default models"},
		{"effort without default", `{"data":[{"model":"only","isDefault":true,"supportedReasoningEfforts":[{"reasoningEffort":"low"}]}],"nextCursor":null}`, "0 default reasoning efforts"},
		{"duplicate effort", `{"data":[{"model":"only","isDefault":true,"defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"low"}]}],"nextCursor":null}`, "more than once"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeCodex()
			fake.modelPages = map[string]string{"": tc.page}
			models, err := providerWithCatalog(fake).Models(context.Background(), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
			if models != nil {
				t.Fatalf("failed catalog returned models: %#v", models)
			}
		})
	}
}

func TestModelsStopsOnARepeatedPaginationCursor(t *testing.T) {
	fake := newFakeCodex()
	fake.modelPages = map[string]string{
		"":      `{"data":[{"model":"first","isDefault":true}],"nextCursor":"again"}`,
		"again": `{"data":[{"model":"second"}],"nextCursor":"again"}`,
	}

	_, err := providerWithCatalog(fake).Models(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "repeated model-list cursor") {
		t.Fatalf("error = %v, want repeated-cursor diagnostic", err)
	}
}

func TestModelListingReportsConfigurationAndProtocolFailures(t *testing.T) {
	t.Run("configuration", func(t *testing.T) {
		fake := newFakeCodex()
		models, err := providerWithCatalog(fake).Models(context.Background(), map[string]any{execution.ModelFieldName: true})
		if err == nil {
			t.Fatal("a non-string model was accepted")
		}
		if models != nil || len(fake.methodsCalled()) != 0 {
			t.Fatalf("invalid configuration started the provider: models=%#v methods=%v", models, fake.methodsCalled())
		}
	})

	t.Run("model list refusal", func(t *testing.T) {
		fake := newFakeCodex()
		fake.modelErr = &rpcError{Code: -32000, Message: "catalog unavailable"}
		models, err := providerWithCatalog(fake).Models(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "catalog unavailable") {
			t.Fatalf("error = %v, want provider refusal", err)
		}
		if models != nil {
			t.Fatalf("a refused listing returned models: %#v", models)
		}
	})
}
