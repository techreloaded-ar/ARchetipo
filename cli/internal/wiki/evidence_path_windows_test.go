//go:build windows

package wiki

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsReparseClassification(t *testing.T) {
	const (
		unknownNameSurrogate = uint32(0xA000001D)
		unknownReparse       = uint32(0x8000001E)
	)
	tests := []struct {
		name       string
		attributes uint32
		tag        uint32
		terminal   bool
		want       error
	}{
		{name: "ordinary intermediate", terminal: false},
		{name: "ordinary terminal", terminal: true},
		{name: "terminal symlink", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: windows.IO_REPARSE_TAG_SYMLINK, terminal: true},
		{name: "intermediate symlink", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: windows.IO_REPARSE_TAG_SYMLINK, terminal: false, want: ErrUnsafeSourcePath},
		{name: "terminal junction or mount point", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: windows.IO_REPARSE_TAG_MOUNT_POINT, terminal: true, want: ErrUnsafeSourcePath},
		{name: "terminal unknown name surrogate", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: unknownNameSurrogate, terminal: true, want: ErrUnsafeSourcePath},
		{name: "terminal unsupported reparse", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: unknownReparse, terminal: true, want: ErrUnsupportedEvidenceEntry},
		{name: "intermediate unsupported reparse", attributes: windows.FILE_ATTRIBUTE_REPARSE_POINT, tag: unknownReparse, terminal: false, want: ErrUnsafeSourcePath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyWindowsReparse(test.attributes, test.tag, test.terminal)
			if !errors.Is(err, test.want) || test.want == nil && err != nil {
				t.Fatalf("classifyWindowsReparse() error=%v want=%v", err, test.want)
			}
		})
	}
}
