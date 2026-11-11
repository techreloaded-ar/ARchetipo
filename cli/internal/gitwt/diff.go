package gitwt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FileDiff is the structured diff of a single file. Status is one of
// "modified", "added", "deleted", "renamed".
type FileDiff struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Status  string `json:"status"`
	Hunks   []Hunk `json:"hunks"`
}

// Hunk is a contiguous block of changes within a file.
type Hunk struct {
	Header string `json:"header"`
	Lines  []Line `json:"lines"`
}

// Line is one line of a hunk. Kind is "context", "add" or "del". OldLine and
// NewLine are 1-based line numbers in the pre/post image (0 when not present on
// that side), used to anchor inline review comments.
type Line struct {
	Kind    string `json:"kind"`
	OldLine int    `json:"old_line"`
	NewLine int    `json:"new_line"`
	Text    string `json:"text"`
}

// Diff returns the structured diff between forkBase and branch, restricted to
// the changes introduced on branch since it diverged from forkBase
// (`git diff <forkBase>...<branch>`), minus the runtime artifacts of the viewer.
func Diff(ctx context.Context, repoRoot, forkBase, branch string) ([]FileDiff, error) {
	args := append([]string{"diff", "--no-color", "--find-renames", forkBase + "..." + branch, "--", "."}, RuntimeArtifactPathspecs...)
	out, err := runGit(ctx, repoRoot, args...)
	if err != nil {
		return nil, err
	}
	return parseUnifiedDiff(out), nil
}

// DiffWorkingTree returns the structured diff between base and the working tree
// (`git diff <base>`), minus the runtime artifacts of the viewer. Used when a
// spec has no recorded branch.
func DiffWorkingTree(ctx context.Context, repoRoot, base string) ([]FileDiff, error) {
	args := append([]string{"diff", "--no-color", "--find-renames", base, "--", "."}, RuntimeArtifactPathspecs...)
	out, err := runGit(ctx, repoRoot, args...)
	if err != nil {
		return nil, err
	}
	// `git diff` is silent about files git does not track yet, and without a
	// worktree or an automatic commit that is every file the work created.
	untracked, err := untrackedFilesDiff(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	return parseUnifiedDiff(out + "\n" + untracked), nil
}

// untrackedFileDiffMaxBytes keeps a generated or vendored file from being read
// into the review whole; such a file is listed by `git status`, not reviewed.
const untrackedFileDiffMaxBytes = 1 << 20

// untrackedFilesDiff renders the untracked, non-ignored text files of the
// working tree as the unified diff of a new file, without touching the index.
// ARchetipo's own directory is left out: an untracked backlog or record is the
// tool's bookkeeping, never the work under review.
func untrackedFilesDiff(ctx context.Context, repoRoot string) (string, error) {
	args := []string{"ls-files", "--others", "--exclude-standard", "-z", "--", ".", ":(exclude).archetipo"}
	out, err := runGit(ctx, repoRoot, args...)
	if err != nil {
		return "", err
	}
	var diff strings.Builder
	for _, path := range strings.Split(out, "\x00") {
		if path == "" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(path)))
		if err != nil || len(content) == 0 || len(content) > untrackedFileDiffMaxBytes || bytes.IndexByte(content, 0) >= 0 {
			continue
		}
		lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
		fmt.Fprintf(&diff, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, path, len(lines))
		for _, line := range lines {
			diff.WriteString("+" + line + "\n")
		}
	}
	return diff.String(), nil
}

// parseUnifiedDiff converts the textual unified diff produced by `git diff`
// into structured FileDiffs. It is tolerant of the metadata lines git emits
// (index, mode, rename from/to, new/deleted file) and tracks old/new line
// numbers across hunks.
func parseUnifiedDiff(text string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var oldLine, newLine int

	flush := func() {
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}

	lines := strings.Split(text, "\n")
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "diff --git "):
			flush()
			old, new := parseDiffGitHeader(ln)
			cur = &FileDiff{OldPath: old, NewPath: new, Status: "modified"}
		case cur == nil:
			// Skip anything before the first file header.
			continue
		case strings.HasPrefix(ln, "new file mode"):
			cur.Status = "added"
		case strings.HasPrefix(ln, "deleted file mode"):
			cur.Status = "deleted"
		case strings.HasPrefix(ln, "rename from "):
			cur.Status = "renamed"
			cur.OldPath = strings.TrimPrefix(ln, "rename from ")
		case strings.HasPrefix(ln, "rename to "):
			cur.Status = "renamed"
			cur.NewPath = strings.TrimPrefix(ln, "rename to ")
		case strings.HasPrefix(ln, "--- "):
			if p := trimDiffPath(strings.TrimPrefix(ln, "--- ")); p != "" {
				cur.OldPath = p
			}
		case strings.HasPrefix(ln, "+++ "):
			if p := trimDiffPath(strings.TrimPrefix(ln, "+++ ")); p != "" {
				cur.NewPath = p
			}
		case strings.HasPrefix(ln, "@@"):
			oldLine, newLine = parseHunkHeader(ln)
			cur.Hunks = append(cur.Hunks, Hunk{Header: ln})
		case strings.HasPrefix(ln, "index ") || strings.HasPrefix(ln, "old mode ") ||
			strings.HasPrefix(ln, "new mode ") || strings.HasPrefix(ln, "similarity index ") ||
			strings.HasPrefix(ln, "dissimilarity index ") || strings.HasPrefix(ln, "Binary files "):
			continue
		case strings.HasPrefix(ln, "\\"):
			// "\ No newline at end of file" — attach to nothing, just skip.
			continue
		default:
			if len(cur.Hunks) == 0 {
				continue
			}
			h := &cur.Hunks[len(cur.Hunks)-1]
			if ln == "" {
				continue
			}
			switch ln[0] {
			case '+':
				h.Lines = append(h.Lines, Line{Kind: "add", NewLine: newLine, Text: ln[1:]})
				newLine++
			case '-':
				h.Lines = append(h.Lines, Line{Kind: "del", OldLine: oldLine, Text: ln[1:]})
				oldLine++
			case ' ':
				h.Lines = append(h.Lines, Line{Kind: "context", OldLine: oldLine, NewLine: newLine, Text: ln[1:]})
				oldLine++
				newLine++
			}
		}
	}
	flush()
	return files
}

// parseDiffGitHeader extracts the a/ and b/ paths from a "diff --git a/x b/y"
// line. Paths with spaces are not quoted by git in this position for the common
// case; we split on " b/" which is unambiguous for the prefixes git uses.
func parseDiffGitHeader(ln string) (old, new string) {
	rest := strings.TrimPrefix(ln, "diff --git ")
	if i := strings.Index(rest, " b/"); i >= 0 {
		old = strings.TrimPrefix(rest[:i], "a/")
		new = strings.TrimPrefix(rest[i+1:], "b/")
		return old, new
	}
	return "", ""
}

// trimDiffPath strips the a/ or b/ prefix from a --- / +++ path and maps
// /dev/null to empty.
func trimDiffPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "/dev/null" {
		return ""
	}
	p = strings.TrimPrefix(p, "a/")
	p = strings.TrimPrefix(p, "b/")
	return p
}

// parseHunkHeader reads the starting old/new line numbers from a hunk header
// like "@@ -12,7 +13,8 @@ optional section heading".
func parseHunkHeader(ln string) (oldStart, newStart int) {
	// Format: @@ -oldStart[,oldCount] +newStart[,newCount] @@
	body := ln
	if i := strings.Index(body, "@@"); i >= 0 {
		body = body[i+2:]
	}
	if i := strings.Index(body, "@@"); i >= 0 {
		body = body[:i]
	}
	for _, tok := range strings.Fields(body) {
		if strings.HasPrefix(tok, "-") {
			oldStart = leadingInt(tok[1:])
		} else if strings.HasPrefix(tok, "+") {
			newStart = leadingInt(tok[1:])
		}
	}
	return oldStart, newStart
}

func leadingInt(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}
