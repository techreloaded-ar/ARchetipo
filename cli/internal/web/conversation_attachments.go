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

// A file attached to a message travels inline, base64 in the JSON body, and
// is stored on disk beside the conversation's record: the adapter reads it from
// there, the history shows it back from there, and the journal never holds the
// bytes. An image reaches the agent as an image block of the native frame;
// any other file reaches it as a path to read, named by a note appended to
// the words that are sent — never to the words the history shows. The limits
// are per file and per body, because a body is several files plus their
// base64 overhead.
const (
	maxAttachmentBytes  = 10 << 20
	maxMessageBodyBytes = 48 << 20
)

// attachmentImageMediaTypes are the image formats served back as images, each
// with the extension its stored file takes; every other stored file is served
// as a download.
var attachmentImageMediaTypes = execution.AttachmentExtensions

// attachmentMediaTypeShape is the only shape an uploaded media type takes.
var attachmentMediaTypeShape = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`)

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
// prefix of its content hash and a plain extension, so the route that serves
// it back cannot be led anywhere else.
var attachmentFileName = regexp.MustCompile(`^[0-9a-f]{24}\.[a-z0-9]{1,16}$`)

// attachmentUpload is one file as the browser sends it.
type attachmentUpload struct {
	Name       string `json:"name"`
	MediaType  string `json:"media_type"`
	DataBase64 string `json:"data_base64"`
}

// requireAttachmentsCapability refuses files for a session whose provider has
// not declared it takes them: the composer offers the control on the same
// declaration, so this is the server saying the same thing to a client that
// did not look.
func requireAttachmentsCapability(ctx context.Context, snapshot conversationSnapshot) error {
	if snapshot.sessionProvider == nil {
		return iox.NewConflict("the provider of this conversation does not accept attachments", "send the message without attachments", nil)
	}
	discovery, err := snapshot.sessionProvider.DiscoverSession(ctx, execution.SessionDiscoveryRequest{
		ProviderConfig: snapshot.providerConfig, WorkingDir: snapshot.workingDir, Native: &snapshot.session.Native,
	})
	if err != nil || !execution.SupportsSessionCapability(discovery.Capabilities, execution.SessionCapabilityAttachments) {
		return iox.NewConflict("the provider of this conversation does not accept attachments", "send the message without attachments", err)
	}
	return nil
}

// storeConversationAttachments decodes the uploaded files and writes each
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
		if mediaType == "" {
			mediaType = execution.AttachmentMediaTypeBinary
		}
		if !attachmentMediaTypeShape.MatchString(mediaType) {
			return nil, iox.NewInvalidInput(field+": media_type must be a media type such as text/plain", "", nil)
		}
		data, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(upload.DataBase64))
		if decodeErr != nil || len(data) == 0 {
			return nil, iox.NewInvalidInput(field+": data_base64 must be a non-empty base64 file", "", decodeErr)
		}
		if len(data) > maxAttachmentBytes {
			return nil, iox.NewInvalidInput(fmt.Sprintf("%s: a file may weigh at most %d MB", field, maxAttachmentBytes>>20), "", nil)
		}
		attachment, storeErr := execution.StoreAttachment(dir, filepath.Base(strings.TrimSpace(upload.Name)), mediaType, data)
		if storeErr != nil {
			return nil, iox.NewInternal("storing the attachment "+upload.Name, storeErr)
		}
		attachment.URL = conversationAttachmentURL(conversationID, attachment.ID)
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

// attachedFilesNote is what the agent is told about the attachments it does
// not see as images: their names and the paths it reads them from. Empty where
// every attachment is an image.
func attachedFilesNote(attachments []execution.Attachment) string {
	var lines []string
	for _, attachment := range attachments {
		if attachment.IsImage() {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", attachment.Name, attachment.MediaType, attachment.Path))
	}
	if len(lines) == 0 {
		return ""
	}
	return "The person attached these files to the message. Read each one from its path before answering:\n" + strings.Join(lines, "\n")
}

// attachmentNames is what the history shows for a message that had no words,
// only files.
func attachmentNames(attachments []execution.Attachment) string {
	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		names = append(names, attachment.Name)
	}
	return strings.Join(names, ", ")
}

// handleGetWorkspaceConversationAttachment serves one stored file back: an
// image as itself, anything else as a download, so a stored page is never
// rendered under the viewer's origin. The name is content-addressed, so the
// answer may be cached for ever.
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
	w.Header().Set("Content-Type", execution.AttachmentMediaTypeBinary)
	w.Header().Set("Content-Disposition", "attachment")
	for mediaType, extension := range attachmentImageMediaTypes {
		if strings.HasSuffix(fileName, extension) {
			w.Header().Set("Content-Type", mediaType)
			w.Header().Del("Content-Disposition")
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
