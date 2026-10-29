package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// An image attached to a message is refused where the provider has not declared
// it takes images, and otherwise is stored beside the conversation, handed to
// the turn by reference and served back by the route the reference names.
func TestConversationAttachmentsAreStoredHandedToTheTurnAndServedBack(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	httpServer := httptest.NewServer(srv.mux)
	defer httpServer.Close()

	status, body := nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("open = %d: %s", status, body)
	}
	id := decodeConversation(t, string(body)).Conversation.ID
	image := map[string]any{"name": "schermata.png", "media_type": "image/png", "data_base64": base64.StdEncoding.EncodeToString([]byte("PNGBYTES"))}

	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "guarda", "attachments": []any{image}})
	if status != http.StatusConflict || !strings.Contains(string(body), "does not accept attachments") {
		t.Fatalf("without the capability = %d: %s", status, body)
	}
	if len(provider.turnStarts) != 0 {
		t.Fatalf("a refused message opened a turn: %#v", provider.turnStarts)
	}

	provider.acceptsAttachments = true
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"attachments": []any{image}})
	if status != http.StatusAccepted {
		t.Fatalf("image without words = %d: %s", status, body)
	}
	start := provider.lastStart
	if start.Message != "" || len(start.Attachments) != 1 {
		t.Fatalf("turn = %#v", start)
	}
	attachment := start.Attachments[0]
	dir, _ := srv.session().conversationStore().AttachmentsDir(id)
	if attachment.Name != "schermata.png" || attachment.MediaType != "image/png" || filepath.Dir(attachment.Path) != dir || !strings.HasSuffix(attachment.ID, ".png") || attachment.URL != conversationsRoute+"/"+id+"/attachments/"+attachment.ID {
		t.Fatalf("attachment = %#v (dir %s)", attachment, dir)
	}
	if stored, err := os.ReadFile(attachment.Path); err != nil || string(stored) != "PNGBYTES" {
		t.Fatalf("stored bytes = %q, %v", stored, err)
	}

	response, err := http.Get(httpServer.URL + attachment.URL)
	if err != nil {
		t.Fatal(err)
	}
	served := make([]byte, 16)
	n, _ := response.Body.Read(served)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" || string(served[:n]) != "PNGBYTES" {
		t.Fatalf("served = %d %s %q", response.StatusCode, response.Header.Get("Content-Type"), served[:n])
	}

	for _, path := range []string{"/attachments/../" + attachment.ID, "/attachments/" + strings.TrimSuffix(attachment.ID, ".png") + ".svg", "/attachments/ffffffffffffffffffffffff.png"} {
		status, _ = nativeHTTPRequest(t, httpServer.URL, http.MethodGet, conversationsRoute+"/"+id+path, nil)
		if status != http.StatusNotFound {
			t.Fatalf("%s = %d; want 404", path, status)
		}
	}

	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "x", "attachments": []any{map[string]any{"name": "a.svg", "media_type": "image/svg+xml", "data_base64": "QUJD"}}})
	if status != http.StatusBadRequest || !strings.Contains(string(body), "media_type") {
		t.Fatalf("unknown media type = %d: %s", status, body)
	}
	status, body = nativeHTTPRequest(t, httpServer.URL, http.MethodPost, conversationsRoute+"/"+id+"/messages", map[string]any{"message": "x", "attachments": []any{map[string]any{"name": "a.png", "media_type": "image/png", "data_base64": "not base64!"}}})
	if status != http.StatusBadRequest || !strings.Contains(string(body), "data_base64") {
		t.Fatalf("broken base64 = %d: %s", status, body)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("refused uploads left files behind: %d entries", len(entries))
	}
}

// The images attached to the words that start an action travel with that
// start, on the spec-scoped and on the workspace-scoped route alike: stored
// beside the conversation the action runs in and handed to its first turn by
// reference, exactly as the message route does. Where the provider takes no
// images the start is refused with the same sentence, and no turn is opened.
func TestAttachmentsTravelWithTheWordsThatStartAnAction(t *testing.T) {
	srv, provider, _ := newActionSessionServer(t)
	provider.capabilities = append(provider.capabilities, execution.CapabilityWorkspaceSpecDraft)
	image := map[string]any{"name": "schermata.png", "media_type": "image/png", "data_base64": base64.StdEncoding.EncodeToString([]byte("PNGBYTES"))}

	w := doJSON(t, srv, http.MethodPost, "/api/spec/US-901/execution", map[string]any{"action": "plan", "attachments": []any{image}})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "does not accept attachments") {
		t.Fatalf("without the capability = %d: %s", w.Code, w.Body.String())
	}
	if len(provider.turnStarts) != 0 {
		t.Fatalf("a refused start opened a turn: %#v", provider.turnStarts)
	}

	provider.acceptsAttachments = true
	w = doJSON(t, srv, http.MethodPost, "/api/spec/US-901/execution", map[string]any{"action": "plan", "attachments": []any{image}})
	if w.Code != http.StatusCreated {
		t.Fatalf("spec start with an image = %d: %s", w.Code, w.Body.String())
	}
	start := provider.lastStart
	thread := conversationOfExecution(t, srv, decodeExecutionID(t, w.Body.String()))
	dir, _ := srv.session().conversationStore().AttachmentsDir(thread)
	if len(start.Attachments) != 1 || start.Attachments[0].Name != "schermata.png" || filepath.Dir(start.Attachments[0].Path) != dir {
		t.Fatalf("the spec start lost the attachment: %#v (dir %s)", start.Attachments, dir)
	}
	if stored, err := os.ReadFile(start.Attachments[0].Path); err != nil || string(stored) != "PNGBYTES" {
		t.Fatalf("stored bytes = %q, %v", stored, err)
	}

	w = doJSON(t, srv, http.MethodPost, "/api/workspace/execution", map[string]any{"action": "spec-draft", "user_input": "una storia", "attachments": []any{image}})
	if w.Code != http.StatusCreated {
		t.Fatalf("workspace start with an image = %d: %s", w.Code, w.Body.String())
	}
	start = provider.lastStart
	thread = conversationOfExecution(t, srv, decodeExecutionID(t, w.Body.String()))
	dir, _ = srv.session().conversationStore().AttachmentsDir(thread)
	if len(start.Attachments) != 1 || filepath.Dir(start.Attachments[0].Path) != dir || !strings.Contains(start.Message, "una storia") {
		t.Fatalf("the workspace start lost the attachment or the words: %#v (dir %s) %q", start.Attachments, dir, start.Message)
	}
}

func decodeExecutionID(t *testing.T, body string) string {
	t.Helper()
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil || decoded.ID == "" {
		t.Fatalf("no execution id in %s", body)
	}
	return decoded.ID
}
