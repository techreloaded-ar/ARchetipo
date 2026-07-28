//go:build windows

package wiki

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openEvidenceRegular keeps one no-follow native handle through the content
// read (including Git stdin) and verifies that handle's stable identity after
// the read. The resolver has already attested every parent component.
func openEvidenceRegular(path string) (*os.File, func() error, error) {
	path16, err := windows.UTF16PtrFromString(windowsLongPath(path))
	if err != nil {
		return nil, nil, err
	}
	handle, err := windows.CreateFile(
		path16,
		windows.GENERIC_READ|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, nil, err
	}
	var before windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &before); err != nil {
		windows.CloseHandle(handle)
		return nil, nil, err
	}
	var tagInfo windowsFileAttributeTagInfo
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&tagInfo)),
		uint32(unsafe.Sizeof(tagInfo)),
	); err != nil {
		windows.CloseHandle(handle)
		return nil, nil, err
	}
	if tagInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(handle)
		return nil, nil, ErrUnsafeSourcePath
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		windows.CloseHandle(handle)
		return nil, nil, errors.New("creating evidence file from native handle")
	}
	verify := func() error {
		var after windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(handle, &after); err != nil {
			return err
		}
		if !sameWindowsFileIdentity(before, after) {
			return errors.New("evidence entry identity changed during read")
		}
		return nil
	}
	return file, verify, nil
}
