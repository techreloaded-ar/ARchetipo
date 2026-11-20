//go:build windows

package conversationlog

import "golang.org/x/sys/windows"

// exitCodeOfRunningProcess is what Windows reports as the exit code of a
// process that has not ended yet (STILL_ACTIVE).
const exitCodeOfRunningProcess = 259

// processAlive reports whether the process is still running. Opening the pid
// is not enough on Windows: the process object outlives its process for as
// long as anyone holds a handle to it (the parent that started it, an
// antivirus), and stays openable meanwhile. Only a process that has no exit
// code yet is alive.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var exitCode uint32
	if err = windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		// The process exists and its state cannot be read: never recover the
		// record of an owner that may still be running.
		return true
	}
	return exitCode == exitCodeOfRunningProcess
}
