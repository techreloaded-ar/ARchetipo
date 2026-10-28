package web

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
)

// Only report references already present in this workspace's conversation may
// be opened. IDs are stable across restarts; neither endpoint accepts a path.
var reportMarkdownLink = regexp.MustCompile(`\]\(<?([^\s<>]+)>?\)`)

type conversationReportLink struct {
	Href      string `json:"href"`
	URL       string `json:"url,omitempty"`
	Available bool   `json:"available"`
	path      string
	root      string
	scope     string
}

func conversationReportPath(root, results, href string) string {
	u, err := url.Parse(href)
	if err != nil || u.RawQuery != "" {
		return ""
	}
	if u.Scheme != "" && u.Scheme != "file" {
		return ""
	}
	if u.Host != "" {
		return ""
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	// Do not expose the environment runtime, configuration, credentials or
	// arbitrary JSON files. These two named artifacts are the technical reports.
	switch filepath.ToSlash(rel) {
	case ".archetipo/environment/technical-summary.json", ".archetipo/environment/technical-session.md":
		return rel
	}
	if !filepath.IsLocal(results) {
		return ""
	}
	inside, err := filepath.Rel(results, rel)
	if err != nil || !filepath.IsLocal(inside) {
		return ""
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".txt", ".log", ".json", ".html", ".csv":
		return rel
	}
	return ""
}

func (s *Server) conversationReportLinks(ws *workspaceSession, record conversationlog.Record) []conversationReportLink {
	root := record.WorkingDir
	if root == "" {
		root = ws.cfg.ProjectRoot
	}
	// The recorded conversation owns its checkout. An increment that has gone
	// away must never fall back to the main checkout's reports.
	var text strings.Builder
	for _, event := range record.Events {
		if event.Kind == "text" {
			text.WriteString(event.Text)
		}
		if event.Kind == "turn_end" {
			text.WriteString(event.FinalMessage)
		}
	}
	links := []conversationReportLink{}
	seen := map[string]bool{}
	for _, match := range reportMarkdownLink.FindAllStringSubmatch(text.String(), -1) {
		href := match[1]
		if seen[href] {
			continue
		}
		seen[href] = true
		rel := conversationReportPath(root, ws.cfg.Paths.TestResults, href)
		if rel == "" {
			continue
		}
		sum := sha256.Sum256([]byte(record.ID + "\x00" + filepath.ToSlash(rel)))
		link := conversationReportLink{Href: href, URL: fmt.Sprintf("/api/workspace/conversations/%s/reports/%x", record.ID, sum), path: rel, root: root}
		link.scope = ws.cfg.Paths.TestResults
		if strings.HasPrefix(filepath.ToSlash(rel), ".archetipo/environment/") {
			link.scope = filepath.Join(".archetipo", "environment")
		}
		link.path, _ = filepath.Rel(link.scope, rel)
		file, err := openConversationReport(link)
		if err == nil {
			link.Available = true
			_ = file.Close()
		}
		links = append(links, link)
	}
	return links
}

func openConversationReport(link conversationReportLink) (*os.File, error) {
	root, err := os.OpenRoot(link.root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	// OpenRoot confines symlinks within the report directory as well as checkout.
	for current := link.scope; current != "."; current = filepath.Dir(current) {
		info, err := root.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, os.ErrNotExist
		}
	}
	dir, err := root.OpenRoot(link.scope)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	for current := link.path; current != "."; current = filepath.Dir(current) {
		info, err := dir.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, os.ErrNotExist
		}
	}
	file, err := dir.Open(link.path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}

func (s *Server) handleConversationReports(w http.ResponseWriter, r *http.Request) {
	ws := s.session()
	record, err := s.readPastConversation(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		reportUnavailable(w)
		return
	}
	links := s.conversationReportLinks(ws, record)
	if r.PathValue("report") == "" {
		writeJSON(w, http.StatusOK, links)
		return
	}
	for _, link := range links {
		if link.URL != r.URL.Path {
			continue
		}
		file, err := openConversationReport(link)
		if err != nil {
			break
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			break
		}
		reportHeaders(w)
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
		return
	}
	reportUnavailable(w)
}

func reportHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func reportUnavailable(w http.ResponseWriter) {
	reportHeaders(w)
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Report non più disponibile o accesso non consentito. Torna alla conversazione per consultare gli altri risultati."))
}
