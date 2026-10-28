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
		reportUnavailable(w)
		return
	}
	reportsPath := filepath.Join(ws.cfg.Paths.TestResults, record.ID)
	if !filepath.IsLocal(reportsPath) || !filepath.IsLocal(r.PathValue("path")) {
		reportUnavailable(w)
		return
	}
	checkout, err := os.OpenRoot(record.Workdir)
	if err != nil {
		reportUnavailable(w)
		return
	}
	defer func() { _ = checkout.Close() }()
	reports, err := checkout.OpenRoot(reportsPath)
	if err != nil {
		reportUnavailable(w)
		return
	}
	defer func() { _ = reports.Close() }()
	file, err := reports.Open(r.PathValue("path"))
	if err != nil {
		reportUnavailable(w)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		reportUnavailable(w)
		return
	}
	// Reports may contain HTML or tool output. Display original text inertly.
	reportHeaders(w)

	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
