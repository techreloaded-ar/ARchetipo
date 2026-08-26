//go:build liveprobe

package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// TestLiveClaudePlansASpec dispatches one real spec.plan action to the real
// Claude Code binary in a real workspace. It is the only check that exercises
// the flags in buildArgs against the CLI that has to accept them — every other
// test in this package goes through the process seam and would happily agree on
// a flag Claude rejects.
//
// It is behind a build tag because it costs an agent run and mutates the
// backlog: the spec it names really is planned. Run it by hand after a Claude
// Code upgrade, or when buildArgs changes:
//
//	LIVE_WORKSPACE=/path/to/workspace LIVE_SPEC=US-0XX \
//	  go test -tags liveprobe -run TestLiveClaudePlansASpec -timeout 40m ./internal/execution/claude/
func TestLiveClaudePlansASpec(t *testing.T) {
	root := os.Getenv("LIVE_WORKSPACE")
	spec := os.Getenv("LIVE_SPEC")
	if root == "" || spec == "" {
		t.Skip("set LIVE_WORKSPACE and LIVE_SPEC to run the live Claude probe")
	}

	p := New(Options{WorkingDir: func() (string, error) { return root, nil }})
	res, err := p.Execute(context.Background(), execution.Request{
		ExecutionID:    "live-probe",
		SpecCode:       spec,
		Action:         execution.ActionPlan,
		Capability:     execution.CapabilitySpecPlan,
		ProviderConfig: map[string]any{"timeout_seconds": 1800},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	t.Logf("payload: %s", string(res.Payload))
}

// TestLiveClaudeDialogue drives the real Claude Code binary through the real
// streaming protocol: the session, a message delivered while the turn is alive,
// and an interrupt.
//
// It exists because the protocol this package speaks was established by
// observing a real binary — Claude Code 2.1.235 — and every other test here
// goes through the process seam, so it would happily agree on a frame shape
// that Claude does not produce or a control request it does not answer. This is
// the check that notices when a future release changes any of that.
//
// It costs a few seconds of agent time, touches no backlog and writes nothing:
// the prompt asks for counting, in a temporary directory.
//
//	LIVE_CLAUDE=1 go test -tags liveprobe -run TestLiveClaudeDialogue -timeout 5m ./internal/execution/claude/
func TestLiveClaudeDialogue(t *testing.T) {
	if os.Getenv("LIVE_CLAUDE") == "" {
		t.Skip("set LIVE_CLAUDE=1 to run the live Claude dialogue probe")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cfg := settings{Command: defaultCommand, PermissionMode: defaultPermissionMode, Timeout: 3 * time.Minute}
	process, err := localrun.ExecStarter{}.Start(ctx, dir, cfg.Command, buildArgs(cfg))
	if err != nil {
		t.Fatalf("starting the claude session: %v", err)
	}
	session := localrun.NewSession("live-probe", nil)
	client := newStreamSession(process, session, false)
	go client.consume()

	const prompt = "Conta lentamente da 1 a 40, un numero per riga, senza usare strumenti."
	const steered = "Fermati e rispondi solo CIAO."

	if err := client.start(ctx, prompt); err != nil {
		t.Fatalf("the session the production client opens was refused: %v", err)
	}
	session.AttachDialogue(client)

	// The re-emission is the mechanism the whole dialogue rests on — it is what
	// `--replay-user-messages` buys — and it is verified below against the real
	// binary rather than against a double, on the operator's own message. The
	// opening prompt is replayed too, and is deliberately absent from the
	// history: it is the caller's instruction, not something anybody said. That
	// absence is asserted after the turn, over the whole transcript.
	if err := client.Send(ctx, steered); err != nil {
		assertDeliveredOrRefused(t, "the operator message", err)
	} else {
		waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
			return event.Kind == localrun.KindUserMessage && event.Text == steered
		}, "the message re-emitted by the process")
	}

	assertDeliveredOrRefused(t, "the interrupt", client.Interrupt(ctx))

	select {
	case <-client.TurnDone():
	case <-ctx.Done():
		t.Fatal("the turn never ended")
	}
	_ = process.Close()

	for _, event := range session.Events(0) {
		t.Logf("event %d %s %q", event.ID, event.Kind, event.Text)
		if event.Kind == localrun.KindUserMessage && event.Text == prompt {
			t.Fatalf("the opening prompt entered the history as event %d", event.ID)
		}
	}
	if snapshot := session.Snapshot(); snapshot.State != execution.RunActive {
		t.Fatalf("the session closed itself: %#v — only the observed end of the process may do that", snapshot)
	}
}

// TestLiveClaudeMultiTurn asserts the single fact no fake can establish: that
// the real binary accepts a `user` frame *after* it has closed a turn with a
// `result`, and opens a second turn on it.
//
// Everything else about the conversation — the re-arming of the turn, the
// outcome published for each one, the refusal of a message once the session is
// over — is decided by this package and is proved against the process seam. But
// whether Claude Code keeps reading its standard input after a `result` is a
// property of Claude Code, and the whole inception rests on it: if a future
// release ended the session with the turn, every conversation would die on the
// agent's first question and every fake in this package would keep agreeing.
//
// It costs a few seconds of agent time, touches no backlog and writes nothing.
//
//	LIVE_CLAUDE=1 go test -tags liveprobe -run TestLiveClaudeMultiTurn -timeout 5m ./internal/execution/claude/
func TestLiveClaudeMultiTurn(t *testing.T) {
	if os.Getenv("LIVE_CLAUDE") == "" {
		t.Skip("set LIVE_CLAUDE=1 to run the live Claude multi-turn probe")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	cfg := settings{Command: defaultCommand, PermissionMode: defaultPermissionMode, Timeout: 4 * time.Minute}
	process, err := localrun.ExecStarter{}.Start(ctx, dir, cfg.Command, buildArgs(cfg))
	if err != nil {
		t.Fatalf("starting the claude session: %v", err)
	}
	session := localrun.NewSession("live-multiturn", nil)
	// Conversational, which is the mode an inception really runs in: the end of
	// a turn must not end the session.
	client := newStreamSession(process, session, true)
	go client.consume()
	t.Cleanup(func() { _ = process.Close() })

	if err := client.start(ctx, "Rispondi solo con la parola UNO. Non usare strumenti."); err != nil {
		t.Fatalf("the session the production client opens was refused: %v", err)
	}
	session.AttachDialogue(client)

	first := waitForLiveTurn(t, ctx, client, "the first turn")
	t.Logf("first turn: completed=%v final=%q", first.Completed, first.Final)
	if !first.Completed {
		t.Fatalf("the real binary did not complete the first turn: %#v", first)
	}

	// The whole point: a message sent after the `result` must be accepted and
	// must open a second turn.
	if err := client.Send(ctx, "Ora rispondi solo con la parola DUE."); err != nil {
		t.Fatalf("the real binary refused a message sent after a result: %v", err)
	}
	second := waitForLiveTurn(t, ctx, client, "the second turn")
	t.Logf("second turn: completed=%v final=%q", second.Completed, second.Final)
	if !second.Completed {
		t.Fatalf("the second turn did not complete: %#v", second)
	}

	for _, event := range session.Events(0) {
		t.Logf("event %d seq %d %s %q", event.ID, event.Seq, event.Kind, event.Text)
	}
	if snapshot := session.Snapshot(); snapshot.State != execution.RunActive {
		t.Fatalf("the session closed itself between the two turns: %#v", snapshot)
	}
}

// waitForLiveTurn blocks until the process itself declares a turn finished.
func waitForLiveTurn(t *testing.T, ctx context.Context, client *streamSession, what string) TurnOutcome {
	t.Helper()
	select {
	case outcome := <-client.Turns():
		return outcome
	case <-client.Gone():
		t.Fatalf("the process ended before %s", what)
	case <-ctx.Done():
		t.Fatalf("%s never ended", what)
	}
	return TurnOutcome{}
}

// waitForLiveEvent polls the history for an event instead of sleeping for an
// arbitrary time.
func waitForLiveEvent(t *testing.T, ctx context.Context, session *localrun.Session, matches func(execution.RunEvent) bool, what string) {
	t.Helper()
	for {
		for _, event := range session.Events(0) {
			if matches(event) {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s never arrived", what)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// assertDeliveredOrRefused accepts the one refusal that is a legitimate outcome
// against a live agent: a turn that ended before the command reached it.
func assertDeliveredOrRefused(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	reason, refused := execution.RefusalOf(err)
	if refused && reason == execution.RunRefusedNotActive {
		t.Logf("%s was refused because the turn had already ended, which is a valid outcome", what)
		return
	}
	t.Fatalf("%s failed against the real binary: %v", what, err)
}

// TestLiveClaudeAsksAndAcceptsAPermissionDecision drives the real Claude Code
// binary through the permission half of the control protocol: the process stops
// to ask whether it may use a tool, the answer travels back on the same stream,
// and the run goes on.
//
// It is the one check that the bridge really works. Every other test of this
// package goes through the process seam, so it would happily agree on a frame
// shape the binary does not produce or a flag it does not understand — and the
// whole reason the bridge exists is a run that failed because nobody could
// answer such a question.
//
// The workspace it runs in declares an ask rule for Bash, so the escalation is
// a fact of the configuration and not of a classifier that may change its mind
// between releases. Both answers are exercised: the allowed call runs and its
// output comes back, the denied one comes back to the agent as a tool result in
// error, and the agent goes on from there.
//
//	LIVE_CLAUDE=1 go test -tags liveprobe -run TestLiveClaudeAsksAndAcceptsAPermissionDecision -timeout 10m ./internal/execution/claude/
func TestLiveClaudeAsksAndAcceptsAPermissionDecision(t *testing.T) {
	if os.Getenv("LIVE_CLAUDE") == "" {
		t.Skip("set LIVE_CLAUDE=1 to run the live Claude permission probe")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The escalation is declared, not hoped for: a classifier deciding on its
	// own that a command is safe would make this probe pass without ever having
	// exercised the bridge.
	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{"permissions":{"ask":["Bash"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("PAROLA-SEGRETA\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := settings{Command: defaultCommand, PermissionMode: defaultPermissionMode, Timeout: 5 * time.Minute}
	process, err := localrun.ExecStarter{}.Start(ctx, dir, cfg.Command, buildArgs(cfg))
	if err != nil {
		t.Fatalf("starting the claude session: %v", err)
	}
	session := localrun.NewSession("live-approval", nil)
	client := newStreamSession(process, session, true)
	go client.consume()
	t.Cleanup(func() { _ = process.Close() })

	if err := client.start(ctx, "Esegui `cat file.txt` con lo strumento Bash e riportami il suo output. Poi fermati."); err != nil {
		t.Fatalf("the session was refused: %v", err)
	}
	session.AttachDialogue(client)

	allowed := waitForLivePermission(t, ctx, client, "the first permission request")
	t.Logf("asked about %s: %s", allowed.ToolName, string(allowed.Args))
	if allowed.ToolName == "" || len(allowed.Options) != 2 {
		t.Fatalf("the request did not arrive as a decision the viewer can render: %#v", allowed)
	}
	if err := client.RespondApproval(ctx, allowed.ID, localrun.ApprovalAllow); err != nil {
		t.Fatalf("the real binary refused the answer: %v", err)
	}
	first := waitForLiveTurn(t, ctx, client, "the allowed turn")
	t.Logf("allowed turn: completed=%v final=%q", first.Completed, first.Final)
	if !first.Completed || !strings.Contains(first.Final, "PAROLA-SEGRETA") {
		t.Fatalf("the allowed call did not run: %#v", first)
	}

	// The other answer, on the same session: a refusal must reach the agent as
	// something it can read and go on from, not as a wall.
	if err := client.Send(ctx, "Ora esegui `cat file.txt` un'altra volta con lo strumento Bash."); err != nil {
		t.Fatalf("the second message was refused: %v", err)
	}
	denied := waitForLivePermission(t, ctx, client, "the second permission request")
	if err := client.RespondApproval(ctx, denied.ID, localrun.ApprovalDeny); err != nil {
		t.Fatalf("the real binary refused the denial: %v", err)
	}
	second := waitForLiveTurn(t, ctx, client, "the denied turn")
	t.Logf("denied turn: completed=%v final=%q", second.Completed, second.Final)
	if !second.Completed {
		t.Fatalf("a denied call ended the turn in error instead of letting the agent go on: %#v", second)
	}
	// The refusal is history: it comes back on the user side of the protocol as
	// a tool result in error, which is what the agent read to write its answer.
	waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
		return event.Kind == localrun.KindToolError && strings.Contains(event.Text, "ARchetipo")
	}, "the refusal as a tool result in error")

	if pending := client.PendingApprovals(); len(pending) != 0 {
		t.Fatalf("a decision already taken is still pending: %#v", pending)
	}
}

// waitForLivePermission blocks until the real process asks for one.
func waitForLivePermission(t *testing.T, ctx context.Context, client *streamSession, what string) execution.PendingApproval {
	t.Helper()
	for {
		if pending := client.PendingApprovals(); len(pending) > 0 {
			return pending[0]
		}
		select {
		case <-client.Gone():
			t.Fatalf("the process ended before %s", what)
		case <-ctx.Done():
			t.Fatalf("%s never arrived", what)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
