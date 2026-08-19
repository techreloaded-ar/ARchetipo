package workspace

import (
	"strconv"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// RenderInput carries the values an initialization writes into the packaged
// config.yaml template. Everything else in that file — comments included —
// survives untouched.
type RenderInput struct {
	Connector string
	Paths     domain.ConfigPaths
	Worktree  domain.WorktreeConfig
	Wiki      bool
	Template  template.Template
}

// RenderConfig applies the chosen values onto the packaged config.yaml body.
//
// The edits are deliberately textual rather than a yaml.Node round-trip: the
// packaged template is a documented file, and re-marshalling it would drop
// every comment and reflow the blank lines — an observable change to what a
// plain `archetipo init` produces today.
func RenderConfig(templateBody string, in RenderInput) string {
	out := setConnectorField(templateBody, in.Connector)
	out = setPathsFields(out, in.Paths)
	out = setWorktreeFields(out, in.Worktree)
	out = setWikiEnabledField(out, in.Wiki)
	out = setTemplateFields(out, in.Template.ID, in.Template.Version)
	return out
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

// setPathsFields rewrites the four shared paths inside the top-level `paths:`
// mapping. An empty value is left alone: the caller has already substituted
// the defaults, so an empty string here would mean "erase the key", which is
// never what an initialization wants.
func setPathsFields(body string, paths domain.ConfigPaths) string {
	return setMappingFields(body, "paths", []mappingField{
		{Key: "prd", Value: paths.PRD},
		{Key: "wiki", Value: paths.Wiki},
		{Key: "mockups", Value: paths.Mockups},
		{Key: "test_results", Value: paths.TestResults},
	})
}

// setWorktreeFields rewrites the per-spec worktree settings inside the
// top-level `worktree:` mapping.
func setWorktreeFields(body string, worktree domain.WorktreeConfig) string {
	return setMappingFields(body, "worktree", []mappingField{
		{Key: "enabled", Value: strconv.FormatBool(worktree.Enabled), Always: true},
		{Key: "base", Value: worktree.Base},
		{Key: "dir", Value: worktree.Dir},
		{Key: "branch_prefix", Value: worktree.BranchPrefix},
	})
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
// asset). The version is quoted so a value like "1.0" stays a string.
func setTemplateFields(body, id, version string) string {
	return setMappingFields(body, "template", []mappingField{
		{Key: "id", Value: id},
		{Key: "version", Value: strconv.Quote(version)},
	})
}

// mappingField is one key to write inside a top-level YAML mapping. Value is
// the YAML scalar written verbatim, so a caller that needs quoting quotes it
// itself. Always forces the write even when Value is empty, which is what
// makes a `false` boolean distinguishable from "leave this key alone".
type mappingField struct {
	Key    string
	Value  string
	Always bool
}

// setMappingFields rewrites the given keys inside a top-level YAML mapping,
// preserving the indentation and the comments already in the file. Keys the
// section does not carry are inserted after its last child; a section that is
// absent altogether is appended at the end of the document.
//
// It is deliberately textual for the same reason RenderConfig is: the packaged
// template is documentation as much as configuration.
func setMappingFields(body, section string, fields []mappingField) string {
	wanted := make([]mappingField, 0, len(fields))
	for _, f := range fields {
		if f.Always || strings.TrimSpace(f.Value) != "" {
			wanted = append(wanted, f)
		}
	}
	if len(wanted) == 0 {
		return body
	}

	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == section+":" {
			start = i
			break
		}
	}
	if start < 0 {
		var b strings.Builder
		b.WriteString(strings.TrimRight(body, "\n"))
		b.WriteString("\n\n" + section + ":\n")
		for _, f := range wanted {
			b.WriteString("  " + f.Key + ": " + f.Value + "\n")
		}
		return b.String()
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

	byKey := make(map[string]mappingField, len(wanted))
	order := make([]string, 0, len(wanted))
	for _, f := range wanted {
		byKey[f.Key] = f
		order = append(order, f.Key)
	}

	seen := make(map[string]bool, len(wanted))
	lastChild := start
	for i := start + 1; i < end; i++ {
		content := strings.TrimSpace(lines[i])
		if content == "" {
			continue
		}
		lastChild = i
		if strings.HasPrefix(content, "#") {
			continue
		}
		key, _, ok := strings.Cut(content, ":")
		if !ok {
			continue
		}
		f, wantsIt := byKey[strings.TrimSpace(key)]
		if !wantsIt {
			continue
		}
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		lines[i] = indent + f.Key + ": " + f.Value
		seen[f.Key] = true
	}

	var missing []string
	for _, key := range order {
		if seen[key] {
			continue
		}
		f := byKey[key]
		missing = append(missing, "  "+f.Key+": "+f.Value)
	}
	if len(missing) > 0 {
		tail := append([]string{}, lines[lastChild+1:]...)
		lines = append(lines[:lastChild+1], append(missing, tail...)...)
	}
	return strings.Join(lines, "\n")
}
