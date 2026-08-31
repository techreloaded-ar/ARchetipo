package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/workspace"
)

// toolDef and allTools are the workspace tool registry, aliased here so
// `doctor` and `uninstall` keep reading the names they always have. The
// registry itself lives in internal/workspace: it is the single place that
// says what an initialization accepts, CLI and viewer alike.
type toolDef = workspace.Tool

var allTools = workspace.Tools()

func validToolKeysHint() string {
	return workspace.ToolKeysHint()
}

// allSkills is the skill set of the default process Template. The Template
// package is the single place where a process is written down; `uninstall` and
// `doctor` keep reading this variable.
var allSkills = template.Default().Skills

func newInitProjectCmd(s streams) *cobra.Command {
	var toolFlags []string
	var connectorFlag string
	var templateFlag string
	var assumeYes bool
	var withWiki bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install ARchetipo skills into the project",
		Long: "Copies the skills of the selected process Template into the chosen tool directories. " +
			"Also creates .archetipo/config.yaml and .archetipo/shared-runtime.md in the current directory.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInitProject(s, toolFlags, connectorFlag, templateFlag, assumeYes, withWiki)
		},
	}
	cmd.Flags().StringSliceVar(&toolFlags, "tool", nil, "Tool key(s) to install for: "+validToolKeysHint()+". Repeat or comma-separate.")
	cmd.Flags().StringVar(&connectorFlag, "connector", "", "Connector for .archetipo/config.yaml: file|github|jira")
	cmd.Flags().StringVar(&templateFlag, "template", "", "Process Template id (default: "+template.DefaultID+")")
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "Assume 'yes' to overwrite prompts (non-interactive).")
	cmd.Flags().BoolVar(&withWiki, "wiki", false, "Write wiki.enabled: true, so the standard workflow maintains the Living Wiki. Off by default; the archetipo-wiki skill is installed either way and stays usable on demand.")
	return cmd
}

func runInitProject(s streams, toolFlags []string, connectorFlag, templateFlag string, assumeYes, withWiki bool) error {
	// Resolved first, before anything on disk is created or written: an unknown
	// Template must be rejected without leaving a partial initialization behind.
	tpl, err := template.Resolve(strings.TrimSpace(templateFlag))
	if err != nil {
		return iox.NewInvalidInput(
			"unknown template: "+strings.TrimSpace(templateFlag),
			"valid: "+template.DefaultID,
			err,
		)
	}

	dataDir, err := discoverDataDir()
	if err != nil {
		return err
	}
	skillsDir := filepath.Join(dataDir, "skills")
	runtimeDir := filepath.Join(dataDir, "runtime")

	if _, statErr := os.Stat(skillsDir); statErr != nil {
		return iox.NewPrecondition(
			"skills directory not found",
			"set ARCHETIPO_DATA_DIR to the package root, or reinstall the CLI via `npm i -g @techreloaded/archetipo`",
			statErr,
		)
	}

	tools, err := resolveToolFlags(toolFlags)
	if err != nil {
		return err
	}
	if len(tools) == 0 {
		tools, err = pickToolsInteractive(s)
		if err != nil {
			return err
		}
	}
	if len(tools) == 0 {
		fmt.Fprintln(s.out, "No tools selected.")
		return nil
	}

	var conn string
	if connectorFlag != "" {
		if !workspace.IsConnector(connectorFlag) {
			return iox.NewInvalidInput("--connector must be 'file', 'github' or 'jira'", "", nil)
		}
		conn = connectorFlag
	} else {
		conn, err = pickConnectorInteractive(s)
		if err != nil {
			return err
		}
	}

	fmt.Fprintln(s.out, "Installing...")
	for _, t := range tools {
		target := t.SkillsDir
		if err := os.MkdirAll(target, 0o755); err != nil {
			return iox.NewInternal("cannot create "+target, err)
		}
		for _, sk := range tpl.Skills {
			src := filepath.Join(skillsDir, sk)
			dst := filepath.Join(target, sk)
			if _, err := os.Stat(src); err != nil {
				return iox.NewPrecondition("skill missing in package: "+sk, "reinstall the CLI", err)
			}
			if err := os.RemoveAll(dst); err != nil {
				return iox.NewInternal("cannot clean "+dst, err)
			}
			if err := workspace.CopyTree(src, dst); err != nil {
				return iox.NewInternal("copy "+sk, err)
			}
		}
		fmt.Fprintf(s.out, "  ✓ %s → %s\n", t.Name, target)
	}

	if err := installRuntimeAssets(s, runtimeDir, conn, tpl, assumeYes, withWiki); err != nil {
		return err
	}

	fmt.Fprintln(s.out, "Done.")
	if withWiki {
		fmt.Fprintln(s.out, "Automatic Wiki enabled (wiki.enabled: true).")
		fmt.Fprintln(s.out, "Next: run /archetipo-wiki bootstrap")
		return nil
	}
	// The skill is installed regardless: the gate only stops the standard
	// workflow from maintaining a Wiki, never the on-demand use of one.
	fmt.Fprintln(s.out, "Automatic Wiki disabled (wiki.enabled: false, the default). Run /archetipo-wiki explicitly to build or query one, or re-run init with --wiki.")
	fmt.Fprintln(s.out, "Next: run /archetipo-inception")
	return nil
}

// discoverDataDir locates the directory containing skills/ and runtime/. The
// resolution lives in internal/workspace, which the viewer uses as well.
func discoverDataDir() (string, error) {
	return workspace.DiscoverDataDir()
}

func resolveToolFlags(flags []string) ([]toolDef, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out, err := workspace.ResolveTools(flags)
	if err != nil {
		var unknown *workspace.UnknownToolError
		if errors.As(err, &unknown) {
			return nil, iox.NewInvalidInput("unknown tool: "+unknown.Key, "valid: "+validToolKeysHint(), nil)
		}
		return nil, err
	}
	return out, nil
}

func pickToolsInteractive(s streams) ([]toolDef, error) {
	options := allTools
	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, "Select tools to install for:")
	fmt.Fprintln(s.out)
	for i, t := range options {
		fmt.Fprintf(s.out, "  %d) %s\n", i+1, t.Name)
	}
	fmt.Fprintln(s.out)
	fmt.Fprint(s.out, "Numbers separated by spaces, or 'all' (Enter to cancel): ")

	line, err := readLine(s.in)
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	if strings.EqualFold(line, "all") {
		return options, nil
	}
	seen := map[string]struct{}{}
	out := []toolDef{}
	for _, tok := range strings.Fields(line) {
		idx, err := strconv.Atoi(tok)
		if err != nil || idx < 1 || idx > len(options) {
			return nil, iox.NewInvalidInput("invalid selection: "+tok, "use numbers from the list or 'all'", nil)
		}
		t := options[idx-1]
		if _, dup := seen[t.Key]; dup {
			continue
		}
		seen[t.Key] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

func pickConnectorInteractive(s streams) (string, error) {
	fmt.Fprintln(s.out)
	fmt.Fprintln(s.out, "Select connector:")
	fmt.Fprintln(s.out, "  1) file    — backlog and planning as local Markdown files")
	fmt.Fprintln(s.out, "  2) github  — GitHub Projects v2 (requires gh CLI)")
	fmt.Fprintln(s.out, "  3) jira    — Jira Cloud (requires JIRA_EMAIL/JIRA_API_TOKEN)")
	fmt.Fprintln(s.out)
	fmt.Fprint(s.out, "Choice [1]: ")
	line, err := readLine(s.in)
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	switch line {
	case "", "1", "file":
		return "file", nil
	case "2", "github":
		return "github", nil
	case "3", "jira":
		return "jira", nil
	}
	return "", iox.NewInvalidInput("invalid connector choice: "+line, "enter 1, 2 or 3", nil)
}

func installRuntimeAssets(s streams, runtimeDir, connector string, tpl template.Template, assumeYes, withWiki bool) error {
	root := runtimeDir
	if _, err := os.Stat(filepath.Join(root, "config.yaml")); err != nil {
		// dataDir/runtime missing -> try repo .archetipo/
		alt := filepath.Join(filepath.Dir(runtimeDir), ".archetipo")
		if _, err := os.Stat(filepath.Join(alt, "config.yaml")); err == nil {
			root = alt
		} else {
			return iox.NewPrecondition("runtime assets not found", "package may be incomplete; reinstall the CLI", err)
		}
	}

	if err := os.MkdirAll(".archetipo", 0o755); err != nil {
		return iox.NewInternal("cannot create .archetipo/", err)
	}

	configPath := ".archetipo/config.yaml"
	if _, err := os.Stat(configPath); err == nil {
		overwrite := assumeYes
		if !overwrite {
			fmt.Fprintf(s.out, "\n  ! .archetipo/config.yaml already exists. Overwrite? [s/N] ")
			line, err := readLine(s.in)
			if err != nil {
				return err
			}
			ans := strings.ToLower(strings.TrimSpace(line))
			overwrite = ans == "s" || ans == "y"
		}
		if !overwrite {
			// Declining the overwrite must leave the config untouched, template
			// block included.
			fmt.Fprintln(s.out, "  config left unchanged")
		} else {
			// The overwrite discards every customization in the file, so the
			// previous config is saved first: it is the only way back.
			backupPath, err := backupExistingConfig(configPath)
			if err != nil {
				return err
			}
			if err := writeConfig(filepath.Join(root, "config.yaml"), configPath, connector, tpl, withWiki); err != nil {
				return err
			}
			fmt.Fprintf(s.out, "  ✓ backup of the previous config: %s\n", backupPath)
			printConfigWritten(s, connector, tpl)
		}
	} else {
		if err := writeConfig(filepath.Join(root, "config.yaml"), configPath, connector, tpl, withWiki); err != nil {
			return err
		}
		printConfigWritten(s, connector, tpl)
	}

	sharedSrc := filepath.Join(root, "shared-runtime.md")
	sharedDst := ".archetipo/shared-runtime.md"
	if _, err := os.Stat(sharedSrc); err == nil {
		if err := workspace.CopyFile(sharedSrc, sharedDst); err != nil {
			return iox.NewInternal("copy shared-runtime.md", err)
		}
		fmt.Fprintln(s.out, "  ✓ .archetipo/shared-runtime.md")
	}
	return nil
}

// backupExistingConfig copies the current config next to it, with a timestamped
// name so that repeated init runs never clobber an earlier backup. Returns the
// path of the file that was written.
func backupExistingConfig(configPath string) (string, error) {
	backupPath := uniqueBackupPath(configPath, time.Now())
	if err := workspace.CopyFile(configPath, backupPath); err != nil {
		return "", iox.NewInternal("cannot back up "+configPath, err)
	}
	return backupPath, nil
}

// uniqueBackupPath builds `<config>.backup-<timestamp>`, adding a numeric suffix
// when two init runs land within the same second.
func uniqueBackupPath(configPath string, now time.Time) string {
	base := configPath + ".backup-" + now.Format("20060102-150405")
	candidate := base
	for attempt := 2; ; attempt++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = base + "-" + strconv.Itoa(attempt)
	}
}

func printConfigWritten(s streams, connector string, tpl template.Template) {
	fmt.Fprintf(s.out, "  ✓ .archetipo/config.yaml (connector: %s)\n", connector)
	fmt.Fprintf(s.out, "  ✓ template: %s (%s %s)\n", tpl.Label, tpl.ID, tpl.Version)
}

func writeConfig(src, dst, connector string, tpl template.Template, withWiki bool) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return iox.NewInternal("read config template", err)
	}
	defaults := config.Default()
	out := workspace.RenderConfig(string(body), workspace.RenderInput{
		Connector: connector,
		Paths:     defaults.Paths,
		Worktree:  defaults.Worktree,
		Wiki:      withWiki,
		Template:  tpl,
	})
	if err := os.WriteFile(dst, []byte(out), 0o644); err != nil {
		return iox.NewInternal("write "+dst, err)
	}
	return nil
}

func readLine(r io.Reader) (string, error) {
	if r == nil {
		return "", errNonInteractiveInput(errors.New("no input stream"))
	}
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return "", errNonInteractiveInput(err)
		}
		return "", err
	}
	return line, nil
}

// errNonInteractiveInput explains how to run init without a terminal (CI,
// piped stdin) instead of surfacing a bare EOF.
func errNonInteractiveInput(cause error) error {
	return iox.NewPrecondition(
		"interactive input is not available",
		"run non-interactively: archetipo init --tool <"+strings.ReplaceAll(validToolKeysHint(), ", ", "|")+"> --connector <file|github|jira> [--yes]",
		cause,
	)
}
