package localrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// fakeDialogue stands in for the live process and for nothing else: the
// history, the refusals and the state under test are the production ones.
type fakeDialogue struct {
	mu         sync.Mutex
	sent       []string
	interrupts int
	sendErr    error
	stopErr    error
}

func (d *fakeDialogue) Send(_ context.Context, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sent = append(d.sent, text)
	return d.sendErr
}

func (d *fakeDialogue) Interrupt(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interrupts++
	return d.stopErr
}

func (d *fakeDialogue) delivered() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string{}, d.sent...)
}

func (d *fakeDialogue) stopped() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.interrupts
}

func newActiveRun(t *testing.T) (*Collaborator, *Session, *fakeDialogue) {
	t.Helper()
	collaborator := NewCollaborator(NewRegistry())
	session := NewSession("run-1", fixedClock())
	dialogue := &fakeDialogue{}
	session.AttachDialogue(dialogue)
	collaborator.Registry().Register(session)
	appendText(session, "text", "already in the history")
	return collaborator, session, dialogue
}

func historyJSON(t *testing.T, session *Session) string {
	t.Helper()
	body, err := json.Marshal(session.Events(0))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func request(runID string) execution.RunRequest {
	return execution.RunRequest{RunID: runID}
}

// AC-3 — the message reaches the process and the history stays untouched until
// the process re-emits it.
func TestSendRunMessageReachesTheProcessWithoutWritingHistory(t *testing.T) {
	collaborator, session, dialogue := newActiveRun(t)
	before := historyJSON(t, session)

	const sentinel = "riprendi dal punto due"
	if err := collaborator.SendRunMessage(context.Background(), request("run-1"), sentinel); err != nil {
		t.Fatalf("SendRunMessage failed: %v", err)
	}
	if got := dialogue.delivered(); len(got) != 1 || got[0] != sentinel {
		t.Fatalf("the process received %v; want exactly one %q", got, sentinel)
	}
	if after := historyJSON(t, session); after != before {
		t.Fatalf("the message entered the history before the process re-emitted it:\nbefore=%s\nafter =%s", before, after)
	}

	// The process re-emits it: only now is it part of the run's history.
	session.Append(execution.RunEvent{Kind: "user_message", Text: sentinel})
	events := session.Events(0)
	last := events[len(events)-1]
	if last.Kind != "user_message" || last.Text != sentinel {
		t.Fatalf("got %#v; want the re-emitted message", last)
	}
}

// AC-3 — an empty message is the caller's mistake and never reaches the process.
func TestSendRunMessageRefusesAnEmptyMessage(t *testing.T) {
	collaborator, _, dialogue := newActiveRun(t)
	for _, message := range []string{"", "   ", "\n\t"} {
		err := collaborator.SendRunMessage(context.Background(), request("run-1"), message)
		reason, refused := execution.RefusalOf(err)
		if !refused || reason != execution.RunRefusedUnsupported {
			t.Fatalf("message %q got %v; want an unsupported refusal", message, err)
		}
	}
	if got := dialogue.delivered(); len(got) != 0 {
		t.Fatalf("an empty message reached the process: %v", got)
	}
}

// AC-4 — cancelling asks the process to stop and reports the state the session
// observes, which is still ACTIVE until the process really ends.
func TestCancelRunNeverDerivesATerminalState(t *testing.T) {
	collaborator, session, dialogue := newActiveRun(t)

	if err := collaborator.CancelRun(context.Background(), request("run-1")); err != nil {
		t.Fatalf("CancelRun failed: %v", err)
	}
	if got := dialogue.stopped(); got != 1 {
		t.Fatalf("the process was asked to stop %d times; want once", got)
	}
	snapshot, err := collaborator.ReadRun(context.Background(), request("run-1"))
	if err != nil {
		t.Fatalf("ReadRun failed: %v", err)
	}
	if snapshot.State != execution.RunActive || snapshot.ClosedAt != nil {
		t.Fatalf("got %#v; want the run still active until the process ends", snapshot)
	}

	// The end of the process is what closes the run.
	session.Close(execution.RunClosed, "")
	snapshot, err = collaborator.ReadRun(context.Background(), request("run-1"))
	if err != nil {
		t.Fatalf("ReadRun failed: %v", err)
	}
	if snapshot.State != execution.RunClosed || snapshot.ClosedAt == nil {
		t.Fatalf("got %#v; want the observed terminal state", snapshot)
	}
}

// AC-5 — on a run that is over, both commands are refused with the reason and
// the history is byte-for-byte what it was.
func TestCommandsOnAClosedRunAreRefusedAndChangeNothing(t *testing.T) {
	collaborator, session, dialogue := newActiveRun(t)
	appendText(session, "text", "the last thing the agent said")
	session.Close(execution.RunClosed, "")
	before := historyJSON(t, session)

	sendErr := collaborator.SendRunMessage(context.Background(), request("run-1"), "sei ancora lì?")
	reason, refused := execution.RefusalOf(sendErr)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("SendRunMessage got %v; want a run_not_active refusal", sendErr)
	}
	cancelErr := collaborator.CancelRun(context.Background(), request("run-1"))
	reason, refused = execution.RefusalOf(cancelErr)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("CancelRun got %v; want a run_not_active refusal", cancelErr)
	}
	for _, err := range []error{sendErr, cancelErr} {
		if err.Error() == "" {
			t.Fatal("a refusal must state a reason")
		}
	}
	if got := dialogue.delivered(); len(got) != 0 {
		t.Fatalf("a refused message still reached the process: %v", got)
	}
	if got := dialogue.stopped(); got != 0 {
		t.Fatalf("a refused cancellation still reached the process: %d", got)
	}
	if after := historyJSON(t, session); after != before {
		t.Fatalf("the history changed across two refused commands:\nbefore=%s\nafter =%s", before, after)
	}
	// The run is still readable after it ended: that is why a finished session
	// stays in the registry.
	if events := session.Events(0); len(events) != 2 {
		t.Fatalf("the history of a finished run must stay readable, got %d events", len(events))
	}
}

// A run this process never opened is not_found, which is a different situation
// from a run that has ended.
func TestCommandsOnAnUnknownRunAreNotFound(t *testing.T) {
	collaborator, _, _ := newActiveRun(t)
	ctx := context.Background()
	checks := map[string]error{
		"ReadRun":     errorOfSnapshot(collaborator.ReadRun(ctx, request("run-nope"))),
		"Approvals":   errorOfApprovals(collaborator.ReadRunApprovals(ctx, request("run-nope"))),
		"Send":        collaborator.SendRunMessage(ctx, request("run-nope"), "ciao"),
		"Cancel":      collaborator.CancelRun(ctx, request("run-nope")),
		"Stream":      collaborator.StreamRunEvents(ctx, request("run-nope"), 0, func(execution.RunEvent) error { return nil }),
		"EmptyRunID":  collaborator.CancelRun(ctx, request("  ")),
		"RespondAppr": collaborator.RespondRunApproval(ctx, request("run-nope"), "a", "b"),
	}
	for name, err := range checks {
		reason, refused := execution.RefusalOf(err)
		if !refused || reason != execution.RunRefusedNotFound {
			t.Fatalf("%s got %v; want a not_found refusal", name, err)
		}
	}
}

func errorOfSnapshot(_ execution.RunSnapshot, err error) error { return err }

func errorOfApprovals(_ []execution.PendingApproval, err error) error { return err }

// ResolveRun answers with the run when there is one and with absence when there
// is not — absence being an answer, never a failure.
func TestResolveRunAnswersAbsenceWithoutFailing(t *testing.T) {
	collaborator, _, _ := newActiveRun(t)
	runID, err := collaborator.ResolveRun(context.Background(), execution.Execution{ID: "run-1"}, nil)
	if err != nil || runID != "run-1" {
		t.Fatalf("got %q, %v; want run-1, nil", runID, err)
	}
	runID, err = collaborator.ResolveRun(context.Background(), execution.Execution{ID: "exec-without-session"}, nil)
	if err != nil || runID != "" {
		t.Fatalf("got %q, %v; want an empty id and no error", runID, err)
	}
	runID, err = collaborator.ResolveRun(context.Background(), execution.Execution{}, nil)
	if err != nil || runID != "" {
		t.Fatalf("got %q, %v; want an empty id and no error", runID, err)
	}
}

// A dialogue that never asks says so without ever failing, and refuses a
// decision it has none of. It is the behaviour every local run had before the
// permission bridge existed, and the one a provider whose process cannot ask
// still has: absence of questions is not absence of an answer.
func TestADialogueThatNeverAsksReportsNoApprovals(t *testing.T) {
	collaborator, _, _ := newActiveRun(t)
	approvals, err := collaborator.ReadRunApprovals(context.Background(), request("run-1"))
	if err != nil {
		t.Fatalf("ReadRunApprovals failed: %v", err)
	}
	if approvals == nil || len(approvals) != 0 {
		t.Fatalf("got %#v; want an empty, non-nil list", approvals)
	}
	err = collaborator.RespondRunApproval(context.Background(), request("run-1"), "appr", "opt")
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedUnsupported {
		t.Fatalf("got %v; want an unsupported refusal", err)
	}
}

// A refusal the process itself expressed travels back unchanged; anything else
// stays a fault, because nothing was decided.
func TestDeliveryPreservesTheProcessRefusal(t *testing.T) {
	collaborator, _, dialogue := newActiveRun(t)
	dialogue.sendErr = &execution.RunCommandError{Reason: execution.RunRefusedNotActive, RunID: "run-1"}
	err := collaborator.SendRunMessage(context.Background(), request("run-1"), "ciao")
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("got %v; want the process refusal unchanged", err)
	}

	broken := errors.New("the pipe is gone")
	dialogue.sendErr = broken
	err = collaborator.SendRunMessage(context.Background(), request("run-1"), "ciao")
	if _, refused := execution.RefusalOf(err); refused {
		t.Fatalf("a delivery failure must not become a refusal: %v", err)
	}
	if !errors.Is(err, broken) {
		t.Fatalf("got %v; want the underlying failure preserved", err)
	}
}

// A command sent before the process is attached is transient, not terminal.
func TestCommandsBeforeTheProcessIsAttachedAreTransient(t *testing.T) {
	collaborator := NewCollaborator(NewRegistry())
	session := NewSession("run-2", fixedClock())
	collaborator.Registry().Register(session)

	err := collaborator.SendRunMessage(context.Background(), request("run-2"), "ciao")
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedRunnerOffline {
		t.Fatalf("got %v; want a runner_offline refusal", err)
	}
	if fmt.Sprint(err) == "" {
		t.Fatal("a refusal must state a reason")
	}
}

// arbiterDialogue is a process that stops to ask. It carries the two methods of
// Arbiter and nothing else, so what is exercised here is the discovery and the
// routing this package does — never a provider's protocol.
type arbiterDialogue struct {
	fakeDialogue
	pending   []execution.PendingApproval
	answered  [][2]string
	answerErr error
}

func (d *arbiterDialogue) PendingApprovals() []execution.PendingApproval { return d.pending }

func (d *arbiterDialogue) RespondApproval(_ context.Context, approvalID, optionID string) error {
	d.answered = append(d.answered, [2]string{approvalID, optionID})
	return d.answerErr
}

// A dialogue that asks is discovered on the session, and its questions and its
// answers travel through the collaborator unchanged.
func TestADialogueThatAsksIsReadAndAnswered(t *testing.T) {
	session := NewSession("run-1", nil)
	dialogue := &arbiterDialogue{pending: []execution.PendingApproval{{ID: "appr-1", ToolName: "Bash"}}}
	session.AttachDialogue(dialogue)
	registry := NewRegistry()
	registry.Register(session)
	collaborator := NewCollaborator(registry)

	approvals, err := collaborator.ReadRunApprovals(context.Background(), request("run-1"))
	if err != nil {
		t.Fatalf("ReadRunApprovals failed: %v", err)
	}
	if len(approvals) != 1 || approvals[0].ID != "appr-1" {
		t.Fatalf("got %#v; want the one open decision", approvals)
	}
	if err := collaborator.RespondRunApproval(context.Background(), request("run-1"), "appr-1", ApprovalAllow); err != nil {
		t.Fatalf("RespondRunApproval failed: %v", err)
	}
	if len(dialogue.answered) != 1 || dialogue.answered[0] != [2]string{"appr-1", ApprovalAllow} {
		t.Fatalf("the answer did not reach the dialogue: %#v", dialogue.answered)
	}
}

// An answer with nothing named is the caller's mistake, decided here rather
// than spent on a round trip to the process to be told so.
func TestAnEmptyApprovalOrOptionIsRefused(t *testing.T) {
	session := NewSession("run-1", nil)
	dialogue := &arbiterDialogue{}
	session.AttachDialogue(dialogue)
	registry := NewRegistry()
	registry.Register(session)
	collaborator := NewCollaborator(registry)

	for _, tc := range [][2]string{{"", ApprovalAllow}, {"appr-1", "  "}} {
		err := collaborator.RespondRunApproval(context.Background(), request("run-1"), tc[0], tc[1])
		reason, refused := execution.RefusalOf(err)
		if !refused || reason != execution.RunRefusedUnsupported {
			t.Fatalf("RespondRunApproval(%q, %q) = %v; want an unsupported refusal", tc[0], tc[1], err)
		}
	}
	if len(dialogue.answered) != 0 {
		t.Fatalf("a refused answer still reached the process: %#v", dialogue.answered)
	}
}

// The approvals of a run that has ended are an empty list and never a refusal:
// a finished run has no decision open, which is an answer.
func TestApprovalsOfAFinishedRunAreEmpty(t *testing.T) {
	session := NewSession("run-1", nil)
	session.AttachDialogue(&arbiterDialogue{pending: []execution.PendingApproval{{ID: "appr-1"}}})
	registry := NewRegistry()
	registry.Register(session)
	collaborator := NewCollaborator(registry)
	session.Close(execution.RunClosed, "")

	if _, err := collaborator.ReadRunApprovals(context.Background(), request("run-1")); err != nil {
		t.Fatalf("reading the approvals of a finished run failed: %v", err)
	}
	err := collaborator.RespondRunApproval(context.Background(), request("run-1"), "appr-1", ApprovalAllow)
	reason, refused := execution.RefusalOf(err)
	if !refused || reason != execution.RunRefusedNotActive {
		t.Fatalf("got %v; want a run_not_active refusal", err)
	}
}
