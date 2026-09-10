package conversationlog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/execution"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
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

type EventPage struct {
	Events  []execution.RunEvent `json:"events"`
	LastID  int64                `json:"last_id"`
	HasMore bool                 `json:"has_more"`
}

func NewFileStore(projectRoot string) (*FileStore, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	return &FileStore{dir: filepath.Join(root, ".archetipo", "conversations")}, nil
}

func (s *FileStore) path(id string) (string, error) {
	if !recordfile.ValidID(id) {
		return "", &StoreError{Kind: StoreInvalidID, ID: id}
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *FileStore) eventsPath(id string) (string, error) {
	path, err := s.path(id)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(path, ".json") + ".events.jsonl", nil
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
	if record.Native() && len(record.Events) > 0 {
		if err := s.AppendEvents(ctx, record.ID, record.Events); err != nil {
			return err
		}
		record.Events = nil
	}
	// A nil slice would serialize as null, and every reader would then need to
	// tell null from [] for a distinction that does not exist: a conversation
	// with no events yet has an empty history, not an unknown one.
	if record.Events == nil && !record.Native() {
		record.Events = []execution.RunEvent{}
	}
	return recordfile.WriteAtomic(s.dir, path, ".conversation-*.tmp", record)
}

// Get reads one record. An absent file is a typed not-found, so the caller can
// tell "this conversation was never written" from "this store is broken".
func (s *FileStore) Get(ctx context.Context, id string) (Record, error) {
	record, err := s.GetMetadata(ctx, id)
	if err != nil {
		return Record{}, err
	}
	page, err := s.ReadEvents(ctx, id, 0, 0)
	if err != nil {
		return Record{}, err
	}
	if len(page.Events) > 0 {
		record.Events = mergeEvents(record.Events, page.Events)
	}
	if record.Events == nil {
		record.Events = []execution.RunEvent{}
	}
	return record, nil
}

// GetMetadata reads only the atomic record. Native timelines can be large, so
// list and orchestration paths use this method instead of loading the journal.
func (s *FileStore) GetMetadata(ctx context.Context, id string) (Record, error) {
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

// AppendEvents durably appends events in cursor order. Replayed events are
// ignored, while a gap or a conflicting id is refused so recovery never
// manufactures a continuous history from incomplete input.
func (s *FileStore) AppendEvents(ctx context.Context, id string, events []execution.RunEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	appender, err := s.NewEventAppender(ctx, id)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := appender.Append(event); err != nil {
			_ = appender.Close()
			return err
		}
	}
	return appender.Close()
}

// EventAppender holds the append lock and file across a provider stream. Each
// callback writes directly to the JSONL file, without rescanning and reopening
// it for every event.
type EventAppender struct {
	id       string
	last     int64
	file     *os.File
	encoder  *json.Encoder
	lock     *ConversationLock
	unsynced int
}

func (s *FileStore) NewEventAppender(ctx context.Context, id string) (*EventAppender, error) {
	appendLock, err := s.Lock(ctx, id+"-events")
	if err != nil {
		return nil, err
	}
	path, err := s.eventsPath(id)
	if err != nil {
		_ = appendLock.Unlock()
		return nil, err
	}
	if err := repairPartialEvent(path); err != nil {
		_ = appendLock.Unlock()
		return nil, err
	}
	last, err := s.lastEvent(ctx, id)
	if err != nil {
		_ = appendLock.Unlock()
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_ = appendLock.Unlock()
		return nil, err
	}
	return &EventAppender{id: id, last: last, file: file, encoder: json.NewEncoder(file), lock: appendLock}, nil
}

// repairPartialEvent removes only a final unterminated JSONL fragment. A
// complete but invalid line still fails decoding: recovery may discard bytes a
// crashed append never completed, never an event that claimed completion with
// its newline.
func repairPartialEvent(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	last := []byte{0}
	if _, err := file.ReadAt(last, info.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	const repairWindow int64 = 4 * 1024 * 1024
	start := max(int64(0), info.Size()-repairWindow)
	buffer := make([]byte, info.Size()-start)
	if _, err := file.ReadAt(buffer, start); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	newline := bytes.LastIndexByte(buffer, '\n')
	truncateAt := int64(0)
	if newline >= 0 {
		truncateAt = start + int64(newline) + 1
	}
	if err := file.Truncate(truncateAt); err != nil {
		return err
	}
	return file.Sync()
}

func (a *EventAppender) Append(event execution.RunEvent) error {
	if event.ID <= a.last {
		return nil
	}
	if a.last != 0 && event.ID != a.last+1 {
		return fmt.Errorf("append conversation %q event %d after %d: event gap", a.id, event.ID, a.last)
	}
	if err := a.encoder.Encode(event); err != nil {
		return err
	}
	a.last = event.ID
	a.unsynced++
	if a.unsynced >= 64 {
		if err := a.file.Sync(); err != nil {
			return err
		}
		a.unsynced = 0
	}
	return nil
}

// Flush makes durable what has been appended so far.
//
// The appender syncs every 64 events, which is right for a stream nobody is
// reading back; it is not right the moment somebody *is*, because a reader of
// the journal file — the summary of the record, for one — would miss what the
// appender is still holding. Flushing at the few points that are read back
// costs one sync each and keeps the file and the reader in agreement.
func (a *EventAppender) Flush() error {
	if a == nil || a.file == nil || a.unsynced == 0 {
		return nil
	}
	if err := a.file.Sync(); err != nil {
		return err
	}
	a.unsynced = 0
	return nil
}

func (a *EventAppender) LastID() int64 {
	if a == nil {
		return 0
	}
	return a.last
}

func (a *EventAppender) Close() error {
	if a == nil {
		return nil
	}
	var errs []error
	if a.file != nil {
		if a.unsynced > 0 {
			errs = append(errs, a.file.Sync())
		}
		errs = append(errs, a.file.Close())
		a.file = nil
	}
	if a.lock != nil {
		errs = append(errs, a.lock.Unlock())
		a.lock = nil
	}
	return errors.Join(errs...)
}

func (s *FileStore) lastEvent(ctx context.Context, id string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	path, err := s.eventsPath(id)
	if err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return 0, err
	}
	const tailSize int64 = 4 * 1024 * 1024
	start := max(int64(0), info.Size()-tailSize)
	buffer := make([]byte, info.Size()-start)
	if _, err := file.ReadAt(buffer, start); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	lines := bytes.Split(bytes.TrimSpace(buffer), []byte("\n"))
	if start > 0 && len(lines) > 1 {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return 0, nil
	}
	var event execution.RunEvent
	if err := json.Unmarshal(lines[len(lines)-1], &event); err != nil {
		return 0, fmt.Errorf("decode last conversation %q event: %w", id, err)
	}
	return event.ID, nil
}

// ReadEvents reads the durable timeline strictly after afterID. limit <= 0
// means all remaining events and is used only by legacy-compatible Get.
func (s *FileStore) ReadEvents(ctx context.Context, id string, afterID int64, limit int) (EventPage, error) {
	page := EventPage{Events: []execution.RunEvent{}, LastID: afterID}
	if afterID < 0 || limit < 0 {
		return page, fmt.Errorf("after id and limit must not be negative")
	}
	path, err := s.eventsPath(id)
	if err != nil {
		return page, err
	}
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return page, nil
	}
	if err != nil {
		return page, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return page, err
		}
		var event execution.RunEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return page, fmt.Errorf("decode conversation %q event: %w", id, err)
		}
		if event.ID <= afterID {
			continue
		}
		if limit > 0 && len(page.Events) == limit {
			page.HasMore = true
			break
		}
		page.Events = append(page.Events, event)
		page.LastID = event.ID
	}
	if err := scanner.Err(); err != nil {
		return page, err
	}
	return page, nil
}

func mergeEvents(first, second []execution.RunEvent) []execution.RunEvent {
	merged := append([]execution.RunEvent(nil), first...)
	last := int64(0)
	if len(merged) > 0 {
		last = merged[len(merged)-1].ID
	}
	for _, event := range second {
		if event.ID > last {
			merged = append(merged, event)
			last = event.ID
		}
	}
	return merged
}

// Delete removes one record from the workspace, for good. An absent file is a
// typed not-found for the same reason Get's is: erasing a conversation nobody
// ever wrote is a mistake, and answering "done" would hide it from the caller
// who has to tell the person that the thread they pressed is no longer there.
func (s *FileStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &StoreError{Kind: StoreNotFound, ID: id, Err: err}
		}
		return err
	}
	if eventsPath, eventsErr := s.eventsPath(id); eventsErr == nil {
		if eventsErr = os.Remove(eventsPath); eventsErr != nil && !errors.Is(eventsErr, fs.ErrNotExist) {
			return eventsErr
		}
	}
	return nil
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

// ConversationLock serializes one conversation across tabs and View
// processes. Its directory is removed on release; a dead owner is detected by
// pid, so a crash cannot leave the session permanently locked.
type ConversationLock struct{ path string }

// ErrLockHeld says a live process already holds the lock. Only TryLock returns
// it: Lock waits instead, which is what every writer of a record wants. A
// caller that must not wait — one deciding whether it, and not another View,
// owns a native runtime — asks with TryLock and treats this as "somebody else
// owns it", never as a failure.
var ErrLockHeld = errors.New("the conversation lock is held by a live process")

// Lock waits until the lock is free. TryLock makes a single attempt and answers
// ErrLockHeld when a live owner holds it. Both recover a lock whose owner
// process is gone.
func (s *FileStore) Lock(ctx context.Context, id string) (*ConversationLock, error) {
	return s.lock(ctx, id, true)
}

func (s *FileStore) TryLock(ctx context.Context, id string) (*ConversationLock, error) {
	return s.lock(ctx, id, false)
}

func (s *FileStore) lock(ctx context.Context, id string, wait bool) (*ConversationLock, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	lockPath := strings.TrimSuffix(path, ".json") + ".lock"
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return nil, err
		}
		candidate, err := os.MkdirTemp(s.dir, ".conversation-lock-*")
		if err != nil {
			return nil, err
		}
		owner := []byte(strconv.Itoa(os.Getpid()) + "\n" + time.Now().UTC().Format(time.RFC3339Nano))
		if writeErr := os.WriteFile(filepath.Join(candidate, "owner"), owner, 0o600); writeErr != nil {
			_ = os.RemoveAll(candidate)
			return nil, writeErr
		}
		err = os.Rename(candidate, lockPath)
		if err == nil {
			return &ConversationLock{path: lockPath}, nil
		}
		_ = os.RemoveAll(candidate)
		if _, statErr := os.Stat(lockPath); statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return nil, err
		}
		if _, statErr := os.Stat(lockPath); errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		owner, readErr := os.ReadFile(filepath.Join(lockPath, "owner"))
		fields := strings.Fields(string(owner))
		pid, parseErr := strconv.Atoi(first(fields))
		if readErr != nil || parseErr != nil || !processAlive(pid) {
			_ = os.RemoveAll(lockPath)
			continue
		}
		if !wait {
			return nil, ErrLockHeld
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (l *ConversationLock) Unlock() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.RemoveAll(l.path)
	l.path = ""
	return err
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
