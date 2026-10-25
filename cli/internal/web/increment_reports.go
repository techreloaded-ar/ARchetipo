package web

import (
	"net/http"
	"os"
	"path/filepath"
)

// Report links resolve through the increment's own checkout, never through the
// viewer's main checkout. Each open root confines symlinks and relative paths.
func (s *Server) handleIncrementReport(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := s.incrementService(ws).Read(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	reportsPath := filepath.Join(ws.cfg.Paths.TestResults, record.ID)
	if !filepath.IsLocal(reportsPath) || !filepath.IsLocal(r.PathValue("path")) {
		http.NotFound(w, r)
		return
	}
	checkout, err := os.OpenRoot(record.Workdir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = checkout.Close() }()
	reports, err := checkout.OpenRoot(reportsPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = reports.Close() }()
	file, err := reports.Open(r.PathValue("path"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	// Reports may contain HTML or tool output. Display original text inertly.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
