package wiki

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const representativeLimit = 8

// Inspection is a compact, content-free inventory used to ground semantic Wiki bootstrap.
type Inspection struct {
	ProjectRoot     string                 `json:"project_root"`
	Revision        string                 `json:"revision,omitempty"`
	Files           int                    `json:"files"`
	Languages       []InspectionCount      `json:"languages"`
	Manifests       []string               `json:"manifests"`
	Workspaces      []string               `json:"workspaces"`
	SourceRoots     []string               `json:"source_roots"`
	Boundaries      []InspectionBoundary   `json:"boundaries"`
	Capabilities    []InspectionCapability `json:"capability_candidates"`
	EntryPoints     []string               `json:"entry_points"`
	PublicContracts []string               `json:"public_contracts"`
	Routes          []string               `json:"routes"`
	Schemas         []string               `json:"schemas"`
	Tests           []string               `json:"tests"`
	Automation      []string               `json:"automation"`
	Infrastructure  []string               `json:"infrastructure"`
	Configuration   []string               `json:"configuration"`
	Documentation   []string               `json:"documentation"`
	ProjectSources  []InspectionSource     `json:"project_sources"`
	Excluded        []string               `json:"excluded"`
	Uninspected     []string               `json:"uninspected"`
}

type InspectionCount struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
}

type InspectionBoundary struct {
	Path            string   `json:"path"`
	Kind            string   `json:"kind"`
	Files           int      `json:"files"`
	Representatives []string `json:"representatives"`
}

// InspectionCapability is a deterministic cluster of code signals. It is an
// input to DDD interpretation, not a claim that the cluster is a bounded context.
type InspectionCapability struct {
	ID            string   `json:"id"`
	EntryPoints   []string `json:"entry_points"`
	UI            []string `json:"ui"`
	Application   []string `json:"application"`
	Domain        []string `json:"domain"`
	Data          []string `json:"data"`
	Contracts     []string `json:"contracts"`
	Tests         []string `json:"tests"`
	Configuration []string `json:"configuration"`
}

type InspectionSource struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// Inspect inventories repository paths without returning source or secret contents.
func Inspect(projectRoot, wikiRoot, prdPath string) (Inspection, error) {
	paths, source, err := repositoryFiles(projectRoot)
	if err != nil {
		return Inspection{}, err
	}
	wikiRel, _ := filepath.Rel(projectRoot, wikiRoot)
	wikiRel = filepath.ToSlash(wikiRel)

	result := Inspection{ProjectRoot: projectRoot, Revision: gitRevision(projectRoot)}
	languages := map[string]int{}
	boundaryFiles := map[string][]string{}
	safePaths := []string{}
	excludedSensitive := 0
	for _, rel := range paths {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == "" || rel == wikiRel || strings.HasPrefix(rel, strings.TrimSuffix(wikiRel, "/")+"/") {
			continue
		}
		if hasExcludedDirectory(rel) {
			continue
		}
		if isSensitivePath(rel) {
			excludedSensitive++
			continue
		}
		safePaths = append(safePaths, rel)
		result.Files++
		base := filepath.Base(rel)
		ext := strings.ToLower(filepath.Ext(base))
		lower := strings.ToLower(rel)
		testFile := isTest(lower, base)
		if language := languageForExtension(ext); language != "" {
			languages[language]++
		}
		if isManifest(base) {
			result.Manifests = append(result.Manifests, rel)
			if dir := filepath.ToSlash(filepath.Dir(rel)); dir != "." {
				result.Workspaces = append(result.Workspaces, dir)
			}
		}
		if root := sourceRoot(rel); root != "" {
			result.SourceRoots = append(result.SourceRoots, root)
		}
		if !testFile && isEntryPoint(rel, base) {
			result.EntryPoints = append(result.EntryPoints, rel)
		}
		if !testFile && isPublicContract(rel, base, ext) {
			result.PublicContracts = append(result.PublicContracts, rel)
		}
		if !testFile && isRoute(lower) {
			result.Routes = append(result.Routes, rel)
		}
		if !testFile && isSchema(lower, ext) {
			result.Schemas = append(result.Schemas, rel)
		}
		if testFile {
			result.Tests = append(result.Tests, rel)
		}
		if isAutomation(lower, base) {
			result.Automation = append(result.Automation, rel)
		}
		if isInfrastructure(lower, base, ext) {
			result.Infrastructure = append(result.Infrastructure, rel)
		}
		if isConfiguration(base, ext) {
			result.Configuration = append(result.Configuration, rel)
		}
		if isDocumentation(lower, ext) {
			result.Documentation = append(result.Documentation, rel)
		}
		if boundary := boundaryFor(rel, ext); boundary != "" {
			boundaryFiles[boundary] = append(boundaryFiles[boundary], rel)
		}
	}

	for name, count := range languages {
		result.Languages = append(result.Languages, InspectionCount{Name: name, Files: count})
	}
	sort.Slice(result.Languages, func(i, j int) bool { return result.Languages[i].Name < result.Languages[j].Name })
	for path, files := range boundaryFiles {
		sort.Slice(files, func(i, j int) bool {
			si, sj := representativeScore(files[i]), representativeScore(files[j])
			if si == sj {
				return files[i] < files[j]
			}
			return si > sj
		})
		representatives := files
		if len(representatives) > representativeLimit {
			representatives = representatives[:representativeLimit]
			result.Uninspected = append(result.Uninspected, path+": representative sample only")
		}
		result.Boundaries = append(result.Boundaries, InspectionBoundary{Path: path, Kind: boundaryKind(path), Files: len(files), Representatives: representatives})
	}
	result.Capabilities = buildCapabilityCandidates(safePaths)
	sort.Slice(result.Boundaries, func(i, j int) bool { return result.Boundaries[i].Path < result.Boundaries[j].Path })
	for _, list := range []*[]string{&result.Manifests, &result.Workspaces, &result.SourceRoots, &result.EntryPoints, &result.PublicContracts, &result.Routes, &result.Schemas, &result.Tests, &result.Automation, &result.Infrastructure, &result.Configuration, &result.Documentation, &result.Uninspected} {
		*list = uniqueSorted(*list)
	}
	if prdPath != "" {
		abs := prdPath
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(projectRoot, filepath.FromSlash(prdPath))
		}
		if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() && info.Size() > 0 {
			result.ProjectSources = append(result.ProjectSources, InspectionSource{Path: filepath.ToSlash(prdPath), Kind: "configured-product-document"})
		}
	}
	result.Excluded = []string{"repository metadata", "dependency and build directories", "configured Wiki root"}
	if excludedSensitive > 0 {
		result.Excluded = append(result.Excluded, "sensitive files: "+strconv.Itoa(excludedSensitive))
	}
	if source == "filesystem" {
		result.Uninspected = append(result.Uninspected, "Git inventory unavailable; filesystem fallback used")
	}
	if result.Files == 0 || (len(result.Languages) == 0 && len(result.Manifests) == 0 && len(result.Documentation) == 0) {
		return Inspection{}, ErrNoProjectEvidence
	}
	return result, nil
}

var ErrNoProjectEvidence = errors.New("repository has no code, manifest, or documentation evidence")

func repositoryFiles(root string) ([]string, string, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		parts := strings.Split(string(out), "\x00")
		return uniqueSorted(parts), "git", nil
	}
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() && rel != "." && excludedDir(entry.Name()) {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return uniqueSorted(paths), "filesystem", err
}

func excludedDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".archetipo", ".claude", ".agents", ".cursor", ".gemini", ".opencode", ".pi", ".idea", ".vscode", "node_modules", "vendor", "dist", "build", "target", ".next", ".cache", "coverage", "__pycache__", ".venv", "venv":
		return true
	default:
		return false
	}
}

func hasExcludedDirectory(path string) bool {
	if strings.HasPrefix(path, ".github/skills/") {
		return true
	}
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		if excludedDir(part) {
			return true
		}
	}
	return false
}

func isSensitivePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, ".env") && base != ".env.example" && base != ".env.sample" {
		return true
	}
	return strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.Contains(base, "credentials") || strings.Contains(base, "secret")
}

func languageForExtension(ext string) string {
	return map[string]string{".go": "Go", ".ts": "TypeScript", ".tsx": "TypeScript", ".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript", ".py": "Python", ".rs": "Rust", ".java": "Java", ".kt": "Kotlin", ".kts": "Kotlin", ".cs": "C#", ".rb": "Ruby", ".php": "PHP", ".swift": "Swift", ".c": "C", ".h": "C/C++", ".cc": "C/C++", ".cpp": "C/C++", ".vue": "Vue", ".svelte": "Svelte", ".sql": "SQL", ".sh": "Shell"}[ext]
}

func isManifest(base string) bool {
	switch strings.ToLower(base) {
	case "package.json", "go.mod", "cargo.toml", "pyproject.toml", "requirements.txt", "pom.xml", "build.gradle", "build.gradle.kts", "composer.json", "gemfile", "mix.exs", "pubspec.yaml":
		return true
	default:
		return strings.HasSuffix(strings.ToLower(base), ".csproj") || strings.HasSuffix(strings.ToLower(base), ".sln")
	}
}

func sourceRoot(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts[:len(parts)-1] {
		switch part {
		case "src", "app", "cmd", "internal", "pkg", "lib", "apps", "packages", "services", "modules":
			return strings.Join(parts[:i+1], "/")
		}
	}
	return ""
}

func isEntryPoint(path, base string) bool {
	lower := strings.ToLower(base)
	if lower == "main.go" || lower == "main.py" || lower == "app.py" || lower == "server.py" || lower == "manage.py" || lower == "program.cs" {
		return true
	}
	return (strings.HasPrefix(lower, "index.") || strings.HasPrefix(lower, "main.") || strings.HasPrefix(lower, "server.")) && strings.Count(path, "/") <= 3
}

func isPublicContract(path, base, ext string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/interfaces/") || strings.Contains(lower, "/types/") || strings.Contains(lower, "/proto/") || strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger") || strings.HasSuffix(strings.ToLower(base), ".proto") || strings.HasSuffix(strings.ToLower(base), ".d.ts") || (ext == ".graphql" || ext == ".gql")
}

func isRoute(lower string) bool {
	return strings.Contains(lower, "/api/") || strings.Contains(lower, "/routes/") || strings.Contains(lower, "/controllers/") || strings.HasSuffix(lower, "/route.ts") || strings.HasSuffix(lower, "/route.js")
}

func isSchema(lower, ext string) bool {
	return strings.Contains(lower, "schema") || strings.Contains(lower, "migration") || ext == ".sql" || ext == ".prisma"
}

func isTest(lower, base string) bool {
	return strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") || strings.Contains(lower, "__tests__") || strings.Contains(strings.ToLower(base), "_test.") || strings.Contains(strings.ToLower(base), ".test.") || strings.Contains(strings.ToLower(base), ".spec.")
}

func isAutomation(lower, base string) bool {
	return strings.HasPrefix(lower, ".github/workflows/") || strings.Contains(lower, ".gitlab-ci") || strings.EqualFold(base, "jenkinsfile") || strings.Contains(lower, "azure-pipelines")
}

func isInfrastructure(lower, base, ext string) bool {
	return strings.HasPrefix(strings.ToLower(base), "dockerfile") || strings.Contains(lower, "docker-compose") || ext == ".tf" || strings.Contains(lower, "/k8s/") || strings.Contains(lower, "/helm/") || strings.Contains(lower, "vercel.json") || strings.Contains(lower, "netlify.toml") || strings.Contains(lower, "amplify")
}

func isConfiguration(base, ext string) bool {
	lower := strings.ToLower(base)
	return isManifest(base) || ext == ".yaml" || ext == ".yml" || ext == ".toml" || ext == ".json" || strings.Contains(lower, "config") || strings.HasPrefix(lower, ".env.")
}

func isDocumentation(lower, ext string) bool {
	return ext == ".md" || ext == ".mdx" || strings.HasPrefix(lower, "docs/")
}

func boundaryFor(path, ext string) string {
	if languageForExtension(ext) == "" && !isManifest(filepath.Base(path)) {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		return "."
	}
	if (parts[0] == "apps" || parts[0] == "packages" || parts[0] == "services" || parts[0] == "modules") && len(parts) > 2 {
		return strings.Join(parts[:2], "/")
	}
	return parts[0]
}

func boundaryKind(path string) string {
	if path == "." {
		return "root"
	}
	first := strings.Split(path, "/")[0]
	switch first {
	case "apps", "services":
		return "application"
	case "packages", "modules", "pkg", "lib":
		return "package"
	case "test", "tests":
		return "test"
	case "scripts", "tools":
		return "tooling"
	default:
		return "source"
	}
}

func representativeScore(path string) int {
	lower := strings.ToLower(path)
	score := 0
	if isManifest(filepath.Base(path)) {
		score += 8
	}
	if isEntryPoint(path, filepath.Base(path)) {
		score += 7
	}
	if isSchema(lower, strings.ToLower(filepath.Ext(path))) {
		score += 6
	}
	if isPublicContract(path, filepath.Base(path), strings.ToLower(filepath.Ext(path))) {
		score += 5
	}
	if isTest(lower, filepath.Base(path)) {
		score += 3
	}
	return score
}

type capabilityAccumulator struct {
	entryPoints   []string
	ui            []string
	application   []string
	domain        []string
	data          []string
	contracts     []string
	tests         []string
	configuration []string
}

func buildCapabilityCandidates(paths []string) []InspectionCapability {
	clusters := map[string]*capabilityAccumulator{}
	for _, path := range paths {
		if isTest(strings.ToLower(path), filepath.Base(path)) {
			continue
		}
		id := capabilityID(path)
		if id == "" {
			continue
		}
		cluster := clusters[id]
		if cluster == nil {
			cluster = &capabilityAccumulator{}
			clusters[id] = cluster
		}
		cluster.add(capabilityRole(path), path)
	}
	aliases := map[string][]string{}
	for id := range clusters {
		compact := compactCapability(id)
		aliases[compact] = append(aliases[compact], id)
	}
	for _, path := range paths {
		for _, id := range capabilityIDsInPath(path, aliases) {
			cluster := clusters[id]
			if isTest(strings.ToLower(path), filepath.Base(path)) {
				cluster.tests = append(cluster.tests, path)
				continue
			}
			cluster.add(capabilityRole(path), path)
		}
	}
	result := make([]InspectionCapability, 0, len(clusters))
	for id, cluster := range clusters {
		candidate := InspectionCapability{
			ID:            id,
			EntryPoints:   limitedUnique(cluster.entryPoints),
			UI:            limitedUnique(cluster.ui),
			Application:   limitedUnique(cluster.application),
			Domain:        limitedUnique(cluster.domain),
			Data:          limitedUnique(cluster.data),
			Contracts:     limitedUnique(cluster.contracts),
			Tests:         limitedUnique(cluster.tests),
			Configuration: limitedUnique(cluster.configuration),
		}
		if capabilitySignalGroups(candidate) >= 2 {
			result = append(result, candidate)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (cluster *capabilityAccumulator) add(role, path string) {
	switch role {
	case "entry_point":
		cluster.entryPoints = append(cluster.entryPoints, path)
	case "ui":
		cluster.ui = append(cluster.ui, path)
	case "domain":
		cluster.domain = append(cluster.domain, path)
	case "data":
		cluster.data = append(cluster.data, path)
	case "contract":
		cluster.contracts = append(cluster.contracts, path)
	case "configuration":
		cluster.configuration = append(cluster.configuration, path)
	default:
		cluster.application = append(cluster.application, path)
	}
}

func capabilityID(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for index, part := range parts {
		lower := strings.ToLower(part)
		switch lower {
		case "api", "controllers", "controller", "routes", "route":
			if id := nextCapabilityPart(parts, index+1); id != "" {
				return id
			}
		case "components", "component", "features", "feature", "domains", "domain", "modules", "module", "services", "service", "lib", "hooks":
			if id := nextCapabilityPart(parts, index+1); id != "" {
				return id
			}
		case "app", "pages":
			if index+1 < len(parts) && strings.ToLower(parts[index+1]) != "api" {
				if id := nextCapabilityPart(parts, index+1); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func nextCapabilityPart(parts []string, start int) string {
	for _, raw := range parts[start:] {
		part := strings.ToLower(strings.TrimSuffix(raw, filepath.Ext(raw)))
		if part == "webhooks" || part == "webhook" {
			continue
		}
		if isTechnicalPathPart(part) {
			continue
		}
		return normalizeCapabilityID(part)
	}
	return ""
}

func isTechnicalPathPart(part string) bool {
	if part == "" || strings.HasPrefix(part, "[") || strings.HasPrefix(part, "(") {
		return true
	}
	switch part {
	case "src", "app", "api", "admin", "internal", "public", "private", "common", "shared", "core", "utils", "types", "interfaces", "providers", "actions", "handlers", "middleware", "unit", "integration", "e2e", "test", "tests", "route", "routes", "page", "layout", "index", "main", "server", "service", "services", "ui", "prisma":
		return true
	default:
		return false
	}
}

func normalizeCapabilityID(value string) string {
	value = strings.Trim(strings.ToLower(value), "-_.")
	if strings.HasSuffix(value, "ies") && len(value) > 4 {
		return strings.TrimSuffix(value, "ies") + "y"
	}
	if strings.HasSuffix(value, "s") && !strings.HasSuffix(value, "ss") && len(value) > 4 {
		return strings.TrimSuffix(value, "s")
	}
	return value
}

func capabilityRole(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	ext := strings.ToLower(filepath.Ext(base))
	if isRoute(lower) || strings.Contains(lower, "/controllers/") {
		return "entry_point"
	}
	if strings.Contains(lower, "/components/") || strings.HasSuffix(lower, "/page.tsx") || strings.HasSuffix(lower, "/page.jsx") || strings.Contains(lower, "/pages/") {
		return "ui"
	}
	if strings.Contains(lower, "/domain/") || strings.Contains(lower, "/domains/") {
		return "domain"
	}
	if isSchema(lower, ext) || strings.Contains(lower, "/models/") || strings.Contains(lower, "/entities/") {
		return "data"
	}
	if isPublicContract(path, filepath.Base(path), ext) {
		return "contract"
	}
	if isConfiguration(filepath.Base(path), ext) {
		return "configuration"
	}
	return "application"
}

func capabilityIDsInPath(path string, aliases map[string][]string) []string {
	matched := map[string]bool{}
	for _, raw := range strings.Split(strings.ToLower(filepath.ToSlash(path)), "/") {
		part := normalizeCapabilityID(strings.TrimSuffix(raw, filepath.Ext(raw)))
		for _, id := range aliases[compactCapability(part)] {
			matched[id] = true
		}
	}
	result := make([]string, 0, len(matched))
	for id := range matched {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func compactCapability(value string) string {
	return strings.NewReplacer("-", "", "_", "", ".", "").Replace(value)
}

func capabilitySignalGroups(candidate InspectionCapability) int {
	groups := 0
	for _, count := range []int{len(candidate.EntryPoints), len(candidate.UI), len(candidate.Application), len(candidate.Domain), len(candidate.Data), len(candidate.Contracts), len(candidate.Tests), len(candidate.Configuration)} {
		if count > 0 {
			groups++
		}
	}
	return groups
}

func limitedUnique(items []string) []string {
	items = uniqueSorted(items)
	if len(items) > representativeLimit {
		return items[:representativeLimit]
	}
	return items
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
