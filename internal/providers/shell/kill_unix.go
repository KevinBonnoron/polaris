//go:build unix && !linux

package shell

import "syscall"

// killTerminalProcesses kills the shell's process group. On non-Linux Unix
// platforms we cannot enumerate all session processes efficiently, so we fall
// back to killing the shell's own process group (PGID == shell PID because
// pty.Start sets Setsid=true).
func killTerminalProcesses(shellPID int) {
	_ = syscall.Kill(-shellPID, syscall.SIGKILL)
}
