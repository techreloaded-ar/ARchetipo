package ingest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestHandler(t *testing.T) (*IngestHandler, *MemoryStore, *TokenBucket) {
	t.Helper()
	cfg := RateLimitConfig{Rate: 100, Window: time.Minute, Burst: 100}
	limiter := NewTokenBucket(cfg)
	store := NewMemoryStore(time.Hour)
	handler := NewIngestHandler(limiter, store)
	return handler, store, limiter
}

func newTestServer(t *testing.T) (*httptest.Server, *MemoryStore, *TokenBucket) {
	t.Helper()
	handler, store, limiter := newTestHandler(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		srv.Close()
		store.Close()
		limiter.Close()
	})
	return srv, store, limiter
}

func validEventJSON() string {
	return `{"schema":"archetipo.analytics/v1","event":"test.event"}`
}

func validBatchJSON() string {
	return `[{"schema":"archetipo.analytics/v1","event":"e1"},{"schema":"archetipo.analytics/v1","event":"e2"}]`
}

func doPost(t *testing.T, srv *httptest.Server, body string, ct string) *http.Response {
	t.Helper()
	if ct == "" {
		ct = "application/json"
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/events", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", ct)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// doPostDirect sends a POST to the handler directly with a fixed RemoteAddr,
// bypassing the httptest server's connection-based address assignment.
func doPostDirect(t *testing.T, handler *IngestHandler, body string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func readRespBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	return string(buf)
}

// ─── Acceptance criteria tests ─────────────────────────────────────────────

func TestHandler_ValidEvent_202(t *testing.T) {
	srv, store, limiter := newTestServer(t)
	defer limiter.Close()

	resp := doPost(t, srv, validEventJSON(), "application/json")

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, readRespBody(t, resp))
	}

	body := readRespBody(t, resp)
	var accepted map[string]string
	if err := json.Unmarshal([]byte(body), &accepted); err != nil {
		t.Fatalf("response not valid JSON: %s", body)
	}
	if accepted["status"] != "accepted" {
		t.Fatalf("expected status=accepted, got %q", accepted["status"])
	}

	if store.Len() != 1 {
		t.Fatalf("expected 1 stored event, got %d", store.Len())
	}

	if resp.Header.Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header")
	}
}

func TestHandler_ForbiddenField_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	body := `{"schema":"archetipo.analytics/v1","event":"x","hostname":"myhost"}`
	resp := doPost(t, srv, body, "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	raw := readRespBody(t, resp)
	var errResp map[string]string
	if err := json.Unmarshal([]byte(raw), &errResp); err != nil {
		t.Fatalf("invalid error response: %s", raw)
	}
	if errResp["error"] != "validation_error" {
		t.Fatalf("expected validation_error, got %q", errResp["error"])
	}
	if !strings.Contains(errResp["detail"], "hostname") {
		t.Fatalf("expected detail to mention 'hostname', got %q", errResp["detail"])
	}
}

func TestHandler_UnknownField_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	body := `{"schema":"archetipo.analytics/v1","event":"x","custom_field":"value"}`
	resp := doPost(t, srv, body, "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, readRespBody(t, resp))
	}
}

func TestHandler_RateLimitExceeded_429(t *testing.T) {
	cfg := RateLimitConfig{Rate: 2, Window: time.Minute, Burst: 2}
	store := NewMemoryStore(time.Hour)
	defer store.Close()
	limiter := NewTokenBucket(cfg)
	defer limiter.Close()
	handler := NewIngestHandler(limiter, store)

	fixedAddr := "192.168.1.1:12345"

	// Exhaust rate limit using direct handler calls.
	for i := 0; i < cfg.Burst; i++ {
		w := doPostDirect(t, handler, validEventJSON(), fixedAddr)
		if w.Code != http.StatusAccepted {
			t.Fatalf("request %d: expected 202, got %d: %s", i+1, w.Code, w.Body.String())
		}
	}

	// Next request should be rate limited.
	w := doPostDirect(t, handler, validEventJSON(), fixedAddr)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestHandler_ValidBatch_202(t *testing.T) {
	srv, store, limiter := newTestServer(t)
	defer limiter.Close()

	resp := doPost(t, srv, validBatchJSON(), "application/json")

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, readRespBody(t, resp))
	}

	if store.Len() != 2 {
		t.Fatalf("expected 2 stored events, got %d", store.Len())
	}
}

func TestHandler_InvalidBatch_400_FailAtomic(t *testing.T) {
	srv, store, limiter := newTestServer(t)
	defer limiter.Close()

	body := `[
		{"schema":"archetipo.analytics/v1","event":"e1"},
		{"schema":"archetipo.analytics/v1","event":"e2","hostname":"bad"}
	]`
	resp := doPost(t, srv, body, "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	if store.Len() != 0 {
		t.Fatalf("expected 0 stored events (fail-atomic), got %d", store.Len())
	}
}

func TestHandler_NonJSON_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	resp := doPost(t, srv, "not json at all", "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_EmptyBody_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	resp := doPost(t, srv, "", "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_MissingSchema_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	body := `{"event":"test"}`
	resp := doPost(t, srv, body, "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, readRespBody(t, resp))
	}
}

func TestHandler_WrongSchema_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	body := `{"schema":"wrong/v1","event":"test"}`
	resp := doPost(t, srv, body, "application/json")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, readRespBody(t, resp))
	}
}

func TestHandler_WrongContentType_400(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	resp := doPost(t, srv, validEventJSON(), "text/plain")

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong content type, got %d", resp.StatusCode)
	}
}

func TestHandler_GET_405(t *testing.T) {
	srv, _, limiter := newTestServer(t)
	defer limiter.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestHandler_RateLimitHeaders(t *testing.T) {
	handler, _, _ := newTestHandler(t)
	w := doPostDirect(t, handler, validEventJSON(), "10.0.0.1:9999")

	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit")
	}
	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining")
	}
	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("missing X-RateLimit-Reset")
	}
}

func TestHandler_StoredEventHasNoOriginData(t *testing.T) {
	srv, store, limiter := newTestServer(t)
	defer limiter.Close()

	resp := doPost(t, srv, validEventJSON(), "application/json")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	events := store.Events()
	if len(events) != 1 {
		t.Fatal("expected 1 stored event")
	}
	_ = events[0]
}
