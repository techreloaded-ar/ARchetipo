package wiki

import (
	"errors"
	"fmt"
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
	byPath           map[string][]gitIndexEntry
	portablePrefixes map[string]map[string]struct{}
}

func loadGitIndexWithRunner(projectRoot string, runner gitCommandRunner) (*gitIndex, error) {
	raw, err := runner.Output(projectRoot, nil, "ls-files", "--stage", "-z")
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
	if err := index.validatePathIdentity(path); err != nil {
		return nil, err
	}
	return index.exactEntry(path)
}

func (index *gitIndex) exactEntry(path string) (*gitIndexEntry, error) {
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

// validatePathIdentity enforces exact index spelling component by component.
// It scans only prefixes portable-equivalent to the cited path, so collisions
// elsewhere in the repository do not poison unrelated evidence.
func (index *gitIndex) validatePathIdentity(path string) error {
	normalized, err := normalizePortableEvidencePath(path)
	if err != nil {
		return err
	}
	if normalized == "." {
		return nil
	}
	index.ensurePortablePrefixes()
	components := strings.Split(normalized, "/")
	keys := make([]string, 0, len(components))
	for depth, component := range components {
		key, keyErr := portableComponentKey(component)
		if keyErr != nil {
			return keyErr
		}
		keys = append(keys, key)
		spellings := index.portablePrefixes[strings.Join(keys, "/")]
		if len(spellings) > 1 {
			return errors.Join(ErrEvidenceUnreadable, errPortablePathCollision)
		}
		for spelling := range spellings {
			if spelling != strings.Join(components[:depth+1], "/") {
				return errors.Join(ErrInvalidSourcePath, errNonCanonicalPath)
			}
		}
	}
	return nil
}

func (index *gitIndex) ensurePortablePrefixes() {
	if index.portablePrefixes != nil {
		return
	}
	index.portablePrefixes = map[string]map[string]struct{}{}
	for indexPath := range index.byPath {
		components := strings.Split(indexPath, "/")
		keys := make([]string, 0, len(components))
		for depth, component := range components {
			// Git records slash separators on every host. A literal backslash is
			// therefore a non-portable separator alias, not a valid index
			// component. Preserve every valid ancestor before the first invalid
			// component so an invalid descendant cannot hide an intersecting
			// case/NFC collision.
			if component == "" || strings.Contains(component, `\`) {
				break
			}
			key, err := portableComponentKey(component)
			if err != nil {
				break
			}
			keys = append(keys, key)
			portablePrefix := strings.Join(keys, "/")
			if index.portablePrefixes[portablePrefix] == nil {
				index.portablePrefixes[portablePrefix] = map[string]struct{}{}
			}
			index.portablePrefixes[portablePrefix][strings.Join(components[:depth+1], "/")] = struct{}{}
		}
	}
}

func (index *gitIndex) strictGitlinkAncestor(path string) (*gitIndexEntry, error) {
	if err := index.validatePathIdentity(path); err != nil {
		return nil, err
	}
	components := strings.Split(path, "/")
	for count := 1; count < len(components); count++ {
		ancestor := strings.Join(components[:count], "/")
		entry, err := index.exactEntry(ancestor)
		if err != nil {
			return nil, err
		}
		if entry != nil && entry.Mode == "160000" {
			return entry, nil
		}
	}
	return nil, nil
}

func (index *gitIndex) pathsWithin(parent string) ([]string, error) {
	if err := index.validatePathIdentity(parent); err != nil {
		return nil, err
	}
	paths := []string{}
	for indexPath := range index.byPath {
		if !pathContains(parent, indexPath) {
			continue
		}
		normalized, err := normalizePortableEvidencePath(indexPath)
		if err != nil || normalized != indexPath {
			return nil, fmt.Errorf("%w: non-portable Git index path %q", ErrEvidenceUnreadable, indexPath)
		}
		if err := index.validatePathIdentity(indexPath); err != nil {
			return nil, err
		}
		entry, err := index.exactEntry(indexPath)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			paths = append(paths, indexPath)
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
