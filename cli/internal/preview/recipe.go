// Package preview runs an explicit browser preview recipe. Project discovery
// belongs to the agent; this package never guesses frameworks or commands.
package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

const RecipePath = ".archetipo/preview.json"

type Recipe struct {
	Version   int      `json:"version"`
	Directory string   `json:"directory"`
	Argv      []string `json:"argv,omitempty"`
	URL       string   `json:"url"`
}

func invalid(message string) error {
	return iox.NewInvalidInput(message, "configure an explicit version 1 preview recipe with directory, argv and a loopback HTTP URL", nil)
}

func (r Recipe) Validate(root string) (string, error) {
	if r.Version != 1 {
		return "", invalid("preview version must be 1")
	}
	rel := r.Directory
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, `\:`) || filepath.ToSlash(filepath.Clean(rel)) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", invalid("preview directory must be relative to the conversation working directory")
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	actual, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", invalid("preview directory does not exist")
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(canonicalRoot, actual)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", invalid("preview directory escapes the working directory")
	}
	info, err := os.Stat(actual)
	if err != nil || !info.IsDir() {
		return "", invalid("preview directory is not a directory")
	}
	u, err := url.Parse(r.URL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Fragment != "" || u.Host == "" {
		return "", invalid("preview URL must be a loopback HTTP URL without credentials or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", invalid("preview URL must address localhost or a loopback IP")
	}
	for _, arg := range r.Argv {
		if strings.ContainsRune(arg, 0) {
			return "", invalid("preview argv contains a NUL byte")
		}
	}
	if len(r.Argv) > 0 && strings.TrimSpace(r.Argv[0]) == "" {
		return "", invalid("preview command is empty")
	}
	return actual, nil
}

func Load(root string) (Recipe, error) {
	var r Recipe
	raw, err := os.ReadFile(filepath.Join(root, RecipePath))
	if os.IsNotExist(err) {
		return r, iox.NewPrecondition("preview is not configured", "ask the agent to configure archetipo execution preview configure in this conversation's working directory", err)
	}
	if err != nil {
		return r, err
	}
	return parse(root, raw)
}
func parse(root string, raw []byte) (Recipe, error) {
	var r Recipe
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, invalid(err.Error())
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return r, invalid("expected one preview recipe")
	}
	_, err := r.Validate(root)
	return r, err
}
func Configure(root string, raw []byte) (Recipe, error) {
	r, err := parse(root, raw)
	if err != nil {
		return r, err
	}
	dir := filepath.Join(root, ".archetipo")
	if err = recordfile.WriteAtomic(dir, filepath.Join(root, RecipePath), ".preview-", r); err != nil {
		return r, fmt.Errorf("saving preview recipe: %w", err)
	}
	return r, nil
}
