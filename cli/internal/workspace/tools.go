// Package workspace owns the initialization of an ARchetipo workspace: what an
// initialization accepts (tools, connectors), where the packaged assets live,
// how the config file is rendered, and how the whole thing is written to disk
// without ever leaving a half-created workspace behind.
//
// It is deliberately the single place where those choices are written down.
// Before this package existed the tool registry and the connector list lived
// inside the `init` command, so any second caller — the viewer, for one — had
// to restate them, and a list restated is a list that drifts.
package workspace

import (
	"fmt"
	"strings"
)

// Tool is a coding agent an ARchetipo workspace can install its skills for.
// SkillsDir is the skills directory relative to the workspace root.
type Tool struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	SkillsDir string `json:"skills_dir"`
}

// tools is the registry. Order is the order every caller shows, so a new tool
// is appended: moving an existing one would move the entry a user picks by
// position in the interactive prompt.
var tools = []Tool{
	{Key: "claude", Name: "Claude Code", SkillsDir: ".claude/skills"},
	{Key: "codex", Name: "Codex", SkillsDir: ".agents/skills"},
	{Key: "cursor", Name: "Cursor", SkillsDir: ".cursor/skills"},
	{Key: "gemini", Name: "Gemini CLI", SkillsDir: ".gemini/skills"},
	{Key: "opencode", Name: "OpenCode", SkillsDir: ".opencode/skills"},
	{Key: "copilot", Name: "GitHub Copilot", SkillsDir: ".github/skills"},
	{Key: "pi", Name: "Pi", SkillsDir: ".pi/skills"},
}

// Tools returns the registered tools. The slice is a copy: a caller that sorts
// or truncates it cannot reach back into the registry.
func Tools() []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	return out
}

// ToolKeys lists the registered tool keys in registration order.
func ToolKeys() []string {
	keys := make([]string, len(tools))
	for i, t := range tools {
		keys[i] = t.Key
	}
	return keys
}

// ToolKeysHint renders the keys for an error hint.
func ToolKeysHint() string {
	return strings.Join(ToolKeys(), ", ")
}

// UnknownToolError is returned by ResolveTools for a key the registry does not
// know. It is a domain error on purpose: the package says what is wrong, and
// the caller decides how that becomes a CLI envelope or an HTTP field error.
type UnknownToolError struct {
	Key string
}

func (e *UnknownToolError) Error() string {
	return fmt.Sprintf("unknown tool: %s", e.Key)
}

// ResolveTools maps keys onto registered tools, preserving the order they were
// given and dropping duplicates. An unknown key is an error, never a silent
// omission.
func ResolveTools(keys []string) ([]Tool, error) {
	byKey := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byKey[t.Key] = t
	}
	seen := make(map[string]struct{}, len(keys))
	out := []Tool{}
	for _, raw := range keys {
		key := strings.ToLower(strings.TrimSpace(raw))
		t, ok := byKey[key]
		if !ok {
			return nil, &UnknownToolError{Key: raw}
		}
		if _, dup := seen[t.Key]; dup {
			continue
		}
		seen[t.Key] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}
