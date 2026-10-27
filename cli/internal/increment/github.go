package increment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// GitHub uses the user's installed gh CLI, never a token stored by ARchetipo.
type GitHub struct{}

func gh(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, iox.NewConnector(iox.CodeConnectorBackend, "GitHub: "+strings.TrimSpace(string(raw)), "check gh authentication and repository access", err)
	}
	return raw, nil
}
func (GitHub) Find(ctx context.Context, dir, branch, base string) (*PullRequest, error) {
	raw, err := gh(ctx, dir, "pr", "list", "--head", branch, "--base", base, "--state", "all", "--json", "url,number,state,headRefOid")
	if err != nil {
		return nil, err
	}
	var prs []PullRequest
	if err = json.Unmarshal(raw, &prs); err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	if len(prs) != 1 {
		return nil, iox.NewConflict("multiple PRs name this increment branch", "resolve the ambiguous PR association before continuing", nil)
	}
	return &prs[0], nil
}

// withBodyFile hands gh a Markdown body through a file, never through argv.
func withBodyFile(body string, use func(path string) error) error {
	file, err := os.CreateTemp("", "archetipo-increment-pr-*.md")
	if err != nil {
		return err
	}
	path := file.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err = file.WriteString(body); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return use(path)
}
func (GitHub) Create(ctx context.Context, dir, branch, base, title, body string) (*PullRequest, error) {
	err := withBodyFile(body, func(path string) error {
		_, err := gh(ctx, dir, "pr", "create", "--head", branch, "--base", base, "--title", title, "--body-file", path)
		return err
	})
	if err != nil {
		return nil, err
	}
	pr, err := (GitHub{}).Find(ctx, dir, branch, base)
	if err == nil && pr == nil {
		return nil, fmt.Errorf("GitHub did not return the created PR")
	}
	return pr, err
}

// Comment posts body once: a comment already carrying its first line (the
// increment marker) is left alone, so a retried publication adds nothing.
func (GitHub) Comment(ctx context.Context, dir string, number int, body string) error {
	raw, err := gh(ctx, dir, "pr", "view", fmt.Sprint(number), "--json", "comments")
	if err != nil {
		return err
	}
	var existing struct {
		Comments []struct {
			Body string `json:"body"`
		} `json:"comments"`
	}
	if err = json.Unmarshal(raw, &existing); err != nil {
		return err
	}
	marker, _, _ := strings.Cut(body, "\n")
	for _, comment := range existing.Comments {
		if strings.Contains(comment.Body, marker) {
			return nil
		}
	}
	return withBodyFile(body, func(path string) error {
		_, err := gh(ctx, dir, "pr", "comment", fmt.Sprint(number), "--body-file", path)
		return err
	})
}
func (GitHub) Read(ctx context.Context, dir string, number int) (*PullRequest, error) {
	raw, err := gh(ctx, dir, "pr", "view", fmt.Sprint(number), "--json", "url,number,state,headRefOid")
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	if err = json.Unmarshal(raw, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}
