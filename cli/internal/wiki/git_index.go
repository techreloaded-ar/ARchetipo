package wiki

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type gitIndexEntry struct {
	Mode  string
	OID   string
	Stage int
	Path  string
}

type gitIndex struct {
	byPath map[string][]gitIndexEntry
}

func loadGitIndex(projectRoot string) (*gitIndex, error) {
	cmd := exec.Command("git", "ls-files", "--stage", "-z")
	cmd.Dir = projectRoot
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: listing Git index: %v", ErrEvidenceUnreadable, err)
	}
	entries, err := parseGitIndex(raw)
	if err != nil {
		return nil, err
	}
	index := &gitIndex{byPath: map[string][]gitIndexEntry{}}
	for _, entry := range entries {
		index.byPath[entry.Path] = append(index.byPath[entry.Path], entry)
	}
	return index, nil
}

func parseGitIndex(raw []byte) ([]gitIndexEntry, error) {
	entries := []gitIndexEntry{}
	for _, record := range strings.Split(string(raw), "\x00") {
		if record == "" {
			continue
		}
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("%w: malformed Git index record", ErrEvidenceUnreadable)
		}
		metadata := strings.Fields(record[:tab])
		if len(metadata) != 3 || metadata[0] == "" || metadata[1] == "" {
			return nil, fmt.Errorf("%w: malformed Git index metadata", ErrEvidenceUnreadable)
		}
		stage, err := strconv.Atoi(metadata[2])
		if err != nil || stage < 0 || stage > 3 {
			return nil, fmt.Errorf("%w: malformed Git index stage", ErrEvidenceUnreadable)
		}
		path := record[tab+1:]
		if path == "" {
			return nil, fmt.Errorf("%w: empty Git index path", ErrEvidenceUnreadable)
		}
		entries = append(entries, gitIndexEntry{Mode: metadata[0], OID: metadata[1], Stage: stage, Path: path})
	}
	return entries, nil
}

func (index *gitIndex) entry(path string) (*gitIndexEntry, error) {
	entries := index.byPath[path]
	var stageZero *gitIndexEntry
	for entryIndex := range entries {
		entry := &entries[entryIndex]
		if entry.Stage != 0 {
			return nil, fmt.Errorf("%w: %s", ErrGitIndexConflict, path)
		}
		if stageZero != nil {
			return nil, fmt.Errorf("%w: duplicate stage-0 entry for %s", ErrEvidenceUnreadable, path)
		}
		stageZero = entry
	}
	return stageZero, nil
}

func (index *gitIndex) pathsWithin(parent string) ([]string, error) {
	paths := []string{}
	for path := range index.byPath {
		if !pathContains(parent, path) {
			continue
		}
		entry, err := index.entry(path)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			paths = append(paths, path)
		}
	}
	return uniqueSorted(paths), nil
}

func gitIndexMode(entry *gitIndexEntry) (executable string, regular, symlink bool, err error) {
	if entry == nil {
		return "0", false, false, nil
	}
	switch entry.Mode {
	case "100644":
		return "0", true, false, nil
	case "100755":
		return "1", true, false, nil
	case "120000":
		return "", false, true, nil
	default:
		return "", false, false, fmt.Errorf("%w: Git index mode %s for %s", ErrUnsupportedEvidenceEntry, entry.Mode, entry.Path)
	}
}
