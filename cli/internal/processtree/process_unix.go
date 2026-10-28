//go:build !windows

package processtree

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

func isolate(cmd *exec.Cmd) func() error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var once sync.Once
	var stopErr error
	stop := func() error {
		once.Do(func() {
			if cmd.Process == nil {
				return
			}
			// This group was created by this command. Never discover targets by
			// executable name, port or a PID recovered from a stale journal.
			group := -cmd.Process.Pid
			if err := syscall.Kill(group, syscall.SIGTERM); errors.Is(err, syscall.ESRCH) {
				return
			} else if err != nil {
				stopErr = fmt.Errorf("stop owned process group: %w", err)
				return
			}
			deadline := time.Now().Add(750 * time.Millisecond)
			for time.Now().Before(deadline) {
				if errors.Is(syscall.Kill(group, 0), syscall.ESRCH) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := syscall.Kill(group, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				stopErr = fmt.Errorf("kill owned process group: %w", err)
			}
		})
		return stopErr
	}
	cmd.Cancel = stop
	return stop
}
