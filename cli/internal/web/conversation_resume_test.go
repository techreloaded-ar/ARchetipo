package web

import (
	"context"
	"encoding/json"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution/localrun"
	"net/http"
	"strings"
	"testing"
)

// --- resuming a past conversation -------------------------------------------
//
// A resume opens a *new* conversation that has been handed the old one as
// context. The oracle of every assertion below is what the provider actually
// received — the ConversationRequest and its Context — and never a mocked call
// that happened to succeed: the promise of AC-4 is that the past conversation
// reaches the agent, and only the request the provider was given can say so.

// resumeTestResponse is the wire shape the resume route answers with. It is
// written out here rather than reusing conversationResponse because the two
// fields this route exists for — the spec it inherited and the conversation it
// took up — are exactly the ones that struct does not carry.
type resumeTestResponse struct {
	Conversation *struct {
		ID          string `json:"id"`
		SpecCode    string `json:"spec_code"`
		ResumedFrom string `json:"resumed_from"`
	} `json:"conversation"`
	Notice string `json:"notice"`
}

func resumeTestDecode(t *testing.T, body string) resumeTestResponse {
	t.Helper()
	var view resumeTestResponse
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("undecodable resume view: %v (%s)", err, body)
	}
	if view.Conversation == nil {
		t.Fatalf("the resumed conversation is null: %s", body)
	}
	return view
}

// resumeTestPost asks for the resume of a conversation, returning the raw
// answer so a refusal is asserted on the very body the browser would read.
func resumeTestPost(t *testing.T, srv *Server, id, message string) (int, string) {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversations/"+id+"/resume", map[string]any{"message": message})
	return w.Code, w.Body.String()
}

func resumeTestResume(t *testing.T, srv *Server, id, message string) resumeTestResponse {
	t.Helper()
	status, body := resumeTestPost(t, srv, id, message)
	if status != http.StatusCreated {
		t.Fatalf("POST resume = %d, want 201: %s", status, body)
	}
	return resumeTestDecode(t, body)
}

// resumeTestOpenWithSpec opens a conversation bound to a spec, which the shared
// helper deliberately does not do: the ordinary conversation is free, and the
// binding is the exception a caller has to state.
func resumeTestOpenWithSpec(t *testing.T, srv *Server, specCode string) string {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/workspace/conversation", map[string]any{"spec_code": specCode})
	if w.Code != http.StatusCreated {
		t.Fatalf("POST conversation (spec %s) = %d, want 201: %s", specCode, w.Code, w.Body.String())
	}
	return resumeTestDecode(t, w.Body.String()).Conversation.ID
}

// resumeTestRecord reads a conversation back from the disk of the workspace,
// which is where a conversation that has ended lives.
func resumeTestRecord(t *testing.T, srv *Server, id string) conversationlog.Record {
	t.Helper()
	store := srv.session().conversationStore()
	if store == nil {
		t.Fatal("the workspace keeps no conversation journal")
	}
	record, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("reading the conversation %q from disk: %v", id, err)
	}
	return record
}

// resumeTestPastConversation holds a conversation, makes it say something, and
// closes it — the ordinary way a past conversation comes into existence.
func resumeTestPastConversation(t *testing.T, srv *Server, provider *conversingProvider, said ...string) string {
	t.Helper()
	id := openConversationOK(t, srv).Conversation.ID
	for i, text := range said {
		kind := localrun.KindUserMessage
		if i%2 == 1 {
			kind = localrun.KindText
		}
		provider.emit(t, id, kind, text)
	}
	if status, _, body := closeConversation(t, srv); status != http.StatusOK {
		t.Fatalf("DELETE conversation = %d, want 200: %s", status, body)
	}
	return id
}

func resumeTestOpeningOf(t *testing.T, provider *conversingProvider, id string) execution.ConversationRequest {
	t.Helper()
	for _, opening := range provider.openings() {
		if opening.ConversationID == id {
			return opening
		}
	}
	t.Fatalf("the provider was never asked to open the conversation %q", id)
	return execution.ConversationRequest{}
}

// --- transcriptOf -----------------------------------------------------------

// The transcript is the exchange and nothing around it: what the person said
// and what the agent answered, in the order it happened. A tool call is part of
// the timeline a reader scrolls and not of the conversation an agent is handed.
func TestResumeTranscriptKeepsOnlyWhatWasSaid(t *testing.T) {
	record := conversationlog.Record{Events: []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "come sta la spec US-058?"},
		{ID: 2, Kind: localrun.KindToolStart, Text: "grep -rn conversationlog"},
		{ID: 3, Kind: localrun.KindThinking, Text: "sto ragionando"},
		{ID: 4, Kind: localrun.KindText, Text: "è ferma sulla ripresa"},
		{ID: 5, Kind: localrun.KindTurnEnd, Text: ""},
	}}

	got := transcriptOf(record)

	want := conversationHumanPrefix + "come sta la spec US-058?\n" + conversationAgentPrefix + "è ferma sulla ripresa"
	if got != want {
		t.Fatalf("transcript =\n%s\nwant\n%s", got, want)
	}
	for _, dropped := range []string{"grep -rn conversationlog", "sto ragionando"} {
		if strings.Contains(got, dropped) {
			t.Errorf("the transcript carries %q, which is machinery and not conversation:\n%s", dropped, got)
		}
	}
}

// One event is one turn: a message written over several lines must not read as
// several turns, and an event that said nothing must not become an empty one.
func TestResumeTranscriptRendersOneLinePerTurn(t *testing.T) {
	record := conversationlog.Record{Events: []execution.RunEvent{
		{ID: 1, Kind: localrun.KindUserMessage, Text: "prima riga\n\nseconda riga"},
		{ID: 2, Kind: localrun.KindText, Text: "   \n\t "},
		{ID: 3, Kind: localrun.KindText, Text: "ok"},
	}}

	lines := strings.Split(transcriptOf(record), "\n")

	if len(lines) != 2 {
		t.Fatalf("transcript has %d lines, want 2:\n%q", len(lines), lines)
	}
	if lines[0] != conversationHumanPrefix+"prima riga seconda riga" {
		t.Errorf("the first turn was not collapsed into one line: %q", lines[0])
	}
	if lines[1] != conversationAgentPrefix+"ok" {
		t.Errorf("second line = %q, want the only other thing that was said", lines[1])
	}
}

// A conversation with nothing said in it renders as nothing at all, so the
// prompt of the new conversation is the ordinary one rather than an empty fence.
func TestResumeTranscriptOfASilentConversationIsEmpty(t *testing.T) {
	if got := transcriptOf(conversationlog.Record{}); got != "" {
		t.Fatalf("transcript of a silent conversation = %q, want empty", got)
	}
}

// Too long a history is cut from the *beginning*: a conversation is resumed for
// what it ended on, and the cut is announced so a partial history is read as a
// partial one.
func TestResumeTranscriptCutsTheOldestPartAndSaysSo(t *testing.T) {
	const oldest = "la primissima cosa detta in questa conversazione"
	const newest = "l'ultima cosa detta prima di chiudere"
	events := []execution.RunEvent{{ID: 1, Kind: localrun.KindUserMessage, Text: oldest}}
	// Enough filler to push the head of the transcript past the ceiling.
	for i := 0; i < 400; i++ {
		events = append(events, execution.RunEvent{ID: int64(i + 2), Kind: localrun.KindText, Text: strings.Repeat("riempitivo ", 10)})
	}
	events = append(events, execution.RunEvent{ID: 999, Kind: localrun.KindUserMessage, Text: newest})

	got := transcriptOf(conversationlog.Record{Events: events})

	if !strings.HasPrefix(got, conversationContextOmissionNotice+"\n") {
		t.Fatalf("a truncated transcript does not open on the omission notice: %.120q", got)
	}
	if strings.Contains(got, oldest) {
		t.Error("the truncation kept the oldest turn, so it did not cut from the beginning")
	}
	if !strings.Contains(got, newest) {
		t.Error("the truncation dropped the newest turn, which is the part being resumed")
	}
	kept := strings.TrimPrefix(got, conversationContextOmissionNotice+"\n")
	if runes := len([]rune(kept)); runes != conversationContextLimit {
		t.Fatalf("the kept transcript is %d runes, want the ceiling of %d", runes, conversationContextLimit)
	}
}

// --- the route --------------------------------------------------------------

// AC-4: writing in a past conversation answers in a *new* one that declares
// what it took up, and the past conversation really reaches the agent — the
// oracle is the Context the provider was handed, not a call that returned nil.
func TestResumeOpensANewConversationCarryingThePastOneAsContext(t *testing.T) {
	const sentinel = "avevamo deciso di rinviare la rotta di ripresa a dopo il journaling"
	provider := newConversingProvider("resumable", 0)
	srv := newConversationServer(t, provider)

	past := resumeTestPastConversation(t, srv, provider, sentinel, "d'accordo, la rinviamo")

	view := resumeTestResume(t, srv, past, "riprendiamo da lì")

	if view.Conversation.ID == past {
		t.Fatalf("the resume reopened the same conversation %q instead of opening a new one", past)
	}
	if view.Conversation.ResumedFrom != past {
		t.Errorf("resumed_from = %q, want %q", view.Conversation.ResumedFrom, past)
	}
	opening := resumeTestOpeningOf(t, provider, view.Conversation.ID)
	if !strings.Contains(opening.Context, sentinel) {
		t.Fatalf("the provider did not receive the past conversation as context: %q", opening.Context)
	}
	if !strings.Contains(opening.Context, conversationHumanPrefix+sentinel) {
		t.Errorf("the context does not say who spoke: %q", opening.Context)
	}
	// The message that asked for the resume is delivered to the conversation it
	// asked for, and to no other.
	if sent := provider.dialogueOf(t, view.Conversation.ID).messages(); len(sent) != 1 || sent[0] != "riprendiamo da lì" {
		t.Fatalf("the new conversation received %q, want the message that asked for the resume", sent)
	}
	// The new conversation is journalled as a resume, so a reload finds the
	// link and not just a conversation that appeared from nowhere.
	if record := resumeTestRecord(t, srv, view.Conversation.ID); record.ResumedFrom != past {
		t.Errorf("the record of the new conversation has resumed_from %q, want %q", record.ResumedFrom, past)
	}
}

// A workspace holds one conversation at a time, so resuming a past one closes
// the one that is live — and the one being left behind is sealed with the state
// it ended in rather than dropped.
func TestResumeClosesTheLiveConversationFirst(t *testing.T) {
	provider := newConversingProvider("resumable", 0)
	srv := newConversationServer(t, provider)

	past := resumeTestPastConversation(t, srv, provider, "una vecchia conversazione")
	live := openConversationOK(t, srv).Conversation.ID
	provider.emit(t, live, localrun.KindUserMessage, "qualcosa detto in quella viva")

	view := resumeTestResume(t, srv, past, "riprendiamo la vecchia")

	closed := provider.closings()
	if len(closed) == 0 || closed[len(closed)-1] != live {
		t.Fatalf("the provider closed %q, want the live conversation %q released", closed, live)
	}
	record := resumeTestRecord(t, srv, live)
	if record.FinalState == "" {
		t.Error("the conversation left behind was not sealed with a final state")
	}
	// Sealed with everything said in it, not with the history the last read had
	// happened to see.
	if record.MessageCount != 1 {
		t.Errorf("the sealed record counts %d messages, want the one that was said", record.MessageCount)
	}
	if view.Conversation.ID == live {
		t.Fatalf("the resume answered with the conversation it had just closed")
	}
}

// A conversation taken up is about whatever the thread was about: the spec
// travels to the new conversation, so a resume cannot silently make a thread
// change subject.
func TestResumeInheritsTheSpecOfTheConversationItTakesUp(t *testing.T) {
	provider := newConversingProvider("resumable", 0)
	srv := newConversationServer(t, provider)

	past := resumeTestOpenWithSpec(t, srv, "US-901")
	provider.emit(t, past, localrun.KindUserMessage, "parliamo di US-901")
	if status, _, body := closeConversation(t, srv); status != http.StatusOK {
		t.Fatalf("DELETE conversation = %d, want 200: %s", status, body)
	}

	view := resumeTestResume(t, srv, past, "riprendiamo US-901")

	if view.Conversation.SpecCode != "US-901" {
		t.Errorf("spec_code = %q, want the spec of the conversation being resumed", view.Conversation.SpecCode)
	}
	if record := resumeTestRecord(t, srv, view.Conversation.ID); record.SpecCode != "US-901" {
		t.Errorf("the record of the new conversation has spec_code %q, want %q", record.SpecCode, "US-901")
	}
}

// The three refusals, each with its own status and its own sentence: a
// conversation that does not exist, a resume with nothing written in it, and
// the conversation that is happening right now.
func TestResumeRefusesWhatCannotBeResumed(t *testing.T) {
	t.Run("an id this workspace never held", func(t *testing.T) {
		provider := newConversingProvider("resumable", 0)
		srv := newConversationServer(t, provider)

		status, body := resumeTestPost(t, srv, "conv-inesistente", "ci sei?")

		if status != http.StatusNotFound {
			t.Fatalf("POST resume = %d, want 404: %s", status, body)
		}
		if message := refusalMessage(t, body); !strings.Contains(message, "conv-inesistente") {
			t.Errorf("the refusal does not name the conversation asked for: %q", message)
		}
		if openings := provider.openings(); len(openings) != 0 {
			t.Fatalf("a refused resume still opened %d conversations", len(openings))
		}
	})

	t.Run("a resume with nothing written in it", func(t *testing.T) {
		provider := newConversingProvider("resumable", 0)
		srv := newConversationServer(t, provider)
		past := resumeTestPastConversation(t, srv, provider, "qualcosa")
		before := len(provider.openings())

		status, body := resumeTestPost(t, srv, past, "   ")

		if status != http.StatusBadRequest {
			t.Fatalf("POST resume = %d, want 400: %s", status, body)
		}
		if message := refusalMessage(t, body); !strings.Contains(message, "message") {
			t.Errorf("the refusal does not name the missing message: %q", message)
		}
		if openings := provider.openings(); len(openings) != before {
			t.Fatalf("a resume with no message still opened a conversation")
		}
	})

	t.Run("the conversation that is happening right now", func(t *testing.T) {
		provider := newConversingProvider("resumable", 0)
		srv := newConversationServer(t, provider)
		live := openConversationOK(t, srv).Conversation.ID

		status, body := resumeTestPost(t, srv, live, "riprendiamo questa")

		if status != http.StatusConflict {
			t.Fatalf("POST resume = %d, want 409: %s", status, body)
		}
		message := refusalMessage(t, body)
		if !strings.Contains(message, live) {
			t.Errorf("the refusal does not name the conversation: %q", message)
		}
		if !strings.Contains(strings.ToLower(message), "currently open") {
			t.Errorf("the refusal does not say why it refused: %q", message)
		}
		// And it is still open: a refused resume releases nothing.
		if closings := provider.closings(); len(closings) != 0 {
			t.Fatalf("a refused resume closed %q", closings)
		}
	})
}

// A conversation that resumes nothing hands the provider no context at all, so
// the ordinary conversation is untouched by the existence of the resume.
func TestOpeningAConversationCarriesNoResumedContext(t *testing.T) {
	provider := newConversingProvider("resumable", 0)
	srv := newConversationServer(t, provider)

	id := openConversationOK(t, srv).Conversation.ID

	if opening := resumeTestOpeningOf(t, provider, id); opening.Context != "" {
		t.Fatalf("a conversation that resumes nothing was given a context: %q", opening.Context)
	}
	if record := resumeTestRecord(t, srv, id); record.ResumedFrom != "" {
		t.Fatalf("a conversation that resumes nothing was recorded as resumed from %q", record.ResumedFrom)
	}
}
