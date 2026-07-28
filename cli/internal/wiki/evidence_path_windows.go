//go:build windows

package wiki

import (
	"errors"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsReparseTagNameSurrogate = 0x20000000

type windowsFileAttributeTagInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
}

type platformEvidenceRoot struct {
	volumeSerial uint32
}

func newPlatformEvidenceRoot(root string) (platformEvidenceRoot, error) {
	handle, err := openWindowsEvidenceHandle(root)
	if err != nil {
		return platformEvidenceRoot{}, errors.Join(ErrEvidenceUnreadable, err)
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return platformEvidenceRoot{}, errors.Join(ErrEvidenceUnreadable, err)
	}
	return platformEvidenceRoot{volumeSerial: info.VolumeSerialNumber}, nil
}

func validatePlatformEvidenceComponent(root platformEvidenceRoot, path, expectedName string, terminal bool) error {
	handle, err := openWindowsEvidenceHandle(path)
	if err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	defer windows.CloseHandle(handle)

	var before windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &before); err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	if before.VolumeSerialNumber != root.volumeSerial {
		return ErrUnsafeSourcePath
	}

	var tagInfo windowsFileAttributeTagInfo
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&tagInfo)),
		uint32(unsafe.Sizeof(tagInfo)),
	); err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	if err := classifyWindowsReparse(tagInfo.FileAttributes, tagInfo.ReparseTag, terminal); err != nil {
		return err
	}

	canonical, err := windowsFinalPathName(handle)
	if err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	if filepath.Base(canonical) != expectedName {
		return errors.Join(ErrInvalidSourcePath, errNonCanonicalPath)
	}

	var after windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &after); err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	if !sameWindowsFileIdentity(before, after) {
		return errors.Join(ErrEvidenceUnreadable, errors.New("evidence entry identity changed during inspection"))
	}
	return nil
}

func openWindowsEvidenceHandle(path string) (windows.Handle, error) {
	path16, err := windows.UTF16PtrFromString(windowsLongPath(path))
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(
		path16,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
}

func windowsLongPath(path string) string {
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, `\\?\`) {
		return clean
	}
	if strings.HasPrefix(clean, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(clean, `\\`)
	}
	return `\\?\` + clean
}

func windowsFinalPathName(handle windows.Handle) (string, error) {
	buffer := make([]uint16, 512)
	for {
		length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if length < uint32(len(buffer)) {
			return windows.UTF16ToString(buffer[:length]), nil
		}
		buffer = make([]uint16, length+1)
	}
}

func sameWindowsFileIdentity(left, right windows.ByHandleFileInformation) bool {
	return left.VolumeSerialNumber == right.VolumeSerialNumber &&
		left.FileIndexHigh == right.FileIndexHigh &&
		left.FileIndexLow == right.FileIndexLow
}

func classifyWindowsReparse(attributes, tag uint32, terminal bool) error {
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return nil
	}
	if !terminal || tag == windows.IO_REPARSE_TAG_MOUNT_POINT || tag&windowsReparseTagNameSurrogate != 0 && tag != windows.IO_REPARSE_TAG_SYMLINK {
		return ErrUnsafeSourcePath
	}
	if tag == windows.IO_REPARSE_TAG_SYMLINK {
		return nil
	}
	// Cloud placeholders and other non-redirection reparses still have
	// filesystem-provider-specific semantics. Unsupported terminal types fail
	// closed rather than being read through as ordinary evidence.
	return ErrUnsupportedEvidenceEntry
}
