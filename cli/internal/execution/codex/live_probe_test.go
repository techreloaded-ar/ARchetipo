//go:build liveprobe

package codex

import (
	"context"
	"os"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
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
