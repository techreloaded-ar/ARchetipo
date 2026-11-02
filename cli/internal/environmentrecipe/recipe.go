// Package environmentrecipe distributes one explicit workspace recipe, including
// its declared helper files. Runtime state, credentials and installed dependencies
// never travel between checkouts.
package environmentrecipe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/conversationlog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

const Path = ".archetipo/environment-recipe.json"
const Environment = ".archetipo/environment.json"
const Preview = ".archetipo/preview.json"

type File struct {
	Path       string `json:"path"`
	Content    []byte `json:"content"`
	Executable bool   `json:"executable,omitempty"`
}
type Recipe struct {
	Version int    `json:"version"`
	Files   []File `json:"files"`
}

// Workspace resolves the original checkout for both increments and ordinary Git
// worktrees. Git, rather than branch names or directory heuristics, owns this link.
func Workspace(root string) string {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	raw, err := cmd.Output()
	if err != nil {
		return root
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		return root
	}
	top, topErr := filepath.EvalSymlinks(lines[0])
	current, currentErr := filepath.EvalSymlinks(root)
	// An initialized workspace nested inside another repository is not owned by
	// that outer repository. Only an actual checkout root follows Git's link.
	if topErr != nil || currentErr != nil || top != current {
		return root
	}
	if filepath.Base(lines[1]) == ".git" {
		return filepath.Dir(lines[1])
	}
	return root
}

func safe(root, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, `\:`) || filepath.ToSlash(filepath.Clean(rel)) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("unsafe environment recipe asset: %s", rel)
	}
	if rel != Environment && rel != Preview && !strings.HasPrefix(rel, "scripts/archetipo/") {
		return "", fmt.Errorf("environment helpers must live under scripts/archetipo/: %s", rel)
	}
	current := root
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("environment recipe asset traverses symlink: %s", rel)
		}
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

func Load(root string) (Recipe, error) {
	var r Recipe
	raw, err := os.ReadFile(filepath.Join(Workspace(root), Path))
	if err != nil {
		return r, err
	}
	if len(raw) > 4<<20 {
		return r, fmt.Errorf("workspace environment recipe exceeds 4 MiB")
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if r.Version != 1 {
		return r, fmt.Errorf("unsupported workspace environment recipe version")
	}
	seen := map[string]bool{}
	for _, f := range r.Files {
		if _, err = safe(root, f.Path); err != nil {
			return r, err
		}
		if seen[f.Path] {
			return r, fmt.Errorf("duplicate environment asset: %s", f.Path)
		}
		seen[f.Path] = true
	}
	if !seen[Environment] {
		return r, fmt.Errorf("workspace recipe has no environment configuration")
	}
	return r, validate(r)
}

// Publish explicitly names every non-secret helper; there is no directory scan.
// One atomic bundle is the authoritative revision, including before Git merge.
func Publish(root string, assets []string) (Recipe, error) { return publish(root, assets, "") }
func PublishQualified(root string, assets []string, digest string) (Recipe, error) {
	return publish(root, assets, digest)
}
func publish(root string, assets []string, digest string) (Recipe, error) {
	lease, err := writeLease(root)
	if err != nil {
		return Recipe{}, err
	}
	defer func() { _ = lease.Unlock() }()
	r, err := Collect(root, assets)
	if err != nil {
		return r, err
	}
	if digest != "" && Digest(r) != digest {
		return r, iox.NewConflict("candidate bundle changed after qualification", "qualify the exact recipe and assets again", nil)
	}
	dir := filepath.Join(Workspace(root), ".archetipo")
	raw, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	if len(raw) > 4<<20 {
		return r, fmt.Errorf("workspace environment recipe exceeds 4 MiB")
	}
	return r, recordfile.WriteAtomic(dir, filepath.Join(Workspace(root), Path), ".environment-recipe-", r)
}

// Sync materializes the common recipe in the execution directory. It never copies
// checkpoints or uses another checkout's services. Unknown helper changes are
// refused rather than overwritten; a previous inherited helper can be updated.
func Sync(root string) error {
	return sync(root, false)
}

// Inherit installs the shared bundle in a newly created checkout. Its unchanged
// versioned helpers may come from an older main commit; edited or untracked
// helpers remain protected exactly as during ordinary synchronization.
func Inherit(root string) error {
	return sync(root, true)
}

func sync(root string, freshCheckout bool) error {
	r, err := Load(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	previous := map[string][]byte{}
	checkpoint := filepath.Join(root, ".archetipo/environment/inherited-recipe.json")
	if raw, readErr := os.ReadFile(checkpoint); readErr == nil {
		var old Recipe
		if json.Unmarshal(raw, &old) == nil {
			for _, f := range old.Files {
				previous[f.Path] = f.Content
			}
		}
	}
	for _, f := range r.Files {
		path, pathErr := safe(root, f.Path)
		if pathErr != nil {
			return pathErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		if bytes.Equal(raw, f.Content) {
			continue
		}
		if readErr == nil && f.Path != Environment && f.Path != Preview && !bytes.Equal(raw, previous[f.Path]) {
			if !freshCheckout || !unchangedVersionedHelper(root, f.Path) {
				return iox.NewConflict("environment helper differs from the workspace recipe: "+f.Path, "repair and publish the common workspace configuration before preparing", nil)
			}
		}
	}
	for _, f := range r.Files {
		path, _ := safe(root, f.Path)
		raw, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Equal(raw, f.Content) {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Executable {
			mode = 0o755
		}
		// Rename a complete sibling file; readers never observe partial scripts/JSON.
		temp, createErr := os.CreateTemp(filepath.Dir(path), ".environment-asset-")
		if createErr != nil {
			return createErr
		}
		name := temp.Name()
		if _, err = temp.Write(f.Content); err == nil {
			err = temp.Chmod(mode)
		}
		closeErr := temp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(name, path)
		}
		_ = os.Remove(name)
		if err != nil {
			return err
		}
	}
	return recordfile.WriteAtomic(filepath.Dir(checkpoint), checkpoint, ".inherited-", r)
}

func unchangedVersionedHelper(root, path string) bool {
	if err := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", path).Run(); err != nil {
		return false
	}
	return exec.Command("git", "-C", root, "diff", "--quiet", "HEAD", "--", path).Run() == nil
}

// Read resolves the sole common configuration without materializing files during
// a read-only status request. Legacy projects retain their local recipe until
// their first explicit publication.
func Read(root, path string) ([]byte, error) {
	r, err := Load(root)
	if os.IsNotExist(err) {
		return os.ReadFile(filepath.Join(root, path))
	}
	if err != nil {
		return nil, err
	}
	for _, f := range r.Files {
		if f.Path == path {
			return f.Content, nil
		}
	}
	return nil, os.ErrNotExist
}

// Update preserves configure's immediate effect for an already shared project.
// New non-secret helpers must still be explicitly included through publish.
func Update(root, path string, content []byte) error {
	lease, err := writeLease(root)
	if err != nil {
		return err
	}
	defer func() { _ = lease.Unlock() }()
	r, err := Load(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	found := false
	for index := range r.Files {
		if r.Files[index].Path == path {
			r.Files[index].Content = content
			found = true
		}
	}
	if !found {
		r.Files = append(r.Files, File{Path: path, Content: content})
	}
	if err = validate(r); err != nil {
		return err
	}
	dir := filepath.Join(Workspace(root), ".archetipo")
	return recordfile.WriteAtomic(dir, filepath.Join(Workspace(root), Path), ".environment-recipe-", r)
}
func validate(r Recipe) error {
	for _, file := range r.Files {
		if file.Path == Environment && !json.Valid(file.Content) {
			return fmt.Errorf("invalid common environment JSON")
		}
		if file.Path == Preview {
			var p struct {
				URL     string   `json:"url"`
				Argv    []string `json:"argv"`
				Prepare bool     `json:"prepare_environment"`
			}
			if err := json.Unmarshal(file.Content, &p); err != nil {
				return err
			}
			if !p.Prepare || !strings.Contains(p.URL, "{port}") || !strings.Contains(strings.Join(p.Argv, "\x00"), "{port}") {
				return iox.NewInvalidInput("common preview requires prepare_environment and a dynamic {port} launch command", "configure a portable workspace preview before publishing", nil)
			}
		}
	}
	return nil
}

func writeLease(root string) (*conversationlog.ConversationLock, error) {
	store, err := conversationlog.NewFileStore(Workspace(root))
	if err != nil {
		return nil, err
	}
	return store.Lock(context.Background(), "environment-recipe-write")
}

// Collect snapshots explicitly selected files without publishing them.
func Collect(root string, assets []string) (Recipe, error) {
	r := Recipe{Version: 1, Files: []File{}}
	names := append([]string{Environment, Preview}, assets...)
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		path, err := safe(root, name)
		if err != nil {
			return r, err
		}
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) && name == Preview {
			continue
		}
		if err != nil {
			return r, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return r, err
		}
		if !info.Mode().IsRegular() {
			return r, fmt.Errorf("recipe asset is not a regular file: %s", name)
		}
		r.Files = append(r.Files, File{Path: name, Content: raw, Executable: info.Mode()&0o111 != 0})
	}
	if err := validate(r); err != nil {
		return r, err
	}
	return r, nil
}

func Digest(r Recipe) string {
	files := append([]File(nil), r.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	raw, _ := json.Marshal(Recipe{Version: r.Version, Files: files})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
