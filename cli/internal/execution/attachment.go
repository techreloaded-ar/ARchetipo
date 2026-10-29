package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AttachmentExtensions are the image formats a conversation may carry, each
// with the extension its stored file takes. The viewer accepts uploads in these
// formats only, and an adapter stores what the agent read in the same ones.
var AttachmentExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
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

// StoreAttachment writes one image under dir, named by its content, and hands
// back the attachment that names the file. The URL is left empty: where the
// file is reached from is the viewer's to say, not the writer's. An empty dir
// stores nothing and says so, because a session that was not given a place
// for images has nowhere to keep them.
func StoreAttachment(dir, mediaType string, data []byte) (Attachment, error) {
	if strings.TrimSpace(dir) == "" {
		return Attachment{}, fmt.Errorf("no attachments directory")
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	extension, known := AttachmentExtensions[mediaType]
	if !known {
		return Attachment{}, fmt.Errorf("unsupported image media type %q", mediaType)
	}
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("empty image")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Attachment{}, fmt.Errorf("creating the attachments directory: %w", err)
	}
	sum := sha256.Sum256(data)
	fileName := hex.EncodeToString(sum[:12]) + extension
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Attachment{}, fmt.Errorf("storing the image: %w", err)
	}
	return Attachment{ID: fileName, Name: fileName, MediaType: mediaType, Path: path}, nil
}
