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
