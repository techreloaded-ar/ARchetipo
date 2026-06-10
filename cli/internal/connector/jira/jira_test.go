package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

// fakeJira is a minimal in-memory Jira Cloud backend exercising exactly the
// REST endpoints the connector calls. It keeps enough state (issues, statuses,
// subtasks) to drive the connector through a full backlog -> plan -> transition
// lifecycle without a network.
type fakeJira struct {
	t      *testing.T
	issues map[string]*fakeIssue
	order  []string // creation order, so search results are deterministic
	nextID int
}

type fakeIssue struct {
	Key       string
	Fields    map[string]any
	StatusNm  string
	ParentKey string
}

func newFakeJira(t *testing.T) *fakeJira {
	return &fakeJira{t: t, issues: map[string]*fakeIssue{}, nextID: 1}
}

func (f *fakeJira) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	path := req.URL.Path
	switch {
	case req.Method == "GET" && path == "/rest/api/2/myself":
		return jsonResp(200, map[string]any{"accountId": "acc-1"}), nil
	case req.Method == "POST" && path == "/rest/api/2/issue":
		return f.createIssue(body), nil
	case req.Method == "POST" && path == "/rest/api/2/search":
		return f.search(body), nil
	case req.Method == "GET" && strings.HasPrefix(path, "/rest/api/2/issue/") && strings.HasSuffix(path, "/transitions"):
		return f.transitionsList(), nil
	case req.Method == "POST" && strings.HasPrefix(path, "/rest/api/2/issue/") && strings.HasSuffix(path, "/transitions"):
		return f.applyTransition(path, body), nil
	case req.Method == "POST" && strings.HasPrefix(path, "/rest/api/2/issue/") && strings.HasSuffix(path, "/comment"):
		return jsonResp(201, map[string]any{"id": "c1"}), nil
	case req.Method == "GET" && strings.HasPrefix(path, "/rest/api/2/issue/"):
		return f.getIssue(path), nil
	case req.Method == "PUT" && strings.HasPrefix(path, "/rest/api/2/issue/"):
		return f.updateIssue(path, body), nil
	}
	f.t.Fatalf("unexpected jira call: %s %s", req.Method, path)
	return nil, nil
}

func (f *fakeJira) createIssue(body []byte) *http.Response {
	var in struct {
		Fields map[string]any `json:"fields"`
	}
	_ = json.Unmarshal(body, &in)
	key := "ARCH-" + strconv.Itoa(f.nextID)
	f.nextID++
	iss := &fakeIssue{Key: key, Fields: in.Fields, StatusNm: "TODO"}
	if p, ok := in.Fields["parent"].(map[string]any); ok {
		iss.ParentKey, _ = p["key"].(string)
	}
	f.issues[key] = iss
	f.order = append(f.order, key)
	return jsonResp(201, map[string]any{"key": key})
}

func (f *fakeJira) keyFromPath(path string) string {
	rest := strings.TrimPrefix(path, "/rest/api/2/issue/")
	return strings.SplitN(rest, "/", 2)[0]
}

func (f *fakeJira) getIssue(path string) *http.Response {
	key := f.keyFromPath(path)
	// path may carry "?fields=..." already stripped by net/url into RawQuery,
	// but here keyFromPath split on '/', so trim a stray '?'.
	if i := strings.IndexByte(key, '?'); i >= 0 {
		key = key[:i]
	}
	iss, ok := f.issues[key]
	if !ok {
		return jsonResp(404, map[string]any{"errorMessages": []string{"not found"}})
	}
	return jsonResp(200, map[string]any{"key": key, "fields": f.fieldsOf(iss)})
}

func (f *fakeJira) fieldsOf(iss *fakeIssue) map[string]any {
	out := map[string]any{}
	for k, v := range iss.Fields {
		out[k] = v
	}
	out["status"] = map[string]any{"name": iss.StatusNm}
	return out
}

func (f *fakeJira) updateIssue(path string, body []byte) *http.Response {
	key := f.keyFromPath(path)
	iss, ok := f.issues[key]
	if !ok {
		return jsonResp(404, map[string]any{"errorMessages": []string{"not found"}})
	}
	var in struct {
		Fields map[string]any `json:"fields"`
	}
	_ = json.Unmarshal(body, &in)
	for k, v := range in.Fields {
		iss.Fields[k] = v
	}
	return jsonResp(204, nil)
}

func (f *fakeJira) transitionsList() *http.Response {
	// Offer a transition to every canonical status name used in tests.
	names := []string{"TODO", "PLANNED", "IN PROGRESS", "REVIEW", "DONE"}
	var ts []map[string]any
	for i, n := range names {
		ts = append(ts, map[string]any{"id": strconv.Itoa(i + 1), "to": map[string]any{"name": n}})
	}
	return jsonResp(200, map[string]any{"transitions": ts})
}

func (f *fakeJira) applyTransition(path string, body []byte) *http.Response {
	key := f.keyFromPath(path)
	iss, ok := f.issues[key]
	if !ok {
		return jsonResp(404, map[string]any{"errorMessages": []string{"not found"}})
	}
	var in struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	}
	_ = json.Unmarshal(body, &in)
	names := map[string]string{"1": "TODO", "2": "PLANNED", "3": "IN PROGRESS", "4": "REVIEW", "5": "DONE"}
	if n, ok := names[in.Transition.ID]; ok {
		iss.StatusNm = n
	}
	return jsonResp(204, nil)
}

func (f *fakeJira) search(body []byte) *http.Response {
	var in struct {
		JQL string `json:"jql"`
	}
	_ = json.Unmarshal(body, &in)
	var matched []map[string]any
	for _, key := range f.order {
		iss := f.issues[key]
		if !f.matchesJQL(iss, in.JQL) {
			continue
		}
		matched = append(matched, map[string]any{"key": iss.Key, "fields": f.fieldsOf(iss)})
	}
	return jsonResp(200, map[string]any{
		"startAt": 0, "maxResults": 100, "total": len(matched), "issues": matched,
	})
}

func (f *fakeJira) matchesJQL(iss *fakeIssue, jql string) bool {
	if strings.Contains(jql, "labels = "+backlogLabel) {
		labels, _ := iss.Fields["labels"].([]any)
		for _, l := range labels {
			if s, _ := l.(string); s == backlogLabel {
				return true
			}
		}
		return false
	}
	if strings.Contains(jql, "parent = ") {
		// extract the key after "parent = "
		idx := strings.Index(jql, "parent = ")
		rest := jql[idx+len("parent = "):]
		want := strings.Fields(rest)[0]
		return iss.ParentKey == want
	}
	return false
}

func jsonResp(status int, payload any) *http.Response {
	var buf []byte
	if payload != nil {
		buf, _ = json.Marshal(payload)
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(buf)),
		Header:     make(http.Header),
	}
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Connector = config.ConnectorJira
	cfg.Jira = config.JiraConfig{BaseURL: "https://acme.atlassian.net", ProjectKey: "ARCH"}
	return cfg
}

func newTestConnector(t *testing.T) (*Connector, *fakeJira) {
	t.Setenv("JIRA_EMAIL", "bot@acme.com")
	t.Setenv("JIRA_API_TOKEN", "tok")
	f := newFakeJira(t)
	return NewWithDoer(testConfig(), f), f
}

func sampleSpecs() []domain.Spec {
	return []domain.Spec{
		{Code: "US-001", Title: "Setup", Epic: domain.Epic{Code: "EP-001", Title: "Foundations"},
			Priority: domain.PriorityHigh, Points: 3, Status: domain.StatusTodo, Body: "## Spec\n\nAs a user, I want X."},
		{Code: "US-002", Title: "Auth", Epic: domain.Epic{Code: "EP-001", Title: "Foundations"},
			Priority: domain.PriorityMedium, Points: 5, Status: domain.StatusTodo, Body: "## Spec\n\nLogin."},
	}
}

func TestInitialize_VerifiesAuth(t *testing.T) {
	c, _ := newTestConnector(t)
	info, err := c.InitializeConnector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Connector != config.ConnectorJira {
		t.Errorf("connector name = %q", info.Connector)
	}
	if info.Workflow.Statuses.Todo == "" {
		t.Errorf("workflow statuses not populated")
	}
}

func TestInitialize_MissingCredentials(t *testing.T) {
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_API_TOKEN", "")
	c := NewWithDoer(testConfig(), newFakeJira(t))
	if _, err := c.InitializeConnector(context.Background()); err == nil {
		t.Fatal("expected auth error when credentials are missing")
	}
}

// TestSpecRoundTrip covers the Jira-specific encoding the shared conformance
// suite does not assert: the epic title is carried in a description marker, the
// marker is stripped from the returned body, and the priority is mapped through
// the default High/Medium/Low names. Idempotency (a second SaveInitialBacklog
// must conflict) is also Jira-path specific.
func TestSpecRoundTrip(t *testing.T) {
	c, _ := newTestConnector(t)
	ctx := context.Background()
	specs := sampleSpecs()
	if _, err := c.SaveInitialBacklog(ctx, specs); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveInitialBacklog(ctx, specs); err == nil {
		t.Fatal("expected conflict on second SaveInitialBacklog")
	}
	det, err := c.ReadSpecDetail(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if det.Epic.Code != "EP-001" || det.Epic.Title != "Foundations" {
		t.Errorf("epic lost: %+v", det.Epic)
	}
	if strings.Contains(det.Body, "archetipo:epic") {
		t.Errorf("epic marker leaked into body: %q", det.Body)
	}
	if det.Priority != domain.PriorityHigh {
		t.Errorf("priority lost: %s", det.Priority)
	}
}

// TestReadPlanBody verifies the PlanBodyReader optional capability: the plan
// body SavePlan appends to the story description is returned separately.
func TestReadPlanBody(t *testing.T) {
	c, _ := newTestConnector(t)
	ctx := context.Background()
	if _, err := c.SaveInitialBacklog(ctx, sampleSpecs()); err != nil {
		t.Fatal(err)
	}
	plan := domain.PlanInput{PlanBody: "## Soluzione Tecnica\n\nSpiegazione."}
	if _, err := c.SavePlan(ctx, "US-001", plan); err != nil {
		t.Fatal(err)
	}
	body, err := c.ReadPlanBody(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Soluzione Tecnica") {
		t.Errorf("plan body not returned: %q", body)
	}
}

func TestUpdateSpec(t *testing.T) {
	c, _ := newTestConnector(t)
	ctx := context.Background()
	if _, err := c.SaveInitialBacklog(ctx, sampleSpecs()); err != nil {
		t.Fatal(err)
	}
	newTitle := "Setup rinominato"
	if _, err := c.UpdateSpec(ctx, "US-001", domain.SpecUpdate{Title: &newTitle}); err != nil {
		t.Fatal(err)
	}
	det, err := c.ReadSpecDetail(ctx, "US-001")
	if err != nil {
		t.Fatal(err)
	}
	if det.Title != newTitle {
		t.Errorf("title not updated: %q", det.Title)
	}
	if det.Code != "US-001" {
		t.Errorf("code prefix lost after rename: %q", det.Code)
	}
}
