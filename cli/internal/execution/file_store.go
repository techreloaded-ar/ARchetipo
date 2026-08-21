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
//
// An empty specCode is not a wildcard: it selects the executions whose object
// is the workspace rather than a spec, which are stored with an empty
// spec_code. That is the read the viewer uses to list workspace-scoped runs.
func (s *FileStore) ListBySpec(ctx context.Context, specCode string) ([]Execution, error) {
	records, err := s.readAll(ctx)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(specCode)
	out := []Execution{}
	for _, record := range records {
		if record.SpecCode != code {
			continue
		}
		out = append(out, record)
	}
	sortByRecency(out)
	return out, nil
}

// List reads the same record directory as ListBySpec and keeps everything: the
// caller, not the store, decides which of those records counts as "in progress".
func (s *FileStore) List(ctx context.Context) ([]Execution, error) {
	out, err := s.readAll(ctx)
	if err != nil {
		return nil, err
	}
	sortByRecency(out)
	return out, nil
}

// readAll is the single place that scans the record directory. Files that are
// not records — nested directories and non-.json entries — are ignored, an
// absent directory reads as no records at all, and a record that cannot be read
// or decoded fails the whole scan naming the file.
func (s *FileStore) readAll(ctx context.Context) ([]Execution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []Execution{}
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
		out = append(out, record)
	}
	return out, nil
}

// sortByRecency orders records most recent first, with the ID breaking ties so
// two records written in the same instant still come out in a stable order.
func sortByRecency(records []Execution) {
	sort.Slice(records, func(i, j int) bool {
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		}
		return records[i].ID > records[j].ID
	})
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
