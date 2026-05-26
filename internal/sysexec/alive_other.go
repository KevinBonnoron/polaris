//go:build !windows

package sysexec

import "syscall"

// ProcessAlive reports whether a process with the given pid is currently
// running. Signal 0 performs the kernel's existence/permission check without
// actually delivering a signal: nil means the process exists and we may signal
// it; EPERM means it exists but is owned by another user (still alive); ESRCH
// means no such process.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
