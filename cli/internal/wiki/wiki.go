// Package wiki implements the connector-independent living project Wiki.
package wiki

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

type Page struct {
	Meta domain.WikiPageMeta `json:"meta"`
	Path string              `json:"path"`
	Body string              `json:"body,omitempty"`
}

type Report struct {
	OK       bool                 `json:"ok"`
	Pages    int                  `json:"pages"`
	Findings []domain.WikiFinding `json:"findings"`
}

func Init(root string) ([]string, error) {
	created := []string{}
	for _, dir := range []string{"", "sources"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, err
		}
	}
	files := map[string]string{
		"index.md": "# Project Wiki\n\nThis catalog is maintained by `archetipo wiki publish`.\n",
		"log.md":   "# Wiki log\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := atomicWrite(path, []byte(body)); err != nil {
				return nil, err
			}
			created = append(created, filepath.ToSlash(path))
		} else if err != nil {
			return nil, err
		}
	}
	sort.Strings(created)
	return created, nil
}

func Load(root string, includeSources bool) ([]Page, error) {
	pages := []Page{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if !includeSources && path != root && filepath.Base(path) == "sources" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" || filepath.Base(path) == "index.md" || filepath.Base(path) == "log.md" {
			return nil
		}
		page, err := parsePage(root, path)
		if err != nil {
			return err
		}
		pages = append(pages, page)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Meta.ID < pages[j].Meta.ID })
	return pages, err
}

func parsePage(root, path string) (Page, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Page{}, err
	}
	parts := bytes.SplitN(raw, []byte("---\n"), 3)
	if len(parts) != 3 || len(bytes.TrimSpace(parts[0])) != 0 {
		return Page{}, fmt.Errorf("%s: missing YAML frontmatter", path)
	}
	var meta domain.WikiPageMeta
	if err := yaml.Unmarshal(parts[1], &meta); err != nil {
		return Page{}, fmt.Errorf("%s: %w", path, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return Page{}, err
	}
	return Page{Meta: meta, Path: filepath.ToSlash(rel), Body: string(parts[2])}, nil
}

func Validate(projectRoot, root string) Report {
	pages, err := Load(root, false)
	if err != nil {
		return Report{Findings: []domain.WikiFinding{{Code: "WIKI_UNREADABLE", Severity: "error", Path: root, Message: err.Error()}}}
	}
	findings := []domain.WikiFinding{}
	ids := map[string]Page{}
	allowed := map[domain.WikiStatus]bool{domain.WikiStatusDraft: true, domain.WikiStatusVerified: true, domain.WikiStatusNeedsReview: true, domain.WikiStatusSuperseded: true}
	for _, p := range pages {
		add := func(code, message string) {
			findings = append(findings, domain.WikiFinding{Code: code, Severity: "error", PageID: p.Meta.ID, Path: p.Path, Message: message})
		}
		if p.Meta.ID == "" {
			add("WIKI_MISSING_ID", "page id is required")
		} else if previous, ok := ids[p.Meta.ID]; ok {
			add("WIKI_DUPLICATE_ID", "page id is also used by "+previous.Path)
		} else {
			ids[p.Meta.ID] = p
		}
		if p.Meta.ID != "" {
			expected := canonicalPagePath(p.Meta.ID)
			if p.Path != expected {
				add("WIKI_NONCANONICAL_PATH", "page id "+p.Meta.ID+" must live at "+expected)
			}
		}
		if p.Meta.Type == "" {
			add("WIKI_MISSING_TYPE", "page type is required")
		}
		if p.Meta.Summary == "" {
			add("WIKI_MISSING_SUMMARY", "routing summary is required")
		}
		if !allowed[p.Meta.Status] {
			add("WIKI_INVALID_STATUS", "status must be draft, verified, needs-review, or superseded")
		}
		for _, source := range p.Meta.Sources {
			if source.Path == "" {
				add("WIKI_INVALID_SOURCE", "source path is required")
				continue
			}
			if !isExternal(source.Path) {
				candidate := source.Path
				if !filepath.IsAbs(candidate) {
					candidate = filepath.Join(projectRoot, filepath.FromSlash(candidate))
				}
				if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
					findings = append(findings, domain.WikiFinding{Code: "WIKI_STALE_SOURCE", Severity: "warning", PageID: p.Meta.ID, Path: p.Path, Message: "source does not exist: " + source.Path})
				}
			}
		}
		for _, coverage := range p.Meta.Coverage {
			if coverage.Path == "" {
				add("WIKI_INVALID_COVERAGE", "coverage path is required")
			}
			switch coverage.Status {
			case "documented":
				if len(coverage.Pages) == 0 {
					add("WIKI_INVALID_COVERAGE", "documented coverage requires at least one page id")
				}
			case "mapped-only", "needs-review", "excluded":
				if strings.TrimSpace(coverage.Note) == "" {
					add("WIKI_INVALID_COVERAGE", coverage.Status+" coverage requires a note")
				}
			default:
				add("WIKI_INVALID_COVERAGE", "coverage status must be documented, mapped-only, needs-review, or excluded")
			}
		}
	}
	for _, p := range pages {
		for _, link := range p.Meta.Links {
			if _, ok := ids[link.ID]; !ok {
				findings = append(findings, domain.WikiFinding{Code: "WIKI_BROKEN_LINK", Severity: "error", PageID: p.Meta.ID, Path: p.Path, Message: "linked page does not exist: " + link.ID})
			}
		}
		for _, match := range wikiLinkPattern.FindAllStringSubmatch(p.Body, -1) {
			target := strings.TrimSpace(strings.SplitN(match[1], "|", 2)[0])
			target = strings.SplitN(target, "#", 2)[0]
			if _, ok := ids[target]; !ok {
				findings = append(findings, domain.WikiFinding{Code: "WIKI_BROKEN_BODY_LINK", Severity: "error", PageID: p.Meta.ID, Path: p.Path, Message: "body link targets a missing page id: " + target})
			}
		}
		for _, coverage := range p.Meta.Coverage {
			for _, pageID := range coverage.Pages {
				if _, ok := ids[pageID]; !ok {
					findings = append(findings, domain.WikiFinding{Code: "WIKI_BROKEN_COVERAGE_PAGE", Severity: "error", PageID: p.Meta.ID, Path: p.Path, Message: "coverage references a missing page id: " + pageID})
				}
			}
		}
	}
	linked := map[string]bool{}
	for _, p := range pages {
		for _, l := range p.Meta.Links {
			linked[l.ID] = true
		}
	}
	for _, p := range pages {
		if len(pages) > 1 && !linked[p.Meta.ID] && len(p.Meta.Links) == 0 {
			findings = append(findings, domain.WikiFinding{Code: "WIKI_ORPHAN_PAGE", Severity: "warning", PageID: p.Meta.ID, Path: p.Path, Message: "page has no relationships"})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].Path < findings[j].Path
	})
	ok := true
	for _, f := range findings {
		if f.Severity == "error" {
			ok = false
		}
	}
	return Report{OK: ok, Pages: len(pages), Findings: findings}
}

// ValidateBootstrap adds codebase coverage requirements to structural validation.
func ValidateBootstrap(projectRoot, root, prdPath string) (Report, error) {
	report := Validate(projectRoot, root)
	pages, err := Load(root, false)
	if err != nil {
		return report, err
	}
	byID := map[string]Page{}
	for _, page := range pages {
		byID[page.Meta.ID] = page
	}
	add := func(code, pageID, path, message string) {
		report.Findings = append(report.Findings, domain.WikiFinding{Code: code, Severity: "error", PageID: pageID, Path: path, Message: message})
		report.OK = false
	}
	for _, id := range []string{"overview", "architecture", "engineering.code-map", "operations.development"} {
		page, ok := byID[id]
		if !ok {
			add("WIKI_BOOTSTRAP_PAGE_MISSING", id, canonicalPagePath(id), "bootstrap requires page "+id)
			continue
		}
		hasRepositoryEvidence := false
		for _, source := range page.Meta.Sources {
			if source.Path == "" || isExternal(source.Path) {
				continue
			}
			candidate := source.Path
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(projectRoot, filepath.FromSlash(candidate))
			}
			if _, statErr := os.Stat(candidate); statErr == nil {
				hasRepositoryEvidence = true
				break
			}
		}
		if !hasRepositoryEvidence {
			add("WIKI_BOOTSTRAP_SOURCE_MISSING", id, page.Path, "bootstrap core page requires repository evidence")
		}
	}
	inspection, err := Inspect(projectRoot, root, prdPath)
	if err != nil {
		return report, err
	}
	codeMap, ok := byID["engineering.code-map"]
	if ok {
		covered := map[string]bool{}
		known := map[string]bool{}
		for _, boundary := range inspection.Boundaries {
			known[boundary.Path] = true
		}
		for _, coverage := range codeMap.Meta.Coverage {
			covered[coverage.Path] = true
			if !known[coverage.Path] {
				report.Findings = append(report.Findings, domain.WikiFinding{Code: "WIKI_UNKNOWN_COVERAGE", Severity: "warning", PageID: codeMap.Meta.ID, Path: codeMap.Path, Message: "coverage path is not an inspected boundary: " + coverage.Path})
			}
		}
		for _, boundary := range inspection.Boundaries {
			if !covered[boundary.Path] {
				add("WIKI_UNCOVERED_BOUNDARY", codeMap.Meta.ID, codeMap.Path, "inspected boundary is not represented in coverage: "+boundary.Path)
			}
		}
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Path == report.Findings[j].Path {
			return report.Findings[i].Code < report.Findings[j].Code
		}
		return report.Findings[i].Path < report.Findings[j].Path
	})
	return report, nil
}

func canonicalPagePath(id string) string {
	return strings.ReplaceAll(id, ".", "/") + ".md"
}

func Search(root, query, pageType, status string, includeSources bool) ([]Page, error) {
	pages, err := Load(root, includeSources)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	result := []Page{}
	for _, p := range pages {
		if pageType != "" && p.Meta.Type != pageType {
			continue
		}
		if status != "" && string(p.Meta.Status) != status {
			continue
		}
		haystack := strings.ToLower(p.Meta.ID + " " + p.Meta.Summary + " " + p.Body)
		if q != "" && !strings.Contains(haystack, q) {
			continue
		}
		p.Body = ""
		result = append(result, p)
	}
	return result, nil
}

func Affected(projectRoot, root string, files []string) ([]Page, error) {
	pages, err := Load(root, false)
	if err != nil {
		return nil, err
	}
	result := []Page{}
	for _, p := range pages {
		matched := false
		for _, s := range p.Meta.Sources {
			for _, f := range files {
				clean := filepath.ToSlash(strings.TrimPrefix(f, "./"))
				src := filepath.ToSlash(strings.TrimPrefix(s.Path, "./"))
				if clean == src || strings.HasPrefix(clean, strings.TrimSuffix(src, "/")+"/") {
					matched = true
				}
			}
		}
		if matched {
			p.Body = ""
			result = append(result, p)
		}
	}
	_ = projectRoot
	return result, nil
}

func GitChangedFiles(projectRoot, base, head string) ([]string, error) {
	if base == "" {
		base = "HEAD~1"
	}
	if head == "" {
		head = "HEAD"
	}
	cmd := exec.Command("git", "diff", "--name-only", base+"..."+head)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(out))
	sort.Strings(fields)
	return fields, nil
}

func Publish(projectRoot, root string) (int, error) {
	report := Validate(projectRoot, root)
	if !report.OK {
		return 0, fmt.Errorf("wiki validation failed")
	}
	pages, err := Load(root, false)
	if err != nil {
		return 0, err
	}
	published := 0
	now := time.Now().UTC().Format(time.RFC3339)
	revision := gitRevision(projectRoot)
	for i := range pages {
		p := pages[i]
		if p.Meta.Status != domain.WikiStatusDraft {
			continue
		}
		p.Meta.Status = domain.WikiStatusVerified
		p.Meta.LastVerifiedAt = now
		p.Meta.GitRevision = revision
		raw, err := renderPage(p)
		if err != nil {
			return published, err
		}
		if err := atomicWrite(filepath.Join(root, filepath.FromSlash(p.Path)), raw); err != nil {
			return published, err
		}
		pages[i] = p
		published++
	}
	if err := writeIndex(root, pages); err != nil {
		return published, err
	}
	if published > 0 {
		if err := appendLog(root, fmt.Sprintf("published %d page(s) at `%s`", published, revision)); err != nil {
			return published, err
		}
	}
	return published, nil
}

// Catalog rebuilds navigation without changing page lifecycle state.
func Catalog(root string) (int, error) {
	pages, err := Load(root, false)
	if err != nil {
		return 0, err
	}
	if err := writeIndex(root, pages); err != nil {
		return 0, err
	}
	if err := appendLog(root, fmt.Sprintf("cataloged %d page(s) without promotion", len(pages))); err != nil {
		return 0, err
	}
	return len(pages), nil
}

func appendLog(root, action string) error {
	f, err := os.OpenFile(filepath.Join(root, "log.md"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "\n- %s: %s.\n", time.Now().UTC().Format(time.RFC3339), action)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func renderPage(p Page) ([]byte, error) {
	meta, err := yaml.Marshal(p.Meta)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(meta) + "---\n" + p.Body), nil
}
func writeIndex(root string, pages []Page) error {
	var b strings.Builder
	b.WriteString("# Project Wiki\n\n| ID | Type | Status | Summary | Path |\n|---|---|---|---|---|\n")
	for _, p := range pages {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | [%s](%s) |\n", p.Meta.ID, p.Meta.Type, p.Meta.Status, strings.ReplaceAll(p.Meta.Summary, "|", "\\|"), p.Path, p.Path)
	}
	return atomicWrite(filepath.Join(root, "index.md"), []byte(b.String()))
}
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".wiki-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func gitRevision(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func isExternal(path string) bool { return strings.Contains(path, "://") }
