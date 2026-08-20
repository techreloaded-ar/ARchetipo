package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// modelChoiceCatalog is the catalog every test of this file reads: two models,
// one of which declares an option and the other of which is the entry the
// provider itself would use when the configuration names no model. It is a
// function and not a package variable because the routes hand the catalog out
// by value and a shared slice would let one test observe another one's edit.
func modelChoiceCatalog() []execution.ModelOption {
	return []execution.ModelOption{
		{
			ID:    "m1",
			Label: "Model one",
			Options: []execution.ModelOptionField{{
				Name:  "opt",
				Label: "Option",
				Help:  "left unset the runtime decides",
				Choices: []execution.ModelOptionChoice{
					{Value: "a", Default: true},
					{Value: "b"},
				},
			}},
		},
		{ID: "m2", Default: true},
	}
}

// readModelChoice asks the route and decodes the answer into a plain map. The
// decoding is deliberately untyped: what these tests protect is the exact set
// of JSON key names the browser reads, and decoding into the Go view would
// assert the Go struct against itself and let a renamed tag pass unnoticed.
func readModelChoice(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/execution/model-choice", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the model choice is a fact to report, never an HTTP failure: %d %s", w.Code, w.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	return view
}

func modelChoiceString(t *testing.T, view map[string]any, key string) string {
	t.Helper()
	value, present := view[key]
	if !present {
		t.Fatalf("the answer carries no %q key: %#v", key, view)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("%q is not a string: %#v", key, value)
	}
	return text
}

func modelChoiceBool(t *testing.T, view map[string]any, key string) bool {
	t.Helper()
	value, present := view[key]
	if !present {
		t.Fatalf("the answer carries no %q key: %#v", key, view)
	}
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("%q is not a boolean: %#v", key, value)
	}
	return flag
}

// modelChoiceModelIDs reads the identifiers of the served catalog, in order.
func modelChoiceModelIDs(t *testing.T, view map[string]any) []string {
	t.Helper()
	raw, present := view["models"]
	if !present {
		t.Fatalf("the answer serves no catalog: %#v", view)
	}
	entries, ok := raw.([]any)
	if !ok {
		t.Fatalf("models is not a list: %#v", raw)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		model, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("a catalog entry is not an object: %#v", entry)
		}
		ids = append(ids, modelChoiceString(t, model, "id"))
	}
	return ids
}

// modelChoiceEntry returns one entry of the served catalog by identifier.
func modelChoiceEntry(t *testing.T, view map[string]any, id string) map[string]any {
	t.Helper()
	entries, ok := view["models"].([]any)
	if !ok {
		t.Fatalf("models is not a list: %#v", view["models"])
	}
	for _, entry := range entries {
		model, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("a catalog entry is not an object: %#v", entry)
		}
		if modelChoiceString(t, model, "id") == id {
			return model
		}
	}
	t.Fatalf("model %q is missing from the served catalog: %#v", id, view["models"])
	return nil
}

// newModelChoiceServer builds a viewer over one provider, optionally with that
// provider persisted as the workspace default carrying the given configuration.
// Passing a nil configuration persists nothing at all, which is how the missing
// default provider becomes observable.
func newModelChoiceServer(t *testing.T, provider execution.Provider, persisted map[string]any) *Server {
	t.Helper()
	var selection *config.DefaultProviderConfig
	if persisted != nil {
		selection = &config.DefaultProviderConfig{ID: provider.ID(), Config: persisted}
	}
	return newProviderListServer(t, selection, provider)
}

// AC-1: with a model saved for the workspace, the route reports that model, says
// it was inherited from the workspace, carries the options saved for it, and
// offers the whole catalog as the alternatives a single run could pick from.
func TestModelChoiceInheritsWorkspaceModel(t *testing.T) {
	provider := newCatalogProvider("cataloged", "", modelChoiceCatalog(), nil)
	srv := newModelChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})

	view := readModelChoice(t, srv)
	if got := modelChoiceString(t, view, "provider_id"); got != "cataloged" {
		t.Fatalf("provider_id is %q, want %q", got, "cataloged")
	}
	if got := modelChoiceString(t, view, "model"); got != "m1" {
		t.Fatalf("model is %q, want the configured %q", got, "m1")
	}
	if got := modelChoiceString(t, view, "model_source"); got != execution.ModelChoiceSourceWorkspace {
		t.Fatalf("model_source is %q, want %q: nothing was chosen for a single run", got, execution.ModelChoiceSourceWorkspace)
	}
	if got := modelChoiceString(t, view, "model_field"); got != execution.ModelFieldName {
		t.Fatalf("model_field is %q, want %q", got, execution.ModelFieldName)
	}
	options, ok := view["options"].(map[string]any)
	if !ok {
		t.Fatalf("options is not an object: %#v", view["options"])
	}
	if len(options) != 1 || options["opt"] != "a" {
		t.Fatalf("the options saved for the workspace did not reach the reader: %#v", options)
	}
	if !modelChoiceBool(t, view, "available") {
		t.Fatalf("choosing must be possible when the catalog is in hand: %#v", view)
	}
	if _, present := view["unavailable_reason"]; present {
		t.Fatalf("an available choice carries a reason: %#v", view["unavailable_reason"])
	}
	if ids := modelChoiceModelIDs(t, view); len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Fatalf("the catalog did not reach the reader whole and in order: %#v", ids)
	}
	// AC-2: the options a model declares travel inside that model, so the panel
	// can render the controls of the model the reader picks without a second
	// call.
	declared, ok := modelChoiceEntry(t, view, "m1")["options"].([]any)
	if !ok || len(declared) != 1 {
		t.Fatalf("model m1 lost the option it declares: %#v", modelChoiceEntry(t, view, "m1"))
	}
	option, ok := declared[0].(map[string]any)
	if !ok {
		t.Fatalf("the declared option is not an object: %#v", declared[0])
	}
	if got := modelChoiceString(t, option, "name"); got != "opt" {
		t.Fatalf("the declared option is named %q, want %q", got, "opt")
	}
	choices, ok := option["choices"].([]any)
	if !ok || len(choices) != 2 {
		t.Fatalf("the option lost its admissible values: %#v", option["choices"])
	}
}

// AC-1, AC-5: with no model saved, the effective model is the entry the catalog
// marks as the provider's own default, so a run started without choosing is
// never described with an empty model. The source stays "workspace" because
// nothing was chosen for this run: the domain declares only the two sources.
func TestModelChoiceFallsBackToCatalogDefault(t *testing.T) {
	provider := newCatalogProvider("cataloged", "", modelChoiceCatalog(), nil)
	srv := newModelChoiceServer(t, provider, map[string]any{})

	view := readModelChoice(t, srv)
	if got := modelChoiceString(t, view, "model"); got != "m2" {
		t.Fatalf("model is %q, want the catalog default %q", got, "m2")
	}
	if got := modelChoiceString(t, view, "model_source"); got != execution.ModelChoiceSourceWorkspace {
		t.Fatalf("model_source is %q, want %q", got, execution.ModelChoiceSourceWorkspace)
	}
	if !modelChoiceBool(t, view, "available") {
		t.Fatalf("choosing must stay possible when only the model is unset: %#v", view)
	}
}

// AC-6, first way: the provider declares no catalog at all. Choosing is not
// possible, the reason names the provider, no catalog is served — and the model
// the run would use is reported all the same, because the run can still start.
func TestModelChoiceOnProviderWithoutCatalog(t *testing.T) {
	srv := newModelChoiceServer(t, releasedProvider("plain", nil), map[string]any{"model": "m1"})

	view := readModelChoice(t, srv)
	if modelChoiceBool(t, view, "available") {
		t.Fatalf("a provider without a catalog offers a choice: %#v", view)
	}
	reason := modelChoiceString(t, view, "unavailable_reason")
	if !strings.Contains(reason, "plain") {
		t.Fatalf("the reason does not name the provider: %q", reason)
	}
	if _, present := view["models"]; present {
		t.Fatalf("a provider without a catalog served one: %#v", view["models"])
	}
	if got := modelChoiceString(t, view, "model"); got != "m1" {
		t.Fatalf("model is %q, want the configured %q: the run still uses it", got, "m1")
	}
	if got := modelChoiceString(t, view, "model_source"); got != execution.ModelChoiceSourceWorkspace {
		t.Fatalf("model_source is %q, want %q", got, execution.ModelChoiceSourceWorkspace)
	}
}

// AC-6, second way: the catalog is declared but cannot be obtained. The
// provider's own diagnostic is what the reader is told, verbatim.
func TestModelChoiceOnUnobtainableCatalog(t *testing.T) {
	const failure = "claude models: unexpected output from the runtime"
	provider := newCatalogProvider("failing", "", nil, errors.New(failure))
	srv := newModelChoiceServer(t, provider, map[string]any{"model": "m1"})

	view := readModelChoice(t, srv)
	if modelChoiceBool(t, view, "available") {
		t.Fatalf("an unobtainable catalog offered a choice: %#v", view)
	}
	if got := modelChoiceString(t, view, "unavailable_reason"); got != failure {
		t.Fatalf("the provider's diagnostic was rewritten: %q", got)
	}
	if _, present := view["models"]; present {
		t.Fatalf("an unobtainable catalog was served: %#v", view["models"])
	}
	if got := modelChoiceString(t, view, "model"); got != "m1" {
		t.Fatalf("model is %q, want the configured %q: the run still uses it", got, "m1")
	}
}

// AC-6, third way: no default provider is configured at all. There is nothing
// to choose from and nothing to start, but the question itself is answerable,
// so it is answered — not refused with a conflict or an internal error.
func TestModelChoiceWithoutDefaultProvider(t *testing.T) {
	srv := newModelChoiceServer(t, newCatalogProvider("cataloged", "", modelChoiceCatalog(), nil), nil)

	w := doJSON(t, srv, http.MethodGet, "/api/execution/model-choice", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("a missing default provider is a fact to report, not an HTTP failure: %d %s", w.Code, w.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if modelChoiceBool(t, view, "available") {
		t.Fatalf("a workspace without a default provider offered a choice: %#v", view)
	}
	reason := modelChoiceString(t, view, "unavailable_reason")
	if !strings.Contains(reason, "execution.default_provider") {
		t.Fatalf("the reason does not name what is missing: %q", reason)
	}
	if got := modelChoiceString(t, view, "provider_id"); got != "" {
		t.Fatalf("provider_id is %q, want empty: no provider is configured", got)
	}
}

// runChoiceProvider is the provider the start tests dispatch to: a cataloged
// provider that records the Request it is handed. Capturing the Request is the
// only way to assert what a per-run choice actually did, because the merged
// configuration is never written anywhere — it exists only between the route
// and the provider.
//
// It declares both a spec capability and a workspace one so the very same
// double can answer the two start routes, which is what makes the two tests
// comparable.
type runChoiceProvider struct {
	*catalogProvider

	requestMu sync.Mutex
	requests  []execution.Request
}

func newRunChoiceProvider(id string, models []execution.ModelOption) *runChoiceProvider {
	return &runChoiceProvider{catalogProvider: newCatalogProvider(id, "", models, nil)}
}

func (p *runChoiceProvider) Capabilities(context.Context) ([]execution.Capability, error) {
	return []execution.Capability{execution.CapabilitySpecPlan, execution.CapabilityWorkspaceInception}, nil
}

func (p *runChoiceProvider) Execute(_ context.Context, request execution.Request) (execution.Result, error) {
	p.requestMu.Lock()
	p.requests = append(p.requests, request)
	p.requestMu.Unlock()
	return execution.Result{Payload: json.RawMessage(`{"ok":true}`)}, nil
}

// dispatched returns the requests the provider was handed, in order.
func (p *runChoiceProvider) dispatched() []execution.Request {
	p.requestMu.Lock()
	defer p.requestMu.Unlock()
	return append([]execution.Request(nil), p.requests...)
}

// newRunChoiceServer builds a viewer over a real filefs workspace whose default
// provider is the given one, carrying the given configuration. It is
// newRunServer with the persisted configuration under the test's control:
// what a per-run choice is merged onto is exactly that configuration, so a
// server that could only persist an empty one could not observe the merge.
func newRunChoiceServer(t *testing.T, provider execution.Provider, persisted map[string]any) (*Server, config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := filefs.New(cfg)
	seedRunSpecs(t, conn)
	registry := execution.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if _, err := config.UpdateDefaultProvider(dir, config.DefaultProviderConfig{ID: provider.ID(), Config: persisted}); err != nil {
		t.Fatal(err)
	}
	srv, err := NewServer(conn, cfg, registry, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.session().dispatch.wait(5 * time.Second) })
	return srv, cfg
}

// startWithChoice posts a start request carrying whatever the test wants to
// send, which is how a payload with model and model_options reaches the route.
func startWithChoice(t *testing.T, srv *Server, path string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, path, payload)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("undecodable response (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, body
}

// awaitDispatchedRequest waits for the run to reach a terminal state and
// returns the single Request the provider was handed. Waiting on the record is
// what makes the assertion deterministic without a sleep: the dispatch outlives
// the response that started it.
func awaitDispatchedRequest(t *testing.T, srv *Server, provider *runChoiceProvider, id string) execution.Request {
	t.Helper()
	awaitTerminal(t, srv, id)
	requests := provider.dispatched()
	if len(requests) != 1 {
		t.Fatalf("the provider was handed %d requests, want exactly 1: %#v", len(requests), requests)
	}
	return requests[0]
}

// recordModelChoice reads the run record the way the browser does and returns
// its model_choice object untyped, so what is asserted is the JSON the panel
// renders and not the Go struct behind it.
func recordModelChoice(t *testing.T, srv *Server, id string) map[string]any {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/execution/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/execution/%s: %d %s", id, w.Code, w.Body.String())
	}
	var record map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	choice, ok := record["model_choice"].(map[string]any)
	if !ok {
		t.Fatalf("the record carries no model_choice object: %s", w.Body.String())
	}
	return choice
}

// executionRecordCount counts every record on disk, whatever spec it belongs
// to. A refused start must leave the directory exactly as it found it, and a
// count filtered by spec code could not see a workspace-scoped record.
func executionRecordCount(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".archetipo", "executions"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	return count
}

// assertNoTrace is the whole of what a refused start owes: the spec did not
// move, no record was written, and the provider was never called. The three
// facts are asserted together because a rejection that satisfies only two of
// them is still a rejection that left something behind.
func assertNoTrace(t *testing.T, srv *Server, provider *runChoiceProvider, root, code string) {
	t.Helper()
	if detail := runSpecDetail(t, srv, code); detail.Spec.Status != domain.StatusTodo {
		t.Fatalf("a refused start moved %s to %s", code, detail.Spec.Status)
	}
	if got := executionRecordCount(t, root); got != 0 {
		t.Fatalf("a refused start wrote %d execution records", got)
	}
	if requests := provider.dispatched(); len(requests) != 0 {
		t.Fatalf("a refused start still reached the provider: %#v", requests)
	}
}

// AC-2, AC-3: a start that names another model runs with that model and with
// the options that model declares — and with nothing else. The whole map is
// asserted because the point of the merge is a key that is *absent*: "opt"
// belongs to m1, and m2 does not declare it, so it must not survive.
func TestRunSpecActionAppliesRunModelChoice(t *testing.T) {
	provider := newRunChoiceProvider("cataloged", modelChoiceCatalog())
	srv, _ := newRunChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})

	status, started := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{"action": "plan", "model": "m2"})
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatalf("the started record has no id: %v", started)
	}

	request := awaitDispatchedRequest(t, srv, provider, id)
	want := map[string]any{"model": "m2"}
	if !reflect.DeepEqual(request.ProviderConfig, want) {
		t.Fatalf("the provider ran with %#v, want %#v: the option of the previous model must not survive", request.ProviderConfig, want)
	}

	// AC-3: the record says which model ran and that it was chosen for this run
	// alone, so the detail can tell it apart from an inherited one.
	choice := recordModelChoice(t, srv, id)
	wantChoice := map[string]any{"model": "m2", "source": execution.ModelChoiceSourceRun}
	if !reflect.DeepEqual(choice, wantChoice) {
		t.Fatalf("model_choice is %#v, want %#v", choice, wantChoice)
	}
}

// AC-4: a run started with a different model changes nothing on disk. The
// configuration file is compared byte for byte, and the panel is asked again,
// because a workspace that reads back changed would be the same defect twice.
func TestRunSpecActionLeavesWorkspaceConfigUntouched(t *testing.T) {
	provider := newRunChoiceProvider("cataloged", modelChoiceCatalog())
	srv, cfg := newRunChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})
	before := configFileContent(t, cfg.ProjectRoot)

	status, started := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{
		"action":        "plan",
		"model":         "m1",
		"model_options": map[string]string{"opt": "b"},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	awaitTerminal(t, srv, id)

	if after := configFileContent(t, cfg.ProjectRoot); after != before {
		t.Fatalf("the run rewrote the workspace configuration:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	view := readProviders(t, srv)
	if view.Default == nil {
		t.Fatalf("the workspace lost its default provider: %#v", view)
	}
	if view.Default.Config["model"] != "m1" || view.Default.Config["opt"] != "a" {
		t.Fatalf("the workspace configuration changed: %#v", view.Default.Config)
	}
}

// AC-5: a start that chooses nothing is the start of before this spec — the
// configuration reaches the provider verbatim — and the record still says which
// model ran, inherited from the workspace.
func TestRunSpecActionWithoutChoiceUsesWorkspaceConfig(t *testing.T) {
	provider := newRunChoiceProvider("cataloged", modelChoiceCatalog())
	srv, _ := newRunChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})

	status, started := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{"action": "plan"})
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	id, _ := started["id"].(string)

	request := awaitDispatchedRequest(t, srv, provider, id)
	want := map[string]any{"model": "m1", "opt": "a"}
	if !reflect.DeepEqual(request.ProviderConfig, want) {
		t.Fatalf("the provider ran with %#v, want the workspace configuration %#v", request.ProviderConfig, want)
	}

	choice := recordModelChoice(t, srv, id)
	wantChoice := map[string]any{
		"model":   "m1",
		"options": map[string]any{"opt": "a"},
		"source":  execution.ModelChoiceSourceWorkspace,
	}
	if !reflect.DeepEqual(choice, wantChoice) {
		t.Fatalf("model_choice is %#v, want %#v", choice, wantChoice)
	}
}

// AC-2: a model the catalog does not admit is invalid input, named by field, and
// it stops before anything happens at all.
func TestRunSpecActionRejectsUnknownModel(t *testing.T) {
	provider := newRunChoiceProvider("cataloged", modelChoiceCatalog())
	srv, cfg := newRunChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})

	status, body := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{"action": "plan", "model": "nope"})
	if status != http.StatusBadRequest {
		t.Fatalf("POST: %d %v", status, body)
	}
	if body["field"] != execution.ModelFieldName {
		t.Fatalf("the refusal names field %v, want %q", body["field"], execution.ModelFieldName)
	}
	assertNoTrace(t, srv, provider, cfg.ProjectRoot, "US-901")
}

// AC-2: a value the chosen model does not admit is refused the same way, naming
// the option itself and not the model.
func TestRunSpecActionRejectsUnknownOptionValue(t *testing.T) {
	provider := newRunChoiceProvider("cataloged", modelChoiceCatalog())
	srv, cfg := newRunChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})

	status, body := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{
		"action":        "plan",
		"model":         "m1",
		"model_options": map[string]string{"opt": "zzz"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("POST: %d %v", status, body)
	}
	if body["field"] != "opt" {
		t.Fatalf("the refusal names field %v, want %q", body["field"], "opt")
	}
	assertNoTrace(t, srv, provider, cfg.ProjectRoot, "US-901")
}

// AC-6: with no catalog to choose from, choosing is a conflict carrying the
// reason — and starting without choosing still works, which is the whole point
// of keeping the two apart.
func TestRunSpecActionRefusesChoiceWithoutCatalog(t *testing.T) {
	provider := releasedProvider("plain", nil)
	srv, cfg := newRunChoiceServer(t, provider, map[string]any{"model": "m1"})

	status, body := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{"action": "plan", "model": "x"})
	if status != http.StatusConflict {
		t.Fatalf("POST with a choice: %d %v", status, body)
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, "plain") {
		t.Fatalf("the refusal does not carry the reason: %q", message)
	}
	if got := executionRecordCount(t, cfg.ProjectRoot); got != 0 {
		t.Fatalf("a refused choice wrote %d execution records", got)
	}

	status, started := startWithChoice(t, srv, "/api/spec/US-901/execution", map[string]any{"action": "plan"})
	if status != http.StatusCreated {
		t.Fatalf("POST without a choice: %d %v", status, started)
	}
	id, _ := started["id"].(string)
	awaitTerminal(t, srv, id)
}

// AC-2, AC-3: the workspace-scoped start behaves exactly like the spec-scoped
// one — the same merge, the same pruning, the same record — on a run that
// belongs to no spec.
func TestRunWorkspaceActionAppliesRunModelChoice(t *testing.T) {
	provider := newRunChoiceProvider("cataloged", modelChoiceCatalog())
	srv, _ := newRunChoiceServer(t, provider, map[string]any{"model": "m1", "opt": "a"})

	status, started := startWithChoice(t, srv, "/api/workspace/execution", map[string]any{"action": "inception", "model": "m2"})
	if status != http.StatusCreated {
		t.Fatalf("POST: %d %v", status, started)
	}
	if started["spec_code"] != "" {
		t.Fatalf("a workspace run was recorded against a spec: %v", started["spec_code"])
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatalf("the started record has no id: %v", started)
	}

	request := awaitDispatchedRequest(t, srv, provider, id)
	want := map[string]any{"model": "m2"}
	if !reflect.DeepEqual(request.ProviderConfig, want) {
		t.Fatalf("the provider ran with %#v, want %#v", request.ProviderConfig, want)
	}
	if request.SpecCode != "" {
		t.Fatalf("the workspace request names spec %q", request.SpecCode)
	}

	choice := recordModelChoice(t, srv, id)
	wantChoice := map[string]any{"model": "m2", "source": execution.ModelChoiceSourceRun}
	if !reflect.DeepEqual(choice, wantChoice) {
		t.Fatalf("model_choice is %#v, want %#v", choice, wantChoice)
	}
}
