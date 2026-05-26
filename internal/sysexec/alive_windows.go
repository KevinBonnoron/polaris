//go:build windows

package sysexec

import "syscall"

// stillActive is the exit code Windows reports for a process that has not yet
// terminated (STILL_ACTIVE).
const stillActive = 259

// ProcessAlive reports whether a process with the given pid is currently
// running. It opens a query handle and inspects the exit code: a handle that
// cannot be opened means the process is gone, and an exit code other than
// STILL_ACTIVE means it has terminated.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
