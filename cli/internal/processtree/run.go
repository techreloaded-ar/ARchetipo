// Package processtree owns the subprocesses of a foreground command. Persistent
// dependencies must instead use the environment's registered resource adapters.
package processtree

import (
	"errors"
	"os/exec"
)

// SeparateSupervisor keeps a private-pipe cleanup owner alive when a harness
// terminates its caller's entire POSIX process group. The supervisor still
// exits on control-pipe EOF; this is not a detached application service.
func SeparateSupervisor(cmd *exec.Cmd) { separateSupervisor(cmd) }

// Run stops remaining children even when the launcher exits before cancellation.
// Cleanup completes before the caller can release its environment lock.
func Run(cmd *exec.Cmd) error {
	cleanup := isolate(cmd)
	err := cmd.Run()
	return errors.Join(err, cleanup())
}
