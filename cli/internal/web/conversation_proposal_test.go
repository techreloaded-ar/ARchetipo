package web

// These tests are the server-side oracle of US-054: a proposal is read, then
// answered, and only an acceptance starts anything.
//
// Nothing on the start path is doubled. The record store, the connector, the
// process Template and the start functions are the production ones, and the
// only fake is the agent process that speaks the proposal — which is exactly
// the boundary the feature is about. A double on the start would make the
// parity of AC-2 an assertion about a call instead of about what the two doors
// leave behind them.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/connector"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/domain"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/template"
)

// proposalConversationResponse decodes the two fields US-054 adds to the
// conversation payload. It is written out separately, next to the existing
// conversationResponse, so the assertions are about the JSON the browser reads
// and never about the server's own structs.
type proposalConversationResponse struct {
	Conversation *struct {
		ID    string `json:"id"`
		State string `json:"state"`
	} `json:"conversation"`
	Proposal *struct {
		EventID           int64  `json:"event_id"`
		Action            string `json:"action"`
		Label             string `json:"label"`
		Scope             string `json:"scope"`
		SpecCode          string `json:"spec_code"`
		SpecTitle         string `json:"spec_title"`
		SpecStatus        string `json:"spec_status"`
		Runnable          bool   `json:"runnable"`
		UnavailableReason string `json:"unavailable_reason"`
		UnlockedBy        string `json:"unlocked_by"`
	} `json:"proposal"`
	Outcome *struct {
		ProposalID  int64  `json:"proposal_id"`
		Decision    string `json:"decision"`
		Action      string `json:"action"`
		Label       string `json:"label"`
		Scope       string `json:"scope"`
		SpecCode    string `json:"spec_code"`
		ExecutionID string `json:"execution_id"`
	} `json:"outcome"`
}

func decodeProposalConversation(t *testing.T, body string) proposalConversationResponse {
	t.Helper()
	var view proposalConversationResponse
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("undecodable conversation view: %v (%s)", err, body)
	}
	return view
}

// newProposalServer is newConversationServer with the connector kept, because
// the provider behaviour these tests dispatch — a real planning run — writes
// through it.
func newProposalServer(t *testing.T, provider execution.Provider) (*Server, connector.Connector) {
	t.Helper()
	srv, _, conn := newRunServer(t, provider, true)
	t.Cleanup(func() {
		ws := srv.session()
		if ws == nil {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), conversationCloseTimeout)
		defer cancel()
		_ = ws.conversation.shutdown(closeCtx)
	})
	return srv, conn
}

// proposalLine is what the agent closes a message with when it wants to
// propose. It is built with the production artifact name, so a test never
// hard-codes a vocabulary the parser could stop recognizing.
func proposalLine(action, code string) string {
	proposal := execution.ActionProposal{
		Artifact: execution.ActionProposalArtifact,
		Action:   action,
		Spec:     code,
	}
	line, err := json.Marshal(proposal)
	if err != nil {
		panic(err)
	}
	return string(line)
}

// emitProposal is the agent speaking: a sentence, then the proposal line, in
// one text event — the shape a real message has.
func emitProposal(t *testing.T, provider *conversingProvider, conversationID, sentence, action, code string) int64 {
	t.Helper()
	event := provider.emit(t, conversationID, localrun.KindText, sentence+"\n"+proposalLine(action, code))
	if event.ID == 0 {
		t.Fatalf("the emitted proposal has no event id: %#v", event)
	}
	return event.ID
}

func readProposalConversation(t *testing.T, srv *Server) proposalConversationResponse {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, "/api/workspace/conversation", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", w.Code, w.Body.String())
	}
	return decodeProposalConversation(t, w.Body.String())
}

func decideProposal(t *testing.T, srv *Server, proposalID int64, decision string) (int, proposalConversationResponse, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversation/proposal", map[string]any{
		"proposal_id": proposalID,
		"decision":    decision,
	})
	body := w.Body.String()
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		return w.Code, proposalConversationResponse{}, body
	}
	return w.Code, decodeProposalConversation(t, body), body
}

// specStatusOf reads the spec through the very connector the viewer serves, so
// "the spec did not move" is asserted against the backlog and not against a
// payload the route could have rendered from memory.
func specStatusOf(t *testing.T, srv *Server, code string) domain.Status {
	t.Helper()
	spec, err := srv.session().conn.ReadSpecDetail(context.Background(), code)
	if err != nil {
		t.Fatalf("reading %s: %v", code, err)
	}
	return spec.Status
}

func recordsOf(t *testing.T, srv *Server, code string) []execution.Execution {
	t.Helper()
	records, err := srv.session().store.ListBySpec(context.Background(), code)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func requireNoExecution(t *testing.T, srv *Server, code string) {
	t.Helper()
	if records := recordsOf(t, srv, code); len(records) != 0 {
		t.Fatalf("%d execution records exist for %s, want none: %#v", len(records), code, records)
	}
}

// openProposalConversation opens a conversation and answers with its id, which
// is what every emission is addressed to.
func openProposalConversation(t *testing.T, srv *Server) string {
	t.Helper()
	view := openConversationOK(t, srv)
	return view.Conversation.ID
}

// TestConversationCarriesThePendingProposalWithoutStartingAnything is AC-1:
// the agent proposes, the viewer resolves, and reading the proposal is not
// deciding it — no record, no transition, nothing but a card to answer.
func TestConversationCarriesThePendingProposalWithoutStartingAnything(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv, _ := newProposalServer(t, provider)
	conversationID := openProposalConversation(t, srv)

	eventID := emitProposal(t, provider, conversationID, "Posso pianificare US-901.", "plan", "US-901")

	view := readProposalConversation(t, srv)
	if view.Proposal == nil {
		t.Fatalf("the conversation carries no proposal after the agent made one")
	}
	if view.Proposal.EventID != eventID {
		t.Errorf("proposal.event_id = %d, want the event that carried it (%d)", view.Proposal.EventID, eventID)
	}
	if view.Proposal.Action != "plan" || view.Proposal.Label != "Pianifica" {
		t.Errorf("the proposal does not name the action: action=%q label=%q", view.Proposal.Action, view.Proposal.Label)
	}
	if view.Proposal.Scope != template.ScopeSpec || view.Proposal.SpecCode != "US-901" || view.Proposal.SpecTitle != "Da pianificare" {
		t.Errorf("the proposal does not name its target: %#v", view.Proposal)
	}
	if !view.Proposal.Runnable {
		t.Errorf("a proposal the process admits is not runnable: reason=%q", view.Proposal.UnavailableReason)
	}
	if view.Proposal.UnavailableReason != "" || view.Proposal.UnlockedBy != "" {
		t.Errorf("a runnable proposal carries a reason: %#v", view.Proposal)
	}
	if view.Outcome != nil {
		t.Errorf("an undecided proposal already has an outcome: %#v", view.Outcome)
	}
	// And the whole point: reading it started nothing.
	requireNoExecution(t, srv, "US-901")
	if status := specStatusOf(t, srv, "US-901"); status != domain.StatusTodo {
		t.Errorf("US-901 is %s after a proposal was merely read, want TODO", status)
	}
}

// TestAcceptingAProposalStartsTheSameExecutionAsTheBoard is AC-2. The parity is
// asserted on what the two doors leave behind — the record and the status of
// the spec — because that is what "the same execution" means to whoever reads
// the workspace afterwards.
func TestAcceptingAProposalStartsTheSameExecutionAsTheBoard(t *testing.T) {
	fromBoard := func(t *testing.T) (*Server, execution.Execution) {
		t.Helper()
		var conn connector.Connector
		provider := newConversingProvider("chatty", 0)
		provider.execute = func(ctx context.Context, request execution.Request) (execution.Result, error) {
			return planningExecute(conn)(ctx, request)
		}
		srv, backlog := newProposalServer(t, provider)
		conn = backlog

		status, started := startAction(t, srv, "US-901", "plan")
		if status != http.StatusCreated {
			t.Fatalf("POST board execution = %d, want 201: %v", status, started)
		}
		id, _ := started["id"].(string)
		return srv, awaitTerminal(t, srv, id)
	}

	fromConversation := func(t *testing.T) (*Server, execution.Execution) {
		t.Helper()
		var conn connector.Connector
		provider := newConversingProvider("chatty", 0)
		provider.execute = func(ctx context.Context, request execution.Request) (execution.Result, error) {
			return planningExecute(conn)(ctx, request)
		}
		srv, backlog := newProposalServer(t, provider)
		conn = backlog
		conversationID := openProposalConversation(t, srv)
		eventID := emitProposal(t, provider, conversationID, "Posso pianificare US-901.", "plan", "US-901")

		status, view, body := decideProposal(t, srv, eventID, "accept")
		if status != http.StatusCreated {
			t.Fatalf("POST proposal accept = %d, want 201: %s", status, body)
		}
		if view.Outcome == nil || view.Outcome.ExecutionID == "" {
			t.Fatalf("the acceptance does not name the execution it started: %s", body)
		}
		return srv, awaitTerminal(t, srv, view.Outcome.ExecutionID)
	}

	boardSrv, boardRecord := fromBoard(t)
	conversationSrv, conversationRecord := fromConversation(t)

	if boardRecord.Action != conversationRecord.Action {
		t.Errorf("action: board=%q conversation=%q", boardRecord.Action, conversationRecord.Action)
	}
	if boardRecord.SpecCode != conversationRecord.SpecCode {
		t.Errorf("spec: board=%q conversation=%q", boardRecord.SpecCode, conversationRecord.SpecCode)
	}
	if boardRecord.ProviderID != conversationRecord.ProviderID {
		t.Errorf("provider: board=%q conversation=%q", boardRecord.ProviderID, conversationRecord.ProviderID)
	}
	if boardRecord.Status != conversationRecord.Status {
		t.Errorf("terminal status: board=%q conversation=%q", boardRecord.Status, conversationRecord.Status)
	}
	boardStatus := specStatusOf(t, boardSrv, "US-901")
	conversationStatus := specStatusOf(t, conversationSrv, "US-901")
	if boardStatus != domain.StatusPlanned || conversationStatus != boardStatus {
		t.Errorf("the spec transition differs: board=%s conversation=%s", boardStatus, conversationStatus)
	}
	if got := len(recordsOf(t, conversationSrv, "US-901")); got != 1 {
		t.Errorf("the confirmation wrote %d records for US-901, want exactly 1", got)
	}
}

// TestAProposalTheProcessDoesNotAdmitIsRefusedWithItsOwnReason is AC-3: the
// reason is not written here, it is borrowed. The assertion compares the
// proposal's sentence with the one the board's own start route answers with for
// the identical case, character for character.
func TestAProposalTheProcessDoesNotAdmitIsRefusedWithItsOwnReason(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv, _ := newProposalServer(t, provider)
	conversationID := openProposalConversation(t, srv)

	// US-901 is TODO, and the process admits "review" only on a spec in REVIEW.
	eventID := emitProposal(t, provider, conversationID, "Posso rivedere US-901.", "review", "US-901")

	view := readProposalConversation(t, srv)
	if view.Proposal == nil {
		t.Fatalf("a proposal the process does not admit is not carried at all")
	}
	if view.Proposal.Runnable {
		t.Fatalf("a proposal the process does not admit is runnable: %#v", view.Proposal)
	}
	if view.Proposal.SpecCode != "US-901" || view.Proposal.Action != "review" {
		t.Errorf("a refused proposal must still name action and target: %#v", view.Proposal)
	}
	if view.Proposal.UnlockedBy == "" {
		t.Errorf("a refused proposal says nothing about what would unlock it: %#v", view.Proposal)
	}

	// The board refuses the very same start; the two sentences must be one.
	boardStatus, boardBody := startAction(t, srv, "US-901", "review")
	if boardStatus != http.StatusConflict {
		t.Fatalf("POST board execution = %d, want 409: %v", boardStatus, boardBody)
	}
	boardReason, _ := boardBody["error"].(string)
	if view.Proposal.UnavailableReason != boardReason {
		t.Errorf("the proposal refuses with %q, want the process's own sentence %q", view.Proposal.UnavailableReason, boardReason)
	}
	want := fmt.Sprintf("the %s process does not admit the %q action while %s is %s", template.FabbricaDelSoftware, "review", "US-901", domain.StatusTodo)
	if boardReason != want {
		t.Errorf("the refusal drifted from the process's formulation: %q, want %q", boardReason, want)
	}

	// And accepting it refuses too, with nothing started.
	status, _, body := decideProposal(t, srv, eventID, "accept")
	if status != http.StatusConflict {
		t.Fatalf("POST proposal accept = %d, want 409: %s", status, body)
	}
	if refused := refusalMessage(t, body); refused != view.Proposal.UnavailableReason {
		t.Errorf("the acceptance refuses with %q, want %q", refused, view.Proposal.UnavailableReason)
	}
	requireNoExecution(t, srv, "US-901")
	if got := specStatusOf(t, srv, "US-901"); got != domain.StatusTodo {
		t.Errorf("US-901 is %s after a refused acceptance, want TODO", got)
	}
}

// TestDecliningAProposalLeavesTheWorkspaceUntouched is AC-4: a refusal is not a
// failure. Nothing is created, nothing moves, and the conversation goes on.
func TestDecliningAProposalLeavesTheWorkspaceUntouched(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv, _ := newProposalServer(t, provider)
	conversationID := openProposalConversation(t, srv)
	eventID := emitProposal(t, provider, conversationID, "Posso pianificare US-901.", "plan", "US-901")

	status, view, body := decideProposal(t, srv, eventID, "decline")
	if status != http.StatusOK {
		t.Fatalf("POST proposal decline = %d, want 200: %s", status, body)
	}
	if view.Proposal != nil {
		t.Errorf("a declined proposal is still pending in the answer: %#v", view.Proposal)
	}
	if view.Outcome == nil || view.Outcome.Decision != conversationDecisionDeclined || view.Outcome.ProposalID != eventID {
		t.Errorf("the refusal is not recorded as the outcome: %#v", view.Outcome)
	}
	if view.Outcome != nil && view.Outcome.ExecutionID != "" {
		t.Errorf("a refusal names an execution: %#v", view.Outcome)
	}

	polled := readProposalConversation(t, srv)
	if polled.Proposal != nil {
		t.Errorf("the declined proposal comes back pending on the next poll: %#v", polled.Proposal)
	}
	if polled.Conversation == nil || polled.Conversation.State != string(execution.RunActive) {
		t.Fatalf("the conversation did not survive the refusal: %#v", polled.Conversation)
	}
	requireNoExecution(t, srv, "US-901")
	if got := specStatusOf(t, srv, "US-901"); got != domain.StatusTodo {
		t.Errorf("US-901 is %s after a refusal, want TODO", got)
	}

	// The conversation goes on: the next message is accepted and reaches the
	// agent process.
	sendStatus, _, sendBody := sendConversationMessage(t, srv, "Allora parliamone.")
	if sendStatus != http.StatusAccepted {
		t.Fatalf("POST message after a refusal = %d, want 202: %s", sendStatus, sendBody)
	}
	if sent := provider.dialogueOf(t, conversationID).messages(); len(sent) != 1 || sent[0] != "Allora parliamone." {
		t.Errorf("the message did not reach the agent after a refusal: %#v", sent)
	}
}

// TestAnAcceptedProposalNamesTheRunItStarted is AC-5: from the outcome one
// reaches the run that was born, by its id and with its target named.
func TestAnAcceptedProposalNamesTheRunItStarted(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv, _ := newProposalServer(t, provider)
	conversationID := openProposalConversation(t, srv)
	eventID := emitProposal(t, provider, conversationID, "Posso pianificare US-901.", "plan", "US-901")

	status, view, body := decideProposal(t, srv, eventID, "accept")
	if status != http.StatusCreated {
		t.Fatalf("POST proposal accept = %d, want 201: %s", status, body)
	}
	if view.Outcome == nil {
		t.Fatalf("an acceptance carries no outcome: %s", body)
	}
	if view.Outcome.Decision != conversationDecisionConfirmed || view.Outcome.ProposalID != eventID {
		t.Errorf("the outcome does not record the confirmation: %#v", view.Outcome)
	}
	if view.Outcome.Scope != template.ScopeSpec || view.Outcome.SpecCode != "US-901" || view.Outcome.Action != "plan" {
		t.Errorf("the outcome does not identify the target: %#v", view.Outcome)
	}
	if view.Proposal != nil {
		t.Errorf("a confirmed proposal is still pending: %#v", view.Proposal)
	}

	records := recordsOf(t, srv, "US-901")
	if len(records) != 1 {
		t.Fatalf("the confirmation wrote %d records, want exactly 1", len(records))
	}
	if view.Outcome.ExecutionID != records[0].ID {
		t.Errorf("outcome.execution_id = %q, want the record that was created (%q)", view.Outcome.ExecutionID, records[0].ID)
	}
	// The id is enough to reach the run: it is the same id the run routes serve.
	code, record := readExecution(t, srv, view.Outcome.ExecutionID)
	if code != http.StatusOK || record.SpecCode != "US-901" || record.Action != "plan" {
		t.Fatalf("the run named by the outcome is not reachable: %d %#v", code, record)
	}
	// The outcome survives the poll that follows, so a reload still finds the run.
	polled := readProposalConversation(t, srv)
	if polled.Outcome == nil || polled.Outcome.ExecutionID != view.Outcome.ExecutionID {
		t.Errorf("the outcome does not survive the next poll: %#v", polled.Outcome)
	}

	awaitTerminal(t, srv, view.Outcome.ExecutionID)
}

// TestOnlyTheCurrentProposalCanBeDecided: one decides what one is looking at.
// An id that is not the pending proposal is refused, and starts nothing.
func TestOnlyTheCurrentProposalCanBeDecided(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv, _ := newProposalServer(t, provider)
	conversationID := openProposalConversation(t, srv)

	stale := emitProposal(t, provider, conversationID, "Posso pianificare US-901.", "plan", "US-901")
	current := emitProposal(t, provider, conversationID, "Meglio ancora, pianifico US-902.", "plan", "US-902")
	if stale == current {
		t.Fatalf("the two proposals share the event id %d", stale)
	}

	status, _, body := decideProposal(t, srv, stale, "accept")
	if status != http.StatusConflict {
		t.Fatalf("accepting a superseded proposal = %d, want 409: %s", status, body)
	}
	requireNoExecution(t, srv, "US-901")
	requireNoExecution(t, srv, "US-902")

	view := readProposalConversation(t, srv)
	if view.Proposal == nil || view.Proposal.EventID != current {
		t.Fatalf("the pending proposal is not the last one: %#v", view.Proposal)
	}
	if view.Outcome != nil {
		t.Errorf("a refused decision was recorded as an outcome: %#v", view.Outcome)
	}
}

// TestASecondProposalSupersedesTheDecidedOne: a decision holds while the agent
// keeps talking, and a new proposal becomes pending again without the previous
// outcome being forgotten.
func TestASecondProposalSupersedesTheDecidedOne(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv, _ := newProposalServer(t, provider)
	conversationID := openProposalConversation(t, srv)

	first := emitProposal(t, provider, conversationID, "Posso pianificare US-901.", "plan", "US-901")
	status, _, body := decideProposal(t, srv, first, "decline")
	if status != http.StatusOK {
		t.Fatalf("POST proposal decline = %d, want 200: %s", status, body)
	}
	// The agent keeps talking: plain text does not resurrect the decided one.
	provider.emit(t, conversationID, localrun.KindText, "Va bene, non la pianifico.")
	if view := readProposalConversation(t, srv); view.Proposal != nil {
		t.Fatalf("the decided proposal came back pending after more talk: %#v", view.Proposal)
	}

	second := emitProposal(t, provider, conversationID, "E US-902 invece?", "plan", "US-902")
	view := readProposalConversation(t, srv)
	if view.Proposal == nil || view.Proposal.EventID != second {
		t.Fatalf("the new proposal is not pending: %#v", view.Proposal)
	}
	if view.Proposal.SpecCode != "US-902" || !view.Proposal.Runnable {
		t.Errorf("the new proposal is not resolved against the workspace: %#v", view.Proposal)
	}
	if view.Outcome == nil || view.Outcome.ProposalID != first || view.Outcome.Decision != conversationDecisionDeclined {
		t.Errorf("the previous decision was forgotten: %#v", view.Outcome)
	}
	if !strings.EqualFold(view.Outcome.SpecCode, "US-901") {
		t.Errorf("the outcome no longer names what was decided: %#v", view.Outcome)
	}
	requireNoExecution(t, srv, "US-901")
	requireNoExecution(t, srv, "US-902")
}
