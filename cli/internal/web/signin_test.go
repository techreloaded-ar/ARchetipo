package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/signin"
)

func TestSignInIsOfferedOnlyInTheIsolatedRuntime(t *testing.T) {
	t.Setenv(signin.EnvRuntime, "")
	srv, _ := newWorkspacesTestServer(t, t.TempDir())
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/signin", nil))
	var list struct {
		Available bool             `json:"available"`
		Accounts  []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /api/signin = %d %s", rec.Code, rec.Body)
	}
	if list.Available || list.Accounts == nil || len(list.Accounts) != 0 {
		t.Fatalf("sign-in offered outside the isolated runtime: %s", rec.Body)
	}
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/signin/claude", nil),
		httptest.NewRequest(http.MethodPost, "/api/signin/claude/code", strings.NewReader(`{"code":"x"}`)),
		httptest.NewRequest(http.MethodDelete, "/api/signin/claude", nil),
	} {
		rec = httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s %s outside the isolated runtime = %d", req.Method, req.URL.Path, rec.Code)
		}
	}

	t.Setenv(signin.EnvRuntime, "isolated")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/signin/nobody", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/signin/codex/code", strings.NewReader(`{"code":"x"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("code without a sign-in = %d", rec.Code)
	}
}
