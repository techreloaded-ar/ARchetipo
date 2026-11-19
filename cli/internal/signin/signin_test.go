package signin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// script is a fake sign-in command: it writes to out and reads what the
// manager writes to its input.
type script func(ctx context.Context, out io.Writer, in *bufio.Reader) error

type fakeProcess struct {
	output io.Reader
	input  io.Writer
	done   chan struct{}
	err    error
}

func (p *fakeProcess) Output() io.Reader { return p.output }
func (p *fakeProcess) Input() io.Writer  { return p.input }
func (p *fakeProcess) Wait() error       { <-p.done; return p.err }

func starter(scripts map[string]script) Starter {
	return func(ctx context.Context, c Command) (Process, error) {
		run, ok := scripts[c.Name]
		if !ok {
			return nil, fmt.Errorf("no script for %s", c.Name)
		}
		outR, outW := io.Pipe()
		inR, inW := io.Pipe()
		p := &fakeProcess{output: outR, input: inW, done: make(chan struct{})}
		go func() {
			p.err = run(ctx, outW, bufio.NewReader(inR))
			if ctx.Err() != nil && p.err == nil {
				p.err = ctx.Err()
			}
			_ = outW.Close()
			_ = inR.Close()
			close(p.done)
		}()
		return p, nil
	}
}

type recordedExec struct {
	mu    sync.Mutex
	calls []string
	reply func(c Command) (string, int, error)
}

func (r *recordedExec) run(_ context.Context, c Command) (string, int, error) {
	r.mu.Lock()
	r.calls = append(r.calls, c.Name+" "+strings.Join(c.Args, " "))
	r.mu.Unlock()
	return r.reply(c)
}

func waitState(t *testing.T, m *Manager, kind, want string) Flow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		f := m.flows[kind]
		var view Flow
		if f != nil {
			view = f.view
		}
		m.mu.Unlock()
		if view.State == want {
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t.Fatalf("%s never reached %s: %+v", kind, want, m.flows[kind])
	return Flow{}
}

func waitFor(t *testing.T, m *Manager, kind string, ok func(Flow) bool) Flow {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		view := m.flows[kind].view
		m.mu.Unlock()
		if ok(view) {
			return view
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: condition never met", kind)
	return Flow{}
}

func TestClaudeSignInShowsTheLinkAndRelaysThePastedCode(t *testing.T) {
	received := make(chan string, 1)
	m := newManager(Specs(), (&recordedExec{reply: func(Command) (string, int, error) { return "", 0, nil }}).run, starter(map[string]script{
		"claude": func(ctx context.Context, out io.Writer, in *bufio.Reader) error {
			fmt.Fprintln(out, "Opening browser to sign in…")
			fmt.Fprintln(out, "Also see https://evil.example/phish")
			fmt.Fprint(out, "If the browser didn't open, visit: https://claude.com/cai/oauth/authorize?code=true&state=abc\nPaste code here if prompted > ")
			code, err := in.ReadString('\n')
			if err != nil {
				return err
			}
			received <- strings.TrimSpace(code)
			fmt.Fprintln(out, "Login successful.")
			return nil
		},
	}))
	flow, err := m.Start(Claude)
	if err != nil || flow.State != StateWaiting || !flow.NeedsCode {
		t.Fatalf("start: %+v %v", flow, err)
	}
	flow = waitFor(t, m, Claude, func(f Flow) bool { return f.URL != "" })
	if flow.URL != "https://claude.com/cai/oauth/authorize?code=true&state=abc" {
		t.Fatalf("link %q", flow.URL)
	}
	if again, _ := m.Start(Claude); again.StartedAt != flow.StartedAt {
		t.Fatal("a second press started a second sign-in")
	}
	if _, err = m.SubmitCode(Claude, "line\nbreak"); !errors.Is(err, ErrNoCodeExpected) {
		t.Fatalf("multi-line code accepted: %v", err)
	}
	if flow, err = m.SubmitCode(Claude, "  abc#123  "); err != nil || !flow.CodeSent {
		t.Fatalf("submit: %+v %v", flow, err)
	}
	if got := <-received; got != "abc#123" {
		t.Fatalf("command received %q", got)
	}
	waitState(t, m, Claude, StateSucceeded)
}

func TestDeviceCodeSignInsShowTheCodeAndGitHubIsSetUpForGit(t *testing.T) {
	exec := &recordedExec{reply: func(Command) (string, int, error) { return "", 0, nil }}
	continued := make(chan string, 1)
	m := newManager(Specs(), exec.run, starter(map[string]script{
		"codex": func(ctx context.Context, out io.Writer, in *bufio.Reader) error {
			fmt.Fprintln(out, "1. Open this link in your browser and sign in to your account")
			fmt.Fprintln(out, "   https://auth.openai.com/codex/device")
			fmt.Fprintln(out, "2. Enter this one-time code (expires in 15 minutes)")
			fmt.Fprintln(out, "   \x1b[94mLD2K-UTC6P\x1b[0m")
			<-time.After(50 * time.Millisecond)
			return nil
		},
		"gh": func(ctx context.Context, out io.Writer, in *bufio.Reader) error {
			fmt.Fprintln(out, "! First copy your one-time code: 3F2A-9B1C")
			fmt.Fprintln(out, "Press Enter to open https://github.com/login/device in your browser...")
			line, err := in.ReadString('\n')
			continued <- line
			return err
		},
	}))
	if _, err := m.Start(Codex); err != nil {
		t.Fatal(err)
	}
	flow := waitState(t, m, Codex, StateSucceeded)
	if flow.URL != "https://auth.openai.com/codex/device" || flow.Code != "LD2K-UTC6P" || flow.NeedsCode {
		t.Fatalf("codex flow %+v", flow)
	}
	if _, err := m.SubmitCode(Codex, "x"); !errors.Is(err, ErrNoCodeExpected) {
		t.Fatal("a device-code sign-in accepted a pasted code")
	}

	if _, err := m.Start(GitHub); err != nil {
		t.Fatal(err)
	}
	if got := <-continued; got != "\n" {
		t.Fatalf("gh was not told to continue: %q", got)
	}
	flow = waitState(t, m, GitHub, StateSucceeded)
	if flow.Code != "3F2A-9B1C" || flow.URL != "https://github.com/login/device" {
		t.Fatalf("github flow %+v", flow)
	}
	exec.mu.Lock()
	defer exec.mu.Unlock()
	if strings.Join(exec.calls, ";") != "gh auth setup-git --hostname github.com" {
		t.Fatalf("after-sign-in commands %v", exec.calls)
	}
}

func TestFailedAndCancelledSignInsSayHowTheyEnded(t *testing.T) {
	m := newManager(Specs(), (&recordedExec{reply: func(Command) (string, int, error) { return "", 0, nil }}).run, starter(map[string]script{
		"codex": func(ctx context.Context, out io.Writer, in *bufio.Reader) error {
			fmt.Fprintln(out, "Error logging in with device code: denied")
			return errors.New("exit status 1")
		},
		"claude": func(ctx context.Context, out io.Writer, in *bufio.Reader) error {
			fmt.Fprint(out, "Paste code here if prompted > ")
			<-ctx.Done()
			return ctx.Err()
		},
	}))
	if _, err := m.Start(Codex); err != nil {
		t.Fatal(err)
	}
	if flow := waitState(t, m, Codex, StateFailed); !strings.Contains(flow.Error, "denied") {
		t.Fatalf("failure reason %q", flow.Error)
	}
	if _, err := m.Start(Claude); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(Claude); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, Claude, StateCancelled)
	if _, err := m.Start("other"); !errors.Is(err, ErrUnknownKind) {
		t.Fatal("unknown account accepted")
	}
}

func TestAccountsAskEachToolWhetherItIsSignedIn(t *testing.T) {
	exec := &recordedExec{reply: func(c Command) (string, int, error) {
		switch c.Name {
		case "claude":
			return `{"loggedIn": true, "authMethod": "claude.ai", "subscriptionType": "max"}`, 0, nil
		case "codex":
			return "Not logged in", 1, nil
		default:
			return "", 0, errors.New("executable file not found")
		}
	}}
	m := newManager(Specs(), exec.run, starter(nil))
	accounts := m.Accounts(context.Background())
	if len(accounts) != 3 || accounts[0].Kind != Claude || accounts[1].Kind != Codex || accounts[2].Kind != GitHub {
		t.Fatalf("accounts %+v", accounts)
	}
	if !accounts[0].SignedIn || accounts[0].Detail != "abbonamento max" || accounts[0].Domain != "claude.ai" {
		t.Fatalf("claude %+v", accounts[0])
	}
	if !accounts[1].Available || accounts[1].SignedIn {
		t.Fatalf("codex %+v", accounts[1])
	}
	if accounts[2].Available {
		t.Fatalf("github without gh %+v", accounts[2])
	}
}

func TestStatusParsers(t *testing.T) {
	if ok, _ := claudeSignedIn(`{"loggedIn": false}`, 0); ok {
		t.Fatal("claude logged out read as signed in")
	}
	if ok, _ := claudeSignedIn(`not json`, 0); ok {
		t.Fatal("claude garbage read as signed in")
	}
	if ok, detail := codexSignedIn("WARNING: x\nLogged in using ChatGPT\n", 0); !ok || detail != "Logged in using ChatGPT" {
		t.Fatalf("codex %v %q", ok, detail)
	}
	if ok, detail := githubSignedIn("github.com\n  ✓ Logged in to github.com account octocat (keyring)\n", 0); !ok || detail != "octocat" {
		t.Fatalf("github %v %q", ok, detail)
	}
	if ok, _ := githubSignedIn("You are not logged into any GitHub hosts.", 1); ok {
		t.Fatal("github logged out read as signed in")
	}
}
