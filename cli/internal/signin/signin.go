// Package signin relays the agents' and the Git host's own sign-in commands to
// the viewer, so a person signs in from the page without a terminal (ADR 0004).
//
// Each sign-in is the account owner's own command, run as a child process with
// piped input: the package only reads the link and the code it prints, writes
// back a code the person pastes when the command asks for one, and asks the
// command itself whether the account is signed in. It never handles a password
// and never reads credentials from anywhere.
package signin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/processtree"
)

// EnvRuntime marks the process as running in the isolated runtime, where the
// viewer is the only way to sign in. Elsewhere the person keeps signing in from
// their own terminal and nothing is offered.
const EnvRuntime = "ARCHETIPO_RUNTIME"

// Offered reports whether this viewer offers sign-in.
func Offered() bool { return strings.TrimSpace(os.Getenv(EnvRuntime)) == "isolated" }

// Account kinds.
const (
	Claude = "claude"
	Codex  = "codex"
	GitHub = "github"
)

// Flow states.
const (
	StateWaiting   = "waiting"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
)

// FlowTimeout bounds a sign-in the person abandoned.
var FlowTimeout = 20 * time.Minute

// Command is one invocation of an account's own tool.
type Command struct {
	Name string
	Args []string
}

// Spec describes one account: how to ask whether it is signed in, how to sign
// in, and which hosts its sign-in links may point to.
type Spec struct {
	Kind  string
	Label string
	// Domain is the site the person's browser must reach; the page names it
	// because a filter on the person's computer may block it in one browser.
	Domain    string
	LinkHosts []string
	Status    Command
	// SignedIn reads the status command's output and exit code.
	SignedIn func(output string, exitCode int) (bool, string)
	Login    Command
	// NeedsCode is true when the command waits for a code the person copies
	// from the sign-in page; otherwise it shows a code to type on that page.
	NeedsCode bool
	// Continue is written once a code to show was read, for a command that
	// waits for Enter before polling.
	Continue string
	// After runs once the sign-in succeeded.
	After []Command
}

// Specs are the accounts of this version.
func Specs() []Spec {
	return []Spec{
		{
			Kind: Claude, Label: "Claude", Domain: "claude.ai",
			LinkHosts: []string{"claude.com", "claude.ai"},
			Status:    Command{Name: "claude", Args: []string{"auth", "status"}},
			SignedIn:  claudeSignedIn,
			Login:     Command{Name: "claude", Args: []string{"auth", "login", "--claudeai"}},
			NeedsCode: true,
		},
		{
			Kind: Codex, Label: "Codex (ChatGPT)", Domain: "auth.openai.com",
			LinkHosts: []string{"auth.openai.com"},
			Status:    Command{Name: "codex", Args: []string{"login", "status"}},
			SignedIn:  codexSignedIn,
			Login:     Command{Name: "codex", Args: []string{"login", "--device-auth"}},
		},
		{
			Kind: GitHub, Label: "GitHub", Domain: "github.com",
			LinkHosts: []string{"github.com"},
			Status:    Command{Name: "gh", Args: []string{"auth", "status", "--hostname", "github.com"}},
			SignedIn:  githubSignedIn,
			Login:     Command{Name: "gh", Args: []string{"auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--web"}},
			Continue:  "\n",
			After:     []Command{{Name: "gh", Args: []string{"auth", "setup-git", "--hostname", "github.com"}}},
		},
	}
}

func claudeSignedIn(output string, exitCode int) (bool, string) {
	var status struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		SubscriptionType string `json:"subscriptionType"`
	}
	if json.Unmarshal([]byte(output), &status) != nil || !status.LoggedIn {
		return false, ""
	}
	if status.SubscriptionType != "" {
		return true, "abbonamento " + status.SubscriptionType
	}
	return true, status.AuthMethod
}

func codexSignedIn(output string, exitCode int) (bool, string) {
	line := lastLine(output)
	if exitCode != 0 || !strings.HasPrefix(line, "Logged in") {
		return false, ""
	}
	return true, line
}

func githubSignedIn(output string, exitCode int) (bool, string) {
	if exitCode != 0 {
		return false, ""
	}
	if match := regexp.MustCompile(`account (\S+)`).FindStringSubmatch(output); match != nil {
		return true, match[1]
	}
	return true, ""
}

// Account is what the page shows for one kind.
type Account struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Domain    string `json:"domain"`
	Available bool   `json:"available"`
	SignedIn  bool   `json:"signed_in"`
	Detail    string `json:"detail,omitempty"`
	Flow      *Flow  `json:"flow,omitempty"`
}

// Flow is one sign-in in progress or just ended.
type Flow struct {
	State     string    `json:"state"`
	URL       string    `json:"url,omitempty"`
	Code      string    `json:"code,omitempty"`
	NeedsCode bool      `json:"needs_code"`
	CodeSent  bool      `json:"code_sent"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

// Exec runs a command and returns its combined output and exit code; err is
// set only when it could not be run at all.
type Exec func(ctx context.Context, c Command) (string, int, error)

// Start starts a long-running command with piped input and combined output.
type Starter func(ctx context.Context, c Command) (Process, error)

// Process is a started sign-in command.
type Process interface {
	Output() io.Reader
	Input() io.Writer
	Wait() error
}

// Manager keeps at most one flow per kind.
type Manager struct {
	specs map[string]Spec
	order []string
	exec  Exec
	start Starter

	mu    sync.Mutex
	flows map[string]*flow
}

type flow struct {
	view   Flow
	cancel context.CancelFunc
	input  io.Writer
}

// NewManager uses the real commands.
func NewManager() *Manager { return newManager(Specs(), runCommand, startCommand) }

func newManager(specs []Spec, exec Exec, start Starter) *Manager {
	m := &Manager{specs: map[string]Spec{}, exec: exec, start: start, flows: map[string]*flow{}}
	for _, spec := range specs {
		m.specs[spec.Kind] = spec
		m.order = append(m.order, spec.Kind)
	}
	return m
}

// ErrUnknownKind is returned for an account this version does not know.
var ErrUnknownKind = errors.New("unknown account")

// ErrNoCodeExpected is returned when a code is sent to a flow that waits for none.
var ErrNoCodeExpected = errors.New("this sign-in is not waiting for a code")

// Accounts asks every account's own tool whether it is signed in.
func (m *Manager) Accounts(ctx context.Context) []Account {
	accounts := make([]Account, len(m.order))
	var wg sync.WaitGroup
	for index, kind := range m.order {
		wg.Add(1)
		go func(index int, spec Spec) {
			defer wg.Done()
			accounts[index] = m.account(ctx, spec)
		}(index, m.specs[kind])
	}
	wg.Wait()
	return accounts
}

func (m *Manager) account(ctx context.Context, spec Spec) Account {
	account := Account{Kind: spec.Kind, Label: spec.Label, Domain: spec.Domain}
	statusCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, code, err := m.exec(statusCtx, spec.Status)
	if err == nil {
		account.Available = true
		account.SignedIn, account.Detail = spec.SignedIn(output, code)
	}
	m.mu.Lock()
	if f := m.flows[spec.Kind]; f != nil {
		view := f.view
		account.Flow = &view
	}
	m.mu.Unlock()
	return account
}

// Start begins a sign-in, replacing an ended one. A flow already waiting is
// returned as is, so a second press never opens a second sign-in.
func (m *Manager) Start(kind string) (Flow, error) {
	spec, ok := m.specs[kind]
	if !ok {
		return Flow{}, ErrUnknownKind
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if f := m.flows[kind]; f != nil && f.view.State == StateWaiting {
		return f.view, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), FlowTimeout)
	process, err := m.start(ctx, spec.Login)
	if err != nil {
		cancel()
		return Flow{}, err
	}
	f := &flow{view: Flow{State: StateWaiting, NeedsCode: spec.NeedsCode, StartedAt: time.Now().UTC()}, cancel: cancel, input: process.Input()}
	m.flows[kind] = f
	go m.follow(ctx, spec, f, process)
	return f.view, nil
}

var (
	ansi     = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	linkPat  = regexp.MustCompile(`https://[^\s"'<>]+`)
	codePat  = regexp.MustCompile(`\b[A-Z0-9]{4}-[A-Z0-9]{4,5}\b`)
	tailSize = 6
)

func (m *Manager) follow(ctx context.Context, spec Spec, f *flow, process Process) {
	var tail []string
	scanner := bufio.NewScanner(process.Output())
	scanner.Split(scanLinesOrPrompts)
	for scanner.Scan() {
		line := strings.TrimSpace(ansi.ReplaceAllString(scanner.Text(), ""))
		if line == "" {
			continue
		}
		tail = append(tail, line)
		if len(tail) > tailSize {
			tail = tail[1:]
		}
		m.mu.Lock()
		if f.view.URL == "" {
			if link := allowedLink(line, spec.LinkHosts); link != "" {
				f.view.URL = link
			}
		}
		if !spec.NeedsCode && f.view.Code == "" {
			if code := codePat.FindString(line); code != "" {
				f.view.Code = code
				if spec.Continue != "" {
					// Written aside: the command may still be printing, and
					// this loop is the only reader of what it prints.
					go func(input io.Writer) { _, _ = io.WriteString(input, spec.Continue) }(f.input)
				}
			}
		}
		m.mu.Unlock()
	}
	waitErr := process.Wait()
	state, message := StateSucceeded, ""
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		state = StateCancelled
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		state, message = StateFailed, "the sign-in was not completed in time"
	case waitErr != nil:
		state, message = StateFailed, strings.Join(tail, " · ")
	default:
		for _, after := range spec.After {
			if output, code, err := m.exec(context.Background(), after); err != nil || code != 0 {
				state, message = StateFailed, "signed in, but setting it up failed: "+strings.TrimSpace(output)
				break
			}
		}
	}
	m.mu.Lock()
	f.view.State, f.view.Error = state, message
	f.cancel()
	m.mu.Unlock()
}

// SubmitCode writes the code the person copied from the sign-in page.
func (m *Manager) SubmitCode(kind, code string) (Flow, error) {
	if _, ok := m.specs[kind]; !ok {
		return Flow{}, ErrUnknownKind
	}
	code = strings.TrimSpace(code)
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.flows[kind]
	if f == nil || f.view.State != StateWaiting || !f.view.NeedsCode || code == "" || strings.ContainsAny(code, "\r\n") {
		return Flow{}, ErrNoCodeExpected
	}
	if _, err := io.WriteString(f.input, code+"\n"); err != nil {
		return Flow{}, err
	}
	f.view.CodeSent = true
	return f.view, nil
}

// Cancel stops a waiting sign-in.
func (m *Manager) Cancel(kind string) error {
	if _, ok := m.specs[kind]; !ok {
		return ErrUnknownKind
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if f := m.flows[kind]; f != nil {
		f.cancel()
	}
	return nil
}

// Close stops every flow, when the viewer shuts down.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.flows {
		f.cancel()
	}
}

// allowedLink returns the first https link of the line whose host belongs to
// the account; any other link is ignored, so the page never offers a link the
// account's own tool did not mean.
func allowedLink(line string, hosts []string) string {
	for _, candidate := range linkPat.FindAllString(line, -1) {
		u, err := url.Parse(candidate)
		if err != nil || u.User != nil {
			continue
		}
		for _, host := range hosts {
			if u.Hostname() == host || strings.HasSuffix(u.Hostname(), "."+host) {
				return candidate
			}
		}
	}
	return ""
}

// scanLinesOrPrompts splits on newlines, and also hands over a trailing ">"
// prompt that waits for input without ending its line.
func scanLinesOrPrompts(data []byte, atEOF bool) (int, []byte, error) {
	advance, token, err := bufio.ScanLines(data, atEOF)
	if advance > 0 || err != nil || atEOF {
		return advance, token, err
	}
	if strings.HasSuffix(strings.TrimRight(string(data), " "), ">") {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func runCommand(ctx context.Context, c Command) (string, int, error) {
	if _, err := exec.LookPath(c.Name); err != nil {
		return "", 0, err
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(output), exitErr.ExitCode(), nil
	}
	return string(output), 0, err
}

// execProcess shares one Wait between the goroutine that closes the output
// when the command ends and the caller that asks how it ended.
type execProcess struct {
	output io.Reader
	input  io.WriteCloser
	done   chan struct{}
	err    error
}

func (p *execProcess) Output() io.Reader { return p.output }
func (p *execProcess) Input() io.Writer  { return p.input }
func (p *execProcess) Wait() error       { <-p.done; return p.err }

func startCommand(ctx context.Context, c Command) (Process, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	stop := processtree.Own(cmd)
	cmd.WaitDelay = 5 * time.Second
	// BROWSER=true makes a command that tries to open a browser succeed without
	// one, so it goes on printing the link instead of failing.
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "BROWSER=true")
	reader, writer := io.Pipe()
	cmd.Stdout, cmd.Stderr = writer, writer
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	process := &execProcess{output: reader, input: input, done: make(chan struct{})}
	go func() {
		process.err = cmd.Wait()
		_ = stop()
		_ = writer.Close()
		close(process.done)
	}()
	return process, nil
}
