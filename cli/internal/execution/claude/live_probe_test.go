//go:build liveprobe

package claude

import (
	"context"
	"os"
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
	client := newStreamSession(process, session)
	go client.consume()

	const prompt = "Conta lentamente da 1 a 40, un numero per riga, senza usare strumenti."
	const steered = "Fermati e rispondi solo CIAO."

	if err := client.start(ctx, prompt); err != nil {
		t.Fatalf("the session the production client opens was refused: %v", err)
	}
	session.AttachDialogue(client)

	// The first thing the history must show is the prompt itself, re-emitted by
	// Claude as a user message. That re-emission is the mechanism the whole
	// dialogue rests on — it is what `--replay-user-messages` buys — and it is
	// verified here against the real binary rather than against a double.
	waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
		return event.Kind == localrun.KindUserMessage && event.Text == prompt
	}, "the prompt re-emitted as a user message")

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
	}
	if snapshot := session.Snapshot(); snapshot.State != execution.RunActive {
		t.Fatalf("the session closed itself: %#v — only the observed end of the process may do that", snapshot)
	}
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
