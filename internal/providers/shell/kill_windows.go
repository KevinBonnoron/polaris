//go:build windows

package shell

import (
	"os/exec"
	"strconv"
)

// killTerminalProcesses terminates the shell process and its entire child tree.
// Windows has no process groups equivalent to Unix sessions, so we rely on
// taskkill's /T flag to walk and kill descendants.
func killTerminalProcesses(shellPID int) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(shellPID)).Run()
}
