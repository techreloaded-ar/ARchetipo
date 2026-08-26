package arcipelago

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// probeHub answers the two routes the probe reads, and counts what it was
// asked, so a test can assert that a check which should cost nothing did.
type probeHub struct {
	mu         sync.Mutex
	requests   []string
	me         hubResponse
	workspaces hubResponse
}

func (h *probeHub) served() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.requests...)
}

func startProbeHub(t *testing.T, hub *probeHub) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		hub.requests = append(hub.requests, r.URL.Path)
		hub.mu.Unlock()
		answer := hub.me
		if r.URL.Path == pathExternalWorkspaces {
			answer = hub.workspaces
		}
		if answer.status == 0 {
			answer.status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(answer.status)
		_, _ = w.Write([]byte(answer.body))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func configFor(baseURL, workspaceID string) map[string]any {
	config := map[string]any{"base_url": baseURL, "token_env": "ARCIPELAGO_TOKEN"}
	if workspaceID != "" {
		config["workspace_id"] = workspaceID
	}
	return config
}

func TestAvailableReportsTheFirstCauseAndProbesNothingWithoutACredential(t *testing.T) {
	t.Parallel()

	t.Run("a malformed configuration is rejected before anything else", func(t *testing.T) {
		t.Parallel()
		provider := New(Options{Getenv: func(string) string { return testToken }})
		err := provider.Available(context.Background(), nil)
		if err == nil {
			t.Fatal("Available accepted a configuration without a base URL")
		}
		var configErr *execution.ConfigurationError
		if !errors.As(err, &configErr) || configErr.Field != "base_url" {
			t.Fatalf("Available error = %v, want a ConfigurationError on base_url", err)
		}
	})

	t.Run("a missing credential costs no request", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return "" }})

		err := provider.Available(context.Background(), configFor(baseURL, "ws-1"))
		if err == nil || !strings.Contains(err.Error(), "ARCIPELAGO_TOKEN") {
			t.Fatalf("Available error = %v, want the name of the variable to export", err)
		}
		if served := hub.served(); len(served) != 0 {
			t.Fatalf("the hub was called %v; a probe with no credential must not reach the network", served)
		}
	})

	t.Run("the sentence about a missing credential is the one Execute says", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return "" }})
		config := configFor(baseURL, "ws-1")

		probeErr := provider.Available(context.Background(), config)
		_, executeErr := provider.Execute(context.Background(), execution.Request{
			SpecCode:       "US-001",
			Action:         execution.ActionPlan,
			Capability:     execution.CapabilitySpecPlan,
			ExecutionID:    "exec-1",
			ProviderConfig: config,
		})
		// Compared against each other rather than against a literal: what matters
		// is that the two cannot drift, not what they happen to say today.
		if probeErr == nil || executeErr == nil || probeErr.Error() != executeErr.Error() {
			t.Fatalf("probe said %v, Execute said %v; they must say the same thing", probeErr, executeErr)
		}
	})

	t.Run("a rejected credential is named as such", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{me: hubResponse{status: http.StatusUnauthorized, body: `{"error":"unauthorized"}`}}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return testToken }})

		err := provider.Available(context.Background(), configFor(baseURL, "ws-1"))
		if err == nil || !strings.Contains(err.Error(), "rejected the application credential") {
			t.Fatalf("Available error = %v, want the credential named as the cause", err)
		}
	})

	t.Run("a workspace outside the grant is named as such", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{
			me: hubResponse{body: `{"kind":"application","identity":{"id":"a","name":"ARchetipo","workspaceIds":["ws-other"]}}`},
		}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return testToken }})

		err := provider.Available(context.Background(), configFor(baseURL, "ws-1"))
		if err == nil || !strings.Contains(err.Error(), `not granted workspace "ws-1"`) {
			t.Fatalf("Available error = %v, want the workspace named as the cause", err)
		}
		// The diagnosis is precise where the dispatch used to give only a 404.
		if !strings.Contains(err.Error(), "apps grant") {
			t.Fatalf("Available error = %v, want the command that fixes it", err)
		}
	})

	t.Run("a fleet that could never host the work is unavailable", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{
			me: hubResponse{body: `{"kind":"application","identity":{"id":"a","name":"ARchetipo","workspaceIds":["ws-1"]}}`},
			workspaces: hubResponse{body: `{"workspaces":[{"id":"ws-1","name":"demo","cwdHint":"/workspace",` +
				`"requirements":["project:demo"],"archived":false,` +
				`"eligibleRunners":{"known":0,"online":0,"missing":["project:demo"]}}]}`},
		}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return testToken }})

		err := provider.Available(context.Background(), configFor(baseURL, "ws-1"))
		if err == nil || !strings.Contains(err.Error(), "no runner known to ARcipelago") {
			t.Fatalf("Available error = %v, want the absent runner named as the cause", err)
		}
		if !strings.Contains(err.Error(), "project:demo") {
			t.Fatalf("Available error = %v, want the missing capability named", err)
		}
	})

	t.Run("a fleet that is merely asleep is available", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{
			me: hubResponse{body: `{"kind":"application","identity":{"id":"a","name":"ARchetipo","workspaceIds":["ws-1"]}}`},
			workspaces: hubResponse{body: `{"workspaces":[{"id":"ws-1","name":"demo","cwdHint":"/workspace",` +
				`"requirements":["project:demo"],"archived":false,` +
				`"eligibleRunners":{"known":2,"online":0,"missing":[]}}]}`},
		}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return testToken }})

		// The work would wait for a machine that is coming back. Calling that
		// unavailable would send somebody to fix what is not broken.
		if err := provider.Available(context.Background(), configFor(baseURL, "ws-1")); err != nil {
			t.Fatalf("Available error = %v, want nil for an offline but sufficient fleet", err)
		}
	})

	t.Run("a hub too old to answer the discovery route is still available", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{
			me:         hubResponse{body: `{"kind":"application","identity":{"id":"a","name":"ARchetipo","workspaceIds":["ws-1"]}}`},
			workspaces: hubResponse{status: http.StatusNotFound, body: `{"error":"not_found"}`},
		}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return testToken }})

		if err := provider.Available(context.Background(), configFor(baseURL, "ws-1")); err != nil {
			t.Fatalf("Available error = %v, want nil: everything checkable was checked", err)
		}
	})

	t.Run("a configuration with no destination yet stops after the credential", func(t *testing.T) {
		t.Parallel()
		hub := &probeHub{
			me: hubResponse{body: `{"kind":"application","identity":{"id":"a","name":"ARchetipo","workspaceIds":["ws-1"]}}`},
		}
		baseURL := startProbeHub(t, hub)
		provider := New(Options{Getenv: func(string) string { return testToken }})

		// This is what `execution setup` holds while it works out which
		// workspace to write. It is not yet usable, but nothing about it is wrong.
		if err := provider.Available(context.Background(), configFor(baseURL, "")); err != nil {
			t.Fatalf("Available error = %v, want nil before a workspace is chosen", err)
		}
	})
}

func TestDiscoverWorkspacesDescribesEveryDestinationIncludingTheUnusableOnes(t *testing.T) {
	t.Parallel()

	hub := &probeHub{
		workspaces: hubResponse{body: `{"workspaces":[` +
			`{"id":"ws-1","name":"demo","cwdHint":"/workspace","requirements":["project:demo"],"archived":false,` +
			`"eligibleRunners":{"known":2,"online":1,"missing":[]}},` +
			`{"id":"ws-2","name":"stale","requirements":["project:gone"],"archived":false,` +
			`"eligibleRunners":{"known":0,"online":0,"missing":["project:gone"]}},` +
			`{"id":"ws-3","name":"retired","requirements":[],"archived":true,` +
			`"eligibleRunners":{"known":3,"online":3,"missing":[]}}]}`},
	}
	baseURL := startProbeHub(t, hub)
	provider := New(Options{Getenv: func(string) string { return testToken }})

	// No workspace_id: this is the call that finds one, so requiring it would
	// make the question unaskable until its own answer was known.
	refs, declared, err := execution.DiscoverWorkspaces(
		context.Background(), provider, configFor(baseURL, ""),
	)
	if err != nil {
		t.Fatalf("DiscoverWorkspaces failed: %v", err)
	}
	if !declared {
		t.Fatal("the arcipelago provider must declare that it has destinations to offer")
	}
	if len(refs) != 3 {
		t.Fatalf("got %d destinations, want 3", len(refs))
	}

	if refs[0].Name != "demo" || !refs[0].Ready {
		t.Fatalf("first destination = %+v, want demo and ready", refs[0])
	}
	if !strings.Contains(refs[0].Detail, "/workspace") || !strings.Contains(refs[0].Detail, "1 runner online") {
		t.Fatalf("detail = %q, want where the work lands and how much is awake", refs[0].Detail)
	}
	if refs[1].Ready || !strings.Contains(refs[1].NotReadyReason, "project:gone") {
		t.Fatalf("second destination = %+v, want not ready, naming what is missing", refs[1])
	}
	if refs[2].Ready || !strings.Contains(refs[2].NotReadyReason, "archived") {
		t.Fatalf("third destination = %+v, want not ready because archived", refs[2])
	}
}

func TestDiscoverWorkspacesIsNotDeclaredByProvidersWithoutDestinations(t *testing.T) {
	t.Parallel()

	// A local provider has nowhere to dispatch to, and "no destinations" would
	// read as a setup problem instead of as how it works.
	refs, declared, err := execution.DiscoverWorkspaces(context.Background(), stubLocalProvider{}, nil)
	if err != nil {
		t.Fatalf("DiscoverWorkspaces failed: %v", err)
	}
	if declared || refs != nil {
		t.Fatalf("declared = %v, refs = %v; want a provider that does not answer the question", declared, refs)
	}
}

type stubLocalProvider struct{}

func (stubLocalProvider) ID() string { return "local" }
func (stubLocalProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return nil, nil
}
func (stubLocalProvider) ValidateConfig(context.Context, map[string]any) error { return nil }
func (stubLocalProvider) Execute(context.Context, execution.Request) (execution.Result, error) {
	return execution.Result{}, nil
}
