package claude

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// canUseToolFrame renders the permission request Claude Code writes when it may
// not make a tool call on its own. The shape is the one the installed build was
// observed to produce, field for field.
func canUseToolFrame(requestID, tool, command string) string {
	payload, err := json.Marshal(map[string]any{
		"type":       frameControlRequest,
		"request_id": requestID,
		"request": map[string]any{
			"subtype":              subtypeCanUseTool,
			"tool_name":            tool,
			"display_name":         tool,
			"input":                map[string]any{"command": command, "description": "run it"},
			"description":          "run it",
			"decision_reason_type": "rule",
			"tool_use_id":          "toolu_" + requestID,
		},
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// decisionsWritten is every permission answer the process was written, in order.
func decisionsWritten(fake *fakeClaude) []map[string]any {
	out := make([]map[string]any, 0, 2)
	for _, frame := range fake.framesReceived() {
		var payload struct {
			Type     string `json:"type"`
			Response struct {
				Subtype   string         `json:"subtype"`
				RequestID string         `json:"request_id"`
				Response  map[string]any `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(frame, &payload) != nil || payload.Type != frameControlResponse {
			continue
		}
		decision := map[string]any{"request_id": payload.Response.RequestID, "subtype": payload.Response.Subtype}
		for key, value := range payload.Response.Response {
			decision[key] = value
		}
		out = append(out, decision)
	}
	return out
}

// A permission request is a decision waiting for somebody, not a line of the
// history: it becomes readable as a pending approval and leaves the transcript
// untouched. What the agent then does with the answer is what enters the
// history, which is the same rule a message follows.
func TestPermissionRequestBecomesAPendingApprovalAndNotHistory(t *testing.T) {
	fake := newFakeClaude()
	client, session := openConversation(t, fake)
	before := lastEventID(session)

	fake.emit(canUseToolFrame("req-a", "Bash", "git push"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	pending := client.PendingApprovals()[0]
	if pending.ID != "req-a" {
		t.Fatalf("approval id = %q, want req-a", pending.ID)
	}
	if pending.ToolName != "Bash" {
		t.Fatalf("tool name = %q, want Bash", pending.ToolName)
	}
	if pending.Title != "run it" {
		t.Fatalf("title = %q, want the description the process gave", pending.Title)
	}
	if !strings.Contains(string(pending.Args), "git push") {
		t.Fatalf("the arguments do not carry the command being judged: %s", pending.Args)
	}
	if len(pending.Options) != 2 || pending.Options[0].ID != localrun.ApprovalAllow || pending.Options[1].ID != localrun.ApprovalDeny {
		t.Fatalf("the approval does not offer allow and deny: %#v", pending.Options)
	}
	if pending.CreatedAt.IsZero() {
		t.Fatal("the approval carries no instant")
	}
	if events := session.Events(before); len(events) != 0 {
		t.Fatalf("the permission request entered the history: %#v", events)
	}
}

// Allowing writes the answer the process is waiting for and gives the decision
// up: it is no longer pending, because it has been decided.
func TestAllowingAnApprovalAnswersTheProcess(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	if err := client.RespondApproval(context.Background(), "req-a", localrun.ApprovalAllow); err != nil {
		t.Fatalf("allowing failed: %v", err)
	}
	decisions := decisionsWritten(fake)
	if len(decisions) != 1 {
		t.Fatalf("the process was written %d decisions, want exactly one: %#v", len(decisions), decisions)
	}
	if decisions[0]["request_id"] != "req-a" || decisions[0]["subtype"] != controlSuccess {
		t.Fatalf("the answer does not correlate with the question: %#v", decisions[0])
	}
	if decisions[0]["behavior"] != "allow" {
		t.Fatalf("behavior = %v, want allow", decisions[0]["behavior"])
	}
	// No input of ours travels with an allowed call: the process falls back to
	// the input it asked about, which is the only one the person judged.
	if _, rewritten := decisions[0]["updatedInput"]; rewritten {
		t.Fatalf("an allowed call carried a rewritten input: %#v", decisions[0])
	}
	if pending := client.PendingApprovals(); len(pending) != 0 {
		t.Fatalf("the decision is still pending after being taken: %#v", pending)
	}
}

// Denying carries a message, because the refusal reaches the agent as the
// content of a failed tool call and a refusal with nothing to read is a refusal
// the agent can only guess at.
func TestDenyingAnApprovalCarriesTheReason(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "rm -rf /"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	if err := client.RespondApproval(context.Background(), "req-a", localrun.ApprovalDeny); err != nil {
		t.Fatalf("denying failed: %v", err)
	}
	decisions := decisionsWritten(fake)
	if len(decisions) != 1 || decisions[0]["behavior"] != "deny" {
		t.Fatalf("the process was not told no: %#v", decisions)
	}
	if message, _ := decisions[0]["message"].(string); strings.TrimSpace(message) == "" {
		t.Fatalf("the refusal carries no reason: %#v", decisions[0])
	}
}

// An option the decision does not offer is the caller's mistake, refused
// without writing anything: the question stays open, so it can still be
// answered properly.
func TestAnUnknownOptionIsRefusedAndLeavesTheDecisionOpen(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	err := client.RespondApproval(context.Background(), "req-a", "always-allow")
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedUnsupported {
		t.Fatalf("err = %v, want an unsupported refusal", err)
	}
	if decisions := decisionsWritten(fake); len(decisions) != 0 {
		t.Fatalf("a refused answer was written to the process anyway: %#v", decisions)
	}
	if pending := client.PendingApprovals(); len(pending) != 1 {
		t.Fatalf("the question was lost by a refused answer: %#v", pending)
	}
}

// A decision that was never opened, or one already taken, is refused rather
// than written: answering it would reach a process that has already acted.
func TestAnsweringAnApprovalTwiceIsRefused(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })
	if err := client.RespondApproval(context.Background(), "req-a", localrun.ApprovalAllow); err != nil {
		t.Fatalf("allowing failed: %v", err)
	}

	err := client.RespondApproval(context.Background(), "req-a", localrun.ApprovalDeny)
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedUnsupported {
		t.Fatalf("err = %v, want an unsupported refusal", err)
	}
	if decisions := decisionsWritten(fake); len(decisions) != 1 {
		t.Fatalf("the process was answered twice: %#v", decisions)
	}
}

// The process withdraws a question it no longer needs an answer to — a turn
// that was interrupted, a decision another client took. Keeping it on offer
// would let an operator answer something nobody is listening for.
func TestAWithdrawnRequestStopsBeingPending(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	fake.emit(`{"type":"control_cancel_request","request_id":"req-a"}`)
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 0 })
}

// The end of the process withdraws every open question at once. A card with
// live buttons on a run that has stopped is an offer nothing can honour.
func TestTheEndOfTheProcessWithdrawsEveryPendingDecision(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	fake.emit(canUseToolFrame("req-b", "Write", "notes.md"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 2 })

	fake.end()
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 0 })
}

// The same request id arriving twice is a replay — a client re-attaching is
// handed the requests still in flight — and not a second decision.
func TestARepeatedRequestIDIsNotASecondDecision(t *testing.T) {
	fake := newFakeClaude()
	client, _ := openConversation(t, fake)

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })
	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	fake.emit(canUseToolFrame("req-b", "Write", "notes.md"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 2 })

	pending := client.PendingApprovals()
	if pending[0].ID != "req-a" || pending[1].ID != "req-b" {
		t.Fatalf("the questions are not in the order they were asked: %#v", pending)
	}
}

// A control request of a kind this package does not understand is left alone
// rather than answered blindly: a wrong answer is worse than a question that
// stays open, and it must not become history either.
func TestAnUnknownControlRequestIsNeitherAnsweredNorRecorded(t *testing.T) {
	fake := newFakeClaude()
	client, session := openConversation(t, fake)
	before := lastEventID(session)

	fake.emit(`{"type":"control_request","request_id":"req-z","request":{"subtype":"request_user_dialog","dialog_kind":"plan"}}`)
	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	if pending := client.PendingApprovals()[0]; pending.ID != "req-a" {
		t.Fatalf("an unknown control request became a decision: %#v", pending)
	}
	if decisions := decisionsWritten(fake); len(decisions) != 0 {
		t.Fatalf("an unknown control request was answered: %#v", decisions)
	}
	if events := session.Events(before); len(events) != 0 {
		t.Fatalf("a control request entered the history: %#v", events)
	}
}

// The collaborator is the door the viewer really uses, and it must show the
// same decision the client holds and deliver the same answer to the process.
func TestTheCollaboratorReadsAndAnswersALocalApproval(t *testing.T) {
	fake := newFakeClaude()
	client, session := openConversation(t, fake)
	registry := localrun.NewRegistry()
	registry.Register(session)
	collaborator := localrun.NewCollaborator(registry)
	req := execution.RunRequest{RunID: session.RunID()}

	fake.emit(canUseToolFrame("req-a", "Bash", "ls"))
	waitFor(t, func() bool { return len(client.PendingApprovals()) == 1 })

	pending, err := collaborator.ReadRunApprovals(context.Background(), req)
	if err != nil {
		t.Fatalf("reading the approvals failed: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "req-a" {
		t.Fatalf("the collaborator does not report the open decision: %#v", pending)
	}
	if err := collaborator.RespondRunApproval(context.Background(), req, "req-a", localrun.ApprovalAllow); err != nil {
		t.Fatalf("answering through the collaborator failed: %v", err)
	}
	if decisions := decisionsWritten(fake); len(decisions) != 1 || decisions[0]["behavior"] != "allow" {
		t.Fatalf("the answer did not reach the process: %#v", decisions)
	}
}

// A run with nothing open answers with an empty list, and that is an answer:
// no decision is pending, which is not the same as no answer being available.
func TestARunWithNothingOpenReportsNoApprovals(t *testing.T) {
	fake := newFakeClaude()
	_, session := openConversation(t, fake)
	registry := localrun.NewRegistry()
	registry.Register(session)
	collaborator := localrun.NewCollaborator(registry)

	pending, err := collaborator.ReadRunApprovals(context.Background(), execution.RunRequest{RunID: session.RunID()})
	if err != nil {
		t.Fatalf("reading the approvals failed: %v", err)
	}
	if pending == nil || len(pending) != 0 {
		t.Fatalf("approvals = %#v, want an empty list", pending)
	}
}
