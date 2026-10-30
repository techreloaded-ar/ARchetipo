//go:build !windows

package recordfile

func retryFileOperation(operation func() error) error { return operation() }
