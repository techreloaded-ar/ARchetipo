//go:build windows

package viewreg

import "os"

// terminate stops the viewer. Windows has no SIGTERM, and delivering a console
// control event across processes is unreliable, so we fall back to a hard kill
// (TerminateProcess). The viewer's deferred pidfile cleanup does not run in
// this case, so Stop removes the pidfile itself.
func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
