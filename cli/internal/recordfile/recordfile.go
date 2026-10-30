package recordfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func ValidID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}

func WriteAtomic(dir, path, prefix string, record any) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	temporary, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(body); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return retryFileOperation(func() error { return os.Rename(temporaryPath, path) })
}

// ReadFile and Remove share WriteAtomic's bounded handling of transient Windows
// sharing violations. Persistent permissions/errors remain failures.
func ReadFile(path string) (body []byte, err error) {
	err = retryFileOperation(func() error {
		body, err = os.ReadFile(path)
		return err
	})
	return body, err
}

func Remove(path string) error {
	return retryFileOperation(func() error { return os.Remove(path) })
}
