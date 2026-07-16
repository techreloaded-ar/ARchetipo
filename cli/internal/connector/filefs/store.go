package filefs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

const (
	backlogSchema     = "archetipo/backlog/v2"
	specSchema        = "archetipo/spec/v2"
	planSchema        = "archetipo/plan/v2"
	wikiBacklogSchema = "archetipo/backlog-wiki/v1"
	wikiSpecSchema    = "archetipo/spec-wiki/v1"
	wikiPlanSchema    = "archetipo/plan-wiki/v1"
)

const (
	specBodyMarker  = "<!-- archetipo:spec-body -->"
	specLinksMarker = "<!-- archetipo:spec-links -->"
	planBodyMarker  = "<!-- archetipo:plan-body -->"
	planTasksMarker = "<!-- archetipo:plan-tasks -->"
)

type backlogDoc struct {
	Schema  string        `yaml:"schema"`
	Version int           `yaml:"version"`
	Epics   []domain.Epic `yaml:"epics,omitempty"`
	Order   []string      `yaml:"order"`
}

type boardColumnDoc struct {
	ID     string
	Title  string
	Status domain.Status
}

type specDoc struct {
	Schema    string                `yaml:"schema"`
	Code      string                `yaml:"code"`
	Title     string                `yaml:"title"`
	Epic      specEpicDoc           `yaml:"epic,omitempty"`
	Priority  domain.Priority       `yaml:"priority"`
	Points    int                   `yaml:"points"`
	Status    domain.Status         `yaml:"status"`
	BlockedBy []string              `yaml:"blocked_by,omitempty"`
	Scope     domain.Scope          `yaml:"scope,omitempty"`
	Body      string                `yaml:"body,omitempty"`
	Ref       string                `yaml:"ref,omitempty"`
	URL       string                `yaml:"url,omitempty"`
	Branch    string                `yaml:"branch,omitempty"`
	Worktree  string                `yaml:"worktree,omitempty"`
	ForkBase  string                `yaml:"fork_base,omitempty"`
	Rework    bool                  `yaml:"rework,omitempty"`
	History   []domain.StatusChange `yaml:"history,omitempty"`
}

// specEpicDoc is the on-disk representation of a spec's epic. It accepts
// the legacy scalar form (`epic: EP-001`) on read and always writes the full
// mapping form (`epic: {code, title}`) so each spec file is self-contained.
type specEpicDoc struct {
	Code  string `yaml:"code"`
	Title string `yaml:"title,omitempty"`
}

func (e *specEpicDoc) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		e.Code = node.Value
		e.Title = ""
		return nil
	}
	type rawEpic specEpicDoc
	var raw rawEpic
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*e = specEpicDoc(raw)
	return nil
}

func (e specEpicDoc) IsZero() bool {
	return e.Code == "" && e.Title == ""
}

type planDoc struct {
	Schema   string        `yaml:"schema"`
	SpecCode string        `yaml:"spec_code"`
	Body     string        `yaml:"body"`
	Tasks    []domain.Task `yaml:"tasks"`
}

// The file connector persists its canonical state as ordinary Wiki pages.
// Knowledge lifecycle (status/review) and delivery workflow (workflow_status)
// intentionally remain separate so that a spec can move through the board
// without overloading Wiki review semantics.
type wikiBacklogMeta struct {
	Type        string            `yaml:"type"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Status      domain.WikiStatus `yaml:"status"`
	Schema      string            `yaml:"schema"`
	Version     int               `yaml:"version"`
	Epics       []domain.Epic     `yaml:"epics,omitempty"`
	Order       []string          `yaml:"order"`
}

type wikiSpecMeta struct {
	Type           string                `yaml:"type"`
	Title          string                `yaml:"title"`
	Description    string                `yaml:"description"`
	Status         domain.WikiStatus     `yaml:"status"`
	Schema         string                `yaml:"schema"`
	Code           string                `yaml:"code"`
	Epic           specEpicDoc           `yaml:"epic,omitempty"`
	Priority       domain.Priority       `yaml:"priority"`
	Points         int                   `yaml:"points"`
	WorkflowStatus domain.Status         `yaml:"workflow_status"`
	BlockedBy      []string              `yaml:"blocked_by,omitempty"`
	Scope          domain.Scope          `yaml:"scope,omitempty"`
	Branch         string                `yaml:"branch,omitempty"`
	Worktree       string                `yaml:"worktree,omitempty"`
	ForkBase       string                `yaml:"fork_base,omitempty"`
	Rework         bool                  `yaml:"rework,omitempty"`
	History        []domain.StatusChange `yaml:"history,omitempty"`
}

type wikiPlanMeta struct {
	Type        string            `yaml:"type"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Status      domain.WikiStatus `yaml:"status"`
	Schema      string            `yaml:"schema"`
	SpecCode    string            `yaml:"spec_code"`
	Tasks       []domain.Task     `yaml:"tasks"`
}

type yamlStore struct {
	Backlog backlogDoc
	Specs   map[string]domain.Spec
}

func (c *Connector) backlogPath() string {
	return filepath.Join(c.wikiBacklogDir(), "overview.md")
}

func (c *Connector) specsDir() string {
	return filepath.Join(c.wikiBacklogDir(), "specs")
}

func (c *Connector) planPath(specRef string) string {
	return filepath.Join(c.wikiBacklogDir(), "plans", specRef+".md")
}

func (c *Connector) specPath(specCode string) string {
	return filepath.Join(c.specsDir(), specCode+".md")
}

func (c *Connector) wikiRoot() string       { return c.cfg.AbsPath(c.cfg.Paths.Wiki) }
func (c *Connector) wikiBacklogDir() string { return filepath.Join(c.wikiRoot(), "backlog") }

func (c *Connector) legacyYAMLBacklogPath() string { return c.cfg.AbsPath(c.cfg.File.Backlog) }
func (c *Connector) legacyYAMLSpecsDir() string {
	return filepath.Join(filepath.Dir(c.legacyYAMLBacklogPath()), "specs")
}
func (c *Connector) legacyYAMLPlanPath(specCode string) string {
	return filepath.Join(c.cfg.AbsPath(c.cfg.File.Planning), specCode+"-plan.yaml")
}

func (c *Connector) loadStore() (yamlStore, error) {
	store, err := c.loadWikiStore()
	if err == nil {
		return store, nil
	}
	var ce *iox.CodedError
	if !errors.As(err, &ce) || ce.Code != iox.CodePreconditionMissing {
		return yamlStore{}, err
	}
	return c.loadYAMLStore()
}

func (c *Connector) loadWikiStore() (yamlStore, error) {
	path := c.backlogPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return yamlStore{}, iox.NewPrecondition(
				fmt.Sprintf("Wiki backlog not found at %s", path),
				"run `archetipo spec add` or `archetipo-spec` first", errBacklogMissing,
			)
		}
		return yamlStore{}, fmt.Errorf("reading Wiki backlog: %w", err)
	}
	frontmatter, _, err := splitWikiPage(raw)
	if err != nil {
		return yamlStore{}, iox.NewInvalidInput("invalid Wiki backlog page", "check docs/wiki/backlog/overview.md", err)
	}
	var meta wikiBacklogMeta
	if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
		return yamlStore{}, iox.NewInvalidInput("invalid Wiki backlog frontmatter", "check docs/wiki/backlog/overview.md", err)
	}
	if meta.Schema != wikiBacklogSchema {
		return yamlStore{}, iox.NewInvalidInput("unsupported Wiki backlog schema", "expected "+wikiBacklogSchema, nil)
	}
	backlog := backlogDoc{Schema: backlogSchema, Version: 2, Epics: meta.Epics, Order: meta.Order}
	specs, err := c.readWikiSpecDocs(backlog.Epics)
	if err != nil {
		return yamlStore{}, err
	}
	backlog = c.normalizeBacklog(backlog, specs)
	return yamlStore{Backlog: backlog, Specs: specs}, nil
}

func (c *Connector) readWikiSpecDocs(epics []domain.Epic) (map[string]domain.Spec, error) {
	dir := c.specsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]domain.Spec{}, nil
		}
		return nil, fmt.Errorf("reading specs dir: %w", err)
	}
	epicTitles := make(map[string]string, len(epics))
	for _, epic := range epics {
		if epic.Code == "" {
			continue
		}
		epicTitles[epic.Code] = epic.Title
	}
	out := make(map[string]domain.Spec, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading spec file %s: %w", path, err)
		}
		frontmatter, body, err := splitWikiPage(raw)
		if err != nil {
			return nil, iox.NewInvalidInput(fmt.Sprintf("invalid spec Wiki page at %s", path), "", err)
		}
		var meta wikiSpecMeta
		if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
			return nil, iox.NewInvalidInput(fmt.Sprintf("invalid spec Wiki frontmatter at %s", path), "", err)
		}
		if meta.Schema != wikiSpecSchema {
			return nil, iox.NewInvalidInput(
				fmt.Sprintf("unsupported spec Wiki schema at %s", path),
				"expected "+wikiSpecSchema, nil,
			)
		}
		fileCode := strings.TrimSuffix(entry.Name(), ".md")
		if meta.Code != "" && meta.Code != fileCode {
			return nil, iox.NewInvalidInput(
				fmt.Sprintf("spec Wiki code %s does not match file %s", meta.Code, entry.Name()),
				"rename the file or restore its managed code frontmatter", nil,
			)
		}
		sp := domain.Spec{
			Code: meta.Code, Title: strings.TrimPrefix(meta.Title, meta.Code+": "),
			Epic:     domain.Epic{Code: meta.Epic.Code, Title: meta.Epic.Title},
			Priority: meta.Priority, Points: meta.Points, Status: meta.WorkflowStatus,
			BlockedBy: meta.BlockedBy, Scope: meta.Scope, Branch: meta.Branch,
			Worktree: meta.Worktree, ForkBase: meta.ForkBase, Rework: meta.Rework,
			History: meta.History, Body: markerContent(body, specBodyMarker, specLinksMarker),
		}
		if sp.Code == "" {
			sp.Code = fileCode
		}
		if sp.Epic.Code != "" && sp.Epic.Title == "" {
			sp.Epic.Title = epicTitles[sp.Epic.Code]
		}
		if sp.Ref == "" {
			sp.Ref = sp.Code
		}
		out[sp.Code] = sp
	}
	return out, nil
}

func (c *Connector) loadYAMLStore() (yamlStore, error) {
	path := c.legacyYAMLBacklogPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.loadLegacyStore()
		}
		return yamlStore{}, fmt.Errorf("reading legacy backlog store: %w", err)
	}
	var backlog backlogDoc
	if err := yaml.Unmarshal(raw, &backlog); err != nil {
		return yamlStore{}, iox.NewInvalidInput("invalid legacy backlog YAML", "check "+path, err)
	}
	specs, err := c.readYAMLSpecDocs(backlog.Epics)
	if err != nil {
		return yamlStore{}, err
	}
	backlog = c.normalizeBacklog(backlog, specs)
	return yamlStore{Backlog: backlog, Specs: specs}, nil
}

func (c *Connector) readYAMLSpecDocs(epics []domain.Epic) (map[string]domain.Spec, error) {
	dir := c.legacyYAMLSpecsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]domain.Spec{}, nil
		}
		return nil, fmt.Errorf("reading legacy specs dir: %w", err)
	}
	epicTitles := make(map[string]string, len(epics))
	for _, epic := range epics {
		epicTitles[epic.Code] = epic.Title
	}
	out := make(map[string]domain.Spec, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var doc specDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, iox.NewInvalidInput("invalid legacy spec YAML at "+path, "", err)
		}
		sp := doc.toSpec()
		if sp.Code == "" {
			sp.Code = strings.TrimSuffix(entry.Name(), ".yaml")
		}
		if sp.Epic.Code != "" && sp.Epic.Title == "" {
			sp.Epic.Title = epicTitles[sp.Epic.Code]
		}
		if sp.Ref == "" {
			sp.Ref = sp.Code
		}
		out[sp.Code] = sp
	}
	return out, nil
}

func (c *Connector) readPlan(specCode string) (planDoc, error) {
	path := c.planPath(specCode)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.readYAMLPlan(specCode)
		}
		return planDoc{}, fmt.Errorf("reading Wiki plan page: %w", err)
	}
	frontmatter, body, err := splitWikiPage(raw)
	if err != nil {
		return planDoc{}, iox.NewInvalidInput("invalid Wiki plan page at "+path, "", err)
	}
	var meta wikiPlanMeta
	if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
		return planDoc{}, iox.NewInvalidInput(fmt.Sprintf("invalid Wiki plan frontmatter at %s", path), "", err)
	}
	if meta.Schema != wikiPlanSchema {
		return planDoc{}, iox.NewInvalidInput("unsupported Wiki plan schema", "expected "+wikiPlanSchema, nil)
	}
	doc := planDoc{Schema: planSchema, SpecCode: meta.SpecCode, Body: markerContent(body, planBodyMarker, planTasksMarker), Tasks: meta.Tasks}
	if doc.SpecCode == "" {
		doc.SpecCode = specCode
	}
	for i := range doc.Tasks {
		if doc.Tasks[i].Ref == "" {
			doc.Tasks[i].Ref = doc.Tasks[i].ID
		}
	}
	domain.NormalizeTaskBodies(doc.Tasks)
	return doc, nil
}

func (c *Connector) readYAMLPlan(specCode string) (planDoc, error) {
	path := c.legacyYAMLPlanPath(specCode)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.readLegacyPlan(specCode)
		}
		return planDoc{}, fmt.Errorf("reading legacy plan YAML: %w", err)
	}
	var doc planDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return planDoc{}, iox.NewInvalidInput("invalid legacy plan YAML at "+path, "", err)
	}
	if doc.SpecCode == "" {
		doc.SpecCode = specCode
	}
	for i := range doc.Tasks {
		if doc.Tasks[i].Ref == "" {
			doc.Tasks[i].Ref = doc.Tasks[i].ID
		}
	}
	domain.NormalizeTaskBodies(doc.Tasks)
	return doc, nil
}

func (c *Connector) readLegacyPlan(specCode string) (planDoc, error) {
	legacyPath := filepath.Join(c.legacyPlanningDir(), specCode+".md")
	raw, err := os.ReadFile(legacyPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return planDoc{}, iox.NewPrecondition(
				fmt.Sprintf("planning file for %s not found", specCode),
				"run `archetipo spec plan` first", err,
			)
		}
		return planDoc{}, fmt.Errorf("reading legacy plan: %w", err)
	}
	body, tasks, err := parsePlan(string(raw))
	if err != nil {
		return planDoc{}, err
	}
	for i := range tasks {
		tasks[i].Ref = tasks[i].ID
	}
	return planDoc{
		Schema:   planSchema,
		SpecCode: specCode,
		Body:     body,
		Tasks:    tasks,
	}, nil
}

func (c *Connector) loadLegacyStore() (yamlStore, error) {
	legacyPath := c.legacyBacklogPath()
	raw, err := os.ReadFile(legacyPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return yamlStore{}, iox.NewPrecondition(
				fmt.Sprintf("backlog not found at %s", c.backlogPath()),
				"run `archetipo spec add` or `archetipo-spec` first", errBacklogMissing,
			)
		}
		return yamlStore{}, fmt.Errorf("reading legacy backlog: %w", err)
	}
	specs, err := parseBacklog(string(raw))
	if err != nil {
		return yamlStore{}, err
	}
	out := make(map[string]domain.Spec, len(specs))
	for _, spec := range specs {
		spec.Ref = spec.Code
		out[spec.Code] = spec
	}
	backlog := c.normalizeBacklog(backlogDoc{
		Schema:  backlogSchema,
		Version: 2,
		Order:   []string{},
	}, out)
	return yamlStore{Backlog: backlog, Specs: out}, nil
}

func (c *Connector) legacyBacklogPath() string {
	if strings.HasSuffix(strings.ToLower(c.cfg.File.Backlog), ".md") {
		return c.cfg.AbsPath(c.cfg.File.Backlog)
	}
	return filepath.Join(c.cfg.ProjectRoot, "docs", "BACKLOG.md")
}

func (c *Connector) legacyPlanningDir() string {
	if strings.HasSuffix(strings.ToLower(c.cfg.File.Planning), ".md") {
		return filepath.Dir(c.cfg.AbsPath(c.cfg.File.Planning))
	}
	if strings.HasSuffix(strings.ToLower(c.cfg.File.Planning), "/") || !strings.Contains(filepath.Base(c.cfg.File.Planning), ".") {
		path := c.cfg.AbsPath(c.cfg.File.Planning)
		if strings.HasSuffix(strings.ToLower(path), ".yaml") {
			return filepath.Join(c.cfg.ProjectRoot, "docs", "planning")
		}
		return path
	}
	return filepath.Join(c.cfg.ProjectRoot, "docs", "planning")
}

func (c *Connector) writeStore(store yamlStore) error {
	store.Backlog = c.normalizeBacklog(store.Backlog, store.Specs)
	if err := c.ensureWiki(); err != nil {
		return err
	}
	if err := c.preflightWikiCatalog(); err != nil {
		return err
	}
	if err := writeWikiPage(c.backlogPath(), wikiBacklogMeta{
		Type: "backlog", Title: "Backlog", Description: "Delivery backlog and canonical specification index",
		Status: domain.WikiStatusGenerated, Schema: wikiBacklogSchema, Version: 1,
		Epics: store.Backlog.Epics, Order: store.Backlog.Order,
	}, renderBacklogWikiBody(store)); err != nil {
		return err
	}
	for _, code := range store.Backlog.Order {
		spec, ok := store.Specs[code]
		if !ok {
			continue
		}
		if err := writeWikiPage(c.specPath(code), wikiSpecMetaFromSpec(spec), renderSpecWikiBody(spec)); err != nil {
			return err
		}
	}
	if err := c.migrateLegacyPlans(store); err != nil {
		return err
	}
	if err := wiki.RefreshCatalog(c.cfg.ProjectRoot, c.wikiRoot()); err != nil {
		return err
	}
	return c.cleanupLegacyArtifacts(store)
}

func (c *Connector) ensureWiki() error {
	_, err := wiki.Init(c.wikiRoot())
	return err
}

// preflightWikiCatalog verifies that all existing Wiki pages are readable
// before connector-managed files are changed. RefreshCatalog performs the same
// load after writes; doing it first prevents malformed unrelated pages from
// causing a command to report failure after its primary state already changed.
func (c *Connector) preflightWikiCatalog() error {
	_, err := wiki.Load(c.wikiRoot())
	if err != nil {
		return iox.NewInvalidInput("cannot refresh Wiki catalog", "fix malformed Wiki pages before changing the backlog", err)
	}
	return nil
}

func wikiSpecMetaFromSpec(spec domain.Spec) wikiSpecMeta {
	return wikiSpecMeta{
		Type: "spec", Title: spec.Code + ": " + spec.Title,
		Description: "Delivery specification " + spec.Code,
		Status:      domain.WikiStatusGenerated, Schema: wikiSpecSchema,
		Code: spec.Code, Epic: specEpicDoc{Code: spec.Epic.Code, Title: spec.Epic.Title},
		Priority: spec.Priority, Points: spec.Points, WorkflowStatus: spec.Status,
		BlockedBy: spec.BlockedBy, Scope: spec.Scope, Branch: spec.Branch,
		Worktree: spec.Worktree, ForkBase: spec.ForkBase, Rework: spec.Rework,
		History: spec.History,
	}
}

func renderBacklogWikiBody(store yamlStore) string {
	var b strings.Builder
	b.WriteString("# Backlog\n\n")
	b.WriteString("This page is the canonical delivery index managed by `archetipo`.\n")
	if len(store.Backlog.Order) == 0 {
		return b.String()
	}
	linked := map[string]bool{}
	for _, epic := range store.Backlog.Epics {
		fmt.Fprintf(&b, "\n## %s: %s\n\n", epic.Code, epic.Title)
		for _, code := range store.Backlog.Order {
			spec, ok := store.Specs[code]
			if !ok || spec.Epic.Code != epic.Code {
				continue
			}
			linked[code] = true
			fmt.Fprintf(&b, "- [%s: %s](specs/%s.md) — **%s**, %d point(s)", spec.Code, spec.Title, spec.Code, spec.Status, spec.Points)
			b.WriteString(".\n")
		}
	}
	if len(linked) != len(store.Specs) {
		b.WriteString("\n## Unassigned\n\n")
		for _, code := range store.Backlog.Order {
			if linked[code] {
				continue
			}
			spec, ok := store.Specs[code]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "- [%s: %s](specs/%s.md) — **%s**, %d point(s).\n", spec.Code, spec.Title, spec.Code, spec.Status, spec.Points)
		}
	}
	return b.String()
}

func renderSpecWikiBody(spec domain.Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\n%s\n\n", spec.Code, spec.Title, specBodyMarker)
	if body := strings.TrimSpace(spec.Body); body != "" {
		b.WriteString(body + "\n\n")
	}
	b.WriteString(specLinksMarker + "\n\n## Related\n\n")
	b.WriteString("- [Backlog](../overview.md)\n")
	return b.String()
}

func (d specDoc) toSpec() domain.Spec {
	return domain.Spec{
		Code:      d.Code,
		Title:     d.Title,
		Epic:      domain.Epic{Code: d.Epic.Code, Title: d.Epic.Title},
		Priority:  d.Priority,
		Points:    d.Points,
		Status:    d.Status,
		BlockedBy: d.BlockedBy,
		Scope:     d.Scope,
		Body:      d.Body,
		Ref:       d.Ref,
		URL:       d.URL,
		Branch:    d.Branch,
		Worktree:  d.Worktree,
		ForkBase:  d.ForkBase,
		Rework:    d.Rework,
		History:   d.History,
	}
}

func (c *Connector) writePlan(specCode string, plan domain.PlanInput) error {
	domain.NormalizePlanInput(&plan)
	if err := c.ensureWiki(); err != nil {
		return err
	}
	if err := c.preflightWikiCatalog(); err != nil {
		return err
	}
	tasks := append([]domain.Task(nil), plan.Tasks...)
	for i := range tasks {
		tasks[i].Ref = ""
	}
	meta := wikiPlanMeta{
		Type: "plan", Title: "Plan for " + specCode,
		Description: "Implementation plan and executable tasks for " + specCode,
		Status:      domain.WikiStatusGenerated, Schema: wikiPlanSchema,
		SpecCode: specCode, Tasks: tasks,
	}
	if err := writeWikiPage(c.planPath(specCode), meta, renderPlanWikiBody(specCode, plan.PlanBody, tasks)); err != nil {
		return err
	}
	return wiki.RefreshCatalog(c.cfg.ProjectRoot, c.wikiRoot())
}

func renderPlanWikiBody(specCode, planBody string, tasks []domain.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan for %s\n\n%s\n\n", specCode, planBodyMarker)
	if body := strings.TrimSpace(planBody); body != "" {
		b.WriteString(body + "\n\n")
	}
	b.WriteString(planTasksMarker + "\n\n## Implementation tasks\n")
	for _, task := range tasks {
		fmt.Fprintf(&b, "\n### %s: %s\n\n", task.ID, task.Title)
		fmt.Fprintf(&b, "- **Status:** %s\n- **Type:** %s\n", task.Status, task.Type)
		if len(task.Dependencies) > 0 {
			fmt.Fprintf(&b, "- **Dependencies:** %s\n", strings.Join(task.Dependencies, ", "))
		}
		content := strings.TrimSpace(task.Body)
		if content == "" {
			content = strings.TrimSpace(task.Description)
		}
		if content != "" {
			b.WriteString("\n" + content + "\n")
		}
	}
	b.WriteString("\n## Related\n\n")
	fmt.Fprintf(&b, "- [Specification](../specs/%s.md)\n- [Backlog](../overview.md)\n", specCode)
	return b.String()
}

func writeWikiPage(path string, meta any, body string) error {
	frontmatter, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding Wiki frontmatter: %w", err)
	}
	desiredBody := strings.TrimLeft(body, "\n")
	if raw, readErr := os.ReadFile(path); readErr == nil {
		existingFrontmatter, existingBody, splitErr := splitWikiPage(raw)
		if splitErr == nil && sameManagedWikiContent(existingFrontmatter, frontmatter, existingBody, desiredBody) {
			return nil
		}
	}
	return atomicWriteFile(path, []byte("---\n"+string(frontmatter)+"---\n"+desiredBody))
}

func sameManagedWikiContent(existing, desired []byte, existingBody, desiredBody string) bool {
	var existingMeta, desiredMeta map[string]any
	if yaml.Unmarshal(existing, &existingMeta) != nil || yaml.Unmarshal(desired, &desiredMeta) != nil {
		return false
	}
	delete(existingMeta, "status")
	delete(existingMeta, "review")
	delete(desiredMeta, "status")
	delete(desiredMeta, "review")
	return reflect.DeepEqual(existingMeta, desiredMeta) && strings.TrimSpace(existingBody) == strings.TrimSpace(desiredBody)
}

func splitWikiPage(raw []byte) ([]byte, string, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, "", errors.New("missing YAML frontmatter")
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, "", errors.New("unterminated YAML frontmatter")
	}
	return []byte(rest[:end]), rest[end+5:], nil
}

func markerContent(body, startMarker, endMarker string) string {
	start := strings.Index(body, startMarker)
	if start < 0 {
		return strings.TrimSpace(body)
	}
	content := body[start+len(startMarker):]
	if end := strings.Index(content, endMarker); end >= 0 {
		content = content[:end]
	}
	return strings.TrimSpace(content)
}

func atomicWriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".archetipo-wiki-*")
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

func (c *Connector) migrateLegacyPlans(store yamlStore) error {
	for code := range store.Specs {
		if _, err := os.Stat(c.planPath(code)); err == nil {
			continue
		}
		plan, err := c.readYAMLPlan(code)
		if err != nil {
			var ce *iox.CodedError
			if errors.As(err, &ce) && ce.Code == iox.CodePreconditionMissing {
				continue
			}
			return err
		}
		if err := c.writePlan(code, domain.PlanInput{PlanBody: plan.Body, Tasks: plan.Tasks}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Connector) cleanupLegacyArtifacts(store yamlStore) error {
	paths := []string{c.legacyYAMLBacklogPath(), c.legacyBacklogPath()}
	for code := range store.Specs {
		paths = append(paths, filepath.Join(c.legacyYAMLSpecsDir(), code+".yaml"), c.legacyYAMLPlanPath(code), filepath.Join(c.legacyPlanningDir(), code+".md"))
	}
	for _, path := range paths {
		if path == "" || path == c.backlogPath() {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing migrated legacy artifact %s: %w", path, err)
		}
	}
	for _, dir := range []string{c.legacyYAMLSpecsDir(), c.cfg.AbsPath(c.cfg.File.Planning), c.legacyPlanningDir()} {
		if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrInvalid) {
			// Non-empty legacy directories may contain reviews or user-owned
			// files. Their known backlog artifacts are already removed above.
			continue
		}
	}
	return nil
}

func writeYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("closing YAML encoder: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func (c *Connector) normalizeBacklog(doc backlogDoc, specs map[string]domain.Spec) backlogDoc {
	doc.Schema = backlogSchema
	doc.Version = 2
	epics := map[string]domain.Epic{}
	for _, spec := range specs {
		if spec.Ref == "" {
			spec.Ref = spec.Code
		}
		if spec.Epic.Code != "" {
			epics[spec.Epic.Code] = spec.Epic
		}
	}
	doc.Epics = doc.Epics[:0]
	for _, epic := range epics {
		doc.Epics = append(doc.Epics, epic)
	}
	sort.Slice(doc.Epics, func(i, j int) bool { return doc.Epics[i].Code < doc.Epics[j].Code })

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(specs))
	for _, code := range doc.Order {
		if _, ok := specs[code]; !ok {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		normalized = append(normalized, code)
		seen[code] = struct{}{}
	}
	remaining := make([]string, 0, len(specs))
	for code := range specs {
		if _, ok := seen[code]; ok {
			continue
		}
		remaining = append(remaining, code)
	}
	sort.Strings(remaining)
	normalized = append(normalized, remaining...)
	doc.Order = normalized
	return doc
}

func (c *Connector) boardColumns() []boardColumnDoc {
	return defaultBoardColumns(c.cfg.Workflow.Statuses)
}

func defaultBoardColumns(labels domain.StatusLabels) []boardColumnDoc {
	return []boardColumnDoc{
		{ID: "todo", Title: labels.Todo, Status: domain.Status(labels.Todo)},
		{ID: "planned", Title: labels.Planned, Status: domain.Status(labels.Planned)},
		{ID: "in_progress", Title: labels.InProgress, Status: domain.Status(labels.InProgress)},
		{ID: "review", Title: labels.Review, Status: domain.Status(labels.Review)},
		{ID: "done", Title: labels.Done, Status: domain.Status(labels.Done)},
	}
}

func columnIDForStatus(columns []boardColumnDoc, status domain.Status) (string, bool) {
	for _, col := range columns {
		if col.Status == status {
			return col.ID, true
		}
	}
	return "", false
}

func columnStatus(columns []boardColumnDoc, id string) (domain.Status, bool) {
	for _, col := range columns {
		if col.ID == id {
			return col.Status, true
		}
	}
	return "", false
}

func insertRelative(list []string, code string, anchor domain.ReorderAnchor) ([]string, error) {
	list = removeCode(list, code)
	switch {
	case anchor.Before != "" && anchor.After != "":
		return nil, iox.NewInvalidInput("before and after are mutually exclusive", "pass only one anchor", nil)
	case anchor.Before != "":
		idx := indexOf(list, anchor.Before)
		if idx == -1 {
			return nil, iox.NewPrecondition(fmt.Sprintf("spec %s not found in target order", anchor.Before), "", nil)
		}
		list = append(list[:idx], append([]string{code}, list[idx:]...)...)
	case anchor.After != "":
		idx := indexOf(list, anchor.After)
		if idx == -1 {
			return nil, iox.NewPrecondition(fmt.Sprintf("spec %s not found in target order", anchor.After), "", nil)
		}
		idx++
		if idx >= len(list) {
			list = append(list, code)
		} else {
			list = append(list[:idx], append([]string{code}, list[idx:]...)...)
		}
	default:
		list = append(list, code)
	}
	return list, nil
}

func removeCode(list []string, code string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item != code {
			out = append(out, item)
		}
	}
	return out
}

func indexOf(list []string, code string) int {
	for i, item := range list {
		if item == code {
			return i
		}
	}
	return -1
}
