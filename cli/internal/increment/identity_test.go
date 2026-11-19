package increment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestChooseGitHubIdentityUsesAccountNameAndFirstEmail(t *testing.T) {
	user := gitHubUser{ID: 42, Login: "mrossi", Name: "Mario Rossi", Email: "public@example.test"}
	emails := []gitHubEmail{{Email: "first@example.test"}, {Email: "second@example.test"}}
	got, ok := chooseGitHubIdentity("github.com", user, emails)
	if !ok || got != (CommitIdentity{Name: "Mario Rossi", Email: "first@example.test"}) {
		t.Fatalf("got %+v %v", got, ok)
	}
}

func TestChooseGitHubIdentityFallbacks(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		user   gitHubUser
		emails []gitHubEmail
		want   CommitIdentity
	}{
		{"unreadable email list uses public email", "github.com", gitHubUser{ID: 42, Login: "mrossi", Name: "Mario Rossi", Email: "public@example.test"}, nil, CommitIdentity{"Mario Rossi", "public@example.test"}},
		{"blank first entry is skipped", "github.com", gitHubUser{ID: 42, Login: "mrossi", Name: "Mario Rossi"}, []gitHubEmail{{Email: " "}, {Email: "real@example.test"}}, CommitIdentity{"Mario Rossi", "real@example.test"}},
		{"no address uses noreply", "github.com", gitHubUser{ID: 42, Login: "mrossi", Name: "Mario Rossi"}, nil, CommitIdentity{"Mario Rossi", "42+mrossi@users.noreply.github.com"}},
		{"no name uses login", "github.com", gitHubUser{ID: 42, Login: "mrossi"}, []gitHubEmail{{Email: "first@example.test"}}, CommitIdentity{"mrossi", "first@example.test"}},
		{"enterprise host noreply", "git.example.test", gitHubUser{ID: 7, Login: "mrossi"}, nil, CommitIdentity{"mrossi", "7+mrossi@users.noreply.git.example.test"}},
		{"name stripped of ident delimiters", "github.com", gitHubUser{ID: 42, Login: "mrossi", Name: " Mario <Rossi>\n"}, []gitHubEmail{{Email: "first@example.test"}}, CommitIdentity{"Mario Rossi", "first@example.test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := chooseGitHubIdentity(tc.host, tc.user, tc.emails)
			if !ok || got != tc.want {
				t.Fatalf("got %+v %v, want %+v", got, ok, tc.want)
			}
		})
	}
}

func TestChooseGitHubIdentityWithoutAccount(t *testing.T) {
	if got, ok := chooseGitHubIdentity("github.com", gitHubUser{}, []gitHubEmail{{Email: "first@example.test"}}); ok {
		t.Fatalf("an account without login produced %+v", got)
	}
}

func TestRemoteHost(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/owner/repo.git":     "github.com",
		"https://user@github.com/owner/repo":    "github.com",
		"ssh://git@git.example.test:22/o/r.git": "git.example.test",
		"git@github.com:owner/repo.git":         "github.com",
		"file:///srv/origin.git":                "",
		"/srv/origin.git":                       "",
		`C:\Users\me\origin.git`:                "",
		"C:/Users/me/origin.git":                "",
		"../origin.git":                         "",
	} {
		if got := remoteHost(remote); got != want {
			t.Errorf("remoteHost(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestGitHubCommitIdentityReadsAccountOfOriginHost(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/owner/repo.git")
	var calls []string
	hosting := GitHub{readAPI: func(_ context.Context, _, host, endpoint string) ([]byte, error) {
		calls = append(calls, host+" "+endpoint)
		if endpoint == "user" {
			return []byte(`{"id":42,"login":"mrossi","name":"Mario Rossi","email":null}`), nil
		}
		return []byte(`[{"email":"first@example.test","primary":false},{"email":"primary@example.test","primary":true}]`), nil
	}}
	got, ok := hosting.CommitIdentity(context.Background(), dir)
	if !ok || got != (CommitIdentity{"Mario Rossi", "first@example.test"}) {
		t.Fatalf("got %+v %v", got, ok)
	}
	if len(calls) != 2 || calls[0] != "github.com user" || calls[1] != "github.com user/emails" {
		t.Fatalf("unexpected gh calls %v", calls)
	}

	// Without the user:email scope the list is refused: the noreply stands in.
	hosting.readAPI = func(_ context.Context, _, _, endpoint string) ([]byte, error) {
		if endpoint == "user" {
			return []byte(`{"id":42,"login":"mrossi","name":"","email":null}`), nil
		}
		return nil, errors.New("HTTP 404")
	}
	if got, ok = hosting.CommitIdentity(context.Background(), dir); !ok || got != (CommitIdentity{"mrossi", "42+mrossi@users.noreply.github.com"}) {
		t.Fatalf("fallback got %+v %v", got, ok)
	}

	// Not signed in: no account.
	hosting.readAPI = func(context.Context, string, string, string) ([]byte, error) {
		return nil, errors.New("not logged in")
	}
	if got, ok = hosting.CommitIdentity(context.Background(), dir); ok {
		t.Fatalf("signed-out gh produced %+v", got)
	}
}

func TestGitHubCommitIdentityIgnoresLocalOrigin(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "remote", "add", "origin", filepath.Join(t.TempDir(), "origin.git"))
	hosting := GitHub{readAPI: func(context.Context, string, string, string) ([]byte, error) {
		t.Fatal("gh consulted for a local origin")
		return nil, nil
	}}
	if got, ok := hosting.CommitIdentity(context.Background(), dir); ok {
		t.Fatalf("local origin produced %+v", got)
	}
}

// accountHosting is a Hosting adapter signed in to an account.
type accountHosting struct {
	fakeHosting
	identity CommitIdentity
	signedIn bool
}

func (h *accountHosting) CommitIdentity(context.Context, string) (CommitIdentity, bool) {
	return h.identity, h.signedIn
}

// commitAsAgent commits the way an agent without a Git identity does: with an
// identity of its own invention, passed only to that command.
func commitAsAgent(t *testing.T, r Record, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Workdir, "hello.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, r.Workdir, "add", "hello.txt")
	runGit(t, r.Workdir, "-c", "user.name=dev", "-c", "user.email=dev@sandbox.local", "commit", "-m", "feat: export")
}

func readyResult() Result {
	return Result{SquashCommit: &SquashCommit{Title: "feat: export customers"}, Summary: "Try the customer export.", Checks: []string{"git diff --check HEAD~1 HEAD — passed"}}
}

func identityOf(t *testing.T, dir, rev string) string {
	t.Helper()
	return runGit(t, dir, "log", "-1", "--format=%an <%ae>|%cn <%ce>", rev)
}

func TestPublishedCandidateIsAttributedToTheGitHubAccount(t *testing.T) {
	s := fixture(t)
	remote(t, s)
	account := CommitIdentity{Name: "Mario Rossi", Email: "first@example.test"}
	hosting := &accountHosting{identity: account, signedIn: true}
	s.Hosting = hosting
	r := specify(t, s, create(t, s))
	baseIdentity := identityOf(t, r.Workdir, r.BaseSHA)
	commitAsAgent(t, r, "increment "+r.ID+"\n")
	agentTree := runGit(t, r.Workdir, "rev-parse", "HEAD^{tree}")
	agentMessage := runGit(t, r.Workdir, "log", "-1", "--format=%B", "HEAD")
	verifyProduct(t, r)

	r, err := s.Ready(context.Background(), r.ID, readyResult())
	if err != nil {
		t.Fatal(err)
	}
	want := "Mario Rossi <first@example.test>|Mario Rossi <first@example.test>"
	if got := identityOf(t, r.Workdir, r.ReadySHA); got != want {
		t.Fatalf("candidate identity %q, want %q", got, want)
	}
	if runGit(t, r.Workdir, "rev-parse", r.ReadySHA+"^{tree}") != agentTree || runGit(t, r.Workdir, "log", "-1", "--format=%B", r.ReadySHA) != agentMessage {
		t.Fatal("attribution changed the candidate's content")
	}
	if runGit(t, r.Workdir, "rev-parse", "HEAD") != r.ReadySHA || runGit(t, r.Workdir, "status", "--porcelain") != "" {
		t.Fatal("worktree does not stand on the attributed candidate")
	}
	if identityOf(t, r.Workdir, r.BaseSHA) != baseIdentity {
		t.Fatal("base commit was rewritten")
	}

	if _, err = s.Publish(context.Background(), r.ID, r.ReadySHA, r.Revisions[len(r.Revisions)-1].Number); err != nil {
		t.Fatal(err)
	}
	origin := runGit(t, s.Config.ProjectRoot, "remote", "get-url", "origin")
	if got := identityOf(t, origin, r.Branch); got != want {
		t.Fatalf("published identity %q, want %q", got, want)
	}
	if hosting.pr == nil || hosting.pr.HeadSHA != r.ReadySHA {
		t.Fatal("published head is not the approved candidate")
	}
}

func TestCandidateFallsBackToThePersonsGitIdentity(t *testing.T) {
	s := fixture(t)
	s.Hosting = &accountHosting{signedIn: false}
	r := specify(t, s, create(t, s))
	commitAsAgent(t, r, "increment "+r.ID+"\n")
	verifyProduct(t, r)
	r, err := s.Ready(context.Background(), r.ID, readyResult())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := identityOf(t, r.Workdir, r.ReadySHA), "Tests <tests@example.test>|Tests <tests@example.test>"; got != want {
		t.Fatalf("candidate identity %q, want %q", got, want)
	}
}

func TestCandidateKeepsItsIdentityWithoutAccountOrGitIdentity(t *testing.T) {
	s := fixture(t)
	isolateFromPersonalGitConfig(t)
	runGit(t, s.Config.ProjectRoot, "config", "--unset", "user.name")
	runGit(t, s.Config.ProjectRoot, "config", "--unset", "user.email")
	s.Hosting = &accountHosting{signedIn: false}
	r := specify(t, s, create(t, s))
	commitAsAgent(t, r, "increment "+r.ID+"\n")
	committed := runGit(t, r.Workdir, "rev-parse", "HEAD")
	verifyProduct(t, r)
	r, err := s.Ready(context.Background(), r.ID, readyResult())
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadySHA != committed {
		t.Fatalf("candidate rewritten without an identity to apply: %s != %s", r.ReadySHA, committed)
	}
}

func TestLaterAttributionPreservesEarlierRevisionHead(t *testing.T) {
	s := fixture(t)
	hosting := &accountHosting{identity: CommitIdentity{"Mario Rossi", "first@example.test"}, signedIn: true}
	s.Hosting = hosting
	r := specify(t, s, create(t, s))
	commitAsAgent(t, r, "first version\n")
	verifyProduct(t, r)
	r, err := s.Ready(context.Background(), r.ID, readyResult())
	if err != nil {
		t.Fatal(err)
	}
	firstHead := r.ReadySHA
	if r, err = s.FeedbackFor(context.Background(), r.ID, "Use Excel instead of CSV."); err != nil {
		t.Fatal(err)
	}
	r = specify(t, s, r)
	commitAsAgent(t, r, "second version\n")
	verifyProduct(t, r)
	// The person signs in with another account between the two revisions.
	hosting.identity = CommitIdentity{"Maria Bianchi", "maria@example.test"}
	if r, err = s.Ready(context.Background(), r.ID, readyResult()); err != nil {
		t.Fatal(err)
	}
	if runGit(t, r.Workdir, "rev-parse", r.ReadySHA+"^") != firstHead {
		t.Fatal("the head of the earlier revision was rewritten")
	}
	if got := identityOf(t, r.Workdir, r.ReadySHA); got != "Maria Bianchi <maria@example.test>|Maria Bianchi <maria@example.test>" {
		t.Fatalf("second revision identity %q", got)
	}
}

// isolateFromPersonalGitConfig hides the global and system Git configuration
// of the machine running the tests, so "no Git identity" is really none.
func isolateFromPersonalGitConfig(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}
