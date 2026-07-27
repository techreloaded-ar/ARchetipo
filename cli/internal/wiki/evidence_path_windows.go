//go:build windows

package wiki

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsReparseTagNameSurrogate = 0x20000000

type windowsFileAttributeTagInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
}

func validatePlatformEvidenceComponent(path string, terminal bool) error {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	handle, err := windows.CreateFile(
		path16,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	defer windows.CloseHandle(handle)

	var info windowsFileAttributeTagInfo
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return errors.Join(ErrEvidenceUnreadable, err)
	}
	return classifyWindowsReparse(info.FileAttributes, info.ReparseTag, terminal)
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
