package recordfile

import (
	"errors"
	"syscall"
	"testing"
)

func TestRetriesOnlyTransientWindowsFileErrors(t *testing.T) {
	attempts := 0
	if err := retryFileOperation(func() error {
		attempts++
		if attempts < 3 {
			return syscall.Errno(32)
		}
		return nil
	}); err != nil || attempts != 3 {
		t.Fatalf("sharing violation: attempts=%d, error=%v", attempts, err)
	}
	permanent := errors.New("invalid record")
	attempts = 0
	if err := retryFileOperation(func() error { attempts++; return permanent }); err != permanent || attempts != 1 {
		t.Fatalf("permanent error retried: attempts=%d, error=%v", attempts, err)
	}
}
