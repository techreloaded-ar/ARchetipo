package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPreviewCannotSubmitViewerCommands(t *testing.T) {
	handler := viewerOriginGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for _, origin := range []string{"http://127.0.0.1:3000", "null", "http://evil.test", "", "http://viewer.local"} {
		req := httptest.NewRequest(http.MethodPost, "http://viewer.local/api/workspace/conversations/c/messages", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		want := http.StatusForbidden
		if origin == "" || origin == "http://viewer.local" {
			want = 204
		}
		if w.Code != want {
			t.Fatalf("origin %q: got %d want %d", origin, w.Code, want)
		}
	}
}
