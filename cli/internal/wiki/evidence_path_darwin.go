//go:build darwin

package wiki

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

type platformEvidenceRoot struct {
	fsid    syscall.Fsid
	mountOn string
}

func newPlatformEvidenceRoot(root string) (platformEvidenceRoot, error) {
	identity, err := darwinMountIdentity(root)
	if err != nil {
		return platformEvidenceRoot{}, errors.Join(ErrEvidenceUnreadable, err)
	}
	return identity, nil
}

func validatePlatformEvidenceComponent(root platformEvidenceRoot, path, _ string, terminal bool) error {
	inspectionPath := path
	if terminal {
		if info, err := os.Lstat(path); err != nil {
			return errors.Join(ErrEvidenceUnreadable, err)
		} else if info.Mode()&os.ModeSymlink != 0 {
			inspectionPath = filepath.Dir(path)
		}
	}
	identity, err := darwinMountIdentity(inspectionPath)
	if err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	return classifyDarwinMountIdentity(root, identity)
}

func classifyDarwinMountIdentity(root, candidate platformEvidenceRoot) error {
	if candidate.fsid != root.fsid || candidate.mountOn != root.mountOn {
		return ErrUnsafeSourcePath
	}
	return nil
}

func darwinMountIdentity(path string) (platformEvidenceRoot, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return platformEvidenceRoot{}, err
	}
	mountOn := darwinInt8String(stat.Mntonname[:])
	if mountOn == "" {
		return platformEvidenceRoot{}, errors.New("mounted-on identity is unavailable")
	}
	return platformEvidenceRoot{fsid: stat.Fsid, mountOn: mountOn}, nil
}

func darwinInt8String(value []int8) string {
	bytes := make([]byte, 0, len(value))
	for _, item := range value {
		if item == 0 {
			break
		}
		bytes = append(bytes, byte(item))
	}
	return string(bytes)
}
