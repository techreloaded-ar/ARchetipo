package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
)

// Refusal codes for the registry. They are part of the contract in the same
// way the initialization ones are: the viewer renders them and the smoke tests
// assert on them, so they never change silently.
const (
	CodeRegistryPathRequired      = "WORKSPACE_REGISTRY_PATH_REQUIRED"
	CodeRegistryPathNotAbsolute   = "WORKSPACE_REGISTRY_PATH_NOT_ABSOLUTE"
	CodeRegistryPathMissing       = "WORKSPACE_REGISTRY_PATH_MISSING"
	CodeRegistryPathNotADirectory = "WORKSPACE_REGISTRY_PATH_NOT_A_DIRECTORY"
	CodeRegistryNotAWorkspace     = "WORKSPACE_REGISTRY_NOT_A_WORKSPACE"
)

// ErrEntryNotFound is returned when an id is not in the registry, so the HTTP
// layer can tell "nothing to remove" apart from "the removal failed".
var ErrEntryNotFound = errors.New("workspace registry entry not found")

// Status is how reachable a known workspace is right now. It is computed on
// every read and never stored: storing it would be storing something that
// changes while nobody is looking, and it is what makes AC-3 true by
// construction — the entry stays in the list whatever the disk says, and only
// the label beside it changes.
type Status string

const (
	StatusReachable    Status = "reachable"
	StatusMissing      Status = "missing"
	StatusNotDirectory Status = "not_a_directory"
	StatusNotReadable  Status = "not_readable"
	StatusNotWorkspace Status = "not_a_workspace"
)

// Reachable reduces the status to the boolean the common case asks for.
func (s Status) Reachable() bool { return s == StatusReachable }

// EnvStateDir overrides the user-level state directory that holds the registry
// of known workspaces. Tests and the e2e smokes set it so they never touch the
// real state of the machine that runs them.
const EnvStateDir = "ARCHETIPO_STATE_DIR"

const (
	registryFileName = "workspaces.json"
	registrySchema   = 1
)

// Entry is one known workspace. Name is stored at registration time rather
// than recomputed from Path on every read: a directory that has been renamed
// away must still be recognizable as "the one that was called this".
type Entry struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
}

// registryFile is the persisted document. The schema number exists because a
// format without a version can only be guessed at, and the tolerant reader
// below would silence any future incompatibility instead of reporting it.
type registryFile struct {
	Schema     int     `json:"schema"`
	Workspaces []Entry `json:"workspaces"`
}

// StateDir resolves the directory that holds user-level ARchetipo state. It is
// deliberately not the cache directory: a cache is what the system may delete
// without harm, and the list of workspaces a person knows about cannot be
// regenerated from any other source.
//
//	macOS   ~/Library/Application Support/archetipo/
//	Linux   $XDG_CONFIG_HOME/archetipo/  (or ~/.config/archetipo/)
//	Windows %AppData%\archetipo\
func StateDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvStateDir)); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", iox.NewPrecondition("could not locate the ARchetipo state directory", "set "+EnvStateDir+" to a writable directory", err)
	}
	return filepath.Join(base, "archetipo"), nil
}

// Registry is the persistent list of known workspaces, rooted at an explicit
// directory. It is a type with a directory rather than a handful of functions
// over the global path so that tests can build one on a temporary directory
// without reaching for environment variables.
type Registry struct{ Dir string }

// OpenRegistry resolves the state directory. It deliberately does not create
// it: the directory appears on the first write, so merely starting a viewer
// never leaves anything behind.
func OpenRegistry() (*Registry, error) {
	dir, err := StateDir()
	if err != nil {
		return nil, err
	}
	return &Registry{Dir: dir}, nil
}

// Path is the registry file inside the state directory.
func (r *Registry) Path() string { return filepath.Join(r.Dir, registryFileName) }

// load reads the persisted entries and never fails. A missing, unreadable,
// non-JSON or unknown-schema file all mean the same thing to a caller: there
// is nothing known yet. Refusing to work because of it is exactly what AC-5
// forbids, and the next write repairs the file from scratch.
func (r *Registry) load() []Entry {
	b, err := os.ReadFile(r.Path())
	if err != nil {
		return nil
	}
	var doc registryFile
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil
	}
	if doc.Schema != registrySchema {
		return nil
	}
	return doc.Workspaces
}

// List returns the known workspaces, most recently opened first. Reading the
// list is not an access and does not touch LastOpenedAt: otherwise opening the
// list would reorder the list.
func (r *Registry) List() ([]Entry, error) {
	if r.Dir == "" {
		return nil, iox.NewInternal("workspace registry has no directory", nil)
	}
	entries := r.load()
	if len(entries) == 0 {
		return nil, nil
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].LastOpenedAt.Equal(entries[j].LastOpenedAt) {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].LastOpenedAt.After(entries[j].LastOpenedAt)
	})
	return entries, nil
}

// save rewrites the whole document atomically. The temporary file lives in the
// same directory as the final one, otherwise os.Rename could cross a
// filesystem boundary and fail; the rename is what makes a half-written
// registry impossible.
func (r *Registry) save(entries []Entry) error {
	if r.Dir == "" {
		return iox.NewInternal("workspace registry has no directory", nil)
	}
	if entries == nil {
		entries = []Entry{}
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	b, err := json.MarshalIndent(registryFile{Schema: registrySchema, Workspaces: entries}, "", "  ")
	if err != nil {
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	tmp, err := os.CreateTemp(r.Dir, "workspaces-*.json")
	if err != nil {
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	if err := os.Rename(name, r.Path()); err != nil {
		_ = os.Remove(name)
		return iox.NewInternal("cannot write the workspace registry", err)
	}
	return nil
}

// EntryID derives the identity of a workspace from its path, because the path
// is what a workspace is. Deriving it instead of generating one makes
// registration idempotent, and the hexadecimal form is what lets the id travel
// in a URL segment where an encoded path would not.
func EntryID(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])[:16]
}

// Probe asks the disk about one entry. `not_a_workspace` is finer than the
// letter of the story, but it is what a person actually hits when someone
// deletes `.archetipo/` instead of the whole directory: telling it apart makes
// the case diagnosable without changing the verdict, which stays "unreachable".
func Probe(e Entry) Status {
	info, err := os.Stat(e.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return StatusMissing
	case err != nil:
		return StatusNotReadable
	case !info.IsDir():
		return StatusNotDirectory
	}

	f, err := os.Open(e.Path)
	if err != nil {
		return StatusNotReadable
	}
	_, readErr := f.ReadDir(1)
	_ = f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return StatusNotReadable
	}

	if _, err := os.Stat(filepath.Join(e.Path, config.RelativePath)); err != nil {
		return StatusNotWorkspace
	}
	return StatusReachable
}

// Touch records path as opened now, creating the entry if it is new. It does
// not validate the directory on purpose: whoever calls it already holds a real
// workspace — Initialize has just finished creating it, or the viewer has just
// started serving it — and refusing to record a workspace that demonstrably
// exists would be absurd.
func (r *Registry) Touch(path string) (Entry, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		abs, err := filepath.Abs(clean)
		if err != nil {
			return Entry{}, iox.NewInvalidInput("cannot resolve the workspace path", "provide an absolute path", err)
		}
		clean = abs
	}

	id := EntryID(clean)
	entries := r.load()
	now := time.Now().UTC()

	for i := range entries {
		if entries[i].ID == id {
			entries[i].LastOpenedAt = now
			if err := r.save(entries); err != nil {
				return Entry{}, err
			}
			return entries[i], nil
		}
	}

	entry := Entry{ID: id, Name: filepath.Base(clean), Path: clean, LastOpenedAt: now}
	entries = append(entries, entry)
	if err := r.save(entries); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Add records a workspace the person named in a text field, which is why it
// validates where Touch does not. Registering the same path twice updates the
// existing entry instead of creating a second one, because the identity is
// derived from the path.
func (r *Registry) Add(path string) (Entry, error) {
	trimmed := strings.TrimSpace(path)
	invalid := &ValidationError{}

	if trimmed == "" {
		invalid.add("path", CodeRegistryPathRequired, "the workspace path is required")
		return Entry{}, invalid
	}
	if !filepath.IsAbs(trimmed) {
		invalid.add("path", CodeRegistryPathNotAbsolute, "indicate an absolute path: a relative one would be resolved against the viewer's directory, not yours")
		return Entry{}, invalid
	}

	clean := filepath.Clean(trimmed)
	info, err := os.Stat(clean)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		invalid.add("path", CodeRegistryPathMissing, "the directory does not exist")
		return Entry{}, invalid
	case err != nil:
		invalid.add("path", CodeRegistryPathMissing, "the path is not readable: "+err.Error())
		return Entry{}, invalid
	case !info.IsDir():
		invalid.add("path", CodeRegistryPathNotADirectory, "the path exists and is a file, not a directory")
		return Entry{}, invalid
	}

	if _, err := os.Stat(filepath.Join(clean, config.RelativePath)); err != nil {
		invalid.add("path", CodeRegistryNotAWorkspace, "the directory is not an ARchetipo workspace: "+config.RelativePath+" is missing")
		return Entry{}, invalid
	}

	return r.Touch(clean)
}

// Get returns one entry by id, so the HTTP layer can answer 404 distinguishably.
func (r *Registry) Get(id string) (Entry, error) {
	for _, e := range r.load() {
		if e.ID == id {
			return e, nil
		}
	}
	return Entry{}, ErrEntryNotFound
}

// Remove drops one entry from the list. It rewrites the registry file and
// nothing else: there is deliberately no os.Remove or os.RemoveAll anywhere in
// this function pointing at the workspace path, which is what makes forgetting
// a workspace different from deleting it (AC-4).
func (r *Registry) Remove(id string) error {
	entries := r.load()
	remaining := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.ID == id {
			continue
		}
		remaining = append(remaining, e)
	}
	if len(remaining) == len(entries) {
		return ErrEntryNotFound
	}
	return r.save(remaining)
}
