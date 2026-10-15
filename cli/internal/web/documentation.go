package web

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

// Collections allow other document sources to join the Wiki without inheriting
// its review lifecycle. State and findings are derived by the same service as CLI.
type documentationPage struct {
	wiki.Page
	State      string `json:"state"`
	StateLabel string `json:"state_label"`
}

type documentationCollection struct {
	ID        string               `json:"id"`
	Label     string               `json:"label"`
	Available bool                 `json:"available"`
	Items     []documentationPage  `json:"items"`
	Findings  []domain.WikiFinding `json:"findings,omitempty"`
}

func readWikiDocumentation(ws *workspaceSession) (documentationCollection, error) {
	collection := documentationCollection{ID: "wiki", Label: "Wiki", Items: []documentationPage{}}
	root := ws.cfg.Paths.Wiki
	if !filepath.IsAbs(root) {
		root = filepath.Join(ws.cfg.ProjectRoot, root)
	}
	_, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return collection, nil
	}
	if err != nil {
		return collection, iox.NewConflict("La Wiki non è leggibile", "Verifica il percorso configurato e il formato delle pagine Wiki.", err)
	}
	pages, states, report, err := wiki.Status(ws.cfg.ProjectRoot, root)
	if err != nil {
		return collection, iox.NewConflict("La Wiki non è leggibile", "Verifica il percorso configurato e il formato delle pagine Wiki.", err)
	}
	collection.Available = true
	collection.Findings = report.Findings
	labels := map[string]string{
		"generated": "Generata", "reviewed": "Revisionata", "stale": "Da revisionare",
		"evidence-changed": "Fonti cambiate", "attention": "Richiede attenzione",
	}
	for index, page := range pages {
		label := labels[states[index]]
		if label == "" {
			label = states[index]
		}
		collection.Items = append(collection.Items, documentationPage{Page: page, State: states[index], StateLabel: label})
	}
	return collection, nil
}

func (s *Server) handleGetDocumentation(w http.ResponseWriter, r *http.Request) {
	collection, err := readWikiDocumentation(s.session())
	if err != nil {
		writeError(w, err)
		return
	}
	items := []documentationPage{}
	for _, page := range collection.Items {
		if wiki.MatchesQuery(page.Page, r.URL.Query().Get("q")) {
			page.Body = ""
			items = append(items, page)
		}
	}
	collection.Items = items
	writeJSON(w, http.StatusOK, map[string]any{"collections": []documentationCollection{collection}})
}

func (s *Server) handleGetDocumentationPage(w http.ResponseWriter, r *http.Request) {
	// The caller selects an ID from the collection, never a filesystem path.
	if r.PathValue("collection") != "wiki" {
		writeError(w, iox.NewNotFound("Raccolta non trovata", "Scegli una raccolta disponibile.", nil))
		return
	}
	collection, err := readWikiDocumentation(s.session())
	if err != nil {
		writeError(w, err)
		return
	}
	for _, page := range collection.Items {
		if page.ID == r.URL.Query().Get("id") {
			writeJSON(w, http.StatusOK, page)
			return
		}
	}
	writeError(w, iox.NewNotFound("Pagina non trovata", "Scegli una pagina dall’indice della Wiki.", nil))
}
