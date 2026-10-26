// Package verification executes explicit project recipes. It does not choose a
// framework, provision implicit services, or interpret workflow stages.
package verification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/environmentrecipe"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/recordfile"
)

const RecipePath = ".archetipo/environment.json"

type Command struct {
	Name           string   `json:"name"`
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type Check struct {
	Command
	Inputs        []string `json:"inputs"`
	Cache         bool     `json:"cache"`
	DuringPreview bool     `json:"during_preview,omitempty"`
}

// Resource commands are finite project adapters. Start must register/create only
// the injected owner; Probe and Stop must verify that same identity. Stopped
// succeeds only when that owner's service has actually stopped, preserving data.
type Resource struct {
	Name    string  `json:"name"`
	Start   Command `json:"start"`
	Probe   Command `json:"probe"`
	Stop    Command `json:"stop"`
	Stopped Command `json:"stopped"`
}

type Recipe struct {
	Ports     []string         `json:"ports,omitempty"`
	Resources []Resource       `json:"resources,omitempty"`
	Version   int              `json:"version"`
	Inputs    []string         `json:"inputs"`
	Outputs   []string         `json:"outputs"`
	EnvKeys   []string         `json:"env_keys,omitempty"`
	Prepare   []Command        `json:"prepare"`
	Probe     []Command        `json:"probe"`
	Cleanup   []Command        `json:"cleanup,omitempty"`
	Checks    map[string]Check `json:"checks"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var envPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func Load(root string) (Recipe, error) {
	var recipe Recipe
	raw, err := environmentrecipe.Read(root, RecipePath)
	if os.IsNotExist(err) {
		return recipe, iox.NewPrecondition("no project environment recipe", "create .archetipo/environment.json with explicit project setup, probes and checks; do not guess services or credentials", err)
	}
	if err != nil {
		return recipe, err
	}
	return parseRecipe(root, raw)
}

// LoadLocal reads a candidate without materializing or updating the common bundle.
func LoadLocal(root string) (Recipe, error) {
	raw, err := os.ReadFile(filepath.Join(root, RecipePath))
	if err != nil {
		return Recipe{}, err
	}
	return parseRecipe(root, raw)
}

func Configure(root string, raw []byte) (Recipe, error) {
	return configure(root, raw, false)
}

func ConfigureCandidate(root string, raw []byte) (Recipe, error) {
	return configure(root, raw, true)
}

func configure(root string, raw []byte, candidate bool) (Recipe, error) {
	recipe, err := parseRecipe(root, raw)
	if err != nil {
		return recipe, err
	}
	dir := filepath.Join(root, ".archetipo")
	encoded, err := json.Marshal(recipe)
	if err != nil {
		return recipe, err
	}
	if !candidate {
		if err = environmentrecipe.Update(root, RecipePath, encoded); err != nil {
			return recipe, err
		}
	}
	return recipe, recordfile.WriteAtomic(dir, filepath.Join(root, RecipePath), ".recipe-", recipe)
}

func parseRecipe(root string, raw []byte) (Recipe, error) {
	var recipe Recipe
	var err error
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&recipe); err != nil {
		return recipe, invalid("invalid environment recipe: " + err.Error())
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return recipe, invalid("environment recipe must contain one JSON object")
	}
	if recipe.Version != 1 || len(recipe.Probe) == 0 {
		return recipe, invalid("environment recipe requires version 1 and at least one readiness probe")
	}
	for _, paths := range [][]string{recipe.Inputs, recipe.Outputs} {
		for _, path := range paths {
			if _, err = localPath(root, path); err != nil {
				return recipe, err
			}
		}
	}
	if err = validateResources(recipe); err != nil {
		return recipe, err
	}
	for _, key := range recipe.EnvKeys {
		if !envPattern.MatchString(key) || (strings.HasPrefix(key, "ARCHETIPO_ENVIRONMENT_") || strings.HasPrefix(key, "ARCHETIPO_RESOURCE_") || strings.HasPrefix(key, "ARCHETIPO_PORT_")) {
			return recipe, invalid("invalid environment input key: " + key)
		}
	}
	for _, commands := range [][]Command{recipe.Prepare, recipe.Probe, recipe.Cleanup} {
		if err = validateCommands(commands); err != nil {
			return recipe, err
		}
	}
	for name, check := range recipe.Checks {
		if !namePattern.MatchString(name) || len(check.Inputs) == 0 {
			return recipe, invalid("every check requires a safe name and explicit source inputs")
		}
		if err = validateCommands([]Command{check.Command}); err != nil {
			return recipe, err
		}
		for _, path := range check.Inputs {
			if _, err = localPath(root, path); err != nil {
				return recipe, err
			}
		}
	}
	return recipe, nil
}

func validateCommands(commands []Command) error {
	names := map[string]bool{}
	for _, command := range commands {
		if !namePattern.MatchString(command.Name) || names[command.Name] || len(command.Argv) == 0 || strings.TrimSpace(command.Argv[0]) == "" || command.TimeoutSeconds < 1 || command.TimeoutSeconds > 1800 {
			return invalid("commands require unique names, argv and timeout_seconds between 1 and 1800")
		}
		names[command.Name] = true
		for _, arg := range command.Argv {
			if strings.ContainsRune(arg, 0) {
				return invalid("command argv contains a NUL byte")
			}
		}
	}
	return nil
}

// Reject lexical escapes and symlinks at every existing component, including
// dangling links. Dependency outputs must belong to this checkout.
func localPath(root, rel string) (string, error) {
	if rel == "" || rel == "." || filepath.IsAbs(rel) || strings.ContainsAny(rel, `\:`) || filepath.ToSlash(filepath.Clean(rel)) != rel || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", invalid("expected a project-relative file or directory: " + rel)
	}
	current := root
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", invalid("recipe path traverses a symlink: " + rel)
		}
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

func invalid(message string) error {
	return iox.NewInvalidInput(message, fmt.Sprintf("use the version 1 %s contract; paths and resources must belong to this checkout", RecipePath), nil)
}

func validateResources(recipe Recipe) error {
	names := map[string]bool{}
	if len(recipe.Resources) > 16 || len(recipe.Ports) > 32 {
		return invalid("at most 16 resources and 32 ports are supported")
	}
	for _, port := range recipe.Ports {
		if !envPattern.MatchString(port) || names[port] {
			return invalid("ports require unique uppercase environment names")
		}
		names[port] = true
	}
	names = map[string]bool{}
	for _, r := range recipe.Resources {
		if !namePattern.MatchString(r.Name) || names[r.Name] {
			return invalid("resources require unique safe names")
		}
		names[r.Name] = true
		// Names may repeat between lifecycle operations; they are logged separately.
		for _, c := range []Command{r.Start, r.Probe, r.Stop, r.Stopped} {
			if err := validateCommands([]Command{c}); err != nil {
				return err
			}
			if c.TimeoutSeconds > 30 {
				return invalid("resource commands must finish within 30 seconds")
			}
		}
	}
	if len(recipe.Ports) > 0 && len(recipe.Resources) == 0 {
		return invalid("named ports require managed resources")
	}
	return nil
}
