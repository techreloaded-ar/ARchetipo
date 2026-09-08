//go:build !windows

package conversationlog

import "syscall"

func processAlive(pid int) bool {
	return pid > 0 && (syscall.Kill(pid, 0) == nil || syscall.Kill(pid, 0) == syscall.EPERM)
}
