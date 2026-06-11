//go:build !windows

package main

import (
	"os"
	"syscall"
)

// pidAlive reports whether a process with the given PID is running.
// On Unix it uses kill(pid, 0) which succeeds for both owned and unowned
// living processes.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
