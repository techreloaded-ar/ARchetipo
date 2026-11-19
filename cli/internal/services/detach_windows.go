//go:build windows

package services

import "os/exec"

// detach has nothing to do on Windows, where installed runtimes are not used:
// the isolated environment is always Linux.
func detach(*exec.Cmd) {}
