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
func (GitHub) Create(ctx context.Context, dir, branch, base, title, body string) (*PullRequest, error) {
	file, err := os.CreateTemp("", "archetipo-increment-pr-*.md")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err = file.WriteString(body); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	if _, err = gh(ctx, dir, "pr", "create", "--head", branch, "--base", base, "--title", title, "--body-file", path); err != nil {
		return nil, err
	}
	pr, err := (GitHub{}).Find(ctx, dir, branch, base)
	if err == nil && pr == nil {
		return nil, fmt.Errorf("GitHub did not return the created PR")
	}
	return pr, err
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
