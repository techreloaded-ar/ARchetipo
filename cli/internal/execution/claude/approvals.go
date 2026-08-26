package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// The permission half of the control protocol, verified against Claude Code
// 2.1.246 by driving the installed build by hand.
//
// With `--permission-prompt-tool stdio` on the command line, a tool call the
// process may not make on its own no longer ends in a silent refusal: the
// process writes a `control_request` of subtype `can_use_tool` on the same
// stream the history travels on, and waits. The caller answers with an ordinary
// `control_response` whose payload is the decision. An allowed call runs; a
// denied one comes back to the agent as a tool result in error, carrying the
// message the refusal was given, and the agent goes on from there.
//
// That flag is what makes `--permission-mode auto` mean what it says. Without
// it, `--print` has nobody to ask, so every escalation is denied where it
// stands and the agent — correctly — ends its turn asking a person for
// something no one could give it.
type canUseToolRequest struct {
	Subtype     string          `json:"subtype"`
	ToolName    string          `json:"tool_name"`
	DisplayName string          `json:"display_name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Input       json.RawMessage `json:"input"`
	ToolUseID   string          `json:"tool_use_id"`
}

// ask records one question the process is waiting on.
//
// Only `can_use_tool` is understood. Another control request — an elicitation,
// a dialog — is left unanswered on purpose rather than refused blindly: this
// package would be guessing at both the payload the answer must carry and at
// what refusing means for the tool behind it, and a wrong answer is worse than
// a question that stays open. The process withdraws what it no longer needs,
// and the turn ends either way.
func (s *streamSession) ask(f frame) {
	id := strings.TrimSpace(f.RequestID)
	if id == "" || len(f.Request) == 0 {
		return
	}
	var request canUseToolRequest
	if json.Unmarshal(f.Request, &request) != nil {
		return
	}
	if request.Subtype != subtypeCanUseTool {
		return
	}
	approval := execution.PendingApproval{
		ID:        id,
		ToolName:  toolNameOf(request),
		Title:     approvalTitleOf(request),
		Args:      localrun.RawOf(request.Input),
		Options:   localrun.ApprovalOptions(),
		CreatedAt: s.now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, already := s.asked[id]; already {
		// The same request id twice is a replay, not a second question: a client
		// re-attaching to a session is handed the requests still in flight, and
		// recording it again would show one decision as two.
		return
	}
	s.asked[id] = approval
	s.askedIn = append(s.askedIn, id)
}

// toolNameOf names the tool the question is about, preferring what the process
// says it displays over the internal name. Both are its own words; neither is
// invented here.
func toolNameOf(request canUseToolRequest) string {
	if name := strings.TrimSpace(request.DisplayName); name != "" {
		return name
	}
	return strings.TrimSpace(request.ToolName)
}

// approvalTitleOf is the one sentence the decision card leads with.
//
// It is the process's own words or nothing: the title it composed for a host to
// render, then the description of the call, and then empty. Nothing is written
// here to fill the gap — a sentence invented at this layer would be describing
// a call this package has not read, which is exactly the thing the person is
// being asked to judge. An empty title leaves the card its own wording, and the
// tool name is rendered beside it in any case.
func approvalTitleOf(request canUseToolRequest) string {
	if title := strings.TrimSpace(request.Title); title != "" {
		return title
	}
	return strings.TrimSpace(request.Description)
}

// withdraw drops a question the process no longer needs an answer to.
func (s *streamSession) withdraw(requestID string) {
	id := strings.TrimSpace(requestID)
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forgetAskedLocked(id)
}

// withdrawAll drops every open question, which is what the end of the process
// does to all of them at once.
func (s *streamSession) withdrawAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = make(map[string]execution.PendingApproval)
	s.askedIn = nil
}

// forgetAskedLocked removes one question from both the lookup and the order.
// The caller holds mu.
func (s *streamSession) forgetAskedLocked(id string) {
	if _, ok := s.asked[id]; !ok {
		return
	}
	delete(s.asked, id)
	for i, kept := range s.askedIn {
		if kept == id {
			s.askedIn = append(s.askedIn[:i], s.askedIn[i+1:]...)
			return
		}
	}
}

// PendingApprovals lists what the process is waiting on, oldest first.
func (s *streamSession) PendingApprovals() []execution.PendingApproval {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]execution.PendingApproval, 0, len(s.askedIn))
	for _, id := range s.askedIn {
		if approval, ok := s.asked[id]; ok {
			out = append(out, approval)
		}
	}
	return out
}

// denialMessage is what a refused tool call is told, and through it the agent.
// It says who decided and leaves the agent free to go on: a refusal is an
// answer to one call, not the end of the work.
const denialMessage = "The person following this run in the ARchetipo viewer did not allow this tool use. Do not retry it as it is: say what you needed it for, or find another way."

// RespondApproval answers one open question and gives it up.
//
// The question is taken out of the pending set *before* the answer is written,
// and never put back. Writing first and forgetting after would leave a window
// in which the same decision could be answered twice, and the second answer
// would reach a process that has already acted on the first. A write that fails
// therefore loses the question, which is the honest outcome: a process that
// cannot be written to is a process that will not be answered.
func (s *streamSession) RespondApproval(ctx context.Context, approvalID, optionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(approvalID)
	decision, err := decisionOf(optionID)
	if err != nil {
		return &execution.RunCommandError{
			Reason: execution.RunRefusedUnsupported,
			RunID:  s.session.RunID(),
			Err:    err,
		}
	}
	s.mu.Lock()
	_, open := s.asked[id]
	s.forgetAskedLocked(id)
	s.mu.Unlock()
	if !open {
		// Not a fault: a decision the process withdrew, or one another tab
		// answered first, is a decision that is simply no longer there.
		return &execution.RunCommandError{
			Reason: execution.RunRefusedUnsupported,
			RunID:  s.session.RunID(),
			Err:    fmt.Errorf("the claude session is not waiting on the decision %q", id),
		}
	}
	payload, err := json.Marshal(map[string]any{
		"type": frameControlResponse,
		"response": map[string]any{
			"subtype":    controlSuccess,
			"request_id": id,
			"response":   decision,
		},
	})
	if err != nil {
		return fmt.Errorf("encoding the decision for the claude session: %w", err)
	}
	if err := s.process.Send(payload); err != nil {
		// The same reading Send makes of a failed write, and for the same reason:
		// once the process has gone, the failure is a state the caller branches
		// on and not the fault a failed write otherwise is.
		if s.sessionOver() {
			return &execution.RunCommandError{
				Reason: execution.RunRefusedNotActive,
				RunID:  s.session.RunID(),
				Err:    fmt.Errorf("the claude session is already over"),
			}
		}
		return fmt.Errorf("sending the decision to the claude session: %w", err)
	}
	return nil
}

// decisionOf renders one of the two answers a local approval accepts into the
// payload the process expects. An allowed call carries no input of ours: the
// process falls back to the input it asked about, which is the only input the
// person actually judged.
func decisionOf(optionID string) (map[string]any, error) {
	switch strings.TrimSpace(optionID) {
	case localrun.ApprovalAllow:
		return map[string]any{"behavior": "allow"}, nil
	case localrun.ApprovalDeny:
		return map[string]any{"behavior": "deny", "message": denialMessage}, nil
	default:
		return nil, fmt.Errorf("%q is not one of the options this decision offers", optionID)
	}
}

var _ localrun.Arbiter = (*streamSession)(nil)
