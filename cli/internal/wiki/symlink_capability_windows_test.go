//go:build windows

package wiki

import (
	"errors"

	"golang.org/x/sys/windows"
)

func symlinkCapabilityUnavailable(err error) bool {
	return errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) ||
		errors.Is(err, windows.ERROR_NOT_SUPPORTED)
}
