package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
)

// fakeDialogue is the channel toward the agent process of a conversation.
//
// It writes nothing into the history, and that absence is the point: a message
// becomes history when the process re-emits it, so a test that wants to see it
// there has to emit it explicitly — which is exactly what the production rule
// says happens.
type fakeDialogue struct {
	mu        sync.Mutex
	sent      []string
	interrupt int
}

func (d *fakeDialogue) Send(_ context.Context, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sent = append(d.sent, text)
	return nil
}

func (d *fakeDialogue) Interrupt(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interrupt++
	return nil
}

func (d *fakeDialogue) messages() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.sent...)
}

// conversingProvider is a provider that can hold a conversation. Only the agent
// process is a double: the session, the collaborator, the retention window, the
// routes and the JSON serialization are the production ones, so what these
// tests assert is the boundary the browser really talks to.
//
// It deliberately declares no capability of its own beyond the one every test
// provider declares: workspace.converse is derived from the interface, and a
// provider that declared it by hand would be testing the declaration instead of
// the derivation.
type conversingProvider struct {
	*runTestProvider
	*localrun.Collaborator

	// retain bounds the history of the sessions it opens, so a test can produce
	// a genuinely partial history instead of a simulated one.
	retain int
	// openErr, when set, is what the provider answers instead of opening.
	openErr error

	mu        sync.Mutex
	opened    []execution.ConversationRequest
	closed    []string
	sessions  map[string]*localrun.Session
	dialogues map[string]*fakeDialogue
}

func newConversingProvider(id string, retain int) *conversingProvider {
	return &conversingProvider{
		runTestProvider: releasedProvider(id, nil),
		Collaborator:    localrun.NewCollaborator(localrun.NewRegistry()),
		retain:          retain,
		sessions:        make(map[string]*localrun.Session),
		dialogues:       make(map[string]*fakeDialogue),
	}
}

func (p *conversingProvider) OpenConversation(_ context.Context, req execution.ConversationRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = append(p.opened, req)
	if p.openErr != nil {
		return p.openErr
	}
	session := localrun.NewSession(req.ConversationID, nil)
	if p.retain > 0 {
		session = localrun.NewBoundedSession(req.ConversationID, nil, p.retain)
	}
	dialogue := &fakeDialogue{}
	session.AttachDialogue(dialogue)
	p.Registry().Register(session)
	p.sessions[req.ConversationID] = session
	p.dialogues[req.ConversationID] = dialogue
	return nil
}

// CloseConversation releases the conversation and closes its session, keeping
// it in the registry — the history of a conversation that has ended stays
// readable, which is the rule localrun.Registry already writes down.
func (p *conversingProvider) CloseConversation(_ context.Context, conversationID string) error {
	p.mu.Lock()
	session := p.sessions[conversationID]
	p.closed = append(p.closed, conversationID)
	p.mu.Unlock()
	if session != nil {
		session.Close(execution.RunClosed, "")
	}
	return nil
}

func (p *conversingProvider) openings() []execution.ConversationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]execution.ConversationRequest(nil), p.opened...)
}

func (p *conversingProvider) closings() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.closed...)
}

func (p *conversingProvider) sessionOf(t *testing.T, id string) *localrun.Session {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	session, ok := p.sessions[id]
	if !ok {
		t.Fatalf("the provider holds no session for the conversation %q", id)
	}
	return session
}

func (p *conversingProvider) dialogueOf(t *testing.T, id string) *fakeDialogue {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	dialogue, ok := p.dialogues[id]
	if !ok {
		t.Fatalf("the provider holds no dialogue for the conversation %q", id)
	}
	return dialogue
}

// emit is the agent process speaking: it is the only way an event ever enters
// the history of a conversation in these tests.
func (p *conversingProvider) emit(t *testing.T, id, kind, text string) execution.RunEvent {
	t.Helper()
	return p.sessionOf(t, id).Append(execution.RunEvent{Kind: kind, Text: text})
}

var (
	_ execution.Provider          = (*conversingProvider)(nil)
	_ execution.Conversationalist = (*conversingProvider)(nil)
	_ execution.RunCollaborator   = (*conversingProvider)(nil)
)

// conversationResponse decodes the wire shape the browser receives, written out
// on purpose so the assertions are about the JSON and not about the server's
// own struct.
type conversationResponse struct {
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason"`
	ProviderID        string `json:"provider_id"`
	Conversation      *struct {
		ID         string `json:"id"`
		State      string `json:"state"`
		Error      string `json:"error"`
		WorkingDir string `json:"working_dir"`
		OpenedAt   string `json:"opened_at"`
	} `json:"conversation"`
	Events []struct {
		ID   int64  `json:"id"`
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"events"`
	LastID    int64  `json:"last_id"`
	Truncated bool   `json:"truncated"`
	Notice    string `json:"notice"`
	// Runs is the block list the flow draws inside the conversation. It is
	// decoded from the wire like everything else here, so the assertions are
	// about the JSON the browser reads and not about the server's own struct.
	Runs []conversationRunResponse `json:"runs"`
}

// conversationRunResponse is one run block of the conversation, as the browser
// receives it.
type conversationRunResponse struct {
	ExecutionID   string `json:"execution_id"`
	AnchorEventID int64  `json:"anchor_event_id"`
	Action        string `json:"action"`
	Label         string `json:"label"`
	Scope         string `json:"scope"`
	SpecCode      string `json:"spec_code"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	Decision      string `json:"decision"`
	Run           *struct {
		RunID string `json:"run_id"`
		State string `json:"state"`
	} `json:"run"`
	Events []struct {
		ID   int64  `json:"id"`
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"events"`
	Approvals []struct {
		ID       string          `json:"id"`
		ToolName string          `json:"tool_name"`
		Title    string          `json:"title"`
		Args     json.RawMessage `json:"args"`
	} `json:"approvals"`
	AwaitingResponse bool   `json:"awaiting_response"`
	Notice           string `json:"notice"`
}

func decodeConversation(t *testing.T, body string) conversationResponse {
	t.Helper()
	var view conversationResponse
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("undecodable conversation view: %v (%s)", err, body)
	}
	return view
}

func conversationPath(afterID int64) string {
	if afterID <= 0 {
		return "/api/workspace/conversation"
	}
	return "/api/workspace/conversation?after_id=" + strconv.FormatInt(afterID, 10)
}

func readConversation(t *testing.T, srv *Server, afterID int64) (int, conversationResponse, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodGet, conversationPath(afterID), nil)
	if w.Code != http.StatusOK {
		return w.Code, conversationResponse{}, w.Body.String()
	}
	return w.Code, decodeConversation(t, w.Body.String()), w.Body.String()
}

func openConversation(t *testing.T, srv *Server) (int, conversationResponse, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversation", map[string]any{})
	if w.Code != http.StatusCreated {
		return w.Code, conversationResponse{}, w.Body.String()
	}
	return w.Code, decodeConversation(t, w.Body.String()), w.Body.String()
}

func sendConversationMessage(t *testing.T, srv *Server, message string) (int, conversationResponse, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversation/messages", map[string]any{"message": message})
	if w.Code != http.StatusAccepted {
		return w.Code, conversationResponse{}, w.Body.String()
	}
	return w.Code, decodeConversation(t, w.Body.String()), w.Body.String()
}

func closeConversation(t *testing.T, srv *Server) (int, conversationResponse, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodDelete, "/api/workspace/conversation", nil)
	if w.Code != http.StatusOK {
		return w.Code, conversationResponse{}, w.Body.String()
	}
	return w.Code, decodeConversation(t, w.Body.String()), w.Body.String()
}

// newConversationServer is a viewer on a real workspace whose default provider
// is the fake that can converse.
func newConversationServer(t *testing.T, provider execution.Provider) *Server {
	t.Helper()
	srv, _, _ := newRunServer(t, provider, true)
	t.Cleanup(func() {
		ws := srv.session()
		if ws == nil {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), conversationCloseTimeout)
		defer cancel()
		_ = ws.conversation.shutdown(closeCtx)
	})
	return srv
}

func openConversationOK(t *testing.T, srv *Server) conversationResponse {
	t.Helper()
	status, view, body := openConversation(t, srv)
	if status != http.StatusCreated {
		t.Fatalf("POST conversation = %d, want 201: %s", status, body)
	}
	if view.Conversation == nil {
		t.Fatalf("the open conversation is null: %s", body)
	}
	return view
}

// refusalMessage reads the sentence out of a refusal, so an assertion compares
// the phrase itself and not its JSON escaping.
func refusalMessage(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("undecodable refusal: %v (%s)", err, body)
	}
	return payload.Error
}

func conversationEventTexts(view conversationResponse) []string {
	out := make([]string, 0, len(view.Events))
	for _, event := range view.Events {
		out = append(out, event.Text)
	}
	return out
}

// TestConversationRoutesRefuseWithoutAnOpenWorkspace is the gate: a
// conversation is about a workspace, so with none open the four routes refuse
// instead of answering emptily.
func TestConversationRoutesRefuseWithoutAnOpenWorkspace(t *testing.T) {
	srv, _ := homeServer(t)

	routes := []scopedRoute{
		{"GET /api/workspace/conversation", http.MethodGet, "/api/workspace/conversation", ""},
		{"POST /api/workspace/conversation", http.MethodPost, "/api/workspace/conversation", "{}"},
		{"POST /api/workspace/conversation/messages", http.MethodPost, "/api/workspace/conversation/messages", `{"message":"hi"}`},
		{"DELETE /api/workspace/conversation", http.MethodDelete, "/api/workspace/conversation", ""},
	}
	for _, route := range routes {
		t.Run(route.pattern, func(t *testing.T) {
			rec := callRoute(t, srv, route)
			if rec.Code != http.StatusConflict {
				t.Fatalf("%s = %d, want 409: %s", route.pattern, rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding the refusal: %v (%s)", err, rec.Body.String())
			}
			if open, ok := body["workspaceOpen"].(bool); !ok || open {
				t.Errorf("workspaceOpen = %v, want false", body["workspaceOpen"])
			}
		})
	}
}

// TestConversationIsNotOfferedByAProviderThatCannotConverse is AC-4: the
// provider is available — its runtime is usable, it executes actions — and it
// simply cannot hold a conversation. The reason names the capability, and the
// same sentence refuses the open.
func TestConversationIsNotOfferedByAProviderThatCannotConverse(t *testing.T) {
	provider := releasedProvider("mute", nil)
	srv := newConversationServer(t, provider)

	status, view, body := readConversation(t, srv, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200: %s", status, body)
	}
	if view.Available {
		t.Errorf("available = true for a provider that cannot converse: %s", body)
	}
	if !strings.Contains(view.UnavailableReason, string(execution.CapabilityWorkspaceConverse)) {
		t.Errorf("unavailable_reason = %q, want it to name %q", view.UnavailableReason, execution.CapabilityWorkspaceConverse)
	}
	if view.Conversation != nil {
		t.Errorf("conversation is not null with no conversation open: %s", body)
	}

	openStatus, _, openBody := openConversation(t, srv)
	if openStatus != http.StatusConflict {
		t.Fatalf("POST conversation = %d, want 409: %s", openStatus, openBody)
	}
	if refused := refusalMessage(t, openBody); refused != view.UnavailableReason {
		t.Errorf("the refusal says %q, want the very reason the GET declares: %q", refused, view.UnavailableReason)
	}
	// The provider received no opening — it has no way to receive one, and the
	// workspace is holding nothing that would have to be closed later.
	if _, open := srv.session().conversation.current(); open {
		t.Error("a conversation was opened with a provider that cannot converse")
	}
	// And the derivation itself: nothing declares the capability.
	capabilities, err := execution.DeclaredCapabilities(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Supports(capabilities, execution.CapabilityWorkspaceConverse) {
		t.Errorf("a provider that does not implement Conversationalist declares %q", execution.CapabilityWorkspaceConverse)
	}
}

// TestOpeningAConversationWritesNoExecution is AC-2: a conversation is not an
// action of the process, so it leaves no record, no file, and does not show up
// among the actions of the workspace.
func TestOpeningAConversationWritesNoExecution(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	ws := srv.session()

	before, _ := readWorkspaceActions(t, srv)
	if before.Execution != nil {
		t.Fatalf("the workspace already has an execution before anything ran: %#v", before.Execution)
	}

	openConversationOK(t, srv)

	records, err := ws.store.ListBySpec(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("opening a conversation wrote %d execution records", len(records))
	}
	dir := filepath.Join(ws.cfg.ProjectRoot, ".archetipo", "executions")
	if entries, err := os.ReadDir(dir); err == nil && len(entries) != 0 {
		t.Errorf("%s holds %d entries after a conversation was opened", dir, len(entries))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	after, raw := readWorkspaceActions(t, srv)
	if after.Execution != nil {
		t.Errorf("the conversation appears as the execution of the workspace: %s", raw)
	}
	if strings.Contains(raw, "conv-") {
		t.Errorf("the conversation leaked into the workspace actions: %s", raw)
	}
}

// TestOpeningAConversationPassesTheWorkspaceProjectRoot is AC-1: where the
// agent has to work is a fact of the workspace that opened the conversation,
// and it travels on the request.
func TestOpeningAConversationPassesTheWorkspaceProjectRoot(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	ws := srv.session()

	view := openConversationOK(t, srv)

	openings := provider.openings()
	if len(openings) != 1 {
		t.Fatalf("the provider received %d openings, want exactly 1", len(openings))
	}
	if openings[0].WorkingDir != ws.cfg.ProjectRoot {
		t.Errorf("WorkingDir = %q, want the project root of the open workspace %q", openings[0].WorkingDir, ws.cfg.ProjectRoot)
	}
	if openings[0].ConversationID != view.Conversation.ID {
		t.Errorf("the provider opened %q while the view reports %q", openings[0].ConversationID, view.Conversation.ID)
	}
	if !strings.HasPrefix(view.Conversation.ID, conversationIDPrefix) {
		t.Errorf("conversation id = %q, want the %q prefix that keeps it out of the execution namespace", view.Conversation.ID, conversationIDPrefix)
	}
	if view.Conversation.WorkingDir != ws.cfg.ProjectRoot {
		t.Errorf("the view reports working_dir = %q, want %q", view.Conversation.WorkingDir, ws.cfg.ProjectRoot)
	}
	if view.Conversation.State != string(execution.RunActive) {
		t.Errorf("state = %q, want %q", view.Conversation.State, execution.RunActive)
	}
	if !view.Available {
		t.Errorf("available = false right after a conversation was opened")
	}

	// The reading of a live conversation reports the history the session holds.
	provider.emit(t, view.Conversation.ID, "text", "hello")
	_, read, body := readConversation(t, srv, 0)
	if got := conversationEventTexts(read); len(got) != 1 || got[0] != "hello" {
		t.Errorf("events = %v, want the one the process emitted: %s", got, body)
	}
}

// TestASecondConversationIsRefused: a workspace holds one conversation, and the
// refusal names the one it already has instead of starting a second process
// nobody holds the handle for.
func TestASecondConversationIsRefused(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	first := openConversationOK(t, srv)

	status, _, body := openConversation(t, srv)
	if status != http.StatusConflict {
		t.Fatalf("second POST conversation = %d, want 409: %s", status, body)
	}
	if !strings.Contains(body, first.Conversation.ID) {
		t.Errorf("the refusal %s does not name the conversation %q already open", body, first.Conversation.ID)
	}
	if openings := provider.openings(); len(openings) != 1 {
		t.Errorf("the provider received %d openings, want exactly 1", len(openings))
	}
}

// TestConversationHistoryIsReadTwiceIdentically is AC-3, and it is the oracle
// of a page reload: the history lives in this process, so re-reading it from
// the same cursor gives back exactly the same events.
func TestConversationHistoryIsReadTwiceIdentically(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	provider.emit(t, id, "text", "one")
	provider.emit(t, id, "text", "two")
	last := provider.emit(t, id, "text", "three")

	_, first, firstBody := readConversation(t, srv, 0)
	if got := conversationEventTexts(first); len(got) != 3 {
		t.Fatalf("events = %v, want the three the process emitted: %s", got, firstBody)
	}
	if first.LastID != last.ID {
		t.Errorf("last_id = %d, want %d", first.LastID, last.ID)
	}
	if first.Truncated {
		t.Errorf("truncated = true on a whole history: %s", firstBody)
	}
	if first.Notice != "" {
		t.Errorf("notice = %q on a whole history", first.Notice)
	}

	_, second, secondBody := readConversation(t, srv, 0)
	if firstBody != secondBody {
		t.Errorf("re-reading the same cursor answered differently:\nfirst:  %s\nsecond: %s", firstBody, secondBody)
	}
	if second.Conversation == nil || second.Conversation.ID != id {
		t.Errorf("the second read lost the conversation: %s", secondBody)
	}

	// And a cursor in the middle returns only what follows it.
	_, tail, tailBody := readConversation(t, srv, first.Events[0].ID)
	if got := conversationEventTexts(tail); len(got) != 2 || got[0] != "two" {
		t.Errorf("events after the first = %v: %s", got, tailBody)
	}
}

// TestPartialConversationHistoryIsDeclared is the other half of AC-3: a history
// the retention window has cut is declared as partial, never shown as if it
// were whole.
func TestPartialConversationHistoryIsDeclared(t *testing.T) {
	provider := newConversingProvider("chatty", 2)
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	dropped := provider.emit(t, id, "text", "one")
	provider.emit(t, id, "text", "two")
	provider.emit(t, id, "text", "three")
	provider.emit(t, id, "text", "four")

	_, partial, body := readConversation(t, srv, dropped.ID)
	if !partial.Truncated {
		t.Errorf("truncated = false while the window dropped events: %s", body)
	}
	if strings.TrimSpace(partial.Notice) == "" {
		t.Errorf("a partial history carries no notice: %s", body)
	}
	if got := conversationEventTexts(partial); len(got) != 2 {
		t.Errorf("events = %v, want the two the window still keeps: %s", got, body)
	}

	// A cursor the window has not overtaken is not partial.
	_, whole, wholeBody := readConversation(t, srv, partial.Events[0].ID)
	if whole.Truncated {
		t.Errorf("truncated = true for a cursor the window has not overtaken: %s", wholeBody)
	}
}

// TestAConversationMessageEntersHistoryOnlyWhenTheProcessEchoesIt: the 202 does
// not contain the message, and it appears — once — only after the process
// re-emits it.
func TestAConversationMessageEntersHistoryOnlyWhenTheProcessEchoesIt(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	id := view.Conversation.ID

	status, accepted, body := sendConversationMessage(t, srv, "ciao")
	if status != http.StatusAccepted {
		t.Fatalf("POST message = %d, want 202: %s", status, body)
	}
	if len(accepted.Events) != 0 {
		t.Errorf("the 202 already carries the message: %s", body)
	}
	if sent := provider.dialogueOf(t, id).messages(); len(sent) != 1 || sent[0] != "ciao" {
		t.Errorf("the process received %v, want exactly one \"ciao\"", sent)
	}

	_, before, _ := readConversation(t, srv, 0)
	if len(before.Events) != 0 {
		t.Errorf("the message entered the history before the process re-emitted it: %v", conversationEventTexts(before))
	}

	provider.emit(t, id, "user", "ciao")
	_, after, afterBody := readConversation(t, srv, 0)
	if got := conversationEventTexts(after); len(got) != 1 || got[0] != "ciao" {
		t.Errorf("events = %v, want the message exactly once: %s", got, afterBody)
	}
}

// TestAnEmptyConversationMessageIsRefused: the caller's mistake, refused before
// anything is delivered.
func TestAnEmptyConversationMessageIsRefused(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	view := openConversationOK(t, srv)

	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversation/messages", map[string]any{"message": "   "})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST empty message = %d, want 400: %s", w.Code, w.Body.String())
	}
	if sent := provider.dialogueOf(t, view.Conversation.ID).messages(); len(sent) != 0 {
		t.Errorf("an empty message was delivered anyway: %v", sent)
	}
}

// TestClosingAConversationReleasesTheProviderOnce is AC-6: the close reaches
// the provider exactly once, the view it answers with still holds the history,
// and everything that comes after is refused.
func TestClosingAConversationReleasesTheProviderOnce(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	provider.emit(t, id, "text", "hello")

	status, closed, body := closeConversation(t, srv)
	if status != http.StatusOK {
		t.Fatalf("DELETE conversation = %d, want 200: %s", status, body)
	}
	if closed.Conversation == nil || closed.Conversation.ID != id {
		t.Fatalf("the final view lost the conversation it closed: %s", body)
	}
	if closed.Conversation.State != string(execution.RunClosed) {
		t.Errorf("state = %q, want %q", closed.Conversation.State, execution.RunClosed)
	}
	if got := conversationEventTexts(closed); len(got) != 1 || got[0] != "hello" {
		t.Errorf("the history is no longer readable after the close: %s", body)
	}
	if closings := provider.closings(); len(closings) != 1 || closings[0] != id {
		t.Fatalf("the provider was closed with %v, want exactly one %q", closings, id)
	}

	secondStatus, _, secondBody := closeConversation(t, srv)
	if secondStatus != http.StatusConflict {
		t.Fatalf("second DELETE = %d, want 409: %s", secondStatus, secondBody)
	}
	if closings := provider.closings(); len(closings) != 1 {
		t.Errorf("the second DELETE reached the provider: %v", closings)
	}

	messageStatus, _, messageBody := sendConversationMessage(t, srv, "still there?")
	if messageStatus != http.StatusConflict {
		t.Fatalf("POST message after the close = %d, want 409: %s", messageStatus, messageBody)
	}

	_, view, readBody := readConversation(t, srv, 0)
	if view.Conversation != nil {
		t.Errorf("the workspace still holds a conversation after the close: %s", readBody)
	}
}

// TestLeavingTheWorkspaceClosesItsConversation is AC-5 and AC-6 on the same
// mechanism: the conversation belongs to the workspace, so stopping the session
// releases the agent process nobody else could have released.
func TestLeavingTheWorkspaceClosesItsConversation(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)
	ws := srv.session()

	view := openConversationOK(t, srv)
	id := view.Conversation.ID

	ws.stop(2 * time.Second)

	if closings := provider.closings(); len(closings) != 1 || closings[0] != id {
		t.Fatalf("leaving the workspace closed %v, want exactly one %q", closings, id)
	}
	_, after, body := readConversation(t, srv, 0)
	if after.Conversation != nil {
		t.Errorf("the conversation survived the workspace it belonged to: %s", body)
	}
	// And the workspace that has been left admits no new one: nothing would be
	// left to close it. What the provider may not keep is a process nobody
	// holds, so the refusal has to release whatever it had started.
	status, _, openBody := openConversation(t, srv)
	if status != http.StatusConflict {
		t.Fatalf("POST conversation on a stopped session = %d, want 409: %s", status, openBody)
	}
	openings := provider.openings()
	closings := provider.closings()
	if len(closings) != len(openings) {
		t.Errorf("the provider was opened %d times and closed %d: a refused open left a process nobody holds", len(openings), len(closings))
	}
	if _, open := srv.session().conversation.current(); open {
		t.Error("a stopped session installed a conversation nothing could close")
	}
}

// TestRefusedConversationCommandsLeaveTheViewUnchanged: every refusal is free
// of consequences, and the projection is the evidence — byte for byte.
func TestRefusedConversationCommandsLeaveTheViewUnchanged(t *testing.T) {
	provider := newConversingProvider("chatty", 0)
	srv := newConversationServer(t, provider)

	view := openConversationOK(t, srv)
	provider.emit(t, view.Conversation.ID, "text", "hello")

	_, _, before := readConversation(t, srv, 0)

	refusals := []struct {
		name   string
		do     func() int
		expect int
	}{
		{"a second open", func() int { status, _, _ := openConversation(t, srv); return status }, http.StatusConflict},
		{"an empty message", func() int {
			return doJSON(t, srv, http.MethodPost, "/api/workspace/conversation/messages", map[string]any{"message": ""}).Code
		}, http.StatusBadRequest},
		{"an invalid cursor", func() int {
			return doJSON(t, srv, http.MethodGet, "/api/workspace/conversation?after_id=-3", nil).Code
		}, http.StatusBadRequest},
	}
	for _, refusal := range refusals {
		if got := refusal.do(); got != refusal.expect {
			t.Fatalf("%s = %d, want %d", refusal.name, got, refusal.expect)
		}
		_, _, after := readConversation(t, srv, 0)
		if after != before {
			t.Errorf("%s changed the projection:\nbefore: %s\nafter:  %s", refusal.name, before, after)
		}
	}
}

// probingProvider counts how many times its runtime is probed. The probe is
// what `--version` costs in production: a real subprocess, forked every time
// somebody asks whether the runtime is usable.
type probingProvider struct {
	*conversingProvider

	mu     sync.Mutex
	probes int
}

func (p *probingProvider) Available(context.Context, map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probes++
	return nil
}

func (p *probingProvider) probeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.probes
}

var _ execution.AvailabilityReporter = (*probingProvider)(nil)

// TestReadingAnOpenConversationProbesNothing guards the reading loop: the panel
// polls this route every couple of seconds for as long as the conversation is
// alive, and a probe on that path forks a process per tick, forever.
//
// With nothing open the probe is the answer to the only question that is being
// asked — can one be opened here — so it happens; with one already open there
// is no such question, and the projection is rendered from the conversation the
// workspace is holding.
func TestReadingAnOpenConversationProbesNothing(t *testing.T) {
	provider := &probingProvider{conversingProvider: newConversingProvider("chatty", 0)}
	srv := newConversationServer(t, provider)

	// Nothing open: the verdict decides whether the button is offered, so it is
	// honestly probed.
	if _, _, body := readConversation(t, srv, 0); provider.probeCount() == 0 {
		t.Fatalf("reading with no conversation open probed nothing: the verdict was not computed: %s", body)
	}

	view := openConversationOK(t, srv)
	provider.emit(t, view.Conversation.ID, "text", "hello")
	// The open is a user gesture and stays honest.
	if provider.probeCount() < 2 {
		t.Fatalf("opening a conversation probed %d times, want the open to probe", provider.probeCount())
	}
	before := provider.probeCount()

	for tick := 0; tick < 5; tick++ {
		status, polled, body := readConversation(t, srv, 0)
		if status != http.StatusOK {
			t.Fatalf("GET conversation = %d, want 200: %s", status, body)
		}
		if polled.Conversation == nil || polled.Conversation.State != string(execution.RunActive) {
			t.Fatalf("the poll lost the open conversation: %s", body)
		}
		if !polled.Available {
			t.Fatalf("available = false while a conversation is open: %s", body)
		}
		if len(polled.Events) != 1 || polled.Events[0].Text != "hello" {
			t.Fatalf("the poll lost the history: %s", body)
		}
	}
	if got := provider.probeCount(); got != before {
		t.Errorf("following an open conversation probed the runtime %d extra times: the reading loop forks a process per tick", got-before)
	}
}

// conversationRunsProvider is the conversing provider that also owns the runs
// the conversation starts.
//
// It is a second double and not a widening of conversingProvider because the
// two answer different questions: conversingProvider is the agent process
// behind the conversation, while this one is the hub behind the executions that
// conversation asked for. Only the provider side is a double — the store, the
// followers, the routes and the JSON serialization are the production ones.
//
// streams counts, per run, how many times the event stream was really opened.
// Without it "the block carries no events" could equally mean "this run was
// never followed" or "the stream delivered nothing", and only the first of the
// two is the assertion a terminal run is worth making.
type conversationRunsProvider struct {
	*conversingProvider

	runsMu sync.Mutex
	// runs maps an execution id to the run behind it, events maps a run id to
	// what its stream delivers, and approvals maps a run id to what it is
	// waiting on.
	runs      map[string]string
	events    map[string][]execution.RunEvent
	approvals map[string][]execution.PendingApproval
	streams   map[string]int
}

func newConversationRunsProvider(id string) *conversationRunsProvider {
	return &conversationRunsProvider{
		conversingProvider: newConversingProvider(id, 0),
		runs:               map[string]string{},
		events:             map[string][]execution.RunEvent{},
		approvals:          map[string][]execution.PendingApproval{},
		streams:            map[string]int{},
	}
}

func (p *conversationRunsProvider) ResolveRun(_ context.Context, exec execution.Execution, _ map[string]any) (string, error) {
	p.runsMu.Lock()
	defer p.runsMu.Unlock()
	return p.runs[exec.ID], nil
}

// ReadRun answers for two different subjects with one method, because the
// viewer really does ask one collaborator both questions: the conversation is
// read by its own id — and answered by the local session holding it — while a
// run is read by the run id the hub assigned.
func (p *conversationRunsProvider) ReadRun(ctx context.Context, req execution.RunRequest) (execution.RunSnapshot, error) {
	if strings.HasPrefix(req.RunID, conversationIDPrefix) {
		return p.conversingProvider.ReadRun(ctx, req)
	}
	return execution.RunSnapshot{RunID: req.RunID, State: execution.RunActive}, nil
}

func (p *conversationRunsProvider) ReadRunApprovals(ctx context.Context, req execution.RunRequest) ([]execution.PendingApproval, error) {
	if strings.HasPrefix(req.RunID, conversationIDPrefix) {
		return p.conversingProvider.ReadRunApprovals(ctx, req)
	}
	p.runsMu.Lock()
	defer p.runsMu.Unlock()
	pending := p.approvals[req.RunID]
	out := make([]execution.PendingApproval, len(pending))
	copy(out, pending)
	return out, nil
}

// StreamRunEvents delivers the configured backlog and then stays open until the
// follower is closed: a stream that returned would only make the follower
// declare the run over and reconnect in a loop.
func (p *conversationRunsProvider) StreamRunEvents(ctx context.Context, req execution.RunRequest, afterID int64, sink func(execution.RunEvent) error) error {
	if strings.HasPrefix(req.RunID, conversationIDPrefix) {
		return p.conversingProvider.StreamRunEvents(ctx, req, afterID, sink)
	}
	p.runsMu.Lock()
	p.streams[req.RunID]++
	pending := append([]execution.RunEvent(nil), p.events[req.RunID]...)
	p.runsMu.Unlock()
	for _, event := range pending {
		if event.ID <= afterID {
			continue
		}
		if err := sink(event); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (p *conversationRunsProvider) setRun(executionID, runID string) {
	p.runsMu.Lock()
	defer p.runsMu.Unlock()
	p.runs[executionID] = runID
}

func (p *conversationRunsProvider) setRunEvents(runID string, events ...execution.RunEvent) {
	p.runsMu.Lock()
	defer p.runsMu.Unlock()
	p.events[runID] = events
}

func (p *conversationRunsProvider) setRunApprovals(runID string, approvals ...execution.PendingApproval) {
	p.runsMu.Lock()
	defer p.runsMu.Unlock()
	p.approvals[runID] = approvals
}

func (p *conversationRunsProvider) streamCount(runID string) int {
	p.runsMu.Lock()
	defer p.runsMu.Unlock()
	return p.streams[runID]
}

var (
	_ execution.Provider          = (*conversationRunsProvider)(nil)
	_ execution.Conversationalist = (*conversationRunsProvider)(nil)
	_ execution.RunCollaborator   = (*conversationRunsProvider)(nil)
)

// newConversationRunsServer is a viewer whose default provider both holds the
// conversation and owns the runs it starts.
func newConversationRunsServer(t *testing.T) (*Server, *conversationRunsProvider) {
	t.Helper()
	provider := newConversationRunsProvider("chatty-runs")
	srv := newConversationServer(t, provider)
	// The followers this route attaches outlive the request that started them,
	// and their streams stay open until they are cancelled.
	t.Cleanup(func() { srv.session().followers.closeAll() })
	return srv, provider
}

// seedConversationRun writes one execution record straight into the store, the
// way a confirmed proposal would have. No provider is dispatched: what the
// conversation reads is the record, and starting a real run would only add a
// scheduler to the oracle.
func seedConversationRun(t *testing.T, srv *Server, specCode string, action execution.ActionID, status execution.ExecutionStatus) string {
	t.Helper()
	id, err := execution.RandomID()
	if err != nil {
		t.Fatal(err)
	}
	capability, err := execution.RequiredCapability(action)
	if err != nil {
		t.Fatal(err)
	}
	record := execution.Execution{
		ID:         id,
		SpecCode:   specCode,
		Action:     action,
		Capability: capability,
		ProviderID: "chatty-runs",
		Status:     status,
		Result:     &execution.Result{ExternalID: "task-" + id},
		CreatedAt:  time.Now().UTC(),
	}
	if status != execution.StatusRunning {
		completed := time.Now().UTC()
		record.CompletedAt = &completed
	}
	if err := srv.session().store.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	return id
}

// confirmProposal records in the holder the decision a person took on the
// proposal carried by proposalID. It is the register the routes write when a
// proposal is confirmed, seeded here directly so the test owns the execution id
// and its status instead of inheriting them from a scheduler.
func confirmProposal(t *testing.T, srv *Server, proposalID int64, executionID string, action execution.ActionID, specCode string) {
	t.Helper()
	scope := string(execution.ScopeWorkspace)
	if specCode != "" {
		scope = string(execution.ScopeSpec)
	}
	if err := srv.session().conversation.decide(proposalID, conversationOutcome{
		ProposalID:  proposalID,
		Decision:    "confirmed",
		Action:      string(action),
		Label:       "Pianificare",
		Scope:       scope,
		SpecCode:    specCode,
		ExecutionID: executionID,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestConversationViewCarriesTheRunsItAsked is AC-1 and AC-2 on one payload: the
// run a conversation started travels inside that conversation, anchored to the
// event that proposed it, and its events arrive there too — nobody has to open
// a separate list to follow it.
func TestConversationViewCarriesTheRunsItAsked(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	proposal := provider.emit(t, view.Conversation.ID, "text", "posso pianificare US-901?")
	executionID := seedConversationRun(t, srv, "US-901", execution.ActionPlan, execution.StatusRunning)
	provider.setRun(executionID, "run-plan")
	provider.setRunEvents("run-plan", event(1, "planning"), event(2, "still planning"))
	confirmProposal(t, srv, proposal.ID, executionID, execution.ActionPlan, "US-901")

	var read conversationResponse
	var body string
	waitFor(t, "the run block to carry the events of its run", func() bool {
		var status int
		status, read, body = readConversation(t, srv, 0)
		return status == http.StatusOK && len(read.Runs) == 1 && len(read.Runs[0].Events) == 2
	})

	block := read.Runs[0]
	if block.ExecutionID != executionID {
		t.Errorf("execution_id = %q, want the execution the confirmation started %q: %s", block.ExecutionID, executionID, body)
	}
	if block.AnchorEventID != proposal.ID {
		t.Errorf("anchor_event_id = %d, want the event that proposed it %d: %s", block.AnchorEventID, proposal.ID, body)
	}
	if block.Decision != "confirmed" {
		t.Errorf("decision = %q, want the answer the person gave: %s", block.Decision, body)
	}
	if block.Status != string(execution.StatusRunning) {
		t.Errorf("status = %q, want %q: %s", block.Status, execution.StatusRunning, body)
	}
	if block.Run == nil || block.Run.State != string(execution.RunActive) {
		t.Fatalf("the run block carries no active run: %s", body)
	}
	if block.Events[0].Text != "planning" || block.Events[1].Text != "still planning" {
		t.Errorf("the run events did not travel in the conversation: %s", body)
	}
	if block.Notice != "" {
		t.Errorf("notice = %q on a run that was read: %s", block.Notice, body)
	}
}

// TestConversationViewCarriesOneBlockPerDecision is the other half of AC-1: two
// steps asked at two points of the discourse are two blocks, each anchored
// where it was asked for, in the order the decisions were taken.
func TestConversationViewCarriesOneBlockPerDecision(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	id := view.Conversation.ID
	firstProposal := provider.emit(t, id, "text", "posso pianificare US-901?")
	firstExecution := seedConversationRun(t, srv, "US-901", execution.ActionPlan, execution.StatusSucceeded)
	confirmProposal(t, srv, firstProposal.ID, firstExecution, execution.ActionPlan, "US-901")

	provider.emit(t, id, "text", "fatto")
	secondProposal := provider.emit(t, id, "text", "posso pianificare US-902?")
	secondExecution := seedConversationRun(t, srv, "US-902", execution.ActionPlan, execution.StatusSucceeded)
	confirmProposal(t, srv, secondProposal.ID, secondExecution, execution.ActionPlan, "US-902")

	_, read, body := readConversation(t, srv, 0)
	if len(read.Runs) != 2 {
		t.Fatalf("runs = %d, want one block per decision: %s", len(read.Runs), body)
	}
	if read.Runs[0].ExecutionID != firstExecution || read.Runs[1].ExecutionID != secondExecution {
		t.Fatalf("the blocks are not in the order the decisions were taken: %s", body)
	}
	if read.Runs[0].AnchorEventID != firstProposal.ID || read.Runs[1].AnchorEventID != secondProposal.ID {
		t.Errorf("the two blocks are not anchored to the two proposals (%d, %d): %s", firstProposal.ID, secondProposal.ID, body)
	}
	if read.Runs[0].AnchorEventID == read.Runs[1].AnchorEventID {
		t.Errorf("two steps asked at two points share one anchor: %s", body)
	}
	if read.Runs[0].SpecCode != "US-901" || read.Runs[1].SpecCode != "US-902" {
		t.Errorf("the blocks do not name the specs they are about: %s", body)
	}
}

// TestConversationViewReportsAnUnreachableRunAsANotice: a run whose record
// cannot be read is a block with a notice, never an error — the person decided
// it, and the conversation has to stay readable and keep saying so.
func TestConversationViewReportsAnUnreachableRunAsANotice(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	proposal := provider.emit(t, view.Conversation.ID, "text", "posso pianificare US-901?")
	confirmProposal(t, srv, proposal.ID, "exec-does-not-exist", execution.ActionPlan, "US-901")

	status, read, body := readConversation(t, srv, 0)
	if status != http.StatusOK {
		t.Fatalf("GET conversation = %d, want 200 despite the unreachable run: %s", status, body)
	}
	if len(read.Runs) != 1 {
		t.Fatalf("runs = %d, want the block of the decision that was taken: %s", len(read.Runs), body)
	}
	block := read.Runs[0]
	if strings.TrimSpace(block.Notice) == "" {
		t.Fatalf("the unreachable run carries no notice, so an empty block reads as a run with nothing to say: %s", body)
	}
	if block.ExecutionID != "exec-does-not-exist" || block.AnchorEventID != proposal.ID {
		t.Errorf("the block lost what the decision already knew: %#v", block)
	}
	if block.AwaitingResponse {
		t.Errorf("a run that could not even be read is reported as waiting: %#v", block)
	}
	if read.Conversation == nil || len(read.Events) != 1 {
		t.Errorf("the conversation stopped being readable because one run could not be: %s", body)
	}
}

// TestConversationViewOpensNoFollowerForATerminalRun: rendering the
// conversation must not resurrect a run that has ended. The count on the double
// is the oracle — the stream is either opened or it is not, and no internal
// state is inspected to find out.
func TestConversationViewOpensNoFollowerForATerminalRun(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	proposal := provider.emit(t, view.Conversation.ID, "text", "posso pianificare US-901?")
	executionID := seedConversationRun(t, srv, "US-901", execution.ActionPlan, execution.StatusSucceeded)
	provider.setRun(executionID, "run-done")
	provider.setRunEvents("run-done", event(1, "planning"))
	confirmProposal(t, srv, proposal.ID, executionID, execution.ActionPlan, "US-901")

	for tick := 0; tick < 3; tick++ {
		_, read, body := readConversation(t, srv, 0)
		if len(read.Runs) != 1 {
			t.Fatalf("runs = %d, want the block of the step that ran: %s", len(read.Runs), body)
		}
		block := read.Runs[0]
		if block.Status != string(execution.StatusSucceeded) {
			t.Errorf("status = %q, want %q: %s", block.Status, execution.StatusSucceeded, body)
		}
		if block.Run != nil && len(block.Events) != 0 {
			t.Errorf("a terminal run was followed and delivered events: %s", body)
		}
		if block.AwaitingResponse {
			t.Errorf("a terminal run is reported as waiting: %s", body)
		}
	}
	if got := provider.streamCount("run-done"); got != 0 {
		t.Fatalf("reading the conversation opened %d streams on a run that had already ended", got)
	}
}

// TestConversationViewMarksTheRunAwaitingAnAnswer is AC-3: the conversation says
// the run is stopped on a decision, and carries the command in clear — the very
// argument the hub is waiting on, not a summary of it.
func TestConversationViewMarksTheRunAwaitingAnAnswer(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	proposal := provider.emit(t, view.Conversation.ID, "text", "posso pianificare US-901?")
	executionID := seedConversationRun(t, srv, "US-901", execution.ActionPlan, execution.StatusRunning)
	provider.setRun(executionID, "run-waiting")
	provider.setRunApprovals("run-waiting", workspaceApproval("appr-7", "Eseguire il comando"))
	confirmProposal(t, srv, proposal.ID, executionID, execution.ActionPlan, "US-901")

	var read conversationResponse
	var body string
	waitFor(t, "the conversation to report the run waiting for an answer", func() bool {
		var status int
		status, read, body = readConversation(t, srv, 0)
		return status == http.StatusOK && len(read.Runs) == 1 && read.Runs[0].AwaitingResponse
	})

	block := read.Runs[0]
	if len(block.Approvals) != 1 {
		t.Fatalf("the waiting run carries %d approvals, want the one it is stopped on: %s", len(block.Approvals), body)
	}
	pending := block.Approvals[0]
	if pending.ID != "appr-7" || pending.Title != "Eseguire il comando" {
		t.Errorf("the decision is not named: %#v", pending)
	}
	if pending.ToolName != "Bash" {
		t.Errorf("tool_name = %q, want the tool the run is asking to use", pending.ToolName)
	}
	if !strings.Contains(string(pending.Args), `"command":"ls"`) {
		t.Errorf("args = %s, want the command in clear, neither summarized nor truncated", pending.Args)
	}
}

// TestSendConversationMessageKeepsTheConversationAvailable is AC-5 on the
// command route: a conversation is held by the provider holding it, not by
// today's default, so writing to a live conversation whose workspace default
// has since changed is answered — and answered as available, exactly as the
// reading route answers.
func TestSendConversationMessageKeepsTheConversationAvailable(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	id := view.Conversation.ID

	// The default is changed the way the Execution panel changes it: on disk,
	// while the conversation is alive.
	mute := releasedProvider("mute", nil)
	if err := srv.registry.Register(mute); err != nil {
		t.Fatal(err)
	}
	root := srv.session().cfg.ProjectRoot
	if _, err := config.UpdateDefaultProvider(root, config.DefaultProviderConfig{ID: mute.ID(), Config: map[string]any{}}); err != nil {
		t.Fatal(err)
	}

	status, accepted, body := sendConversationMessage(t, srv, "ci sei?")
	if status != http.StatusAccepted {
		t.Fatalf("POST message = %d, want 202: %s", status, body)
	}
	if !accepted.Available {
		t.Errorf("available = false on a conversation that is alive and just accepted a message: %s", body)
	}
	if accepted.Conversation == nil || accepted.Conversation.ID != id {
		t.Fatalf("the 202 lost the conversation it wrote to: %s", body)
	}
	if sent := provider.dialogueOf(t, id).messages(); len(sent) != 1 || sent[0] != "ci sei?" {
		t.Errorf("the process received %v, want exactly one \"ci sei?\"", sent)
	}

	_, read, readBody := readConversation(t, srv, 0)
	if !read.Available {
		t.Errorf("the reading route disagrees with the command route about availability: %s", readBody)
	}
}

// TestConversationViewAlwaysCarriesARunsArray: a conversation that has started
// nothing answers with an empty array and never with null, so the flow iterates
// without testing.
func TestConversationViewAlwaysCarriesARunsArray(t *testing.T) {
	srv, provider := newConversationRunsServer(t)

	view := openConversationOK(t, srv)
	provider.emit(t, view.Conversation.ID, "text", "buongiorno")

	_, read, body := readConversation(t, srv, 0)
	if len(read.Runs) != 0 {
		t.Fatalf("runs = %#v, want empty on a conversation that decided nothing", read.Runs)
	}
	if !strings.Contains(body, `"runs":[]`) {
		t.Fatalf("the empty list is not an array on the wire: %s", body)
	}
}
