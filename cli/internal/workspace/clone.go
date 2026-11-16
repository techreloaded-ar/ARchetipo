package workspace

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
)

// EnvCloneDir names the directory where workspaces are cloned from a
// repository URL. Cloning is offered only where it is set: in the isolated
// runtime it is a directory of the environment's volume, so the person adds a
// project without ever choosing a path on a disk they cannot see.
const EnvCloneDir = "ARCHETIPO_CLONE_DIR"

// Refusal codes of a clone, rendered per field like the other workspace ones.
const (
	CodeCloneURLInvalid     = "WORKSPACE_CLONE_URL_INVALID"
	CodeCloneDestination    = "WORKSPACE_CLONE_DESTINATION_EXISTS"
	CodeCloneUnreachable    = "WORKSPACE_CLONE_UNREACHABLE"
	CodeCloneNotOffered     = "WORKSPACE_CLONE_NOT_OFFERED"
	cloneTimeout            = 15 * time.Minute
	cloneFieldURL           = "url"
	maxRepositoryNameLength = 100
)

// allowedCloneSchemes is a variable only so tests can clone a local fixture.
var allowedCloneSchemes = map[string]bool{"https": true}

var safeRepositoryName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// CloneDir returns the configured clone directory, or "" when cloning is not
// offered by this process.
func CloneDir() string {
	return strings.TrimSpace(os.Getenv(EnvCloneDir))
}

// CloneResult reports where the repository landed and whether it is already an
// ARchetipo workspace. A repository that is not one still has to be
// initialized with the person's choices.
type CloneResult struct {
	Dir         string `json:"dir"`
	Name        string `json:"name"`
	Initialized bool   `json:"initialized"`
}

// Clone clones rawURL into baseDir/<repository name>. It never overwrites an
// existing directory, never stores credentials in the URL, and never waits for
// a terminal prompt: a repository that needs access the environment lacks is
// refused with a reason.
func Clone(ctx context.Context, baseDir, rawURL string) (CloneResult, error) {
	if baseDir == "" {
		return CloneResult{}, &ValidationError{Fields: []FieldError{{Field: cloneFieldURL, Code: CodeCloneNotOffered, Message: "adding a project from a repository URL is not offered here"}}}
	}
	name, err := repositoryName(rawURL)
	if err != nil {
		return CloneResult{}, err
	}
	dest := filepath.Join(baseDir, name)
	if _, statErr := os.Lstat(dest); statErr == nil {
		return CloneResult{}, &ValidationError{Fields: []FieldError{{Field: cloneFieldURL, Code: CodeCloneDestination, Message: dest + " already exists: add it as an existing workspace or remove it"}}}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return CloneResult{}, statErr
	}
	if err = os.MkdirAll(baseDir, 0o700); err != nil {
		return CloneResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--", strings.TrimSpace(rawURL), dest)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		_ = os.RemoveAll(dest)
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		return CloneResult{}, &ValidationError{Fields: []FieldError{{Field: cloneFieldURL, Code: CodeCloneUnreachable, Message: "the repository could not be cloned (a private repository needs Git host access): " + detail}}}
	}
	_, statErr := os.Stat(filepath.Join(dest, config.RelativePath))
	return CloneResult{Dir: dest, Name: name, Initialized: statErr == nil}, nil
}

func repositoryName(rawURL string) (string, error) {
	invalid := func(message string) error {
		return &ValidationError{Fields: []FieldError{{Field: cloneFieldURL, Code: CodeCloneURLInvalid, Message: message}}}
	}
	trimmed := strings.TrimSpace(rawURL)
	u, err := url.Parse(trimmed)
	if trimmed == "" || err != nil || !allowedCloneSchemes[u.Scheme] {
		return "", invalid("use an https URL of a Git repository, for example https://github.com/owner/repository")
	}
	if u.User != nil {
		return "", invalid("the URL must not contain a user name or token")
	}
	if u.Scheme != "file" && u.Host == "" {
		return "", invalid("the URL has no host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", invalid("the URL must not contain a query or a fragment")
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(u.Path, "/")), ".git")
	if !safeRepositoryName.MatchString(name) || len(name) > maxRepositoryNameLength {
		return "", invalid("the URL does not end with a repository name")
	}
	return name, nil
}
