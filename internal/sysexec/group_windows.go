//go:build windows

package sysexec

import (
	"os/exec"
	"strconv"
)

// SetProcessGroup is a no-op on Windows: process-tree termination is handled by
// taskkill /T, which walks descendants by parent PID rather than by group.
func SetProcessGroup(_ *exec.Cmd) {}

// InterruptGroup requests termination of the process tree rooted at cmd.
// Windows has no cross-process SIGINT we can reliably deliver, so this is a
// best-effort graceful attempt via taskkill without /F.
func InterruptGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

// KillGroup force-terminates the process tree rooted at cmd.
func KillGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
