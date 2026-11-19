//go:build !windows

package services

import (
	"os/exec"
	"syscall"
)

// detach starts a server in its own session, so it outlives the operation
// that started it and the process group the lifecycle ends after each step.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
