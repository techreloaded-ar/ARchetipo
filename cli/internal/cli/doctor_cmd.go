package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/e2e"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

// doctorCheck is one diagnostic line. Skipped checks count as neither pass nor
// failure (e.g. gh when the connector is file).
type doctorCheck struct {
	name    string
	ok      bool
	skipped bool
	detail  string
	hint    string
}

// newDoctorCmd diagnoses the local installation: data directory, skills,
// runtime assets, project config, installed skills per tool, git and gh.
// Human-readable output (like `version`), exit code 4 when any check fails.
func newDoctorCmd(s streams, deps executionDependencies) *cobra.Command {
	offline := false
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the ARchetipo installation and project setup",
		Long: "Checks the CLI installation (data directory, packaged skills, runtime assets), " +
			"the project setup (.archetipo/config.yaml, skills installed in tool directories), " +
			"the configured execution provider, and external dependencies (git, gh when the " +
			"github connector is configured). Exits non-zero when a check fails.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := runDoctorChecks(cmd, deps, offline)
			failed := 0
			fmt.Fprintf(s.out, "archetipo %s\n\n", version.Version)
			for _, c := range checks {
				switch {
				case c.skipped:
					fmt.Fprintf(s.out, "- %s: %s\n", c.name, c.detail)
				case c.ok:
					fmt.Fprintf(s.out, "✓ %s: %s\n", c.name, c.detail)
				default:
					failed++
					fmt.Fprintf(s.out, "✗ %s: %s\n", c.name, c.detail)
					if c.hint != "" {
						fmt.Fprintf(s.out, "  → %s\n", c.hint)
					}
				}
			}
			fmt.Fprintln(s.out)
			if failed > 0 {
				return iox.NewPrecondition(
					fmt.Sprintf("%d doctor check(s) failed", failed),
					"see the report above for per-check fixes", nil)
			}
			fmt.Fprintln(s.out, "All checks passed.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "skip the checks that contact a remote provider")
	return cmd
}

func runDoctorChecks(cmd *cobra.Command, deps executionDependencies, offline bool) []doctorCheck {
	ctx := cmd.Context()
	var checks []doctorCheck

	// 1. Data directory (skills + runtime shipped with the CLI).
	dataDir, err := discoverDataDir()
	if err != nil {
		checks = append(checks, doctorCheck{
			name:   "data directory",
			detail: "not found",
			hint:   "set ARCHETIPO_DATA_DIR or reinstall via `npm i -g @techreloaded/archetipo`",
		})
	} else {
		checks = append(checks, doctorCheck{name: "data directory", ok: true, detail: dataDir})
		checks = append(checks, checkPackagedSkills(dataDir))
		checks = append(checks, checkRuntimeAssets(dataDir))
	}

	// 2. Project config.
	cfg, cfgErr := loadConfigFor(cmd)
	if cfgErr != nil {
		checks = append(checks, doctorCheck{
			name:   "project config",
			detail: cfgErr.Error(),
			hint:   "fix .archetipo/config.yaml or delete it to fall back to defaults",
		})
	} else {
		detail := fmt.Sprintf("connector %q, project root %s", cfg.Connector, cfg.ProjectRoot)
		// doctor prints human-readable text instead of an envelope, so the
		// root-resolution notices ride along in the check detail.
		for _, notice := range cfg.ResolutionNotices {
			detail += "; " + notice
		}
		checks = append(checks, doctorCheck{name: "project config", ok: true, detail: detail})
		checks = append(checks, checkWiki(cfg))
	}

	// 3. Skills installed in the project's tool directories.
	checks = append(checks, checkInstalledSkills())

	// 4. git availability.
	if gitPath, err := exec.LookPath("git"); err != nil {
		checks = append(checks, doctorCheck{
			name:   "git",
			detail: "not found in PATH",
			hint:   "install git; the worktree workflow and the github connector require it",
		})
	} else {
		checks = append(checks, doctorCheck{name: "git", ok: true, detail: gitPath})
	}

	// 5. gh availability + auth, only relevant for the github connector.
	if cfgErr == nil && cfg.Connector == config.ConnectorGitHub {
		checks = append(checks, checkGH(ctx))
	} else {
		checks = append(checks, doctorCheck{name: "gh", skipped: true, detail: "skipped (connector is not github)"})
	}

	// 6. Jira credentials, only relevant for the jira connector.
	if cfgErr == nil && cfg.Connector == config.ConnectorJira {
		checks = append(checks, checkJira(cfg))
	} else {
		checks = append(checks, doctorCheck{name: "jira", skipped: true, detail: "skipped (connector is not jira)"})
	}

	// 7. The execution provider: whether work can be dispatched at all.
	if cfgErr == nil {
		credential, probe := checkExecution(ctx, cfg, deps, offline)
		checks = append(checks, credential, probe)
	} else {
		checks = append(checks,
			doctorCheck{name: "execution credential", skipped: true, detail: "skipped (project config unavailable)"},
			doctorCheck{name: "execution provider", skipped: true, detail: "skipped (project config unavailable)"},
		)
	}

	// 8. e2e toolchain, only relevant for Node projects that use it.
	if cfgErr == nil {
		checks = append(checks, checkE2E(cfg))
	} else {
		checks = append(checks, doctorCheck{name: "e2e", skipped: true, detail: "skipped (project config unavailable)"})
	}

	return checks
}

func checkWiki(cfg config.Config) doctorCheck {
	root := cfg.Paths.Wiki
	if !filepath.IsAbs(root) {
		root = filepath.Join(cfg.ProjectRoot, filepath.FromSlash(root))
	}
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		// With the workflow gate off, no Wiki is the configured state, not a
		// gap: the project opted out of maintaining one automatically. A Wiki
		// that exists is still validated below, because `archetipo wiki` and an
		// explicit /archetipo-wiki invocation stay available either way.
		if !cfg.WikiEnabled() {
			return doctorCheck{name: "project Wiki", skipped: true, detail: "skipped (wiki.enabled: false)"}
		}
		return doctorCheck{name: "project Wiki", detail: "not initialized", hint: "run `/archetipo-wiki bootstrap` or `archetipo wiki init`"}
	} else if err != nil {
		return doctorCheck{name: "project Wiki", detail: err.Error(), hint: "check paths.wiki and filesystem permissions"}
	}
	report := wiki.Validate(cfg.ProjectRoot, root)
	if !report.OK {
		return doctorCheck{name: "project Wiki", detail: fmt.Sprintf("%d page(s), %d finding(s)", report.Pages, len(report.Findings)), hint: "run `archetipo wiki validate` and repair error findings"}
	}
	return doctorCheck{name: "project Wiki", ok: true, detail: fmt.Sprintf("%d page(s), valid", report.Pages)}
}

// checkE2E reports the e2e toolchain state. It is informational for projects
// that are not (yet) using e2e: it only fails when a Node project has e2e but
// node/npm are missing from PATH.
func checkE2E(cfg config.Config) doctorCheck {
	if _, err := os.Stat(filepath.Join(cfg.ProjectRoot, "package.json")); err != nil {
		return doctorCheck{name: "e2e", skipped: true, detail: "skipped (no package.json)"}
	}
	det, err := e2e.Detect(cfg.ProjectRoot)
	if err != nil {
		return doctorCheck{name: "e2e", detail: "detection failed: " + err.Error()}
	}
	if det.Framework == "" {
		return doctorCheck{name: "e2e", skipped: true, detail: "skipped (no e2e framework configured)"}
	}
	_, nodeErr := exec.LookPath("node")
	_, npmErr := exec.LookPath("npm")
	if nodeErr != nil || npmErr != nil {
		return doctorCheck{
			name:   "e2e",
			detail: fmt.Sprintf("%s detected but node/npm missing in PATH", det.Framework),
			hint:   "install Node.js (provides node + npm) to run `archetipo e2e ensure`",
		}
	}
	if !det.Installed {
		return doctorCheck{
			name:   "e2e",
			ok:     true,
			detail: fmt.Sprintf("%s configured; package not installed yet (%s)", det.Framework, demoRecordingState(cfg)),
			hint:   "run `archetipo e2e ensure` to install it",
		}
	}
	return doctorCheck{name: "e2e", ok: true, detail: fmt.Sprintf("%s installed (%s)", det.Framework, demoRecordingState(cfg))}
}

// demoRecordingState renders the e2e.record_demo_video gate for the doctor
// report so it is visible whether `archetipo e2e demo` will record or skip.
func demoRecordingState(cfg config.Config) string {
	if cfg.E2E.RecordDemoVideo {
		return "demo recording enabled"
	}
	return "demo recording disabled (e2e.record_demo_video)"
}

// checkJira verifies the jira connector has the base URL and the credentials
// it needs. project_key is not required: the connector auto-detects (or
// creates) the project on first run. It does not hit the network (a failing
// token would be surfaced at the first real operation).
func checkJira(cfg config.Config) doctorCheck {
	var missing []string
	baseURL := cfg.Jira.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("JIRA_BASE_URL")
	}
	if baseURL == "" {
		missing = append(missing, "jira.base_url (or JIRA_BASE_URL)")
	}
	email := cfg.Jira.Email
	if email == "" {
		email = os.Getenv("JIRA_EMAIL")
	}
	if email == "" {
		missing = append(missing, "JIRA_EMAIL (or jira.email)")
	}
	if os.Getenv("JIRA_API_TOKEN") == "" {
		missing = append(missing, "JIRA_API_TOKEN")
	}
	if len(missing) > 0 {
		return doctorCheck{
			name:   "jira",
			detail: "missing: " + strings.Join(missing, ", "),
			hint:   "set jira.base_url in .archetipo/config.yaml and export JIRA_EMAIL + JIRA_API_TOKEN; project_key is auto-detected on first run",
		}
	}
	project := cfg.Jira.ProjectKey
	if project == "" {
		project = "auto-detect (dir " + filepath.Base(cfg.ProjectRoot) + ")"
	}
	return doctorCheck{name: "jira", ok: true, detail: fmt.Sprintf("%s project %s (%s)", baseURL, project, email)}
}

func checkPackagedSkills(dataDir string) doctorCheck {
	skillsDir := filepath.Join(dataDir, "skills")
	var missing []string
	for _, sk := range allSkills {
		if _, err := os.Stat(filepath.Join(skillsDir, sk)); err != nil {
			missing = append(missing, sk)
		}
	}
	if len(missing) > 0 {
		return doctorCheck{
			name:   "packaged skills",
			detail: fmt.Sprintf("%d/%d present, missing: %s", len(allSkills)-len(missing), len(allSkills), strings.Join(missing, ", ")),
			hint:   "reinstall the CLI via `npm i -g @techreloaded/archetipo`",
		}
	}
	return doctorCheck{name: "packaged skills", ok: true, detail: fmt.Sprintf("%d/%d present", len(allSkills), len(allSkills))}
}

func checkRuntimeAssets(dataDir string) doctorCheck {
	// Mirror installRuntimeAssets: runtime/ (npm layout) or .archetipo/ (repo layout).
	for _, dir := range []string{filepath.Join(dataDir, "runtime"), filepath.Join(dataDir, ".archetipo")} {
		if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
			return doctorCheck{name: "runtime assets", ok: true, detail: dir}
		}
	}
	return doctorCheck{
		name:   "runtime assets",
		detail: "config.yaml template not found in the package",
		hint:   "reinstall the CLI via `npm i -g @techreloaded/archetipo`",
	}
}

// checkInstalledSkills reports, for each tool directory present in the
// project, how many ARchetipo skills are installed. No tool directory at all
// is a failure: `archetipo init` has not been run here.
func checkInstalledSkills() doctorCheck {
	var found []string
	var stale []string
	for _, t := range allTools {
		if _, err := os.Stat(t.SkillsDir); err != nil {
			continue
		}
		installed := 0
		for _, sk := range allSkills {
			if _, err := os.Stat(filepath.Join(t.SkillsDir, sk)); err == nil {
				installed++
			}
		}
		if installed == 0 {
			continue
		}
		found = append(found, fmt.Sprintf("%s %d/%d", t.Key, installed, len(allSkills)))
		if installed < len(allSkills) {
			stale = append(stale, t.Key)
		}
	}
	if len(found) == 0 {
		return doctorCheck{
			name:   "installed skills",
			detail: "no ARchetipo skills found in any tool directory",
			hint:   "run `archetipo init` in this project",
		}
	}
	if len(stale) > 0 {
		return doctorCheck{
			name:   "installed skills",
			detail: strings.Join(found, ", "),
			hint:   "some skills are missing for " + strings.Join(stale, ", ") + "; re-run `archetipo init` to refresh them",
		}
	}
	return doctorCheck{name: "installed skills", ok: true, detail: strings.Join(found, ", ")}
}

func checkGH(ctx context.Context) doctorCheck {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return doctorCheck{
			name:   "gh",
			detail: "not found in PATH",
			hint:   "install the GitHub CLI: https://cli.github.com",
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gh", "auth", "status").CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return doctorCheck{
			name:   "gh",
			detail: "not authenticated: " + firstLine(detail),
			hint:   "run `gh auth login`",
		}
	}
	return doctorCheck{name: "gh", ok: true, detail: ghPath + " (authenticated)"}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// executionProbeTimeout bounds the network half of the execution diagnosis.
// `doctor` is run when something is already wrong, so a hub that has gone away
// must answer quickly rather than hold the whole report open.
const executionProbeTimeout = 10 * time.Second

// checkExecution diagnoses the configured execution provider in two lines,
// split the way `gh` and `jira` are split elsewhere in this report: what can be
// known locally, and what has to be asked.
//
// The first line is the credential, and needs no network: a provider that names
// an environment variable is unusable the moment that variable is empty, and
// saying so without a round trip means the most common failure is also the
// fastest to diagnose. The second is the provider's own probe, which answers
// the remaining questions at once — is the hub reachable, is the token good, is
// the workspace granted, could any machine take the work.
//
// Before this, `doctor` said nothing at all about remote execution: a project
// configured to dispatch to a fleet passed every check and then failed at the
// first `execution run`.
func checkExecution(
	ctx context.Context,
	cfg config.Config,
	deps executionDependencies,
	offline bool,
) (credential doctorCheck, probe doctorCheck) {
	selection := cfg.Execution.DefaultProvider
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		skipped := "skipped (no default execution provider)"
		return doctorCheck{name: "execution credential", skipped: true, detail: skipped},
			doctorCheck{name: "execution provider", skipped: true, detail: skipped}
	}
	providerID := strings.TrimSpace(selection.ID)
	provider, err := deps.registry.Resolve(providerID)
	if err != nil {
		return doctorCheck{
				name:   "execution credential",
				detail: fmt.Sprintf("provider %q is not registered", providerID),
				hint:   "run `archetipo execution provider list` to see the registered providers",
			}, doctorCheck{
				name:    "execution provider",
				skipped: true,
				detail:  "skipped (provider not registered)",
			}
	}

	credential = checkExecutionCredential(providerID, selection.Config)
	switch {
	case offline:
		probe = doctorCheck{name: "execution provider", skipped: true, detail: "skipped (--offline)"}
	case !credential.ok && !credential.skipped:
		// Probing without a credential would report the same thing twice, and
		// the second report would be the less precise of the two.
		probe = doctorCheck{name: "execution provider", skipped: true, detail: "skipped (credential missing)"}
	default:
		probeCtx, cancel := context.WithTimeout(ctx, executionProbeTimeout)
		defer cancel()
		if err := execution.CheckAvailability(probeCtx, provider, selection.Config); err != nil {
			probe = doctorCheck{
				name:   "execution provider",
				detail: providerID + " cannot dispatch",
				// The provider writes its errors to be read by a person, so the
				// hint is that sentence rather than a paraphrase of it.
				hint: err.Error(),
			}
		} else {
			probe = doctorCheck{
				name:   "execution provider",
				ok:     true,
				detail: providerID + " is ready to dispatch",
			}
		}
	}
	return credential, probe
}

// checkExecutionCredential reports whether the variable the provider names is
// populated. It never reads the value, and never prints it.
func checkExecutionCredential(providerID string, providerConfig map[string]any) doctorCheck {
	tokenEnv, ok := providerConfig["token_env"].(string)
	if strings.TrimSpace(tokenEnv) == "" {
		if !ok && providerConfig["base_url"] == nil {
			// A local provider authenticates through the tool it drives, so
			// there is no variable of ours to look at.
			return doctorCheck{
				name:    "execution credential",
				skipped: true,
				detail:  fmt.Sprintf("skipped (%s needs no credential of its own)", providerID),
			}
		}
		tokenEnv = defaultTokenEnvName
	}
	if strings.TrimSpace(os.Getenv(tokenEnv)) == "" {
		return doctorCheck{
			name:   "execution credential",
			detail: tokenEnv + " is not set",
			hint: "get one with `arcipelago apps create archetipo --workspace <name>` on the hub, " +
				"then `export " + tokenEnv + "=<token>`",
		}
	}
	detail := tokenEnv + " is set"
	if baseURL, ok := providerConfig["base_url"].(string); ok && strings.TrimSpace(baseURL) != "" {
		detail = fmt.Sprintf("%s → %s", tokenEnv, baseURL)
	}
	return doctorCheck{name: "execution credential", ok: true, detail: detail}
}
