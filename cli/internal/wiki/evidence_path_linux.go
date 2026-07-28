//go:build linux

package wiki

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type platformEvidenceRoot struct {
	mountID int
}

func newPlatformEvidenceRoot(root string) (platformEvidenceRoot, error) {
	mountID, err := linuxMountID(root)
	if err != nil {
		return platformEvidenceRoot{}, errors.Join(ErrEvidenceUnreadable, err)
	}
	return platformEvidenceRoot{mountID: mountID}, nil
}

func validatePlatformEvidenceComponent(root platformEvidenceRoot, path, _ string, _ bool) error {
	mountID, err := linuxMountID(path)
	if err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	return classifyLinuxMountIdentity(root.mountID, mountID)
}

func classifyLinuxMountIdentity(rootMountID, candidateMountID int) error {
	if rootMountID != candidateMountID {
		return ErrUnsafeSourcePath
	}
	return nil
}

func linuxMountID(path string) (int, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	bestLength := -1
	bestID := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 {
			continue
		}
		mountID, parseErr := strconv.Atoi(fields[0])
		if parseErr != nil {
			continue
		}
		mountPoint, decodeErr := decodeLinuxMountInfoPath(fields[4])
		if decodeErr != nil {
			continue
		}
		mountPoint = filepath.Clean(mountPoint)
		if !pathIsWithinMount(mountPoint, absolute) || len(mountPoint) <= bestLength {
			continue
		}
		bestLength = len(mountPoint)
		bestID = mountID
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if bestLength < 0 {
		return 0, errors.New("mount identity is unavailable")
	}
	return bestID, nil
}

func pathIsWithinMount(mountPoint, candidate string) bool {
	relative, err := filepath.Rel(mountPoint, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func decodeLinuxMountInfoPath(encoded string) (string, error) {
	var decoded strings.Builder
	for index := 0; index < len(encoded); index++ {
		if encoded[index] != '\\' {
			decoded.WriteByte(encoded[index])
			continue
		}
		if index+3 >= len(encoded) {
			return "", errors.New("malformed mountinfo path escape")
		}
		value, err := strconv.ParseUint(encoded[index+1:index+4], 8, 8)
		if err != nil {
			return "", errors.New("malformed mountinfo path escape")
		}
		decoded.WriteByte(byte(value))
		index += 3
	}
	return decoded.String(), nil
}
