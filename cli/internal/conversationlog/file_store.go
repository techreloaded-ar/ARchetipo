package conversationlog

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

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
)

// StoreErrorKind names why a store operation could not be carried out. It is a
// closed set so a caller branches on the kind, never on the text of an error.
type StoreErrorKind string

const (
	StoreNotFound  StoreErrorKind = "not_found"
	StoreInvalidID StoreErrorKind = "invalid_id"
)

type StoreError struct {
	Kind StoreErrorKind
	ID   string
	Err  error
}

func (e *StoreError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("conversation store %s for %q: %v", e.Kind, e.ID, e.Err)
	}
	return fmt.Sprintf("conversation store %s for %q", e.Kind, e.ID)
}

func (e *StoreError) Unwrap() error { return e.Err }

// FileStore keeps one conversation record per file under
// <project_root>/.archetipo/conversations. The directory of the open workspace
// is the whole scope of the store: there is no global list to filter, so there
// is no way to filter it wrong.
type FileStore struct{ dir string }

func NewFileStore(projectRoot string) (*FileStore, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	return &FileStore{dir: filepath.Join(root, ".archetipo", "conversations")}, nil
}

// validID rejects anything that could make an id escape the store directory:
// the id is a file name, and only a file name.
func validID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\`)
}

func (s *FileStore) path(id string) (string, error) {
	if !validID(id) {
		return "", &StoreError{Kind: StoreInvalidID, ID: id}
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func encode(record Record) ([]byte, error) {
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

// Save writes a record, creating it or replacing it in place. It is an upsert
// rather than a create/update pair because the journal rewrites the same record
// on every round that brought new events, and a caller that had to know whether
// the file already existed would be keeping a second copy of the truth.
//
// The write is atomic — a temporary file in the same directory, then a rename —
// so a reader never sees a half-written history, and the temporary file is
// removed on every path so a failed write leaves no residue behind.
func (s *FileStore) Save(ctx context.Context, record Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.path(record.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return err
	}
	// A nil slice would serialize as null, and every reader would then need to
	// tell null from [] for a distinction that does not exist: a conversation
	// with no events yet has an empty history, not an unknown one.
	if record.Events == nil {
		record.Events = []execution.RunEvent{}
	}
	body, err := encode(record)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".conversation-*.tmp")
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

// Get reads one record. An absent file is a typed not-found, so the caller can
// tell "this conversation was never written" from "this store is broken".
func (s *FileStore) Get(ctx context.Context, id string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	path, err := s.path(id)
	if err != nil {
		return Record{}, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, &StoreError{Kind: StoreNotFound, ID: id, Err: err}
	}
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return Record{}, fmt.Errorf("decode conversation %q: %w", id, err)
	}
	return record, nil
}

// List returns every record of the workspace, most recent first. There is no
// index and no cache on purpose: the number of local records is small, and an
// index would add a state to invalidate for a gain nobody can measure.
//
// Entries that are not records — nested directories and non-.json files — are
// ignored, and an absent directory reads as no conversations at all: a workspace
// nobody has talked to yet is an answer, not a failure. A record that cannot be
// read or decoded fails the whole scan naming the file, because a history
// silently missing one conversation is indistinguishable from a complete one.
func (s *FileStore) List(ctx context.Context) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []Record{}
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
			return nil, fmt.Errorf("read conversation record %q: %w", entry.Name(), err)
		}
		var record Record
		if err := json.Unmarshal(body, &record); err != nil {
			return nil, fmt.Errorf("decode conversation record %q: %w", entry.Name(), err)
		}
		out = append(out, record)
	}
	sortByRecency(out)
	return out, nil
}

// sortByRecency orders records most recent first by the instant of their last
// message, with the ID breaking ties so two records written in the same instant
// still come out in a stable order.
func sortByRecency(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		if !records[i].LastMessageAt.Equal(records[j].LastMessageAt) {
			return records[i].LastMessageAt.After(records[j].LastMessageAt)
		}
		return records[i].ID > records[j].ID
	})
}
