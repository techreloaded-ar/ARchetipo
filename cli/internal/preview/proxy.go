package preview

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

//go:embed picker.js
var picker []byte

// The proxy has its own loopback origin: application scripts cannot read the
// viewer DOM or its same-origin API. Root paths and WebSockets remain intact.
func proxy(recipe Recipe, parentOrigin, token string) http.Handler {
	target, _ := url.Parse(recipe.URL)
	target.Path = ""
	target.RawQuery = ""
	p := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		request.SetURL(target)
		request.Out.Host = target.Host
		request.Out.Header.Set("Accept-Encoding", "identity")
	}}
	p.ModifyResponse = func(r *http.Response) error {
		if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "text/html") || r.StatusCode != 200 {
			return nil
		}
		originalBody := r.Body
		body, err := io.ReadAll(io.LimitReader(originalBody, 8<<20+1))
		if err != nil {
			_ = originalBody.Close()
			return err
		}
		if len(body) > 8<<20 {
			r.Body = &joinedBody{Reader: io.MultiReader(bytes.NewReader(body), originalBody), closer: originalBody}
			return nil
		}
		_ = originalBody.Close()
		script := []byte(`<script src="/__archetipo_picker.js?token=` + token + `"></script>`)
		lower := bytes.ToLower(body)
		at := bytes.Index(lower, []byte("</body>"))
		if at < 0 {
			at = len(body)
		}
		body = append(append(append([]byte{}, body[:at]...), script...), body[at:]...)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
		r.Header.Del("ETag")
		r.Header.Set("Cache-Control", "no-store")
		return nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__archetipo_picker.js" {
			if r.URL.Query().Get("token") != token {
				http.NotFound(w, r)
				return
			}
			config, _ := json.Marshal(map[string]string{"origin": parentOrigin, "token": token})
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte("(" + string(picker) + ")(" + string(config) + ");"))
			return
		}
		p.ServeHTTP(w, r)
	})
}

type joinedBody struct {
	io.Reader
	closer io.Closer
}

func (b *joinedBody) Close() error { return b.closer.Close() }
