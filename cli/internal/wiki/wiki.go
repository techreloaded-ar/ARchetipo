// Package wiki implements the connector-independent living project Wiki.
package wiki

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
)

var wikiFrontmatterPattern = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---(?:\r?\n|$)`)
var markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+[^)]*)?\)`)
var nonSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)
var wikiSectionPattern = regexp.MustCompile(`<!--\s*archetipo:wiki\s+section=([a-z0-9-]+)\s*-->`)
var wikiProtocolArtifactPattern = regexp.MustCompile(`(?m)^\s*</?(?:content|invoke|tool_use|tool_result)>\s*$`)
var wikiBodyIssuesPattern = regexp.MustCompile(`(?m)^\s*<!--\s*archetipo:wiki\s+section=issues\s*-->`)
var wikiContentHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var wikiRevisionPattern = regexp.MustCompile(`^([0-9a-fA-F]{7,64}|unavailable)$`)
var wikiLogDatePattern = regexp.MustCompile(`^## \d{4}-\d{2}-\d{2}$`)
var wikiLogEntryPattern = regexp.MustCompile(`^\* \*\*(?:Review|Update)\*\*: .+\.$`)

var requiredPageSections = map[string][]string{
	"domain":                   {"purpose", "language", "ownership", "contracts", "flows", "code", "invariants", "verification"},
	"decision":                 {"context", "decision", "alternatives", "consequences", "verification"},
	"architecture/context-map": {"contexts", "relationships", "shared", "uncertainties"},
	"engineering/code-map":     {"domain-code", "shared", "unmapped", "coverage"},
}

var (
	ErrValidationFailed = errors.New("wiki validation failed")
	ErrUnresolvedIssues = errors.New("wiki page has unresolved issues")
	ErrMissingEvidence  = errors.New("wiki page has missing evidence")
	ErrPageNotFound     = errors.New("wiki page does not exist")
)

type Page struct {
	ID   string              `json:"id"`
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
	for _, dir := range []string{"", "references"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, err
		}
	}
	files := map[string]string{
		"index.md": "# Project Wiki\n",
		"log.md":   "# Wiki Update Log\n",
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

func Load(root string) ([]Page, error) {
	pages := []Page{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
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
	sort.Slice(pages, func(i, j int) bool { return pages[i].ID < pages[j].ID })
	return pages, err
}

func parsePage(root, path string) (Page, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Page{}, err
	}
	match := wikiFrontmatterPattern.FindSubmatchIndex(raw)
	if match == nil {
		return Page{}, fmt.Errorf("%s: missing YAML frontmatter", path)
	}
	var meta domain.WikiPageMeta
	if err := yaml.Unmarshal(raw[match[2]:match[3]], &meta); err != nil {
		return Page{}, fmt.Errorf("%s: %w", path, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return Page{}, err
	}
	rel = filepath.ToSlash(rel)
	return Page{ID: conceptID(rel), Meta: meta, Path: rel, Body: string(raw[match[1]:])}, nil
}

func conceptID(pagePath string) string {
	return strings.TrimSuffix(filepath.ToSlash(pagePath), ".md")
}

func markdownConceptLinks(page Page) []string {
	links := []string{}
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(page.Body, -1) {
		href := strings.Trim(match[1], "<>")
		if href == "" || strings.HasPrefix(href, "#") || isExternal(href) {
			continue
		}
		href = strings.SplitN(href, "#", 2)[0]
		href = strings.SplitN(href, "?", 2)[0]
		if !strings.HasSuffix(strings.ToLower(href), ".md") {
			continue
		}
		var target string
		if strings.HasPrefix(href, "/") {
			target = strings.TrimPrefix(pathpkg.Clean(href), "/")
		} else {
			target = pathpkg.Clean(pathpkg.Join(pathpkg.Dir(page.Path), href))
		}
		if target == ".." || strings.HasPrefix(target, "../") {
			continue
		}
		if target == "index.md" || target == "log.md" || strings.HasSuffix(target, "/index.md") || strings.HasSuffix(target, "/log.md") {
			continue
		}
		links = append(links, conceptID(target))
	}
	return uniqueSorted(links)
}

func referencePagePath(sourcePath string) string {
	base := strings.ToLower(filepath.Base(sourcePath))
	name := strings.TrimSuffix(base, filepath.Ext(base))
	slug := strings.Trim(nonSlugPattern.ReplaceAllString(name, "-"), "-")
	if slug == "" {
		slug = "reference"
	}
	return pathpkg.Join("references", slug+".md")
}

func Validate(projectRoot, root string) Report {
	pages, err := Load(root)
	if err != nil {
		return Report{Findings: []domain.WikiFinding{{Code: "WIKI_UNREADABLE", Severity: "error", Path: root, Message: err.Error()}}}
	}
	findings := validateWikiLog(root)
	ids := map[string]Page{}
	allowed := map[domain.WikiStatus]bool{domain.WikiStatusGenerated: true, domain.WikiStatusReviewed: true}
	for _, p := range pages {
		add := func(code, message string) {
			findings = append(findings, domain.WikiFinding{Code: code, Severity: "error", PageID: p.ID, Path: p.Path, Message: message})
		}
		ids[p.ID] = p
		if p.Meta.Type == "" {
			add("WIKI_MISSING_TYPE", "page type is required")
		}
		if strings.TrimSpace(p.Meta.Title) == "" {
			add("WIKI_MISSING_TITLE", "page title is required")
		}
		if strings.TrimSpace(p.Meta.Description) == "" {
			add("WIKI_MISSING_DESCRIPTION", "page description is required")
		}
		if p.Meta.Timestamp != "" {
			if _, err := time.Parse(time.RFC3339, p.Meta.Timestamp); err != nil {
				add("WIKI_INVALID_TIMESTAMP", "timestamp must be RFC3339")
			}
		}
		if !allowed[p.Meta.Status] {
			add("WIKI_INVALID_STATUS", "status must be generated or reviewed")
		}
		if p.Meta.Type == "domain" {
			switch p.Meta.Classification {
			case "candidate", "bounded-context":
			default:
				add("WIKI_INVALID_DOMAIN_CLASSIFICATION", "domain classification must be candidate or bounded-context")
			}
			if len(p.Meta.Sources) == 0 {
				add("WIKI_DOMAIN_SOURCE_MISSING", "domain pages require repository evidence")
			}
		}
		if p.Meta.Type == "decision" {
			switch p.Meta.DecisionStatus {
			case "accepted", "superseded":
			default:
				add("WIKI_INVALID_DECISION_STATUS", "decision_status must be accepted or superseded")
			}
			if len(p.Meta.Sources) == 0 {
				add("WIKI_DECISION_SOURCE_MISSING", "decision pages require repository evidence")
			}
		}
		for _, issue := range p.Meta.Issues {
			if strings.TrimSpace(issue.Code) == "" || strings.TrimSpace(issue.Summary) == "" {
				add("WIKI_INVALID_ISSUE", "issues require both code and summary")
			}
		}
		if p.Meta.Status == domain.WikiStatusReviewed {
			if p.Meta.Review == nil || p.Meta.Review.ContentHash == "" || p.Meta.Review.EvidenceRevision == "" || p.Meta.Review.ReviewedAt == "" {
				add("WIKI_REVIEW_METADATA_MISSING", "reviewed pages require content_hash, evidence_revision, and reviewed_at")
			} else {
				if !wikiContentHashPattern.MatchString(p.Meta.Review.ContentHash) || !wikiRevisionPattern.MatchString(p.Meta.Review.EvidenceRevision) {
					add("WIKI_REVIEW_METADATA_INVALID", "review metadata has an invalid content hash or evidence revision")
				}
				if _, err := time.Parse(time.RFC3339, p.Meta.Review.ReviewedAt); err != nil {
					add("WIKI_REVIEW_METADATA_INVALID", "reviewed_at must be RFC3339")
				}
				if p.Meta.Review.ContentHash != pageContentHash(p) {
					findings = append(findings, domain.WikiFinding{Code: "WIKI_REVIEW_OUTDATED", Severity: "warning", PageID: p.ID, Path: p.Path, Message: "page content changed after review"})
				}
				if pageEvidenceChanged(projectRoot, p) {
					findings = append(findings, domain.WikiFinding{Code: "WIKI_EVIDENCE_CHANGED", Severity: "warning", PageID: p.ID, Path: p.Path, Message: "cited repository evidence changed after review"})
				}
			}
		} else if p.Meta.Review != nil {
			add("WIKI_UNEXPECTED_REVIEW_METADATA", "generated pages must not carry review metadata")
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
					findings = append(findings, domain.WikiFinding{Code: "WIKI_STALE_SOURCE", Severity: "warning", PageID: p.ID, Path: p.Path, Message: "source does not exist: " + source.Path})
				}
			}
			if p.Meta.Type == "domain" && strings.TrimSpace(source.Role) == "" {
				add("WIKI_DOMAIN_SOURCE_ROLE_MISSING", "domain evidence requires a role")
			}
		}
		coverageKeys := map[string]bool{}
		for _, coverage := range p.Meta.Coverage {
			if coverage.Kind != "boundary" && coverage.Kind != "capability" {
				add("WIKI_INVALID_COVERAGE", "coverage kind must be boundary or capability")
			}
			if coverage.Path == "" {
				add("WIKI_INVALID_COVERAGE", "coverage path is required")
			}
			key := coverage.Kind + ":" + coverage.Path
			if coverageKeys[key] {
				add("WIKI_DUPLICATE_COVERAGE", "coverage item is duplicated: "+key)
			}
			coverageKeys[key] = true
			switch coverage.Status {
			case "mapped":
				if len(coverage.Pages) == 0 {
					add("WIKI_INVALID_COVERAGE", "mapped coverage requires at least one page id")
				}
			case "partial", "excluded":
				if strings.TrimSpace(coverage.Note) == "" {
					add("WIKI_INVALID_COVERAGE", coverage.Status+" coverage requires a note")
				}
			default:
				add("WIKI_INVALID_COVERAGE", "coverage status must be mapped, partial, or excluded")
			}
		}
		for _, section := range requiredSectionsForPage(p) {
			if !hasMeaningfulWikiSection(p.Body, section) {
				add("WIKI_DDD_SECTION_MISSING", "page requires section marker "+section)
			}
		}
		if artifact := wikiProtocolArtifactPattern.FindString(p.Body); artifact != "" {
			add("WIKI_PROTOCOL_ARTIFACT", "page body contains a model/tool protocol wrapper: "+strings.TrimSpace(artifact))
		}
		if wikiBodyIssuesPattern.MatchString(p.Body) {
			add("WIKI_BODY_ISSUES", "issues must be structured in frontmatter, not written as a body section")
		}
	}
	for _, p := range pages {
		for _, target := range markdownConceptLinks(p) {
			if _, ok := ids[target]; !ok {
				findings = append(findings, domain.WikiFinding{Code: "WIKI_BROKEN_LINK", Severity: "warning", PageID: p.ID, Path: p.Path, Message: "Markdown link targets a missing concept: " + target})
			}
		}
		for _, coverage := range p.Meta.Coverage {
			hasDomainPage := false
			for _, pageID := range coverage.Pages {
				target, ok := ids[pageID]
				if !ok {
					findings = append(findings, domain.WikiFinding{Code: "WIKI_BROKEN_COVERAGE_PAGE", Severity: "error", PageID: p.ID, Path: p.Path, Message: "coverage references a missing concept: " + pageID})
				} else if target.Meta.Type == "domain" {
					hasDomainPage = true
				}
			}
			if coverage.Kind == "capability" && coverage.Status == "mapped" && !hasDomainPage {
				findings = append(findings, domain.WikiFinding{Code: "WIKI_CAPABILITY_WITHOUT_DOMAIN", Severity: "error", PageID: p.ID, Path: p.Path, Message: "mapped capability requires at least one domain page: " + coverage.Path})
			}
		}
	}
	linked := map[string]bool{}
	for _, p := range pages {
		for _, target := range markdownConceptLinks(p) {
			linked[target] = true
		}
	}
	for _, p := range pages {
		if len(pages) > 1 && !linked[p.ID] && len(markdownConceptLinks(p)) == 0 {
			findings = append(findings, domain.WikiFinding{Code: "WIKI_ORPHAN_PAGE", Severity: "warning", PageID: p.ID, Path: p.Path, Message: "page has no relationships"})
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

func validateWikiLog(root string) []domain.WikiFinding {
	path := filepath.Join(root, "log.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return []domain.WikiFinding{{Code: "WIKI_LOG_UNREADABLE", Severity: "error", Path: "log.md", Message: err.Error()}}
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(raw)), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "# Wiki Update Log" {
		return []domain.WikiFinding{{Code: "WIKI_LOG_FORMAT", Severity: "error", Path: "log.md", Message: "log must start with # Wiki Update Log"}}
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" || wikiLogDatePattern.MatchString(line) || wikiLogEntryPattern.MatchString(line) {
			continue
		}
		return []domain.WikiFinding{{Code: "WIKI_LOG_FORMAT", Severity: "error", Path: "log.md", Message: "log entries must be grouped under ISO date headings and use Review or Update bullets"}}
	}
	return nil
}

// ValidateBootstrap adds codebase coverage requirements to structural validation.
func ValidateBootstrap(projectRoot, root, prdPath string) (Report, error) {
	report := Validate(projectRoot, root)
	pages, err := Load(root)
	if err != nil {
		return report, err
	}
	byID := map[string]Page{}
	for _, page := range pages {
		byID[page.ID] = page
		if page.Meta.Type == "domain" && page.Meta.Classification == "bounded-context" && page.Meta.Status != domain.WikiStatusReviewed {
			report.Findings = append(report.Findings, domain.WikiFinding{
				Code:     "WIKI_BOOTSTRAP_BOUNDARY_UNREVIEWED",
				Severity: "error",
				PageID:   page.ID,
				Path:     page.Path,
				Message:  "generated bootstrap domain pages must remain candidate until an explicit semantic review confirms the bounded-context boundary",
			})
			report.OK = false
		}
	}
	add := func(code, pageID, path, message string) {
		report.Findings = append(report.Findings, domain.WikiFinding{Code: code, Severity: "error", PageID: pageID, Path: path, Message: message})
		report.OK = false
	}
	corePages := map[string]string{"overview": "overview", "architecture/context-map": "context-map", "engineering/code-map": "code-map", "operations/development": "operations"}
	incoming := map[string]bool{}
	outgoing := map[string]bool{}
	for _, page := range pages {
		for _, target := range markdownConceptLinks(page) {
			if _, exists := byID[target]; !exists {
				continue
			}
			outgoing[page.ID] = true
			incoming[target] = true
		}
	}
	for _, id := range []string{"overview", "architecture/context-map", "engineering/code-map", "operations/development"} {
		page, ok := byID[id]
		if !ok {
			add("WIKI_BOOTSTRAP_PAGE_MISSING", id, id+".md", "bootstrap requires page "+id)
			continue
		}
		if page.Meta.Type != corePages[id] {
			add("WIKI_BOOTSTRAP_PAGE_TYPE", id, page.Path, "bootstrap page "+id+" must use type "+corePages[id])
		}
		if len(pages) > 1 && !incoming[id] && !outgoing[id] {
			add("WIKI_BOOTSTRAP_CORE_ORPHAN", id, page.Path, "bootstrap core page must link to or be linked from another Wiki concept")
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
	codeMap, ok := byID["engineering/code-map"]
	if ok {
		covered := map[string]bool{}
		known := map[string]bool{}
		for _, boundary := range inspection.Boundaries {
			known["boundary:"+boundary.Path] = true
		}
		for _, capability := range inspection.Capabilities {
			known["capability:"+capability.ID] = true
		}
		for _, coverage := range codeMap.Meta.Coverage {
			key := coverage.Kind + ":" + coverage.Path
			covered[key] = true
			if !known[key] {
				report.Findings = append(report.Findings, domain.WikiFinding{Code: "WIKI_UNKNOWN_COVERAGE", Severity: "warning", PageID: codeMap.ID, Path: codeMap.Path, Message: "coverage item was not returned by inspection: " + key})
			}
		}
		for _, boundary := range inspection.Boundaries {
			if !covered["boundary:"+boundary.Path] {
				add("WIKI_UNCOVERED_BOUNDARY", codeMap.ID, codeMap.Path, "inspected boundary is not represented in coverage: "+boundary.Path)
			}
		}
		for _, capability := range inspection.Capabilities {
			if !covered["capability:"+capability.ID] {
				add("WIKI_UNMAPPED_CAPABILITY", codeMap.ID, codeMap.Path, "capability candidate is not represented in coverage: "+capability.ID)
			}
		}
	}
	for _, source := range inspection.ProjectSources {
		expectedPath := referencePagePath(source.Path)
		reference, exists := byID[conceptID(expectedPath)]
		if !exists || reference.Meta.Type != "reference" || !pageCitesSource(reference, source.Path) {
			add("WIKI_PROJECT_REFERENCE_MISSING", conceptID(expectedPath), expectedPath, "configured project source was not represented as a reference concept: "+source.Path)
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

func pageCitesSource(page Page, sourcePath string) bool {
	for _, source := range page.Meta.Sources {
		if source.Path == sourcePath {
			return true
		}
	}
	return false
}

func Search(projectRoot, root, query, pageType, status string) ([]Page, error) {
	pages, err := Load(root)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	result := []Page{}
	for _, p := range pages {
		if pageType != "" && p.Meta.Type != pageType {
			continue
		}
		if status != "" && PageState(projectRoot, p) != status {
			continue
		}
		haystack := strings.ToLower(p.ID + " " + p.Meta.Title + " " + p.Meta.Description + " " + p.Body)
		if q != "" && !strings.Contains(haystack, q) {
			continue
		}
		p.Body = ""
		result = append(result, p)
	}
	return result, nil
}

func Affected(projectRoot, root string, files []string) ([]Page, error) {
	pages, err := Load(root)
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

// Approve marks selected generated pages as reviewed. An empty ID list approves
// every generated page, but pages with unresolved issues are never promoted.
func Approve(projectRoot, root string, ids []string) (int, error) {
	report := Validate(projectRoot, root)
	if !report.OK {
		return 0, ErrValidationFailed
	}
	pages, err := Load(root)
	if err != nil {
		return 0, err
	}
	approved := 0
	now := time.Now().UTC().Format(time.RFC3339)
	revision := gitRevision(projectRoot)
	if revision == "" {
		revision = "unavailable"
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	found := map[string]bool{}
	for _, page := range pages {
		if len(wanted) > 0 && !wanted[page.ID] {
			continue
		}
		found[page.ID] = true
		if page.Meta.Status == domain.WikiStatusGenerated && len(page.Meta.Issues) > 0 {
			return 0, fmt.Errorf("%w: %s", ErrUnresolvedIssues, page.ID)
		}
		if page.Meta.Status == domain.WikiStatusGenerated && pageHasMissingEvidence(projectRoot, page) {
			return 0, fmt.Errorf("%w: %s", ErrMissingEvidence, page.ID)
		}
	}
	for id := range wanted {
		if !found[id] {
			return 0, fmt.Errorf("%w: %s", ErrPageNotFound, id)
		}
	}
	for i := range pages {
		p := pages[i]
		if len(wanted) > 0 && !wanted[p.ID] {
			continue
		}
		if p.Meta.Status != domain.WikiStatusGenerated {
			continue
		}
		p.Meta.Status = domain.WikiStatusReviewed
		p.Meta.Review = &domain.WikiReview{ContentHash: pageContentHash(p), EvidenceRevision: revision, ReviewedAt: now}
		raw, err := renderPage(p)
		if err != nil {
			return approved, err
		}
		if err := atomicWrite(filepath.Join(root, filepath.FromSlash(p.Path)), raw); err != nil {
			return approved, err
		}
		pages[i] = p
		approved++
	}
	if err := writeIndex(projectRoot, root, pages); err != nil {
		return approved, err
	}
	if approved > 0 {
		if err := appendLog(root, "Review", fmt.Sprintf("Approved %d page(s) at `%s`", approved, revision)); err != nil {
			return approved, err
		}
	}
	return approved, nil
}

// Reset returns selected reviewed pages to generated before semantic updates.
func Reset(projectRoot, root string, ids []string) (int, error) {
	pages, err := Load(root)
	if err != nil {
		return 0, err
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	found := map[string]bool{}
	for _, page := range pages {
		if wanted[page.ID] {
			found[page.ID] = true
		}
	}
	for id := range wanted {
		if !found[id] {
			return 0, fmt.Errorf("%w: %s", ErrPageNotFound, id)
		}
	}
	reset := 0
	for index := range pages {
		page := pages[index]
		if !wanted[page.ID] {
			continue
		}
		if page.Meta.Status == domain.WikiStatusGenerated && page.Meta.Review == nil {
			continue
		}
		page.Meta.Status = domain.WikiStatusGenerated
		page.Meta.Review = nil
		raw, err := renderPage(page)
		if err != nil {
			return reset, err
		}
		if err := atomicWrite(filepath.Join(root, filepath.FromSlash(page.Path)), raw); err != nil {
			return reset, err
		}
		pages[index] = page
		reset++
	}
	if err := writeIndex(projectRoot, root, pages); err != nil {
		return reset, err
	}
	if reset > 0 {
		if err := appendLog(root, "Update", fmt.Sprintf("Reset %d page(s) to generated", reset)); err != nil {
			return reset, err
		}
	}
	return reset, nil
}

// Catalog rebuilds navigation without changing review state.
func Catalog(projectRoot, root string) (int, error) {
	pages, err := Load(root)
	if err != nil {
		return 0, err
	}
	if err := writeIndex(projectRoot, root, pages); err != nil {
		return 0, err
	}
	if err := appendLog(root, "Update", fmt.Sprintf("Cataloged %d page(s) without review changes", len(pages))); err != nil {
		return 0, err
	}
	return len(pages), nil
}

// RefreshCatalog rebuilds index.md without appending an Update entry to the
// Wiki log. Connector-managed pages call this after operational writes so new
// specs and plans become navigable without turning every task transition into
// an editorial Wiki event.
func RefreshCatalog(projectRoot, root string) error {
	pages, err := Load(root)
	if err != nil {
		return err
	}
	return writeIndex(projectRoot, root, pages)
}

func appendLog(root, kind, action string) error {
	logPath := filepath.Join(root, "log.md")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(string(raw))
	const header = "# Wiki Update Log"
	if content == "" {
		content = header
	}
	today := time.Now().UTC().Format(time.DateOnly)
	marker := "## " + today
	entry := fmt.Sprintf("* **%s**: %s.", kind, strings.TrimSuffix(strings.TrimSpace(action), "."))
	if index := strings.Index(content, marker); index >= 0 {
		insertAt := index + len(marker)
		content = content[:insertAt] + "\n\n" + entry + content[insertAt:]
	} else {
		rest := strings.TrimSpace(strings.TrimPrefix(content, header))
		content = header + "\n\n" + marker + "\n\n" + entry
		if rest != "" {
			content += "\n\n" + rest
		}
	}
	return atomicWrite(logPath, []byte(content+"\n"))
}

func renderPage(p Page) ([]byte, error) {
	meta, err := yaml.Marshal(p.Meta)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(meta) + "---\n" + p.Body), nil
}
func writeIndex(projectRoot, root string, pages []Page) error {
	var b strings.Builder
	b.WriteString("# Project Wiki\n")
	groups := map[string][]Page{}
	for _, p := range pages {
		segment := strings.SplitN(p.ID, "/", 2)[0]
		if !strings.Contains(p.ID, "/") {
			segment = "project"
		}
		groups[segment] = append(groups[segment], p)
	}
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)
	for _, group := range groupNames {
		fmt.Fprintf(&b, "\n## %s\n\n", displayGroupName(group))
		for _, p := range groups[group] {
			title := strings.ReplaceAll(p.Meta.Title, "]", "\\]")
			description := strings.TrimSuffix(strings.TrimSpace(p.Meta.Description), ".")
			fmt.Fprintf(&b, "* [%s](%s) - %s. _State: %s._\n", title, p.Path, description, PageState(projectRoot, p))
		}
	}
	return atomicWrite(filepath.Join(root, "index.md"), []byte(b.String()))
}

func displayGroupName(group string) string {
	words := strings.FieldsFunc(group, func(r rune) bool { return r == '-' || r == '_' })
	for index := range words {
		if words[index] != "" {
			words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
		}
	}
	return strings.Join(words, " ")
}

func requiredSectionsForPage(page Page) []string {
	if sections, ok := requiredPageSections[page.ID]; ok {
		return sections
	}
	return requiredPageSections[page.Meta.Type]
}

func hasMeaningfulWikiSection(body, section string) bool {
	matches := wikiSectionPattern.FindAllStringSubmatchIndex(body, -1)
	for index, match := range matches {
		if body[match[2]:match[3]] != section {
			continue
		}
		end := len(body)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		for _, line := range strings.Split(body[match[1]:end], "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "<!--") && !strings.HasPrefix(line, "#") {
				return true
			}
		}
		return false
	}
	return false
}

func pageContentHash(page Page) string {
	meta := page.Meta
	// Lifecycle metadata is produced by approval itself. Every other metadata
	// field is semantic page content and must invalidate an earlier review.
	meta.Status = ""
	meta.Review = nil
	encoded, err := yaml.Marshal(meta)
	if err != nil {
		encoded = []byte(fmt.Sprintf("%#v", meta))
	}
	semantic := append([]byte(page.ID+"\n"), encoded...)
	semantic = append(semantic, []byte(strings.TrimSpace(page.Body))...)
	digest := sha256.Sum256(semantic)
	return "sha256:" + fmt.Sprintf("%x", digest)
}

// PageState derives operational trust from review metadata, issues, content,
// and evidence changes. Only generated/reviewed are persisted in the page.
func PageState(projectRoot string, page Page) string {
	if len(page.Meta.Issues) > 0 {
		return "attention"
	}
	if page.Meta.Status != domain.WikiStatusReviewed {
		return "generated"
	}
	if page.Meta.Review == nil || page.Meta.Review.ContentHash != pageContentHash(page) {
		return "stale"
	}
	if pageEvidenceChanged(projectRoot, page) {
		return "stale"
	}
	return "reviewed"
}

func pageEvidenceChanged(projectRoot string, page Page) bool {
	if page.Meta.Review == nil {
		return false
	}
	if page.Meta.Review.EvidenceRevision == "unavailable" {
		return false
	}
	paths := []string{}
	for _, source := range page.Meta.Sources {
		if source.Path == "" || isExternal(source.Path) || filepath.IsAbs(source.Path) {
			continue
		}
		paths = append(paths, filepath.FromSlash(source.Path))
	}
	if len(paths) == 0 {
		return false
	}
	args := append([]string{"diff", "--quiet", page.Meta.Review.EvidenceRevision, "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = projectRoot
	return cmd.Run() != nil
}

func pageHasMissingEvidence(projectRoot string, page Page) bool {
	for _, source := range page.Meta.Sources {
		if source.Path == "" || isExternal(source.Path) {
			continue
		}
		candidate := source.Path
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(projectRoot, filepath.FromSlash(candidate))
		}
		if _, err := os.Stat(candidate); err != nil {
			return true
		}
	}
	return false
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
