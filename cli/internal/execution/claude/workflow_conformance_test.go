package claude

import (
	"encoding/json"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun/conformance"
	"testing"
)

type workflowHarness struct {
	provider     *Provider
	fake         *fakeClaude
	dir, command string
}

func (h *workflowHarness) Provider() execution.Provider { return h.provider }
func (h *workflowHarness) Request() execution.Request {
	return execution.Request{WorkingDir: h.dir, ProviderConfig: map[string]any{"command": h.command}}
}
func TestLocalWorkflowConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Harness {
		fake := newFakeClaude()
		t.Cleanup(fake.end)
		dir := conformance.Workspace(t, ".claude/skills")
		return &workflowHarness{provider: newSessionProvider(dir, &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil), fake: fake, dir: dir, command: fakeCommand(t)}
	})
}

func (h *workflowHarness) Messages() []string { return h.fake.messagesReceived() }
func (h *workflowHarness) Complete(text string, completed bool) {
	encoded, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}}})
	h.fake.emit(string(encoded))
	h.fake.emit(resultFrame(text, !completed))
}
