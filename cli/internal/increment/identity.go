package increment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// CommitIdentity is the person a candidate's commits are attributed to: the
// author and the committer of every commit the increment publishes.
type CommitIdentity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (identity CommitIdentity) complete() bool {
	return identity.Name != "" && identity.Email != ""
}

// CommitIdentitySource is an optional capability of a Hosting adapter: the
// identity of the account it publishes with. ok is false when no account is
// configured or it cannot be read, so the caller falls back to Git.
type CommitIdentitySource interface {
	CommitIdentity(ctx context.Context, dir string) (identity CommitIdentity, ok bool)
}

// gitHubIdentityTimeout bounds the two reads of the account: readiness must
// not hang on an unreachable host, it falls back to the Git identity instead.
const gitHubIdentityTimeout = 20 * time.Second

type gitHubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type gitHubEmail struct {
	Email string `json:"email"`
}

// CommitIdentity reads the account gh is signed in with for the host of the
// repository's origin: its name and the first of its email addresses.
func (hosting GitHub) CommitIdentity(ctx context.Context, dir string) (CommitIdentity, bool) {
	originURL, err := git(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return CommitIdentity{}, false
	}
	host := remoteHost(originURL)
	if host == "" {
		return CommitIdentity{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, gitHubIdentityTimeout)
	defer cancel()
	readAPI := hosting.readAPI
	if readAPI == nil {
		readAPI = ghAPI
	}
	rawUser, err := readAPI(ctx, dir, host, "user")
	if err != nil {
		return CommitIdentity{}, false
	}
	var user gitHubUser
	if json.Unmarshal(rawUser, &user) != nil {
		return CommitIdentity{}, false
	}
	// The address list needs the user:email scope, which a default gh login
	// may lack: then the public or the noreply address stands in for it.
	var emails []gitHubEmail
	if rawEmails, emailsErr := readAPI(ctx, dir, host, "user/emails"); emailsErr == nil {
		if json.Unmarshal(rawEmails, &emails) != nil {
			emails = nil
		}
	}
	return chooseGitHubIdentity(host, user, emails)
}

// chooseGitHubIdentity names the account's person: its name, or its login when
// it has none, and the first address of its list, or else its public address,
// or else the noreply address GitHub attributes to that account.
func chooseGitHubIdentity(host string, user gitHubUser, emails []gitHubEmail) (CommitIdentity, bool) {
	login := strings.TrimSpace(user.Login)
	if login == "" {
		return CommitIdentity{}, false
	}
	name := sanitizeIdentityPart(user.Name)
	if name == "" {
		name = sanitizeIdentityPart(login)
	}
	email := ""
	for _, candidate := range emails {
		if email = sanitizeIdentityPart(candidate.Email); email != "" {
			break
		}
	}
	if email == "" {
		email = sanitizeIdentityPart(user.Email)
	}
	if email == "" {
		noreplyDomain := "users.noreply.github.com"
		if host != "" && !strings.EqualFold(host, "github.com") {
			noreplyDomain = "users.noreply." + host
		}
		if user.ID > 0 {
			email = fmt.Sprintf("%d+%s@%s", user.ID, login, noreplyDomain)
		} else {
			email = login + "@" + noreplyDomain
		}
	}
	identity := CommitIdentity{Name: name, Email: email}
	return identity, identity.complete()
}

// sanitizeIdentityPart removes what Git cannot keep in an ident line.
func sanitizeIdentityPart(value string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', '\n', '\r', 0:
			return -1
		}
		return r
	}, value))
}

func ghAPI(ctx context.Context, dir, host, endpoint string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "--hostname", host, endpoint)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %s: %w", endpoint, strings.TrimSpace(stderr.String()), err)
	}
	return out, nil
}

// remoteHost is the network host of a Git remote, empty for a local path.
func remoteHost(remote string) string {
	remote = strings.TrimSpace(remote)
	if strings.Contains(remote, "://") {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Scheme == "file" {
			return ""
		}
		return parsed.Hostname()
	}
	// scp-like syntax: [user@]host:path. A drive letter is a local path.
	colon := strings.Index(remote, ":")
	if colon <= 1 || strings.ContainsAny(remote[:colon], `/\`) {
		return ""
	}
	host := remote[:colon]
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	return host
}

// gitConfigIdentity is the person's own Git identity as Git resolves it from
// configuration files. Variables an agent may have set for its own process,
// such as GIT_AUTHOR_NAME, are deliberately not part of it.
func gitConfigIdentity(ctx context.Context, dir string) (CommitIdentity, bool) {
	name, nameErr := git(ctx, dir, "config", "--get", "user.name")
	email, emailErr := git(ctx, dir, "config", "--get", "user.email")
	if nameErr != nil || emailErr != nil {
		return CommitIdentity{}, false
	}
	identity := CommitIdentity{Name: sanitizeIdentityPart(name), Email: sanitizeIdentityPart(email)}
	return identity, identity.complete()
}

// candidateIdentity chooses who the candidate is attributed to: the account
// the Hosting adapter publishes with, or else the person's Git identity. Without
// either the candidate keeps the identity it was committed with.
func (s *Service) candidateIdentity(ctx context.Context, dir string) (CommitIdentity, bool) {
	if source, ok := s.Hosting.(CommitIdentitySource); ok && source != nil {
		if identity, found := source.CommitIdentity(ctx, dir); found && identity.complete() {
			return identity, true
		}
	}
	return gitConfigIdentity(ctx, dir)
}

// attributeCandidate makes the person the author and committer of every
// commit the increment adds on its own line, before the candidate is frozen.
// Trees, messages and dates are preserved, so the working tree, the index and
// every verification input stay identical: only the attribution changes. The
// base, the base branch and the heads of earlier revisions are never rewritten.
func (s *Service) attributeCandidate(ctx context.Context, r Record) error {
	identity, ok := s.candidateIdentity(ctx, r.Workdir)
	if !ok {
		return nil
	}
	head, err := git(ctx, r.Workdir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	listing := []string{"rev-list", "--first-parent", "--reverse", "HEAD", "^" + r.BaseSHA}
	for _, excluded := range s.preservedCommits(ctx, r) {
		listing = append(listing, "^"+excluded)
	}
	listed, err := git(ctx, r.Workdir, listing...)
	if err != nil {
		return err
	}
	rewritten := map[string]string{}
	newHead := head
	for _, sha := range strings.Fields(listed) {
		commit, readErr := readRawCommit(ctx, r.Workdir, sha)
		if readErr != nil {
			return readErr
		}
		parentsChanged := false
		for index, parent := range commit.parents {
			if replacement, found := rewritten[parent]; found {
				commit.parents[index] = replacement
				parentsChanged = true
			}
		}
		if !parentsChanged && commit.author.matches(identity) && commit.committer.matches(identity) {
			continue
		}
		replacement, writeErr := writeAttributedCommit(ctx, r.Workdir, commit, identity)
		if writeErr != nil {
			return writeErr
		}
		if replacement != sha {
			rewritten[sha] = replacement
		}
		newHead = replacement
	}
	if newHead == head {
		return nil
	}
	_, err = git(ctx, r.Workdir, "update-ref", "-m", "archetipo: candidate attributed to "+identity.Name+" <"+identity.Email+">", "refs/heads/"+r.Branch, newHead, head)
	return err
}

// preservedCommits are commits the attribution must never rewrite, together
// with their history: the base branch, locally and on origin, and the heads
// earlier revisions were verified at.
func (s *Service) preservedCommits(ctx context.Context, r Record) []string {
	candidates := []string{}
	if r.BaseBranch != "" {
		candidates = append(candidates, "refs/heads/"+r.BaseBranch, "refs/remotes/origin/"+r.BaseBranch)
	}
	for _, revision := range r.Revisions {
		if revision.HeadSHA != "" {
			candidates = append(candidates, revision.HeadSHA)
		}
	}
	preserved := []string{}
	for _, candidate := range candidates {
		sha, err := git(ctx, r.Workdir, "rev-parse", "--verify", "--quiet", candidate+"^{commit}")
		if err == nil && sha != "" {
			preserved = append(preserved, sha)
		}
	}
	return preserved
}

type commitIdent struct {
	name  string
	email string
	date  string
}

func (ident commitIdent) matches(identity CommitIdentity) bool {
	return ident.name == identity.Name && ident.email == identity.Email
}

type rawCommit struct {
	tree      string
	parents   []string
	author    commitIdent
	committer commitIdent
	encoding  string
	message   []byte
}

func readRawCommit(ctx context.Context, dir, sha string) (rawCommit, error) {
	cmd := exec.CommandContext(ctx, "git", "cat-file", "commit", sha)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		return rawCommit{}, iox.NewConflict("git cat-file commit "+sha+": "+strings.TrimSpace(stderr.String()), "inspect the increment working directory and retry", err)
	}
	header, message, found := bytes.Cut(raw, []byte("\n\n"))
	if !found {
		header, message = bytes.TrimSuffix(raw, []byte("\n")), nil
	}
	commit := rawCommit{message: message}
	for _, line := range strings.Split(string(header), "\n") {
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "tree":
			commit.tree = value
		case "parent":
			commit.parents = append(commit.parents, value)
		case "author":
			commit.author = parseCommitIdent(value)
		case "committer":
			commit.committer = parseCommitIdent(value)
		case "encoding":
			commit.encoding = value
		}
	}
	if commit.tree == "" {
		return rawCommit{}, iox.NewConflict("commit "+sha+" has no tree", "inspect the increment working directory and retry", nil)
	}
	return commit, nil
}

// parseCommitIdent reads "Name <email> 1700000000 +0100".
func parseCommitIdent(value string) commitIdent {
	open := strings.Index(value, "<")
	closing := strings.LastIndex(value, ">")
	if open < 0 || closing < open {
		return commitIdent{name: strings.TrimSpace(value)}
	}
	return commitIdent{
		name:  strings.TrimSpace(value[:open]),
		email: strings.TrimSpace(value[open+1 : closing]),
		date:  strings.TrimSpace(value[closing+1:]),
	}
}

func writeAttributedCommit(ctx context.Context, dir string, commit rawCommit, identity CommitIdentity) (string, error) {
	args := []string{}
	if commit.encoding != "" {
		args = append(args, "-c", "i18n.commitEncoding="+commit.encoding)
	}
	args = append(args, "commit-tree", commit.tree)
	for _, parent := range commit.parents {
		args = append(args, "-p", parent)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(commit.message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+identity.Name, "GIT_AUTHOR_EMAIL="+identity.Email,
		"GIT_COMMITTER_NAME="+identity.Name, "GIT_COMMITTER_EMAIL="+identity.Email,
	)
	if commit.author.date != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+commit.author.date)
	}
	if commit.committer.date != "" {
		cmd.Env = append(cmd.Env, "GIT_COMMITTER_DATE="+commit.committer.date)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", iox.NewConflict("git commit-tree: "+strings.TrimSpace(stderr.String()), "inspect the increment working directory and retry", err)
	}
	return strings.TrimSpace(string(out)), nil
}
