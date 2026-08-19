//go:build liveprobe

package codex

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// TestLiveCodexPlansASpec dispatches one real spec.plan action to the real
// Codex binary in a real workspace. It is the only check that exercises the
// flags in defaultExecArgs against the CLI that has to accept them — every
// other test goes through the Runner seam and would happily agree on a flag
// Codex rejects, which is exactly how `--full-auto` survived a full review.
//
// It is behind a build tag because it costs an agent run and mutates the
// backlog: the spec it names really is planned. Run it by hand after a Codex
// upgrade, or when defaultExecArgs changes:
//
//	LIVE_WORKSPACE=/path/to/workspace LIVE_SPEC=US-0XX \
//	  go test -tags liveprobe -run TestLiveCodexPlansASpec -timeout 40m ./internal/execution/codex/
func TestLiveCodexPlansASpec(t *testing.T) {
	root := os.Getenv("LIVE_WORKSPACE")
	spec := os.Getenv("LIVE_SPEC")
	if root == "" || spec == "" {
		t.Skip("set LIVE_WORKSPACE and LIVE_SPEC to run the live Codex probe")
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

// TestLiveCodexDialogue drives the real Codex binary through the real app
// server protocol: handshake, thread, turn, and — while the turn is still
// alive — a steer and an interrupt.
//
// It exists for the same reason as the probe above, and it is the only check
// that can catch the mistake that matters here: every other test in this
// package goes through the process seam and would happily agree on a method
// name, a parameter or a refusal that Codex does not recognize. That is exactly
// how `--full-auto` survived a full review.
//
// It costs a few seconds of agent time, touches no backlog and writes nothing:
// the prompt asks for one word, in a temporary directory.
//
//	LIVE_CODEX=1 go test -tags liveprobe -run TestLiveCodexDialogue -timeout 5m ./internal/execution/codex/
func TestLiveCodexDialogue(t *testing.T) {
	if os.Getenv("LIVE_CODEX") == "" {
		t.Skip("set LIVE_CODEX=1 to run the live Codex dialogue probe")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	process, err := localrun.ExecStarter{}.Start(ctx, dir, "codex", buildArgs())
	if err != nil {
		t.Fatalf("starting the codex app server: %v", err)
	}
	session := localrun.NewSession("live-probe", nil)
	client := newAppServer(process, session)
	go client.consume()

	const prompt = "Conta lentamente da 1 a 40, un numero per riga, senza usare strumenti."
	const steered = "Fermati e rispondi solo CIAO."

	cfg := settings{Command: "codex", Sandbox: defaultSandbox, Timeout: 3 * time.Minute}
	if err := client.start(ctx, cfg, dir, prompt); err != nil {
		t.Fatalf("the handshake the production client speaks was refused: %v", err)
	}
	session.AttachDialogue(client)

	// The first event is the prompt itself, re-emitted by Codex as a user
	// message: this is the mechanism the whole dialogue rests on, and it is
	// verified here against the real binary and not against a double.
	waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
		return event.Kind == kindUserMessage && event.Text == prompt
	}, "the prompt re-emitted as a user message")

	// A steer issued in the instant between `turn/start` returning and the turn
	// actually starting is refused with `no active turn to steer` — observed
	// here — which is why the wait above comes first.
	if err := client.Send(ctx, steered); err != nil {
		assertDeliveredOrRefused(t, "turn/steer", err)
	} else {
		waitForLiveEvent(t, ctx, session, func(event execution.RunEvent) bool {
			return event.Kind == kindUserMessage && event.Text == steered
		}, "the steered message re-emitted by the process")
	}

	assertDeliveredOrRefused(t, "turn/interrupt", client.Interrupt(ctx))

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
