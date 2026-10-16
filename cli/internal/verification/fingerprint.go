package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// Only declared inputs are hashed. The recipe is responsible for declaring all
// code, tests, configuration and toolchain inputs of a cached check. No timestamp
// or Git HEAD shortcut can certify a dirty candidate.
func fingerprint(root string, recipe Recipe, inputs []string, extra any) (string, error) {
	h := sha256.New()
	encoded, _ := json.Marshal(struct {
		Recipe Recipe
		Extra  any
	}{recipe, extra})
	_, _ = h.Write(encoded)
	_, _ = fmt.Fprintln(h, root, runtime.GOOS, runtime.GOARCH, os.Getenv("PATH"))
	for _, key := range recipe.EnvKeys {
		value, present := os.LookupEnv(key)
		_, _ = fmt.Fprintf(h, "%s\x00%t\x00%s\x00", key, present, value)
	}
	commands := append(append(append([]Command{}, recipe.Prepare...), recipe.Probe...), recipe.Cleanup...)
	for _, check := range recipe.Checks {
		commands = append(commands, check.Command)
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Argv[0] < commands[j].Argv[0] })
	for _, command := range commands {
		path, err := exec.LookPath(command.Argv[0])
		if err != nil {
			_, _ = fmt.Fprintln(h, command.Argv[0], "missing")
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintln(h, path, info.Size(), info.ModTime().UnixNano())
	}
	paths := append([]string{}, inputs...)
	sort.Strings(paths)
	for _, rel := range paths {
		path, err := localPath(root, rel)
		if err != nil {
			return "", err
		}
		if _, err = os.Lstat(path); os.IsNotExist(err) {
			_, _ = fmt.Fprintln(h, rel, "missing")
			continue
		} else if err != nil {
			return "", err
		}
		err = filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return invalid("input contains a symlink: " + path)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(h, filepath.ToSlash(relative), info.Mode())
			if entry.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return invalid("input is not a regular file: " + relative)
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(h, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
