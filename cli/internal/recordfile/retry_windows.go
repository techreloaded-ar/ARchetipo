package recordfile

import (
	"errors"
	"syscall"
	"time"
)

func retryFileOperation(operation func() error) error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err := operation()
		if err == nil || (!errors.Is(err, syscall.ERROR_ACCESS_DENIED) && !errors.Is(err, syscall.Errno(32)) && !errors.Is(err, syscall.Errno(33))) || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
}
