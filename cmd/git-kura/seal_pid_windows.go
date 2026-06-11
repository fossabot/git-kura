//go:build windows

package main

import "golang.org/x/sys/windows"

// pidAlive reports whether the process with the given PID is still running.
// It opens a minimal process handle and calls GetExitCodeProcess; a return
// value of STILL_ACTIVE (259) means the process has not exited.
// If the handle cannot be opened due to ERROR_ACCESS_DENIED the process exists
// but is unqueryable, so we conservatively treat it as alive (mirrors the Unix
// EPERM case). Any other OpenProcess failure is treated as "dead".
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_ACCESS_DENIED {
			return true // process exists but we can't query it
		}
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return true // conservative: assume alive if we can't read exit code
	}
	return exitCode == 259 // STILL_ACTIVE / STATUS_PENDING
}
