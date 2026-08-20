package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/inmemory"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/arcipelago"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/claude"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/codex"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// newExecutionServer builds a viewer over a real project root and the real
// arcipelago provider: the validation, the persistence and the process rules
// exercised here are the production ones, so nothing hides the acceptance
// oracle behind a double.
func newExecutionServer(t *testing.T) (*Server, *inmemory.Connector, config.Config) {
	t.Helper()
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	conn := inmemory.New(cfg)
	registry := execution.NewRegistry()
	if err := registry.Register(arcipelago.New(arcipelago.Options{})); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv, conn, cfg
}

func seedExecutionSpecs(t *testing.T, c *inmemory.Connector) {
	t.Helper()
	specs := []domain.Spec{
		{Code: "US-901", Title: "Da pianificare", Epic: domain.Epic{Code: "EP-009", Title: "E"}, Priority: domain.PriorityHigh, Points: 3, Status: domain.StatusTodo},
		{Code: "US-902", Title: "Da implementare", Epic: domain.Epic{Code: "EP-009", Title: "E"}, Priority: domain.PriorityMedium, Points: 2, Status: domain.StatusPlanned},
	}
	if _, err := c.SaveInitialBacklog(context.Background(), specs); err != nil {
		t.Fatal(err)
	}
}

func doJSON(t *testing.T, srv *Server, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Content-Type", "application/json")
	srv.mux.ServeHTTP(w, r)
	return w
}

func validProviderConfig() map[string]any {
	return map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1"}
}

func configFileContent(t *testing.T, root string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, config.RelativePath))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// AC-1
func TestListExecutionProvidersExposesNoSecret(t *testing.T) {
	const sentinel = "super-secret-token-value"
	t.Setenv("ARCIPELAGO_TOKEN", sentinel)
	srv, _, _ := newExecutionServer(t)

	w := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	if strings.Contains(raw, sentinel) {
		t.Fatalf("the provider list leaked a credential:\n%s", raw)
	}
	var view executionProvidersView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Providers) != 1 || view.Providers[0].ID != arcipelago.ProviderID {
		t.Fatalf("unexpected provider list: %#v", view.Providers)
	}
	provider := view.Providers[0]
	if strings.TrimSpace(provider.Label) == "" {
		t.Fatal("provider has no label to show")
	}
	if len(provider.Capabilities) == 0 {
		t.Fatal("provider declares no capability")
	}
	wantFields := map[string]bool{"base_url": true, "workspace_id": true, "token_env": true, "poll_interval_seconds": true, "timeout_seconds": true}
	if len(provider.ConfigFields) != len(wantFields) {
		t.Fatalf("expected %d config fields, got %#v", len(wantFields), provider.ConfigFields)
	}
	for _, field := range provider.ConfigFields {
		if !wantFields[field.Name] {
			t.Fatalf("unexpected config field %q", field.Name)
		}
	}
	if view.Default != nil {
		t.Fatalf("a workspace without a configured provider must report no default, got %#v", view.Default)
	}
}

// AC-2
func TestSaveDefaultExecutionProviderPersistsAcrossReload(t *testing.T) {
	srv, conn, cfg := newExecutionServer(t)

	w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
		"id":     arcipelago.ProviderID,
		"config": validProviderConfig(),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var saved executionProviderSelectionView
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID != arcipelago.ProviderID || saved.Config["base_url"] != "https://hub.test" {
		t.Fatalf("unexpected saved selection: %#v", saved)
	}
	if content := configFileContent(t, cfg.ProjectRoot); !strings.Contains(content, "default_provider") || !strings.Contains(content, arcipelago.ProviderID) {
		t.Fatalf("the selection did not reach the config file:\n%s", content)
	}

	// A brand-new server on the same project root is what "after a reload"
	// means for the viewer: the default must come back from disk, not from the
	// in-memory state of the server that saved it.
	registry := execution.NewRegistry()
	if err := registry.Register(arcipelago.New(arcipelago.Options{})); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	w = doJSON(t, reloaded, http.MethodGet, "/api/execution/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var view executionProvidersView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Default == nil || view.Default.ID != arcipelago.ProviderID {
		t.Fatalf("the default did not survive the reload: %#v", view.Default)
	}
	if view.Default.Config["workspace_id"] != "ws-1" {
		t.Fatalf("the saved configuration did not survive the reload: %#v", view.Default.Config)
	}
}

// AC-3
func TestSaveDefaultExecutionProviderRejectionKeepsPreviousSelection(t *testing.T) {
	srv, _, cfg := newExecutionServer(t)

	if w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
		"id":     arcipelago.ProviderID,
		"config": validProviderConfig(),
	}); w.Code != http.StatusOK {
		t.Fatalf("seeding a valid default failed: %d %s", w.Code, w.Body.String())
	}
	before := configFileContent(t, cfg.ProjectRoot)

	cases := []struct {
		name      string
		config    map[string]any
		wantField string
	}{
		{"url non assoluto", map[string]any{"base_url": "non-un-url", "workspace_id": "ws-1"}, "base_url"},
		{"chiave sconosciuta", map[string]any{"base_url": "https://hub.test", "workspace_id": "ws-1", "token": "segreto"}, "token"},
		{"campo obbligatorio mancante", map[string]any{"base_url": "https://hub.test"}, "workspace_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
				"id":     arcipelago.ProviderID,
				"config": tc.config,
			})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["field"] != tc.wantField {
				t.Fatalf("expected field %q, got %#v", tc.wantField, payload["field"])
			}
			if message, _ := payload["error"].(string); !strings.Contains(message, tc.wantField) {
				t.Fatalf("the message does not name the offending field: %q", message)
			}
			if after := configFileContent(t, cfg.ProjectRoot); after != before {
				t.Fatalf("a rejected configuration rewrote the config file:\n%s", after)
			}
			view := readProviders(t, srv)
			if view.Default == nil || view.Default.ID != arcipelago.ProviderID || view.Default.Config["base_url"] != "https://hub.test" {
				t.Fatalf("the previously valid default was lost: %#v", view.Default)
			}
		})
	}

	t.Run("provider sconosciuto", func(t *testing.T) {
		w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
			"id":     "inesistente",
			"config": validProviderConfig(),
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
		}
		if after := configFileContent(t, cfg.ProjectRoot); after != before {
			t.Fatal("an unknown provider rewrote the config file")
		}
	})
}

func readProviders(t *testing.T, srv *Server) executionProvidersView {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var view executionProvidersView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func readSpecDetail(t *testing.T, srv *Server, code string) (specDetailView, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/spec/"+code, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var detail specDetailView
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return detail, w.Body.String()
}

// AC-4
func TestSpecDetailExposesTemplateActionsForCurrentStatus(t *testing.T) {
	srv, conn, _ := newExecutionServer(t)
	seedExecutionSpecs(t, conn)

	detail, _ := readSpecDetail(t, srv, "US-901")
	want := template.Default().ActionsFor(domain.StatusTodo)
	if len(detail.Actions) != len(want) {
		t.Fatalf("expected %d actions, got %#v", len(want), detail.Actions)
	}
	for i, action := range detail.Actions {
		if action.ID != want[i].ID || action.Label != want[i].Label || action.Skill != want[i].Skill {
			t.Fatalf("action %d diverges from the Template: got %#v, want %#v", i, action, want[i])
		}
	}
	if detail.Actions[0].ID != "plan" || detail.Actions[0].Label != "Pianifica" {
		t.Fatalf("a TODO spec must offer the plan action: %#v", detail.Actions)
	}
	if detail.Template.ID != template.Default().ID || detail.Template.Version != template.Default().Version {
		t.Fatalf("unexpected template identity: %#v", detail.Template)
	}

	planned, _ := readSpecDetail(t, srv, "US-902")
	if len(planned.Actions) != 1 || planned.Actions[0].ID != "implement" {
		t.Fatalf("a PLANNED spec must offer the implement action: %#v", planned.Actions)
	}
}

// AC-5
func TestSpecDetailRecomputesActionsAfterStatusChange(t *testing.T) {
	srv, conn, _ := newExecutionServer(t)
	seedExecutionSpecs(t, conn)

	if w := doJSON(t, srv, http.MethodPost, "/api/board/move", map[string]any{"code": "US-901", "to": "done"}); w.Code != http.StatusOK {
		t.Fatalf("move failed: %d %s", w.Code, w.Body.String())
	}
	detail, raw := readSpecDetail(t, srv, "US-901")
	if detail.Spec.Status != domain.StatusDone {
		t.Fatalf("the spec did not change status: %q", detail.Spec.Status)
	}
	if len(detail.Actions) != 0 {
		t.Fatalf("a DONE spec admits no action, got %#v", detail.Actions)
	}
	if !strings.Contains(raw, `"actions":[]`) {
		t.Fatalf("an empty action list must serialize as [], not null:\n%s", raw)
	}
}

// probeProvider is a provider that reports its own availability. It is a double
// on purpose: what is under test here is how the viewer treats *any* provider
// that declares a runtime, not the diagnostics of one particular provider.
//
// It records every configuration the probe receives, which is how the test can
// tell "the persisted configuration was handed to the default provider" from
// "it was handed to every provider".
type probeProvider struct {
	*runTestProvider
	reason string

	mu     sync.Mutex
	probes []map[string]any
}

func newProbeProvider(id, reason string) *probeProvider {
	return &probeProvider{runTestProvider: releasedProvider(id, nil), reason: reason}
}

func (p *probeProvider) Available(_ context.Context, providerConfig map[string]any) error {
	p.mu.Lock()
	p.probes = append(p.probes, providerConfig)
	p.mu.Unlock()
	if p.reason == "" {
		return nil
	}
	return errors.New(p.reason)
}

// probedConfigs returns the configurations the probe was called with, in order.
func (p *probeProvider) probedConfigs() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]map[string]any, len(p.probes))
	copy(out, p.probes)
	return out
}

// newProviderListServer builds a viewer over the given providers, optionally
// with a persisted default, so the provider list can be observed with runtimes
// the test controls.
func newProviderListServer(t *testing.T, defaultSelection *config.DefaultProviderConfig, providers ...execution.Provider) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	conn := inmemory.New(cfg)
	registry := execution.NewRegistry()
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			t.Fatal(err)
		}
	}
	if defaultSelection != nil {
		if _, err := config.UpdateDefaultProvider(cfg.ProjectRoot, *defaultSelection); err != nil {
			t.Fatal(err)
		}
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func providerViewByID(t *testing.T, view executionProvidersView, id string) executionProviderView {
	t.Helper()
	for _, provider := range view.Providers {
		if provider.ID == id {
			return provider
		}
	}
	t.Fatalf("provider %q is missing from the list: %#v", id, view.Providers)
	return executionProviderView{}
}

// AC-1: a provider that does not report availability has nothing that can be
// missing, so the viewer offers it without a reason.
func TestListExecutionProvidersMarksASilentProviderAvailable(t *testing.T) {
	srv := newProviderListServer(t, nil, releasedProvider("silent", nil))

	provider := providerViewByID(t, readProviders(t, srv), "silent")
	if !provider.Available || provider.UnavailableReason != "" {
		t.Fatalf("a provider that declares no availability must stay available: %#v", provider)
	}
	// The reason is omitted from the wire too, so no client can render an empty
	// explanation next to a usable provider.
	raw := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil).Body.String()
	if strings.Contains(raw, "unavailable_reason") {
		t.Fatalf("an available provider carries an unavailable_reason:\n%s", raw)
	}
}

// AC-1: an unusable runtime is reported as a fact of the list, with the
// provider's own words, and never as an HTTP failure.
func TestListExecutionProvidersReportsAnUnavailableProviderWithItsReason(t *testing.T) {
	const reason = "codex is not installed: install it and run `codex login`"
	srv := newProviderListServer(t, nil, newProbeProvider("broken", reason), newProbeProvider("ready", ""))

	view := readProviders(t, srv)
	if len(view.Providers) != 2 {
		t.Fatalf("unexpected provider list: %#v", view.Providers)
	}
	broken := providerViewByID(t, view, "broken")
	if broken.Available {
		t.Fatalf("an unusable provider is offered as available: %#v", broken)
	}
	if broken.UnavailableReason != reason {
		t.Fatalf("the provider's own diagnostic was rewritten: %q", broken.UnavailableReason)
	}
	// The unusable provider is still listed with everything the panel needs to
	// render it, so the user sees a disabled card instead of a missing one.
	if len(broken.Capabilities) == 0 || strings.TrimSpace(broken.Label) == "" {
		t.Fatalf("an unavailable provider lost its descriptive fields: %#v", broken)
	}
	if ready := providerViewByID(t, view, "ready"); !ready.Available || ready.UnavailableReason != "" {
		t.Fatalf("a usable provider was dragged down by another one: %#v", ready)
	}
}

// AC-1: the persisted configuration belongs to the provider it was saved for,
// so only that provider is probed with it.
func TestListExecutionProvidersProbesOnlyTheDefaultWithThePersistedConfig(t *testing.T) {
	chosen := newProbeProvider("chosen", "")
	other := newProbeProvider("other", "")
	saved := map[string]any{"model": "gpt-test", "profile": "workspace"}
	srv := newProviderListServer(t, &config.DefaultProviderConfig{ID: "chosen", Config: saved}, chosen, other)

	view := readProviders(t, srv)
	if view.Default == nil || view.Default.ID != "chosen" {
		t.Fatalf("the persisted default is not reported: %#v", view.Default)
	}

	chosenProbes := chosen.probedConfigs()
	if len(chosenProbes) != 1 {
		t.Fatalf("the default provider was probed %d times", len(chosenProbes))
	}
	if chosenProbes[0]["model"] != "gpt-test" || chosenProbes[0]["profile"] != "workspace" {
		t.Fatalf("the default provider was not probed with the persisted configuration: %#v", chosenProbes[0])
	}
	otherProbes := other.probedConfigs()
	if len(otherProbes) != 1 {
		t.Fatalf("a non-default provider was probed %d times", len(otherProbes))
	}
	if len(otherProbes[0]) != 0 {
		t.Fatalf("a configuration saved for another provider reached %q: %#v", other.ID(), otherProbes[0])
	}
}

// AC-1 — the dialogue capability is declared by a provider that really exposes
// an interactive run, and shows as absent on one that does not. Both providers
// here are real implementations of the two shapes, so what is asserted is the
// discovery rule and not a hand-written list.
func TestListExecutionProvidersDeclaresTheDialogueCapability(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := inmemory.New(cfg)
	registry := execution.NewRegistry()
	collaborating := &collaboratingProvider{
		runTestProvider:  releasedProvider("collaborating", nil),
		fakeCollaborator: newFakeCollaborator(),
	}
	if err := registry.Register(collaborating); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(releasedProvider("plain", nil)); err != nil {
		t.Fatal(err)
	}
	// The two shipped local providers are registered as themselves, so the
	// table below is a statement about `codex` and `claude` and not only about
	// a test double that happens to implement the same interface.
	probe := &unusableRuntime{}
	if err := registry.Register(codex.New(codex.Options{Runner: probe})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(claude.New(claude.Options{Runner: probe})); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	w := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	var view executionProvidersView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	found := map[string][]execution.Capability{}
	for _, provider := range view.Providers {
		found[provider.ID] = provider.Capabilities
	}
	// Every provider that exposes an interactive run declares the dialogue, and
	// the two local ones are named explicitly: a viewer decides whether to offer
	// the conversation from this list, so a local provider missing from it is a
	// run nobody can follow.
	for _, id := range []string{"collaborating", codex.ProviderID, claude.ProviderID} {
		if !containsCapability(found[id], execution.CapabilityRunDialog) {
			t.Fatalf("a provider that exposes an interactive run must declare %s, got %v for %s", execution.CapabilityRunDialog, found[id], id)
		}
		if !containsCapability(found[id], execution.CapabilitySpecPlan) {
			t.Fatalf("the dialogue capability must join the ones already declared, got %v for %s", found[id], id)
		}
	}
	if containsCapability(found["plain"], execution.CapabilityRunDialog) {
		t.Fatalf("a provider without an interactive run must expose %s as absent, got %v", execution.CapabilityRunDialog, found["plain"])
	}
	if !containsCapability(found["plain"], execution.CapabilitySpecPlan) {
		t.Fatalf("the plain provider lost its own capabilities: %v", found["plain"])
	}
}

// unusableRuntime answers the availability probe of a local provider without a
// process. Listing the providers probes every one of them, and this test is
// about what they declare, not about what is installed on the machine that runs
// it.
type unusableRuntime struct{}

func (unusableRuntime) Run(context.Context, string, string, []string) (string, string, int, error) {
	return "", "not installed", 127, nil
}

func containsCapability(capabilities []execution.Capability, want execution.Capability) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

// catalogProvider is a probeProvider that also declares a model catalog. Both
// halves are configurable by the test, because what is under test here is how
// the viewer serves *any* catalog-declaring provider — its shape on the wire,
// not the models of one particular runtime.
//
// It counts the calls to Models, which is how a test can tell "the catalog was
// not served" from "the catalog was never asked for".
type catalogProvider struct {
	*probeProvider
	models []execution.ModelOption
	err    error

	modelMu    sync.Mutex
	modelCalls int
	modelConfs []map[string]any
}

func newCatalogProvider(id, reason string, models []execution.ModelOption, err error) *catalogProvider {
	return &catalogProvider{probeProvider: newProbeProvider(id, reason), models: models, err: err}
}

func (p *catalogProvider) Models(_ context.Context, providerConfig map[string]any) ([]execution.ModelOption, error) {
	p.modelMu.Lock()
	p.modelCalls++
	p.modelConfs = append(p.modelConfs, providerConfig)
	p.modelMu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	return p.models, nil
}

// modelCallCount reports how many times the catalog was asked for.
func (p *catalogProvider) modelCallCount() int {
	p.modelMu.Lock()
	defer p.modelMu.Unlock()
	return p.modelCalls
}

// AC-1, AC-2: a provider that declares a catalog offers it to the panel with
// the identifiers it declared, in the order it declared them, and with the one
// model it uses by default marked as such.
func TestListExecutionProvidersOffersTheDeclaredModelCatalog(t *testing.T) {
	declared := []execution.ModelOption{
		{ID: "opus-test", Label: "Opus (test)"},
		{ID: "sonnet-test", Default: true},
		{ID: "haiku-test"},
	}
	srv := newProviderListServer(t, nil, newCatalogProvider("cataloged", "", declared, nil))

	provider := providerViewByID(t, readProviders(t, srv), "cataloged")
	if provider.ModelField != execution.ModelFieldName {
		t.Fatalf("the catalog must name the configuration field it fills in: %q", provider.ModelField)
	}
	if len(provider.Models) != len(declared) {
		t.Fatalf("the catalog did not reach the client whole: %#v", provider.Models)
	}
	defaults := 0
	for i, model := range provider.Models {
		if model.ID != declared[i].ID {
			t.Fatalf("the declared order was not preserved: got %#v, want %#v", provider.Models, declared)
		}
		if model.Default {
			defaults++
		}
	}
	// AC-2: the flag survives serialization and stays on exactly one entry, so
	// the panel can point at one predefined model and never at two or none.
	if defaults != 1 {
		t.Fatalf("exactly one model must be marked as the provider default, got %d: %#v", defaults, provider.Models)
	}
	if provider.Models[1].ID != "sonnet-test" || !provider.Models[1].Default {
		t.Fatalf("the default flag landed on the wrong model: %#v", provider.Models)
	}
	if provider.ModelsUnavailableReason != "" {
		t.Fatalf("a catalog that was obtained carries a reason: %q", provider.ModelsUnavailableReason)
	}
	// The reason is omitted from the wire too, so no client can render an empty
	// explanation next to a usable catalog.
	raw := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil).Body.String()
	if strings.Contains(raw, "models_unavailable_reason") {
		t.Fatalf("a provider with a catalog carries a models_unavailable_reason:\n%s", raw)
	}
}

// AC-4: a runtime that cannot answer the availability probe explains, in its
// own words, why the list is not there — and is not asked for a catalog it
// cannot produce, so opening the panel never spawns a second probe.
func TestListExecutionProvidersExplainsAMissingCatalogWithTheProbeReason(t *testing.T) {
	const reason = "claude is not installed: install it and run `claude login`"
	provider := newCatalogProvider("unreachable", reason, []execution.ModelOption{{ID: "never-listed"}}, nil)
	srv := newProviderListServer(t, nil, provider)

	w := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("an unobtainable catalog is a fact of the list, not an HTTP failure: %d %s", w.Code, w.Body.String())
	}
	var view executionProvidersView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	got := providerViewByID(t, view, "unreachable")
	if len(got.Models) != 0 {
		t.Fatalf("a provider that cannot be reached served a catalog: %#v", got.Models)
	}
	if got.ModelsUnavailableReason != reason {
		t.Fatalf("the provider's own diagnostic was rewritten: %q", got.ModelsUnavailableReason)
	}
	if got.ModelField != execution.ModelFieldName {
		t.Fatalf("the field the catalog fills in must stay known even without the catalog: %q", got.ModelField)
	}
	if calls := provider.modelCallCount(); calls != 0 {
		t.Fatalf("an unreachable runtime was asked for its catalog %d times", calls)
	}
}

// AC-4, second branch: the runtime is there but listing fails. The failure is
// the reason the reader sees, textually and unrewritten.
func TestListExecutionProvidersTurnsAFailedCatalogIntoItsReason(t *testing.T) {
	const failure = "claude models: unexpected output from the runtime"
	srv := newProviderListServer(t, nil, newCatalogProvider("failing", "", nil, errors.New(failure)))

	w := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("a failed listing is a fact of the list, not an HTTP failure: %d %s", w.Code, w.Body.String())
	}
	var view executionProvidersView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	got := providerViewByID(t, view, "failing")
	if got.Available != true {
		t.Fatalf("a reachable provider was marked unavailable because its catalog failed: %#v", got)
	}
	if len(got.Models) != 0 {
		t.Fatalf("a failed listing served a catalog: %#v", got.Models)
	}
	if got.ModelsUnavailableReason != failure {
		t.Fatalf("the listing error was rewritten: %q", got.ModelsUnavailableReason)
	}
}

// A provider that declares no catalog must not grow any of the new fields:
// their absence on the wire is what tells the browser to keep the plain text
// input it has always rendered.
func TestListExecutionProvidersOmitsTheModelFieldsForAProviderWithoutACatalog(t *testing.T) {
	srv := newProviderListServer(t, nil, releasedProvider("silent", nil))

	raw := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil).Body.String()
	for _, key := range []string{"model_field", `"models"`, "models_unavailable_reason"} {
		if strings.Contains(raw, key) {
			t.Fatalf("a provider without a catalog carries %s:\n%s", key, raw)
		}
	}
}

// The invariant of the view: a provider that declares a catalog always carries
// either models or a reason. An empty list with nothing to read would leave the
// reader with a mute field and no way to interpret it.
func TestListExecutionProvidersNeverServesAMuteEmptyCatalog(t *testing.T) {
	srv := newProviderListServer(t, nil, newCatalogProvider("empty", "", nil, nil))

	got := providerViewByID(t, readProviders(t, srv), "empty")
	if len(got.Models) != 0 {
		t.Fatalf("the provider declared no model: %#v", got.Models)
	}
	if strings.TrimSpace(got.ModelsUnavailableReason) == "" {
		t.Fatalf("an empty catalog reached the reader with no explanation: %#v", got)
	}
}

// AC-5: a model that is not in the current catalog is saved and read back
// unchanged. The catalog is a suggestion, never a filter that silently drops
// what the workspace already chose.
func TestSaveDefaultExecutionProviderKeepsAModelOutsideTheCatalog(t *testing.T) {
	const chosen = "modello-fuori-catalogo"
	srv := newProviderListServer(t, nil, newCatalogProvider("cataloged", "", []execution.ModelOption{
		{ID: "opus-test", Default: true},
		{ID: "sonnet-test"},
	}, nil))

	w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
		"id":     "cataloged",
		"config": map[string]any{execution.ModelFieldName: chosen},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("saving a model outside the catalog was rejected: %d %s", w.Code, w.Body.String())
	}

	view := readProviders(t, srv)
	if view.Default == nil || view.Default.ID != "cataloged" {
		t.Fatalf("the saved default is not reported: %#v", view.Default)
	}
	if view.Default.Config[execution.ModelFieldName] != chosen {
		t.Fatalf("the chosen model did not survive the round trip: %#v", view.Default.Config)
	}
	// It really is outside the catalog, so the assertion above is about a model
	// the list does not contain and not about one it happens to hold.
	provider := providerViewByID(t, view, "cataloged")
	for _, model := range provider.Models {
		if model.ID == chosen {
			t.Fatalf("the model under test is in the catalog, so the test proves nothing: %#v", provider.Models)
		}
	}
	// And it is on disk: what the panel reads back comes from the persisted
	// workspace configuration, not from a value held in memory.
	raw, err := os.ReadFile(filepath.Join(srv.session().cfg.ProjectRoot, ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatalf("the workspace configuration was not written: %v", err)
	}
	if !strings.Contains(string(raw), chosen) {
		t.Fatalf("the chosen model is missing from the persisted configuration:\n%s", raw)
	}
}

// AC-6: leaving the model empty stays possible, and the saved configuration
// carries no model key at all — an empty string would be a model identifier the
// runtime would then be asked to honour.
func TestSaveDefaultExecutionProviderWritesNoModelKeyWhenTheFieldIsLeftEmpty(t *testing.T) {
	srv := newProviderListServer(t, nil, newCatalogProvider("cataloged", "", []execution.ModelOption{
		{ID: "opus-test", Default: true},
	}, nil))

	w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
		"id":     "cataloged",
		"config": map[string]any{"profile": "workspace"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("saving without a model was rejected: %d %s", w.Code, w.Body.String())
	}

	view := readProviders(t, srv)
	if view.Default == nil {
		t.Fatalf("the saved default is not reported: %#v", view)
	}
	if _, present := view.Default.Config[execution.ModelFieldName]; present {
		t.Fatalf("an empty model field produced a model key: %#v", view.Default.Config)
	}
	raw, err := os.ReadFile(filepath.Join(srv.session().cfg.ProjectRoot, ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatalf("the workspace configuration was not written: %v", err)
	}
	if strings.Contains(string(raw), execution.ModelFieldName+":") {
		t.Fatalf("an empty model field wrote a model key to the configuration:\n%s", raw)
	}
}

// --- US-048: model options -------------------------------------------------

// usableRuntime answers the availability probe of a local provider without
// starting a process, so the tests below are about what the provider declares
// and persists and never about what is installed on the machine that runs them.
type usableRuntime struct{}

func (usableRuntime) Run(context.Context, string, string, []string) (string, string, int, error) {
	return "2.1.236", "", 0, nil
}

// fakeExecutable writes an executable file and returns its absolute path. The
// availability probe of a local provider looks its command up on the
// filesystem, so a real path is what lets the probe succeed deterministically.
func fakeExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// newLocalProviderServer serves the two shipped local providers as themselves,
// with the given persisted configuration as the workspace default. Only the
// operating-system seam is doubled.
func newLocalProviderServer(t *testing.T, id string, providerConfig map[string]any) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	conn := inmemory.New(cfg)
	registry := execution.NewRegistry()
	probe := usableRuntime{}
	if err := registry.Register(claude.New(claude.Options{Runner: probe})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(codex.New(codex.Options{Runner: probe})); err != nil {
		t.Fatal(err)
	}
	if _, err := config.UpdateDefaultProvider(cfg.ProjectRoot, config.DefaultProviderConfig{ID: id, Config: providerConfig}); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

// claudeConfig is a persisted claude configuration whose command really exists,
// so the availability probe passes and the catalog is served.
func claudeConfig(t *testing.T, extra map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{"command": fakeExecutable(t, "claude")}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func codexConfig(t *testing.T, extra map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{"command": fakeExecutable(t, "codex")}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func modelViewByID(t *testing.T, provider executionProviderView, id string) execution.ModelOption {
	t.Helper()
	for _, model := range provider.Models {
		if model.ID == id {
			return model
		}
	}
	t.Fatalf("the catalog of %q does not offer the model %q: %#v", provider.ID, id, provider.Models)
	return execution.ModelOption{}
}

// AC-1: the catalog the browser receives carries, inside each model, exactly
// the options that model declares — and the model that declares none carries
// none.
func TestListExecutionProvidersCarriesTheOptionsDeclaredForEachModel(t *testing.T) {
	srv := newLocalProviderServer(t, claude.ProviderID, claudeConfig(t, map[string]any{"model": "sonnet"}))

	provider := providerViewByID(t, readProviders(t, srv), claude.ProviderID)
	if provider.ModelsUnavailableReason != "" {
		t.Fatalf("the catalog was not obtainable: %q", provider.ModelsUnavailableReason)
	}
	sonnet := modelViewByID(t, provider, "sonnet")
	if len(sonnet.Options) != 1 {
		t.Fatalf("sonnet carries %d options, want 1: %#v", len(sonnet.Options), sonnet.Options)
	}
	option := sonnet.Options[0]
	if option.Name != "effort" {
		t.Fatalf("sonnet carries the option %q, want %q", option.Name, "effort")
	}
	if strings.TrimSpace(option.Label) == "" {
		t.Fatal("the option reached the browser with no label to read")
	}
	if len(option.Choices) == 0 {
		t.Fatalf("the option reached the browser with no choice to pick: %#v", option)
	}
	defaults := 0
	for _, choice := range option.Choices {
		if strings.TrimSpace(choice.Value) == "" {
			t.Fatalf("a choice reached the browser with no value: %#v", option.Choices)
		}
		if choice.Default {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("%d choices are marked as the provider default, want exactly 1: %#v", defaults, option.Choices)
	}

	// AC-5: the model that declares no option carries none, so the panel can
	// tell the two situations apart instead of drawing an empty section.
	haiku := modelViewByID(t, provider, "haiku")
	if len(haiku.Options) != 0 {
		t.Fatalf("haiku carries options it does not declare: %#v", haiku.Options)
	}
	raw := doJSON(t, srv, http.MethodGet, "/api/execution/providers", nil).Body.String()
	if strings.Contains(raw, `"haiku","options"`) {
		t.Fatalf("the model without options carries an empty options key:\n%s", raw)
	}
}

// An option of a model is not a setting of the provider: declaring it in both
// places would draw it twice in the form, once under the model and once among
// the configuration fields.
func TestListExecutionProvidersKeepsModelOptionsOutOfTheConfigFields(t *testing.T) {
	for _, tc := range []struct {
		id     string
		config map[string]any
		option string
	}{
		{claude.ProviderID, claudeConfig(t, nil), "effort"},
		{codex.ProviderID, codexConfig(t, nil), "reasoning_effort"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			srv := newLocalProviderServer(t, tc.id, tc.config)
			provider := providerViewByID(t, readProviders(t, srv), tc.id)
			for _, field := range provider.ConfigFields {
				if field.Name == tc.option {
					t.Fatalf("the model option %q is also declared as a configuration field: %#v", tc.option, provider.ConfigFields)
				}
			}
			declared := false
			for _, model := range provider.Models {
				for _, option := range model.Options {
					if option.Name == tc.option {
						declared = true
					}
				}
			}
			if !declared {
				t.Fatalf("no model of %q declares the option %q, so the test proves nothing: %#v", tc.id, tc.option, provider.Models)
			}
		})
	}
}

// The invariant US-047 established still holds: when the catalog cannot be
// obtained, neither models nor options reach the browser, and the reason does.
func TestListExecutionProvidersServesNoOptionWithoutACatalog(t *testing.T) {
	cfg := config.Default()
	cfg.ProjectRoot = t.TempDir()
	conn := inmemory.New(cfg)
	registry := execution.NewRegistry()
	if err := registry.Register(claude.New(claude.Options{Runner: unusableRuntime{}})); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	provider := providerViewByID(t, readProviders(t, srv), claude.ProviderID)
	if len(provider.Models) != 0 {
		t.Fatalf("an unobtainable catalog still served models: %#v", provider.Models)
	}
	if strings.TrimSpace(provider.ModelsUnavailableReason) == "" {
		t.Fatalf("an unobtainable catalog reached the reader with no explanation: %#v", provider)
	}
	if provider.ModelField != execution.ModelFieldName {
		t.Fatalf("the field the catalog fills in must stay known even without the catalog: %q", provider.ModelField)
	}
}

// AC-2, AC-3: an option that was saved comes back unchanged, and the option of
// a model that is no longer selected is gone from the persisted configuration —
// the oracle is the configuration read back from disk, not the response of the
// save.
func TestSaveDefaultExecutionProviderPersistsAndReplacesAModelOption(t *testing.T) {
	command := fakeExecutable(t, "claude")
	srv := newLocalProviderServer(t, claude.ProviderID, map[string]any{"command": command, "model": "sonnet"})

	if w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
		"id":     claude.ProviderID,
		"config": map[string]any{"command": command, "model": "sonnet", "effort": "high"},
	}); w.Code != http.StatusOK {
		t.Fatalf("saving a model option was rejected: %d %s", w.Code, w.Body.String())
	}

	view := readProviders(t, srv)
	if view.Default == nil || view.Default.Config["effort"] != "high" {
		t.Fatalf("the saved option did not survive the round trip: %#v", view.Default)
	}
	raw, err := os.ReadFile(filepath.Join(srv.session().cfg.ProjectRoot, ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatalf("the workspace configuration was not written: %v", err)
	}
	if !strings.Contains(string(raw), "effort: high") {
		t.Fatalf("the option is missing from the persisted configuration:\n%s", raw)
	}

	// AC-3: the panel now draws a model that declares no effort, so the save
	// carries no effort key — and the superseded option must not linger.
	if w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
		"id":     claude.ProviderID,
		"config": map[string]any{"command": command, "model": "haiku"},
	}); w.Code != http.StatusOK {
		t.Fatalf("saving the second model was rejected: %d %s", w.Code, w.Body.String())
	}
	view = readProviders(t, srv)
	if view.Default == nil {
		t.Fatal("the saved default is not reported")
	}
	if _, present := view.Default.Config["effort"]; present {
		t.Fatalf("the option of the previous model is still in the configuration: %#v", view.Default.Config)
	}
	if view.Default.Config["model"] != "haiku" {
		t.Fatalf("the newly chosen model was not persisted: %#v", view.Default.Config)
	}
	raw, err = os.ReadFile(filepath.Join(srv.session().cfg.ProjectRoot, ".archetipo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "effort:") {
		t.Fatalf("the superseded option is still on disk:\n%s", raw)
	}
}

// AC-4: a value the provider refuses names the option and leaves the previously
// saved configuration exactly as it was, field by field.
func TestSaveDefaultExecutionProviderRejectsAnUnknownOptionValue(t *testing.T) {
	cases := []struct {
		provider string
		option   string
		valid    string
		model    string
	}{
		{claude.ProviderID, "effort", "high", "sonnet"},
		{codex.ProviderID, "reasoning_effort", "high", "gpt-5-codex"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			command := fakeExecutable(t, tc.provider)
			srv := newLocalProviderServer(t, tc.provider, map[string]any{"command": command})
			valid := map[string]any{"command": command, "model": tc.model, tc.option: tc.valid}
			if w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
				"id": tc.provider, "config": valid,
			}); w.Code != http.StatusOK {
				t.Fatalf("seeding a valid configuration failed: %d %s", w.Code, w.Body.String())
			}
			before := configFileContent(t, srv.session().cfg.ProjectRoot)

			w := doJSON(t, srv, http.MethodPut, "/api/execution/provider/default", map[string]any{
				"id":     tc.provider,
				"config": map[string]any{"command": command, "model": tc.model, tc.option: "turbo"},
			})
			if w.Code != http.StatusBadRequest {
				t.Fatalf("a value outside the declared set was accepted: %d %s", w.Code, w.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["field"] != tc.option {
				t.Fatalf("the rejection names field %#v, want %q", payload["field"], tc.option)
			}
			if message, _ := payload["error"].(string); !strings.Contains(message, tc.option) {
				t.Fatalf("the message does not name the option: %q", message)
			}
			if after := configFileContent(t, srv.session().cfg.ProjectRoot); after != before {
				t.Fatalf("a rejected value rewrote the configuration:\n%s", after)
			}
			view := readProviders(t, srv)
			if view.Default == nil || view.Default.ID != tc.provider {
				t.Fatalf("the previously valid default was lost: %#v", view.Default)
			}
			for key, want := range valid {
				if view.Default.Config[key] != want {
					t.Fatalf("field %q is now %#v, want %#v", key, view.Default.Config[key], want)
				}
			}
		})
	}
}
