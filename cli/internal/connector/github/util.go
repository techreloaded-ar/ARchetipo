package github

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// codeFromTitle returns the US-NNN code from an issue title like
// "US-001: Login utente". Returns "" when no code is present.
var codeRegexp = regexp.MustCompile(`^(US-\d+):`)

func codeFromTitle(title string) string {
	m := codeRegexp.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	return m[1]
}

func titleAfterCode(title string) string {
	idx := strings.Index(title, ":")
	if idx == -1 {
		return title
	}
	return strings.TrimSpace(title[idx+1:])
}

// taskIDFromTitle parses "TASK-01: Schema DB" into "TASK-01".
var taskRegexp = regexp.MustCompile(`^(TASK-\d+):`)

func taskIDFromTitle(title string) string {
	m := taskRegexp.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	return m[1]
}

func titleAfterTaskID(title string) string {
	idx := strings.Index(title, ":")
	if idx == -1 {
		return title
	}
	return strings.TrimSpace(title[idx+1:])
}

// epicCodeFromLabel parses "EP-001: [Foundations]" into "EP-001".
var epicCodeRegexp = regexp.MustCompile(`^(EP-\d+):`)

func epicCodeFromLabel(label string) string {
	m := epicCodeRegexp.FindStringSubmatch(label)
	if m == nil {
		return label
	}
	return m[1]
}

// epicTitleFromLabel extracts the bracketed title from "EP-001: [Foundations]".
func epicTitleFromLabel(label string) string {
	open := strings.Index(label, "[")
	close := strings.LastIndex(label, "]")
	if open == -1 || close == -1 || close <= open+1 {
		return ""
	}
	return label[open+1 : close]
}

// writeFile is a duplicate of filefs.writeFile to avoid an import cycle when
// the github connector also needs to persist a local file (PRD).
func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

