//go:build linux

package conversationlog

import (
	"os"
	"strconv"
	"strings"
)

// processIdentity tells a process from a later one that reuses its pid: the
// kernel's boot and the process start time. In a container every restart
// gives the main process the same pid, while the locks stay on the volume.
func processIdentity(pid int) string {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	// The command name may contain spaces; the fields after it do not.
	stat := string(raw)
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return ""
	}
	fields := strings.Fields(stat[end+1:])
	// starttime is field 22 of stat, the 20th after the command name.
	if len(fields) < 20 {
		return ""
	}
	boot, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(boot)) + ":" + fields[19]
}
