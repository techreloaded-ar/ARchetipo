package increment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

func (s *Service) dir() string {
	return filepath.Join(s.Config.ProjectRoot, ".archetipo", "increments")
}
func (s *Service) path(id string) (string, error) {
	if !recordfile.ValidID(id) || !strings.HasPrefix(id, "inc-") {
		return "", iox.NewInvalidInput("invalid increment id", "use the id returned by increment create", nil)
	}
	return filepath.Join(s.dir(), id+".json"), nil
}
func (s *Service) Read(id string) (Record, error) {
	path, err := s.path(id)
	if err != nil {
		return Record{}, err
	}
	localPath := filepath.Join(s.dir(), "worktrees", id, ".archetipo", "increment-local", "increment.json")
	raw, err := os.ReadFile(localPath)
	if errors.Is(err, os.ErrNotExist) {
		// Existing increments migrate on their next mutation, without a write
		// during reads or touching the parent from a sandboxed agent.
		raw, err = os.ReadFile(path)
	}
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, iox.NewPrecondition("increment not found", "use an increment of this workspace", err)
	}
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err = json.Unmarshal(raw, &record); err != nil {
		return Record{}, err
	}
	if record.ID != id || record.Workdir != filepath.Join(s.dir(), "worktrees", id) || record.Branch != "archetipo/increments/"+id {
		return Record{}, iox.NewConflict("invalid increment metadata", "restore the increment record before continuing", nil)
	}
	return record, nil
}
func (s *Service) save(r Record) error {
	if _, err := s.path(r.ID); err != nil {
		return err
	}
	dir := filepath.Join(s.dir(), "worktrees", r.ID, ".archetipo", "increment-local")
	return recordfile.WriteAtomic(dir, filepath.Join(dir, "increment.json"), ".increment-", r)
}
func (s *Service) List() ([]Record, error) {
	entries, err := os.ReadDir(s.dir())
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := []Record{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || !strings.HasPrefix(entry.Name(), "inc-") {
			continue
		}
		r, readErr := s.Read(strings.TrimSuffix(entry.Name(), ".json"))
		if readErr != nil {
			return nil, readErr
		}
		records = append(records, r)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	return records, nil
}
func (s *Service) locked(ctx context.Context, fn func() (Record, error)) (Record, error) {
	// Reuse the existing crash-recoverable workspace lock. This separate id is
	// never a conversation record and serializes code allocation/publication too.
	store, err := conversationlog.NewFileStore(s.Config.ProjectRoot)
	if err != nil {
		return Record{}, err
	}
	lock, err := store.Lock(ctx, "increment-mutations")
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}

// lockedRecord keeps the evolving intent, results and its lock in the sandbox.
// Publication additionally takes the workspace lock to allocate official codes.
func (s *Service) lockedRecord(ctx context.Context, id string, fn func() (Record, error)) (Record, error) {
	r, err := s.Read(id)
	if err != nil {
		return r, err
	}
	store, err := conversationlog.NewFileStore(r.Workdir)
	if err != nil {
		return r, err
	}
	lock, err := store.Lock(ctx, "increment-mutations")
	if err != nil {
		return r, err
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}

func (s *Service) lockedWorkspaceRecord(ctx context.Context, id string, fn func() (Record, error)) (Record, error) {
	return s.locked(ctx, func() (Record, error) { return s.lockedRecord(ctx, id, fn) })
}

func (s *Service) Bind(ctx context.Context, id, conversationID string) (Record, error) {
	return s.lockedRecord(ctx, id, func() (Record, error) {
		r, err := s.Read(id)
		if err != nil {
			return r, err
		}
		if r.ConversationID != "" && r.ConversationID != conversationID {
			return r, iox.NewConflict("increment already has a conversation", "resume its existing native session", nil)
		}
		r.ConversationID = conversationID
		return r, s.save(r)
	})
}
