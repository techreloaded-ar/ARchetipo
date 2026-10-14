package codex

import (
	"encoding/json"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun/conformance"
	"testing"
)

type workflowHarness struct {
	provider     *Provider
	fake         *fakeCodex
	dir, command string
}

func (h *workflowHarness) Provider() execution.Provider { return h.provider }
func (h *workflowHarness) Request() execution.Request {
	return execution.Request{WorkingDir: h.dir, ProviderConfig: map[string]any{"command": h.command}}
}
func TestLocalWorkflowConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Harness {
		fake := newFakeCodex()
		t.Cleanup(fake.end)
		dir := conformance.Workspace(t, ".agents/skills")
		return &workflowHarness{provider: newSessionProvider(dir, &fakeRunner{outcomes: []runOutcome{probeOK}}, fake, nil), fake: fake, dir: dir, command: fakeCommand(t)}
	})
}

func (h *workflowHarness) Messages() []string {
	h.fake.mu.Lock()
	defer h.fake.mu.Unlock()
	var messages []string
	for _, request := range h.fake.requests {
		if request.Method != methodTurnStart {
			continue
		}
		var params struct {
			Input []struct {
				Text string `json:"text"`
			} `json:"input"`
		}
		_ = json.Unmarshal(request.Params, &params)
		if len(params.Input) > 0 {
			messages = append(messages, params.Input[0].Text)
		}
	}
	return messages
}
func (h *workflowHarness) Complete(text string, completed bool) {
	encoded, _ := json.Marshal(map[string]any{"item": map[string]string{"type": "agentMessage", "text": text}})
	h.fake.emit("item/completed", string(encoded))
	if completed {
		h.fake.completeTurn()
	} else {
		h.fake.interruptTurn()
	}
}
