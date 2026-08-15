package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Connector != ConnectorFile {
		t.Errorf("expected default connector %q, got %q", ConnectorFile, c.Connector)
	}
	if c.File.Backlog != ".archetipo/backlog.yaml" {
		t.Errorf("default backlog path: %q", c.File.Backlog)
	}
	if c.File.Planning != ".archetipo/plans/" {
		t.Errorf("default planning path: %q", c.File.Planning)
	}
	if c.Paths.PRD != "docs/PRD.md" {
		t.Errorf("default PRD path: %q", c.Paths.PRD)
	}
	if c.Paths.Wiki != "docs/wiki/" {
		t.Errorf("default Wiki path: %q", c.Paths.Wiki)
	}
}

func TestLoadForTargetUsesTargetConfigOrInvokingFallback(t *testing.T) {
	invoking := t.TempDir()
	targetWithConfig := t.TempDir()
	targetWithoutConfig := t.TempDir()
	for root, wikiPath := range map[string]string{invoking: "outer/wiki", targetWithConfig: "target/wiki"} {
		must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
		must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte("connector: file\npaths:\n  wiki: "+wikiPath+"\n"), 0o644))
	}

	target, err := LoadForTarget(invoking, targetWithConfig)
	if err != nil {
		t.Fatal(err)
	}
	if target.ProjectRoot != targetWithConfig || target.Paths.Wiki != "target/wiki" {
		t.Fatalf("target config = root %q wiki %q", target.ProjectRoot, target.Paths.Wiki)
	}

	nestedTarget := filepath.Join(targetWithConfig, "nested", "checkout")
	must(t, os.MkdirAll(nestedTarget, 0o755))
	nested, err := LoadForTarget(invoking, nestedTarget)
	if err != nil {
		t.Fatal(err)
	}
	if nested.ProjectRoot != nestedTarget || nested.Paths.Wiki != "target/wiki" {
		t.Fatalf("nested target config = root %q wiki %q", nested.ProjectRoot, nested.Paths.Wiki)
	}

	fallback, err := LoadForTarget(invoking, targetWithoutConfig)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.ProjectRoot != targetWithoutConfig || fallback.Paths.Wiki != "outer/wiki" {
		t.Fatalf("fallback config = root %q wiki %q", fallback.ProjectRoot, fallback.Paths.Wiki)
	}
}

func TestLoadFromConfigFile(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: file
file:
  backlog: my/BL.yaml
workflow:
  statuses:
    todo: A_FARE
    planned: PIANIFICATO
    in_progress: IN CORSO
    review: REVISIONE
    done: FATTO
`), 0o644))

	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Connector != ConnectorFile {
		t.Errorf("connector: %q", c.Connector)
	}
	if c.File.Backlog != "my/BL.yaml" {
		t.Errorf("backlog: %q", c.File.Backlog)
	}
	// Defaults preserved for unspecified keys.
	if c.Paths.PRD != "docs/PRD.md" {
		t.Errorf("PRD default lost: %q", c.Paths.PRD)
	}
	if c.File.Planning != ".archetipo/plans/" {
		t.Errorf("planning default lost: %q", c.File.Planning)
	}
	if c.Workflow.Statuses.Todo != "A_FARE" {
		t.Errorf("status override lost: %q", c.Workflow.Statuses.Todo)
	}
	if c.ProjectRoot != root {
		t.Errorf("project root: %q want %q", c.ProjectRoot, root)
	}
}

func TestLoad_E2ERecordDemoVideo(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: file
e2e:
  record_demo_video: true
`), 0o644))
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !c.E2E.RecordDemoVideo {
		t.Errorf("record_demo_video: got false, want true")
	}
}

func TestLoad_E2ERecordDemoVideoDefaultsFalse(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: file
`), 0o644))
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.E2E.RecordDemoVideo {
		t.Errorf("record_demo_video: got true, want false default when section absent")
	}
}

// git.auto_commit is off by default, so the three states a YAML file can
// express — key absent, explicitly true, explicitly false — must resolve to
// disabled, enabled, disabled.
func TestLoad_GitAutoCommit(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml string
		want bool
	}{
		"section absent":   {"connector: file\n", false},
		"explicitly true":  {"connector: file\ngit:\n  auto_commit: true\n", true},
		"explicitly false": {"connector: file\ngit:\n  auto_commit: false\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
			must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(tc.yaml), 0o644))
			c, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if c.Git.AutoCommit != tc.want {
				t.Errorf("auto_commit: got %v, want %v", c.Git.AutoCommit, tc.want)
			}
		})
	}
}

// The Wiki gate is off by default, so the three states a YAML file can express
// — key absent, explicitly true, explicitly false — must resolve to disabled,
// enabled, disabled.
func TestLoad_WikiEnabled(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml string
		want bool
	}{
		"section absent":  {"connector: file\n", false},
		"section empty":   {"connector: file\nwiki:\n", false},
		"explicitly true": {"connector: file\nwiki:\n  enabled: true\n", true},
		"explicitly off":  {"connector: file\nwiki:\n  enabled: false\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
			must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(tc.yaml), 0o644))
			c, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.WikiEnabled(); got != tc.want {
				t.Errorf("WikiEnabled() = %v, want %v for:\n%s", got, tc.want, tc.yaml)
			}
		})
	}
}

// An enabled gate must survive a render/parse round-trip: RenderFull applies
// defaults, and a default applied to the wrong field would silently disable
// the Wiki of every project that saves its config from the viewer.
func TestRenderFullPreservesEnabledWiki(t *testing.T) {
	root := t.TempDir()
	c := Default()
	c.ProjectRoot = root
	c.Wiki.Enabled = true
	out, err := RenderFull(c)
	must(t, err)
	if !strings.Contains(string(out), "enabled: true") {
		t.Fatalf("rendered config lost the enabled gate:\n%s", out)
	}
	reparsed, err := ValidateRaw(root, out)
	must(t, err)
	if !reparsed.WikiEnabled() {
		t.Errorf("round-trip disabled the Wiki:\n%s", out)
	}
}

// SetupBase is what `config show` returns, and it is the only channel a skill
// has for reading the optional sections: a key missing here sends the skill
// back to parsing config.yaml by hand.
func TestSetupBaseReportsEveryOptionalSection(t *testing.T) {
	c := Default()
	c.ProjectRoot = "/tmp/project"
	c.Wiki.Enabled = true
	c.Worktree.Enabled = true
	c.E2E.RecordDemoVideo = true

	setup := c.SetupBase(ConnectorFile)
	if !setup.Wiki.Enabled {
		t.Error("setup.wiki.enabled must mirror the enabled gate")
	}
	if !setup.Worktree.Enabled || setup.Worktree.Base != "main" {
		t.Errorf("setup.worktree = %+v", setup.Worktree)
	}
	if !setup.E2E.RecordDemoVideo {
		t.Error("setup.e2e.record_demo_video must mirror the config")
	}
	if setup.Connector != ConnectorFile || setup.ProjectRoot != "/tmp/project" || setup.Paths.Wiki != "docs/wiki/" {
		t.Errorf("setup base fields = %+v", setup)
	}
}

// An omitted section must report disabled: the automatic Wiki is opt-in, so a
// project that never configured it maintains no pages.
func TestSetupBaseReportsWikiDisabledWhenSectionOmitted(t *testing.T) {
	setup := Config{}.SetupBase(ConnectorFile)
	if setup.Wiki.Enabled {
		t.Error("an unset wiki section must report disabled")
	}
}

func TestLoadFromSubdirectoryWalksUp(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: file
`), 0o644))
	sub := filepath.Join(root, "src", "deep")
	must(t, os.MkdirAll(sub, 0o755))

	c, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	if c.ProjectRoot != root {
		t.Errorf("project root: %q want %q", c.ProjectRoot, root)
	}
}

func TestUnknownConnectorPassesThroughConfig(t *testing.T) {
	// Config intentionally does NOT validate connector names;
	// connector.New rejects unknown names with a dynamic list
	// of registered connectors. This avoids a circular import
	// (config → connector) and keeps config connector-agnostic.
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: gitlab
`), 0o644))
	c, err := Load(root)
	if err != nil {
		t.Fatalf("config should load regardless of connector name: %v", err)
	}
	if c.Connector != "gitlab" {
		t.Errorf("expected connector 'gitlab', got %q", c.Connector)
	}
}

func TestLegacyPathsBacklogIsRejected(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: file
paths:
  backlog: .archetipo/backlog.yaml
`), 0o644))

	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for legacy paths.backlog key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "paths.backlog -> file.backlog") {
		t.Errorf("error should mention migration path; got: %v", err)
	}
}

func TestLegacyPathsPlanningIsRejected(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: file
paths:
  planning: .archetipo/plans/
`), 0o644))

	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for legacy paths.planning key")
	}
	if !strings.Contains(err.Error(), "paths.planning -> file.planning") {
		t.Errorf("error should mention migration path; got: %v", err)
	}
}

func TestPathValidationRejectsUnwritableSharedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unwritable check not portable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root bypasses directory permission checks")
	}
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	readonly := filepath.Join(root, "readonly")
	must(t, os.MkdirAll(readonly, 0o755))
	must(t, os.Chmod(readonly, 0o555))
	defer func() { _ = os.Chmod(readonly, 0o755) }()

	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: github
paths:
  mockups: readonly/inside/
`), 0o644))

	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for unwritable paths.mockups")
	}
	if !strings.Contains(err.Error(), "paths.mockups") {
		t.Errorf("error should mention paths.mockups; got: %v", err)
	}
}

func TestPathValidationSkipsFilePathsForGitHubConnector(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	// file.planning points to a non-existent unrelated absolute path. The
	// github connector should not validate file.* paths.
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: github
file:
  planning: /nonexistent/never/touched/by/github/
`), 0o644))

	if _, err := Load(root); err != nil {
		t.Fatalf("github connector should not validate file.* paths: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSave_PatchesGitHubKeysPreservingComments(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	initial := `connector: github
paths:
  prd: docs/PRD.md

github:
  # auto-detected on first run
  owner: ""
`
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(initial), 0o644))

	c, err := Load(root)
	must(t, err)
	c.GitHub.Owner = "acme"
	c.GitHub.ProjectNumber = 42
	must(t, c.Save())

	out, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(out)
	for _, want := range []string{
		"# auto-detected on first run",
		"owner: acme",
		"project_number: 42",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in saved file:\n%s", want, s)
		}
	}
}

func TestSave_AddsGitHubSectionWhenMissing(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	initial := `connector: github
paths:
  prd: docs/PRD.md
`
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(initial), 0o644))

	c, err := Load(root)
	must(t, err)
	c.GitHub.Owner = "x"
	c.GitHub.ProjectNumber = 7
	must(t, c.Save())

	raw, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(raw)
	for _, want := range []string{"github:", "owner: x", "project_number: 7"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in saved file:\n%s", want, s)
		}
	}
}

func TestSave_ReusesEmptyGitHubSectionFromTemplate(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	initial := `connector: github
paths:
  prd: docs/PRD.md
#only valid for github connector
github:

# owner: auto-detected from repo
# project_number: auto-detected from repo
`
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(initial), 0o644))

	c, err := Load(root)
	must(t, err)
	c.GitHub.Owner = "sleli"
	c.GitHub.ProjectNumber = 23
	must(t, c.Save())

	raw, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(raw)
	if strings.Count(s, "\ngithub:") != 1 {
		t.Fatalf("expected a single github section, got:\n%s", s)
	}
	for _, want := range []string{"owner: sleli", "project_number: 23"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in saved file:\n%s", want, s)
		}
	}
}

func TestSave_NoOpWhenProjectRootEmpty(t *testing.T) {
	c := Default()
	c.GitHub.Owner = "x"
	c.GitHub.ProjectNumber = 1
	if err := c.Save(); err != nil {
		t.Fatalf("Save with empty ProjectRoot should be a no-op, got %v", err)
	}
}

func TestSave_CreatesFileWhenMissing(t *testing.T) {
	root := t.TempDir()
	c := Default()
	c.ProjectRoot = root
	c.Connector = ConnectorGitHub
	c.GitHub.Owner = "y"
	c.GitHub.ProjectNumber = 1
	must(t, c.Save())

	raw, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(raw)
	if !strings.Contains(s, "owner: y") || !strings.Contains(s, "project_number: 1") {
		t.Errorf("fresh config missing github keys:\n%s", s)
	}
}

func TestLoad_JiraProjectKeyOptional(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: jira
jira:
  base_url: https://acme.atlassian.net
`), 0o644))

	c, err := Load(root)
	if err != nil {
		t.Fatalf("project_key should be optional (auto-detected on first run): %v", err)
	}
	if c.Jira.ProjectKey != "" {
		t.Errorf("project_key: %q", c.Jira.ProjectKey)
	}
}

func TestLoad_JiraBaseURLRequiredWithoutEnv(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "")
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: jira
`), 0o644))

	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "jira.base_url") {
		t.Fatalf("expected jira.base_url error, got %v", err)
	}
}

func TestLoad_JiraBaseURLEnvFallback(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://acme.atlassian.net")
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(`connector: jira
`), 0o644))

	if _, err := Load(root); err != nil {
		t.Fatalf("JIRA_BASE_URL should satisfy base_url requirement: %v", err)
	}
}

func TestSave_PatchesJiraKeysPreservingComments(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	initial := `connector: jira
paths:
  prd: docs/PRD.md

jira:
  # project_key is auto-detected on first run
  base_url: https://acme.atlassian.net
`
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(initial), 0o644))

	c, err := Load(root)
	must(t, err)
	c.Jira.ProjectKey = "ARCH"
	c.Jira.StatusMap = map[string]string{"TODO": "To Do", "DONE": "Done"}
	must(t, c.Save())

	out, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(out)
	for _, want := range []string{
		"# project_key is auto-detected on first run",
		"base_url: https://acme.atlassian.net",
		"project_key: ARCH",
		"status_map:",
		"TODO: To Do",
		"DONE: Done",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in saved file:\n%s", want, s)
		}
	}
	if strings.Contains(s, "github:") {
		t.Errorf("jira connector save must not inject a github section:\n%s", s)
	}
}

func TestSave_ReusesEmptyJiraSectionFromTemplate(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	// Mirrors the shipped template: `jira:` followed only by commented keys,
	// i.e. a null value node that Save must convert in place.
	initial := `connector: jira
jira:
# base_url: https://example.atlassian.net
# project_key: ARCH
`
	must(t, os.WriteFile(filepath.Join(root, RelativePath), []byte(initial), 0o644))

	t.Setenv("JIRA_BASE_URL", "https://acme.atlassian.net")
	c, err := Load(root)
	must(t, err)
	c.Jira.ProjectKey = "AID"
	must(t, c.Save())

	raw, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(raw)
	if strings.Count(s, "jira:") != 1 {
		t.Fatalf("expected a single jira section, got:\n%s", s)
	}
	if !strings.Contains(s, "project_key: AID") {
		t.Errorf("missing project_key in saved file:\n%s", s)
	}
	// base_url came from env only: it must NOT be written to the file.
	if strings.Contains(s, "base_url: https://acme.atlassian.net") {
		t.Errorf("env-sourced base_url must not be persisted:\n%s", s)
	}
}

func TestSave_CreatesFileWithJiraSection(t *testing.T) {
	root := t.TempDir()
	c := Default()
	c.ProjectRoot = root
	c.Connector = ConnectorJira
	c.Jira.BaseURL = "https://acme.atlassian.net"
	c.Jira.ProjectKey = "ARCH"
	must(t, c.Save())

	raw, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	s := string(raw)
	for _, want := range []string{"connector: jira", "jira:", "project_key: ARCH"} {
		if !strings.Contains(s, want) {
			t.Errorf("fresh config missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "github:") {
		t.Errorf("jira bootstrap must not emit a github section:\n%s", s)
	}
}

func TestReadRawMissingReturnsPath(t *testing.T) {
	root := t.TempDir()
	raw, exists, path, err := ReadRaw(root)
	must(t, err)
	if exists {
		t.Fatal("expected missing config")
	}
	if raw != "" {
		t.Fatalf("expected empty raw config, got %q", raw)
	}
	if path != filepath.Join(root, RelativePath) {
		t.Fatalf("path = %q, want %q", path, filepath.Join(root, RelativePath))
	}
}

func TestRenderFullRendersCanonicalConfig(t *testing.T) {
	c := Default()
	c.Connector = ConnectorJira
	c.Jira.BaseURL = "https://acme.atlassian.net"
	c.ProjectRoot = "/tmp/project"
	c.ResolutionNotices = []string{"cwd is inside ARchetipo worktree US-001"}
	out, err := RenderFull(c)
	must(t, err)
	s := string(out)
	for _, want := range []string{
		"connector: jira",
		"paths:",
		"workflow:",
		"worktree:",
		"e2e:",
		"jira:",
		"base_url: https://acme.atlassian.net",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, s)
		}
	}
	for _, runtimeOnly := range []string{"project_root", "resolutionnotices", "resolution_notices", "US-001"} {
		if strings.Contains(s, runtimeOnly) {
			t.Fatalf("rendered config leaked runtime field %q:\n%s", runtimeOnly, s)
		}
	}
}

func TestValidateRawRejectsLegacyKeys(t *testing.T) {
	root := t.TempDir()
	_, err := ValidateRaw(root, []byte(`connector: file
paths:
  backlog: .archetipo/backlog.yaml
`))
	if err == nil || !strings.Contains(err.Error(), "paths.backlog -> file.backlog") {
		t.Fatalf("expected legacy-key rejection, got %v", err)
	}
}

func TestSaveRawCreatesFileWhenMissing(t *testing.T) {
	root := t.TempDir()
	raw := []byte("connector: file\n")
	backup, err := SaveRaw(root, raw)
	must(t, err)
	if backup != "" {
		t.Fatalf("did not expect backup on first save, got %q", backup)
	}
	got, err := os.ReadFile(filepath.Join(root, RelativePath))
	must(t, err)
	if string(got) != string(raw) {
		t.Fatalf("saved config mismatch: got %q want %q", string(got), string(raw))
	}
}

func TestSaveRawRejectsInvalidAndPreservesExistingFile(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	path := filepath.Join(root, RelativePath)
	must(t, os.WriteFile(path, []byte("connector: file\n"), 0o644))
	if _, err := SaveRaw(root, []byte("connector: [\n")); err == nil {
		t.Fatal("expected invalid YAML to be rejected")
	}
	got, err := os.ReadFile(path)
	must(t, err)
	if string(got) != "connector: file\n" {
		t.Fatalf("existing config changed after failed save: %q", string(got))
	}
}

func TestSaveRawCreatesBackupOnOverwrite(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	path := filepath.Join(root, RelativePath)
	must(t, os.WriteFile(path, []byte("connector: file\n"), 0o644))
	backup, err := SaveRaw(root, []byte("connector: github\n"))
	must(t, err)
	if backup == "" {
		t.Fatal("expected backup path on overwrite")
	}
	backupRaw, err := os.ReadFile(backup)
	must(t, err)
	if string(backupRaw) != "connector: file\n" {
		t.Fatalf("backup mismatch: %q", string(backupRaw))
	}
	got, err := os.ReadFile(path)
	must(t, err)
	if string(got) != "connector: github\n" {
		t.Fatalf("saved config mismatch: %q", string(got))
	}
}

func TestDefaultProviderRoundTripAndAtomicUpdate(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	path := filepath.Join(root, RelativePath)
	original := "# workspace comment\nconnector: file\ncustom:\n  keep: true\n"
	must(t, os.WriteFile(path, []byte(original), 0o644))
	selection := DefaultProviderConfig{ID: "fake-valid", Config: map[string]any{
		"endpoint": "https://runner.test",
		"regions":  []any{"eu", "us"},
		"nested":   map[string]any{"retries": 2},
	}}
	backup, err := UpdateDefaultProvider(root, selection)
	must(t, err)
	if backup == "" {
		t.Fatal("expected backup for existing config")
	}
	backupRaw, err := os.ReadFile(backup)
	must(t, err)
	if string(backupRaw) != original {
		t.Fatalf("backup mismatch: %q", string(backupRaw))
	}
	updated, err := os.ReadFile(path)
	must(t, err)
	if !strings.Contains(string(updated), "# workspace comment") || !strings.Contains(string(updated), "custom:\n    keep: true") {
		t.Fatalf("unrelated content was not preserved:\n%s", updated)
	}
	cfg, err := LoadExact(root)
	must(t, err)
	if cfg.Execution.DefaultProvider == nil || cfg.Execution.DefaultProvider.ID != "fake-valid" {
		t.Fatalf("default provider not loaded: %#v", cfg.Execution.DefaultProvider)
	}
	if cfg.Execution.DefaultProvider.Config["endpoint"] != "https://runner.test" {
		t.Fatalf("provider config mismatch: %#v", cfg.Execution.DefaultProvider.Config)
	}
}

func TestUpdateDefaultProviderRejectsInvalidDocumentWithoutChangingIt(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".archetipo"), 0o755))
	path := filepath.Join(root, RelativePath)
	original := "- not\n- a mapping\n"
	must(t, os.WriteFile(path, []byte(original), 0o644))
	if _, err := UpdateDefaultProvider(root, DefaultProviderConfig{ID: "fake"}); err == nil {
		t.Fatal("expected non-mapping document rejection")
	}
	got, err := os.ReadFile(path)
	must(t, err)
	if string(got) != original {
		t.Fatalf("invalid update changed file: %q", got)
	}
}

func TestRenderFullDoesNotInventDefaultProvider(t *testing.T) {
	out, err := RenderFull(Default())
	must(t, err)
	if strings.Contains(string(out), "default_provider") {
		t.Fatalf("render invented a default provider:\n%s", out)
	}
}

// seedNestedWorktree builds the layout that made a real autopilot run read
// stale state: a parent checkout plus a per-spec git worktree carrying its own
// committed copy of .archetipo/config.yaml, deliberately pointing at different
// paths so a wrong resolution is visible in the assertions.
func seedNestedWorktree(t *testing.T, parentWorktreeDir string) (parent, worktree string) {
	t.Helper()
	parent = t.TempDir()
	must(t, os.MkdirAll(filepath.Join(parent, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(parent, RelativePath),
		[]byte("connector: file\npaths:\n  wiki: parent/wiki\nworktree:\n  dir: "+parentWorktreeDir+"\n"), 0o644))

	worktree = filepath.Join(parent, filepath.FromSlash(".archetipo/worktrees"), "US-001")
	must(t, os.MkdirAll(filepath.Join(worktree, ".archetipo"), 0o755))
	must(t, os.WriteFile(filepath.Join(worktree, RelativePath),
		[]byte("connector: file\npaths:\n  wiki: stale/wiki\n"), 0o644))
	return parent, worktree
}

func TestLoadResolvesParentCheckoutFromNestedWorktree(t *testing.T) {
	parent, worktree := seedNestedWorktree(t, ".archetipo/worktrees")

	c, err := Load(worktree)
	must(t, err)
	if c.ProjectRoot != parent {
		t.Fatalf("ProjectRoot = %q, want parent %q", c.ProjectRoot, parent)
	}
	if c.Paths.Wiki != "parent/wiki" {
		t.Fatalf("read stale worktree config: wiki = %q", c.Paths.Wiki)
	}
	if len(c.ResolutionNotices) != 1 {
		t.Fatalf("ResolutionNotices = %v, want exactly one", c.ResolutionNotices)
	}
	if !strings.Contains(c.ResolutionNotices[0], "US-001") || !strings.Contains(c.ResolutionNotices[0], parent) {
		t.Fatalf("notice does not name the worktree and the resolved root: %q", c.ResolutionNotices[0])
	}
}

func TestLoadResolvesParentFromDeepInsideWorktree(t *testing.T) {
	parent, worktree := seedNestedWorktree(t, ".archetipo/worktrees")
	deep := filepath.Join(worktree, "src", "sub")
	must(t, os.MkdirAll(deep, 0o755))

	// The guard acts on the resolved root, not on the literal cwd.
	c, err := Load(deep)
	must(t, err)
	if c.ProjectRoot != parent || c.Paths.Wiki != "parent/wiki" {
		t.Fatalf("root = %q wiki = %q, want parent %q / parent/wiki", c.ProjectRoot, c.Paths.Wiki, parent)
	}
	if len(c.ResolutionNotices) != 1 {
		t.Fatalf("ResolutionNotices = %v, want exactly one", c.ResolutionNotices)
	}
}

func TestLoadFromProjectRootIsUnchanged(t *testing.T) {
	parent, _ := seedNestedWorktree(t, ".archetipo/worktrees")

	c, err := Load(parent)
	must(t, err)
	if c.ProjectRoot != parent {
		t.Fatalf("ProjectRoot = %q, want %q", c.ProjectRoot, parent)
	}
	if len(c.ResolutionNotices) != 0 {
		t.Fatalf("unexpected notices from the project root: %v", c.ResolutionNotices)
	}
}

func TestLoadExactKeepsWorktreeConfig(t *testing.T) {
	_, worktree := seedNestedWorktree(t, ".archetipo/worktrees")

	c, err := LoadExact(worktree)
	must(t, err)
	if c.ProjectRoot != worktree {
		t.Fatalf("LoadExact ProjectRoot = %q, want worktree %q", c.ProjectRoot, worktree)
	}
	if c.Paths.Wiki != "stale/wiki" {
		t.Fatalf("LoadExact wiki = %q, want the worktree's own value", c.Paths.Wiki)
	}
	if len(c.ResolutionNotices) != 0 {
		t.Fatalf("LoadExact must not emit notices, got %v", c.ResolutionNotices)
	}
}

func TestLoadForTargetKeepsWorktreeTarget(t *testing.T) {
	parent, worktree := seedNestedWorktree(t, ".archetipo/worktrees")

	// `wiki --project-root <worktree>` must keep targeting the worktree.
	c, err := LoadForTarget(parent, worktree)
	must(t, err)
	if c.ProjectRoot != worktree {
		t.Fatalf("LoadForTarget ProjectRoot = %q, want worktree %q", c.ProjectRoot, worktree)
	}
	if c.Paths.Wiki != "stale/wiki" {
		t.Fatalf("LoadForTarget wiki = %q, want the worktree's own value", c.Paths.Wiki)
	}
	if len(c.ResolutionNotices) != 0 {
		t.Fatalf("LoadForTarget must not emit notices, got %v", c.ResolutionNotices)
	}
}

func TestLoadDoesNotAscendWhenWorktreeDirDiffers(t *testing.T) {
	// The parent stores its worktrees under .wt, so a directory under
	// .archetipo/worktrees is not one of its worktrees and must be left alone.
	_, worktree := seedNestedWorktree(t, ".wt")

	c, err := Load(worktree)
	must(t, err)
	if c.ProjectRoot != worktree || c.Paths.Wiki != "stale/wiki" {
		t.Fatalf("unexpected ascent: root = %q wiki = %q", c.ProjectRoot, c.Paths.Wiki)
	}
	if len(c.ResolutionNotices) != 0 {
		t.Fatalf("unexpected notices: %v", c.ResolutionNotices)
	}
}

func TestLoadAscendsWithForeignSeparatorInConfig(t *testing.T) {
	// A config authored on Windows keeps working on Unix and vice versa.
	parent, worktree := seedNestedWorktree(t, `.archetipo\worktrees`)

	c, err := Load(worktree)
	must(t, err)
	if c.ProjectRoot != parent || c.Paths.Wiki != "parent/wiki" {
		t.Fatalf("root = %q wiki = %q, want parent %q / parent/wiki", c.ProjectRoot, c.Paths.Wiki, parent)
	}
	if len(c.ResolutionNotices) != 1 {
		t.Fatalf("ResolutionNotices = %v, want exactly one", c.ResolutionNotices)
	}
}

func TestLoadKeepsWorktreeWhenParentConfigIsMalformed(t *testing.T) {
	parent, worktree := seedNestedWorktree(t, ".archetipo/worktrees")
	must(t, os.WriteFile(filepath.Join(parent, RelativePath), []byte("connector: file\n  bad: [yaml\n"), 0o644))

	// The guard never turns a parent problem into a Load failure.
	c, err := Load(worktree)
	must(t, err)
	if c.ProjectRoot != worktree || c.Paths.Wiki != "stale/wiki" {
		t.Fatalf("unexpected ascent on malformed parent: root = %q wiki = %q", c.ProjectRoot, c.Paths.Wiki)
	}
	if len(c.ResolutionNotices) != 0 {
		t.Fatalf("unexpected notices: %v", c.ResolutionNotices)
	}
}
