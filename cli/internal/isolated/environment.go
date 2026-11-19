// Package isolated runs the whole viewer inside a container environment that
// cannot read the person's files or reach their local network (ADR 0004).
//
// The host side only orchestrates: it records environments, asks an
// environment provider to create and start them, and opens the browser. What
// runs inside — viewer, agents, recipes, previews — uses the ordinary CLI
// contracts and needs no isolation logic of its own.
package isolated

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/version"
)

// DefaultID names the single shared environment of this version.
const DefaultID = "default"

// Every environment owns one block of host-loopback ports: the viewer on the
// first one, the preview range right after it.
const (
	firstPortBlock = 47800
	portBlockSize  = 100
	previewOffset  = 10
	previewPorts   = 20
	maxPortBlocks  = 50
)

// EnvImage overrides the image, for development builds and private registries.
const EnvImage = "ARCHETIPO_ISOLATED_IMAGE"

// EnvFirstPort overrides the first host-loopback port block. It lets CI and
// parallel local verification avoid a running product environment without
// changing the recorded identity or the size of a block.
const EnvFirstPort = "ARCHETIPO_ISOLATED_FIRST_PORT"

const (
	releaseImage = "ghcr.io/techreloaded-ar/archetipo-isolated"
	devImage     = "archetipo-isolated:latest"
)

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Environment is one gateway, one workspace container, one network and one
// volume. Every Docker object name and every published port derives from it,
// so a second environment never collides with the first.
type Environment struct {
	ID        string    `json:"id"`
	ViewPort  int       `json:"view_port"`
	Image     string    `json:"image"`
	CreatedAt time.Time `json:"created_at"`
}

func (e Environment) prefix() string    { return "archetipo-" + e.ID }
func (e Environment) Gateway() string   { return e.prefix() + "-gateway" }
func (e Environment) Workspace() string { return e.prefix() + "-workspace" }
func (e Environment) Network() string   { return e.prefix() + "-net" }
func (e Environment) Volume() string    { return e.prefix() + "-home" }
func (e Environment) PreviewFirst() int { return e.ViewPort + previewOffset }
func (e Environment) PreviewLast() int  { return e.PreviewFirst() + previewPorts - 1 }
func (e Environment) URL() string       { return fmt.Sprintf("http://127.0.0.1:%d/", e.ViewPort) }
func (e Environment) PreviewRange() string {
	return fmt.Sprintf("%d-%d", e.PreviewFirst(), e.PreviewLast())
}

// ValidateID refuses identifiers that cannot safely name Docker objects.
func ValidateID(id string) error {
	if !validID.MatchString(id) {
		return iox.NewInvalidInput("invalid environment id: "+id, "use lowercase letters, digits and dashes, at most 32 characters", nil)
	}
	return nil
}

// DefaultImage is the image of this release, or the locally built one for a
// development binary.
func DefaultImage() string {
	if image := strings.TrimSpace(os.Getenv(EnvImage)); image != "" {
		return image
	}
	return imageFor(version.Version)
}

// imageFor maps a CLI version to its image. Development builds, including the
// ones installed with `npm run install:dev` (0.0.0-dev.g<sha>), use the image
// built locally from the same checkout: no registry holds one for them.
func imageFor(v string) string {
	if v == "" || v == "dev" || strings.Contains(v, "-dev") {
		return devImage
	}
	return releaseImage + ":" + strings.TrimPrefix(v, "v")
}

// Store records environments in the host's user-level state directory.
type Store struct{ Dir string }

const storeFile = "environments.json"

type storeDocument struct {
	Version      int           `json:"version"`
	Environments []Environment `json:"environments"`
}

func (s Store) path() string { return filepath.Join(s.Dir, storeFile) }

func (s Store) load() ([]Environment, error) {
	raw, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc storeDocument
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 {
		return nil, iox.NewPrecondition("isolated environment records are unreadable", "inspect or remove "+s.path(), err)
	}
	return doc.Environments, nil
}

func (s Store) save(envs []Environment) error {
	sort.Slice(envs, func(i, j int) bool { return envs[i].ID < envs[j].ID })
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	return recordfile.WriteAtomic(s.Dir, s.path(), ".environments-", storeDocument{Version: 1, Environments: envs})
}

// Ensure returns the recorded environment, or records a new one on the first
// free port block. The image of an existing environment follows the running
// binary, so an update reaches it on the next start.
func (s Store) Ensure(id, image string) (Environment, error) {
	if err := ValidateID(id); err != nil {
		return Environment{}, err
	}
	envs, err := s.load()
	if err != nil {
		return Environment{}, err
	}
	used := map[int]bool{}
	for index, env := range envs {
		if env.ID == id {
			if env.Image != image {
				envs[index].Image = image
				if err = s.save(envs); err != nil {
					return Environment{}, err
				}
			}
			return envs[index], nil
		}
		used[env.ViewPort] = true
	}
	firstPort := firstPortBlock
	if configured := strings.TrimSpace(os.Getenv(EnvFirstPort)); configured != "" {
		firstPort, err = strconv.Atoi(configured)
		if err != nil || firstPort < 1024 || firstPort+previewOffset+previewPorts-1 > 65535 {
			return Environment{}, iox.NewInvalidInput("invalid "+EnvFirstPort+": "+configured, "use a base TCP port between 1024 and 65506", err)
		}
	}
	for block := 0; block < maxPortBlocks; block++ {
		port := firstPort + block*portBlockSize
		if port+previewOffset+previewPorts-1 > 65535 {
			break
		}
		if used[port] {
			continue
		}
		env := Environment{ID: id, ViewPort: port, Image: image, CreatedAt: time.Now().UTC()}
		if err = s.save(append(envs, env)); err != nil {
			return Environment{}, err
		}
		return env, nil
	}
	return Environment{}, iox.NewConflict("no free port block for a new isolated environment", "remove an environment you no longer use", nil)
}

// Get returns a recorded environment.
func (s Store) Get(id string) (Environment, bool, error) {
	envs, err := s.load()
	if err != nil {
		return Environment{}, false, err
	}
	for _, env := range envs {
		if env.ID == id {
			return env, true, nil
		}
	}
	return Environment{}, false, nil
}

// Forget drops a record, after its environment was destroyed.
func (s Store) Forget(id string) error {
	envs, err := s.load()
	if err != nil {
		return err
	}
	kept := envs[:0]
	for _, env := range envs {
		if env.ID != id {
			kept = append(kept, env)
		}
	}
	return s.save(kept)
}
