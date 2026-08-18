package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileStore struct{ dir string }

func NewFileStore(projectRoot string) (*FileStore, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	return &FileStore{dir: filepath.Join(root, ".archetipo", "executions")}, nil
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}

func (s *FileStore) path(id string) (string, error) {
	if !validID(id) {
		return "", &StoreError{Kind: StoreInvalidID, ID: id}
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func encode(execution Execution) ([]byte, error) {
	body, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func (s *FileStore) Create(ctx context.Context, execution Execution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.path(execution.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return err
	}
	body, err := encode(execution)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return &StoreError{Kind: StoreAlreadyExist, ID: execution.ID, Err: err}
	}
	if err != nil {
		return err
	}
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return nil
}

func (s *FileStore) Update(ctx context.Context, execution Execution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.path(execution.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return &StoreError{Kind: StoreNotFound, ID: execution.ID, Err: err}
	} else if err != nil {
		return err
	}
	body, err := encode(execution)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".execution-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err = tmp.Write(body); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ListBySpec reads the whole record directory and keeps the records of one
// spec. There is no index and no cache on purpose: the number of local records
// is small, and a cache would add a state to invalidate for a gain nobody can
// measure.
func (s *FileStore) ListBySpec(ctx context.Context, specCode string) ([]Execution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []Execution{}
	code := strings.TrimSpace(specCode)
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read execution record %q: %w", entry.Name(), err)
		}
		var record Execution
		// An unreadable record fails the whole read instead of being skipped: a
		// truncated history is indistinguishable from an absent one, and the
		// caller would draw the wrong conclusion from it in silence.
		if err := json.Unmarshal(body, &record); err != nil {
			return nil, fmt.Errorf("decode execution record %q: %w", entry.Name(), err)
		}
		if record.SpecCode != code {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *FileStore) Get(ctx context.Context, id string) (Execution, error) {
	if err := ctx.Err(); err != nil {
		return Execution{}, err
	}
	path, err := s.path(id)
	if err != nil {
		return Execution{}, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Execution{}, &StoreError{Kind: StoreNotFound, ID: id, Err: err}
	}
	if err != nil {
		return Execution{}, err
	}
	var execution Execution
	if err := json.Unmarshal(body, &execution); err != nil {
		return Execution{}, fmt.Errorf("decode execution %q: %w", id, err)
	}
	return execution, nil
}
