//go:build windows

package processtree

import (
	"os/exec"
	"strconv"
)

// Windows process-tree cancellation retains its existing taskkill contract.
func separateSupervisor(_ *exec.Cmd) {}

func isolate(cmd *exec.Cmd) func() error {
	cmd.Cancel = func() error {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
		return cmd.Process.Kill()
	}
	// Windows retains its taskkill cancellation contract. An already exited
	// launcher's descendants cannot be recovered safely from its old PID.
	return func() error { return nil }
}
