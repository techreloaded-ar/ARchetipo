package web

import "net/http"

// Application code in a preview runs on a separate origin. It may navigate and
// submit its own forms, but cannot submit commands to the viewer. Non-browser
// clients without an Origin retain the existing local API contract.
func viewerOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			expected := "http://" + r.Host
			if r.TLS != nil {
				expected = "https://" + r.Host
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != expected {
				http.Error(w, "cross-origin viewer command refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
