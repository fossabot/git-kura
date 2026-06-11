//go:build windows

package main

import "os"

// pidAlive reports whether a process with the given PID is running.
// On Windows, os.FindProcess only opens a handle and does not verify liveness;
// this is best-effort for v0. A proper check would require OpenProcess from
// golang.org/x/sys/windows, which is deferred to a future improvement.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.FindProcess(pid)
	return err == nil
}
