package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector/filefs"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/validation"
)

func TestNextSpecCode(t *testing.T) {
	specsWith := func(codes ...string) []domain.Spec {
		out := make([]domain.Spec, 0, len(codes))
		for _, c := range codes {
			out = append(out, domain.Spec{Code: c})
		}
		return out
	}
	cases := []struct {
		name  string
		specs []domain.Spec
		want  string
	}{
		{"empty backlog starts at one", nil, "US-001"},
		{"contiguous codes continue", specsWith("US-001", "US-002"), "US-003"},
		{"gap continues from the highest", specsWith("US-001", "US-007", "US-003"), "US-008"},
		{"four digit code keeps padding", specsWith("US-0012"), "US-013"},
		{"non spec codes are ignored", specsWith("EP-001", "TASK-01", "US-002"), "US-003"},
		{"only non spec codes fall back to one", specsWith("EP-001", "TASK-01"), "US-001"},
		{"non numeric suffix is ignored", specsWith("US-abc", "US-004"), "US-005"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextSpecCode(tc.specs); got != tc.want {
				t.Fatalf("nextSpecCode: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeSpecTitle(t *testing.T) {
	if got, want := normalizeSpecTitle("  Creare  una Spec  "), normalizeSpecTitle("creare una spec"); got != want {
		t.Fatalf("normalized titles differ: got %q, want %q", got, want)
	}
	if normalizeSpecTitle("Creare una spec") == normalizeSpecTitle("Creare due spec") {
		t.Fatalf("different titles must not normalize to the same value")
	}
	if got := normalizeSpecTitle("\tCreare\n una  spec "); got != "creare una spec" {
		t.Fatalf("normalizeSpecTitle: got %q", got)
	}
}

func TestFindExistingSpec(t *testing.T) {
	backlog := []domain.Spec{
		{Code: "US-001", Title: "Creare una spec", Epic: domain.Epic{Code: "EP-005", Title: "Esperienza"}},
	}

	got, ok := findExistingSpec(backlog, "EP-005", "  Creare  una Spec  ")
	if !ok || got.Code != "US-001" {
		t.Fatalf("same epic and normalized title must match: ok=%v spec=%+v", ok, got)
	}
	if _, ok := findExistingSpec(backlog, "EP-001", "Creare una spec"); ok {
		t.Fatalf("same title in a different epic must not match")
	}
	if _, ok := findExistingSpec(backlog, "EP-005", "Creare un piano"); ok {
		t.Fatalf("a different title in the same epic must not match")
	}
	if _, ok := findExistingSpec(nil, "EP-005", "Creare una spec"); ok {
		t.Fatalf("an empty backlog must not match")
	}
}

func TestCreateSpecFieldFor(t *testing.T) {
	cases := map[string]string{
		"specs[0].title":     "title",
		"specs[0].epic.code": "epic_code",
		"specs[0].body":      "body",
		"specs[0].priority":  "priority",
		"specs[0].points":    "points",
		"specs[0].code":      "",
		"specs[0].status":    "",
		"specs":              "",
	}
	for path, want := range cases {
		if got := createSpecFieldFor(path); got != want {
			t.Fatalf("createSpecFieldFor(%q): got %q, want %q", path, got, want)
		}
	}
}

func TestCreateSpecFieldErrors(t *testing.T) {
	findings := []domain.ValidationFinding{
		{Code: "SPEC_TITLE_EMPTY", Severity: validation.SeverityError, Path: "specs[0].title", Message: "spec title is required"},
		{Code: "SPEC_BLOCKER_UNKNOWN", Severity: validation.SeverityWarning, Path: "specs[0].blocked_by", Message: "unknown blocker"},
		{Code: "SPEC_ACCEPTANCE_MISSING", Severity: validation.SeverityError, Path: "specs[0].body", Message: "acceptance criteria are required"},
	}
	got := createSpecFieldErrors(findings)
	if got == nil {
		t.Fatalf("createSpecFieldErrors must return a non-nil slice")
	}
	if len(got) != 2 {
		t.Fatalf("only error findings must be kept: got %+v", got)
	}
	if got[0].Field != "title" || got[0].Code != "SPEC_TITLE_EMPTY" || got[0].Message == "" {
		t.Fatalf("unexpected first field error: %+v", got[0])
	}
	if got[1].Field != "body" || got[1].Code != "SPEC_ACCEPTANCE_MISSING" {
		t.Fatalf("unexpected second field error: %+v", got[1])
	}
	for _, fe := range got {
		if fe.Code == "SPEC_BLOCKER_UNKNOWN" {
			t.Fatalf("warning findings must be discarded: %+v", got)
		}
	}
}

// --- POST /api/spec integration tests ---------------------------------------

const validCreateBody = "**User Story**\nCome revisore voglio creare una spec.\n\n**Dimostrazione**\nIl revisore compila il form e vede la nuova card TODO.\n\n**Criteri di accettazione**\n- [ ] AC-1 — la spec appare nella colonna TODO.\n"

func validCreateReq() map[string]any {
	return map[string]any{
		"epic_code": "EP-001",
		"title":     "Creare una spec",
		"priority":  "MEDIUM",
		"points":    3,
		"body":      validCreateBody,
	}
}

func postSpec(t *testing.T, srv *Server, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/spec", bytes.NewReader(raw))
	srv.mux.ServeHTTP(w, r)
	return w
}

func decodeCreateView(t *testing.T, w *httptest.ResponseRecorder) createSpecView {
	t.Helper()
	var view createSpecView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	return view
}

func decodeFieldErrors(t *testing.T, w *httptest.ResponseRecorder) []fieldError {
	t.Helper()
	var body struct {
		Code   string       `json:"code"`
		Fields []fieldError `json:"fields"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (body=%s)", err, w.Body.String())
	}
	return body.Fields
}

func TestCreateSpecAssignsProgressiveCode(t *testing.T) {
	srv, conn := newTestServer(t)
	seedSpecs(t, conn)

	w := postSpec(t, srv, validCreateReq())
	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	view := decodeCreateView(t, w)
	if !view.Created {
		t.Fatalf("created: got false, want true")
	}
	if view.Spec.Code != "US-003" {
		t.Fatalf("code: got %q, want US-003", view.Spec.Code)
	}
	if view.Spec.Status != domain.StatusTodo {
		t.Fatalf("status: got %q, want TODO", view.Spec.Status)
	}
	// The epic title is resolved from the workspace, never sent by the client.
	if view.Spec.Epic.Title != "F" {
		t.Fatalf("epic title: got %q, want F", view.Spec.Epic.Title)
	}

	// AC-2 oracle: the body is readable back through the connector.
	rw := httptest.NewRecorder()
	srv.mux.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/api/spec/US-003", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("GET /api/spec/US-003: got %d, body=%s", rw.Code, rw.Body.String())
	}
	var detail struct {
		Spec domain.Spec `json:"spec"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Spec.Body != validCreateBody {
		t.Fatalf("body round-trip mismatch: got %q", detail.Spec.Body)
	}
}

func TestCreateSpecRejectsUnknownEpic(t *testing.T) {
	srv, conn := newTestServer(t)
	seedSpecs(t, conn)

	req := validCreateReq()
	req["epic_code"] = "EP-999"
	w := postSpec(t, srv, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	fields := decodeFieldErrors(t, w)
	if len(fields) != 1 || fields[0].Field != "epic_code" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
	specs, err := conn.FetchBacklogItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("backlog must be untouched: got %d specs", len(specs))
	}
}

func TestCreateSpecRejectsInvalidPayload(t *testing.T) {
	srv, conn := newTestServer(t)
	seedSpecs(t, conn)

	req := validCreateReq()
	req["title"] = ""
	req["body"] = "**User Story**\nQualcosa.\n\n**Dimostrazione**\nSi osserva qualcosa.\n"
	w := postSpec(t, srv, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	seen := map[string]bool{}
	for _, fe := range decodeFieldErrors(t, w) {
		seen[fe.Field] = true
	}
	if !seen["title"] || !seen["body"] {
		t.Fatalf("expected field errors on title and body: got %v", seen)
	}
	specs, err := conn.FetchBacklogItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("backlog must be untouched: got %d specs", len(specs))
	}

	// The rejected attempt must not have consumed the progressive code.
	ok := postSpec(t, srv, validCreateReq())
	if ok.Code != http.StatusCreated {
		t.Fatalf("follow-up create: got %d, body=%s", ok.Code, ok.Body.String())
	}
	if code := decodeCreateView(t, ok).Spec.Code; code != "US-003" {
		t.Fatalf("code: got %q, want US-003", code)
	}
}

func TestCreateSpecIsIdempotentOnRepeat(t *testing.T) {
	srv, conn := newTestServer(t)
	seedSpecs(t, conn)

	first := postSpec(t, srv, validCreateReq())
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: got %d, body=%s", first.Code, first.Body.String())
	}
	firstView := decodeCreateView(t, first)
	if !firstView.Created {
		t.Fatalf("first create must report created:true")
	}

	second := postSpec(t, srv, validCreateReq())
	if second.Code != http.StatusOK {
		t.Fatalf("repeat: got %d, body=%s", second.Code, second.Body.String())
	}
	secondView := decodeCreateView(t, second)
	if secondView.Created {
		t.Fatalf("repeat must report created:false")
	}
	if secondView.Spec.Code != firstView.Spec.Code {
		t.Fatalf("repeat code: got %q, want %q", secondView.Spec.Code, firstView.Spec.Code)
	}

	// Same identity written with different spacing and case.
	loose := validCreateReq()
	loose["title"] = "  CREARE  una spec "
	third := postSpec(t, srv, loose)
	if third.Code != http.StatusOK {
		t.Fatalf("normalized repeat: got %d, body=%s", third.Code, third.Body.String())
	}
	thirdView := decodeCreateView(t, third)
	if thirdView.Created || thirdView.Spec.Code != firstView.Spec.Code {
		t.Fatalf("normalized repeat must return the existing spec: %+v", thirdView)
	}

	specs, err := conn.FetchBacklogItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 {
		t.Fatalf("backlog must hold 3 specs: got %d", len(specs))
	}
	count := 0
	for _, s := range specs {
		if s.Code == firstView.Spec.Code {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("exactly one spec must carry %q: got %d", firstView.Spec.Code, count)
	}
}

func TestCreateSpecOnEmptyBacklog(t *testing.T) {
	srv, conn := newTestServer(t)

	w := postSpec(t, srv, validCreateReq())
	// This workspace holds neither specs nor declared epics, so the route
	// cannot resolve EP-001:
	// this is the state the UI mirrors by disabling the New spec button. The
	// US-001 path of nextSpecCode is covered by its own unit test, because the
	// route cannot reach it without a workspace epic.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	fields := decodeFieldErrors(t, w)
	if len(fields) != 1 || fields[0].Field != "epic_code" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
	specs, err := conn.FetchBacklogItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Fatalf("backlog must stay empty: got %d specs", len(specs))
	}
}

// --- POST /api/spec over a real filefs workspace -----------------------------

// newFileTestServer builds a viewer over a real `filefs` connector rooted in a
// temp dir. The `inmemory` connector used by newTestServer cannot represent an
// epic declared in the backlog without any spec, which is exactly the shape
// these two tests need.
func newFileTestServer(t *testing.T) (*Server, *filefs.Connector, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ProjectRoot = dir
	conn := filefs.New(cfg)
	srv, err := NewServer(conn, cfg, nil, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv, conn, dir
}

// writeBacklogFixture writes `.archetipo/backlog.yaml` with the declared epics
// and, for every spec, its own file under `.archetipo/specs/`, matching the
// on-disk layout loadStore/readSpecDocs expect.
func writeBacklogFixture(t *testing.T, dir string, epics []domain.Epic, specs []domain.Spec) {
	t.Helper()
	order := make([]string, 0, len(specs))
	for _, sp := range specs {
		order = append(order, sp.Code)
	}
	backlog := map[string]any{
		"schema":  "archetipo/backlog/v2",
		"version": 2,
		"epics":   epics,
		"order":   order,
	}
	raw, err := yaml.Marshal(backlog)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, ".archetipo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backlog.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if len(specs) == 0 {
		return
	}
	specsDir := filepath.Join(root, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, sp := range specs {
		doc := map[string]any{
			"schema":   "archetipo/spec/v2",
			"code":     sp.Code,
			"title":    sp.Title,
			"epic":     map[string]string{"code": sp.Epic.Code, "title": sp.Epic.Title},
			"priority": string(sp.Priority),
			"points":   sp.Points,
			"status":   string(sp.Status),
			"body":     sp.Body,
		}
		blob, err := yaml.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(specsDir, sp.Code+".yaml"), blob, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readBoardEpics(t *testing.T, srv *Server) []domain.Epic {
	t.Helper()
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/board", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/board: got %d, body=%s", w.Code, w.Body.String())
	}
	var view struct {
		Epics []domain.Epic `json:"epics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode board: %v (body=%s)", err, w.Body.String())
	}
	return view.Epics
}

func TestCreateSpecOnDeclaredEpicWithoutSpecs(t *testing.T) {
	srv, _, dir := newFileTestServer(t)
	writeBacklogFixture(t, dir,
		[]domain.Epic{
			{Code: "EP-900", Title: "Epica con spec"},
			{Code: "EP-901", Title: "Epica dichiarata senza spec"},
		},
		[]domain.Spec{{
			Code: "US-001", Title: "Spec esistente",
			Epic:     domain.Epic{Code: "EP-900", Title: "Epica con spec"},
			Priority: domain.PriorityMedium, Points: 3,
			Status: domain.StatusTodo, Body: validCreateBody,
		}},
	)

	// AC-1, first half: EP-901 is among the values the UI is offered, even
	// though no spec references it yet.
	epics := readBoardEpics(t, srv)
	found := false
	for _, e := range epics {
		if e.Code == "EP-901" {
			found = true
			if e.Title != "Epica dichiarata senza spec" {
				t.Fatalf("board epic title: got %q", e.Title)
			}
		}
	}
	if !found {
		t.Fatalf("GET /api/board must offer EP-901: got %+v", epics)
	}

	// AC-1, second half: the same epic is accepted by POST /api/spec.
	req := validCreateReq()
	req["epic_code"] = "EP-901"
	req["title"] = "Creare una spec sull'epica vuota"
	w := postSpec(t, srv, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	view := decodeCreateView(t, w)
	if !view.Created {
		t.Fatalf("created: got false, want true")
	}
	if view.Spec.Epic.Code != "EP-901" {
		t.Fatalf("epic code: got %q, want EP-901", view.Spec.Epic.Code)
	}
	if view.Spec.Epic.Title != "Epica dichiarata senza spec" {
		t.Fatalf("epic title must come from the workspace: got %q", view.Spec.Epic.Title)
	}
	if view.Spec.Epic.Title == req["title"] {
		t.Fatalf("epic title must not be taken from the payload: got %q", view.Spec.Epic.Title)
	}

	// The payload has no channel for an epic title at all: a request trying to
	// send one is refused, so the workspace declaration is the only source.
	spoof := validCreateReq()
	spoof["epic_code"] = "EP-901"
	spoof["title"] = "Creare un'altra spec"
	spoof["epic_title"] = "Titolo inviato dal client"
	if sw := postSpec(t, srv, spoof); sw.Code != http.StatusBadRequest {
		t.Fatalf("a client-sent epic title must be refused: got %d, body=%s", sw.Code, sw.Body.String())
	}
}

func TestCreateSpecOnBacklogWithEpicsAndNoSpecs(t *testing.T) {
	srv, conn, dir := newFileTestServer(t)
	// A backlog that declares epics but holds no spec: this is the
	// SaveInitialBacklog branch of handleCreateSpec.
	writeBacklogFixture(t, dir,
		[]domain.Epic{
			{Code: "EP-900", Title: "Epica con spec"},
			{Code: "EP-901", Title: "Epica dichiarata senza spec"},
		},
		nil,
	)

	req := validCreateReq()
	req["epic_code"] = "EP-900"
	w := postSpec(t, srv, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, body=%s", w.Code, w.Body.String())
	}
	view := decodeCreateView(t, w)
	if !view.Created {
		t.Fatalf("created: got false, want true")
	}
	if view.Spec.Code != "US-001" {
		t.Fatalf("code: got %q, want US-001", view.Spec.Code)
	}
	if view.Spec.Status != domain.StatusTodo {
		t.Fatalf("status: got %q, want TODO", view.Spec.Status)
	}

	// AC-2 oracle: the spec is readable back through the connector.
	rw := httptest.NewRecorder()
	srv.mux.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/api/spec/US-001", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("GET /api/spec/US-001: got %d, body=%s", rw.Code, rw.Body.String())
	}
	var detail struct {
		Spec domain.Spec `json:"spec"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Spec.Title != "Creare una spec" {
		t.Fatalf("title round-trip mismatch: got %q", detail.Spec.Title)
	}
	if detail.Spec.Body != validCreateBody {
		t.Fatalf("body round-trip mismatch: got %q", detail.Spec.Body)
	}

	specs, err := conn.FetchBacklogItems(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("backlog must hold exactly 1 spec: got %d", len(specs))
	}
	if specs[0].Status != domain.StatusTodo {
		t.Fatalf("stored status: got %q, want TODO", specs[0].Status)
	}

	// The other declared epic survives the initial write.
	raw, err := os.ReadFile(filepath.Join(dir, ".archetipo", "backlog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Epics []domain.Epic `yaml:"epics"`
	}
	if err := yaml.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	kept := false
	for _, e := range stored.Epics {
		if e.Code == "EP-901" {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("EP-901 must stay declared on disk: got %+v", stored.Epics)
	}
}
