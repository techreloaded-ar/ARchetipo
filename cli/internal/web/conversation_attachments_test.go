package web

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
