//go:build windows

package preview

import (
	"os/exec"
	"strconv"
)

func isolate(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
		return cmd.Process.Kill()
	}
}
