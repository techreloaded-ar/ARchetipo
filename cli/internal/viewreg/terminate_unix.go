//go:build !windows

package viewreg

import (
	"os"
	"syscall"
)

// terminate asks the viewer to shut down gracefully. The server installs a
// SIGTERM handler (signal.NotifyContext), so it drains connections and removes
// its own pidfile before exiting.
func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
