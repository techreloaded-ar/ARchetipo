package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// An archived native conversation is over, and saying so is the whole of it:
// the workspace can still open another one. Before, its payload left
// `available` unset, and the panel read the zero value as a workspace refusing
// conversations — "non disponibile" with no reason to show.
func TestArchivedNativeConversationKeepsTheWorkspaceOpenable(t *testing.T) {
	provider := newPersistentFakeNativeProvider("native-fake", nil)
	srv, _, _ := newRunServer(t, provider, true)
	id := openConversationOK(t, srv).Conversation.ID

	archived := doJSON(t, srv, http.MethodPost, "/api/workspace/conversations/"+id+"/archive", nil)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive: status %d body %s", archived.Code, archived.Body.String())
	}
	if got := conversationRecord(t, srv.session(), id).Archive; got != execution.ArchiveArchived {
		t.Fatalf("the conversation was not archived: archive=%s", got)
	}
	read := doJSON(t, srv, http.MethodGet, "/api/workspace/conversations/"+id, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("read: status %d body %s", read.Code, read.Body.String())
	}
	for name, body := range map[string][]byte{"archive": archived.Body.Bytes(), "read": read.Body.Bytes()} {
		var view conversationView
		if err := json.Unmarshal(body, &view); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !view.Available || view.UnavailableReason != "" {
			t.Fatalf("%s: available=%v reason=%q, want an openable workspace", name, view.Available, view.UnavailableReason)
		}
		if view.Conversation == nil || view.Conversation.State != execution.RunClosed {
			t.Fatalf("%s: the archived conversation does not read as closed: %#v", name, view.Conversation)
		}
	}
}
