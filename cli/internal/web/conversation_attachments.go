package web

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// An image attached to a message travels inline, base64 in the JSON body, and
// is stored on disk beside the conversation's record: the adapter reads it from
// there, the history shows it back from there, and the journal never holds the
// bytes. The limits are per image and per body, because a body is several
// images plus their base64 overhead.
const (
	maxAttachmentBytes  = 10 << 20
	maxMessageBodyBytes = 48 << 20
)

// attachmentMediaTypes are the image formats the native harnesses accept, each
// with the extension its stored file takes.
var attachmentMediaTypes = execution.AttachmentExtensions

// conversationAttachmentURL is where a stored image of a conversation is
// served back from.
func conversationAttachmentURL(conversationID, fileName string) string {
	return "/api/workspace/conversations/" + conversationID + "/attachments/" + fileName
}

// completeAttachmentURLs gives a URL to the attachments an adapter stored
// without one: the adapter knows the directory, the viewer knows the route.
func completeAttachmentURLs(conversationID string, attachments []execution.Attachment) {
	for i := range attachments {
		if attachments[i].URL == "" && attachments[i].ID != "" {
			attachments[i].URL = conversationAttachmentURL(conversationID, attachments[i].ID)
		}
	}
}

// attachmentFileName is the only shape a stored attachment's name takes: the
// prefix of its content hash and one of the extensions above, so the route that
// serves it back cannot be led anywhere else.
var attachmentFileName = regexp.MustCompile(`^[0-9a-f]{24}\.(png|jpg|gif|webp)$`)

// attachmentUpload is one image as the browser sends it.
type attachmentUpload struct {
	Name       string `json:"name"`
	MediaType  string `json:"media_type"`
	DataBase64 string `json:"data_base64"`
}

// requireAttachmentsCapability refuses images for a session whose provider has
// not declared it takes them: the composer offers the control on the same
// declaration, so this is the server saying the same thing to a client that
// did not look.
func requireAttachmentsCapability(ctx context.Context, snapshot conversationSnapshot) error {
	if snapshot.sessionProvider == nil {
		return iox.NewConflict("the provider of this conversation does not accept attachments", "send the message without images", nil)
	}
	discovery, err := snapshot.sessionProvider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{
		ProviderConfig: snapshot.providerConfig, WorkingDir: snapshot.workingDir, Native: &snapshot.session.Native,
	})
	if err != nil || !execution.SupportsSessionCapability(discovery.Capabilities, execution.SessionCapabilityAttachments) {
		return iox.NewConflict("the provider of this conversation does not accept attachments", "send the message without images", err)
	}
	return nil
}

// storeConversationAttachments decodes the uploaded images and writes each
// under the conversation's attachments directory, named by its content, and
// hands back the references the adapter reads and the history keeps.
func storeConversationAttachments(store *conversationlog.FileStore, conversationID string, uploads []attachmentUpload) ([]execution.Attachment, error) {
	if len(uploads) == 0 {
		return nil, nil
	}
	dir, err := store.AttachmentsDir(conversationID)
	if err != nil {
		return nil, iox.NewInvalidInput("invalid conversation id", "", err)
	}
	attachments := make([]execution.Attachment, 0, len(uploads))
	for i, upload := range uploads {
		field := fmt.Sprintf("attachments[%d]", i)
		mediaType := strings.ToLower(strings.TrimSpace(upload.MediaType))
		if _, known := attachmentMediaTypes[mediaType]; !known {
			return nil, iox.NewInvalidInput(field+": media_type must be one of image/png, image/jpeg, image/gif, image/webp", "", nil)
		}
		data, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(upload.DataBase64))
		if decodeErr != nil || len(data) == 0 {
			return nil, iox.NewInvalidInput(field+": data_base64 must be a non-empty base64 image", "", decodeErr)
		}
		if len(data) > maxAttachmentBytes {
			return nil, iox.NewInvalidInput(fmt.Sprintf("%s: an image may weigh at most %d MB", field, maxAttachmentBytes>>20), "", nil)
		}
		attachment, storeErr := execution.StoreAttachment(dir, mediaType, data)
		if storeErr != nil {
			return nil, iox.NewInternal("storing the attachment "+upload.Name, storeErr)
		}
		if name := strings.TrimSpace(upload.Name); name != "" {
			attachment.Name = name
		}
		attachment.URL = conversationAttachmentURL(conversationID, attachment.ID)
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

// handleGetWorkspaceConversationAttachment serves one stored image back. The
// name is content-addressed, so the answer may be cached for ever.
func (s *Server) handleGetWorkspaceConversationAttachment(w http.ResponseWriter, r *http.Request) {
	fileName := r.PathValue("file")
	if !attachmentFileName.MatchString(fileName) {
		writeError(w, iox.NewNotFound("no such attachment", "", nil))
		return
	}
	dir, err := s.session().conversationStore().AttachmentsDir(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, iox.NewNotFound("no such conversation", "", err))
		return
	}
	path := filepath.Join(dir, fileName)
	if _, statErr := os.Stat(path); statErr != nil {
		writeError(w, iox.NewNotFound("no such attachment", "", statErr))
		return
	}
	for mediaType, extension := range attachmentMediaTypes {
		if strings.HasSuffix(fileName, extension) {
			w.Header().Set("Content-Type", mediaType)
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

// removeConversationAttachments takes back the files of a message that was not
// delivered, so a refused send leaves nothing behind.
func removeConversationAttachments(attachments []execution.Attachment) {
	for _, attachment := range attachments {
		_ = os.Remove(attachment.Path)
	}
}

// bodyTooLarge tells a body refused for its size apart from a malformed one.
func bodyTooLarge(err error) bool {
	var tooLarge *http.MaxBytesError
	return errors.As(err, &tooLarge)
}
