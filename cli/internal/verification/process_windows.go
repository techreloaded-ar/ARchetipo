//go:build windows

package verification

import (
	"os/exec"
	"strconv"
)

func isolateProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		// taskkill /T includes subprocesses that otherwise keep log handles open.
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
		return cmd.Process.Kill()
	}
}
