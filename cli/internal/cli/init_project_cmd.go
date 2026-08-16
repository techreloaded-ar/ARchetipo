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

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

type toolDef struct {
	Key         string
	Name        string
	ProjectPath string // relative to cwd, e.g. ".claude/skills"
}

var allTools = []toolDef{
	{Key: "claude", Name: "Claude Code", ProjectPath: ".claude/skills"},
	{Key: "codex", Name: "Codex", ProjectPath: ".agents/skills"},
	{Key: "cursor", Name: "Cursor", ProjectPath: ".cursor/skills"},
	{Key: "gemini", Name: "Gemini CLI", ProjectPath: ".gemini/skills"},
	{Key: "opencode", Name: "OpenCode", ProjectPath: ".opencode/skills"},
	{Key: "copilot", Name: "GitHub Copilot", ProjectPath: ".github/skills"},
	{Key: "pi", Name: "Pi", ProjectPath: ".pi/skills"},
}

func validToolKeysHint() string {
	keys := make([]string, len(allTools))
	for i, t := range allTools {
		keys[i] = t.Key
	}
	return strings.Join(keys, ", ")
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
			"valid: "+strings.Join(template.Builtin().IDs(), ", "),
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
		if connectorFlag != "file" && connectorFlag != "github" && connectorFlag != "jira" {
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
		target := t.ProjectPath
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
			if err := copyTree(src, dst); err != nil {
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

// discoverDataDir returns the directory containing skills/ and runtime/.
// Resolution order:
//  1. ARCHETIPO_DATA_DIR env var (set by the npm shim)
//  2. directory of the running binary, looking for skills/ alongside or in parent
//  3. the repo layout when running from source (skills/ + .archetipo/ at repo root)
func discoverDataDir() (string, error) {
	if env := strings.TrimSpace(os.Getenv("ARCHETIPO_DATA_DIR")); env != "" {
		if _, err := os.Stat(filepath.Join(env, "skills")); err == nil {
			return env, nil
		}
	}
	exe, err := os.Executable()
	if err == nil {
		resolved, _ := filepath.EvalSymlinks(exe)
		if resolved != "" {
			exe = resolved
		}
		for _, base := range []string{filepath.Dir(exe), filepath.Dir(filepath.Dir(exe))} {
			if _, err := os.Stat(filepath.Join(base, "skills")); err == nil {
				return base, nil
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if _, statErr := os.Stat(filepath.Join(cwd, "skills")); statErr == nil {
			return repoFallbackDataDir(cwd), nil
		}
	}
	return "", iox.NewPrecondition(
		"could not locate ARchetipo data directory",
		"set ARCHETIPO_DATA_DIR or reinstall via `npm i -g @techreloaded/archetipo`",
		nil,
	)
}

// repoFallbackDataDir maps the repo layout (skills/ + .archetipo/) onto the
// expected runtime layout by treating .archetipo/ as runtime/.
func repoFallbackDataDir(repoRoot string) string {
	// The init code uses dataDir/skills and dataDir/runtime, while the repo
	// has skills/ and .archetipo/. We return the repo root and let
	// installRuntimeAssets look in either runtime/ or .archetipo/.
	return repoRoot
}

func resolveToolFlags(flags []string) ([]toolDef, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	keysByKey := map[string]toolDef{}
	for _, t := range allTools {
		keysByKey[t.Key] = t
	}
	seen := map[string]struct{}{}
	out := []toolDef{}
	for _, raw := range flags {
		key := strings.ToLower(strings.TrimSpace(raw))
		t, ok := keysByKey[key]
		if !ok {
			return nil, iox.NewInvalidInput("unknown tool: "+raw, "valid: "+validToolKeysHint(), nil)
		}
		if _, dup := seen[t.Key]; dup {
			continue
		}
		seen[t.Key] = struct{}{}
		out = append(out, t)
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
		if err := copyFile(sharedSrc, sharedDst); err != nil {
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
	if err := copyFile(configPath, backupPath); err != nil {
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
	out := setConnectorField(string(body), connector)
	if withWiki {
		out = setWikiEnabledField(out, true)
	}
	out = setTemplateFields(out, tpl.ID, tpl.Version)
	if err := os.WriteFile(dst, []byte(out), 0o644); err != nil {
		return iox.NewInternal("write "+dst, err)
	}
	return nil
}

// setConnectorField rewrites the top-level `connector:` line of the YAML
// template. It only touches lines that look like `connector:` at column 0 to
// avoid clobbering nested keys.
func setConnectorField(body, connector string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trim := strings.TrimRight(line, "\r")
		if strings.HasPrefix(trim, "connector:") {
			lines[i] = "connector: " + connector
			return strings.Join(lines, "\n")
		}
	}
	// no existing field -> prepend
	return "connector: " + connector + "\n" + body
}

// setWikiEnabledField rewrites `enabled:` inside the top-level `wiki:` mapping
// of the YAML template, preserving the surrounding comments. The key is written
// explicitly rather than left to the default because an omitted `wiki.enabled`
// resolves to false: only a literal `true` in the file turns the gate on.
// When the template carries no `wiki:` section, the section is appended.
func setWikiEnabledField(body string, enabled bool) string {
	value := "false"
	if enabled {
		value = "true"
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, "\r") != "wiki:" {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(strings.TrimRight(lines[j], "\r"))
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			// A non-indented line ends the mapping: the key is absent.
			if !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") {
				break
			}
			if strings.HasPrefix(trimmed, "enabled:") {
				lines[j] = "  enabled: " + value
				return strings.Join(lines, "\n")
			}
		}
		// `wiki:` exists without `enabled:`: insert it as the first child.
		rest := append([]string{"  enabled: " + value}, lines[i+1:]...)
		return strings.Join(append(lines[:i+1], rest...), "\n")
	}
	separator := "\n"
	if strings.HasSuffix(body, "\n") {
		separator = ""
	}
	return body + separator + "\nwiki:\n  enabled: " + value + "\n"
}

// setTemplateFields rewrites `template.id` and `template.version` in the YAML
// template, appending the whole block when the key is absent (an older packaged
// asset). Deliberately textual, like setConnectorField: a yaml.Node round-trip
// would reflow indentation and blank lines of the generated config, which is an
// observable change to what a plain initialization produces today.
func setTemplateFields(body, id, version string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == "template:" {
			start = i
			break
		}
	}
	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\ntemplate:\n  id: " + id + "\n  version: \"" + version + "\"\n"
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimRight(lines[i], "\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		if trimmed[0] != ' ' && trimmed[0] != '\t' {
			end = i
			break
		}
	}

	sawID, sawVersion, lastChild := false, false, start
	for i := start + 1; i < end; i++ {
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		switch content := strings.TrimSpace(lines[i]); {
		case strings.HasPrefix(content, "id:"):
			lines[i] = indent + "id: " + id
			sawID, lastChild = true, i
		case strings.HasPrefix(content, "version:"):
			lines[i] = indent + "version: \"" + version + "\""
			sawVersion, lastChild = true, i
		case content != "":
			lastChild = i
		}
	}

	var missing []string
	if !sawID {
		missing = append(missing, "  id: "+id)
	}
	if !sawVersion {
		missing = append(missing, "  version: \""+version+"\"")
	}
	if len(missing) > 0 {
		lines = append(lines[:lastChild+1], append(missing, lines[lastChild+1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
