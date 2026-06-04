//go:build !windows

package sysexec

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup makes cmd the leader of a new process group, so the whole
// tree it spawns (e.g. the package manager plus the dev server it launches)
// shares one PGID and can be signaled at once. Must be called before cmd.Start.
func SetProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// InterruptGroup sends SIGINT to the entire process group led by cmd, giving
// every process in the tree a chance to shut down gracefully. Signaling the
// negative PID targets the group rather than only the leader.
func InterruptGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}

// KillGroup force-kills the entire process group led by cmd.
func KillGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
