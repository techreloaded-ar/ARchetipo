package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AttachmentExtensions are the image formats the native harnesses show the
// agent as images, each with the extension its stored file takes. Any other
// file is carried as well, but by reference: the agent reads it from disk.
var AttachmentExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// AttachmentMediaTypeBinary is the media type of a file whose type the browser
// did not know.
const AttachmentMediaTypeBinary = "application/octet-stream"

// IsImage says whether the native harness receives this attachment as an image
// block; every other attachment reaches the agent as a path to read.
func (a Attachment) IsImage() bool {
	_, image := AttachmentExtensions[strings.ToLower(a.MediaType)]
	return image
}

// storedExtension is the extension a stored file takes: the one its image
// format dictates, else the one of the name it was given, reduced to letters
// and digits, else .bin. The route that serves the file back accepts only
// this shape.
var storedExtensionShape = regexp.MustCompile(`^\.[a-z0-9]{1,16}$`)

func storedExtension(name, mediaType string) string {
	if extension, image := AttachmentExtensions[mediaType]; image {
		return extension
	}
	if extension := strings.ToLower(filepath.Ext(name)); storedExtensionShape.MatchString(extension) {
		return extension
	}
	return ".bin"
}

// MediaTypeOfPath is the media type a file's extension declares, among the
// formats a conversation carries, or empty for anything else.
func MediaTypeOfPath(path string) string {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".jpeg" {
		extension = ".jpg"
	}
	for mediaType, known := range AttachmentExtensions {
		if extension == known {
			return mediaType
		}
	}
	return ""
}

// StoreAttachment writes one file under dir, named by its content, and hands
// back the attachment that names the file: by name when one was given, by its
// stored file name otherwise. The URL is left empty: where the file is reached
// from is the viewer's to say, not the writer's. An empty dir stores nothing
// and says so, because a session that was not given a place for attachments
// has nowhere to keep them; an empty media type is refused for the same
// reason an adapter gives it, a file it could not tell the type of.
func StoreAttachment(dir, name, mediaType string, data []byte) (Attachment, error) {
	if strings.TrimSpace(dir) == "" {
		return Attachment{}, fmt.Errorf("no attachments directory")
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return Attachment{}, fmt.Errorf("attachment without a media type")
	}
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("empty attachment")
	}
	extension := storedExtension(name, mediaType)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Attachment{}, fmt.Errorf("creating the attachments directory: %w", err)
	}
	sum := sha256.Sum256(data)
	fileName := hex.EncodeToString(sum[:12]) + extension
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Attachment{}, fmt.Errorf("storing the attachment: %w", err)
	}
	if name = strings.TrimSpace(name); name == "" {
		name = fileName
	}
	return Attachment{ID: fileName, Name: name, MediaType: mediaType, Path: path}, nil
}
